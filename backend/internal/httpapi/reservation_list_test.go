package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

func TestReservationList(t *testing.T) {
	t.Run("anonymous user does not read reservations", func(t *testing.T) {
		mux := reservationListMux(func(context.Context) ([]reservations.ListEntry, error) {
			t.Fatal("anonymous request read reservations")
			return nil, nil
		})
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/Reservations", nil))
		assertUnauthorized(t, response.Result(), response.Body.Bytes())
	})

	t.Run("empty list", func(t *testing.T) {
		response := authenticatedReservationList(func(context.Context) ([]reservations.ListEntry, error) {
			return nil, nil
		})
		assertReservationList(t, response.Result(), response.Body.Bytes(), nil)
	})

	t.Run("C# response fields", func(t *testing.T) {
		response := authenticatedReservationList(func(context.Context) ([]reservations.ListEntry, error) {
			return []reservations.ListEntry{
				{
					Reservation: reservations.Reservation{
						Room: "会議室 A", Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
						StartTime: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
						EndTime:   time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC),
					},
					ID: 42, UserID: 1, Name: "架空の主催者", NumberOfPeople: 1,
					Type: "社内", Purpose: "通常予約", RepeatType: "しない",
				},
				{
					Reservation: reservations.Reservation{
						Room: "会議室 B", Date: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
						StartTime: time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC),
						EndTime:   time.Date(2026, 6, 2, 10, 45, 0, 0, time.UTC),
					},
					ID: 7, UserID: 2, Name: "架空の利用者", NumberOfPeople: 3,
					Type: "来客", Purpose: "定例訪問", SeriesID: new("synthetic-weekly-series"),
					ParticipantIDs: []int{29, 5}, Participants: "参加者 29、参加者 5", RepeatType: "毎週",
					RepeatUntil: new(time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)),
				},
				{
					Reservation: reservations.Reservation{
						Room: "会議室 C", Date: time.Date(2026, 6, 10, 0, 0, 0, 100, time.UTC),
						StartTime: time.Date(2026, 6, 10, 8, 0, 0, 123456700, time.UTC),
						EndTime:   time.Date(2026, 6, 10, 9, 0, 0, 340000000, time.UTC),
					},
					ID: 53, UserID: 1, Name: "架空の主催者", NumberOfPeople: 2,
					Type: "社内", Purpose: "旧形式の繰り返し", RepeatType: "隔週",
					RepeatUntil: new(time.Date(2026, 6, 24, 0, 0, 0, 100, time.UTC)),
				},
			}, nil
		})
		want := csharpReservationList(t)
		delete(want, 8) // The second occurrence is exercised by the PostgreSQL test.
		assertReservationList(t, response.Result(), response.Body.Bytes(), want)
	})

	t.Run("database error", func(t *testing.T) {
		response := authenticatedReservationList(func(context.Context) ([]reservations.ListEntry, error) {
			return nil, errors.New("database unavailable")
		})
		if response.Code != http.StatusInternalServerError || response.Body.Len() != 0 {
			t.Fatalf("response = %d %q, want 500 with an empty body", response.Code, response.Body.String())
		}
	})
}

func reservationListMux(list func(context.Context) ([]reservations.ListEntry, error)) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle(ReservationListPattern, NewReservationListHandler(list))
	return mux
}

func authenticatedReservationList(list func(context.Context) ([]reservations.ListEntry, error)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/Reservations", nil)
	request = request.WithContext(WithAuthenticatedUser(request.Context(), 2))
	response := httptest.NewRecorder()
	reservationListMux(list).ServeHTTP(response, request)
	return response
}

func assertUnauthorized(t *testing.T, response *http.Response, body []byte) {
	t.Helper()
	if response.StatusCode != http.StatusUnauthorized || len(body) != 0 || response.Header.Get("Location") != "" {
		t.Fatalf("response = %d %q with Location %q, want 401 with an empty body and no redirect",
			response.StatusCode, body, response.Header.Get("Location"))
	}
}

func assertReservationList(t *testing.T, response *http.Response, body []byte, want map[int]map[string]any) {
	t.Helper()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("response = %d %q %q, want 200 application/json; charset=utf-8",
			response.StatusCode, response.Header.Get("Content-Type"), body)
	}
	got := reservationsByID(t, body)
	if len(got) != len(want) {
		t.Errorf("received %d reservations, want %d", len(got), len(want))
	}
	for id, reservation := range want {
		if !reflect.DeepEqual(got[id], reservation) {
			t.Errorf("reservation %d:\ngot  %v\nwant %v", id, got[id], reservation)
		}
	}
}

func csharpReservationList(t *testing.T) map[int]map[string]any {
	t.Helper()
	body, err := os.ReadFile("testdata/reservation_list_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	return reservationsByID(t, body)
}

func reservationsByID(t *testing.T, body []byte) map[int]map[string]any {
	t.Helper()
	var entries []map[string]any
	if err := json.Unmarshal(body, &entries); err != nil || entries == nil {
		t.Fatalf("expected a JSON reservation array, got %q (%v)", body, err)
	}
	byID := make(map[int]map[string]any, len(entries))
	for _, entry := range entries {
		id, ok := entry["id"].(float64)
		if !ok || id != float64(int(id)) {
			t.Fatalf("reservation has invalid ID: %v", entry)
		}
		if _, exists := byID[int(id)]; exists {
			t.Fatalf("duplicate reservation ID %v", id)
		}
		byID[int(id)] = entry
	}
	return byID
}
