package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const fakeChatworkToken = "synthetic-local-token"

type chatworkTestRequest struct {
	RoomID, Body string
}

type chatworkTestLog struct {
	ReservationID, TargetUserID                int
	WorkScheduleEntryID                        *int
	DeliveryType, DeliveryKey, Message, Status string
	ScheduledStartTime, AttemptedAt, CreatedAt string
	RoomID, ErrorMessage, SentAt               *string
}

type chatworkTestUser struct {
	ID     int
	Name   string
	RoomID *string
}

func applyChatworkTestMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0003_enable_reservation_creation.sql")
	applyReservationNotificationMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0007_create_chatwork_delivery_logs.sql")
}

func seedChatworkTestProfiles(t *testing.T, ctx context.Context, pool *pgxpool.Pool, users []chatworkTestUser) {
	t.Helper()
	for _, user := range users {
		_, err := pool.Exec(ctx, `
			INSERT INTO users (id,name,email,chatwork_direct_room_id)
			VALUES ($1,$2,'chatwork-fixture-' || $1::integer || '@example.test',$3)
			ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, chatwork_direct_room_id=EXCLUDED.chatwork_direct_room_id`,
			user.ID, user.Name, user.RoomID)
		if err != nil {
			t.Fatal(err)
		}
	}
}

type chatworkCancelCase struct {
	Name, Dataset, Method, Path     string
	Status                          int
	Request                         json.RawMessage
	Remaining                       []int
	ExistingKey, ForceDeleteFailure bool
	FakeResponses                   map[string]any
	Requests                        []chatworkTestRequest
	Logs                            []chatworkTestLog
}

func TestChatworkReservationCancellationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "CHATWORK_CANCELLATION_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	password := seedAuthTestUsers(t, ctx, pool)["member"]
	var reference struct {
		Users    []chatworkTestUser
		Datasets map[string][]seriesEditRow
		Cases    []chatworkCancelCase
	}
	loadSeriesDeleteFixture(t, "chatwork_reservation_cancel_csharp.json", &reference)
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
		seedSeriesEditRows(t, ctx, pool, reference.Datasets["single"])
		authTestRequest(t, ctx, base, http.MethodDelete, "/api/Reservations/55", nil, login(t, base), http.StatusNoContent)
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
			dataset := reference.Datasets[scenario.Dataset]
			seedSeriesEditRows(t, ctx, pool, dataset)
			if _, err := pool.Exec(ctx, "TRUNCATE chatwork_delivery_logs RESTART IDENTITY"); err != nil {
				t.Fatal(err)
			}
			if scenario.ExistingKey {
				seedChatworkCancelLog(t, ctx, pool, scenario.Logs[0])
			}
			if scenario.ForceDeleteFailure {
				rejectChatworkCancelCommit(t, ctx, pool)
			}
			var deleted []int
			for _, row := range dataset {
				if !slices.Contains(scenario.Remaining, row.ID) {
					deleted = append(deleted, row.ID)
				}
			}
			fake.reset(deleted, scenario.FakeResponses)
			var payload any
			if string(scenario.Request) != "null" {
				payload = scenario.Request
			}
			started := time.Now()
			_, body := authTestRequest(t, ctx, base, scenario.Method, scenario.Path, payload, cookie, scenario.Status)
			if len(body) != 0 {
				t.Fatalf("expected an empty response, got %s", body)
			}
			assertSeriesDeleteRows(t, readSeriesEditRows(t, ctx, base, cookie), dataset, scenario.Remaining)
			if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
				t.Fatalf("Chatwork HTTP calls = %+v, want %+v", got, scenario.Requests)
			}
			logs := readChatworkLogs(t, ctx, pool)
			compareChatworkLogs(t, scenario.Name, scenario.Logs, logs, started, scenario.ExistingKey)
			if scenario.Name == "provider_failure_and_timeout" {
				// Reusing the IDs simulates replay against an imported delivery history.
				seedSeriesEditRows(t, ctx, pool, dataset)
				fake.reset(deleted, nil)
				authTestRequest(t, ctx, base, scenario.Method, scenario.Path, payload, cookie, http.StatusNoContent)
				if len(fake.recorded()) != 0 || !reflect.DeepEqual(readChatworkLogs(t, ctx, pool), logs) {
					t.Fatal("a failed, skipped, or uncertain prior attempt was retried")
				}
			}
		})
	}
	t.Run("result_write_failure_leaves_claim", func(t *testing.T) {
		dataset := reference.Datasets["single"]
		seedSeriesEditRows(t, ctx, pool, dataset)
		if _, err := pool.Exec(ctx, "TRUNCATE chatwork_delivery_logs RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
		constrainSeriesTestWrites(t, ctx, pool, "chatwork_delivery_logs", "status <> 'Succeeded'")
		fake.reset([]int{55}, nil)
		authTestRequest(t, ctx, base, http.MethodDelete, "/api/Reservations/55", nil, cookie, http.StatusNoContent)
		logs := readChatworkLogs(t, ctx, pool)
		if len(logs) != 3 || logs[0].Status != "Unknown" || logs[2].Status != "Unknown" ||
			!slices.Equal(fake.recorded(), reference.Cases[0].Requests) {
			t.Fatalf("successful HTTP sends did not retain both uncertain claims: %+v", logs)
		}
		seedSeriesEditRows(t, ctx, pool, dataset)
		fake.reset([]int{55}, nil)
		authTestRequest(t, ctx, base, http.MethodDelete, "/api/Reservations/55", nil, cookie, http.StatusNoContent)
		if len(fake.recorded()) != 0 || !reflect.DeepEqual(readChatworkLogs(t, ctx, pool), logs) {
			t.Fatal("replay retried a delivery whose final result was never recorded")
		}
	})
	t.Run("concurrent_claims", func(t *testing.T) {
		seedSeriesEditRows(t, ctx, pool, reference.Datasets["single"])
		if _, err := pool.Exec(ctx, "TRUNCATE chatwork_delivery_logs RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
		fake.reset([]int{55}, nil)
		deleted, err := reservations.Delete(ctx, pool, 55, 2, false)
		if err != nil {
			t.Fatal(err)
		}
		sender, err := chatwork.NewSender(pool, fakeChatworkToken, server.URL+"/v2/", true)
		if err != nil {
			t.Fatal(err)
		}
		var workers sync.WaitGroup
		for range 2 {
			workers.Go(func() { sender.SendCancellation(ctx, deleted) })
		}
		workers.Wait()
		got, want := fake.recorded(), slices.Clone(reference.Cases[0].Requests)
		order := func(a, b chatworkTestRequest) int { return cmp.Compare(a.RoomID, b.RoomID) }
		slices.SortFunc(got, order)
		slices.SortFunc(want, order)
		if !slices.Equal(got, want) || len(readChatworkLogs(t, ctx, pool)) != 3 {
			t.Fatalf("racing senders duplicated requests or changed log count: %+v", got)
		}
	})
}

type chatworkHTTPFake struct {
	t           *testing.T
	pool        *pgxpool.Pool
	mu          sync.Mutex
	checkCommit func(context.Context) error
	replies     map[string]any
	calls       []chatworkTestRequest
}

func (fake *chatworkHTTPFake) reset(deleted []int, replies map[string]any) {
	fake.expect(replies, func(ctx context.Context) error {
		var remaining int
		err := fake.pool.QueryRow(ctx, "SELECT count(*) FROM reservations WHERE id = ANY($1::integer[])", deleted).Scan(&remaining)
		if err != nil {
			return err
		}
		if remaining != 0 {
			return fmt.Errorf("%d reservations still present", remaining)
		}
		return nil
	})
}

func (fake *chatworkHTTPFake) expect(replies map[string]any, checkCommit func(context.Context) error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.checkCommit, fake.replies, fake.calls = checkCommit, replies, nil
}

func (fake *chatworkHTTPFake) recorded() []chatworkTestRequest {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return slices.Clone(fake.calls)
}

func (fake *chatworkHTTPFake) serveHTTP(w http.ResponseWriter, r *http.Request) {
	room := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/rooms/"), "/messages")
	if r.Method != http.MethodPost || r.URL.Path != "/v2/rooms/"+room+"/messages" ||
		r.Header.Get("X-ChatWorkToken") != fakeChatworkToken ||
		r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
		fake.t.Error("invalid Chatwork request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	message := r.PostForm.Get("body")
	fake.mu.Lock()
	checkCommit, reply := fake.checkCommit, fake.replies[room]
	fake.calls = append(fake.calls, chatworkTestRequest{room, message})
	fake.mu.Unlock()

	if checkCommit != nil {
		if err := checkCommit(r.Context()); err != nil {
			fake.t.Errorf("HTTP arrived before reservation commit: %v", err)
		}
	}
	var claims int
	err := fake.pool.QueryRow(r.Context(), `
		SELECT count(*) FROM chatwork_delivery_logs WHERE room_id=$1 AND message=$2 AND status='Unknown'`,
		room, message).Scan(&claims)
	if err != nil || claims != 1 {
		fake.t.Errorf("HTTP arrived before committed claim: claims=%d error=%v", claims, err)
	}
	switch result := reply.(type) {
	case float64:
		w.WriteHeader(int(result))
		_, _ = w.Write([]byte(`{"synthetic":true}`))
	case string:
		if result != "timeout" {
			fake.t.Errorf("unexpected fake reply %q", result)
		}
		<-r.Context().Done()
	default:
		_, _ = w.Write([]byte(`{"message_id":"fake"}`))
	}
}

func seedChatworkCancelLog(t *testing.T, ctx context.Context, pool *pgxpool.Pool, row chatworkTestLog) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO chatwork_delivery_logs (reservation_id,delivery_type,delivery_key,target_user_id,scheduled_start_time_ticks,
		    room_id,status,error_message,attempted_at_ticks,message,created_at_ticks)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		row.ReservationID, row.DeliveryType, row.DeliveryKey, row.TargetUserID, seriesTestTicks(t, row.ScheduledStartTime),
		row.RoomID, row.Status, row.ErrorMessage, seriesTestTicks(t, row.AttemptedAt), row.Message, seriesTestTicks(t, row.CreatedAt)); err != nil {
		t.Fatal(err)
	}
}

func readChatworkLogs(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []chatworkTestLog {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT reservation_id,work_schedule_entry_id,delivery_type,delivery_key,target_user_id,scheduled_start_time_ticks,
		    room_id,status,error_message,attempted_at_ticks,sent_at_ticks,message,created_at_ticks
		FROM chatwork_delivery_logs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	logs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (chatworkTestLog, error) {
		var record chatworkTestLog
		var start, attempted, created int64
		var sent *int64
		err := row.Scan(&record.ReservationID, &record.WorkScheduleEntryID, &record.DeliveryType, &record.DeliveryKey, &record.TargetUserID,
			&start, &record.RoomID, &record.Status, &record.ErrorMessage, &attempted, &sent, &record.Message, &created)
		record.ScheduledStartTime = dotnettime.FromTicks(start).Format(dotnettime.Layout)
		record.AttemptedAt = dotnettime.FromTicks(attempted).Format(dotnettime.Layout)
		record.CreatedAt = dotnettime.FromTicks(created).Format(dotnettime.Layout)
		if sent != nil {
			record.SentAt = new(dotnettime.FromTicks(*sent).Format(dotnettime.Layout))
		}
		return record, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return logs
}

func compareChatworkLogs(t *testing.T, caseName string, expected, actual []chatworkTestLog, started time.Time, existingFirst bool) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("got %d delivery logs, want %d", len(actual), len(expected))
	}
	for i, got := range actual {
		want := expected[i]
		if !existingFirst || i != 0 {
			attempted, err := time.ParseInLocation(dotnettime.Layout, got.AttemptedAt, time.Local)
			if err != nil || attempted.Before(started.Truncate(100*time.Nanosecond)) || attempted.After(time.Now()) || got.CreatedAt != got.AttemptedAt {
				t.Fatalf("invalid attempt timestamps: %+v", got)
			}
			want.AttemptedAt, want.CreatedAt = got.AttemptedAt, got.CreatedAt
			if want.SentAt != nil {
				if got.SentAt == nil {
					t.Fatal("successful delivery has no sent time")
				}
				sent, err := time.ParseInLocation(dotnettime.Layout, *got.SentAt, time.Local)
				if err != nil || sent.Before(attempted) || sent.After(time.Now()) {
					t.Fatalf("invalid sent timestamp: %+v", got)
				}
				want.SentAt = got.SentAt
			}
		}
		if caseName == "provider_failure_and_timeout" {
			switch want.TargetUserID {
			case 2:
				want.Status, want.ErrorMessage = "Unknown", new("Chatwork request timed out.")
			case 3:
				want.ErrorMessage = new("Failed to send Chatwork message. StatusCode: 503.")
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("log %d = %+v, want %+v", i, got, want)
		}
	}
}

func rejectChatworkCancelCommit(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_cancellation_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF OLD.id = 72 THEN
				RAISE EXCEPTION 'synthetic failure during commit';
			END IF;
			RETURN OLD;
		END;
		$$;
		CREATE CONSTRAINT TRIGGER cancellation_commit_failure AFTER DELETE ON reservations
		    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_cancellation_commit();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `
			DROP TRIGGER cancellation_commit_failure ON reservations;
			DROP FUNCTION reject_cancellation_commit();`); err != nil {
			t.Error(err)
		}
	})
}
