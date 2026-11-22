package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type seriesEditRow struct {
	ID, UserID, NumberOfPeople int
	Name, Room, Type, Purpose  string
	Date, StartTime, EndTime   string
	Participants, RepeatType   string
	SeriesID, RepeatUntil      *string
	ParticipantIDs             []int
}

type seriesEditCase struct {
	Name, Dataset, Actor, Trigger string
	ID, Status                    int
	Request, Body                 json.RawMessage
	Updated                       []seriesEditRow
	Notifications                 []notifications.Entry
}

func TestRecurringReservationUpdatePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "RESERVATION_SERIES_UPDATE_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0003_enable_reservation_creation.sql")
	applyReservationNotificationMigrations(t, ctx, pool)
	passwords := seedAuthTestUsers(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		UPDATE users SET name = CASE id WHEN 1 THEN '架空の管理者' ELSE '架空の予約者' END;
		INSERT INTO users (id, name, email, password_hash)
		SELECT fixture.id, fixture.name, fixture.email, u.password_hash
		FROM users u CROSS JOIN (VALUES
		    (3, '架空の参加者', 'other@example.test'),
		    (4, '架空の追加参加者', 'added@example.test')
		) fixture(id, name, email) WHERE u.id = 2`); err != nil {
		t.Fatal(err)
	}
	addr, stop := startAuthTestAPI(t, ctx, dsn, seriesTestOrigin)
	defer stop()
	base := "http://" + addr
	cookies := make(map[string]*http.Cookie)
	for role, email := range map[string]string{"owner": "member", "admin": "admin", "other": "other"} {
		password := passwords["member"]
		if role == "admin" {
			password = passwords["admin"]
		}
		login, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email + "@example.test", "password": password}, nil, http.StatusOK)
		cookies[role] = authTestCookie(t, login, false)
	}

	data, err := os.ReadFile("../../internal/httpapi/testdata/reservation_series_update_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Datasets map[string][]seriesEditRow
		Cases    []seriesEditCase
	}
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range capture.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			seedSeriesEditRows(t, ctx, pool, capture.Datasets[scenario.Dataset])
			switch scenario.Trigger {
			case "notice":
				constrainSeriesTestWrites(t, ctx, pool, "notifications", "target_reservation_id <> 12")
			case "update":
				constrainSeriesTestWrites(t, ctx, pool, "reservations", "id <> 12 OR room <> '新会議室'")
			}
			request := seriesTestRequest(t, ctx, base+seriesEditPath(scenario.ID), scenario.Request, cookies[scenario.Actor])
			started := time.Now()
			response, body := sendSeriesTestRequest(t, request, scenario.Status)
			if response.Header.Get("Access-Control-Allow-Origin") != seriesTestOrigin ||
				response.Header.Get("Access-Control-Allow-Credentials") != "true" {
				t.Fatal("missing credentialed CORS headers")
			}
			switch {
			case scenario.Name == "explicit_null_notify":
				var problem struct{ Status int }
				if json.Unmarshal(body, &problem) != nil || problem.Status != http.StatusBadRequest {
					t.Fatalf("expected 400 Problem Details, got %s", body)
				}
			case string(scenario.Body) == `""`:
				if len(body) != 0 {
					t.Fatalf("expected an empty response, got %q", body)
				}
			default:
				compareSeriesCreateError(t, body, scenario.Body, scenario.Status)
			}

			want := make(map[int]seriesEditRow)
			for _, entry := range capture.Datasets[scenario.Dataset] {
				want[entry.ID] = entry
			}
			// C# commits updates before a notice write fails. Go rolls them back.
			if scenario.Trigger == "" {
				for _, entry := range scenario.Updated {
					want[entry.ID] = entry
				}
			}
			got := readSeriesEditRows(t, ctx, base, cookies["owner"])
			if len(got) != len(want) {
				t.Fatalf("got %d saved rows, want %d", len(got), len(want))
			}
			for id, expected := range want {
				if !reflect.DeepEqual(got[id], expected) {
					t.Errorf("row %d = %+v, want %+v", id, got[id], expected)
				}
			}
			assertSeriesEditNotifications(t, ctx, pool, scenario.Notifications, started)
		})
	}

	keyed := capture.Datasets["keyed"]
	owner := cookies["owner"]
	t.Run("untrusted_origin", func(t *testing.T) {
		seedSeriesEditRows(t, ctx, pool, keyed)
		request := seriesEditRequest(t, ctx, base, owner, keyed[1], "all", "2026-06-09", "blocked")
		request.Header.Set("Origin", "https://untrusted.example.test")
		response, _ := sendSeriesTestRequest(t, request, http.StatusForbidden)
		if response.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin received an allowed-origin header")
		}
		if got := readSeriesEditRows(t, ctx, base, owner)[11]; !reflect.DeepEqual(got, keyed[1]) {
			t.Fatalf("untrusted request changed reservation 11: %+v", got)
		}
	})
	t.Run("overlapping_series_edits_take_rows_in_ID_order", func(t *testing.T) {
		seedSeriesEditRows(t, ctx, pool, keyed)
		blocker := beginSeriesEditBlocker(t, ctx, pool)
		lockEditTestRow(t, ctx, blocker, 10, false)
		first := startSeriesEditRequest(seriesEditRequest(t, ctx, base, owner, keyed[3], "all", "2026-06-16", "first"))
		waitForSeriesEditRowLocks(t, ctx, pool, 1)
		second := startSeriesEditRequest(seriesEditRequest(t, ctx, base, owner, keyed[1], "all", "2026-06-10", "second"))
		waitForSeriesEditRowLocks(t, ctx, pool, 2)
		// Neither request may lock its higher-ID base before the shared first row.
		lockEditTestRow(t, ctx, blocker, 13, true)
		lockEditTestRow(t, ctx, blocker, 11, true)
		releaseSeriesEditBlocker(t, ctx, blocker)
		assertSeriesEditResult(t, first, http.StatusNoContent)
		assertSeriesEditResult(t, second, http.StatusNoContent)
		assertSeriesEditDates(t, readSeriesEditRows(t, ctx, base, owner), "second",
			map[int]string{10: "2026-06-03", 11: "2026-06-10", 12: "2026-06-10", 13: "2026-06-17"})
	})
	t.Run("following_retries_when_an_earlier_row_enters_scope", func(t *testing.T) {
		seedSeriesEditRows(t, ctx, pool, keyed)
		blocker := beginSeriesEditBlocker(t, ctx, pool)
		newDay := seriesTestTicks(t, "2026-05-25T00:00:00")
		if _, err := blocker.Exec(ctx, `
			UPDATE reservations SET date_ticks = $1, start_time_ticks = $1 + 324000000000,
			    end_time_ticks = $1 + 360000000000 WHERE id = 11`, newDay); err != nil {
			t.Fatal(err)
		}
		edit := startSeriesEditRequest(seriesEditRequest(t, ctx, base, owner, keyed[1], "following", "2026-06-09", "expanded scope"))
		waitForSeriesEditRowLocks(t, ctx, pool, 1)
		releaseSeriesEditBlocker(t, ctx, blocker)
		assertSeriesEditResult(t, edit, http.StatusNoContent)
		assertSeriesEditDates(t, readSeriesEditRows(t, ctx, base, owner), "expanded scope",
			map[int]string{10: "2026-06-16", 11: "2026-06-09", 12: "2026-06-23", 13: "2026-06-30"})
	})
	for _, slot := range []struct {
		name, room, day, start, end string
		createStatus                int
		createFirst                 bool
	}{
		{"destination", "新会議室", "2026-06-02", "10:15:00", "10:45:00", http.StatusConflict, false},
		{"vacated", "旧会議室", "2026-06-01", "09:15:00", "09:45:00", http.StatusCreated, false},
		{"destination_create_first", "新会議室", "2026-06-02", "10:15:00", "10:45:00", http.StatusCreated, true},
	} {
		t.Run("concurrent_create_at_"+slot.name, func(t *testing.T) {
			seedSeriesEditRows(t, ctx, pool, keyed)
			blocker := beginSeriesEditBlocker(t, ctx, pool)
			if _, err := blocker.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), $2::text::date - DATE '0001-01-01')`, slot.room, slot.day); err != nil {
				t.Fatal(err)
			}
			const waits = `SELECT count(*) FROM pg_locks
				WHERE locktype = 'advisory' AND NOT granted AND objsubid = 2
				  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
				  AND classid = hashtext($1)::oid AND objid = ($2::text::date - DATE '0001-01-01')::oid`
			payload := fmt.Appendf(nil, `{"name":"架空の予約者","room":%q,"date":%q,"startTime":%q,"endTime":%q}`,
				slot.room, slot.day, slot.day+"T"+slot.start, slot.day+"T"+slot.end)
			editRequest := seriesEditRequest(t, ctx, base, owner, keyed[1], "all", "2026-06-09", "moved series")
			createRequest := seriesTestRequest(t, ctx, base+"/api/Reservations", payload, owner)
			first, second := editRequest, createRequest
			if slot.createFirst {
				first, second = second, first
			}
			firstResult := startSeriesEditRequest(first)
			waitForSeriesEditLocks(t, ctx, pool, 1, waits, slot.room, slot.day)
			secondResult := startSeriesEditRequest(second)
			waitForSeriesEditLocks(t, ctx, pool, 2, waits, slot.room, slot.day)
			releaseSeriesEditBlocker(t, ctx, blocker)
			if slot.createFirst {
				firstResult, secondResult = secondResult, firstResult
			}
			assertSeriesEditResult(t, firstResult, http.StatusNoContent)
			assertSeriesEditResult(t, secondResult, slot.createStatus)
			rows := readSeriesEditRows(t, ctx, base, owner)
			assertSeriesEditDates(t, rows, "moved series",
				map[int]string{10: "2026-06-02", 11: "2026-06-09", 12: "2026-06-09", 13: "2026-06-16"})
			want := len(keyed)
			if slot.createStatus == http.StatusCreated {
				want++
			}
			if len(rows) != want {
				t.Fatalf("row count after competing create = %d, want %d", len(rows), want)
			}
		})
	}
}

func seriesEditPath(id int) string {
	return "/api/Reservations/" + strconv.Itoa(id) + "/series-update"
}

func seedSeriesEditRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rows []seriesEditRow) {
	t.Helper()
	resetSeriesTestData(t, ctx, pool)
	for _, row := range rows {
		var repeatUntil *int64
		if row.RepeatUntil != nil {
			repeatUntil = new(seriesTestTicks(t, *row.RepeatUntil))
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO reservations (id, user_id, name, room, number_of_people, type, purpose, series_id,
			    date_ticks, start_time_ticks, end_time_ticks, participant_ids, participants, repeat_type, repeat_until_ticks)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			row.ID, row.UserID, row.Name, row.Room, row.NumberOfPeople, row.Type, row.Purpose, row.SeriesID,
			seriesTestTicks(t, row.Date), seriesTestTicks(t, row.StartTime), seriesTestTicks(t, row.EndTime),
			row.ParticipantIDs, row.Participants, row.RepeatType, repeatUntil)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func readSeriesEditRows(t *testing.T, ctx context.Context, base string, cookie *http.Cookie) map[int]seriesEditRow {
	t.Helper()
	_, body := authTestRequest(t, ctx, base, http.MethodGet, "/api/Reservations", nil, cookie, http.StatusOK)
	var entries []seriesEditRow
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatal(err)
	}
	rows := make(map[int]seriesEditRow, len(entries))
	for _, entry := range entries {
		rows[entry.ID] = entry
	}
	return rows
}

func assertSeriesEditNotifications(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expected []notifications.Entry, started time.Time) {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT user_id, message, target_date_ticks, is_read, type, target_reservation_id,
		    target_work_schedule_entry_id, created_at_ticks
		FROM notifications ORDER BY target_reservation_id, user_id, id`)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (notifications.Entry, error) {
		var notice notifications.Entry
		var created, target int64
		err := row.Scan(&notice.UserID, &notice.Message, &target, &notice.IsRead, &notice.Type, &notice.TargetReservationID,
			&notice.TargetWorkScheduleEntryID, &created)
		notice.TargetDate = dotnettime.FromTicks(target).Format(dotnettime.Layout)
		now := time.Now()
		if err == nil && (created < dotnettime.Ticks(dotnettime.OnDate(started, started)) || created > dotnettime.Ticks(dotnettime.OnDate(now, now))) {
			return notice, fmt.Errorf("unexpected notification creation time: %d", created)
		}
		return notice, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(actual, expected, func(left, right notifications.Entry) bool { return reflect.DeepEqual(left, right) }) {
		t.Fatalf("stored notifications = %+v, want %+v", actual, expected)
	}
}

func seriesEditRequest(t *testing.T, ctx context.Context, base string, cookie *http.Cookie, row seriesEditRow,
	scope, day, purpose string) *http.Request {
	t.Helper()
	// Marshal an explicit envelope so both snapshots use the same inputs as Blazor.
	original := map[string]any{"id": row.ID, "name": row.Name, "room": row.Room}
	updated := map[string]any{
		"id": row.ID, "userId": row.UserID, "name": row.Name, "room": "新会議室", "purpose": purpose, "date": day,
		"startTime": day + "T10:00:00", "endTime": day + "T11:00:00", "repeatType": row.RepeatType, "repeatUntil": row.RepeatUntil,
	}
	data, err := json.Marshal(map[string]any{
		"originalReservation": original, "updatedReservation": updated, "scope": scope, "notifyParticipants": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	return seriesTestRequest(t, ctx, base+seriesEditPath(row.ID), data, cookie)
}

type seriesEditResult struct {
	status int
	err    error
}

func startSeriesEditRequest(request *http.Request) <-chan seriesEditResult {
	result := make(chan seriesEditResult, 1)
	go func() {
		status, err := reservationTestStatus(request)
		result <- seriesEditResult{status, err}
	}()
	return result
}

func assertSeriesEditResult(t *testing.T, result <-chan seriesEditResult, want int) {
	t.Helper()
	got := <-result
	if got.err != nil || got.status != want {
		t.Fatalf("HTTP status %d, want %d (error %v)", got.status, want, got.err)
	}
}

func beginSeriesEditBlocker(t *testing.T, ctx context.Context, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func releaseSeriesEditBlocker(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func lockEditTestRow(t *testing.T, ctx context.Context, tx pgx.Tx, id int, noWait bool) {
	t.Helper()
	query := `SELECT id FROM reservations WHERE id = $1 FOR UPDATE`
	if noWait {
		query += ` NOWAIT`
	}
	if _, err := tx.Exec(ctx, query, id); err != nil {
		t.Fatal(err)
	}
}

func waitForSeriesEditRowLocks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, count int) {
	t.Helper()
	waitForSeriesEditLocks(t, ctx, pool, count, `SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database() AND wait_event_type = 'Lock'
		  AND query LIKE '%WHERE id = ANY($1::integer[]) ORDER BY id FOR UPDATE%'`)
}

func waitForSeriesEditLocks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, count int, query string, args ...any) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		var waiting int
		if err := pool.QueryRow(waitCtx, query, args...).Scan(&waiting); err != nil {
			t.Fatalf("expected %d waiting writers: %v", count, err)
		}
		if waiting == count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertSeriesEditDates(t *testing.T, entries map[int]seriesEditRow, purpose string, days map[int]string) {
	t.Helper()
	for id, day := range days {
		row := entries[id]
		if row.Date != day+"T00:00:00" || row.Purpose != purpose {
			t.Errorf("row %d date/purpose = %s / %s, want %s / %s", id, row.Date, row.Purpose, day, purpose)
		}
	}
}
