package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workScheduleSeriesDeleteCase struct {
	workScheduleDeleteCase
	Scope   *string
	Failure string
}

func (scenario workScheduleSeriesDeleteCase) path() string {
	path := workScheduleTestPath + "/" + scenario.ID + "/series"
	if scenario.Scope != nil {
		path += "?scope=" + url.QueryEscape(*scenario.Scope)
	}
	return path
}

func TestWorkScheduleSeriesDeletionPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 65*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "WORK_SCHEDULE_SERIES_DELETE_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0008_index_reservation_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0009_index_work_schedule_reminder_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0010_enable_work_schedule_creation.sql")

	var reference struct {
		Users    []chatworkTestUser
		Datasets map[string][]workschedules.Entry
		Cases    []workScheduleSeriesDeleteCase
	}
	loadSeriesDeleteFixture(t, "work_schedule_series_delete_csharp.json", &reference)
	passwords := seedAuthTestUsers(t, ctx, pool)
	seedChatworkTestProfiles(t, ctx, pool, reference.Users)
	execWorkScheduleTestSQL(t, ctx, pool, `
		UPDATE users SET password_hash=(SELECT password_hash FROM users WHERE id=2) WHERE id=3;
		SELECT setval(pg_get_serial_sequence('notifications','id'),1000)`)
	login := func(t *testing.T, base, email, password string) *http.Cookie {
		t.Helper()
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email, "password": password}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}
	reset := func(t *testing.T, scenario workScheduleSeriesDeleteCase) []workschedules.Entry {
		t.Helper()
		resetWorkScheduleSeriesState(t, ctx, pool)
		initial := reference.Datasets[scenario.Dataset]
		seedWorkScheduleUpdateEntries(t, ctx, pool, initial)
		seedWorkScheduleUpdateNotices(t, ctx, pool, scenario.Initial)
		for _, existing := range scenario.ExistingLogs {
			seedChatworkLog(t, ctx, pool, existing)
		}
		return initial
	}
	following := reference.Cases[0]

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
		for _, scenario := range reference.Cases {
			t.Run(scenario.Name, func(t *testing.T) {
				initial := reset(t, scenario)
				status := scenario.Response.Status
				wantRemaining, wantNotices := scenario.Remaining, scenario.Notices
				wantRequests, wantLogs := scenario.Requests, scenario.Logs
				switch scenario.Failure {
				case "notice":
					constrainSeriesTestWrites(t, ctx, pool, "notifications", "user_id <> 3")
					status = http.StatusInternalServerError
					wantRemaining = make([]int, len(initial))
					for i, entry := range initial {
						wantRemaining[i] = entry.ID
					}
					wantNotices, wantRequests, wantLogs = scenario.Initial, nil, scenario.ExistingLogs
				case "delete":
					failSecondWorkScheduleSeriesDelete(t, ctx, pool)
				}
				fake.expect(scenario.FakeResponses, func(ctx context.Context) error {
					return checkSeriesDeletionCommit(ctx, pool, initial, wantRemaining, len(wantNotices))
				})
				path := base + scenario.path()
				started := time.Now()
				res, body := callWorkScheduleRoute(t, ctx, http.MethodDelete, path, nil, actors[scenario.Actor], "", status)
				if status == http.StatusInternalServerError {
					if len(body) != 0 {
						t.Fatalf("expected empty 500, got %q", body)
					}
				} else {
					compareWorkScheduleResponse(t, scenario.Response, res, body)
				}
				verifyScheduleDeletionRows(t, ctx, pool, initial, wantRemaining)
				notices := readWorkScheduleUpdateNotices(t, ctx, pool)
				compareWorkScheduleUpdateNotices(t, scenario.Initial, wantNotices, notices, started)
				logs := readChatworkLogs(t, ctx, pool)
				compareChatworkLogs(t, scenario.Name, wantLogs, logs, started, len(scenario.ExistingLogs) > 0)
				if got := fake.recorded(); !slices.Equal(got, wantRequests) {
					t.Fatalf("Chatwork requests = %+v, want %+v", got, wantRequests)
				}
				if scenario.Again != nil {
					res, body = callWorkScheduleRoute(t, ctx, http.MethodDelete, path, nil, actors[scenario.Actor], "", scenario.Again.Status)
					compareWorkScheduleResponse(t, *scenario.Again, res, body)
					if !reflect.DeepEqual(notices, readWorkScheduleUpdateNotices(t, ctx, pool)) ||
						!reflect.DeepEqual(logs, readChatworkLogs(t, ctx, pool)) || !slices.Equal(fake.recorded(), wantRequests) {
						t.Fatal("repeat series delete changed notifications or attempted delivery")
					}
				}
			})
		}

		t.Run("replay_committed_series_deletion", func(t *testing.T) {
			initial := reset(t, following)
			fake.expect(nil, func(ctx context.Context) error {
				return checkSeriesDeletionCommit(ctx, pool, initial, following.Remaining, len(following.Notices))
			})
			result, err := workschedules.DeleteSeries(ctx, pool, 62, 2, false, *following.Scope)
			if err != nil {
				t.Fatal(err)
			}
			sender, err := chatwork.NewSender(pool, fakeChatworkToken, server.URL+"/v2/", true)
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			sender.SendWorkScheduleDeletion(ctx, result)
			logs := readChatworkLogs(t, ctx, pool)
			compareChatworkLogs(t, "following_boundary", following.Logs, logs, started, false)
			sender.SendWorkScheduleDeletion(ctx, result)
			if !reflect.DeepEqual(logs, readChatworkLogs(t, ctx, pool)) || !slices.Equal(fake.recorded(), following.Requests) {
				t.Fatal("replaying a committed series deletion sent another DM")
			}
		})

		t.Run("origin_protection", func(t *testing.T) {
			initial := reset(t, following)
			fake.expect(nil, nil)
			callWorkScheduleRoute(t, ctx, http.MethodDelete, base+following.path(), nil, actors["owner"],
				"https://untrusted.example.test", http.StatusForbidden)
			allIDs := make([]int, len(initial))
			for i, entry := range initial {
				allIDs[i] = entry.ID
			}
			verifyScheduleDeletionRows(t, ctx, pool, initial, allIDs)
			if len(fake.recorded()) != 0 || len(readChatworkLogs(t, ctx, pool)) != 0 ||
				!reflect.DeepEqual(following.Initial, readWorkScheduleUpdateNotices(t, ctx, pool)) {
				t.Fatal("untrusted series delete had side effects")
			}
			fake.expect(nil, func(ctx context.Context) error {
				return checkSeriesDeletionCommit(ctx, pool, initial, following.Remaining, len(following.Notices))
			})
			res, _ := callWorkScheduleRoute(t, ctx, http.MethodDelete, base+following.path(), nil, actors["owner"],
				origin, http.StatusNoContent)
			if res.Header.Get("Access-Control-Allow-Origin") != origin || !slices.Equal(fake.recorded(), following.Requests) {
				t.Fatal("trusted series delete did not delete and deliver")
			}
		})
	})

	t.Run("delivery_disabled", func(t *testing.T) {
		t.Setenv("CHATWORK_ENABLED", "")
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		base := "http://" + addr
		owner := login(t, base, "member@example.test", passwords["member"])
		fake.expect(nil, nil)
		t.Run("series_update_expands_following", func(t *testing.T) {
			initial := reset(t, following)
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			execWorkScheduleTestSQL(t, ctx, pool, `
				CREATE FUNCTION hold_edit_before_series_deletion() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN
				    IF NEW.title = '先行一括更新' THEN
				        PERFORM pg_advisory_xact_lock(75662);
				    END IF;
				    RETURN NEW;
				END $$;
				CREATE TRIGGER hold_edit_before_series_deletion BEFORE UPDATE ON work_schedule_entries
				FOR EACH ROW EXECUTE FUNCTION hold_edit_before_series_deletion()`)
			t.Cleanup(func() {
				_ = blocker.Rollback(context.Background())
				execWorkScheduleTestSQL(t, ctx, pool, `DROP TRIGGER hold_edit_before_series_deletion ON work_schedule_entries;
					DROP FUNCTION hold_edit_before_series_deletion()`)
			})
			if _, err := blocker.Exec(ctx, "SELECT pg_advisory_xact_lock(75662)"); err != nil {
				t.Fatal(err)
			}
			edit := startWorkScheduleSeriesEdit(t, ctx, base+following.path(), []byte(`{
				"type":0,"title":"先行一括更新","date":"2026-06-19","startTime":"2026-06-19T07:00:00",
				"endTime":"2026-06-19T08:00:00","participantIds":[5]
			}`), owner)
			waitForSeriesEditLocks(t, ctx, pool, 1, `SELECT count(*) FROM pg_stat_activity
				WHERE datname=current_database() AND wait_event='advisory' AND query LIKE '%UPDATE work_schedule_entries%'`)
			deletion := startWorkScheduleBatchDelete(t, ctx, base+following.path(), owner)
			waitForWorkScheduleSeriesRows(t, ctx, pool)
			releaseSeriesEditBlocker(t, ctx, blocker)
			assertSeriesEditResult(t, edit, http.StatusNoContent)
			assertSeriesEditResult(t, deletion, http.StatusNoContent)

			// The edited anchor at 07:00 now includes 65 at 08:00. The
			// untimed occurrence still compares as midnight and remains.
			verifyScheduleDeletionRows(t, ctx, pool, initial, []int{60, 63, 67, 68, 69})
			verifySeriesDeletionSummary(t, ctx, pool, 61, "先行一括更新", "5件")
		})

		t.Run("single_delete_shrinks_following", func(t *testing.T) {
			initial := reset(t, following)
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			execWorkScheduleTestSQL(t, ctx, pool, `
				CREATE FUNCTION hold_single_before_series_deletion() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN
				    IF OLD.id = 61 THEN
				        PERFORM pg_advisory_xact_lock(75661);
				    END IF;
				    RETURN OLD;
				END $$;
				CREATE TRIGGER hold_single_before_series_deletion BEFORE DELETE ON work_schedule_entries
				FOR EACH ROW EXECUTE FUNCTION hold_single_before_series_deletion()`)
			t.Cleanup(func() {
				_ = blocker.Rollback(context.Background())
				execWorkScheduleTestSQL(t, ctx, pool, `DROP TRIGGER hold_single_before_series_deletion ON work_schedule_entries;
					DROP FUNCTION hold_single_before_series_deletion()`)
			})
			if _, err := blocker.Exec(ctx, "SELECT pg_advisory_xact_lock(75661)"); err != nil {
				t.Fatal(err)
			}
			single := startWorkScheduleBatchDelete(t, ctx, base+workScheduleTestPath+"/61", owner)
			waitForSeriesEditLocks(t, ctx, pool, 1, `SELECT count(*) FROM pg_stat_activity
				WHERE datname=current_database() AND wait_event='advisory' AND query LIKE '%DELETE FROM work_schedule_entries%'`)
			batch := startWorkScheduleBatchDelete(t, ctx, base+following.path(), owner)
			waitForWorkScheduleSeriesRows(t, ctx, pool)
			releaseSeriesEditBlocker(t, ctx, blocker)
			assertSeriesEditResult(t, single, http.StatusNoContent)
			assertSeriesEditResult(t, batch, http.StatusNoContent)
			verifyScheduleDeletionRows(t, ctx, pool, initial, following.Remaining)
			verifySeriesDeletionSummary(t, ctx, pool, 62, "変更前の架空予定", "3件")
		})

		t.Run("permission_rechecked_after_wait", func(t *testing.T) {
			initial := slices.Clone(reset(t, following))
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			if _, err := blocker.Exec(ctx, "UPDATE work_schedule_entries SET created_by_user_id=5 WHERE id=66"); err != nil {
				t.Fatal(err)
			}
			deletion := startWorkScheduleBatchDelete(t, ctx, base+following.path(), owner)
			waitForWorkScheduleSeriesRows(t, ctx, pool)
			releaseSeriesEditBlocker(t, ctx, blocker)
			assertSeriesEditResult(t, deletion, http.StatusForbidden)
			ids := make([]int, len(initial))
			for i := range initial {
				ids[i] = initial[i].ID
				if initial[i].ID == 66 {
					initial[i].CreatedByUserID = 5
				}
			}
			verifyScheduleDeletionRows(t, ctx, pool, initial, ids)
			if !reflect.DeepEqual(following.Initial, readWorkScheduleUpdateNotices(t, ctx, pool)) {
				t.Fatal("forbidden batch wrote notices")
			}
		})
		if len(fake.recorded()) > 0 || len(readChatworkLogs(t, ctx, pool)) > 0 {
			t.Fatal("disabled Chatwork attempted delivery")
		}
	})
}

func checkSeriesDeletionCommit(ctx context.Context, pool *pgxpool.Pool, initial []workschedules.Entry, remaining []int, notices int) error {
	for _, entry := range initial {
		if slices.Contains(remaining, entry.ID) {
			continue
		}
		row, err := workschedules.Get(ctx, pool, entry.ID)
		if err != nil {
			return err
		}
		if row != nil {
			return fmt.Errorf("work schedule %d was still visible during delivery", entry.ID)
		}
	}
	return checkWorkScheduleCreationCounts(ctx, pool, len(remaining), notices)
}

func failSecondWorkScheduleSeriesDelete(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	execWorkScheduleTestSQL(t, ctx, pool, `
		CREATE FUNCTION fail_second_work_schedule_delete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		    IF current_setting('mrb_test.series_delete_started', true) = 'yes' THEN
		        RAISE EXCEPTION 'synthetic second delete failure';
		    END IF;
		    PERFORM set_config('mrb_test.series_delete_started', 'yes', true);
		    RETURN OLD;
		END $$;
		CREATE TRIGGER fail_second_work_schedule_delete BEFORE DELETE ON work_schedule_entries
		FOR EACH ROW EXECUTE FUNCTION fail_second_work_schedule_delete()`)
	t.Cleanup(func() {
		execWorkScheduleTestSQL(t, ctx, pool, `DROP TRIGGER fail_second_work_schedule_delete ON work_schedule_entries;
			DROP FUNCTION fail_second_work_schedule_delete()`)
	})
}

func startWorkScheduleBatchDelete(t *testing.T, ctx context.Context, target string, cookie *http.Cookie) <-chan seriesEditResult {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	return startSeriesEditRequest(req)
}

func verifySeriesDeletionSummary(t *testing.T, ctx context.Context, pool *pgxpool.Pool, referenceID int, title, count string) {
	t.Helper()
	found := false
	for _, notice := range readWorkScheduleUpdateNotices(t, ctx, pool) {
		if !strings.Contains(notice.Message, "をまとめて削除しました。") {
			continue
		}
		found = true
		if notice.TargetWorkScheduleEntryID != referenceID || !strings.Contains(notice.Message, "「"+title+"」") ||
			!strings.Contains(notice.Message, "件数: "+count) {
			t.Fatalf("series deletion summary used a stale target or count: %+v", notice)
		}
	}
	if !found {
		t.Fatal("missing series deletion summary")
	}
}
