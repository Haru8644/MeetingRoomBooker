package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	seriesTestPath   = "/api/Reservations/series"
	seriesTestOrigin = "http://127.0.0.1:5169"
)

type seriesCreateCase struct {
	Name, Query, Location string
	Status                int
	Anonymous             bool
	Request, Body         json.RawMessage
	Existing              []map[string]any
	Notifications         []notifications.Entry
	Appointments          []seriesTestAppointment
}

type seriesTestAppointment struct {
	ID, ParticipantID       int
	Date, Start, End, Title string
}

func TestRecurringReservationCreatePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "RESERVATION_SERIES_CREATE_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0003_enable_reservation_creation.sql")
	applyReservationNotificationMigrations(t, ctx, pool)
	passwords := seedAuthTestUsers(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		UPDATE users SET name = '架空の予約者' WHERE id = 2;
		INSERT INTO users (id, name, email, password_hash)
		SELECT fixture.id, fixture.name, fixture.email, u.password_hash
		FROM users u CROSS JOIN (VALUES
		    (3, '架空の参加者', 'participant@example.test'),
		    (4, '架空の別参加者', 'other@example.test')
		) fixture(id, name, email) WHERE u.id = 2`); err != nil {
		t.Fatal(err)
	}
	addr, stop := startAuthTestAPI(t, ctx, dsn, seriesTestOrigin)
	defer stop()
	base := "http://" + addr
	login, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
		map[string]string{"email": "member@example.test", "password": passwords["member"]}, nil, http.StatusOK)
	organizer := authTestCookie(t, login, false)

	data, err := os.ReadFile("../../internal/httpapi/testdata/reservation_series_create_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture []seriesCreateCase
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}

	seenSeries := make(map[string]bool)
	for _, scenario := range capture {
		t.Run(scenario.Name, func(t *testing.T) {
			resetSeriesTestData(t, ctx, pool)
			seedSeriesTestData(t, ctx, pool, scenario)
			cookie := organizer
			if scenario.Anonymous {
				cookie = nil
			}
			request := seriesTestRequest(t, ctx, base+seriesTestPath+scenario.Query, scenario.Request, cookie)
			started := time.Now()
			response, body := sendSeriesTestRequest(t, request, scenario.Status)
			wantStored := append([]map[string]any{}, scenario.Existing...)
			generatedIDs := make(map[int]int)
			if scenario.Status == http.StatusCreated {
				if response.Header.Get("Location") != base+scenario.Location {
					t.Fatalf("Location = %q, want %q", response.Header.Get("Location"), base+scenario.Location)
				}
				if response.Header.Get("Access-Control-Allow-Origin") != seriesTestOrigin ||
					response.Header.Get("Access-Control-Allow-Credentials") != "true" {
					t.Fatal("missing credentialed CORS headers")
				}
				created, ids, seriesID := compareSeriesCreation(t, body, scenario.Body)
				if seriesID != "" && seenSeries[seriesID] {
					t.Fatalf("reused series ID %q", seriesID)
				}
				seenSeries[seriesID] = true
				wantStored = append(wantStored, created...)
				generatedIDs = ids
			} else {
				compareSeriesCreateError(t, body, scenario.Body, scenario.Status)
			}
			assertStoredSeriesReservations(t, ctx, base, organizer, wantStored)
			assertStoredSeriesNotifications(t, ctx, pool, scenario.Notifications, generatedIDs, started)
		})
	}

	t.Run("untrusted_origin", func(t *testing.T) {
		resetSeriesTestData(t, ctx, pool)
		request := seriesTestRequest(t, ctx, base+seriesTestPath, capture[0].Request, organizer)
		request.Header.Set("Origin", "https://untrusted.example.test")
		response, _ := sendSeriesTestRequest(t, request, http.StatusForbidden)
		if response.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin received an allowed-origin header")
		}
		assertSeriesCounts(t, ctx, pool, 0, 0)
	})

	t.Run("notification_failure_rolls_back_all", func(t *testing.T) {
		resetSeriesTestData(t, ctx, pool)
		constrainSeriesTestWrites(t, ctx, pool, "notifications", "user_id <> 4")
		request := seriesTestRequest(t, ctx, base+seriesTestPath, capture[0].Request, organizer)
		sendSeriesTestRequest(t, request, http.StatusInternalServerError)
		assertSeriesCounts(t, ctx, pool, 0, 0)
	})

	t.Run("later_insert_failure_rolls_back_all", func(t *testing.T) {
		resetSeriesTestData(t, ctx, pool)
		secondDate := seriesTestTicks(t, "2026-06-23T00:00:00")
		constrainSeriesTestWrites(t, ctx, pool, "reservations", "date_ticks < "+strconv.FormatInt(secondDate, 10))
		request := seriesTestRequest(t, ctx, base+seriesTestPath, capture[0].Request, organizer)
		sendSeriesTestRequest(t, request, http.StatusInternalServerError)
		assertSeriesCounts(t, ctx, pool, 0, 0)
		var allocated int
		if err := pool.QueryRow(ctx, "SELECT last_value FROM reservations_id_seq").Scan(&allocated); err != nil {
			t.Fatal(err)
		}
		if allocated != 2 {
			t.Fatalf("sequence allocated %d IDs, want the successful first and failing second inserts", allocated)
		}
	})

	t.Run("concurrent_single_create", func(t *testing.T) {
		resetSeriesTestData(t, ctx, pool)
		assertConcurrentSeriesAndSingle(t, ctx, pool, base, organizer, capture[0].Request)
	})

	t.Run("concurrent_series_lock_order", func(t *testing.T) {
		resetSeriesTestData(t, ctx, pool)
		assertCanonicalSeriesLocks(t, ctx, pool, base, organizer)
	})
}

func seriesTestRequest(t *testing.T, ctx context.Context, url string, data []byte, cookie *http.Cookie) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Origin", seriesTestOrigin)
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func sendSeriesTestRequest(t *testing.T, request *http.Request, status int) (*http.Response, []byte) {
	t.Helper()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("%s: status %d, want %d. Body: %s", request.URL.Path, response.StatusCode, status, body)
	}
	return response, body
}

func compareSeriesCreateError(t *testing.T, actual, reference []byte, status int) {
	t.Helper()
	if status == http.StatusUnauthorized {
		if len(actual) != 0 || string(reference) != `""` {
			t.Fatalf("expected empty unauthorized response, got %q", actual)
		}
		return
	}
	var got, want any
	if json.Unmarshal(actual, &got) != nil || json.Unmarshal(reference, &want) != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("body %s, want %s", actual, reference)
	}
}

func compareSeriesCreation(t *testing.T, actual, reference []byte) ([]map[string]any, map[int]int, string) {
	t.Helper()
	got, want := decodeSeriesRows(t, actual), decodeSeriesRows(t, reference)
	if len(got) != len(want) {
		t.Fatalf("returned %d rows, want %d", len(got), len(want))
	}
	var seriesID string
	ids := make(map[int]int, len(got))
	seenIDs := make(map[int]bool, len(got))
	for i, row := range got {
		id, ok := row["id"].(float64)
		if !ok || id <= 0 || seenIDs[int(id)] {
			t.Fatalf("invalid or reused reservation ID %v", row["id"])
		}
		seenIDs[int(id)] = true
		captured := want[i]
		ids[int(captured["id"].(float64))] = int(id)
		captured["id"] = id
		if captured["seriesId"] != nil {
			key, ok := row["seriesId"].(string)
			raw, err := hex.DecodeString(key)
			if !ok || err != nil || len(raw) != 16 || key != strings.ToLower(key) ||
				(seriesID != "" && key != seriesID) {
				t.Fatalf("invalid or unshared series ID %q", key)
			}
			seriesID = key
			captured["seriesId"] = key
		}
		// C# POST preserves the UTC marker; both SQLite reads and Go responses use wall times.
		for _, field := range []string{"date", "startTime", "endTime", "repeatUntil"} {
			if date, ok := captured[field].(string); ok {
				captured[field] = strings.TrimSuffix(date, "Z")
			}
		}
		if !reflect.DeepEqual(row, captured) {
			t.Fatalf("row %d = %v, want %v", i, row, captured)
		}
	}
	return got, ids, seriesID
}

func decodeSeriesRows(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func assertStoredSeriesReservations(t *testing.T, ctx context.Context, base string, cookie *http.Cookie, want []map[string]any) {
	t.Helper()
	_, body := authTestRequest(t, ctx, base, http.MethodGet, "/api/Reservations", nil, cookie, http.StatusOK)
	stored := decodeSeriesRows(t, body)
	if len(stored) != len(want) {
		t.Fatalf("%d stored reservations, want %d", len(stored), len(want))
	}
	byID := make(map[any]map[string]any)
	for _, row := range stored {
		byID[row["id"]] = row
	}
	for _, row := range want {
		if !reflect.DeepEqual(byID[row["id"]], row) {
			t.Fatalf("stored row %v, want %v", byID[row["id"]], row)
		}
	}
}

func assertStoredSeriesNotifications(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want []notifications.Entry, ids map[int]int, started time.Time) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(want) {
		t.Fatalf("%d stored notifications, want %d", count, len(want))
	}
	inbox := notifications.NewInbox(pool)
	for _, expected := range want {
		entries, err := inbox.List(ctx, expected.UserID)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("user %d has %d notifications, want 1", expected.UserID, len(entries))
		}
		entry := entries[0]
		created, err := time.ParseInLocation(dotnettime.Layout, entry.CreatedAt, time.Local)
		if err != nil || created.Before(started.Truncate(100*time.Nanosecond)) || created.After(time.Now()) || entry.ID <= 0 {
			t.Fatalf("unexpected generated notice ID or creation time: %+v", entry)
		}
		targetID, ok := ids[*expected.TargetReservationID]
		if !ok {
			t.Fatalf("C# notification target %d does not identify a created row", *expected.TargetReservationID)
		}
		expected.ID, expected.CreatedAt, expected.TargetReservationID = entry.ID, entry.CreatedAt, &targetID
		if !reflect.DeepEqual(entry, expected) {
			t.Fatalf("notification for user %d: %+v, want %+v", expected.UserID, entry, expected)
		}
	}
}

func seedSeriesTestData(t *testing.T, ctx context.Context, pool *pgxpool.Pool, scenario seriesCreateCase) {
	t.Helper()
	for _, row := range scenario.Existing {
		_, err := pool.Exec(ctx, `
			INSERT INTO reservations (id, user_id, name, room, number_of_people, type, purpose,
			    date_ticks, start_time_ticks, end_time_ticks, participants, repeat_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			int(row["id"].(float64)), int(row["userId"].(float64)), row["name"], row["room"], int(row["numberOfPeople"].(float64)),
			row["type"], row["purpose"], seriesTestTicks(t, row["date"].(string)), seriesTestTicks(t, row["startTime"].(string)),
			seriesTestTicks(t, row["endTime"].(string)), row["participants"], row["repeatType"])
		if err != nil {
			t.Fatal(err)
		}
	}
	seedSeriesTestAppointments(t, ctx, pool, scenario.Appointments)
}

func seedSeriesTestAppointments(t *testing.T, ctx context.Context, pool *pgxpool.Pool, appointments []seriesTestAppointment) {
	t.Helper()
	for _, a := range appointments {
		date := seriesTestTicks(t, a.Date+"T00:00:00")
		_, err := pool.Exec(ctx, `
			INSERT INTO work_schedule_entries
			    (id, created_by_user_id, type, title, date_ticks, start_time_ticks, end_time_ticks,
			     leave_period, participant_ids, participants, created_at_ticks, updated_at_ticks)
			VALUES ($1, 2, 0, $2, $3, $4, $5, 0, $6, '', $3, $3)`,
			a.ID, a.Title, date, seriesTestTicks(t, a.Date+"T"+a.Start),
			seriesTestTicks(t, a.Date+"T"+a.End), []int{a.ParticipantID})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func seriesTestTicks(t *testing.T, value string) int64 {
	t.Helper()
	date, err := time.Parse(dotnettime.Layout, value)
	if err != nil {
		t.Fatal(err)
	}
	return dotnettime.Ticks(date)
}

func resetSeriesTestData(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "TRUNCATE reservations, notifications, work_schedule_entries RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}
}

func constrainSeriesTestWrites(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, condition string) {
	t.Helper()
	if _, err := pool.Exec(ctx, "ALTER TABLE "+table+" ADD CONSTRAINT series_test_failure CHECK ("+condition+")"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "ALTER TABLE "+table+" DROP CONSTRAINT series_test_failure"); err != nil {
			t.Error(err)
		}
	})
}

func assertSeriesCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantReservations, wantNotifications int) {
	t.Helper()
	var reservations, notices int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM reservations), (SELECT count(*) FROM notifications)`).
		Scan(&reservations, &notices); err != nil {
		t.Fatal(err)
	}
	if reservations != wantReservations || notices != wantNotifications {
		t.Fatalf("stored %d reservations and %d notifications, want %d and %d", reservations, notices, wantReservations, wantNotifications)
	}
}

func assertConcurrentSeriesAndSingle(t *testing.T, ctx context.Context, pool *pgxpool.Pool, base string, cookie *http.Cookie, series []byte) {
	t.Helper()
	const single = `{"name":"架空の予約者","room":"大会議室","purpose":"同時の単発","date":"2026-06-23","startTime":"2026-06-23T09:15:00","endTime":"2026-06-23T09:45:00"}`
	seriesStatus, singleStatus := competeForReservationSlot(t, ctx, pool, "大会議室", "2026-06-23", "",
		seriesTestRequest(t, ctx, base+seriesTestPath, series, cookie),
		seriesTestRequest(t, ctx, base+"/api/Reservations", []byte(single), cookie))
	switch {
	case seriesStatus == http.StatusCreated && singleStatus == http.StatusConflict:
		assertSeriesCounts(t, ctx, pool, 3, 2)
	case seriesStatus == http.StatusConflict && singleStatus == http.StatusCreated:
		assertSeriesCounts(t, ctx, pool, 1, 0)
	default:
		t.Fatalf("series = %d, single = %d; want one 201 and one 409", seriesStatus, singleStatus)
	}
	t.Logf("both writers waited for the same lock; series %d, single %d", seriesStatus, singleStatus)
}

func assertCanonicalSeriesLocks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, base string, cookie *http.Cookie) {
	t.Helper()
	firstRoom, secondRoom := "北会議室", "南会議室"
	var ascending bool
	if err := pool.QueryRow(ctx, "SELECT hashtext($1) < hashtext($2)", firstRoom, secondRoom).Scan(&ascending); err != nil {
		t.Fatal(err)
	}
	if !ascending {
		firstRoom, secondRoom = secondRoom, firstRoom
	}
	const day = "2026-06-22"
	series := func(first, second string) []byte {
		const occurrence = `{"name":"架空の予約者","room":%q,"date":"2026-06-22",
			"startTime":"2026-06-22T%s:00","endTime":"2026-06-22T10:30:00",
			"repeatType":"毎日","repeatUntil":"2026-06-22"}`
		return []byte("[" + fmt.Sprintf(occurrence, first, "09:00") + "," + fmt.Sprintf(occurrence, second, "09:30") + "]")
	}
	first, second := competeForReservationSlot(t, ctx, pool, firstRoom, day, secondRoom,
		seriesTestRequest(t, ctx, base+seriesTestPath, series(firstRoom, secondRoom), cookie),
		seriesTestRequest(t, ctx, base+seriesTestPath, series(secondRoom, firstRoom), cookie))
	if (first != http.StatusCreated || second != http.StatusConflict) &&
		(first != http.StatusConflict || second != http.StatusCreated) {
		t.Fatalf("oppositely ordered series returned %d and %d; want one 201 and one 409", first, second)
	}
	assertSeriesCounts(t, ctx, pool, 2, 0)
}

func competeForReservationSlot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, room, day, laterRoom string,
	first, second *http.Request) (int, int) {
	t.Helper()
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), $2::text::date - DATE '0001-01-01')`, room, day); err != nil {
		t.Fatal(err)
	}
	type result struct {
		status int
		err    error
	}
	start := func(request *http.Request) <-chan result {
		done := make(chan result, 1)
		go func() {
			status, err := reservationTestStatus(request)
			done <- result{status, err}
		}()
		return done
	}
	firstDone, secondDone := start(first), start(second)
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		var waiting int
		err := pool.QueryRow(waitCtx, `
			SELECT count(*) FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted AND objsubid = 2
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND classid = hashtext($1)::oid AND objid = ($2::text::date - DATE '0001-01-01')::oid`, room, day).Scan(&waiting)
		if err != nil {
			t.Fatalf("both requests did not wait for the same lock: %v", err)
		}
		if waiting == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if laterRoom != "" {
		var free bool
		if err := blocker.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1), $2::text::date - DATE '0001-01-01')`, laterRoom, day).
			Scan(&free); err != nil || !free {
			t.Fatalf("later lock should still be available, got available=%t, error=%v", free, err)
		}
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	firstResult, secondResult := <-firstDone, <-secondDone
	if err := errors.Join(firstResult.err, secondResult.err); err != nil {
		t.Fatal(err)
	}
	return firstResult.status, secondResult.status
}
