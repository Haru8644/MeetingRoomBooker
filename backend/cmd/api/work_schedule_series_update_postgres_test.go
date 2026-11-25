package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workScheduleSeriesUpdateCase struct {
	Name, Actor, Dataset, Failure string
	ID                            int
	Scope                         *string
	Request                       json.RawMessage
	Response                      workScheduleCreationCapture
	Saved                         []workschedules.Entry
	Notices                       []workScheduleCreationNotice
	Logs                          []chatworkTestLog
	Requests                      []chatworkTestRequest
	Seeds                         []workScheduleConflictSeed
	FakeResponses                 map[string]any
}

func (scenario workScheduleSeriesUpdateCase) path() string {
	path := workScheduleTestPath + "/" + strconv.Itoa(scenario.ID) + "/series"
	if scenario.Scope != nil {
		path += "?scope=" + url.QueryEscape(*scenario.Scope)
	}
	return path
}

func TestWorkScheduleSeriesUpdatePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 65*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "WORK_SCHEDULE_SERIES_UPDATE_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0008_index_reservation_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0009_index_work_schedule_reminder_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0010_enable_work_schedule_creation.sql")

	var reference struct {
		Users    []chatworkTestUser
		Datasets map[string][]workschedules.Entry
		Cases    []workScheduleSeriesUpdateCase
	}
	loadSeriesDeleteFixture(t, "work_schedule_series_update_csharp.json", &reference)
	passwords := seedAuthTestUsers(t, ctx, pool)
	seedChatworkTestProfiles(t, ctx, pool, reference.Users)
	execWorkScheduleTestSQL(t, ctx, pool, "UPDATE users SET password_hash=(SELECT password_hash FROM users WHERE id=2) WHERE id=3")
	login := func(t *testing.T, base, email, password string) *http.Cookie {
		t.Helper()
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email, "password": password}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}
	reset := func(t *testing.T, entries []workschedules.Entry) {
		t.Helper()
		resetWorkScheduleSeriesState(t, ctx, pool)
		seedWorkScheduleUpdateEntries(t, ctx, pool, entries)
	}

	fake := &chatworkHTTPFake{t: t, pool: pool}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()
	t.Setenv("CHATWORK_ENABLED", "true")
	t.Setenv("CHATWORK_API_TOKEN", fakeChatworkToken)
	t.Setenv("CHATWORK_API_BASE_URL", server.URL+"/v2/")

	t.Run("reference", func(t *testing.T) {
		const origin = "http://127.0.0.1:5168"
		addr, stop := startAuthTestAPI(t, ctx, dsn, origin)
		defer stop()
		base := "http://" + addr
		actors := map[string]*http.Cookie{
			"admin":  login(t, base, "admin@example.test", passwords["admin"]),
			"owner":  login(t, base, "member@example.test", passwords["member"]),
			"member": login(t, base, "chatwork-fixture-3@example.test", passwords["member"]),
		}
		var noChange workScheduleSeriesUpdateCase
		for _, scenario := range reference.Cases {
			if scenario.Name == "no_change_still_notifies" {
				noChange = scenario
			}
			t.Run(scenario.Name, func(t *testing.T) {
				initial := reference.Datasets[scenario.Dataset]
				reset(t, initial)
				seedWorkScheduleCreationConflicts(t, ctx, pool, scenario.Seeds)
				status := scenario.Response.Status
				switch scenario.Failure {
				case "notice":
					constrainSeriesTestWrites(t, ctx, pool, "notifications", "user_id <> 3")
					status = http.StatusInternalServerError
				case "update":
					constrainSeriesTestWrites(t, ctx, pool, "work_schedule_entries", "id <> 53 OR updated_at_ticks = created_at_ticks")
				}
				started := time.Now()
				fake.expect(scenario.FakeResponses, func(ctx context.Context) error {
					return checkSeriesEditCommit(ctx, pool, scenario.Saved, len(scenario.Notices), started)
				})

				res, body := callWorkScheduleRoute(t, ctx, http.MethodPut, base+scenario.path(),
					scenario.Request, actors[scenario.Actor], "", status)
				wantNotices, wantLogs, wantRequests := scenario.Notices, scenario.Logs, scenario.Requests
				wantSaved := scenario.Saved
				switch status {
				case http.StatusNoContent:
					if len(body) != 0 || res.Header.Get("Location") != "" || res.Header.Get("Content-Type") != scenario.Response.ContentType {
						t.Fatalf("unexpected 204 body or headers: %s / %v", body, res.Header)
					}
				case http.StatusInternalServerError:
					if len(body) != 0 {
						t.Fatalf("failed transaction exposed a body: %s", body)
					}
					wantSaved, wantNotices, wantLogs, wantRequests = nil, nil, nil, nil
				default:
					compareWorkScheduleResponse(t, scenario.Response, res, body)
				}
				verifyWorkScheduleSeriesEditRows(t, ctx, pool, initial, wantSaved, started)
				verifyWorkScheduleSeriesEditNotices(t, ctx, pool, wantNotices, started)
				if got := fake.recorded(); !slices.Equal(got, wantRequests) {
					t.Fatalf("Chatwork requests = %+v, want %+v", got, wantRequests)
				}
				logs := readChatworkLogs(t, ctx, pool)
				capturedChange := workScheduleSeriesEditChangeID(t, wantLogs, scenario.Scope, wantSaved)
				observedChange := workScheduleSeriesEditChangeID(t, logs, scenario.Scope, wantSaved)
				for i := range wantLogs {
					wantLogs[i].DeliveryKey = strings.TrimSuffix(wantLogs[i].DeliveryKey, capturedChange) + observedChange
				}
				compareChatworkLogs(t, scenario.Name, wantLogs, logs, started, false)
			})
		}
		t.Run("replay", func(t *testing.T) {
			reset(t, reference.Datasets[noChange.Dataset])
			entry := noChange.Saved[0]
			started := time.Now()
			fake.expect(nil, func(ctx context.Context) error {
				return checkSeriesEditCommit(ctx, pool, noChange.Saved, len(noChange.Notices), started)
			})
			result, err := workschedules.UpdateSeries(ctx, pool, noChange.ID, workschedules.EntryInput{
				Type: entry.Type, Title: entry.Title, Date: workScheduleUpdateTime(t, entry.Date),
				Start: new(workScheduleUpdateTime(t, *entry.StartTime)), End: new(workScheduleUpdateTime(t, *entry.EndTime)),
				ParticipantIDs: entry.ParticipantIDs, Participants: entry.Participants,
			}, 2, false, *noChange.Scope)
			if err != nil {
				t.Fatal(err)
			}
			sender, err := chatwork.NewSender(pool, fakeChatworkToken, server.URL+"/v2/", true)
			if err != nil {
				t.Fatal(err)
			}
			sender.SendWorkScheduleUpdate(ctx, result)
			logs := readChatworkLogs(t, ctx, pool)
			calls := fake.recorded()
			if len(logs) != 3 || len(calls) != 2 ||
				workScheduleSeriesEditChangeID(t, logs, noChange.Scope, noChange.Saved) != result.ChangeID {
				t.Fatal("series event did not claim its deliveries")
			}
			sender.SendWorkScheduleUpdate(ctx, result)
			if !reflect.DeepEqual(logs, readChatworkLogs(t, ctx, pool)) || !slices.Equal(calls, fake.recorded()) {
				t.Fatal("replayed series event sent another DM")
			}
		})
		t.Run("origin_protection", func(t *testing.T) {
			first := reference.Cases[0]
			initial := reference.Datasets[first.Dataset]
			reset(t, initial)
			fake.expect(nil, nil)
			callWorkScheduleRoute(t, ctx, http.MethodPut, base+first.path(), first.Request, actors["owner"],
				"https://untrusted.example.test", http.StatusForbidden)
			verifyWorkScheduleSeriesEditRows(t, ctx, pool, initial, nil, time.Now())
			verifyWorkScheduleSeriesEditNotices(t, ctx, pool, nil, time.Now())
			if len(fake.recorded()) > 0 || len(readChatworkLogs(t, ctx, pool)) > 0 {
				t.Fatal("untrusted series edit attempted delivery")
			}
			res, _ := callWorkScheduleRoute(t, ctx, http.MethodPut, base+first.path(), first.Request, actors["owner"],
				origin, http.StatusNoContent)
			if res.Header.Get("Access-Control-Allow-Origin") != origin || len(fake.recorded()) != 4 {
				t.Fatal("trusted origin did not edit and deliver")
			}
		})
	})

	t.Run("disabled_and_concurrent", func(t *testing.T) {
		t.Setenv("CHATWORK_ENABLED", "")
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		base := "http://" + addr
		cookie := login(t, base, "member@example.test", passwords["member"])
		fake.expect(nil, nil)
		first := reference.Cases[0]
		initial := reference.Datasets[first.Dataset]
		t.Run("single_edit_expands_following_scope", func(t *testing.T) {
			reset(t, initial)
			verifySingleAndSeriesWorkScheduleEdit(t, ctx, pool, base, cookie, first)
		})
		t.Run("checks_permissions_after_wait", func(t *testing.T) {
			reset(t, initial)
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			if _, err := blocker.Exec(ctx, "UPDATE work_schedule_entries SET created_by_user_id=5 WHERE id=66"); err != nil {
				t.Fatal(err)
			}
			pending := startWorkScheduleSeriesEdit(t, ctx, base+first.path(), first.Request, cookie)
			waitForWorkScheduleSeriesRows(t, ctx, pool)
			releaseSeriesEditBlocker(t, ctx, blocker)
			assertSeriesEditResult(t, pending, http.StatusForbidden)
			expected := slices.Clone(initial)
			for i := range expected {
				if expected[i].ID == 66 {
					expected[i].CreatedByUserID = 5
				}
			}
			verifyWorkScheduleSeriesEditRows(t, ctx, pool, expected, nil, time.Now())
			verifyWorkScheduleSeriesEditNotices(t, ctx, pool, nil, time.Now())
		})
		if len(fake.recorded()) != 0 || len(readChatworkLogs(t, ctx, pool)) != 0 {
			t.Fatal("disabled Chatwork attempted delivery")
		}
	})
}

func checkSeriesEditCommit(ctx context.Context, pool *pgxpool.Pool, entries []workschedules.Entry, noticeCount int, started time.Time) error {
	for _, expected := range entries {
		entry, err := workschedules.Get(ctx, pool, expected.ID)
		if err != nil || entry == nil {
			return fmt.Errorf("cannot read committed row %d: %v", expected.ID, err)
		}
		expected.UpdatedAt = entry.UpdatedAt
		if !reflect.DeepEqual(*entry, expected) {
			return fmt.Errorf("committed row %d does not match before the DM, started %s", entry.ID, started)
		}
	}
	var actual int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&actual); err != nil || actual != noticeCount {
		return fmt.Errorf("committed notices = %d, want %d, err = %v", actual, noticeCount, err)
	}
	return nil
}

func verifyWorkScheduleSeriesEditRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, initial, updated []workschedules.Entry, started time.Time) {
	t.Helper()
	for _, expected := range initial {
		i := slices.IndexFunc(updated, func(entry workschedules.Entry) bool { return entry.ID == expected.ID })
		entry, err := workschedules.Get(ctx, pool, expected.ID)
		if err != nil || entry == nil {
			t.Fatalf("read work schedule %d: %v", expected.ID, err)
		}
		if i >= 0 {
			expected = updated[i]
			assertWorkScheduleUpdateTimestamp(t, entry.UpdatedAt, started)
			expected.UpdatedAt = entry.UpdatedAt
		}
		if !reflect.DeepEqual(*entry, expected) {
			t.Fatalf("entry %d = %+v, want %+v", entry.ID, *entry, expected)
		}
	}
}

func verifyWorkScheduleSeriesEditNotices(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expected []workScheduleCreationNotice, started time.Time) {
	t.Helper()
	actual := readWorkScheduleUpdateNotices(t, ctx, pool)
	if len(actual) != len(expected) {
		t.Fatalf("saved notices = %+v, want %+v", actual, expected)
	}
	for i, got := range actual {
		assertWorkScheduleUpdateTimestamp(t, got.CreatedAt, started)
		if !reflect.DeepEqual(got.workScheduleCreationNotice, expected[i]) {
			t.Fatalf("notice = %+v, want %+v", got, expected[i])
		}
	}
}

func workScheduleSeriesEditChangeID(t *testing.T, logs []chatworkTestLog, scope *string, saved []workschedules.Entry) string {
	t.Helper()
	var change string
	for i, row := range logs {
		if row.WorkScheduleEntryID == nil {
			t.Fatal("work schedule delivery has no entry ID")
		}
		id := *row.WorkScheduleEntryID
		prefix := fmt.Sprintf("WorkScheduleUpdated:work-schedule:%d:user:%d:change:", id, row.TargetUserID)
		if scope != nil {
			normalized := strings.ToLower(strings.TrimSpace(*scope))
			if normalized == "all" || normalized == "following" {
				j := slices.IndexFunc(saved, func(entry workschedules.Entry) bool { return entry.ID == id })
				if j < 0 || saved[j].SeriesID == nil {
					t.Fatalf("delivery points to a row outside the series edit: %d", id)
				}
				prefix = fmt.Sprintf("WorkScheduleSeriesUpdated:work-schedule-series:%s:scope:%s:user:%d:change:",
					*saved[j].SeriesID, normalized, row.TargetUserID)
			}
		}
		key, found := strings.CutPrefix(row.DeliveryKey, prefix)
		if !found || !workScheduleSeriesUUID.MatchString(key) || (i > 0 && key != change) {
			t.Fatalf("invalid series delivery key: %q", row.DeliveryKey)
		}
		change = key
	}
	return change
}

func verifySingleAndSeriesWorkScheduleEdit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, base string,
	cookie *http.Cookie, series workScheduleSeriesUpdateCase,
) {
	t.Helper()
	execWorkScheduleTestSQL(t, ctx, pool, `
		CREATE FUNCTION hold_single_work_schedule_edit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		    IF NEW.title = '先行単発更新' THEN
		        PERFORM pg_advisory_xact_lock(75262);
		    END IF;
		    RETURN NEW;
		END $$;
		CREATE TRIGGER hold_single_work_schedule_edit BEFORE UPDATE ON work_schedule_entries
		FOR EACH ROW EXECUTE FUNCTION hold_single_work_schedule_edit()`)
	t.Cleanup(func() {
		execWorkScheduleTestSQL(t, ctx, pool, `DROP TRIGGER hold_single_work_schedule_edit ON work_schedule_entries;
			DROP FUNCTION hold_single_work_schedule_edit()`)
	})
	blocker := beginSeriesEditBlocker(t, ctx, pool)
	if _, err := blocker.Exec(ctx, "SELECT pg_advisory_xact_lock(75262)"); err != nil {
		t.Fatal(err)
	}
	single := startWorkScheduleSeriesEdit(t, ctx, base+workScheduleTestPath+"/62", []byte(`{
		"type":0,"title":"先行単発更新","date":"2026-06-19","startTime":"2026-06-19T07:00:00",
		"endTime":"2026-06-19T08:00:00","participantIds":[2,3,4]
	}`), cookie)
	waitForSeriesEditLocks(t, ctx, pool, 1, `SELECT count(*) FROM pg_stat_activity
		WHERE datname=current_database() AND wait_event='advisory' AND query LIKE '%UPDATE work_schedule_entries%'`)
	batch := startWorkScheduleSeriesEdit(t, ctx, base+series.path(), series.Request, cookie)
	waitForWorkScheduleSeriesRows(t, ctx, pool)
	releaseSeriesEditBlocker(t, ctx, blocker)
	assertSeriesEditResult(t, single, http.StatusOK)
	assertSeriesEditResult(t, batch, http.StatusNoContent)

	for _, id := range []int{65, 61, 62, 64, 66} {
		entry, err := workschedules.Get(ctx, pool, id)
		if err != nil || entry == nil || entry.Title != "変更後の架空予定" {
			t.Fatalf("id %d was not updated after scope re-evaluation: %+v, %v", id, entry, err)
		}
	}
	entry, err := workschedules.Get(ctx, pool, 60)
	if err != nil || entry == nil || entry.Title != "変更前の架空予定" {
		t.Fatalf("earlier date was changed: %+v, %v", entry, err)
	}
}

func startWorkScheduleSeriesEdit(t *testing.T, ctx context.Context, target string, body []byte, cookie *http.Cookie) <-chan seriesEditResult {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	return startSeriesEditRequest(req)
}

func waitForWorkScheduleSeriesRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	waitForSeriesEditLocks(t, ctx, pool, 1, `SELECT count(*) FROM pg_stat_activity
		WHERE datname=current_database() AND wait_event_type='Lock'
		  AND query LIKE '%FROM work_schedule_entries WHERE id = ANY($1::integer[]) ORDER BY id FOR UPDATE%'`)
}
