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
	"github.com/jackc/pgx/v5/pgxpool"
)

type participationReservation struct {
	ID             int
	ParticipantIDs []int
	Participants   string
	NumberOfPeople int
	StartTime      string
	EndTime        string
	RepeatType     string
	RepeatUntil    *string
	SeriesID       *string
}

func TestReservationParticipationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "RESERVATION_PARTICIPATION_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0005_create_notifications.sql")
	passwords := seedAuthTestUsers(t, ctx, pool)
	seedParticipation(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0006_enable_notification_creation.sql")
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	base := "http://" + addr
	login := func(role, email string) *http.Cookie {
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email, "password": passwords[role]}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}
	cookies := map[int]*http.Cookie{
		1: login("admin", "admin@example.test"),
		2: login("member", "member@example.test"),
		3: login("member", "other@example.test"),
	}
	const path = "/api/Reservations"
	authTestRequest(t, ctx, base, http.MethodPost, path+"/61/join", nil, nil, http.StatusUnauthorized)
	authTestRequest(t, ctx, base, http.MethodPost, path+"/invalid/leave", nil, nil, http.StatusNotFound)
	untrusted, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path+"/61/join", nil)
	if err != nil {
		t.Fatal(err)
	}
	untrusted.AddCookie(cookies[3])
	untrusted.Header.Set("Origin", "https://untrusted.example.test")
	untrusted.Header.Set("Sec-Fetch-Site", "cross-site")
	if status, err := reservationTestStatus(untrusted); err != nil || status != http.StatusForbidden {
		t.Fatalf("cross-origin join = %d, %v; want 403", status, err)
	}

	var reference []struct {
		Name           string
		ActorID        int
		ID             int
		Join           bool
		MarkNoticeRead bool
		Status         int
		Error          string
		Reservations   []participationReservation
		Notices        []notifications.Entry
	}
	data, err := os.ReadFile("../../internal/httpapi/testdata/reservation_participation_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	for _, expected := range reference {
		t.Run(expected.Name, func(t *testing.T) {
			if expected.MarkNoticeRead {
				authTestRequest(t, ctx, base, http.MethodPut, "/api/Notifications/6/read", nil, cookies[2], http.StatusNoContent)
			}
			action := "/leave"
			if expected.Join {
				action = "/join"
			}
			_, body := authTestRequest(t, ctx, base, http.MethodPost, path+"/"+strconv.Itoa(expected.ID)+action,
				nil, cookies[expected.ActorID], expected.Status)
			if (expected.Status == http.StatusNoContent || expected.Error != "") && string(body) != expected.Error {
				t.Fatalf("response = %q, want %q", body, expected.Error)
			}
			if got := participationReservations(t, ctx, base, cookies[2]); !reflect.DeepEqual(got, expected.Reservations) {
				t.Fatalf("reservations = %+v, want %+v", got, expected.Reservations)
			}
			_, body = authTestRequest(t, ctx, base, http.MethodGet, "/api/Notifications/user/2", nil, cookies[2], http.StatusOK)
			var notices []notifications.Entry
			if err := json.Unmarshal(body, &notices); err != nil {
				t.Fatal(err)
			}
			slices.SortFunc(notices, func(a, b notifications.Entry) int { return a.ID - b.ID })
			for i := range notices {
				notices[i].CreatedAt = ""
			}
			if !reflect.DeepEqual(notices, expected.Notices) {
				t.Fatalf("notices = %+v, want %+v", notices, expected.Notices)
			}
		})
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE notifications ADD CHECK (target_reservation_id <> 63)`); err != nil {
		t.Fatal(err)
	}
	authTestRequest(t, ctx, base, http.MethodPost, path+"/63/join", nil, cookies[3], http.StatusInternalServerError)
	last := reference[len(reference)-1]
	if got := participationReservations(t, ctx, base, cookies[2]); !reflect.DeepEqual(got, last.Reservations) {
		t.Fatalf("reservation changed after failed notice write: %+v", got)
	}
	assertConcurrentJoins(t, ctx, base+path+"/62/join", cookies[1], cookies[3])
	entries := participationReservations(t, ctx, base, cookies[2])
	for _, entry := range entries {
		if entry.ID == 62 {
			slices.Sort(entry.ParticipantIDs)
			if !slices.Equal(entry.ParticipantIDs, []int{1, 2, 3}) {
				t.Fatalf("concurrent joins stored participants %v, want [1 2 3]", entry.ParticipantIDs)
			}
		}
	}
}

func participationReservations(t *testing.T, ctx context.Context, base string, actor *http.Cookie) []participationReservation {
	t.Helper()
	_, body := authTestRequest(t, ctx, base, http.MethodGet, "/api/Reservations", nil, actor, http.StatusOK)
	var entries []participationReservation
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(entries, func(a, b participationReservation) int { return a.ID - b.ID })
	return entries
}

func assertConcurrentJoins(t *testing.T, ctx context.Context, url string, cookies ...*http.Cookie) {
	t.Helper()
	type outcome struct {
		status int
		err    error
	}
	results := make(chan outcome, len(cookies))
	for _, cookie := range cookies {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.AddCookie(cookie)
		go func() {
			status, err := reservationTestStatus(request)
			results <- outcome{status, err}
		}()
	}
	for range cookies {
		if result := <-results; result.status != http.StatusNoContent || result.err != nil {
			t.Fatalf("concurrent join = %+v, want 204", result)
		}
	}
}

func seedParticipation(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		UPDATE users SET name = 'Synthetic Owner' WHERE id = 2;
		INSERT INTO users (id, name, email, password_hash)
		SELECT 3, 'Synthetic Participant', 'other@example.test', password_hash FROM users WHERE id = 2;
		INSERT INTO reservations
		    (id, user_id, name, room, number_of_people, type, purpose, series_id,
		     date_ticks, start_time_ticks, end_time_ticks, participant_ids, participants, repeat_type, repeat_until_ticks)
		SELECT id, 2, 'Synthetic Owner', room, people, '社内', purpose, series_id,
		       day + day_offset * 864000000000, day + time_offset * 864000000000 + 360000000000,
		       day + time_offset * 864000000000 + 396000000000, members, participants, repeat_type,
		       CASE WHEN series_id IS NOT NULL THEN day + 14 * 864000000000 END
		FROM (VALUES
		     (61, 'Room A', 4, '定例会議', ' shared-series ', 0, -1, ARRAY[0,4,4,-1], '既存の表示名', '毎週'),
		     (62, 'Room B', 1, '', 'shared-series', 7, 7, ARRAY[2], '', '毎週'),
		     (63, 'Room C', 1, '', NULL, 0, 0, ARRAY[]::int[], '', 'しない')
		) AS r(id, room, people, purpose, series_id, day_offset, time_offset, members, participants, repeat_type)
		CROSS JOIN (SELECT (DATE '2026-07-10' - DATE '0001-01-01')::bigint * 864000000000 AS day) ticks;
		INSERT INTO notifications
		    (id, user_id, message, created_at_ticks, target_date_ticks, type, target_reservation_id, is_read)
		SELECT notice.id, 2, 'Synthetic Participantさんが予約「定例会議」の既存通知',
		       r.date_ticks + notice.id::bigint * 600000000, r.date_ticks, notice.type, 61, true
		FROM reservations r CROSS JOIN (VALUES (5, 'Info'), (6, 'Info'), (7, 'Warning')) notice(id, type) WHERE r.id = 61`); err != nil {
		t.Fatal(err)
	}
}
