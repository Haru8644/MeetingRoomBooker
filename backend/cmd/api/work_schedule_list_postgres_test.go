package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkScheduleListPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "WORK_SCHEDULE_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	credentials := seedAuthTestUsers(t, ctx, pool)
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	baseURL := "http://" + addr
	const path = "/api/work-schedule-entries"

	_, body := authTestRequest(t, ctx, baseURL, http.MethodGet, path, nil, nil, http.StatusUnauthorized)
	if len(body) != 0 {
		t.Fatal("anonymous list should have an empty body")
	}
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0004_create_work_schedule_entries.sql")
	fixture := readWorkScheduleFixture(t, time.Now())
	expectedRows := seedCapturedWorkSchedules(t, ctx, pool, fixture.Rows)
	login, _ := authTestRequest(t, ctx, baseURL, http.MethodPost, "/api/Auth/login",
		map[string]string{"email": "member@example.test", "password": credentials["member"]}, nil, http.StatusOK)
	cookie := authTestCookie(t, login, false)

	for _, test := range fixture.Cases {
		if test.Status == http.StatusUnauthorized {
			continue // Checked above before the table existed.
		}
		t.Run(test.Name, func(t *testing.T) {
			response, body := authTestRequest(t, ctx, baseURL, http.MethodGet, path+"?"+test.Query, nil, cookie, test.Status)
			var expected any
			if test.Status == http.StatusOK {
				var ids []int
				if err := json.Unmarshal(test.Result, &ids); err != nil {
					t.Fatal(err)
				}
				rows := make([]any, 0, len(ids))
				for _, id := range ids {
					rows = append(rows, expectedRows[id])
				}
				expected = rows
			} else if err := json.Unmarshal(test.Result, &expected); err != nil {
				t.Fatal(err)
			}
			var actual any
			if response.Header.Get("Content-Type") == "text/plain; charset=utf-8" {
				actual = string(body)
			} else if err := json.Unmarshal(body, &actual); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("Go response %s, expected C# result %v", body, expected)
			}
		})
	}
}

type workScheduleFixture struct {
	ReferenceDay string
	Rows         []json.RawMessage
	Cases        []struct {
		Name   string
		Query  string
		Status int
		Result json.RawMessage
	}
}

func readWorkScheduleFixture(t *testing.T, today time.Time) workScheduleFixture {
	t.Helper()
	data, err := os.ReadFile("../../internal/httpapi/testdata/work_schedule_list_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture workScheduleFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	referenceDay, err := time.Parse(time.DateOnly, fixture.ReferenceDay)
	if err != nil {
		t.Fatal(err)
	}
	midnight := dotnettime.OnDate(today, time.Time{})
	days := int((midnight.Unix() - referenceDay.Unix()) / (24 * 60 * 60))
	if days == 0 {
		return fixture
	}
	// Shift both seed data and captured queries so the default window is repeatable on later days.
	pattern := regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	shifted := pattern.ReplaceAllFunc(data, func(raw []byte) []byte {
		date, err := time.Parse(time.DateOnly, string(raw))
		if err != nil {
			t.Fatal(err)
		}
		return []byte(date.AddDate(0, 0, days).Format(time.DateOnly))
	})
	if err := json.Unmarshal(shifted, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func seedCapturedWorkSchedules(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entries []json.RawMessage) map[int]map[string]any {
	t.Helper()
	expected := make(map[int]map[string]any, len(entries))
	rows := make([][]any, 0, len(entries))
	for _, raw := range entries {
		var entry workschedules.Entry
		var body map[string]any
		if json.Unmarshal(raw, &entry) != nil || json.Unmarshal(raw, &body) != nil {
			t.Fatal("invalid C# work schedule entry")
		}
		expected[entry.ID] = body
		rows = append(rows, []any{
			entry.ID, entry.CreatedByUserID, entry.Type, entry.Title, entry.SeriesID, fixtureDateTimeTicks(t, &entry.Date),
			fixtureDateTimeTicks(t, entry.StartTime), fixtureDateTimeTicks(t, entry.EndTime), entry.RepeatType,
			fixtureDateTimeTicks(t, entry.RepeatUntil), entry.LeavePeriod, entry.ParticipantIDs, entry.Participants,
			fixtureDateTimeTicks(t, &entry.CreatedAt), fixtureDateTimeTicks(t, &entry.UpdatedAt),
		})
	}
	columns := []string{
		"id", "created_by_user_id", "type", "title", "series_id", "date_ticks", "start_time_ticks", "end_time_ticks", "repeat_type",
		"repeat_until_ticks", "leave_period", "participant_ids", "participants", "created_at_ticks", "updated_at_ticks",
	}
	if _, err := pool.CopyFrom(ctx, pgx.Identifier{"work_schedule_entries"}, columns, pgx.CopyFromRows(rows)); err != nil {
		t.Fatal(err)
	}
	return expected
}

func fixtureDateTimeTicks(t *testing.T, input *string) *int64 {
	t.Helper()
	if input == nil {
		return nil
	}
	value, err := time.Parse(dotnettime.Layout, *input)
	if err != nil {
		t.Fatal(err)
	}
	return new(dotnettime.Ticks(value))
}
