package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/users"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

func TestSessionRoutesWithoutDatabase(t *testing.T) {
	const login = "/api/Auth/login"
	const logout = "/api/Auth/logout"
	const trusted = "http://127.0.0.1:5168"
	const validLogin = `{"email":"member@example.test","password":"test-only"}`
	list := NewReservationListHandler(func(context.Context) ([]reservations.ListEntry, error) {
		t.Fatal("unauthenticated request read the reservation table")
		return nil, nil
	})
	create := func(context.Context, reservations.ListEntry, bool) (reservations.ListEntry, error) {
		t.Fatal("unauthenticated request wrote a reservation")
		return reservations.ListEntry{}, nil
	}
	listWorkSchedules := func(context.Context, time.Time, time.Time) ([]workschedules.Entry, error) {
		t.Fatal("unauthenticated request read work schedules")
		return nil, nil
	}
	listParticipants := func(context.Context) ([]users.Participant, error) {
		t.Fatal("unauthenticated request read participants")
		return nil, nil
	}
	handler, err := RegisterSessionRoutes(NewHandler(nil, time.Second), auth.NewService(nil), list, create, nil, nil, nil, nil, nil, nil, listWorkSchedules, nil, nil, nil, nil, listParticipants, nil,
		SessionOptions{Development: true, FrontendOrigins: []string{trusted}})
	if err != nil {
		t.Fatal(err)
	}

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/Auth/me"}, {http.MethodPost, logout}, {http.MethodGet, "/api/Reservations"}, {http.MethodPost, "/api/Reservations"},
		{http.MethodPost, "/api/Reservations/series"},
		{http.MethodPost, "/api/Reservations/7/series-update"},
		{http.MethodPost, "/api/Reservations/7/series-delete"},
		{http.MethodGet, "/api/work-schedule-entries"}, {http.MethodPut, "/api/work-schedule-entries/7"},
		{http.MethodPut, "/api/work-schedule-entries/7/series"}, {http.MethodDelete, "/api/work-schedule-entries/7"},
		{http.MethodGet, "/api/Users/participants"},
		{http.MethodGet, "/api/Notifications/user/2"}, {http.MethodPut, "/api/Notifications/101/read"}, {http.MethodDelete, "/api/Notifications/101"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		assertUnauthorized(t, response.Result(), response.Body.Bytes())
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, login, nil))
	if response.Code != http.StatusUnsupportedMediaType || response.Header().Get("Content-Type") != "application/problem+json; charset=utf-8" {
		t.Fatalf("login without a media type: got %d, want 415 Problem Details", response.Code)
	}

	for _, test := range []struct {
		name, body string
		fields     map[string][]string
	}{
		{"missing fields", `{}`, map[string][]string{
			"Email":    {"The Email field is required.", "The Email field is not a valid e-mail address."},
			"Password": {"The Password field is required."},
		}},
		{"invalid email", `{"email":"invalid","password":"x"}`, map[string][]string{
			"Email": {"The Email field is not a valid e-mail address."},
		}},
		{"null email", `{"email":null,"password":"x"}`, map[string][]string{
			"Email": {"The Email field is required."},
		}},
		{"empty JSON body", ``, map[string][]string{
			"": {"A non-empty request body is required."}, "request": {"The request field is required."},
		}},
		{"malformed JSON", `{"email":`, map[string][]string{"request": {"A JSON request object is required."}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, login, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var problem struct {
				Status int
				Errors map[string][]string
			}
			if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/problem+json; charset=utf-8" ||
				json.Unmarshal(response.Body.Bytes(), &problem) != nil || problem.Status != 400 || !reflect.DeepEqual(problem.Errors, test.fields) {
				t.Fatalf("unexpected validation response: %d %s", response.Code, response.Body.String())
			}
		})
	}
	for _, path := range []string{login, logout, "/api/Reservations"} {
		for _, origin := range []string{"https://untrusted.example.test", "null"} {
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(validLogin))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("%s, origin %s: got %d without enforcing the trusted origin", path, origin, response.Code)
			}
		}
	}
	request := httptest.NewRequest(http.MethodPost, login, strings.NewReader(validLogin))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-site request without Origin: got %d, want 403", response.Code)
	}
	request.Header.Set("Origin", trusted)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Access-Control-Allow-Origin") != trusted ||
		response.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("trusted origin: got %d, want the handler's 503 and credentialed CORS headers", response.Code)
	}
	preflight := httptest.NewRequest(http.MethodOptions, login, nil)
	preflight.Header.Set("Origin", trusted)
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, preflight)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Headers") != "Content-Type" {
		t.Fatalf("trusted preflight: got %d and headers %v", response.Code, response.Header())
	}
	for _, change := range []struct{ method, path string }{
		{http.MethodPut, "/api/Notifications/101/read"}, {http.MethodDelete, "/api/Notifications/101"},
	} {
		preflight := httptest.NewRequest(http.MethodOptions, change.path, nil)
		preflight.Header.Set("Origin", trusted)
		preflight.Header.Set("Access-Control-Request-Method", change.method)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, preflight)
		if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != trusted ||
			response.Header().Get("Access-Control-Allow-Credentials") != "true" ||
			!strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), change.method) {
			t.Fatalf("trusted %s preflight: got %d and headers %v", change.method, response.Code, response.Header())
		}
		request := httptest.NewRequest(change.method, change.path, nil)
		request.Header.Set("Origin", "https://untrusted.example.test")
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("untrusted %s request: got %d and headers %v", change.method, response.Code, response.Header())
		}
	}
}

func TestCredentialFailureJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/Auth/login", nil)
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	writeNegotiatedString(response, request, http.StatusUnauthorized, "Invalid email or password.")
	var message string
	if response.Code != http.StatusUnauthorized || response.Header().Get("Content-Type") != "application/json; charset=utf-8" ||
		json.Unmarshal(response.Body.Bytes(), &message) != nil || message != "Invalid email or password." {
		t.Fatal("JSON credential failure did not match the C# response")
	}
}

func TestSessionCookieSecurePolicy(t *testing.T) {
	for _, test := range []struct {
		development bool
		url         string
		secure      bool
	}{
		{false, "http://127.0.0.1:8081", true},
		{true, "http://127.0.0.1:8081", false},
		{true, "https://127.0.0.1:8081", true},
	} {
		handler := sessionHandler{development: test.development}
		cookie := handler.cookie(httptest.NewRequest(http.MethodGet, test.url, nil), "synthetic")
		if cookie.Secure != test.secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" {
			t.Fatalf("unexpected cookie attributes (development=%t, URL=%s)", test.development, test.url)
		}
	}
	if _, err := RegisterSessionRoutes(http.NewServeMux(), auth.NewService(nil), http.NotFoundHandler(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		SessionOptions{FrontendOrigins: []string{"http://127.0.0.1:5168"}}); err == nil {
		t.Fatal("HTTP frontend origin accepted outside development")
	}
}
