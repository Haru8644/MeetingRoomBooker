package main

import (
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
)

func TestSingleReservationCancellationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "RESERVATION_CANCEL_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0003_enable_reservation_creation.sql")
	applyReservationNotificationMigrations(t, ctx, pool)
	passwords := seedAuthTestUsers(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash)
		SELECT 3, 'Other member', 'other@example.test', password_hash FROM users WHERE id = 2;
		INSERT INTO reservations
		    (id, user_id, name, room, number_of_people, type, purpose, series_id,
		     date_ticks, start_time_ticks, end_time_ticks, participant_ids, participants, repeat_type)
		SELECT id, 2, 'Organizer', 'Room A', 1, '社内', '',
		       CASE WHEN id IN (61, 62) THEN 'existing-series' END,
		       day, day + 360000000000, day + 396000000000, ARRAY[2,3], '', 'しない'
		FROM generate_series(61, 63) AS r(id)
		CROSS JOIN LATERAL
		    (SELECT (DATE '2026-07-10' - DATE '0001-01-01' + (r.id - 61))::bigint * 864000000000 AS day) ticks;
		INSERT INTO notifications (id, user_id, message, created_at_ticks, target_date_ticks, type, target_reservation_id)
		SELECT 5, 2, 'Existing notice', date_ticks, date_ticks, 'Info', 61 FROM reservations WHERE id = 61`); err != nil {
		t.Fatal(err)
	}
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	baseURL := "http://" + addr
	login := func(role, email string) *http.Cookie {
		response, _ := authTestRequest(t, ctx, baseURL, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email, "password": passwords[role]}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}
	cookies := map[string]*http.Cookie{
		"admin": login("admin", "admin@example.test"),
		"owner": login("member", "member@example.test"),
		"other": login("member", "other@example.test"),
	}
	const reservationsPath = "/api/Reservations"
	authTestRequest(t, ctx, baseURL, http.MethodDelete, reservationsPath+"/61", nil, nil, http.StatusUnauthorized)
	authTestRequest(t, ctx, baseURL, http.MethodDelete, reservationsPath+"/invalid", nil, nil, http.StatusNotFound)
	untrusted, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+reservationsPath+"/61", nil)
	if err != nil {
		t.Fatal(err)
	}
	untrusted.AddCookie(cookies["owner"])
	untrusted.Header.Set("Origin", "https://untrusted.example.test")
	untrusted.Header.Set("Sec-Fetch-Site", "cross-site")
	if status, err := reservationTestStatus(untrusted); err != nil || status != http.StatusForbidden {
		t.Fatalf("cross-origin cancellation = %d, %v; want 403", status, err)
	}

	type noticeReference struct {
		ID                  int
		TargetReservationID *int
	}
	var reference []struct {
		Name           string
		Actor          string
		ID             int
		Status         int
		ReservationIDs []int
		Notices        []noticeReference
	}
	data, err := os.ReadFile("../../internal/httpapi/testdata/reservation_cancel_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	for _, expected := range reference {
		t.Run(expected.Name, func(t *testing.T) {
			_, body := authTestRequest(t, ctx, baseURL, http.MethodDelete, reservationsPath+"/"+strconv.Itoa(expected.ID),
				nil, cookies[expected.Actor], expected.Status)
			if expected.Status == http.StatusNoContent && len(body) != 0 {
				t.Fatalf("successful cancellation returned a body: %s", body)
			}
			_, body = authTestRequest(t, ctx, baseURL, http.MethodGet, reservationsPath, nil, cookies["owner"], http.StatusOK)
			var rows []struct{ ID int }
			if err := json.Unmarshal(body, &rows); err != nil {
				t.Fatal(err)
			}
			ids := make([]int, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row.ID)
			}
			slices.Sort(ids)
			if !slices.Equal(ids, expected.ReservationIDs) {
				t.Fatalf("remaining bookings = %v, want %v", ids, expected.ReservationIDs)
			}
			_, body = authTestRequest(t, ctx, baseURL, http.MethodGet, "/api/Notifications/user/2", nil, cookies["owner"], http.StatusOK)
			var entries []notifications.Entry
			if err := json.Unmarshal(body, &entries); err != nil {
				t.Fatal(err)
			}
			got := make([]noticeReference, 0, len(entries))
			for _, entry := range entries {
				got = append(got, noticeReference{entry.ID, entry.TargetReservationID})
			}
			if !reflect.DeepEqual(got, expected.Notices) {
				t.Fatalf("notices after cancellation = %v, want %v", got, expected.Notices)
			}
		})
	}
	t.Run("concurrent_cancellation_of_same_row", func(t *testing.T) {
		var requests [2]*http.Request
		for i := range requests {
			request, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+reservationsPath+"/62", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.AddCookie(cookies["owner"])
			requests[i] = request
		}
		type outcome struct {
			status int
			err    error
		}
		results := make(chan outcome, len(requests))
		for _, request := range requests {
			go func() {
				status, err := reservationTestStatus(request)
				results <- outcome{status, err}
			}()
		}
		statuses := make([]int, 0, len(requests))
		for range requests {
			result := <-results
			if result.err != nil {
				t.Fatal(result.err)
			}
			statuses = append(statuses, result.status)
		}
		slices.Sort(statuses)
		if !slices.Equal(statuses, []int{http.StatusNoContent, http.StatusNotFound}) {
			t.Fatalf("concurrent cancellation returned %v; want 204 and 404", statuses)
		}
	})
	authTestRequest(t, ctx, baseURL, http.MethodPost, reservationsPath, json.RawMessage(
		`{"name":"Organizer","room":"Room A","date":"2026-07-10","startTime":"2026-07-10T10:00:00","endTime":"2026-07-10T11:00:00"}`),
		cookies["owner"], http.StatusCreated)
}
