package main

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecurringReservationDeletePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "RESERVATION_SERIES_DELETE_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0003_enable_reservation_creation.sql")
	applyReservationNotificationMigrations(t, ctx, pool)
	passwords := seedAuthTestUsers(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		UPDATE users SET name = '架空の予約者' WHERE id = 2;
		INSERT INTO users (id, name, email, password_hash)
		SELECT 3, '架空の参加者', 'other@example.test', password_hash FROM users WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	addr, stop := startAuthTestAPI(t, ctx, dsn, seriesTestOrigin)
	defer stop()
	base := "http://" + addr
	cookies := make(map[string]*http.Cookie)
	for _, account := range []struct{ role, email, password string }{
		{"admin", "admin", passwords["admin"]}, {"owner", "member", passwords["member"]}, {"other", "other", passwords["member"]},
	} {
		login, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": account.email + "@example.test", "password": account.password}, nil, http.StatusOK)
		cookies[account.role] = authTestCookie(t, login, false)
	}
	owner := cookies["owner"]

	var reference struct{ Datasets map[string][]seriesEditRow }
	loadSeriesDeleteFixture(t, "reservation_series_update_csharp.json", &reference)
	var captured []struct {
		Name, Dataset, ID, Actor string
		Status                   int
		Request, Body            json.RawMessage
		Remaining, Notifications []int
		ForceFailure             bool
	}
	loadSeriesDeleteFixture(t, "reservation_series_delete_csharp.json", &captured)
	for _, scenario := range captured {
		t.Run(scenario.Name, func(t *testing.T) {
			dataset := reference.Datasets[scenario.Dataset]
			notices := seedSeriesDeleteData(t, ctx, pool, dataset)
			if scenario.ForceFailure {
				rejectSecondSeriesDelete(t, ctx, pool)
			}
			request := seriesTestRequest(t, ctx, base+seriesDeletePath(scenario.ID), scenario.Request, cookies[scenario.Actor])
			response, body := sendSeriesTestRequest(t, request, scenario.Status)
			if response.Header.Get("Access-Control-Allow-Origin") != seriesTestOrigin ||
				response.Header.Get("Access-Control-Allow-Credentials") != "true" {
				t.Fatal("missing credentialed CORS headers")
			}
			switch {
			case scenario.Name == "invalid_scope_type":
				var problem struct {
					Status int
					Errors map[string][]string
				}
				if json.Unmarshal(body, &problem) != nil || problem.Status != scenario.Status ||
					!slices.Equal(problem.Errors["request"], []string{"A valid series delete JSON object is required."}) {
					t.Fatalf("unexpected JSON type validation: %s", body)
				}
			case string(scenario.Body) == `""`:
				if len(body) != 0 {
					t.Fatalf("expected an empty response, got %q", body)
				}
			default:
				compareSeriesCreateError(t, body, scenario.Body, scenario.Status)
			}
			if scenario.ForceFailure {
				var attempts int
				if err := pool.QueryRow(ctx, "SELECT last_value FROM series_delete_attempts").Scan(&attempts); err != nil || attempts != 2 {
					t.Fatalf("expected deletion to reach the second row, got %d (error %v)", attempts, err)
				}
			}
			got := readSeriesEditRows(t, ctx, base, owner)
			assertSeriesDeleteRows(t, got, dataset, scenario.Remaining)
			for i, notice := range notices {
				if i >= len(scenario.Notifications) || notice.ID != scenario.Notifications[i] {
					t.Fatalf("seeded notice ID %d is missing from the C# result", notice.ID)
				}
			}
			if len(notices) != len(scenario.Notifications) {
				t.Fatal("notice count does not match the C# result")
			}
			assertSeriesDeleteNotices(t, ctx, pool, notices)
		})
	}

	keyed := reference.Datasets["keyed"]
	t.Run("untrusted_origin", func(t *testing.T) {
		notices := seedSeriesDeleteData(t, ctx, pool, keyed)
		request := seriesDeleteTestRequest(t, ctx, base, owner, 11, "all")
		request.Header.Set("Origin", "https://untrusted.example.test")
		response, _ := sendSeriesTestRequest(t, request, http.StatusForbidden)
		if response.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin received an allowed-origin header")
		}
		assertSeriesDeleteRows(t, readSeriesEditRows(t, ctx, base, owner), keyed, []int{10, 11, 12, 13, 14, 15, 16, 17})
		assertSeriesDeleteNotices(t, ctx, pool, notices)
	})
	t.Run("delete_waits_for_edit_and_rechecks_following_scope", func(t *testing.T) {
		notices := seedSeriesDeleteData(t, ctx, pool, keyed)
		blocker := beginSeriesEditBlocker(t, ctx, pool)
		lockEditTestRow(t, ctx, blocker, 11, false)
		edit := startSeriesEditRequest(seriesEditRequest(t, ctx, base, owner, keyed[1], "following", "2026-05-25", "moves scope"))
		waitForSeriesEditRowLocks(t, ctx, pool, 1)
		remove := startSeriesEditRequest(seriesDeleteTestRequest(t, ctx, base, owner, 11, "following"))
		waitForSeriesEditRowLocks(t, ctx, pool, 2)
		releaseSeriesEditBlocker(t, ctx, blocker)
		assertSeriesEditResult(t, edit, http.StatusNoContent)
		assertSeriesEditResult(t, remove, http.StatusNoContent)
		// The earlier row 10 enters the scope only after 11 moves to May 25.
		assertSeriesDeleteRows(t, readSeriesEditRows(t, ctx, base, owner), keyed, []int{14, 15, 16, 17})
		assertSeriesDeleteNotices(t, ctx, pool, notices)
	})
	t.Run("edit_waits_for_delete_and_sees_missing_base", func(t *testing.T) {
		notices := seedSeriesDeleteData(t, ctx, pool, keyed)
		blocker := beginSeriesEditBlocker(t, ctx, pool)
		lockEditTestRow(t, ctx, blocker, 10, false)
		remove := startSeriesEditRequest(seriesDeleteTestRequest(t, ctx, base, owner, 11, "all"))
		waitForSeriesEditRowLocks(t, ctx, pool, 1)
		edit := startSeriesEditRequest(seriesEditRequest(t, ctx, base, owner, keyed[3], "all", "2026-06-16", "deleted base"))
		waitForSeriesEditRowLocks(t, ctx, pool, 2)
		// Neither operation should lock its higher-ID base ahead of row 10.
		lockEditTestRow(t, ctx, blocker, 11, true)
		lockEditTestRow(t, ctx, blocker, 13, true)
		releaseSeriesEditBlocker(t, ctx, blocker)
		assertSeriesEditResult(t, remove, http.StatusNoContent)
		assertSeriesEditResult(t, edit, http.StatusNotFound)
		assertSeriesDeleteRows(t, readSeriesEditRows(t, ctx, base, owner), keyed, []int{14, 15, 16, 17})
		assertSeriesDeleteNotices(t, ctx, pool, notices)
	})

	for _, schedule := range []struct {
		name, start, end string
		createFirst      bool
		createStatus     int
	}{
		{"delete_first", "09:15:00", "09:45:00", false, http.StatusCreated},
		{"overlapping_create_first", "09:15:00", "09:45:00", true, http.StatusConflict},
		{"adjacent_create_first", "10:00:00", "10:30:00", true, http.StatusCreated},
	} {
		t.Run(schedule.name, func(t *testing.T) {
			notices := seedSeriesDeleteData(t, ctx, pool, keyed)
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			const room, day = "旧会議室", "2026-06-01"
			if _, err := blocker.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), $2::text::date - DATE '0001-01-01')`, room, day); err != nil {
				t.Fatal(err)
			}
			remove := seriesDeleteTestRequest(t, ctx, base, owner, 11, "all")
			payload, err := json.Marshal(map[string]string{
				"name": "架空の予約者", "room": room, "date": day,
				"startTime": day + "T" + schedule.start, "endTime": day + "T" + schedule.end,
			})
			if err != nil {
				t.Fatal(err)
			}
			create := seriesTestRequest(t, ctx, base+"/api/Reservations", payload, owner)
			first, second := remove, create
			if schedule.createFirst {
				first, second = create, remove
			}
			const waits = `SELECT count(*) FROM pg_locks
				WHERE locktype = 'advisory' AND NOT granted AND objsubid = 2
				  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
				  AND classid = hashtext($1)::oid AND objid = ($2::text::date - DATE '0001-01-01')::oid`
			firstResult := startSeriesEditRequest(first)
			waitForSeriesEditLocks(t, ctx, pool, 1, waits, room, day)
			secondResult := startSeriesEditRequest(second)
			waitForSeriesEditLocks(t, ctx, pool, 2, waits, room, day)
			releaseSeriesEditBlocker(t, ctx, blocker)
			if schedule.createFirst {
				firstResult, secondResult = secondResult, firstResult
			}
			assertSeriesEditResult(t, firstResult, http.StatusNoContent)
			assertSeriesEditResult(t, secondResult, schedule.createStatus)
			got := readSeriesEditRows(t, ctx, base, owner)
			if schedule.createStatus == http.StatusCreated {
				created := got[1] // The test resets the identity before seeding explicit IDs.
				if created.Room != room || created.StartTime != day+"T"+schedule.start || created.EndTime != day+"T"+schedule.end {
					t.Fatalf("unexpected newly created booking: %+v", created)
				}
				delete(got, created.ID)
			}
			assertSeriesDeleteRows(t, got, keyed, []int{14, 15, 16, 17})
			assertSeriesDeleteNotices(t, ctx, pool, notices)
		})
	}
}

func loadSeriesDeleteFixture(t *testing.T, file string, target any) {
	t.Helper()
	data, err := os.ReadFile("../../internal/httpapi/testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func seriesDeletePath(id string) string {
	return "/api/Reservations/" + id + "/series-delete"
}

func seriesDeleteTestRequest(t *testing.T, ctx context.Context, base string, owner *http.Cookie, id int, scope string) *http.Request {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"originalReservation": map[string]string{"name": "架空の予約者", "room": "旧会議室"}, "scope": scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	return seriesTestRequest(t, ctx, base+seriesDeletePath(strconv.Itoa(id)), payload, owner)
}

func seedSeriesDeleteData(t *testing.T, ctx context.Context, pool *pgxpool.Pool, dataset []seriesEditRow) []notifications.Entry {
	t.Helper()
	seedSeriesEditRows(t, ctx, pool, dataset)
	const createdAt, targetDate = "2026-05-01T10:00:00", "2026-06-08T00:00:00"
	notices := make([]notifications.Entry, len(dataset)+1)
	for i := range notices {
		notice := notifications.Entry{ID: i + 1, UserID: 2, Message: "既存の架空通知 " + strconv.Itoa(i+1),
			CreatedAt: createdAt, TargetDate: targetDate, IsRead: i%2 == 0, Type: "Info"}
		if i < len(dataset) {
			notice.TargetReservationID = new(dataset[i].ID)
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO notifications (id, user_id, message, created_at_ticks, target_date_ticks, is_read, type, target_reservation_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, notice.ID, notice.UserID, notice.Message,
			seriesTestTicks(t, createdAt), seriesTestTicks(t, targetDate), notice.IsRead, notice.Type, notice.TargetReservationID)
		if err != nil {
			t.Fatal(err)
		}
		notices[i] = notice
	}
	return notices
}

func assertSeriesDeleteRows(t *testing.T, rows map[int]seriesEditRow, dataset []seriesEditRow, ids []int) {
	t.Helper()
	if len(rows) != len(ids) {
		t.Errorf("kept %d reservations, want %d", len(rows), len(ids))
	}
	for _, expected := range dataset {
		row, found := rows[expected.ID]
		if slices.Contains(ids, expected.ID) {
			if !found || !reflect.DeepEqual(row, expected) {
				t.Errorf("reservation %d changed or disappeared: %+v", expected.ID, row)
			}
		} else if found {
			t.Errorf("reservation %d was not deleted", expected.ID)
		}
	}
}

func assertSeriesDeleteNotices(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expected []notifications.Entry) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&count); err != nil {
		t.Fatal(err)
	}
	actual, err := notifications.NewInbox(pool).List(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(actual, func(a, b notifications.Entry) int { return cmp.Compare(a.ID, b.ID) })
	if count != len(expected) || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("notification rows changed (%d total): %+v", count, actual)
	}
}

func rejectSecondSeriesDelete(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		CREATE SEQUENCE series_delete_attempts;
		CREATE FUNCTION reject_second_series_delete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF nextval('series_delete_attempts') = 2 THEN
				RAISE EXCEPTION 'synthetic second delete failure';
			END IF;
			RETURN OLD;
		END;
		$$;
		CREATE TRIGGER series_delete_failure BEFORE DELETE ON reservations
		    FOR EACH ROW EXECUTE FUNCTION reject_second_series_delete();`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `
			DROP TRIGGER series_delete_failure ON reservations;
			DROP FUNCTION reject_second_series_delete();
			DROP SEQUENCE series_delete_attempts;`); err != nil {
			t.Error(err)
		}
	})
}
