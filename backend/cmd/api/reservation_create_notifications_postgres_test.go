package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
	"github.com/jackc/pgx/v5/pgxpool"
)

func applyReservationNotificationMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0004_create_work_schedule_entries.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0005_create_notifications.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0006_enable_notification_creation.sql")
}

func TestReservationCreateNotificationsPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "RESERVATION_CREATE_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0003_enable_reservation_creation.sql")
	applyReservationNotificationMigrations(t, ctx, pool)
	passwords := seedAuthTestUsers(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		UPDATE users SET name = '架空の予約者' WHERE id = 2;
		INSERT INTO users (id, name, email, password_hash)
		SELECT added.id, added.name, added.email, u.password_hash
		FROM users u CROSS JOIN (VALUES
		    (3, '架空の参加者', 'participant@example.test'),
		    (4, '架空の別参加者', 'other@example.test')
		) added(id, name, email) WHERE u.id = 2`); err != nil {
		t.Fatal(err)
	}
	seedReservationNotificationAppointments(t, ctx, pool)
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	base := "http://" + addr
	login := func(email string) *http.Cookie {
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email, "password": passwords["member"]}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}
	organizer := login("member@example.test")
	participants := map[int]*http.Cookie{3: login("participant@example.test"), 4: login("other@example.test")}
	const reservationsPath = "/api/Reservations"

	var reference map[string]struct {
		Request       json.RawMessage
		Notifications []notifications.Entry
	}
	data, err := os.ReadFile("../../internal/httpapi/testdata/reservation_create_notifications_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	var noticeCount int
	for _, name := range []string{"info", "warning", "organizer_only"} {
		scenario := reference[name]
		var refreshedID int
		if name == "warning" {
			// Existing administrator-created notices can already target the next reservation ID.
			if err := pool.QueryRow(ctx, `
				WITH inserted AS (
				    INSERT INTO notifications (user_id, target_reservation_id, type, message,
				                               created_at_ticks, target_date_ticks, is_read)
				    VALUES (3, 2, 'Warning', $1, 10, 0, true),
				           (3, 2, 'Warning', $1, 20, 0, true)
				    RETURNING id, created_at_ticks
				)
				SELECT id FROM inserted ORDER BY created_at_ticks DESC LIMIT 1`,
				scenario.Notifications[0].Message).Scan(&refreshedID); err != nil {
				t.Fatal(err)
			}
		}
		started := time.Now()
		_, body := authTestRequest(t, ctx, base, http.MethodPost, reservationsPath, scenario.Request, organizer, http.StatusCreated)
		var reservation struct{ ID int }
		if err := json.Unmarshal(body, &reservation); err != nil || reservation.ID <= 0 {
			t.Fatalf("%s: missing generated reservation ID in %s", name, body)
		}
		for _, want := range scenario.Notifications {
			path := "/api/Notifications/user/" + strconv.Itoa(want.UserID)
			_, body = authTestRequest(t, ctx, base, http.MethodGet, path, nil, participants[want.UserID], http.StatusOK)
			var inbox []notifications.Entry
			if err := json.Unmarshal(body, &inbox); err != nil || len(inbox) == 0 {
				t.Fatalf("%s: missing participant notification in %s", name, body)
			}
			got := inbox[0]
			if got.ID <= 0 || got.TargetReservationID == nil || *got.TargetReservationID != reservation.ID {
				t.Fatalf("%s: invalid notification or target ID in %+v", name, got)
			}
			if refreshedID != 0 && want.UserID == 3 && got.ID != refreshedID {
				t.Fatalf("newest matching notice should retain ID %d, got %d", refreshedID, got.ID)
			}
			createdAt, err := time.Parse(dotnettime.Layout, got.CreatedAt)
			if err != nil || createdAt.Before(dotnettime.OnDate(started, started).Truncate(100*time.Nanosecond)) {
				t.Fatalf("%s: invalid creation time %q", name, got.CreatedAt)
			}
			got.ID, got.CreatedAt, got.TargetReservationID = 0, "", nil
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: notification = %+v, want %+v", name, got, want)
			}
		}
		noticeCount += len(scenario.Notifications)
		assertReservationNotificationCounts(t, ctx, pool, reservation.ID, len(scenario.Notifications))
	}
	_, body := authTestRequest(t, ctx, base, http.MethodGet, "/api/Notifications/user/2", nil, organizer, http.StatusOK)
	if string(body) != "[]\n" && string(body) != "[]" {
		t.Fatalf("organizer inbox = %s, want []", body)
	}
	authTestRequest(t, ctx, base, http.MethodPost, reservationsPath, reference["info"].Request, organizer, http.StatusConflict)

	if _, err := pool.Exec(ctx, `ALTER TABLE notifications ADD CHECK (user_id <> 99)`); err != nil {
		t.Fatal(err)
	}
	const reject = `{"name":"入力名","room":"大会議室","date":"2026-07-12","startTime":"2026-07-12T10:00:00","endTime":"2026-07-12T11:00:00","participantIds":[3,99]}`
	authTestRequest(t, ctx, base, http.MethodPost, reservationsPath, json.RawMessage(reject), organizer, http.StatusInternalServerError)
	var reservations, notices int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM reservations), (SELECT count(*) FROM notifications)`).Scan(&reservations, &notices); err != nil {
		t.Fatal(err)
	}
	if reservations != len(reference) || notices != noticeCount {
		t.Fatalf("after overlap and notification-write rejection: %d bookings and %d notices, want %d and %d",
			reservations, notices, len(reference), noticeCount)
	}
	t.Log("matched C# Info and Warning notices, organizer exclusion and empty recipient set; rejected writes left no booking or notification rows")
}

func assertReservationNotificationCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE target_reservation_id = $1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("reservation %d has %d notification rows, want %d", id, got, want)
	}
}

func seedReservationNotificationAppointments(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO work_schedule_entries
		    (id, created_by_user_id, type, title, date_ticks, start_time_ticks, end_time_ticks,
		     leave_period, participant_ids, participants, created_at_ticks, updated_at_ticks)
		SELECT id, 2, type, title, date_ticks,
		       date_ticks + start_minute::bigint * 600000000,
		       date_ticks + end_minute::bigint * 600000000,
		       0, participant_ids, '', date_ticks, date_ticks
		FROM (VALUES
		    (1, 0, '予約者の訪問', 580, 605, ARRAY[2]),
		    (2, 0, '訪問1', 605, 625, ARRAY[3,3]),
		    (3, 0, '訪問2', 610, 630, ARRAY[3]),
		    (4, 0, '訪問3', 615, 635, ARRAY[3]),
		    (5, 0, '訪問4', 620, 640, ARRAY[3]),
		    (6, 0, '訪問5', 625, 645, ARRAY[3]),
		    (7, 0, '終了が予約開始と同じ', 540, 600, ARRAY[3]),
		    (8, 0, '開始が予約終了と同じ', 660, 690, ARRAY[3]),
		    (9, 1, '在宅', 600, 660, ARRAY[3]),
		    (10, 0, '無関係な人の訪問', 600, 660, ARRAY[1]),
		    (11, 0, '時間未定', NULL, 660, ARRAY[3])
		) e(id, type, title, start_minute, end_minute, participant_ids)
		CROSS JOIN (SELECT $1::bigint AS date_ticks) day`,
		dotnettime.Ticks(time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
}
