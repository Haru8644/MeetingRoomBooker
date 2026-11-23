package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/jackc/pgx/v5/pgxpool"
)

type chatworkCreateCase struct {
	Name, Path                string
	Status                    int
	Request                   json.RawMessage
	CreatedCount, NoticeCount int
	ForceNoticeFailure        bool
	SeedReservations          []seriesEditRow
	Appointments              []seriesTestAppointment
	FakeResponses             map[string]any
	Requests                  []chatworkTestRequest
	Logs                      []chatworkTestLog
}

func TestChatworkReservationCreationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "CHATWORK_CREATION_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	password := seedAuthTestUsers(t, ctx, pool)["member"]
	var reference struct {
		Users []chatworkTestUser
		Cases []chatworkCreateCase
	}
	loadSeriesDeleteFixture(t, "chatwork_reservation_create_csharp.json", &reference)
	seedChatworkTestProfiles(t, ctx, pool, reference.Users)
	login := func(t *testing.T, base string) *http.Cookie {
		t.Helper()
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": "member@example.test", "password": password}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}
	fake := &chatworkHTTPFake{t: t, pool: pool}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()
	t.Setenv("CHATWORK_API_BASE_URL", server.URL+"/v2/")
	t.Setenv("CHATWORK_API_TOKEN", fakeChatworkToken)
	t.Run("disabled_by_default", func(t *testing.T) {
		t.Setenv("CHATWORK_ENABLED", "")
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		base := "http://" + addr
		single := reference.Cases[0]
		authTestRequest(t, ctx, base, http.MethodPost, single.Path, single.Request, login(t, base), http.StatusCreated)
		assertSeriesCounts(t, ctx, pool, single.CreatedCount, single.NoticeCount)
		if len(fake.recorded()) != 0 || len(readChatworkLogs(t, ctx, pool)) != 0 {
			t.Fatal("disabled delivery produced an HTTP request or log")
		}
	})

	t.Setenv("CHATWORK_ENABLED", "true")
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	base := "http://" + addr
	cookie := login(t, base)
	for _, scenario := range reference.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			seedSeriesEditRows(t, ctx, pool, scenario.SeedReservations)
			seedSeriesTestAppointments(t, ctx, pool, scenario.Appointments)
			if _, err := pool.Exec(ctx, "TRUNCATE chatwork_delivery_logs RESTART IDENTITY"); err != nil {
				t.Fatal(err)
			}
			wantRows := len(scenario.SeedReservations) + scenario.CreatedCount
			if scenario.ForceNoticeFailure {
				failChatworkNoticeCommit(t, ctx, pool)
				// C# commits the reservations separately. Go rolls back both writes.
				wantRows = len(scenario.SeedReservations)
			}
			fake.expect(scenario.FakeResponses, func(ctx context.Context) error {
				var rows, notices int
				err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM reservations), (SELECT count(*) FROM notifications)`).Scan(&rows, &notices)
				if err != nil {
					return err
				}
				if rows != wantRows || notices != scenario.NoticeCount {
					return fmt.Errorf("found %d reservations and %d notices, want %d and %d", rows, notices, wantRows, scenario.NoticeCount)
				}
				return nil
			})

			started := time.Now()
			_, body := authTestRequest(t, ctx, base, http.MethodPost, scenario.Path, scenario.Request, cookie, scenario.Status)
			assertSeriesCounts(t, ctx, pool, wantRows, scenario.NoticeCount)
			if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
				t.Fatalf("Chatwork HTTP calls = %+v, want %+v", got, scenario.Requests)
			}
			wantLogs := slices.Clone(scenario.Logs)
			if scenario.Status == http.StatusCreated {
				representative := creationResponseRepresentative(t, body, scenario.CreatedCount)
				for i, row := range wantLogs {
					captureKey := fmt.Sprintf("ReservationCreated:reservation:%d:user:%d", row.ReservationID, row.TargetUserID)
					if captureKey != row.DeliveryKey {
						t.Fatalf("unexpected C# delivery key %s", row.DeliveryKey)
					}
					wantLogs[i].ReservationID = representative
					wantLogs[i].DeliveryKey = fmt.Sprintf("ReservationCreated:reservation:%d:user:%d", representative, row.TargetUserID)
				}
			}
			logs := readChatworkLogs(t, ctx, pool)
			compareChatworkLogs(t, scenario.Name, wantLogs, logs, started, false)
			if scenario.Name == "provider_failure_and_timeout" {
				entries, err := reservations.List(ctx, pool)
				if err != nil {
					t.Fatal(err)
				}
				sender, err := chatwork.NewSender(pool, fakeChatworkToken, server.URL+"/v2/", true)
				if err != nil {
					t.Fatal(err)
				}
				fake.expect(nil, nil)
				sender.SendCreation(ctx, reservations.Creation{Entries: entries})
				if len(fake.recorded()) != 0 || !reflect.DeepEqual(readChatworkLogs(t, ctx, pool), logs) {
					t.Fatal("failed, skipped, or uncertain creation delivery was retried")
				}
			}
		})
	}
}

func creationResponseRepresentative(t *testing.T, body []byte, wantCount int) int {
	t.Helper()
	var entries []seriesEditRow
	if strings.HasPrefix(string(body), "[") {
		if err := json.Unmarshal(body, &entries); err != nil {
			t.Fatal(err)
		}
	} else {
		var entry seriesEditRow
		if err := json.Unmarshal(body, &entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if len(entries) != wantCount || len(entries) == 0 {
		t.Fatalf("got %d created reservations, want %d", len(entries), wantCount)
	}
	first := entries[0]
	for _, entry := range entries[1:] {
		if entry.Date < first.Date || (entry.Date == first.Date && entry.ID < first.ID) {
			first = entry
		}
	}
	if first.ID <= 0 {
		t.Fatalf("invalid representative ID %d", first.ID)
	}
	return first.ID
}

func failChatworkNoticeCommit(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_chatwork_notice_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'synthetic notice commit failure';
		END;
		$$;
		CREATE CONSTRAINT TRIGGER chatwork_notice_commit_failure AFTER INSERT ON notifications
		    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_chatwork_notice_commit();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `
			DROP TRIGGER chatwork_notice_commit_failure ON notifications;
			DROP FUNCTION reject_chatwork_notice_commit();`); err != nil {
			t.Error(err)
		}
	})
}
