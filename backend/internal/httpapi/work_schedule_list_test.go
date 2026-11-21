package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

func TestWorkScheduleListValidatesRange(t *testing.T) {
	for _, test := range []struct{ name, query, problem string }{
		{"inverted range", "from=2026-11-23&to=2026-11-22", "取得開始日は取得終了日以前にしてください。"},
		{"invalid date", "from=not-a-date", "The value 'not-a-date' is not valid."},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/work-schedule-entries?"+test.query, nil)
			newWorkScheduleListHandler(func(context.Context, time.Time, time.Time) ([]workschedules.Entry, error) {
				t.Fatal("invalid date range read the database")
				return nil, nil
			})(response, request, auth.User{ID: 2})
			if response.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", response.Code)
			}
			if test.name == "inverted range" {
				if response.Body.String() != test.problem || response.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
					t.Fatalf("unexpected inverted range response: %s", response.Body.String())
				}
				return
			}
			var problem struct{ Errors map[string][]string }
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil || len(problem.Errors["from"]) != 1 || problem.Errors["from"][0] != test.problem {
				t.Fatalf("unexpected date validation response: %s", response.Body.String())
			}
		})
	}
}

func TestWorkScheduleListDefaultBounds(t *testing.T) {
	local := time.FixedZone("JST", 9*60*60)
	today := time.Date(2026, 11, 22, 2, 0, 0, 0, local)
	from, to := workScheduleBounds(nil, nil, today)
	if from != time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC) || to != time.Date(2026, 12, 23, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("default bounds = %v to %v", from, to)
	}
	q := "from=2026-11-22T00%3A30%3A00%2B09%3A00&to="
	fromParam, fromError := workScheduleDate(q, "from")
	toParam, toError := workScheduleDate(q, "to")
	if fromError != "" || toError != "" || fromParam == nil || toParam != nil {
		t.Fatalf("unexpected bound parsing: %v %q, %v %q", fromParam, fromError, toParam, toError)
	}
	start, end := workScheduleBounds(fromParam, toParam, today)
	if start != time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC) || end != to {
		t.Fatalf("offset/default bounds = %v to %v", start, end)
	}
}
