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
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSingleReservationUpdatePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "RESERVATION_UPDATE_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0003_enable_reservation_creation.sql")
	applyReservationNotificationMigrations(t, ctx, pool)
	passwords := seedAuthTestUsers(t, ctx, pool)
	seedReservationUpdate(t, ctx, pool)
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	base := "http://" + addr
	login := func(actor, email string) *http.Cookie {
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email, "password": passwords[actor]}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}
	cookies := map[string]*http.Cookie{
		"owner": login("member", "member@example.test"),
		"admin": login("admin", "admin@example.test"),
		"other": login("member", "participant@example.test"),
	}
	var reference map[string]struct {
		Actor         string
		Notify        bool
		Request       json.RawMessage
		Stored        map[string]any
		Notifications []map[string]any
	}
	fixture, err := os.ReadFile("../../internal/httpapi/testdata/reservation_update_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture, &reference); err != nil {
		t.Fatal(err)
	}
	const path = "/api/Reservations/61"
	initial := reference["changed"].Request
	authTestRequest(t, ctx, base, http.MethodPut, path, initial, nil, http.StatusUnauthorized)
	authTestRequest(t, ctx, base, http.MethodPut, path, initial, cookies["other"], http.StatusForbidden)
	authTestRequest(t, ctx, base, http.MethodPut, "/api/Reservations/62", initial, cookies["owner"], http.StatusBadRequest)
	authTestRequest(t, ctx, base, http.MethodPut, "/api/Reservations/999",
		json.RawMessage(`{"id":999,"name":"架空","room":"大会議室"}`), cookies["owner"], http.StatusNotFound)
	authTestRequest(t, ctx, base, http.MethodPut, path+"?notifyParticipants=maybe", initial, cookies["owner"], http.StatusBadRequest)
	authTestRequest(t, ctx, base, http.MethodPut, path,
		json.RawMessage(`{"id":61,"name":"架空","room":"大会議室","startTime":"2026-07-10T11:00:00","endTime":"2026-07-10T10:00:00"}`),
		cookies["owner"], http.StatusBadRequest)

	var lastNoticeID, noticeCount int
	for _, name := range []string{"changed", "warning_only", "silent", "no_changes"} {
		scenario := reference[name]
		target := path
		if !scenario.Notify {
			target += "?notifyParticipants=false"
		}
		_, body := authTestRequest(t, ctx, base, http.MethodPut, target, scenario.Request, cookies[scenario.Actor], http.StatusNoContent)
		if len(body) != 0 {
			t.Fatalf("%s: successful update returned a body: %s", name, body)
		}
		assertUpdatedReservation(t, ctx, base, cookies["owner"], scenario.Stored)
		var data []byte
		if err := pool.QueryRow(ctx, `
			SELECT COALESCE(jsonb_agg(jsonb_build_object(
			    'userId', user_id, 'message', message, 'targetDate', target_date_ticks::text,
			    'isRead', is_read, 'type', type, 'targetReservationId', target_reservation_id,
			    'targetWorkScheduleEntryId', target_work_schedule_entry_id
			) ORDER BY user_id), '[]'::jsonb), COALESCE(max(id), $1)
			FROM notifications WHERE id > $1`, lastNoticeID).Scan(&data, &lastNoticeID); err != nil {
			t.Fatal(err)
		}
		var got []map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		for _, row := range got {
			ticks, err := strconv.ParseInt(row["targetDate"].(string), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			row["targetDate"] = dotnettime.FromTicks(ticks).Format(dotnettime.Layout)
		}
		if !reflect.DeepEqual(got, scenario.Notifications) {
			t.Fatalf("%s: notifications = %#v, want %#v", name, got, scenario.Notifications)
		}
		noticeCount += len(got)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE notifications ADD CHECK (user_id <> 7)`); err != nil {
		t.Fatal(err)
	}
	authTestRequest(t, ctx, base, http.MethodPut, path, json.RawMessage(
		`{"id":61,"userId":2,"name":"入力名","room":"別室","date":"2026-07-13","startTime":"2026-07-13T11:00:00","endTime":"2026-07-13T12:00:00","participantIds":[2,3,7]}`),
		cookies["owner"], http.StatusInternalServerError)
	assertUpdatedReservation(t, ctx, base, cookies["owner"], reference["no_changes"].Stored)
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&remaining); err != nil || remaining != noticeCount {
		t.Fatalf("unexpected notice count after rollback: %d, wanted %d, error %v", remaining, noticeCount, err)
	}
}

func assertUpdatedReservation(t *testing.T, ctx context.Context, base string, cookie *http.Cookie, want map[string]any) {
	t.Helper()
	_, body := authTestRequest(t, ctx, base, http.MethodGet, "/api/Reservations", nil, cookie, http.StatusOK)
	var reservations []map[string]any
	if err := json.Unmarshal(body, &reservations); err != nil {
		t.Fatal(err)
	}
	for _, row := range reservations {
		if row["id"] == want["id"] {
			for key, value := range want {
				if !reflect.DeepEqual(row[key], value) {
					t.Fatalf("stored reservation %s = %#v, want %#v", key, row[key], value)
				}
			}
			return
		}
	}
	t.Fatalf("reservation %v is missing", want["id"])
}

func seedReservationUpdate(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		UPDATE users SET name = '架空の予約者' WHERE id = 2;
		INSERT INTO users (id, name, email, password_hash)
		SELECT added.id, added.name, added.email, u.password_hash
		FROM users u CROSS JOIN (VALUES
		    (3, '架空の参加者', 'participant@example.test'),
		    (4, '架空の削除メンバー', 'removed@example.test'),
		    (5, '架空の追加メンバー', 'added@example.test')
		) added(id, name, email) WHERE u.id = 2;
		INSERT INTO reservations
		    (id, user_id, name, room, purpose, series_id, type, number_of_people, date_ticks,
		     start_time_ticks, end_time_ticks, participant_ids, participants, repeat_type)
		SELECT id, 2, '架空の予約者', room, purpose, series, '社内', 1, day,
		       day + start_hour::bigint * 36000000000, day + end_hour::bigint * 36000000000,
		       ids, '', 'しない'
		FROM (VALUES
		    (61, '大会議室', '変更前', 'legacy-keep', DATE '2026-07-10', 10, 11, ARRAY[2,3,4]),
		    (62, '小会議室', '', NULL, DATE '2026-07-11', 11, 12, ARRAY[2])
		) r(id, room, purpose, series, date, start_hour, end_hour, ids)
		CROSS JOIN LATERAL (SELECT (r.date - DATE '0001-01-01')::bigint * 864000000000 AS day) ticks;
		INSERT INTO work_schedule_entries
		    (id, created_by_user_id, type, title, date_ticks, start_time_ticks, end_time_ticks,
		     leave_period, participant_ids, participants, created_at_ticks, updated_at_ticks)
		SELECT 1, 2, 0, '予約者の訪問', day, day + 414000000000, day + 450000000000,
		       0, ARRAY[2], '', day, day
		FROM (SELECT (DATE '2026-07-11' - DATE '0001-01-01')::bigint * 864000000000 AS day) ticks`)
	if err != nil {
		t.Fatal(err)
	}
}
