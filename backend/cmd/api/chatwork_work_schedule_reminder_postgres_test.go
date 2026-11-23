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
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5/pgxpool"
)

type chatworkWorkReminderCase struct {
	Name          string
	Entries       []workschedules.Entry
	Reservations  []seriesEditRow
	DeletedIDs    []int
	ExistingKey   bool
	FakeResponses map[string]any
	Requests      []chatworkTestRequest
	Logs          []chatworkTestLog
}

func TestChatworkWorkScheduleReminderPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "CHATWORK_WORK_SCHEDULE_REMINDER_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0008_index_reservation_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0009_index_work_schedule_reminder_start_times.sql")
	var reference struct {
		Now   time.Time
		Users []chatworkTestUser
		Cases []chatworkWorkReminderCase
	}
	loadSeriesDeleteFixture(t, "chatwork_work_schedule_reminder_csharp.json", &reference)
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
			seedWorkReminderCase(t, ctx, pool, scenario)
			fake.expect(scenario.FakeResponses, nil)
			started := time.Now()
			if err := newSender(t).ProcessReminders(ctx, reference.Now); err != nil {
				t.Fatal(err)
			}
			if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
				t.Fatalf("Chatwork requests = %+v, want %+v", got, scenario.Requests)
			}
			original := readChatworkLogs(t, ctx, pool)
			compareChatworkLogs(t, scenario.Name, scenario.Logs, original, started, scenario.ExistingKey)

			fake.expect(nil, nil)
			if err := newSender(t).ProcessReminders(ctx, reference.Now); err != nil {
				t.Fatal(err)
			}
			if len(fake.recorded()) != 0 || !reflect.DeepEqual(readChatworkLogs(t, ctx, pool), original) {
				t.Fatal("existing delivery claims did not survive a new sender")
			}
		})
	}

	window := reference.Cases[0]
	t.Run("concurrent_passes_claim_each_key_once", func(t *testing.T) {
		seedWorkReminderCase(t, ctx, pool, window)
		fake.expect(nil, nil)
		a, b := newSender(t), newSender(t)
		started := time.Now()
		errs := make(chan error, 2)
		var workers sync.WaitGroup
		for _, sender := range []*chatwork.Sender{a, b} {
			workers.Go(func() { errs <- sender.ProcessReminders(ctx, reference.Now) })
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
		seedWorkReminderCase(t, ctx, pool, window)
		fake.expect(nil, nil)
		if err := newSender(t).ProcessReminders(ctx, reference.Now); err != nil {
			t.Fatal(err)
		}
		old := readChatworkLogs(t, ctx, pool)
		const changed = "2026-06-22T09:08:00.8765432"
		if _, err := pool.Exec(ctx, "UPDATE work_schedule_entries SET start_time_ticks=$1 WHERE id=30", seriesTestTicks(t, changed)); err != nil {
			t.Fatal(err)
		}
		fake.expect(nil, nil)
		if err := newSender(t).ProcessReminders(ctx, reference.Now); err != nil {
			t.Fatal(err)
		}
		logs, calls := readChatworkLogs(t, ctx, pool), fake.recorded()
		if len(logs) != len(old)+1 || len(calls) != 1 || !reflect.DeepEqual(logs[:len(old)], old) {
			t.Fatalf("reschedule did not produce exactly one new reminder: %v", calls)
		}
		row := logs[len(old)]
		if row.DeliveryKey != "WorkScheduleReminder10Minutes:work-schedule:30:user:2:start:"+changed ||
			row.ReservationID != 0 || row.WorkScheduleEntryID == nil || *row.WorkScheduleEntryID != 30 ||
			row.ScheduledStartTime != changed || row.Status != "Succeeded" ||
			calls[0].RoomID != "900002" || !strings.Contains(calls[0].Body, "時間: 09:08〜10:00") {
			t.Fatalf("unexpected rescheduled reminder: %+v", row)
		}
	})

	t.Run("api_disabled_by_default", func(t *testing.T) {
		t.Setenv("CHATWORK_ENABLED", "")
		entry := liveWorkReminderEntry(61, time.Now().Add(5*time.Minute))
		seedWorkReminderCase(t, ctx, pool, chatworkWorkReminderCase{Entries: []workschedules.Entry{entry}})
		fake.expect(nil, nil)
		stop := startReminderTestAPI(t, ctx, dsn)
		time.Sleep(150 * time.Millisecond)
		stop()
		if len(fake.recorded()) != 0 || len(readChatworkLogs(t, ctx, pool)) != 0 {
			t.Fatal("disabled worker sent or recorded a work schedule reminder")
		}
	})

	t.Run("api_startup_and_restart", func(t *testing.T) {
		t.Setenv("CHATWORK_ENABLED", "true")
		start := time.Now().Add(5 * time.Minute)
		first := liveWorkReminderEntry(61, start)
		seedWorkReminderCase(t, ctx, pool, chatworkWorkReminderCase{Entries: []workschedules.Entry{first}})
		fake.expect(nil, nil)
		stop := startReminderTestAPI(t, ctx, dsn)
		firstLogs := waitForSucceededReminders(t, ctx, pool, 1)
		stop()
		if len(fake.recorded()) != 1 || firstLogs[0].WorkScheduleEntryID == nil || *firstLogs[0].WorkScheduleEntryID != first.ID {
			t.Fatalf("unexpected startup reminders: %+v", firstLogs)
		}

		second := liveWorkReminderEntry(62, start.Add(30*time.Second))
		insertWorkReminderEntries(t, ctx, pool, []workschedules.Entry{second})
		fake.expect(nil, nil)
		stop = startReminderTestAPI(t, ctx, dsn)
		afterRestart := waitForSucceededReminders(t, ctx, pool, 2)
		stop()
		if len(fake.recorded()) != 1 || !reflect.DeepEqual(afterRestart[0], firstLogs[0]) ||
			afterRestart[1].WorkScheduleEntryID == nil || *afterRestart[1].WorkScheduleEntryID != second.ID {
			t.Fatalf("restart did not preserve existing work schedule claims: %+v", afterRestart)
		}
	})

	t.Run("api_shutdown_cancels_work_schedule_send", func(t *testing.T) {
		entry := liveWorkReminderEntry(63, time.Now().Add(5*time.Minute))
		seedWorkReminderCase(t, ctx, pool, chatworkWorkReminderCase{Entries: []workschedules.Entry{entry}})
		stopAPIDuringReminder(t, ctx, dsn)
		rows := readChatworkLogs(t, ctx, pool)
		if len(rows) != 1 || rows[0].Status != "Unknown" || rows[0].ReservationID != 0 ||
			rows[0].WorkScheduleEntryID == nil || *rows[0].WorkScheduleEntryID != entry.ID || rows[0].SentAt != nil {
			t.Fatalf("expected one uncertain work schedule delivery claim after shutdown: %+v", rows)
		}
	})
}

func seedWorkReminderCase(t *testing.T, ctx context.Context, pool *pgxpool.Pool, scenario chatworkWorkReminderCase) {
	t.Helper()
	seedSeriesEditRows(t, ctx, pool, scenario.Reservations)
	if _, err := pool.Exec(ctx, "TRUNCATE chatwork_delivery_logs RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}
	insertWorkReminderEntries(t, ctx, pool, scenario.Entries)
	if len(scenario.DeletedIDs) > 0 {
		if _, err := pool.Exec(ctx, "DELETE FROM work_schedule_entries WHERE id = ANY($1::integer[])", scenario.DeletedIDs); err != nil {
			t.Fatal(err)
		}
	}
	if scenario.ExistingKey {
		seedChatworkLog(t, ctx, pool, scenario.Logs[0])
	}
}

func insertWorkReminderEntries(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entries []workschedules.Entry) {
	t.Helper()
	nullableTicks := func(value *string) *int64 {
		if value == nil {
			return nil
		}
		return new(seriesTestTicks(t, *value))
	}
	for _, row := range entries {
		if _, err := pool.Exec(ctx, `
			INSERT INTO work_schedule_entries (id,created_by_user_id,type,title,series_id,date_ticks,
			    start_time_ticks,end_time_ticks,repeat_type,repeat_until_ticks,leave_period,
			    participant_ids,participants,created_at_ticks,updated_at_ticks)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			row.ID, row.CreatedByUserID, row.Type, row.Title, row.SeriesID, seriesTestTicks(t, row.Date),
			nullableTicks(row.StartTime), nullableTicks(row.EndTime), row.RepeatType, nullableTicks(row.RepeatUntil),
			row.LeavePeriod, row.ParticipantIDs, row.Participants, seriesTestTicks(t, row.CreatedAt),
			seriesTestTicks(t, row.UpdatedAt)); err != nil {
			t.Fatal(err)
		}
	}
}

func liveWorkReminderEntry(id int, start time.Time) workschedules.Entry {
	date := start.Format(time.DateOnly) + "T00:00:00"
	return workschedules.Entry{
		ID: id, CreatedByUserID: 2, Type: 0, Title: "起動確認用の架空社外予定", Date: date,
		StartTime: new(start.Format(dotnettime.Layout)), EndTime: new(start.Add(30 * time.Minute).Format(dotnettime.Layout)),
		ParticipantIDs: []int{2}, Participants: "保存済みの対象者", CreatedAt: date, UpdatedAt: date,
	}
}
