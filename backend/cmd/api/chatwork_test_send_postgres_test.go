package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
)

type chatworkTestSendCase struct {
	Name, Actor, Origin, Content, Accept string
	Raw                                  *string
	Model                                json.RawMessage
	Response                             registrationResponse
	Calls                                []chatworkTestSendCall
}

type chatworkTestSendCall struct {
	Path, Message string
}

func TestChatworkTestSendPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "CHATWORK_TEST_SEND_DATABASE_URL")
	applyChatworkTestMigrations(t, ctx, pool)
	passwords := seedAuthTestUsers(t, ctx, pool)
	var reference struct {
		Groups []struct {
			Name, DefaultRoom string
			Enabled           bool
			DeliveryLogRows   int
			Cases             []chatworkTestSendCase
		}
	}
	loadSeriesDeleteFixture(t, "chatwork_test_send_csharp.json", &reference)

	const upstreamDetail = "synthetic upstream detail"
	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousOutput)
	var mu sync.Mutex
	var calls []chatworkTestSendCall
	takeCalls := func() []chatworkTestSendCall {
		mu.Lock()
		defer mu.Unlock()
		got := calls
		calls = nil
		if len(got) == 0 {
			return []chatworkTestSendCall{}
		}
		return got
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-ChatWorkToken") != fakeChatworkToken ||
			r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.ParseForm() != nil ||
			len(r.PostForm) != 1 || len(r.PostForm["body"]) != 1 {
			t.Error("invalid or unexpectedly redirected Chatwork request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		calls = append(calls, chatworkTestSendCall{Path: r.URL.Path, Message: r.PostForm.Get("body")})
		mu.Unlock()
		switch r.URL.Path {
		case "/v2/rooms/95003/messages":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(upstreamDetail + " " + fakeChatworkToken))
		case "/v2/rooms/95004/messages":
			<-r.Context().Done()
		case "/v2/rooms/95005/messages":
			w.Header().Set("Location", "/unexpected")
			w.WriteHeader(http.StatusFound)
		default:
			_, _ = w.Write([]byte(`{"message_id":"fake"}`))
		}
	}))
	defer fake.Close()

	t.Run("base_url_override", func(t *testing.T) {
		if _, err := chatwork.NewSender(pool, fakeChatworkToken, fake.URL+"/v2/", false); err == nil {
			t.Error("the loopback override should require development mode")
		}
		if _, err := chatwork.NewSender(pool, fakeChatworkToken, "https://chatwork.example.test/v2/", true); err == nil {
			t.Error("the development override should require a loopback host")
		}
	})

	for _, group := range reference.Groups {
		t.Run(group.Name, func(t *testing.T) {
			t.Setenv("CHATWORK_API_BASE_URL", fake.URL+"/v2/")
			t.Setenv("CHATWORK_ROOM_ID", group.DefaultRoom)
			t.Setenv("CHATWORK_API_TOKEN", fakeChatworkToken)
			t.Setenv("CHATWORK_ENABLED", "")
			if group.Enabled {
				t.Setenv("CHATWORK_ENABLED", "true")
			} else if group.Name == "disabled" {
				t.Setenv("CHATWORK_ENABLED", "false")
			}
			if group.Name == "missing_token" {
				t.Setenv("CHATWORK_API_TOKEN", "")
				t.Setenv("DATABASE_URL", dsn)
				t.Setenv("APP_ENV", "development")
				t.Setenv("HTTP_ADDR", "127.0.0.1:0")
				err := run(ctx)
				if err == nil || err.Error() != "Chatwork delivery requires a valid CHATWORK_API_TOKEN" {
					t.Fatal("enabled Chatwork must fail startup with a missing token")
				}
				if len(takeCalls()) != 0 {
					t.Fatal("startup with a missing token sent a request")
				}
				return
			}

			const frontend = "http://127.0.0.1:5168"
			addr, stop := startAuthTestAPI(t, ctx, dsn, frontend)
			defer stop()
			base := "http://" + addr
			actors := make(map[string]*http.Cookie)
			for _, role := range []string{"admin", "member"} {
				res, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
					map[string]string{"email": role + "@example.test", "password": passwords[role]}, nil, http.StatusOK)
				actors[role] = authTestCookie(t, res, false)
			}
			const path = "/api/Chatwork/test"
			for _, scenario := range group.Cases {
				t.Run(scenario.Name, func(t *testing.T) {
					raw := scenario.Raw
					if string(scenario.Model) != "null" {
						raw = new(string(scenario.Model))
					}
					req := adminUserRequest(t, ctx, http.MethodPost, base+path, raw, actors[scenario.Actor])
					content := scenario.Content
					if content == "" {
						content = "application/json"
					}
					req.Header.Set("Content-Type", content)
					accept := scenario.Accept
					if accept == "" {
						accept = "application/json"
					}
					req.Header.Set("Accept", accept)
					origin := frontend
					if scenario.Origin != "" {
						origin = scenario.Origin
					}
					req.Header.Set("Origin", origin)
					res, body := sendAdminUserRequest(t, req)
					gotOrigin := res.Header.Get("Access-Control-Allow-Origin")
					if (origin == frontend && gotOrigin != frontend) || (origin != frontend && gotOrigin != "") {
						t.Fatal("unexpected CORS response")
					}
					want, wantCalls := expectedChatworkTestSend(scenario, group.Enabled)
					compareChatworkTestSendResponse(t, res, body, want)
					if got := takeCalls(); !reflect.DeepEqual(got, wantCalls) {
						t.Fatalf("unexpected fake Chatwork calls: got %d, want %d, or request data differed", len(got), len(wantCalls))
					}
				})
			}
			if group.Name == "configured" {
				t.Run("redirect", func(t *testing.T) {
					req := adminUserRequest(t, ctx, http.MethodPost, base+path,
						new(`{"message":"synthetic redirect test","roomId":"95005"}`), actors["admin"])
					res, body := sendAdminUserRequest(t, req)
					compareChatworkTestSendResponse(t, res, body,
						chatworkTestFailure(http.StatusInternalServerError, "Failed to send Chatwork message. StatusCode: 302."))
					if got := takeCalls(); !reflect.DeepEqual(got, []chatworkTestSendCall{{"/v2/rooms/95005/messages", "synthetic redirect test"}}) {
						t.Fatal("the test send should make exactly one request before the redirect")
					}
				})
			}
			var rows int
			err := pool.QueryRow(ctx, "SELECT count(*) FROM chatwork_delivery_logs").Scan(&rows)
			if err != nil || rows != group.DeliveryLogRows || rows != 0 {
				t.Fatalf("unexpected delivery history after test sends: %d rows (%v)", rows, err)
			}
		})
	}
	if bytes.Contains(logs.Bytes(), []byte(upstreamDetail)) || bytes.Contains(logs.Bytes(), []byte(fakeChatworkToken)) {
		t.Fatal("provider response or token was logged")
	}
}

func expectedChatworkTestSend(scenario chatworkTestSendCase, enabled bool) (registrationResponse, []chatworkTestSendCall) {
	switch scenario.Name {
	case "untrusted_origin":
		return registrationResponse{Status: http.StatusForbidden, ContentType: "text/plain"}, []chatworkTestSendCall{}
	case "invalid_json", "non_string_room":
		return registrationResponse{Status: http.StatusBadRequest, ContentType: "application/problem+json", Body: json.RawMessage(`{
			"type":"https://tools.ietf.org/html/rfc9110#section-15.5.1",
			"title":"One or more validation errors occurred.",
			"status":400,
			"errors":{"request":["A JSON request object is required."]}
		}`)}, scenario.Calls
	case "provider_503":
		return chatworkTestFailure(http.StatusInternalServerError, "Failed to send Chatwork message. StatusCode: 503."), scenario.Calls
	case "provider_timeout":
		return chatworkTestFailure(http.StatusInternalServerError, "Chatwork request timed out."), scenario.Calls
	}
	if !enabled && scenario.Name != "disabled_invalid" {
		return chatworkTestFailure(http.StatusServiceUnavailable, "Chatwork delivery is disabled."), scenario.Calls
	}
	return scenario.Response, scenario.Calls
}

func chatworkTestFailure(status int, detail string) registrationResponse {
	raw, _ := json.Marshal(map[string]string{"message": "Chatworkへのテスト送信に失敗しました。", "detail": detail})
	return registrationResponse{Status: status, ContentType: "application/json", Body: raw}
}

func compareChatworkTestSendResponse(t *testing.T, res *http.Response, body []byte, want registrationResponse) {
	t.Helper()
	contentType, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if res.StatusCode != want.Status || contentType != want.ContentType {
		t.Fatalf("status / media type = %d / %q, want %d / %q", res.StatusCode, contentType, want.Status, want.ContentType)
	}
	if want.Body == nil {
		return // The origin middleware owns this response body.
	}
	if contentType == "application/json" || contentType == "application/problem+json" {
		assertAdminUserJSON(t, body, want.Body)
		return
	}
	var wantBody string
	if json.Unmarshal(want.Body, &wantBody) != nil || string(body) != wantBody {
		t.Fatal("plain response did not match")
	}
}
