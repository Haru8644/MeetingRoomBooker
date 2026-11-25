package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const adminUsersTestURL = "ADMIN_USERS_TEST_DATABASE_URL"

type adminUserCase struct {
	Name, Actor, Method, Path, Accept string
	Raw                               *string
	Response                          registrationResponse
	Saved                             map[string]json.RawMessage
}

func TestAdminUsersPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, adminUsersTestURL)
	applyAdminUsersMigrations(t, ctx, pool)
	var reference struct {
		Cases         []adminUserCase
		ChatworkCalls int32
	}
	loadSeriesDeleteFixture(t, "admin_users_csharp.json", &reference)

	var chatworkCalls atomic.Int32
	fakeChatwork := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chatworkCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer fakeChatwork.Close()
	t.Setenv("CHATWORK_ENABLED", "true")
	t.Setenv("CHATWORK_API_TOKEN", fakeChatworkToken)
	t.Setenv("CHATWORK_API_BASE_URL", fakeChatwork.URL+"/v2/")

	const frontend = "http://127.0.0.1:5168"
	addr, stop := startAuthTestAPI(t, ctx, dsn, frontend)
	defer stop()
	base := "http://" + addr
	actors, locations := registerAdminUsers(t, ctx, pool, base)
	seedAdminUserDependencies(t, ctx, pool)

	for _, path := range []string{"/api/Users/2/name", "/api/Users/2/chatwork-account", "/api/Users/2/chatwork-direct-room", "/api/Users/2"} {
		method := http.MethodPut
		if path == "/api/Users/2" {
			method = http.MethodDelete
		}
		req := adminUserRequest(t, ctx, method, base+path, new("{}"), actors["admin"])
		req.Header.Set("Origin", "https://untrusted.example.test")
		res, _ := sendAdminUserRequest(t, req)
		if res.StatusCode != http.StatusForbidden || res.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("untrusted origin reached %s %s", method, path)
		}
	}

	for _, scenario := range reference.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			target := base + scenario.Path
			if scenario.Name == "registration_location" {
				target = locations[2]
			}
			if scenario.Name == "delete_demoted_self" {
				execAdminUsersSQL(t, ctx, pool, `UPDATE users SET is_admin = false WHERE id = 4`)
				defer execAdminUsersSQL(t, ctx, pool, `UPDATE users SET is_admin = true WHERE id = 4`)
			}
			req := adminUserRequest(t, ctx, scenario.Method, target, scenario.Raw, actors[scenario.Actor])
			req.Header.Set("Accept", scenario.Accept)
			req.Header.Set("Origin", frontend)
			res, raw := sendAdminUserRequest(t, req)
			if res.Header.Get("Access-Control-Allow-Origin") != frontend {
				t.Fatal("trusted frontend is missing CORS access")
			}
			compareAdminUserResponse(t, scenario, res, raw)
			if scenario.Saved != nil {
				saved := adminUserDependencies(t, ctx, pool)
				for key, value := range saved {
					if !equalAdminUserJSON(value, scenario.Saved[key]) {
						t.Errorf("%s differed from the SQLite rows", key)
					}
				}
			}
		})
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id = 2`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("deleted user's sessions were not removed")
	}
	if got := chatworkCalls.Load(); got != reference.ChatworkCalls || got != 0 {
		t.Fatalf("fake Chatwork received %d calls, reference received %d", got, reference.ChatworkCalls)
	}
}

func TestAdminUserMutationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, adminUsersTestURL)
	applyAdminUsersMigrations(t, ctx, pool)
	t.Setenv("CHATWORK_ENABLED", "")
	addr, stop := startAuthTestAPI(t, ctx, dsn)
	defer stop()
	base := "http://" + addr
	actors, _ := registerAdminUsers(t, ctx, pool, base)
	seedAdminUserDependencies(t, ctx, pool)
	initial := adminUserDependencies(t, ctx, pool)
	_, before := authTestRequest(t, ctx, base, http.MethodGet, "/api/Users", nil, actors["admin"], http.StatusOK)
	rename := func() *http.Request {
		return adminUserRequest(t, ctx, http.MethodPut, base+"/api/Users/2/name", new(`{"name":"After the lock"}`), actors["admin"])
	}
	remove := func() *http.Request {
		return adminUserRequest(t, ctx, http.MethodDelete, base+"/api/Users/2", nil, actors["admin"])
	}

	t.Run("rollback", func(t *testing.T) {
		execAdminUsersSQL(t, ctx, pool, `
			ALTER TABLE reservations ADD CONSTRAINT synthetic_rename_failure CHECK (id <> 11 OR name <> 'After the lock');
			CREATE FUNCTION reject_admin_user_delete() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				RAISE EXCEPTION 'synthetic failure after related rows were changed';
			END $$;
			CREATE TRIGGER reject_admin_user_delete BEFORE DELETE ON users
				FOR EACH ROW WHEN (OLD.id = 2) EXECUTE FUNCTION reject_admin_user_delete()`)
		for _, req := range []*http.Request{rename(), remove()} {
			res, _ := sendAdminUserRequest(t, req)
			if res.StatusCode != http.StatusInternalServerError {
				t.Fatalf("%s with an injected database failure returned %d", req.Method, res.StatusCode)
			}
			_, users := authTestRequest(t, ctx, base, http.MethodGet, "/api/Users", nil, actors["admin"], http.StatusOK)
			assertAdminUserJSON(t, users, before)
			for key, value := range adminUserDependencies(t, ctx, pool) {
				assertAdminUserJSON(t, value, initial[key])
			}
			authTestRequest(t, ctx, base, http.MethodGet, "/api/Auth/me", nil, actors["target"], http.StatusOK)
		}
		execAdminUsersSQL(t, ctx, pool, `ALTER TABLE reservations DROP CONSTRAINT synthetic_rename_failure;
			DROP TRIGGER reject_admin_user_delete ON users`)
	})

	t.Run("concurrent_rename_then_delete", func(t *testing.T) {
		execAdminUsersSQL(t, ctx, pool, `
			CREATE FUNCTION pause_admin_user_write() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				PERFORM pg_advisory_xact_lock(95011);
				IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
				RETURN NEW;
			END $$;
			CREATE TRIGGER pause_admin_user_rename BEFORE UPDATE ON users
				FOR EACH ROW WHEN (OLD.id = 2) EXECUTE FUNCTION pause_admin_user_write()`)
		assertAdminUserMutationOrder(t, ctx, pool, rename(), remove(), "UPDATE users SET name", http.StatusNoContent)
		execAdminUsersSQL(t, ctx, pool, `DROP TRIGGER pause_admin_user_rename ON users`)
	})

	t.Run("concurrent_delete_then_rename", func(t *testing.T) {
		execAdminUsersSQL(t, ctx, pool, `INSERT INTO users (id, name, email) VALUES (2, 'Synthetic target', 'target@example.test');
			CREATE TRIGGER pause_admin_user_delete BEFORE DELETE ON users
				FOR EACH ROW WHEN (OLD.id = 2) EXECUTE FUNCTION pause_admin_user_write()`)
		assertAdminUserMutationOrder(t, ctx, pool, remove(), rename(), "DELETE FROM users", http.StatusNotFound)
	})
	var surviving int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = 2`).Scan(&surviving); err != nil || surviving != 0 {
		t.Fatal("concurrent mutations left the target user")
	}
	authTestRequest(t, ctx, base, http.MethodGet, "/api/Auth/me", nil, actors["target"], http.StatusUnauthorized)
}

func assertAdminUserMutationOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, first, second *http.Request, firstSQL string, secondStatus int) {
	t.Helper()
	blocker := beginSeriesEditBlocker(t, ctx, pool)
	defer blocker.Rollback(ctx)
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_xact_lock(95011)`); err != nil {
		t.Fatal(err)
	}
	firstResult := startSeriesEditRequest(first)
	const waiting = `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
		AND wait_event_type = 'Lock' AND query LIKE $1`
	waitForSeriesEditLocks(t, ctx, pool, 1, waiting, "%"+firstSQL+"%")
	secondResult := startSeriesEditRequest(second)
	waitForSeriesEditLocks(t, ctx, pool, 1, waiting, "%FROM users WHERE id = $1 FOR UPDATE%")
	releaseSeriesEditBlocker(t, ctx, blocker)
	assertSeriesEditResult(t, firstResult, http.StatusNoContent)
	assertSeriesEditResult(t, secondResult, secondStatus)
}

func compareAdminUserResponse(t *testing.T, scenario adminUserCase, res *http.Response, raw []byte) {
	t.Helper()
	want := scenario.Response
	switch scenario.Name {
	case "delete_demoted_self":
		want = registrationResponse{Status: http.StatusForbidden, Body: json.RawMessage(`""`)}
	case "deleted_participants", "deleted_me":
		want = registrationResponse{Status: http.StatusUnauthorized, Body: json.RawMessage(`""`)}
	}
	contentType, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if res.StatusCode != want.Status || contentType != want.ContentType || res.Header.Get("Location") != want.Location {
		t.Fatalf("response status, media type or location differed from expected (%d %q)", res.StatusCode, contentType)
	}
	if strings.HasSuffix(scenario.Name, "_malformed_before_lookup") || strings.HasSuffix(scenario.Name, "_wrong_type_before_lookup") {
		var problem map[string]json.RawMessage
		if err := json.Unmarshal(raw, &problem); err != nil {
			t.Fatal("invalid validation response")
		}
		if string(problem["errors"]) != `{"request":["A JSON request object is required."]}` {
			t.Fatal("unexpected generic validation error")
		}
		delete(problem, "errors")
		raw, _ = json.Marshal(problem)
		var referenceProblem map[string]json.RawMessage
		if err := json.Unmarshal(want.Body, &referenceProblem); err != nil {
			t.Fatal("invalid reference validation response")
		}
		delete(referenceProblem, "errors")
		want.Body, _ = json.Marshal(referenceProblem)
	}
	if strings.Contains(contentType, "json") {
		assertAdminUserJSON(t, raw, want.Body)
		return
	}
	var body string
	if err := json.Unmarshal(want.Body, &body); err != nil || string(raw) != body {
		t.Fatal("response text differed from expected")
	}
}

func assertAdminUserJSON(t *testing.T, actual, expected []byte) {
	t.Helper()
	if !equalAdminUserJSON(actual, expected) {
		t.Fatal("JSON differed from the expected response or saved rows")
	}
}

func equalAdminUserJSON(actual, expected []byte) bool {
	var got, want any
	return json.Unmarshal(actual, &got) == nil && json.Unmarshal(expected, &want) == nil && reflect.DeepEqual(got, want)
}

func registerAdminUsers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, base string) (map[string]*http.Cookie, map[int]string) {
	t.Helper()
	secret := rand.Text()
	actors := make(map[string]*http.Cookie)
	locations := make(map[int]string)
	for i, role := range []string{"admin", "target", "member", "other-admin"} {
		id := i + 1
		email := role + "@example.test"
		res, _ := authTestRequest(t, ctx, base, http.MethodPost, userRegistrationPath, map[string]string{
			"name": "Synthetic " + role, "email": email, "password": secret,
			"chatworkAccountId": strconv.Itoa(10 + id), "chatworkDirectRoomId": strconv.Itoa(20 + id),
		}, actors["admin"], http.StatusCreated)
		locations[id] = res.Header.Get("Location")
		if locations[id] != base+"/api/Users?id="+strconv.Itoa(id) {
			t.Fatal("registration returned an unexpected Location")
		}
		if role == "other-admin" {
			execAdminUsersSQL(t, ctx, pool, `UPDATE users SET is_admin = true WHERE id = 4`)
		}
		login, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": email, "password": secret}, nil, http.StatusOK)
		actors[role] = authTestCookie(t, login, false)
	}
	return actors, locations
}

func adminUserRequest(t *testing.T, ctx context.Context, method, url string, raw *string, cookie *http.Cookie) *http.Request {
	t.Helper()
	var body io.Reader
	if raw != nil {
		body = strings.NewReader(*raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

func sendAdminUserRequest(t *testing.T, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, raw
}

func applyAdminUsersMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0011_enable_user_registration.sql")
}

func execAdminUsersSQL(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
}

func seedAdminUserDependencies(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	execAdminUsersSQL(t, ctx, pool, `
		INSERT INTO reservations
			(id,user_id,name,room,number_of_people,type,purpose,date_ticks,start_time_ticks,end_time_ticks,participant_ids,participants,repeat_type)
		SELECT r.id,u.id,u.name,'Room A',cardinality(r.ids),'Internal','Synthetic booking',
			639173376000000000,639173808000000000,639173844000000000,r.ids,'Stored display '||r.id,'なし'
		FROM (VALUES (10,2,ARRAY[2,3]),(11,2,ARRAY[2]),(20,3,ARRAY[2,3,2,4,3,0,-1,4]),
		             (21,3,ARRAY[2,2]),(22,3,ARRAY[3,3])) r(id,owner,ids) JOIN users u ON u.id=r.owner;
		INSERT INTO notifications (id,user_id,message,created_at_ticks,target_date_ticks,type,target_reservation_id,target_work_schedule_entry_id)
		SELECT id,recipient,'Synthetic notice',639173808000000000,639173808000000000,'Info',reservation,schedule
		FROM (VALUES (100,3,10,NULL),(101,2,11,NULL),(102,2,NULL,30),(103,3,20,NULL),
		             (104,3,NULL,30),(105,4,NULL,NULL)) n(id,recipient,reservation,schedule);
		INSERT INTO work_schedule_entries
			(id,created_by_user_id,type,title,date_ticks,leave_period,participant_ids,participants,created_at_ticks,updated_at_ticks)
		VALUES (30,2,0,'Synthetic schedule',639173808000000000,0,ARRAY[2,3],'Stored schedule',639173808000000000,639173808000000000);
		INSERT INTO chatwork_delivery_logs
			(id,reservation_id,delivery_type,delivery_key,target_user_id,scheduled_start_time_ticks,room_id,status,attempted_at_ticks,message,created_at_ticks)
		VALUES (40,10,'ReservationCreated','synthetic-key',2,639173808000000000,'22','Sent',639173808000000000,'Synthetic delivery',639173808000000000)`)
}

func adminUserDependencies(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]json.RawMessage {
	t.Helper()
	var reservations, notices, schedules, deliveries json.RawMessage
	err := pool.QueryRow(ctx, `
		SELECT (SELECT coalesce(json_agg(json_build_object('id',id,'name',name,'ids',participant_ids,
		                 'count',number_of_people,'display',participants) ORDER BY id),'[]') FROM reservations),
		       (SELECT coalesce(json_agg(json_build_array(id,user_id,target_reservation_id,target_work_schedule_entry_id)
		                 ORDER BY id),'[]') FROM notifications),
		       (SELECT coalesce(json_agg(json_build_array(id,created_by_user_id,array_to_string(participant_ids,','))
		                 ORDER BY id),'[]') FROM work_schedule_entries),
		       (SELECT coalesce(json_agg(json_build_array(id,reservation_id,target_user_id,delivery_key,status)
		                 ORDER BY id),'[]') FROM chatwork_delivery_logs)`).Scan(&reservations, &notices, &schedules, &deliveries)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]json.RawMessage{"reservations": reservations, "notices": notices, "schedules": schedules, "deliveries": deliveries}
}
