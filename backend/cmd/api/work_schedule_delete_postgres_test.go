package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workScheduleDeleteCase struct {
	Name, Actor, Dataset, ID string
	Response                 workScheduleCreationCapture
	Again                    *workScheduleCreationCapture
	Remaining                []int
	Initial, Notices         []workScheduleUpdateNotice
	ExistingLogs, Logs       []chatworkTestLog
	Requests                 []chatworkTestRequest
	FakeResponses            map[string]any
	ForceNoticeFailure       bool
}

func TestWorkScheduleDeletionPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "WORK_SCHEDULE_DELETE_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0008_index_reservation_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0009_index_work_schedule_reminder_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0010_enable_work_schedule_creation.sql")

	var reference struct {
		Users    []chatworkTestUser
		Datasets map[string][]workschedules.Entry
		Cases    []workScheduleDeleteCase
	}
	loadSeriesDeleteFixture(t, "work_schedule_delete_csharp.json", &reference)
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
	reset := func(t *testing.T, dataset string) []workschedules.Entry {
		t.Helper()
		resetWorkScheduleSeriesState(t, ctx, pool)
		initial := reference.Datasets[dataset]
		seedWorkScheduleUpdateEntries(t, ctx, pool, initial)
		return initial
	}
	creatorCase := reference.Cases[0]

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
			"admin":   login(t, base, "admin@example.test", passwords["admin"]),
			"creator": login(t, base, "member@example.test", passwords["member"]),
			"member":  login(t, base, "chatwork-fixture-3@example.test", passwords["member"]),
			"other":   login(t, base, "chatwork-fixture-6@example.test", passwords["member"]),
		}
		for _, scenario := range reference.Cases {
			t.Run(scenario.Name, func(t *testing.T) {
				initial := reset(t, scenario.Dataset)
				seedWorkScheduleUpdateNotices(t, ctx, pool, scenario.Initial)
				for _, existing := range scenario.ExistingLogs {
					seedChatworkLog(t, ctx, pool, existing)
				}
				if scenario.ForceNoticeFailure {
					constrainSeriesTestWrites(t, ctx, pool, "notifications", "user_id <> 2")
					if slices.Contains(scenario.Remaining, 52) {
						t.Fatal("the C# capture should show an already committed deletion")
					}
					scenario.Remaining = append(scenario.Remaining, 52)
				}
				fake.expect(scenario.FakeResponses, func(ctx context.Context) error {
					return checkScheduleDeletionCommit(ctx, pool, len(scenario.Remaining), len(scenario.Notices))
				})
				path := base + workScheduleTestPath + "/" + scenario.ID
				started := time.Now()
				res, body := callWorkScheduleRoute(t, ctx, http.MethodDelete, path, nil, actors[scenario.Actor], "", scenario.Response.Status)
				if scenario.ForceNoticeFailure {
					if len(body) > 0 {
						t.Fatalf("expected empty 500, got %q", body)
					}
				} else {
					compareWorkScheduleResponse(t, scenario.Response, res, body)
				}

				verifyScheduleDeletionRows(t, ctx, pool, initial, scenario.Remaining)
				notices := readWorkScheduleUpdateNotices(t, ctx, pool)
				compareWorkScheduleUpdateNotices(t, scenario.Initial, scenario.Notices, notices, started)
				if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
					t.Fatalf("Chatwork requests = %+v, want %+v", got, scenario.Requests)
				}
				logs := readChatworkLogs(t, ctx, pool)
				compareChatworkLogs(t, scenario.Name, scenario.Logs, logs, started, len(scenario.ExistingLogs) > 0)
				if scenario.Again != nil {
					res, body = callWorkScheduleRoute(t, ctx, http.MethodDelete, path, nil, actors[scenario.Actor], "", scenario.Again.Status)
					compareWorkScheduleResponse(t, *scenario.Again, res, body)
					if !reflect.DeepEqual(notices, readWorkScheduleUpdateNotices(t, ctx, pool)) ||
						!reflect.DeepEqual(logs, readChatworkLogs(t, ctx, pool)) || !slices.Equal(fake.recorded(), scenario.Requests) {
						t.Fatal("repeated delete changed notices or attempted delivery")
					}
				}
			})
		}

		t.Run("replay_committed_deletion", func(t *testing.T) {
			reset(t, creatorCase.Dataset)
			fake.expect(nil, func(ctx context.Context) error { return checkScheduleDeletionCommit(ctx, pool, 2, 3) })
			result, err := workschedules.Delete(ctx, pool, 52, 2, false)
			if err != nil {
				t.Fatal(err)
			}
			sender, err := chatwork.NewSender(pool, fakeChatworkToken, server.URL+"/v2/", true)
			if err != nil {
				t.Fatal(err)
			}
			sender.SendWorkScheduleDeletion(ctx, result)
			logs := readChatworkLogs(t, ctx, pool)
			sender.SendWorkScheduleDeletion(ctx, result)
			if len(logs) != 3 || !reflect.DeepEqual(logs, readChatworkLogs(t, ctx, pool)) ||
				!slices.Equal(fake.recorded(), creatorCase.Requests) {
				t.Fatal("replay sent a second DM or changed the delivery log")
			}
		})

		t.Run("origin_protection", func(t *testing.T) {
			initial := reset(t, creatorCase.Dataset)
			fake.expect(nil, nil)
			callWorkScheduleRoute(t, ctx, http.MethodDelete, base+workScheduleUpdateTestPath, nil, actors["creator"],
				"https://untrusted.example.test", http.StatusForbidden)
			verifyScheduleDeletionRows(t, ctx, pool, initial, []int{52, 53, 54})
			if len(fake.recorded()) != 0 || len(readWorkScheduleUpdateNotices(t, ctx, pool)) != 0 ||
				len(readChatworkLogs(t, ctx, pool)) != 0 {
				t.Fatal("untrusted deletion had side effects")
			}
			fake.expect(nil, func(ctx context.Context) error { return checkScheduleDeletionCommit(ctx, pool, 2, 3) })
			res, _ := callWorkScheduleRoute(t, ctx, http.MethodDelete, base+workScheduleUpdateTestPath, nil, actors["creator"],
				origin, http.StatusNoContent)
			if res.Header.Get("Access-Control-Allow-Origin") != origin || !slices.Equal(fake.recorded(), creatorCase.Requests) {
				t.Fatal("trusted origin did not delete and deliver")
			}
		})

		t.Run("concurrent_deletions", func(t *testing.T) {
			initial := reset(t, creatorCase.Dataset)
			fake.expect(nil, func(ctx context.Context) error { return checkScheduleDeletionCommit(ctx, pool, 2, 3) })
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			if _, err := blocker.Exec(ctx, "SELECT id FROM work_schedule_entries WHERE id=52 FOR UPDATE"); err != nil {
				t.Fatal(err)
			}
			first := startWorkScheduleDelete(t, ctx, base, actors["creator"])
			second := startWorkScheduleDelete(t, ctx, base, actors["admin"])
			waitForWorkScheduleDeleteRows(t, ctx, pool, 2)
			started := time.Now()
			releaseSeriesEditBlocker(t, ctx, blocker)
			var statuses []int
			for _, result := range []seriesEditResult{<-first, <-second} {
				if result.err != nil {
					t.Fatal(result.err)
				}
				statuses = append(statuses, result.status)
			}
			slices.Sort(statuses)
			if !slices.Equal(statuses, []int{http.StatusNoContent, http.StatusNotFound}) {
				t.Fatalf("concurrent delete statuses = %v", statuses)
			}
			verifyScheduleDeletionRows(t, ctx, pool, initial, creatorCase.Remaining)
			compareWorkScheduleUpdateNotices(t, nil, creatorCase.Notices, readWorkScheduleUpdateNotices(t, ctx, pool), started)
			compareChatworkLogs(t, "concurrent_deletions", creatorCase.Logs, readChatworkLogs(t, ctx, pool), started, false)
			if !slices.Equal(fake.recorded(), creatorCase.Requests) {
				t.Fatal("concurrent deletes sent duplicate notifications")
			}
		})
	})

	t.Run("delivery_disabled", func(t *testing.T) {
		t.Setenv("CHATWORK_ENABLED", "")
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		base := "http://" + addr
		cookie := login(t, base, "member@example.test", passwords["member"])
		fake.expect(nil, nil)
		updatePayload := []byte(`{
				"type":0,"title":"先行更新の勤務予定","date":"2026-06-23","startTime":"2026-06-23T13:00:00",
				"endTime":"2026-06-23T14:00:00","participantIds":[5]
			}`)
		t.Run("delete_waits_for_update", func(t *testing.T) {
			initial := reset(t, creatorCase.Dataset)
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			if _, err := blocker.Exec(ctx, "SELECT pg_advisory_xact_lock(75552)"); err != nil {
				t.Fatal(err)
			}
			execWorkScheduleTestSQL(t, ctx, pool, `
				CREATE FUNCTION hold_update_before_work_schedule_delete() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN
				    IF NEW.title = '先行更新の勤務予定' THEN
				        PERFORM pg_advisory_xact_lock(75552);
				    END IF;
				    RETURN NEW;
				END $$;
				CREATE TRIGGER hold_update_before_work_schedule_delete BEFORE UPDATE ON work_schedule_entries
				FOR EACH ROW EXECUTE FUNCTION hold_update_before_work_schedule_delete()`)
			t.Cleanup(func() {
				_ = blocker.Rollback(context.Background())
				execWorkScheduleTestSQL(t, ctx, pool, `DROP TRIGGER hold_update_before_work_schedule_delete ON work_schedule_entries;
					DROP FUNCTION hold_update_before_work_schedule_delete()`)
			})
			update := startWorkScheduleSeriesEdit(t, ctx, base+workScheduleUpdateTestPath, updatePayload, cookie)
			waitForSeriesEditLocks(t, ctx, pool, 1, `SELECT count(*) FROM pg_stat_activity
				WHERE datname=current_database() AND wait_event='advisory' AND query LIKE '%UPDATE work_schedule_entries%'`)
			deletion := startWorkScheduleDelete(t, ctx, base, cookie)
			waitForWorkScheduleDeleteRows(t, ctx, pool, 1)
			releaseSeriesEditBlocker(t, ctx, blocker)
			assertSeriesEditResult(t, update, http.StatusOK)
			assertSeriesEditResult(t, deletion, http.StatusNoContent)
			verifyScheduleDeletionRows(t, ctx, pool, initial, creatorCase.Remaining)
			var recipients []int
			for _, notice := range readWorkScheduleUpdateNotices(t, ctx, pool) {
				if notice.Message == "社外予定「先行更新の勤務予定」が削除されました。日付: 2026/06/23" {
					if notice.TargetDate != "2026-06-23T00:00:00" || notice.TargetWorkScheduleEntryID != 52 {
						t.Fatalf("delete used a stale date or target: %+v", notice)
					}
					recipients = append(recipients, notice.UserID)
				}
			}
			if !slices.Equal(recipients, []int{2, 5}) {
				t.Fatalf("delete used stale participants: %v", recipients)
			}
		})
		t.Run("update_waits_for_delete", func(t *testing.T) {
			initial := reset(t, creatorCase.Dataset)
			i := slices.IndexFunc(reference.Cases, func(c workScheduleDeleteCase) bool { return c.Name == "refresh_matching_latest" })
			fixture := reference.Cases[i]
			seedWorkScheduleUpdateNotices(t, ctx, pool, fixture.Initial)
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			if _, err := blocker.Exec(ctx, "SELECT id FROM notifications WHERE id=702 FOR UPDATE"); err != nil {
				t.Fatal(err)
			}

			started := time.Now()
			deletion := startWorkScheduleDelete(t, ctx, base, cookie)
			waitForSeriesEditLocks(t, ctx, pool, 1, `SELECT count(*) FROM pg_stat_activity
				WHERE datname=current_database() AND wait_event_type='Lock'
				  AND query LIKE '%target_work_schedule_entry_id = $2 AND type = $3%'`)
			update := startWorkScheduleSeriesEdit(t, ctx, base+workScheduleUpdateTestPath, updatePayload, cookie)
			waitForWorkScheduleDeleteRows(t, ctx, pool, 1)
			releaseSeriesEditBlocker(t, ctx, blocker)
			assertSeriesEditResult(t, deletion, http.StatusNoContent)
			assertSeriesEditResult(t, update, http.StatusNotFound)
			verifyScheduleDeletionRows(t, ctx, pool, initial, creatorCase.Remaining)
			compareWorkScheduleUpdateNotices(t, fixture.Initial, fixture.Notices, readWorkScheduleUpdateNotices(t, ctx, pool), started)
		})
		t.Run("owner_rechecked_after_wait", func(t *testing.T) {
			initial := slices.Clone(reset(t, creatorCase.Dataset))
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			if _, err := blocker.Exec(ctx, "UPDATE work_schedule_entries SET created_by_user_id=5 WHERE id=52"); err != nil {
				t.Fatal(err)
			}
			deletion := startWorkScheduleDelete(t, ctx, base, cookie)
			waitForWorkScheduleDeleteRows(t, ctx, pool, 1)
			releaseSeriesEditBlocker(t, ctx, blocker)
			assertSeriesEditResult(t, deletion, http.StatusForbidden)
			initial[0].CreatedByUserID = 5
			verifyScheduleDeletionRows(t, ctx, pool, initial, []int{52, 53, 54})
			if len(readWorkScheduleUpdateNotices(t, ctx, pool)) != 0 {
				t.Fatal("forbidden delete persisted a notice")
			}
		})
		if len(fake.recorded()) != 0 || len(readChatworkLogs(t, ctx, pool)) != 0 {
			t.Fatal("disabled Chatwork attempted a delivery")
		}
	})
}

func checkScheduleDeletionCommit(ctx context.Context, pool *pgxpool.Pool, remaining, notices int) error {
	deleted, err := workschedules.Get(ctx, pool, 52)
	if err != nil {
		return err
	}
	if deleted != nil {
		return fmt.Errorf("schedule %d still visible to the sender", deleted.ID)
	}
	return checkWorkScheduleCreationCounts(ctx, pool, remaining, notices)
}

func verifyScheduleDeletionRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, initial []workschedules.Entry, remaining []int) {
	t.Helper()
	for _, row := range initial {
		got, err := workschedules.Get(ctx, pool, row.ID)
		var want *workschedules.Entry
		if slices.Contains(remaining, row.ID) {
			want = &row
		}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("schedule %d = %+v, want %+v, error %v", row.ID, got, want, err)
		}
	}
}

func startWorkScheduleDelete(t *testing.T, ctx context.Context, base string, cookie *http.Cookie) <-chan seriesEditResult {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+workScheduleUpdateTestPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	return startSeriesEditRequest(req)
}

func waitForWorkScheduleDeleteRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, count int) {
	t.Helper()
	waitForSeriesEditLocks(t, ctx, pool, count, `SELECT count(*) FROM pg_stat_activity
		WHERE datname=current_database() AND wait_event_type='Lock'
		  AND query LIKE '%FROM work_schedule_entries WHERE id = $1 FOR UPDATE%'`)
}
