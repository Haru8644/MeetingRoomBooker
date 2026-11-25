package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/roomconflicts"
	"github.com/jackc/pgx/v5/pgxpool"
)

type detectionSeed struct {
	ID                             int
	Room, Date, StartTime, EndTime string
}

func insertDetectionSeeds(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seeds []detectionSeed) {
	t.Helper()
	for _, seed := range seeds {
		_, err := pool.Exec(ctx, `INSERT INTO reservations (id, user_id, name, room, number_of_people, type,
			purpose, date_ticks, start_time_ticks, end_time_ticks, participants, repeat_type)
			VALUES ($1,1,'Synthetic',$2,1,'Internal','Synthetic',$3,$4,$5,'','なし')`, seed.ID, seed.Room,
			seriesTestTicks(t, seed.Date), seriesTestTicks(t, seed.StartTime), seriesTestTicks(t, seed.EndTime))
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRoomConflictDetectionPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "ROOM_CONFLICT_DETECTION_TEST_DATABASE_URL")
	applyAdminUsersMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0012_create_room_conflict_records.sql")
	passwords := seedAuthTestUsers(t, ctx, pool)
	var reference struct {
		Now                         string
		LookbackMinutes             int
		Seeds                       []detectionSeed
		Rows                        []roomconflicts.Record
		Existing                    roomconflicts.Record
		First, WithExisting, Replay int
	}
	loadSeriesDeleteFixture(t, "room_conflict_detection_csharp.json", &reference)
	insertDetectionSeeds(t, ctx, pool, reference.Seeds)
	store := roomconflicts.NewStore(pool)
	now, err := time.ParseInLocation(dotnettime.Layout, reference.Now, time.FixedZone("JST", 9*3600))
	if err != nil {
		t.Fatal(err)
	}
	pass := func() (int, error) {
		return store.Detect(ctx, now, time.Duration(reference.LookbackMinutes)*time.Minute)
	}

	// The ordered traversal reaches this pair after other writes in the same pass.
	execAdminUsersSQL(t, ctx, pool, `ALTER TABLE room_conflict_records ADD CONSTRAINT reject_synthetic_pair CHECK (reservation_id_a <> 3 OR reservation_id_b <> 5)`)
	if n, err := pass(); err == nil || n != 0 {
		t.Fatalf("injected failure: count=%d error=%v", n, err)
	}
	if rows, err := store.List(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("partial pass: rows=%d error=%v", len(rows), err)
	}
	execAdminUsersSQL(t, ctx, pool, `ALTER TABLE room_conflict_records DROP CONSTRAINT reject_synthetic_pair`)
	var workers sync.WaitGroup
	start := make(chan struct{})
	counts := make(chan int, 4)
	for range cap(counts) {
		workers.Go(func() {
			<-start
			n, err := pass()
			if err != nil {
				t.Errorf("parallel detection: %v", err)
			}
			counts <- n
		})
	}
	close(start)
	workers.Wait()
	close(counts)
	total := 0
	for n := range counts {
		total += n
	}
	if total != reference.First {
		t.Fatalf("parallel inserts=%d want %d", total, reference.First)
	}
	rows, err := store.List(ctx)
	if err != nil || len(rows) != len(reference.Rows) {
		t.Fatalf("detected rows=%d error=%v", len(rows), err)
	}
	expected := make(map[string]roomconflicts.Record)
	for _, r := range reference.Rows {
		expected[*r.DetectionKey] = r
	}
	for _, r := range rows {
		want, ok := expected[*r.DetectionKey]
		want.ID = r.ID // Identity values may have been consumed by the failed or concurrent pass.
		if !ok || !reflect.DeepEqual(r, want) {
			t.Fatalf("different stored values for key %q: got %+v want %+v", *r.DetectionKey, r, want)
		}
	}

	// Import the edited SQLite record as the only existing key.
	execAdminUsersSQL(t, ctx, pool, `TRUNCATE room_conflict_records`)
	r := reference.Existing
	_, err = pool.Exec(ctx, `INSERT INTO room_conflict_records (id,type,status,occurred_at_ticks,room_name,
		reservation_id_a,reservation_id_b,impact,cause,description,resolution,detection_key,created_at_ticks,updated_at_ticks)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, r.ID, r.Type, r.Status, seriesTestTicks(t, r.OccurredAt),
		r.RoomName, r.ReservationIDA, r.ReservationIDB, r.Impact, r.Cause, r.Description, r.Resolution, r.DetectionKey,
		seriesTestTicks(t, r.CreatedAt), seriesTestTicks(t, *r.UpdatedAt))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{reference.WithExisting, reference.Replay} {
		if n, err := pass(); err != nil || n != want {
			t.Fatalf("replay: count=%d want=%d error=%v", n, want, err)
		}
	}
	if got, err := store.Get(ctx, r.ID); err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("modified imported review: %+v error=%v", got, err)
	}

	edited, err := store.Update(ctx, r.ID, 1, true, roomconflicts.Fields{
		Status: roomconflicts.FalseAlarm, Impact: roomconflicts.Low, Cause: 0,
		Description: "Reviewed by administrator", Resolution: "Keep this classification",
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := pass(); err != nil || n != 0 {
		t.Fatalf("after edit: count=%d error=%v", n, err)
	}
	if got, err := store.Get(ctx, r.ID); err != nil || !reflect.DeepEqual(got, edited) {
		t.Fatalf("re-detection changed admin edit: %+v error=%v", got, err)
	}

	t.Run("api_lifecycle", func(t *testing.T) {
		execAdminUsersSQL(t, ctx, pool, `TRUNCATE reservations, room_conflict_records`)
		// Keep the live server test inside the window across small clock corrections.
		now := time.Now()
		startTime := dotnettime.OnDate(now, now).Add(-7 * time.Minute)
		addPair := func(id int, room string) {
			seed := detectionSeed{
				ID: id, Room: room, Date: startTime.Format("2006-01-02") + "T00:00:00",
				StartTime: startTime.Format(dotnettime.Layout), EndTime: startTime.Add(time.Hour).Format(dotnettime.Layout),
			}
			other := seed
			other.ID++
			insertDetectionSeeds(t, ctx, pool, []detectionSeed{seed, other})
		}
		addPair(1, "One")
		t.Setenv("CHATWORK_ENABLED", "false")
		t.Setenv("ROOM_CONFLICT_DETECTION_ENABLED", "")
		_, stop := startAuthTestAPI(t, ctx, dsn)
		stop()
		if rows, err := store.List(ctx); err != nil || len(rows) != 0 {
			t.Fatalf("default worker inserted rows: %d %v", len(rows), err)
		}

		t.Setenv("ROOM_CONFLICT_DETECTION_ENABLED", "true")
		testServer := func(want int) {
			addr, stop := startAuthTestAPI(t, ctx, dsn)
			defer stop()
			waitForSeriesEditLocks(t, ctx, pool, want, `SELECT count(*) FROM room_conflict_records`)
			base := "http://" + addr
			res, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login", map[string]string{"email": "member@example.test", "password": passwords["member"]}, nil, 200)
			cookie := authTestCookie(t, res, false)
			_, body := authTestRequest(t, ctx, base, http.MethodGet, roomConflictAPI, nil, cookie, 200)
			var list []struct {
				roomconflicts.Record
				CanEdit bool
			}
			if err := json.Unmarshal(body, &list); err != nil || len(list) != want {
				t.Fatalf("list: %s", body)
			}
			for _, row := range list {
				if row.CanEdit || row.DetectionKey == nil || row.Status != roomconflicts.Detected {
					t.Fatalf("invalid detected row: %+v", row)
				}
			}
			_, body = authTestRequest(t, ctx, base, http.MethodGet, roomConflictAPI+"/summary", nil, cookie, 200)
			var summary roomconflicts.Summary
			monthly := want
			if time.Now().Format("2006-01") != startTime.Format("2006-01") {
				monthly = 0
			}
			if err := json.Unmarshal(body, &summary); err != nil || summary.OpenDetectedRecords != want || summary.UnresolvedOverlapsThisMonth != monthly {
				t.Fatalf("summary: %s", body)
			}
		}
		testServer(1)
		addPair(3, "Two")
		testServer(2)

		// Stop an active database query without releasing the lock it is waiting for.
		lock, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Rollback(ctx)
		if _, err := lock.Exec(ctx, `LOCK TABLE reservations IN ACCESS EXCLUSIVE MODE`); err != nil {
			t.Fatal(err)
		}
		_, stop = startAuthTestAPI(t, ctx, dsn)
		waitForSeriesEditLocks(t, ctx, pool, 1, `SELECT count(*) FROM pg_locks WHERE relation='reservations'::regclass AND mode='AccessShareLock' AND NOT granted`)
		stop()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		t.Setenv("HTTP_ADDR", listener.Addr().String())
		if err := run(ctx); !errors.Is(err, syscall.EADDRINUSE) {
			t.Fatalf("listen failure: %v", err)
		}
	})
}
