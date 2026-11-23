package main

import (
	"cmp"
	"context"
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
	"github.com/jackc/pgx/v5/pgxpool"
)

type chatworkReminderCase struct {
	Name          string
	Reservations  []seriesEditRow
	DeletedIDs    []int
	ExistingKey   bool
	FakeResponses map[string]any
	Requests      []chatworkTestRequest
	Logs          []chatworkTestLog
}

func TestChatworkReservationReminderPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "CHATWORK_REMINDER_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0008_index_reservation_start_times.sql")
	var reference struct {
		Now   time.Time
		Users []chatworkTestUser
		Cases []chatworkReminderCase
	}
	loadSeriesDeleteFixture(t, "chatwork_reservation_reminder_csharp.json", &reference)
	seedChatworkTestProfiles(t, ctx, pool, reference.Users)

	fake := &chatworkHTTPFake{t: t, pool: pool}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()
	t.Setenv("CHATWORK_API_BASE_URL", server.URL+"/v2/")
	t.Setenv("CHATWORK_API_TOKEN", fakeChatworkToken)
	newSender := func(t *testing.T) *chatwork.Sender {
		t.Helper()
		sender, err := chatwork.NewSender(pool, fakeChatworkToken, server.URL+"/v2/", true)
		if err != nil {
			t.Fatal(err)
		}
		return sender
	}

	for _, scenario := range reference.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			seedReminderCase(t, ctx, pool, scenario)
			fake.expect(scenario.FakeResponses, nil)
			started := time.Now()
			if err := newSender(t).ProcessReservationReminders(ctx, reference.Now); err != nil {
				t.Fatal(err)
			}
			if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
				t.Fatalf("Chatwork requests = %+v, want %+v", got, scenario.Requests)
			}
			original := readChatworkLogs(t, ctx, pool)
			compareChatworkLogs(t, scenario.Name, scenario.Logs, original, started, scenario.ExistingKey)

			fake.expect(nil, nil)
			if err := newSender(t).ProcessReservationReminders(ctx, reference.Now); err != nil {
				t.Fatal(err)
			}
			if len(fake.recorded()) != 0 || !reflect.DeepEqual(readChatworkLogs(t, ctx, pool), original) {
				t.Fatal("existing delivery claims did not survive a new sender")
			}
		})
	}

	window := reference.Cases[0]
	t.Run("concurrent_passes_claim_each_key_once", func(t *testing.T) {
		seedReminderCase(t, ctx, pool, window)
		fake.expect(nil, nil)
		a, b := newSender(t), newSender(t)
		started := time.Now()
		errs := make(chan error, 2)
		var workers sync.WaitGroup
		for _, sender := range []*chatwork.Sender{a, b} {
			workers.Go(func() { errs <- sender.ProcessReservationReminders(ctx, reference.Now) })
		}
		workers.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		wantCalls, actualCalls := slices.Clone(window.Requests), fake.recorded()
		order := func(a, b chatworkTestRequest) int {
			return cmp.Or(cmp.Compare(a.RoomID, b.RoomID), cmp.Compare(a.Body, b.Body))
		}
		slices.SortFunc(wantCalls, order)
		slices.SortFunc(actualCalls, order)
		if !slices.Equal(actualCalls, wantCalls) {
			t.Fatalf("concurrent passes sent %+v, want %+v", actualCalls, wantCalls)
		}
		wantLogs, actualLogs := slices.Clone(window.Logs), readChatworkLogs(t, ctx, pool)
		byKey := func(a, b chatworkTestLog) int { return cmp.Compare(a.DeliveryKey, b.DeliveryKey) }
		slices.SortFunc(wantLogs, byKey)
		slices.SortFunc(actualLogs, byKey)
		compareChatworkLogs(t, window.Name, wantLogs, actualLogs, started, false)
	})

	t.Run("changed_start_has_a_new_key", func(t *testing.T) {
		seedReminderCase(t, ctx, pool, window)
		fake.expect(nil, nil)
		if err := newSender(t).ProcessReservationReminders(ctx, reference.Now); err != nil {
			t.Fatal(err)
		}
		old := readChatworkLogs(t, ctx, pool)
		const changed = "2026-06-22T09:08:00.8765432"
		if _, err := pool.Exec(ctx, "UPDATE reservations SET start_time_ticks=$1 WHERE id=30", seriesTestTicks(t, changed)); err != nil {
			t.Fatal(err)
		}
		fake.expect(nil, nil)
		if err := newSender(t).ProcessReservationReminders(ctx, reference.Now); err != nil {
			t.Fatal(err)
		}
		logs, calls := readChatworkLogs(t, ctx, pool), fake.recorded()
		if len(logs) != len(old)+1 || len(calls) != 1 || !reflect.DeepEqual(logs[:len(old)], old) {
			t.Fatalf("reschedule did not produce exactly one new reminder: %v", calls)
		}
		if row := logs[len(old)]; row.DeliveryKey != "Reminder10Minutes:reservation:30:user:2:start:"+changed ||
			row.ScheduledStartTime != changed || row.Status != "Succeeded" ||
			calls[0].RoomID != "900002" || !strings.Contains(calls[0].Body, "時間: 09:08〜10:00") {
			t.Fatalf("unexpected rescheduled reminder: %+v", row)
		}
	})

	t.Run("api_disabled_by_default", func(t *testing.T) {
		t.Setenv("CHATWORK_ENABLED", "")
		seedReminderCase(t, ctx, pool, chatworkReminderCase{Reservations: []seriesEditRow{liveReminderRow(61, time.Now().Add(5*time.Minute))}})
		fake.expect(nil, nil)
		stop := startReminderTestAPI(t, ctx, dsn)
		time.Sleep(150 * time.Millisecond)
		stop()
		if len(fake.recorded()) != 0 || len(readChatworkLogs(t, ctx, pool)) != 0 {
			t.Fatal("disabled worker sent or recorded a reminder")
		}
	})

	t.Run("api_startup_and_restart", func(t *testing.T) {
		t.Setenv("CHATWORK_ENABLED", "true")
		start := time.Now().Add(5 * time.Minute)
		first := liveReminderRow(61, start)
		seedReminderCase(t, ctx, pool, chatworkReminderCase{Reservations: []seriesEditRow{first}})
		fake.expect(nil, nil)
		stop := startReminderTestAPI(t, ctx, dsn)
		firstLogs := waitForSucceededReminders(t, ctx, pool, 1)
		stop()
		if len(fake.recorded()) != 1 || firstLogs[0].ReservationID != first.ID {
			t.Fatalf("unexpected startup reminders: %+v", firstLogs)
		}

		second := liveReminderRow(62, start.Add(30*time.Second))
		seedSeriesEditRows(t, ctx, pool, []seriesEditRow{first, second})
		fake.expect(nil, nil)
		stop = startReminderTestAPI(t, ctx, dsn)
		afterRestart := waitForSucceededReminders(t, ctx, pool, 2)
		stop()
		if len(fake.recorded()) != 1 || !reflect.DeepEqual(afterRestart[0], firstLogs[0]) || afterRestart[1].ReservationID != second.ID {
			t.Fatalf("restart did not preserve existing claims: %+v", afterRestart)
		}
	})

	t.Run("api_shutdown_cancels_in_flight_reminder", func(t *testing.T) {
		seedReminderCase(t, ctx, pool, chatworkReminderCase{Reservations: []seriesEditRow{liveReminderRow(63, time.Now().Add(5*time.Minute))}})
		started, canceled, release := make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{})
		blocking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			started <- struct{}{}
			select {
			case <-r.Context().Done():
				canceled <- struct{}{}
			case <-release:
			}
		}))
		defer blocking.Close()
		defer close(release)
		t.Setenv("CHATWORK_API_BASE_URL", blocking.URL+"/v2/")
		t.Setenv("CHATWORK_ENABLED", "true")
		stop := startReminderTestAPI(t, ctx, dsn)
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("API did not begin the initial reminder")
		}
		before := time.Now()
		stop()
		if elapsed := time.Since(before); elapsed >= 2*time.Second {
			t.Fatalf("shutdown waited %s instead of canceling the HTTP request", elapsed)
		}
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("fake server did not observe cancellation")
		}
		if rows := readChatworkLogs(t, ctx, pool); len(rows) != 1 || rows[0].Status != "Unknown" || rows[0].ReservationID != 63 {
			t.Fatalf("expected one uncertain delivery claim after shutdown: %+v", rows)
		}
	})
}

func seedReminderCase(t *testing.T, ctx context.Context, pool *pgxpool.Pool, scenario chatworkReminderCase) {
	t.Helper()
	seedSeriesEditRows(t, ctx, pool, scenario.Reservations)
	if _, err := pool.Exec(ctx, "TRUNCATE chatwork_delivery_logs RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}
	if len(scenario.DeletedIDs) > 0 {
		if _, err := pool.Exec(ctx, "DELETE FROM reservations WHERE id = ANY($1::integer[])", scenario.DeletedIDs); err != nil {
			t.Fatal(err)
		}
	}
	if scenario.ExistingKey {
		seedChatworkCancelLog(t, ctx, pool, scenario.Logs[0])
	}
}

func liveReminderRow(id int, start time.Time) seriesEditRow {
	return seriesEditRow{
		ID: id, UserID: 2, NumberOfPeople: 1, Name: "架空の予約者", Room: "大会議室", Type: "社内", Purpose: "起動確認用の架空予約",
		Date: start.Format(time.DateOnly) + "T00:00:00", StartTime: start.Format(dotnettime.Layout),
		EndTime: start.Add(30 * time.Minute).Format(dotnettime.Layout), ParticipantIDs: []int{2}, RepeatType: "しない",
	}
}

func startReminderTestAPI(t *testing.T, ctx context.Context, dsn string) func() {
	t.Helper()
	_, stop := startAuthTestAPI(t, ctx, dsn)
	once := sync.OnceFunc(stop)
	t.Cleanup(once)
	return once
}

func waitForSucceededReminders(t *testing.T, ctx context.Context, pool *pgxpool.Pool, count int) []chatworkTestLog {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows := readChatworkLogs(t, ctx, pool)
		if len(rows) == count && !slices.ContainsFunc(rows, func(row chatworkTestLog) bool { return row.Status != "Succeeded" }) {
			return rows
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("API did not complete %d reminders in its first pass", count)
	return nil
}
