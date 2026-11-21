package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

func TestReservationCreateMatchesCSharp(t *testing.T) {
	var reference map[string]struct {
		Request  json.RawMessage
		Body     json.RawMessage
		Location string
		Status   int
		Query    string
	}
	data, err := os.ReadFile("testdata/reservation_create_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	user := auth.User{ID: 2, Name: "架空の予約者"}
	for name, mutate := range map[string]func(map[string]any){
		"missing_name":      func(fields map[string]any) { delete(fields, "name") },
		"invalid_room":      func(fields map[string]any) { fields["room"] = " " },
		"invalid_count":     func(fields map[string]any) { fields["numberOfPeople"] = 21 },
		"invalid_time":      func(fields map[string]any) { fields["startTime"] = "2026-06-18T12:00:00" },
		"null_participants": func(fields map[string]any) { fields["participantIds"] = nil },
		"invalid_query":     nil,
		"empty_query":       nil,
		"created":           nil,
		"adjacent":          nil,
		"overlap_allowed":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			want := reference[name]
			payload := want.Request
			if payload == nil {
				var fields map[string]any
				if err := json.Unmarshal(reference["conflict"].Request, &fields); err != nil {
					t.Fatal(err)
				}
				if mutate != nil {
					mutate(fields)
				}
				payload, err = json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
			}
			request := httptest.NewRequest(http.MethodPost, "http://example.test/api/Reservations"+want.Query, bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			newReservationCreateHandler(func(_ context.Context, entry reservations.ListEntry, allowOverlap bool) (reservations.ListEntry, error) {
				if want.Status != http.StatusCreated {
					t.Fatal("invalid request attempted a database write")
				}
				if allowOverlap != (name == "overlap_allowed") {
					t.Errorf("unexpected overlap override: %t", allowOverlap)
				}
				var referenceBody struct{ ID int }
				if err := json.Unmarshal(want.Body, &referenceBody); err != nil {
					t.Fatal(err)
				}
				entry.ID = referenceBody.ID
				return entry, nil
			})(response, request, user)
			if response.Code != want.Status {
				t.Fatalf("status %d, want %d; body %s", response.Code, want.Status, response.Body.Bytes())
			}
			if want.Location != "" && response.Header().Get("Location") != "http://example.test"+want.Location {
				t.Errorf("unexpected Location %q", response.Header().Get("Location"))
			}
			var gotBody, wantBody any
			if err := json.Unmarshal(want.Body, &wantBody); err != nil {
				t.Fatal(err)
			}
			if response.Header().Get("Content-Type") == "text/plain; charset=utf-8" {
				gotBody = response.Body.String()
			} else if err := json.Unmarshal(response.Body.Bytes(), &gotBody); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotBody, wantBody) {
				t.Errorf("body %v, want %v", gotBody, wantBody)
			}
		})
	}
}
