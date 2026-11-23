package main

import (
	"context"
	"encoding/hex"
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

type chatworkUpdateCase struct {
	Name, Method, Path, Dataset string
	Status, NoticeCount         int
	Request                     json.RawMessage
	Rows                        []seriesEditRow
	Appointments                []seriesTestAppointment
	ForceNoticeFailure          bool
	FakeResponses               map[string]any
	Requests                    []chatworkTestRequest
	Logs                        []chatworkTestLog
}

func TestChatworkReservationUpdatePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "CHATWORK_UPDATE_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	password := seedAuthTestUsers(t, ctx, pool)["member"]
	var reference struct {
		Users    []chatworkTestUser
		Datasets map[string][]seriesEditRow
		Cases    []chatworkUpdateCase
	}
	loadSeriesDeleteFixture(t, "chatwork_reservation_update_csharp.json", &reference)
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
		scenario := reference.Cases[0]
		seedSeriesEditRows(t, ctx, pool, reference.Datasets[scenario.Dataset])
		seedSeriesTestAppointments(t, ctx, pool, scenario.Appointments)
		authTestRequest(t, ctx, base, scenario.Method, scenario.Path, scenario.Request, login(t, base), http.StatusNoContent)
		assertSeriesCounts(t, ctx, pool, len(scenario.Rows), scenario.NoticeCount)
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
			seedSeriesTestAppointments(t, ctx, pool, scenario.Appointments)
			resetChatworkUpdateLogs(t, ctx, pool)
			wantRows := scenario.Rows
			if scenario.ForceNoticeFailure {
				failChatworkNoticeCommit(t, ctx, pool)
				// C# commits the edits first. Go rolls them back when notices fail.
				wantRows = dataset
			}
			fake.expect(scenario.FakeResponses, checkCommittedChatworkUpdate(pool, wantRows, scenario.NoticeCount))
			started := time.Now()
			_, body := authTestRequest(t, ctx, base, scenario.Method, scenario.Path, scenario.Request, cookie, scenario.Status)
			if len(body) != 0 {
				t.Fatalf("expected an empty update response, got %s", body)
			}
			assertSeriesCounts(t, ctx, pool, len(wantRows), scenario.NoticeCount)
			actualRows := readSeriesEditRows(t, ctx, base, cookie)
			for _, row := range wantRows {
				if !reflect.DeepEqual(actualRows[row.ID], row) {
					t.Errorf("reservation %d = %+v, want %+v", row.ID, actualRows[row.ID], row)
				}
			}
			if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
				t.Fatalf("Chatwork HTTP calls = %+v, want %+v", got, scenario.Requests)
			}
			logs := readChatworkLogs(t, ctx, pool)
			if len(logs) != len(scenario.Logs) {
				t.Fatalf("logged %d deliveries, want %d", len(logs), len(scenario.Logs))
			}
			wantLogs := slices.Clone(scenario.Logs)
			if len(logs) > 0 {
				changeID := chatworkUpdateChangeID(t, logs)
				referenceChangeID := chatworkUpdateChangeID(t, wantLogs)
				for i := range wantLogs {
					wantLogs[i].DeliveryKey = strings.TrimSuffix(wantLogs[i].DeliveryKey, referenceChangeID) + changeID
				}
			}
			compareChatworkLogs(t, scenario.Name, wantLogs, logs, started, false)
		})
	}

	t.Run("same_event_replay_and_separate_event", func(t *testing.T) {
		const path = "/api/Reservations/51?notifyParticipants=false"
		scenarioIndex := slices.IndexFunc(reference.Cases, func(c chatworkUpdateCase) bool { return c.Name == "single_notify_false" })
		if scenarioIndex < 0 {
			t.Fatal("missing single update fixture")
		}
		scenario := reference.Cases[scenarioIndex]
		seedSeriesEditRows(t, ctx, pool, reference.Datasets[scenario.Dataset])
		resetChatworkUpdateLogs(t, ctx, pool)
		before := readSingleChatworkUpdate(t, ctx, pool)
		fake.expect(nil, checkCommittedChatworkUpdate(pool, scenario.Rows, 0))
		authTestRequest(t, ctx, base, http.MethodPut, path, scenario.Request, cookie, http.StatusNoContent)
		firstLogs := readChatworkLogs(t, ctx, pool)
		firstID := chatworkUpdateChangeID(t, firstLogs)
		after := readSingleChatworkUpdate(t, ctx, pool)
		sender, err := chatwork.NewSender(pool, fakeChatworkToken, server.URL+"/v2/", true)
		if err != nil {
			t.Fatal(err)
		}
		fake.expect(nil, nil)
		sender.SendUpdate(ctx, reservations.UpdateResult{Previous: before, Current: after, Count: 1, ChangeID: firstID})
		if len(fake.recorded()) != 0 || !reflect.DeepEqual(readChatworkLogs(t, ctx, pool), firstLogs) {
			t.Fatal("replaying the same update event repeated a send or changed a claim")
		}

		var request map[string]any
		if err := json.Unmarshal(scenario.Request, &request); err != nil {
			t.Fatal(err)
		}
		request["purpose"] = "2回目の架空更新"
		secondRows := slices.Clone(scenario.Rows)
		secondRows[0].Purpose = "2回目の架空更新"
		fake.expect(nil, checkCommittedChatworkUpdate(pool, secondRows, 0))
		authTestRequest(t, ctx, base, http.MethodPut, path, request, cookie, http.StatusNoContent)
		logs := readChatworkLogs(t, ctx, pool)
		// Only the organizer, current participant and missing-room participant
		// have a claim for the second event. The last event did not remove anyone.
		if len(logs) != len(firstLogs)+3 || len(fake.recorded()) != 2 ||
			!reflect.DeepEqual(logs[:len(firstLogs)], firstLogs) {
			t.Fatalf("second update did not produce independent delivery claims: %+v", logs)
		}
		if id := chatworkUpdateChangeID(t, logs[len(firstLogs):]); id == firstID {
			t.Fatalf("second event reused change ID %s", id)
		}
	})
}

func checkCommittedChatworkUpdate(pool *pgxpool.Pool, rows []seriesEditRow, noticeCount int) func(context.Context) error {
	return func(ctx context.Context) error {
		var notices int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&notices); err != nil {
			return err
		}
		if notices != noticeCount {
			return fmt.Errorf("found %d notices, want %d", notices, noticeCount)
		}
		for _, row := range rows {
			var purpose string
			if err := pool.QueryRow(ctx, "SELECT purpose FROM reservations WHERE id=$1", row.ID).Scan(&purpose); err != nil {
				return err
			}
			if purpose != row.Purpose {
				return fmt.Errorf("reservation %d still has purpose %q", row.ID, purpose)
			}
		}
		return nil
	}
}

func resetChatworkUpdateLogs(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "TRUNCATE chatwork_delivery_logs RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}
}

func readSingleChatworkUpdate(t *testing.T, ctx context.Context, pool *pgxpool.Pool) reservations.ListEntry {
	t.Helper()
	entries, err := reservations.List(ctx, pool)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one reservation, got %d (error %v)", len(entries), err)
	}
	return entries[0]
}

func chatworkUpdateChangeID(t *testing.T, logs []chatworkTestLog) string {
	t.Helper()
	if len(logs) == 0 {
		t.Fatal("expected delivery logs")
	}
	var eventID string
	for _, row := range logs {
		prefix := fmt.Sprintf("ReservationUpdated:reservation:%d:user:%d:change:", row.ReservationID, row.TargetUserID)
		id, found := strings.CutPrefix(row.DeliveryKey, prefix)
		binary, err := hex.DecodeString(id)
		if !found || err != nil || len(binary) != 16 || id != strings.ToLower(id) || (eventID != "" && eventID != id) {
			t.Fatalf("invalid or unshared change ID in %s", row.DeliveryKey)
		}
		eventID = id
	}
	return eventID
}
