package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestParticipantListPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "PARTICIPANT_TEST_DATABASE_URL")
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	baseURL := "http://" + addr
	const path = "/api/Users/participants"

	_, body := authTestRequest(t, ctx, baseURL, http.MethodGet, path, nil, nil, http.StatusUnauthorized)
	if len(body) != 0 {
		t.Fatal("anonymous participant list should have an empty body")
	}

	applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
	// Insert the largest ID first so the expected order differs from insertion and name order.
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, name, email, avatar_color)
		VALUES (17, 'A 架空の参加者', 'guest@example.test', '#22aa66')`); err != nil {
		t.Fatal(err)
	}
	passwords := seedAuthTestUsers(t, ctx, pool)
	data, err := os.ReadFile("../../internal/httpapi/testdata/participant_list_csharp.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected []map[string]any
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}

	for _, account := range []string{"member", "admin"} {
		t.Run(account, func(t *testing.T) {
			login, _ := authTestRequest(t, ctx, baseURL, http.MethodPost, "/api/Auth/login",
				map[string]string{"email": account + "@example.test", "password": passwords[account]}, nil, http.StatusOK)
			cookie := authTestCookie(t, login, false)
			response, body := authTestRequest(t, ctx, baseURL, http.MethodGet, path, nil, cookie, http.StatusOK)
			if contentType := response.Header.Get("Content-Type"); contentType != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want application/json; charset=utf-8", contentType)
			}
			var actual []map[string]any
			if err := json.Unmarshal(body, &actual); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("Go response %v, expected C# response %v", actual, expected)
			}
		})
	}
}
