package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
	"github.com/jackc/pgx/v5/pgxpool"
)

type notificationCreateCase struct {
	Name               string
	Actor              string
	Status             int
	ContentType        string
	Request            json.RawMessage
	RequestText        string
	RequestContentType string
	Body               json.RawMessage
	Location           string
	Stored             json.RawMessage
	DefaultDates       []string
}

func TestAdminNotificationCreatePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "NOTIFICATION_CREATE_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0005_create_notifications.sql")
	seedCapturedNotifications(t, ctx, pool, json.RawMessage(`[{
		"id":4100,"userId":2,"message":"既存の架空通知","createdAt":"2026-06-01T08:00:00",
		"targetDate":"2026-06-01T00:00:00","isRead":true,"type":"Info"
	}]`))
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0006_enable_notification_creation.sql")
	passwords := seedAuthTestUsers(t, ctx, pool)

	const trusted = "http://127.0.0.1:5169"
	addr, stop := startAuthTestAPI(t, ctx, dsn, trusted)
	defer stop()
	base := "http://" + addr
	cookies := make(map[string]*http.Cookie)
	for _, role := range []string{"admin", "member"} {
		login, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": role + "@example.test", "password": passwords[role]}, nil, http.StatusOK)
		cookies[role] = authTestCookie(t, login, false)
	}

	data, err := os.ReadFile("../../internal/httpapi/testdata/notification_create_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct{ Cases []notificationCreateCase }
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}

	t.Run("untrusted_admin_origin", func(t *testing.T) {
		response, _ := postNotificationTestRequest(t, ctx, base, "https://untrusted.example.test", "application/json",
			[]byte(`{"userId":2,"message":"untrusted origin"}`), cookies["admin"], http.StatusForbidden)
		if response.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin received an allowed-origin header")
		}
	})
	for _, test := range capture.Cases {
		if !t.Run(test.Name, func(t *testing.T) {
			contentType, payload := "application/json", []byte(test.Request)
			if test.Request == nil {
				contentType, payload = test.RequestContentType, []byte(test.RequestText)
			}
			before := time.Now()
			response, body := postNotificationTestRequest(t, ctx, base, trusted, contentType, payload, cookies[test.Actor], test.Status)
			if response.Header.Get("Content-Type") != test.ContentType {
				t.Fatalf("Content-Type = %q, want %q", response.Header.Get("Content-Type"), test.ContentType)
			}
			if test.Status != http.StatusCreated {
				compareNotificationCreateError(t, test, body)
				return
			}
			if response.Header.Get("Access-Control-Allow-Origin") != trusted ||
				response.Header.Get("Access-Control-Allow-Credentials") != "true" {
				t.Fatal("trusted origin did not receive credentialed CORS headers")
			}
			if response.Header.Get("Location") != base+test.Location {
				t.Fatalf("Location = %q, want %q", response.Header.Get("Location"), base+test.Location)
			}
			compareCreatedNotification(t, ctx, pool, test, body, before)
		}) {
			return
		}
	}

	t.Run("recipient_lifecycle", func(t *testing.T) {
		postNotificationTestRequest(t, ctx, base, trusted, "application/json",
			[]byte(`{"id":4100,"userId":2,"message":"duplicate explicit ID"}`), cookies["admin"], http.StatusInternalServerError)
		member := cookies["member"]
		getInbox := func() map[int]notifications.Entry {
			t.Helper()
			_, body := authTestRequest(t, ctx, base, http.MethodGet, "/api/Notifications/user/2", nil, member, http.StatusOK)
			var list []notifications.Entry
			if err := json.Unmarshal(body, &list); err != nil {
				t.Fatal(err)
			}
			entries := make(map[int]notifications.Entry)
			for _, entry := range list {
				entries[entry.ID] = entry
			}
			return entries
		}
		inbox := getInbox()
		newEntry := inbox[4101]
		if newEntry.Message != "架空の管理者通知" || newEntry.IsRead || inbox[4100].Message != "既存の架空通知" {
			t.Fatalf("unexpected legacy or new notification: %+v, %+v", inbox[4100], newEntry)
		}
		authTestRequest(t, ctx, base, http.MethodPut, "/api/Notifications/4101/read", nil, member, http.StatusNoContent)
		if !getInbox()[4101].IsRead {
			t.Fatal("recipient did not see the saved read flag")
		}
		authTestRequest(t, ctx, base, http.MethodDelete, "/api/Notifications/4101", nil, member, http.StatusNoContent)
		if _, found := getInbox()[4101]; found {
			t.Fatal("recipient still saw the deleted notification")
		}
	})

	t.Run("sequence_does_not_reuse_deleted_ids", func(t *testing.T) {
		authTestRequest(t, ctx, base, http.MethodDelete, "/api/Notifications/4801", nil, cookies["member"], http.StatusNoContent)
		postNotificationTestRequest(t, ctx, base, trusted, "application/json", []byte(`{"id":4099,"userId":2}`), cookies["admin"], http.StatusCreated)
		_, body := postNotificationTestRequest(t, ctx, base, trusted, "application/json", []byte(`{"userId":2}`), cookies["admin"], http.StatusCreated)
		if id := notificationJSONObject(t, body)["id"]; id != float64(4802) {
			t.Fatalf("next generated ID after deletion and a lower explicit insert = %v, want 4802", id)
		}
	})
}

func postNotificationTestRequest(t *testing.T, ctx context.Context, base, origin, contentType string, payload []byte,
	cookie *http.Cookie, wantStatus int) (*http.Response, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/Notifications", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Origin", origin)
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("POST %s: got %d, want %d, body %s", request.URL.Path, response.StatusCode, wantStatus, body)
	}
	return response, body
}

func compareNotificationCreateError(t *testing.T, test notificationCreateCase, body []byte) {
	t.Helper()
	if test.Status == http.StatusUnauthorized || test.Status == http.StatusForbidden {
		if len(body) != 0 {
			t.Fatalf("expected empty authorization response, got %s", body)
		}
		return
	}
	want, got := notificationJSONObject(t, test.Body), notificationJSONObject(t, body)
	switch test.Name {
	case "invalid_value", "invalid_target", "malformed_json", "null_date":
		// ASP.NET includes its JSON parser's field, line, and byte-position messages.
		want["errors"] = map[string]any{"notification": []any{"A valid notification JSON object is required."}}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response %s, want %v", body, want)
	}
}

func compareCreatedNotification(t *testing.T, ctx context.Context, pool *pgxpool.Pool, test notificationCreateCase, body []byte, before time.Time) {
	t.Helper()
	got, want := notificationJSONObject(t, body), notificationJSONObject(t, test.Body)
	wantStored := notificationJSONObject(t, test.Stored)
	if test.Name == "date_zones" {
		// The capture ran in Asia/Tokyo. Convert its local DateTime to this runner's zone.
		localDate, err := time.Parse(time.RFC3339Nano, want["targetDate"].(string))
		if err != nil {
			t.Fatal(err)
		}
		want["targetDate"] = localDate.Local().Format(dotnettime.Layout + "-07:00")
		wantStored["targetDate"] = localDate.Local().Format(dotnettime.Layout)
	}
	for _, field := range test.DefaultDates {
		value, ok := got[field].(string)
		date, err := time.Parse(time.RFC3339Nano, value)
		if !ok || err != nil || date.Before(before.Truncate(100*time.Nanosecond)) || date.After(time.Now()) ||
			value != date.Local().Format(dotnettime.Layout+"-07:00") {
			t.Fatalf("%s = %q, want current server time with local offset", field, value)
		}
		want[field] = value
		wantStored[field] = date.Local().Format(dotnettime.Layout)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("created response %s, want %v", body, want)
	}
	var stored notifications.Entry
	var createdAt, targetDate int64
	err := pool.QueryRow(ctx, `SELECT id, user_id, message, created_at_ticks, target_date_ticks, is_read, type,
		target_reservation_id, target_work_schedule_entry_id FROM notifications WHERE id = $1`, int(want["id"].(float64))).
		Scan(&stored.ID, &stored.UserID, &stored.Message, &createdAt, &targetDate, &stored.IsRead, &stored.Type,
			&stored.TargetReservationID, &stored.TargetWorkScheduleEntryID)
	if err != nil {
		t.Fatal(err)
	}
	stored.CreatedAt = dotnettime.FromTicks(createdAt).Format(dotnettime.Layout)
	stored.TargetDate = dotnettime.FromTicks(targetDate).Format(dotnettime.Layout)
	storedJSON, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(notificationJSONObject(t, storedJSON), wantStored) {
		t.Fatalf("stored row %s, want %v", storedJSON, wantStored)
	}
}

func notificationJSONObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	return object
}
