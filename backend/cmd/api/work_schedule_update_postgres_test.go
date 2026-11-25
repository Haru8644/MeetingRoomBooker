package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const workScheduleUpdateTestPath = workScheduleTestPath + "/52"

type workScheduleUpdateNotice struct {
	workScheduleCreationNotice
	ID        int
	CreatedAt string
}

type workScheduleUpdateCase struct {
	Name, Actor, ID    string
	Request            json.RawMessage
	Response           workScheduleCreationCapture
	Saved              workschedules.Entry
	Initial, Notices   []workScheduleUpdateNotice
	Logs               []chatworkTestLog
	Requests           []chatworkTestRequest
	Seeds              []workScheduleConflictSeed
	ForceNoticeFailure bool
	FakeResponses      map[string]any
}

func TestWorkScheduleUpdatePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 55*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "WORK_SCHEDULE_UPDATE_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0008_index_reservation_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0009_index_work_schedule_reminder_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0010_enable_work_schedule_creation.sql")

	var reference struct {
		Users          []chatworkTestUser
		InitialEntries []workschedules.Entry
		Cases          []workScheduleUpdateCase
	}
	loadSeriesDeleteFixture(t, "work_schedule_update_csharp.json", &reference)
	passwords := seedAuthTestUsers(t, ctx, pool)
	seedChatworkTestProfiles(t, ctx, pool, reference.Users)
	execWorkScheduleTestSQL(t, ctx, pool, `
		UPDATE users SET password_hash=(SELECT password_hash FROM users WHERE id=2) WHERE id IN (3,6);
		SELECT setval(pg_get_serial_sequence('notifications','id'),1000)`)
	login := func(t *testing.T, base, email, password string) *http.Cookie {
		t.Helper()
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email, "password": password}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}

	fake := &chatworkHTTPFake{t: t, pool: pool}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()
	t.Setenv("CHATWORK_ENABLED", "true")
	t.Setenv("CHATWORK_API_TOKEN", fakeChatworkToken)
	t.Setenv("CHATWORK_API_BASE_URL", server.URL+"/v2/")

	reset := func(t *testing.T) {
		t.Helper()
		resetWorkScheduleSeriesState(t, ctx, pool)
		seedWorkScheduleUpdateEntries(t, ctx, pool, reference.InitialEntries)
	}
	t.Run("compare", func(t *testing.T) {
		const origin = "http://127.0.0.1:5168"
		addr, stop := startAuthTestAPI(t, ctx, dsn, origin)
		defer stop()
		base := "http://" + addr
		actors := map[string]*http.Cookie{
			"admin":     login(t, base, "admin@example.test", passwords["admin"]),
			"owner":     login(t, base, "member@example.test", passwords["member"]),
			"member":    login(t, base, "chatwork-fixture-3@example.test", passwords["member"]),
			"unrelated": login(t, base, "chatwork-fixture-6@example.test", passwords["member"]),
		}
		var warningCase workScheduleUpdateCase
		for _, scenario := range reference.Cases {
			if scenario.Name == "warning_only" {
				warningCase = scenario
			}
			t.Run(scenario.Name, func(t *testing.T) {
				reset(t)
				seedWorkScheduleCreationConflicts(t, ctx, pool, scenario.Seeds)
				seedWorkScheduleUpdateNotices(t, ctx, pool, scenario.Initial)
				expected := scenario.Saved
				if scenario.ForceNoticeFailure {
					constrainSeriesTestWrites(t, ctx, pool, "notifications", "user_id <> 3")
					expected = reference.InitialEntries[0]
				}
				fake.expect(scenario.FakeResponses, func(ctx context.Context) error {
					return checkCommittedWorkScheduleUpdate(ctx, pool, expected.Title, expected.Date, len(scenario.Notices))
				})
				started := time.Now()
				res, body := callWorkScheduleRoute(t, ctx, http.MethodPut, base+workScheduleTestPath+"/"+scenario.ID,
					scenario.Request, actors[scenario.Actor], "", scenario.Response.Status)

				if scenario.Response.Status == http.StatusOK {
					_, detail := callWorkScheduleRoute(t, ctx, http.MethodGet, base+workScheduleUpdateTestPath,
						nil, actors["unrelated"], "", http.StatusOK)
					if !bytes.Equal(body, detail) || res.Header.Get("Content-Type") != scenario.Response.ContentType ||
						res.Header.Get("Location") != scenario.Response.Location {
						t.Fatal("PUT response does not match detail read and C# response headers")
					}
				} else if scenario.ForceNoticeFailure {
					if len(body) != 0 || scenario.Saved.Title == expected.Title {
						t.Fatal("expected C# to retain the edit and Go to return an empty 500")
					}
				} else {
					compareWorkScheduleResponse(t, scenario.Response, res, body)
				}
				verifyWorkScheduleUpdateRows(t, ctx, pool, expected, reference.InitialEntries[1], scenario.Response.Status, started)
				compareWorkScheduleUpdateNotices(t, scenario.Initial, scenario.Notices, readWorkScheduleUpdateNotices(t, ctx, pool), started)
				if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
					t.Fatalf("Chatwork requests = %+v, want %+v", got, scenario.Requests)
				}
				logs := readChatworkLogs(t, ctx, pool)
				compareWorkScheduleUpdateLogs(t, scenario, logs, started)
			})
		}
		t.Run("event_replay_and_distinct_updates", func(t *testing.T) {
			reset(t)
			seedWorkScheduleCreationConflicts(t, ctx, pool, warningCase.Seeds)
			fake.expect(nil, func(ctx context.Context) error {
				return checkCommittedWorkScheduleUpdate(ctx, pool, warningCase.Saved.Title, warningCase.Saved.Date, 3)
			})
			entry := warningCase.Saved
			result, err := workschedules.Update(ctx, pool, 52, workschedules.EntryInput{
				Type: entry.Type, Title: entry.Title, Date: workScheduleUpdateTime(t, entry.Date),
				Start: new(workScheduleUpdateTime(t, *entry.StartTime)), End: new(workScheduleUpdateTime(t, *entry.EndTime)),
				ParticipantIDs: entry.ParticipantIDs, Participants: entry.Participants,
			}, 2, false)
			if err != nil {
				t.Fatal(err)
			}
			sender, err := chatwork.NewSender(pool, fakeChatworkToken, server.URL+"/v2/", true)
			if err != nil {
				t.Fatal(err)
			}
			sender.SendWorkScheduleUpdate(ctx, result)
			logs := readChatworkLogs(t, ctx, pool)
			if len(logs) != 3 || workScheduleUpdateChangeID(t, logs) != result.ChangeID || len(fake.recorded()) != 2 {
				t.Fatal("first event did not deliver under its change ID")
			}
			sender.SendWorkScheduleUpdate(ctx, result)
			if len(fake.recorded()) != 2 || !reflect.DeepEqual(readChatworkLogs(t, ctx, pool), logs) {
				t.Fatal("replaying the same event attempted a second delivery")
			}
			callWorkScheduleRoute(t, ctx, http.MethodPut, base+workScheduleUpdateTestPath,
				warningCase.Request, actors["owner"], "", http.StatusOK)
			laterLogs := readChatworkLogs(t, ctx, pool)
			if len(laterLogs) != 6 || workScheduleUpdateChangeID(t, laterLogs[3:]) == result.ChangeID || len(fake.recorded()) != 4 {
				t.Fatal("a separate update did not generate a distinct event and send again")
			}
		})
		t.Run("waits_for_row_and_reads_new_snapshot", func(t *testing.T) {
			reset(t)
			const previous = "先行トランザクションの変更"
			const current = "行ロック待ち後の変更"
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			if _, err := blocker.Exec(ctx, "UPDATE work_schedule_entries SET title=$1 WHERE id=52", previous); err != nil {
				t.Fatal(err)
			}
			fake.expect(nil, func(ctx context.Context) error {
				return checkCommittedWorkScheduleUpdate(ctx, pool, current, reference.InitialEntries[0].Date, 3)
			})
			var payload map[string]any
			if err := json.Unmarshal(warningCase.Request, &payload); err != nil {
				t.Fatal(err)
			}
			payload["title"] = current
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPut, base+workScheduleUpdateTestPath, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(actors["owner"])
			pending := startSeriesEditRequest(req)
			waitForSeriesEditLocks(t, ctx, pool, 1, `SELECT count(*) FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock'
				AND query LIKE '%FROM work_schedule_entries WHERE id = $1 FOR UPDATE%'`)
			releaseSeriesEditBlocker(t, ctx, blocker)
			assertSeriesEditResult(t, pending, http.StatusOK)
			calls := fake.recorded()
			if len(calls) != 2 || !strings.Contains(calls[0].Body, "内容が「"+previous+"」から「"+current+"」") {
				t.Fatalf("the edit did not compute the diff from the locked snapshot: %+v", calls)
			}
		})
		t.Run("origin_protection", func(t *testing.T) {
			reset(t)
			fake.expect(nil, nil)
			first := reference.Cases[0]
			callWorkScheduleRoute(t, ctx, http.MethodPut, base+workScheduleUpdateTestPath,
				first.Request, actors["owner"], "https://untrusted.example.test", http.StatusForbidden)
			verifyWorkScheduleUpdateRows(t, ctx, pool, reference.InitialEntries[0], reference.InitialEntries[1], http.StatusForbidden, time.Now())
			if len(fake.recorded()) != 0 || len(readWorkScheduleUpdateNotices(t, ctx, pool)) != 0 {
				t.Fatal("untrusted update had side effects")
			}
			fake.expect(nil, func(ctx context.Context) error {
				return checkCommittedWorkScheduleUpdate(ctx, pool, first.Saved.Title, first.Saved.Date, 4)
			})
			res, _ := callWorkScheduleRoute(t, ctx, http.MethodPut, base+workScheduleUpdateTestPath,
				first.Request, actors["owner"], origin, http.StatusOK)
			if res.Header.Get("Access-Control-Allow-Origin") != origin || len(fake.recorded()) != 3 {
				t.Fatal("trusted origin did not edit and deliver")
			}
		})
	})
	t.Run("disabled_by_default", func(t *testing.T) {
		reset(t)
		fake.expect(nil, nil)
		t.Setenv("CHATWORK_ENABLED", "")
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		base := "http://" + addr
		first := reference.Cases[0]
		callWorkScheduleRoute(t, ctx, http.MethodPut, base+workScheduleUpdateTestPath, first.Request,
			login(t, base, "member@example.test", passwords["member"]), "", http.StatusOK)
		if err := checkCommittedWorkScheduleUpdate(ctx, pool, first.Saved.Title, first.Saved.Date, 4); err != nil {
			t.Fatal(err)
		}
		if len(fake.recorded()) != 0 || len(readChatworkLogs(t, ctx, pool)) != 0 {
			t.Fatal("disabled Chatwork attempted a delivery")
		}
	})
}

func workScheduleUpdateTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(dotnettime.Layout, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func seedWorkScheduleUpdateEntries(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entries []workschedules.Entry) {
	t.Helper()
	nullableTicks := func(value *string) *int64 {
		if value == nil {
			return nil
		}
		return new(seriesTestTicks(t, *value))
	}
	for _, entry := range entries {
		execWorkScheduleTestSQL(t, ctx, pool, `
			INSERT INTO work_schedule_entries
			    (id,created_by_user_id,type,title,series_id,date_ticks,start_time_ticks,end_time_ticks,
			     repeat_type,repeat_until_ticks,leave_period,participant_ids,participants,created_at_ticks,updated_at_ticks)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			entry.ID, entry.CreatedByUserID, entry.Type, entry.Title, entry.SeriesID, seriesTestTicks(t, entry.Date),
			nullableTicks(entry.StartTime), nullableTicks(entry.EndTime), entry.RepeatType, nullableTicks(entry.RepeatUntil), entry.LeavePeriod,
			entry.ParticipantIDs, entry.Participants, seriesTestTicks(t, entry.CreatedAt), seriesTestTicks(t, entry.UpdatedAt))
	}
}

func seedWorkScheduleUpdateNotices(t *testing.T, ctx context.Context, pool *pgxpool.Pool, notices []workScheduleUpdateNotice) {
	t.Helper()
	for _, notice := range notices {
		execWorkScheduleTestSQL(t, ctx, pool, `
			INSERT INTO notifications (id,user_id,type,message,created_at_ticks,target_date_ticks,is_read,
			    target_reservation_id,target_work_schedule_entry_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, notice.ID, notice.UserID, notice.Type, notice.Message,
			seriesTestTicks(t, notice.CreatedAt), seriesTestTicks(t, notice.TargetDate), notice.IsRead,
			notice.TargetReservationID, notice.TargetWorkScheduleEntryID)
	}
}

func checkCommittedWorkScheduleUpdate(ctx context.Context, pool *pgxpool.Pool, title, date string, expectedNotices int) error {
	entry, err := workschedules.Get(ctx, pool, 52)
	if err != nil {
		return err
	}
	var notices int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&notices); err != nil {
		return err
	}
	if entry == nil || entry.Title != title || entry.Date != date || notices != expectedNotices {
		return fmt.Errorf("committed schedule = %+v, notice count = %d, want %q on %s and %d notices", entry, notices, title, date, expectedNotices)
	}
	return nil
}

func verifyWorkScheduleUpdateRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expected, sibling workschedules.Entry, status int, started time.Time) {
	t.Helper()
	row, err := workschedules.Get(ctx, pool, expected.ID)
	if err != nil || row == nil {
		t.Fatalf("read updated entry: %+v, %v", row, err)
	}
	if status == http.StatusOK {
		assertWorkScheduleUpdateTimestamp(t, row.UpdatedAt, started)
		expected.UpdatedAt = row.UpdatedAt
	}
	other, err := workschedules.Get(ctx, pool, sibling.ID)
	if err != nil || other == nil || !reflect.DeepEqual(*row, expected) || !reflect.DeepEqual(*other, sibling) {
		t.Fatalf("updated row = %+v, want %+v, sibling = %+v, want %+v, error = %v", row, expected, other, sibling, err)
	}
}

func assertWorkScheduleUpdateTimestamp(t *testing.T, value string, started time.Time) {
	t.Helper()
	clock, err := time.ParseInLocation(dotnettime.Layout, value, time.Local)
	if err != nil || clock.Before(started.Truncate(100*time.Nanosecond)) || clock.After(time.Now()) {
		t.Fatalf("invalid update timestamp %q: %v", value, err)
	}
}

func readWorkScheduleUpdateNotices(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []workScheduleUpdateNotice {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT id,user_id,type,message,created_at_ticks,target_date_ticks,is_read,
		target_reservation_id,target_work_schedule_entry_id FROM notifications ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	notices, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (workScheduleUpdateNotice, error) {
		var notice workScheduleUpdateNotice
		var created, target int64
		err := row.Scan(&notice.ID, &notice.UserID, &notice.Type, &notice.Message, &created, &target,
			&notice.IsRead, &notice.TargetReservationID, &notice.TargetWorkScheduleEntryID)
		notice.CreatedAt = dotnettime.FromTicks(created).Format(dotnettime.Layout)
		notice.TargetDate = dotnettime.FromTicks(target).Format(dotnettime.Layout)
		return notice, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return notices
}

func compareWorkScheduleUpdateNotices(t *testing.T, initial, expected, actual []workScheduleUpdateNotice, started time.Time) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Fatalf("notices = %+v, want %+v", actual, expected)
	}
	for i, want := range expected {
		got := actual[i]
		prior := slices.IndexFunc(initial, func(n workScheduleUpdateNotice) bool { return n.ID == want.ID })
		if prior < 0 || initial[prior].CreatedAt != want.CreatedAt {
			assertWorkScheduleUpdateTimestamp(t, got.CreatedAt, started)
			want.CreatedAt = got.CreatedAt
		}
		if prior < 0 {
			if got.ID <= 1000 {
				t.Fatalf("created notice has an unexpected ID: %+v", got)
			}
			want.ID = got.ID
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("notice = %+v, want %+v", got, want)
		}
	}
}

func workScheduleUpdateChangeID(t *testing.T, logs []chatworkTestLog) string {
	t.Helper()
	var change string
	for i, row := range logs {
		prefix := fmt.Sprintf("WorkScheduleUpdated:work-schedule:52:user:%d:change:", row.TargetUserID)
		value, found := strings.CutPrefix(row.DeliveryKey, prefix)
		if !found || !workScheduleSeriesUUID.MatchString(value) || (i > 0 && value != change) {
			t.Fatalf("invalid or inconsistent change key: %q", row.DeliveryKey)
		}
		change = value
	}
	return change
}

func compareWorkScheduleUpdateLogs(t *testing.T, scenario workScheduleUpdateCase, logs []chatworkTestLog, started time.Time) {
	t.Helper()
	capturedChange := workScheduleUpdateChangeID(t, scenario.Logs)
	actualChange := workScheduleUpdateChangeID(t, logs)
	for i := range scenario.Logs {
		scenario.Logs[i].DeliveryKey = strings.TrimSuffix(scenario.Logs[i].DeliveryKey, capturedChange) + actualChange
	}
	compareChatworkLogs(t, scenario.Name, scenario.Logs, logs, started, false)
}
