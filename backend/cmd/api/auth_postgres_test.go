package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSessionAuthenticationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "AUTH_TEST_DATABASE_URL")

	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	credentials := seedAuthTestUsers(t, ctx, pool)
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	request := func(method, path string, payload any, cookie *http.Cookie, status int) (*http.Response, []byte) {
		t.Helper()
		return authTestRequest(t, ctx, "http://"+addr, method, path, payload, cookie, status)
	}

	const listPath = "/api/Reservations"
	const mePath = "/api/Auth/me"
	const loginPath = "/api/Auth/login"
	const logoutPath = "/api/Auth/logout"
	response, body := request("GET", listPath, nil, nil, 401)
	if len(body) != 0 || response.Header.Get("Location") != "" {
		t.Fatal("anonymous reservation list should have an empty body and no redirect")
	}
	// A request that reached the reader before this point would fail because the table is absent.
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../internal/httpapi/testdata/reservation_list_seed.sql")

	for _, email := range []string{"member@example.test", "unknown@example.test"} {
		response, body = request("POST", loginPath, map[string]any{"email": email, "password": "wrong"}, nil, 401)
		if string(body) != "Invalid email or password." || len(response.Cookies()) != 0 {
			t.Fatal("wrong password and unknown account should return the same failure without issuing a cookie")
		}
	}
	response, body = request("POST", loginPath, map[string]any{
		"email": " MEMBER@EXAMPLE.TEST ", "password": credentials["member"], "rememberMe": true,
	}, nil, 200)
	assertAuthTestUser(t, body, `{"id":2,"name":"Synthetic Member","email":"member@example.test","avatarColor":"#58a6ff","isAdmin":false,"chatworkAccountId":null,"chatworkDirectRoomId":null}`)
	member := authTestCookie(t, response, true)
	response, body = request("GET", mePath, nil, member, 200)
	assertAuthTestUser(t, body, `{"id":2,"name":"Synthetic Member","email":"member@example.test","avatarColor":"#58a6ff","isAdmin":false,"chatworkAccountId":null,"chatworkDirectRoomId":null}`)
	if len(response.Cookies()) != 0 {
		t.Fatal("a new session was refreshed before the halfway point")
	}
	_, body = request("GET", listPath, nil, member, 200)
	var reservations []json.RawMessage
	if err := json.Unmarshal(body, &reservations); err != nil || len(reservations) != 4 {
		t.Fatal("authenticated reservation list should contain the four seeded reservations")
	}

	moveAuthTestSession(t, ctx, pool, member, 6)
	response, _ = request("GET", mePath, nil, member, 200)
	if len(response.Cookies()) != 0 {
		t.Fatal("a six-day-old session was refreshed before the halfway point")
	}
	moveAuthTestSession(t, ctx, pool, member, 8)
	response, _ = request("GET", mePath, nil, member, 200)
	refreshed := authTestCookie(t, response, true)
	if refreshed.Value != member.Value {
		t.Fatal("sliding renewal should extend the same session")
	}
	response, body = request("POST", logoutPath, nil, refreshed, 204)
	cookies := response.Cookies()
	if len(body) != 0 || len(cookies) != 1 || cookies[0].Name != member.Name || cookies[0].MaxAge >= 0 || cookies[0].Value != "" {
		t.Fatal("logout should clear the session cookie and return an empty body")
	}
	_, body = request("GET", listPath, nil, member, 401)
	if len(body) != 0 {
		t.Fatal("the revoked session should receive an empty 401")
	}

	response, body = request("POST", loginPath, map[string]any{
		"email": "admin@example.test", "password": credentials["admin"],
	}, nil, 200)
	const adminJSON = `{"id":1,"name":"Synthetic Admin","email":"admin@example.test","avatarColor":"#111111","isAdmin":true,"chatworkAccountId":"12345","chatworkDirectRoomId":"54321"}`
	assertAuthTestUser(t, body, adminJSON)
	admin := authTestCookie(t, response, false)
	moveAuthTestSession(t, ctx, pool, admin, 8)
	response, body = request("GET", mePath, nil, admin, 200)
	assertAuthTestUser(t, body, adminJSON)
	authTestCookie(t, response, false)
	moveAuthTestSession(t, ctx, pool, admin, 15)
	for _, path := range []string{mePath, listPath} {
		response, body := request("GET", path, nil, admin, 401)
		if len(body) != 0 || len(response.Cookies()) != 0 {
			t.Fatal("expired session should not be accepted or renewed")
		}
	}
	t.Log("verified anonymous access, C# hashes, both roles, both cookie modes, sliding renewal, logout replay and expiry over HTTP")
}

func TestDevelopmentAPIRequiresLoopback(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("HTTP_ADDR", "0.0.0.0:8081")
	if err := run(t.Context()); err == nil || err.Error() != "APP_ENV=development requires a loopback HTTP_ADDR" {
		t.Fatalf("expected development binding to be rejected, got %v", err)
	}
}

func newAPITestDatabase(t *testing.T, ctx context.Context, envName string) (*pgxpool.Pool, string) {
	t.Helper()
	databaseURL := os.Getenv(envName)
	if databaseURL == "" {
		t.Skipf("set %s to run the PostgreSQL test", envName)
	}
	dsn, err := url.Parse(databaseURL)
	if err != nil || (dsn.Scheme != "postgres" && dsn.Scheme != "postgresql") {
		t.Fatalf("%s must be a PostgreSQL URL", envName)
	}
	schema := "api_test_" + strings.ToLower(rand.Text())
	params := dsn.Query()
	params.Set("search_path", schema)
	dsn.RawQuery = params.Encode()
	pool, err := pgxpool.New(ctx, dsn.String())
	if err != nil {
		t.Fatal("could not configure the PostgreSQL test database")
	}
	t.Cleanup(pool.Close)
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop temporary test schema: %v", err)
		}
	})
	return pool, dsn.String()
}

func seedAuthTestUsers(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	data, err := os.ReadFile("../../internal/auth/testdata/identity_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Name, Password, Hash string }
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal("could not read the synthetic Identity vectors")
	}
	passwords := make(map[string]string)
	for _, vector := range vectors {
		passwords[vector.Name] = vector.Password
		var err error
		switch vector.Name {
		case "admin":
			_, err = pool.Exec(ctx, `INSERT INTO users (id, name, email, password_hash, avatar_color, is_admin, chatwork_account_id, chatwork_direct_room_id)
				VALUES (1, 'Synthetic Admin', 'admin@example.test', $1, '#111111', true, '12345', '54321')`, vector.Hash)
		case "member":
			_, err = pool.Exec(ctx, `INSERT INTO users (id, name, email, password_hash) VALUES (2, 'Synthetic Member', 'member@example.test', $1)`, vector.Hash)
		}
		if err != nil {
			t.Fatal("could not insert a synthetic user")
		}
	}
	return passwords
}

func startAuthTestAPI(t *testing.T, ctx context.Context, dsn string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTP_ADDR", addr)
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("APP_ENV", "development")
	t.Setenv("FRONTEND_ORIGINS", "")
	serverCtx, cancel := context.WithCancel(ctx)
	serverErr := make(chan error, 1)
	go func() { serverErr <- run(serverCtx) }()
	stop := func() {
		cancel()
		if err := <-serverErr; err != nil {
			t.Errorf("Go API: %v", err)
		}
	}
	client := http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if response, err := client.Get("http://" + addr + "/healthz"); err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return addr, stop
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	t.Fatal("Go API did not become healthy")
	return "", nil
}

func authTestRequest(t *testing.T, ctx context.Context, baseURL, method, path string, payload any, cookie *http.Cookie, wantStatus int) (*http.Response, []byte) {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatal("could not encode synthetic HTTP request")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s: got %d, want %d", method, path, response.StatusCode, wantStatus)
	}
	return response, body
}

func assertAuthTestUser(t *testing.T, body []byte, wantJSON string) {
	t.Helper()
	var got, want map[string]any
	if json.Unmarshal(body, &got) != nil || json.Unmarshal([]byte(wantJSON), &want) != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("user JSON did not match the C# response fields and values")
	}
}

func authTestCookie(t *testing.T, response *http.Response, persistent bool) *http.Cookie {
	t.Helper()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatal("expected one session cookie")
	}
	cookie := cookies[0]
	if cookie.Name != "MeetingRoomBooker.GoSession" || cookie.Path != "/" || cookie.Domain != "" ||
		!cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 0 {
		t.Fatal("unexpected session cookie attributes on development HTTP")
	}
	if persistent && (time.Until(cookie.Expires)-14*24*time.Hour).Abs() > 5*time.Second {
		t.Fatal("persistent cookie does not expire in 14 days")
	}
	if !persistent && !cookie.Expires.IsZero() {
		t.Fatal("browser-session cookie should not have an expiry attribute")
	}
	return cookie
}

func moveAuthTestSession(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cookie *http.Cookie, ageInDays int) {
	t.Helper()
	rawToken, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(rawToken) != 32 {
		t.Fatal("session cookie did not contain a 32-byte token")
	}
	digest := sha256.Sum256(rawToken)
	issued := time.Now().Add(-time.Duration(ageInDays) * 24 * time.Hour)
	updated, err := pool.Exec(ctx, `UPDATE sessions SET issued_at=$2, expires_at=$3 WHERE token_hash=$1`, digest[:], issued, issued.Add(14*24*time.Hour))
	if err != nil || updated.RowsAffected() != 1 {
		t.Fatal("could not move the stored session expiry")
	}
}

func applyAuthTestSQL(t *testing.T, ctx context.Context, pool *pgxpool.Pool, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply %s: %v", path, err)
	}
}
