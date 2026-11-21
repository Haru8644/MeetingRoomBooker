package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
	"github.com/jackc/pgx/v5/pgxpool"
)

type notificationCase struct {
	Name        string
	Actor       string
	Method      string
	Path        string
	Status      int
	ContentType string
	Body        json.RawMessage
}

func TestNotificationInboxPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "NOTIFICATION_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	passwords := seedAuthTestUsers(t, ctx, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, name, email, password_hash)
		SELECT 3, 'Synthetic Other', 'other@example.test', password_hash FROM users WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	passwords["other"] = passwords["member"]
	const trustedOrigin = "http://127.0.0.1:5169"
	addr, stop := startAuthTestAPI(t, ctx, dsn, trustedOrigin)
	defer stop()
	base := "http://" + addr

	data, err := os.ReadFile("../../internal/httpapi/testdata/notification_inbox_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Cases []notificationCase }
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, test := range fixture.Cases {
		if test.Actor == "anonymous" {
			t.Run(test.Name, func(t *testing.T) { compareNotificationCase(t, ctx, base, test, nil) })
		}
	}

	applyAuthTestSQL(t, ctx, pool, "../../migrations/0005_create_notifications.sql")
	for _, test := range fixture.Cases {
		if test.Name == "owner_inbox" || test.Name == "other_own_inbox" {
			seedCapturedNotifications(t, ctx, pool, test.Body)
		}
	}
	cookies := make(map[string]*http.Cookie)
	for _, account := range []string{"member", "admin", "other"} {
		login, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": account + "@example.test", "password": passwords[account]}, nil, http.StatusOK)
		cookies[account] = authTestCookie(t, login, false)
	}
	t.Run("CORS", func(t *testing.T) { verifyNotificationOrigins(t, ctx, base, trustedOrigin, cookies["member"]) })
	for _, test := range fixture.Cases {
		if test.Actor != "anonymous" {
			t.Run(test.Name, func(t *testing.T) { compareNotificationCase(t, ctx, base, test, cookies[test.Actor]) })
		}
	}
}

func compareNotificationCase(t *testing.T, ctx context.Context, base string, test notificationCase, cookie *http.Cookie) {
	t.Helper()
	response, body := authTestRequest(t, ctx, base, test.Method, test.Path, nil, cookie, test.Status)
	if content := response.Header.Get("Content-Type"); content != test.ContentType {
		t.Fatalf("Content-Type = %q, want %q", content, test.ContentType)
	}
	var expected any
	if err := json.Unmarshal(test.Body, &expected); err != nil {
		t.Fatal(err)
	}
	var actual any = string(body)
	if strings.Contains(test.ContentType, "json") {
		if err := json.Unmarshal(body, &actual); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("response %s, want %v", body, expected)
	}
}

func seedCapturedNotifications(t *testing.T, ctx context.Context, pool *pgxpool.Pool, body json.RawMessage) {
	t.Helper()
	var entries []notifications.Entry
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatal(err)
	}
	// Insert older entries first to exercise the newest-first order independently of insertion order.
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		createdAt, err := time.Parse(dotnettime.Layout, entry.CreatedAt)
		if err != nil {
			t.Fatal(err)
		}
		targetDate, err := time.Parse(dotnettime.Layout, entry.TargetDate)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO notifications (id, user_id, message, created_at_ticks, target_date_ticks, is_read,
			type, target_reservation_id, target_work_schedule_entry_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			entry.ID, entry.UserID, entry.Message, dotnettime.Ticks(createdAt), dotnettime.Ticks(targetDate), entry.IsRead,
			entry.Type, entry.TargetReservationID, entry.TargetWorkScheduleEntryID)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func verifyNotificationOrigins(t *testing.T, ctx context.Context, base, trusted string, cookie *http.Cookie) {
	t.Helper()
	const untrusted = "https://untrusted.example.test"
	for _, change := range []struct{ method, path string }{
		{http.MethodPut, "/api/Notifications/101/read"}, {http.MethodDelete, "/api/Notifications/101"},
	} {
		for _, origin := range []string{trusted, untrusted} {
			request, err := http.NewRequestWithContext(ctx, http.MethodOptions, base+change.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Origin", origin)
			request.Header.Set("Access-Control-Request-Method", change.method)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			allowed := response.Header.Get("Access-Control-Allow-Origin")
			if origin == trusted {
				if response.StatusCode != http.StatusNoContent || allowed != trusted ||
					response.Header.Get("Access-Control-Allow-Credentials") != "true" ||
					!strings.Contains(response.Header.Get("Access-Control-Allow-Methods"), change.method) {
					t.Fatalf("trusted %s preflight: %d, %v", change.method, response.StatusCode, response.Header)
				}
			} else if allowed != "" {
				t.Fatalf("untrusted %s preflight allowed origin %q", change.method, allowed)
			}
		}
		request, err := http.NewRequestWithContext(ctx, change.method, base+change.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", untrusted)
		request.Header.Set("Sec-Fetch-Site", "cross-site")
		request.AddCookie(cookie)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden || response.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("authenticated untrusted %s request: %d, %v", change.method, response.StatusCode, response.Header)
		}
	}
}
