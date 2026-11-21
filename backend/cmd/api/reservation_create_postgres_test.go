package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSingleReservationCreatePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "RESERVATION_CREATE_TEST_DATABASE_URL")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	applyAuthTestSQL(t, ctx, pool, "../../internal/httpapi/testdata/reservation_list_seed.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0003_enable_reservation_creation.sql")
	applyReservationNotificationMigrations(t, ctx, pool)
	credentials := seedAuthTestUsers(t, ctx, pool)
	if _, err := pool.Exec(ctx, `UPDATE users SET name = '架空の予約者' WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	baseURL := "http://" + addr
	path := "/api/Reservations"

	var reference map[string]struct {
		Request json.RawMessage
		Body    json.RawMessage
		Query   string
		Status  int
	}
	data, err := os.ReadFile("../../internal/httpapi/testdata/reservation_create_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	first := reference["created"].Request
	_, body := authTestRequest(t, ctx, baseURL, http.MethodPost, path, first, nil, http.StatusUnauthorized)
	if len(body) != 0 {
		t.Fatal("unauthorized create should have an empty body")
	}
	login, _ := authTestRequest(t, ctx, baseURL, http.MethodPost, "/api/Auth/login",
		map[string]string{"email": "member@example.test", "password": credentials["member"]}, nil, http.StatusOK)
	cookie := authTestCookie(t, login, false)
	forbidden, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	forbidden.Header.Set("Content-Type", "application/json")
	forbidden.Header.Set("Origin", "https://untrusted.example.test")
	forbidden.Header.Set("Sec-Fetch-Site", "cross-site")
	forbidden.AddCookie(cookie)
	if status, err := reservationTestStatus(forbidden); err != nil || status != http.StatusForbidden {
		t.Fatalf("cross-origin create = %d, %v; want 403", status, err)
	}

	created := make(map[float64]map[string]any)
	for _, name := range []string{"created", "conflict", "adjacent", "overlap_allowed"} {
		want := reference[name]
		response, body := authTestRequest(t, ctx, baseURL, http.MethodPost, path+want.Query, want.Request, cookie, want.Status)
		if want.Status == http.StatusConflict {
			var message string
			if err := json.Unmarshal(want.Body, &message); err != nil {
				t.Fatal(err)
			}
			if string(body) != message || response.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
				t.Fatalf("conflict response = %q, want %q", body, message)
			}
			continue
		}
		id := 54 + len(created)
		if location := response.Header.Get("Location"); location != baseURL+path+"?id="+strconv.Itoa(id) {
			t.Fatalf("Location %q, want generated reservation %d", location, id)
		}
		var got, expected map[string]any
		if json.Unmarshal(body, &got) != nil || json.Unmarshal(want.Body, &expected) != nil {
			t.Fatal("expected reservation JSON objects")
		}
		expected["id"] = float64(id)
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s = %v, want %v", name, got, expected)
		}
		created[float64(id)] = got
	}

	_, body = authTestRequest(t, ctx, baseURL, http.MethodGet, path, nil, cookie, http.StatusOK)
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	byID := make(map[float64]map[string]any)
	for _, row := range list {
		byID[row["id"].(float64)] = row
	}
	for _, id := range []float64{7, 8, 42, 53} {
		if byID[id] == nil {
			t.Errorf("missing previously imported ID %v", id)
		}
	}
	for id, post := range created {
		if !reflect.DeepEqual(byID[id], post) {
			t.Errorf("list for reservation %v = %v, POST = %v", id, byID[id], post)
		}
	}
	if len(list) != 7 {
		t.Fatalf("expected four existing and three created reservations, got %d", len(list))
	}
	t.Log("201 JSON and Location, 409, adjacency and explicit overlap match the C# capture")

	assertConcurrentReservationCreate(t, ctx, pool, baseURL+path, cookie)
}

func assertConcurrentReservationCreate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, target string, cookie *http.Cookie) {
	t.Helper()
	const room, day = "大会議室", "2026-06-19"
	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reservations`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), $2::text::date - DATE '0001-01-01')`, room, day); err != nil {
		t.Fatal(err)
	}

	const payload = `{"name":"架空の予約者","room":"大会議室","date":"2026-06-19","startTime":"2026-06-19T09:00:00","endTime":"2026-06-19T10:00:00","participantIds":[3]}`
	type outcome struct {
		status int
		err    error
	}
	results := make(chan outcome, 2)
	for range 2 {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewBufferString(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(cookie)
		go func() {
			status, err := reservationTestStatus(request)
			results <- outcome{status, err}
		}()
	}

	waiting := 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted AND objsubid = 2
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND classid = hashtext($1)::oid AND objid = ($2::text::date - DATE '0001-01-01')::oid`, room, day).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if waiting != 2 {
		t.Fatalf("expected two concurrent requests waiting for the room/date lock, got %d", waiting)
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	statuses := make(map[int]int)
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		statuses[result.status]++
	}
	if statuses[http.StatusCreated] != 1 || statuses[http.StatusConflict] != 1 {
		t.Fatalf("concurrent statuses = %v, want one 201 and one 409", statuses)
	}
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reservations`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf("concurrent create added %d rows, want 1", after-before)
	}
	var notices int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications n
		JOIN reservations r ON r.id = n.target_reservation_id
		WHERE r.room = $1 AND r.date_ticks / 864000000000 = $2::text::date - DATE '0001-01-01'`,
		room, day).Scan(&notices); err != nil {
		t.Fatal(err)
	}
	if notices != 1 {
		t.Fatalf("concurrent create added %d notices, want 1", notices)
	}
	t.Logf("two requests waiting concurrently returned one 201 and one 409; stored reservations %d → %d", before, after)
}

func reservationTestStatus(request *http.Request) (int, error) {
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, response.Body)
	return response.StatusCode, err
}
