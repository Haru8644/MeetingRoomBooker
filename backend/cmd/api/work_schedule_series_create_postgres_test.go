package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5/pgxpool"
)

const workScheduleSeriesTestPath = workScheduleTestPath + "/series"

type workScheduleSeriesCase struct {
	Name, Actor, Failure string
	Request              json.RawMessage
	Response             workScheduleCreationCapture
	Saved                []workschedules.Entry
	Notices              []workScheduleCreationNotice
	Logs                 []chatworkTestLog
	Requests             []chatworkTestRequest
	Seeds                []workScheduleConflictSeed
	FakeResponses        map[string]any
}

func TestWorkScheduleSeriesCreationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "WORK_SCHEDULE_SERIES_CREATE_TEST_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0008_index_reservation_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0009_index_work_schedule_reminder_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0010_enable_work_schedule_creation.sql")

	var reference struct {
		Users []chatworkTestUser
		Cases []workScheduleSeriesCase
	}
	loadSeriesDeleteFixture(t, "work_schedule_series_create_csharp.json", &reference)
	passwords := seedAuthTestUsers(t, ctx, pool)
	seedChatworkTestProfiles(t, ctx, pool, reference.Users)

	fake := &chatworkHTTPFake{t: t, pool: pool}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()
	t.Setenv("CHATWORK_ENABLED", "true")
	t.Setenv("CHATWORK_API_TOKEN", fakeChatworkToken)
	t.Setenv("CHATWORK_API_BASE_URL", server.URL+"/v2/")
	login := func(t *testing.T, base, user string) *http.Cookie {
		t.Helper()
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": user + "@example.test", "password": passwords[user]}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}

	t.Run("compare", func(t *testing.T) {
		const origin = "http://127.0.0.1:5168"
		addr, stop := startAuthTestAPI(t, ctx, dsn, origin)
		defer stop()
		base := "http://" + addr
		actors := map[string]*http.Cookie{"admin": login(t, base, "admin"), "owner": login(t, base, "member")}
		for _, scenario := range reference.Cases {
			t.Run(scenario.Name, func(t *testing.T) {
				resetWorkScheduleSeriesState(t, ctx, pool)
				seedWorkScheduleCreationConflicts(t, ctx, pool, scenario.Seeds)
				seeded := 0
				for _, seed := range scenario.Seeds {
					if seed.Kind == "work" {
						seeded++
					}
				}

				status, wantRows, wantNotices := scenario.Response.Status, len(scenario.Saved), len(scenario.Notices)
				switch scenario.Failure {
				case "notice":
					constrainSeriesTestWrites(t, ctx, pool, "notifications", "user_id <> 3")
					status, wantRows = http.StatusInternalServerError, 0
				case "insert":
					secondDate := seriesTestTicks(t, "2026-06-22T00:00:00")
					constrainSeriesTestWrites(t, ctx, pool, "work_schedule_entries", fmt.Sprintf("date_ticks <> %d", secondDate))
				case "calendar":
					// The C# iterator advances past DateTime.MaxValue after the last valid date.
					status, wantRows, wantNotices = http.StatusOK, 1, 3
				}
				fake.expect(scenario.FakeResponses, func(ctx context.Context) error {
					return checkWorkScheduleCreationCounts(ctx, pool, seeded+wantRows, wantNotices)
				})

				started := time.Now()
				res, body := callWorkScheduleRoute(t, ctx, http.MethodPost, base+workScheduleSeriesTestPath,
					scenario.Request, actors[scenario.Actor], "", status)
				if err := checkWorkScheduleCreationCounts(ctx, pool, seeded+wantRows, wantNotices); err != nil {
					t.Fatal(err)
				}

				if scenario.Failure == "calendar" {
					var entries []workschedules.Entry
					if err := json.Unmarshal(body, &entries); err != nil || len(entries) != 1 ||
						entries[0].Date != "9999-12-31T00:00:00" || entries[0].SeriesID == nil ||
						len(fake.recorded()) != 2 {
						t.Fatalf("final calendar day was not saved and delivered: %s, %v", body, err)
					}
					return
				}

				if status == http.StatusInternalServerError {
					if len(body) != 0 || len(fake.recorded()) != 0 || len(readChatworkLogs(t, ctx, pool)) != 0 {
						t.Fatal("rolled-back request had a response body or delivery side effects")
					}
					return
				}
				if status != http.StatusOK {
					compareWorkScheduleResponse(t, scenario.Response, res, body)
				} else {
					if res.Header.Get("Location") != "" || res.Header.Get("Content-Type") != scenario.Response.ContentType {
						t.Fatalf("unexpected series response headers: %v", res.Header)
					}
					var entries []workschedules.Entry
					if err := json.Unmarshal(body, &entries); err != nil {
						t.Fatal(err)
					}
					compareSavedWorkScheduleSeries(t, scenario.Saved, entries, started)
					reader := actors["admin"]
					if scenario.Actor == "admin" {
						reader = actors["owner"]
					}
					verifyWorkScheduleSeriesReads(t, ctx, base, reader, entries)
					remapWorkScheduleSeriesTargets(t, &scenario, entries[0])
					if scenario.Name == "daily_across_weekend" {
						verifyWorkScheduleDeliveryReplay(t, ctx, pool, fake, server.URL, entries, scenario.Logs[0])
					}
				}
				if got := readWorkScheduleCreationNotices(t, ctx, pool); !reflect.DeepEqual(got, scenario.Notices) {
					t.Fatalf("saved notifications = %+v, want %+v", got, scenario.Notices)
				}
				if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
					t.Fatalf("Chatwork requests = %+v, want %+v", got, scenario.Requests)
				}
				compareChatworkLogs(t, scenario.Name, scenario.Logs, readChatworkLogs(t, ctx, pool), started, false)
			})
		}
		t.Run("origin_protection", func(t *testing.T) {
			resetWorkScheduleSeriesState(t, ctx, pool)
			fake.expect(nil, nil)
			payload := reference.Cases[0].Request
			callWorkScheduleRoute(t, ctx, http.MethodPost, base+workScheduleSeriesTestPath,
				payload, actors["owner"], "https://untrusted.example.test", http.StatusForbidden)
			if err := checkWorkScheduleCreationCounts(ctx, pool, 0, 0); err != nil || len(fake.recorded()) > 0 {
				t.Fatalf("untrusted write had side effects: %v", err)
			}
			fake.expect(nil, func(ctx context.Context) error { return checkWorkScheduleCreationCounts(ctx, pool, 3, 3) })
			res, _ := callWorkScheduleRoute(t, ctx, http.MethodPost, base+workScheduleSeriesTestPath,
				payload, actors["owner"], origin, http.StatusOK)
			if res.Header.Get("Access-Control-Allow-Origin") != origin || len(fake.recorded()) != 2 {
				t.Fatal("trusted origin did not create and deliver")
			}
		})
	})
	t.Run("disabled_by_default", func(t *testing.T) {
		resetWorkScheduleSeriesState(t, ctx, pool)
		fake.expect(nil, nil)
		t.Setenv("CHATWORK_ENABLED", "")
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		base := "http://" + addr
		callWorkScheduleRoute(t, ctx, http.MethodPost, base+workScheduleSeriesTestPath, reference.Cases[0].Request,
			login(t, base, "member"), "", http.StatusOK)
		if err := checkWorkScheduleCreationCounts(ctx, pool, 3, 3); err != nil {
			t.Fatal(err)
		}
		if len(fake.recorded()) > 0 || len(readChatworkLogs(t, ctx, pool)) > 0 {
			t.Fatal("disabled Chatwork had delivery side effects")
		}
	})
}

func resetWorkScheduleSeriesState(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	execWorkScheduleTestSQL(t, ctx, pool, "TRUNCATE reservations, work_schedule_entries, notifications, chatwork_delivery_logs")
}

var workScheduleSeriesUUID = regexp.MustCompile(`^[0-9a-f]{12}4[0-9a-f]{3}[89ab][0-9a-f]{15}$`)

func compareSavedWorkScheduleSeries(t *testing.T, captured, actual []workschedules.Entry, started time.Time) {
	t.Helper()
	if len(actual) != len(captured) || len(actual) == 0 {
		t.Fatalf("returned %d entries, want %d", len(actual), len(captured))
	}
	actualSeries, capturedSeries := actual[0].SeriesID, captured[0].SeriesID
	if (actualSeries == nil) != (capturedSeries == nil) ||
		(actualSeries != nil && (!workScheduleSeriesUUID.MatchString(*actualSeries) || !workScheduleSeriesUUID.MatchString(*capturedSeries))) {
		t.Fatal("series IDs did not have the same UUID format or nullability")
	}
	createdAt := actual[0].CreatedAt
	created, err := time.ParseInLocation(dotnettime.Layout, createdAt, time.Local)
	if err != nil || created.Before(started.Truncate(100*time.Nanosecond)) || created.After(time.Now()) {
		t.Fatalf("invalid creation timestamp %q", createdAt)
	}
	ids := make(map[int]bool, len(actual))
	for i, entry := range actual {
		want := captured[i]
		if entry.ID <= 0 || ids[entry.ID] || entry.CreatedAt != createdAt ||
			!reflect.DeepEqual(entry.SeriesID, actualSeries) ||
			!reflect.DeepEqual(want.SeriesID, capturedSeries) {
			t.Fatalf("invalid ID, series grouping, or timestamp in entry %+v", entry)
		}
		ids[entry.ID] = true
		want.ID, want.SeriesID, want.CreatedAt, want.UpdatedAt = entry.ID, actualSeries, createdAt, createdAt
		if !reflect.DeepEqual(entry, want) {
			t.Fatalf("entry %d = %+v, want %+v", i, entry, want)
		}
	}
}

func verifyWorkScheduleSeriesReads(t *testing.T, ctx context.Context, base string, cookie *http.Cookie, entries []workschedules.Entry) {
	t.Helper()
	route := workScheduleTestPath + "?from=" + entries[0].Date[:10] + "&to=" + entries[len(entries)-1].Date[:10]
	_, body := callWorkScheduleRoute(t, ctx, http.MethodGet, base+route, nil, cookie, "", http.StatusOK)
	var listing []workschedules.Entry
	if err := json.Unmarshal(body, &listing); err != nil {
		t.Fatal(err)
	}
	ids := make(map[int]bool, len(entries))
	for _, entry := range entries {
		ids[entry.ID] = true
	}
	listing = slices.DeleteFunc(listing, func(entry workschedules.Entry) bool { return !ids[entry.ID] })
	if !reflect.DeepEqual(listing, entries) {
		t.Fatal("saved list does not match the POST array and its order")
	}
	for _, entry := range entries {
		_, body := callWorkScheduleRoute(t, ctx, http.MethodGet, base+workScheduleTestPath+"/"+strconv.Itoa(entry.ID),
			nil, cookie, "", http.StatusOK)
		var detail workschedules.Entry
		if err := json.Unmarshal(body, &detail); err != nil || !reflect.DeepEqual(detail, entry) {
			t.Fatalf("detail body = %s, want %+v, error = %v", body, entry, err)
		}
	}
}

func remapWorkScheduleSeriesTargets(t *testing.T, scenario *workScheduleSeriesCase, actual workschedules.Entry) {
	t.Helper()
	captured := scenario.Saved[0]
	for i := range scenario.Notices {
		notice := &scenario.Notices[i]
		if notice.TargetWorkScheduleEntryID != captured.ID {
			t.Fatal("captured notification did not target the representative row")
		}
		notice.TargetWorkScheduleEntryID = actual.ID
	}
	deliveryKey := func(entry workschedules.Entry, user int) string {
		if entry.SeriesID != nil {
			return fmt.Sprintf("WorkScheduleSeriesCreated:work-schedule-series:%s:user:%d", *entry.SeriesID, user)
		}
		return fmt.Sprintf("WorkScheduleCreated:work-schedule:%d:user:%d", entry.ID, user)
	}
	for i := range scenario.Logs {
		row := &scenario.Logs[i]
		if row.WorkScheduleEntryID == nil || *row.WorkScheduleEntryID != captured.ID ||
			row.DeliveryKey != deliveryKey(captured, row.TargetUserID) {
			t.Fatal("captured delivery did not use the expected C# key and representative ID")
		}
		row.WorkScheduleEntryID = &actual.ID
		row.DeliveryKey = deliveryKey(actual, row.TargetUserID)
	}
}
