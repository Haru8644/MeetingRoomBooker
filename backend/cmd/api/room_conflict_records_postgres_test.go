package main

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/roomconflicts"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const roomConflictAPI = "/api/room-conflict-records"

type roomConflictCase struct {
	Name, Actor, Method, Path, Accept, Generated string
	Raw                                          *string
	Model, Saved                                 json.RawMessage
	Response                                     registrationResponse
}

func TestRoomConflictRecordsPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "ROOM_CONFLICT_RECORDS_TEST_DATABASE_URL")
	applyAdminUsersMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0012_create_room_conflict_records.sql")
	passwords := seedAuthTestUsers(t, ctx, pool)
	execAdminUsersSQL(t, ctx, pool, `INSERT INTO users (id,name,email,password_hash)
		SELECT 3,'Synthetic Third Party','third_party@example.test',password_hash FROM users WHERE id=2`)
	var reference struct {
		Seed  []roomconflicts.Record
		Cases []roomConflictCase
	}
	loadSeriesDeleteFixture(t, "room_conflict_records_csharp.json", &reference)

	t.Setenv("CHATWORK_ENABLED", "false")
	const frontend = "http://127.0.0.1:15173"
	addr, stop := startAuthTestAPI(t, ctx, dsn, frontend)
	defer stop()
	base := "http://" + addr
	actors := make(map[string]*http.Cookie)
	for role, account := range map[string]string{"admin": "admin", "reporter": "member", "third_party": "third_party"} {
		password := passwords["member"]
		if role == "admin" {
			password = passwords["admin"]
		}
		res, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": account + "@example.test", "password": password}, nil, http.StatusOK)
		actors[role] = authTestCookie(t, res, false)
	}

	var location string
	generated := make(map[int]map[string]string)
	for _, scenario := range reference.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			switch scenario.Name {
			case "month_summary":
				seedRoomConflictMonth(t, ctx, pool, time.Now())
			case "list_reporter":
				seedRoomConflictRows(t, ctx, pool, reference.Seed)
				verifyRoomConflictOrigins(t, ctx, base, actors["reporter"])
			case "history_after_deletion":
				authTestRequest(t, ctx, base, http.MethodDelete, "/api/Users/2", nil, actors["admin"], http.StatusNoContent)
				var users, reservations int
				if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users WHERE id=2),
					(SELECT count(*) FROM reservations WHERE id=10)`).Scan(&users, &reservations); err != nil || users != 0 || reservations != 0 {
					t.Fatal("historical references were not removed from the source tables")
				}
			}

			target := base + roomConflictAPI + scenario.Path
			if scenario.Name == "location_get" {
				target = location
			}
			raw := scenario.Raw
			if scenario.Model != nil {
				raw = new(string(scenario.Model))
			}
			req := adminUserRequest(t, ctx, scenario.Method, target, raw, actors[scenario.Actor])
			req.Header.Set("Origin", frontend)
			req.Header.Set("Accept", scenario.Accept)
			res, body := sendAdminUserRequest(t, req)
			want := scenario.Response
			contentType, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
			if res.StatusCode != want.Status || contentType != want.ContentType {
				t.Fatalf("response status and media type = %d %q, want %d %q", res.StatusCode, contentType, want.Status, want.ContentType)
			}
			if res.Header.Get("Access-Control-Allow-Origin") != frontend {
				t.Fatal("trusted frontend origin is missing from the response")
			}
			if want.Location != "" {
				if res.Header.Get("Location") != base+want.Location {
					t.Fatal("unexpected Location header")
				}
				if scenario.Name == "creation_defaults" {
					location = res.Header.Get("Location")
				}
			} else if res.Header.Get("Location") != "" {
				t.Fatal("unexpected Location on non-creation response")
			}

			switch {
			case scenario.Generated != "":
				actual := roomConflictJSONObject(t, body)
				id, ok := actual["id"].(float64)
				if !ok {
					t.Fatal("missing numeric record ID")
				}
				stamp, ok := actual[scenario.Generated].(string)
				if !ok {
					t.Fatal("missing generated timestamp")
				}
				if scenario.Method == http.MethodGet {
					if stamp != generated[int(id)][scenario.Generated] {
						t.Fatal("stored server timestamp differed on reread")
					}
				} else {
					stored, err := time.Parse(dotnettime.Layout, stamp)
					now := time.Now()
					if err != nil || stored.Format(dotnettime.Layout) != stamp || dotnettime.OnDate(now, now).Sub(stored).Abs() > 10*time.Second {
						t.Fatal("generated timestamp was not a seven-digit local wall-clock value")
					}
					if generated[int(id)] == nil {
						generated[int(id)] = make(map[string]string)
					}
					generated[int(id)][scenario.Generated] = stamp
				}
				assertRoomConflictRecord(t, scenario, body, want.Body, stamp)
				if scenario.Saved != nil {
					persisted := readRoomConflictRow(t, ctx, pool, int(id))
					assertRoomConflictRecord(t, scenario, persisted, scenario.Saved, stamp)
				}
			case strings.Contains(contentType, "json"):
				if want.Status == http.StatusBadRequest && scenario.Name != "null_room" && contentType == "application/problem+json" {
					problem := roomConflictJSONObject(t, want.Body)
					problem["errors"] = map[string][]string{"request": {"A JSON request object is required."}}
					want.Body, _ = json.Marshal(problem)
				}
				assertAdminUserJSON(t, body, want.Body)
			default:
				var expected string
				if json.Unmarshal(want.Body, &expected) != nil || string(body) != expected {
					t.Fatal("plain HTTP response differed from C#")
				}
			}
		})
	}

	t.Run("local_month_starts_before_utc_month", func(t *testing.T) {
		execAdminUsersSQL(t, ctx, pool, `TRUNCATE room_conflict_records`)
		japan := time.FixedZone("JST", 9*60*60)
		now := time.Date(2026, time.December, 1, 0, 30, 0, 0, japan)
		seedRoomConflictMonth(t, ctx, pool, now)
		summary, err := roomconflicts.NewStore(pool).Summary(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		want := roomconflicts.Summary{
			UnresolvedOverlapsThisMonth: 2, ConfirmedCollisionsThisMonth: 1,
			HighImpactConflictsThisMonth: 2, OpenDetectedRecords: 3,
		}
		if summary != want {
			t.Fatalf("summary = %+v, want %+v", summary, want)
		}
	})
}

func verifyRoomConflictOrigins(t *testing.T, ctx context.Context, base string, reporter *http.Cookie) {
	t.Helper()
	for method, route := range map[string]string{
		http.MethodPost: roomConflictAPI, http.MethodPut: roomConflictAPI + "/20",
	} {
		req := adminUserRequest(t, ctx, method, base+route,
			new(`{"occurredAt":"2026-07-14T09:00","roomName":"Must not write","status":3}`), reporter)
		req.Header.Set("Origin", "https://untrusted.example.test")
		res, _ := sendAdminUserRequest(t, req)
		if res.StatusCode != http.StatusForbidden || res.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("untrusted origin reached %s", method)
		}
	}
}

func seedRoomConflictMonth(t *testing.T, ctx context.Context, pool *pgxpool.Pool, now time.Time) {
	t.Helper()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	next := start.AddDate(0, 1, 0)
	day := 24 * time.Hour
	if _, err := pool.Exec(ctx, `
		INSERT INTO room_conflict_records (type,status,impact,occurred_at_ticks,room_name,cause,description,resolution,created_at_ticks)
		VALUES (0,0,2,$1,'Before month',98,'','',0), (0,3,2,$2,'First instant',98,'','',0),
		       (0,0,0,$3,'Open in month',98,'','',0), (1,1,2,$4,'Last tick',98,'','',0),
		       (1,1,2,$5,'Next month',98,'','',0), (1,2,0,$6,'False alarm',98,'','',0),
		       (0,0,0,$7,'Open next month',98,'','',0)`,
		dotnettime.Ticks(start)-1, dotnettime.Ticks(start), dotnettime.Ticks(start.Add(2*time.Hour)),
		dotnettime.Ticks(next)-1, dotnettime.Ticks(next), dotnettime.Ticks(start.Add(3*day)), dotnettime.Ticks(next.Add(day))); err != nil {
		t.Fatal(err)
	}
}

func seedRoomConflictRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seed []roomconflicts.Record) {
	t.Helper()
	execAdminUsersSQL(t, ctx, pool, `
		TRUNCATE room_conflict_records RESTART IDENTITY;
		INSERT INTO reservations (id,user_id,name,room,number_of_people,type,purpose,
			date_ticks,start_time_ticks,end_time_ticks,participants,repeat_type)
		SELECT id,owner,'Synthetic','Room A',1,'Internal','Synthetic booking',
			639196704000000000,639197064000000000,639197100000000000,'','なし'
		FROM (VALUES (10,2),(20,1),(-2147483648,1),(0,1),(2147483647,1)) fixtures(id,owner)`)
	const insert = `INSERT INTO room_conflict_records (id,type,status,occurred_at_ticks,room_name,
		reservation_id_a,reservation_id_b,impact,cause,description,resolution,detection_key,reported_by_user_id,created_at_ticks)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
	for _, row := range seed {
		_, err := pool.Exec(ctx, insert, row.ID, row.Type, row.Status, seriesTestTicks(t, row.OccurredAt),
			row.RoomName, row.ReservationIDA, row.ReservationIDB, row.Impact, row.Cause, row.Description, row.Resolution,
			row.DetectionKey, row.ReportedByUserID, seriesTestTicks(t, row.CreatedAt))
		if err != nil {
			t.Fatal(err)
		}
	}
	// Imported explicit IDs take effect without raising the identity sequence automatically.
	execAdminUsersSQL(t, ctx, pool, `SELECT setval(pg_get_serial_sequence('room_conflict_records','id'), max(id))
		FROM room_conflict_records`)

	_, err := pool.Exec(ctx, `INSERT INTO room_conflict_records (type,status,occurred_at_ticks,room_name,
		impact,cause,description,resolution,detection_key,created_at_ticks)
		SELECT type,status,occurred_at_ticks,room_name,impact,cause,description,resolution,detection_key,created_at_ticks
		FROM room_conflict_records WHERE detection_key IS NOT NULL`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "room_conflict_records_detection_key_key" {
		t.Fatalf("expected the detection key to reject a duplicate: %v", err)
	}
	// The rejected INSERT reserved an ID. Reset it so the reference IDs align across engines.
	execAdminUsersSQL(t, ctx, pool, `SELECT setval(pg_get_serial_sequence('room_conflict_records','id'), max(id))
		FROM room_conflict_records`)
}

func roomConflictJSONObject(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded == nil {
		t.Fatal("response is not a JSON object")
	}
	return decoded
}

func assertRoomConflictRecord(t *testing.T, scenario roomConflictCase, actual, reference []byte, generated string) {
	t.Helper()
	want := roomConflictJSONObject(t, reference)
	want[scenario.Generated] = generated
	if scenario.Name == "create_offset" {
		input := roomConflictJSONObject(t, scenario.Model)
		value, err := time.Parse(time.RFC3339Nano, input["occurredAt"].(string))
		if err != nil {
			t.Fatal(err)
		}
		want["occurredAt"] = value.In(time.Local).Format(dotnettime.Layout)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	assertAdminUserJSON(t, actual, encoded)
}

func readRoomConflictRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int) []byte {
	t.Helper()
	var row json.RawMessage
	var occurredAt, createdAt int64
	var updatedAt *int64
	err := pool.QueryRow(ctx, `
		SELECT json_build_object('id',id,'type',type,'status',status,'roomName',room_name,
			'reservationIdA',reservation_id_a,'reservationIdB',reservation_id_b,'impact',impact,
			'cause',cause,'description',description,'resolution',resolution,'detectionKey',detection_key,
			'reportedByUserId',reported_by_user_id), occurred_at_ticks, created_at_ticks, updated_at_ticks
		FROM room_conflict_records WHERE id = $1`, id).Scan(&row, &occurredAt, &createdAt, &updatedAt)
	if err != nil {
		t.Fatal(err)
	}
	record := roomConflictJSONObject(t, row)
	record["occurredAt"] = dotnettime.FromTicks(occurredAt).Format(dotnettime.Layout)
	record["createdAt"] = dotnettime.FromTicks(createdAt).Format(dotnettime.Layout)
	record["updatedAt"] = nil
	if updatedAt != nil {
		record["updatedAt"] = dotnettime.FromTicks(*updatedAt).Format(dotnettime.Layout)
	}
	result, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
