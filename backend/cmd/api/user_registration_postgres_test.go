package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

const userRegistrationPath = "/api/Users/register"

type registrationResponse struct {
	Status                int
	ContentType, Location string
	Body                  json.RawMessage
}

type registrationCase struct {
	Name, Actor, PasswordMode, ContentType, Accept, LoginAs string
	Fields                                                  map[string]any
	Raw                                                     *string
	Response                                                registrationResponse
	Saved                                                   *auth.User
	LoginStatus, TrimmedLoginStatus                         int
}

func TestUserRegistrationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "USER_REGISTRATION_TEST_DATABASE_URL")
	secret := rand.Text()
	t.Setenv("CHATWORK_ENABLED", "")

	t.Run("compare_with_csharp", func(t *testing.T) {
		applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
		applyAuthTestSQL(t, ctx, pool, "../../migrations/0011_enable_user_registration.sql")
		var reference struct{ Cases []registrationCase }
		loadSeriesDeleteFixture(t, "user_registration_csharp.json", &reference)
		const trustedOrigin = "http://127.0.0.1:5168"
		addr, stop := startAuthTestAPI(t, ctx, dsn, trustedOrigin)
		defer stop()
		base := "http://" + addr
		actors := make(map[string]*http.Cookie)
		hashes := make(map[string]bool)
		registered := 0
		for _, scenario := range reference.Cases {
			t.Run(scenario.Name, func(t *testing.T) {
				body, password := registrationPayload(t, scenario, secret)
				wantStatus := scenario.Response.Status
				switch scenario.Name {
				case "member_allowed_by_reference":
					wantStatus = http.StatusForbidden
				case "request_over_go_auth_limit":
					wantStatus = http.StatusRequestEntityTooLarge
				}
				origin := ""
				if scenario.Name == "bootstrap" {
					res, _ := sendRegistration(t, ctx, base, body, scenario.ContentType, scenario.Accept, "https://untrusted.example.test", nil, secret)
					if res.StatusCode != http.StatusForbidden || res.Header.Get("Access-Control-Allow-Origin") != "" {
						t.Fatal("untrusted origin could reach registration")
					}
					// The trusted bootstrap below must still produce user ID 1.
					assertRegistrationCount(t, ctx, pool, 0)
					origin = trustedOrigin
				}
				res, raw := sendRegistration(t, ctx, base, body, scenario.ContentType, scenario.Accept, origin, actors[scenario.Actor], secret)
				if res.StatusCode != wantStatus {
					t.Fatalf("registration status = %d, want %d", res.StatusCode, wantStatus)
				}
				switch wantStatus {
				case http.StatusCreated:
					registered++
					if origin != "" && res.Header.Get("Access-Control-Allow-Origin") != origin {
						t.Fatal("trusted frontend did not receive its origin header")
					}
					compareRegistrationResponse(t, scenario, res, raw, base)
					user := *scenario.Saved
					assertRegisteredProfile(t, ctx, pool, user, raw, password, hashes)
					if scenario.LoginStatus != 0 {
						loginEmail := scenario.Fields["email"].(string)
						cookie := confirmRegistrationLogin(t, ctx, addr, loginEmail, password, user, scenario.LoginStatus)
						if scenario.LoginAs != "" {
							actors[scenario.LoginAs] = cookie
						}
						if scenario.TrimmedLoginStatus != 0 {
							authTestRequest(t, ctx, "http://"+addr, http.MethodPost, "/api/Auth/login",
								map[string]string{"email": loginEmail, "password": strings.TrimSpace(password)},
								nil, scenario.TrimmedLoginStatus)
						}
					}
				case http.StatusForbidden, http.StatusRequestEntityTooLarge:
					if len(raw) != 0 || res.Header.Get("Location") != "" {
						t.Fatal("denied registration unexpectedly returned data")
					}
				default:
					compareRegistrationResponse(t, scenario, res, raw, base)
				}
				assertRegistrationCount(t, ctx, pool, registered)
			})
		}
	})

	t.Run("concurrent_requests", func(t *testing.T) {
		pool, dsn := newAPITestDatabase(t, ctx, "USER_REGISTRATION_TEST_DATABASE_URL")
		applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
		applyAuthTestSQL(t, ctx, pool, "../../migrations/0011_enable_user_registration.sql")
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		base := "http://" + addr
		assertRegistrationRace(t, ctx, pool, base, secret, nil,
			[]string{"concurrent-a@example.test", "concurrent-b@example.test"}, []int{http.StatusCreated, http.StatusForbidden})
		assertRegistrationCount(t, ctx, pool, 1)
		var admin auth.User
		if err := pool.QueryRow(ctx, `
			SELECT id,name,email,avatar_color,is_admin,chatwork_account_id,chatwork_direct_room_id
			FROM users WHERE is_admin`).Scan(&admin.ID, &admin.Name, &admin.Email, &admin.AvatarColor, &admin.IsAdmin,
			&admin.ChatworkAccountID, &admin.ChatworkDirectRoomID); err != nil {
			t.Fatal("concurrent bootstrap did not create an administrator")
		}
		cookie := confirmRegistrationLogin(t, ctx, addr, admin.Email, secret, admin, http.StatusOK)
		assertRegistrationRace(t, ctx, pool, base, secret, cookie,
			[]string{"concurrent@example.test", "  CONCURRENT@EXAMPLE.TEST  "}, []int{http.StatusCreated, http.StatusConflict})
		assertRegistrationCount(t, ctx, pool, 2)
		var members int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE email='concurrent@example.test' AND NOT is_admin").Scan(&members); err != nil || members != 1 {
			t.Fatal("concurrent duplicates did not leave exactly one normalized general user")
		}
	})

	t.Run("migration_with_explicit_ids", func(t *testing.T) {
		pool, dsn := newAPITestDatabase(t, ctx, "USER_REGISTRATION_TEST_DATABASE_URL")
		applyAuthTestSQL(t, ctx, pool, "../../migrations/0002_create_users_and_sessions.sql")
		passwords := seedAuthTestUsers(t, ctx, pool)
		if _, err := pool.Exec(ctx, "UPDATE users SET id=4200 WHERE email='admin@example.test'"); err != nil {
			t.Fatal("could not seed pre-migration administrator ID")
		}
		applyAuthTestSQL(t, ctx, pool, "../../migrations/0011_enable_user_registration.sql")
		if _, err := pool.Exec(ctx, "INSERT INTO users (id,name,email) VALUES (19,'Synthetic imported user','imported@example.test')"); err != nil {
			t.Fatal("identity migration did not accept an explicit ID")
		}
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		res, _ := authTestRequest(t, ctx, "http://"+addr, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": "admin@example.test", "password": passwords["admin"]}, nil, http.StatusOK)
		cookie := authTestCookie(t, res, false)
		payload := newRegistrationPayload(t, "after-import@example.test", secret)
		res, raw := sendRegistration(t, ctx, "http://"+addr, payload, "application/json", "application/json", "", cookie, secret)
		want := auth.User{ID: 4201, Name: "Synthetic concurrent user", Email: "after-import@example.test", AvatarColor: "#58a6ff"}
		if res.StatusCode != http.StatusCreated || res.Header.Get("Location") != "http://"+addr+"/api/Users?id=4201" {
			t.Fatal("first generated ID did not follow imported IDs")
		}
		assertRegisteredProfile(t, ctx, pool, want, raw, secret, make(map[string]bool))
		rows, err := pool.Query(ctx, "SELECT id FROM users ORDER BY id")
		if err != nil {
			t.Fatal("could not read users after applying identity migration")
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				t.Fatal("could not read migrated user ID")
			}
			ids = append(ids, id)
		}
		if rows.Err() != nil || !slices.Equal(ids, []int{2, 19, 4200, 4201}) {
			t.Fatalf("user IDs after migration = %v", ids)
		}
	})
}

func registrationPayload(t *testing.T, scenario registrationCase, secret string) ([]byte, string) {
	t.Helper()
	if scenario.Raw != nil {
		return []byte(*scenario.Raw), ""
	}
	var password string
	switch scenario.PasswordMode {
	case "omit":
	case "null":
		scenario.Fields["password"] = nil
	default:
		password = secret
		switch scenario.PasswordMode {
		case "blank":
			password = "  "
		case "padded":
			password = "  " + secret + "  "
		case "oversize":
			password = strings.Repeat(secret, 2700)
		}
		scenario.Fields["password"] = password
	}
	body, err := json.Marshal(scenario.Fields)
	// Leave the capture object free of credential material.
	delete(scenario.Fields, "password")
	if err != nil {
		t.Fatal("could not encode synthetic registration")
	}
	return body, password
}

func sendRegistration(t *testing.T, ctx context.Context, base string, body []byte, mediaType, accept, origin string, cookie *http.Cookie,
	secret string,
) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+userRegistrationPath, bytes.NewReader(body))
	if err != nil {
		t.Fatal("could not construct registration request")
	}
	req.Header.Set("Content-Type", mediaType)
	req.Header.Set("Accept", accept)
	if origin != "" {
		req.Header.Set("Origin", origin)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("registration HTTP request failed")
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal("could not read registration response")
	}
	assertNoRegistrationSecret(t, raw, secret)
	return res, raw
}

func assertNoRegistrationSecret(t *testing.T, raw []byte, secret string) {
	t.Helper()
	if secret != "" && bytes.Contains(raw, []byte(secret)) {
		t.Fatal("registration response contained credential material")
	}
}

func compareRegistrationResponse(t *testing.T, scenario registrationCase, res *http.Response, raw []byte, base string) {
	t.Helper()
	expected := scenario.Response
	location := expected.Location
	if location != "" {
		location = base + location
	}
	if res.Header.Get("Content-Type") != expected.ContentType || res.Header.Get("Location") != location {
		t.Fatalf("registration response headers differ in %s", scenario.Name)
	}
	if !strings.Contains(expected.ContentType, "json") {
		var text string
		if json.Unmarshal(expected.Body, &text) != nil || string(raw) != text {
			t.Fatal("registration text response differs")
		}
		return
	}
	var actual, captured any
	if json.Unmarshal(raw, &actual) != nil || json.Unmarshal(expected.Body, &captured) != nil {
		t.Fatal("unexpected registration JSON")
	}
	if strings.Contains(scenario.Name, "malformed") {
		problem := captured.(map[string]any)
		problem["errors"] = map[string]any{"request": []any{"A JSON request object is required."}}
	}
	if !reflect.DeepEqual(actual, captured) {
		t.Fatalf("registration JSON differs in %s", scenario.Name)
	}
}

func assertRegisteredProfile(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want auth.User, raw []byte,
	password string, hashes map[string]bool,
) {
	t.Helper()
	var body, saved auth.User
	if json.Unmarshal(raw, &body) != nil || !reflect.DeepEqual(body, want) {
		t.Fatal("registered user profile differs")
	}
	var hash string
	err := pool.QueryRow(ctx, `
		SELECT id,name,email,avatar_color,is_admin,chatwork_account_id,chatwork_direct_room_id,password_hash
		FROM users WHERE id=$1`, body.ID).
		Scan(&saved.ID, &saved.Name, &saved.Email, &saved.AvatarColor, &saved.IsAdmin,
			&saved.ChatworkAccountID, &saved.ChatworkDirectRoomID, &hash)
	if err != nil || !reflect.DeepEqual(saved, want) {
		t.Fatal("stored registration profile differs")
	}
	if hash == "" || hash == password || hashes[hash] || bytes.Contains(raw, []byte(hash)) {
		t.Fatal("registration did not store a distinct private password hash")
	}
	hashes[hash] = true
}

func confirmRegistrationLogin(t *testing.T, ctx context.Context, addr, email, password string, want auth.User, status int) *http.Cookie {
	t.Helper()
	res, raw := authTestRequest(t, ctx, "http://"+addr, http.MethodPost, "/api/Auth/login",
		map[string]string{"email": email, "password": password}, nil, status)
	assertNoRegistrationSecret(t, raw, password)
	var actual auth.User
	if json.Unmarshal(raw, &actual) != nil || !reflect.DeepEqual(actual, want) {
		t.Fatal("login after registration returned a different user")
	}
	return authTestCookie(t, res, false)
}

func assertRegistrationCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != want {
		t.Fatalf("registered user count = %d, want %d", count, want)
	}
}

func newRegistrationPayload(t *testing.T, email, secret string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"name": "Synthetic concurrent user", "email": email, "password": secret,
	})
	if err != nil {
		t.Fatal("could not encode synthetic user")
	}
	return body
}

func assertRegistrationRace(t *testing.T, ctx context.Context, pool *pgxpool.Pool, base, secret string, cookie *http.Cookie,
	emails []string, expected []int,
) {
	t.Helper()
	const lock = "LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE"
	blocker := beginSeriesEditBlocker(t, ctx, pool)
	if _, err := blocker.Exec(ctx, lock); err != nil {
		t.Fatal("could not hold registration lock for concurrency test")
	}
	pending := make([]<-chan seriesEditResult, 0, len(emails))
	for _, email := range emails {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+userRegistrationPath,
			bytes.NewReader(newRegistrationPayload(t, email, secret)))
		if err != nil {
			t.Fatal("could not construct concurrent registration")
		}
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		pending = append(pending, startSeriesEditRequest(req))
	}
	waitForSeriesEditLocks(t, ctx, pool, len(pending), `SELECT count(*) FROM pg_stat_activity
		WHERE datname=current_database() AND wait_event_type='Lock' AND query=$1`, lock)
	releaseSeriesEditBlocker(t, ctx, blocker)
	var statuses []int
	for _, call := range pending {
		result := <-call
		if result.err != nil {
			t.Fatal("concurrent registration HTTP request failed")
		}
		statuses = append(statuses, result.status)
	}
	slices.Sort(statuses)
	if !slices.Equal(statuses, expected) {
		t.Fatalf("concurrent registration statuses = %v, want %v", statuses, expected)
	}
}
