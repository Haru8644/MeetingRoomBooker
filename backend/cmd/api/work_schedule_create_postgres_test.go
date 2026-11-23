package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const workScheduleTestPath = "/api/work-schedule-entries"

type workScheduleCreationCapture struct {
	Status                int
	ContentType, Location string
	Body                  json.RawMessage
}

type workScheduleCreationNotice struct {
	UserID, TargetWorkScheduleEntryID int
	TargetReservationID               *int
	Type, Message, TargetDate         string
	IsRead                            bool
}

type workScheduleConflictSeed struct {
	Kind, Title    string
	ID, UserID     int
	Type           int
	Start, End     *string
	ParticipantIDs []int
}

func TestWorkScheduleCreationPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	pool, dsn := newAPITestDatabase(t, ctx, "WORK_SCHEDULE_CREATE_TEST_DATABASE_URL")
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	local := time.Local
	time.Local = tokyo // The C# capture interprets explicit numeric offsets in JST.
	t.Cleanup(func() { time.Local = local })

	applyChatworkTestMigrations(t, ctx, pool)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0008_index_reservation_start_times.sql")
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0009_index_work_schedule_reminder_start_times.sql")
	migrateWorkScheduleCreation(t, ctx, pool)

	var reference struct {
		Users []chatworkTestUser
		Cases []struct {
			Name, Actor        string
			Request            json.RawMessage
			Response           workScheduleCreationCapture
			Saved              []workschedules.Entry
			Notices            []workScheduleCreationNotice
			Logs               []chatworkTestLog
			Requests           []chatworkTestRequest
			Seeds              []workScheduleConflictSeed
			ForceNoticeFailure bool
			FakeResponses      map[string]any
		}
		Gets map[string]workScheduleCreationCapture
	}
	loadSeriesDeleteFixture(t, "work_schedule_create_csharp.json", &reference)
	passwords := seedAuthTestUsers(t, ctx, pool)
	seedChatworkTestProfiles(t, ctx, pool, reference.Users)

	fake := &chatworkHTTPFake{t: t, pool: pool}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()
	t.Setenv("CHATWORK_ENABLED", "true")
	t.Setenv("CHATWORK_API_TOKEN", fakeChatworkToken)
	t.Setenv("CHATWORK_API_BASE_URL", server.URL+"/v2/")
	login := func(t *testing.T, base, user string) *http.Cookie {
		t.Helper()
		response, _ := authTestRequest(t, ctx, base, http.MethodPost, "/api/Auth/login",
			map[string]string{"email": user + "@example.test", "password": passwords[user]}, nil, http.StatusOK)
		return authTestCookie(t, response, false)
	}

	t.Run("enabled", func(t *testing.T) {
		addr, stop := startAuthTestAPI(t, ctx, dsn, "http://127.0.0.1:5168")
		defer stop()
		base := "http://" + addr
		admin, owner := login(t, base, "admin"), login(t, base, "member")
		actors := map[string]*http.Cookie{"admin": admin, "owner": owner}
		for route, expected := range reference.Gets {
			cookie := admin
			if expected.Status == http.StatusUnauthorized {
				cookie = nil
			}
			res, body := callWorkScheduleRoute(t, ctx, http.MethodGet, base+route, nil, cookie, "", expected.Status)
			compareWorkScheduleResponse(t, expected, res, body)
		}
		for _, scenario := range reference.Cases {
			t.Run(scenario.Name, func(t *testing.T) {
				execWorkScheduleTestSQL(t, ctx, pool, "TRUNCATE reservations, work_schedule_entries, notifications, chatwork_delivery_logs")
				seedWorkScheduleCreationConflicts(t, ctx, pool, scenario.Seeds)
				if scenario.ForceNoticeFailure {
					constrainSeriesTestWrites(t, ctx, pool, "notifications", "user_id <> 3")
				}
				expected := scenario.Response
				seeded := 0
				for _, seed := range scenario.Seeds {
					if seed.Kind == "work" {
						seeded++
					}
				}
				wantNew := 0
				if expected.Status == http.StatusCreated {
					wantNew = 1
				}
				fake.expect(scenario.FakeResponses, func(ctx context.Context) error {
					return checkWorkScheduleCreationCounts(ctx, pool, seeded+wantNew, len(scenario.Notices))
				})

				started := time.Now()
				res, body := callWorkScheduleRoute(t, ctx, http.MethodPost, base+workScheduleTestPath,
					scenario.Request, actors[scenario.Actor], "", expected.Status)
				if err := checkWorkScheduleCreationCounts(ctx, pool, seeded+wantNew, len(scenario.Notices)); err != nil {
					t.Fatal(err)
				}
				if scenario.ForceNoticeFailure {
					if len(body) != 0 || len(scenario.Saved) != 1 {
						t.Fatal("expected an empty Go 500 and the C# separately committed row")
					}
				} else if wantNew == 0 {
					compareWorkScheduleResponse(t, expected, res, body)
				} else {
					var entry, want workschedules.Entry
					if err := json.Unmarshal(body, &entry); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(expected.Body, &want); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(want, scenario.Saved[0]) || res.Header.Get("Content-Type") != expected.ContentType {
						t.Fatal("POST does not match the expected response format and captured SQLite row")
					}
					createdAt, err := time.ParseInLocation(dotnettime.Layout, entry.CreatedAt, time.Local)
					if err != nil || createdAt.Before(started.Truncate(100*time.Nanosecond)) ||
						createdAt.After(time.Now()) || entry.UpdatedAt != entry.CreatedAt {
						t.Fatalf("invalid timestamps: %+v", entry)
					}
					if entry.ID <= 7200 {
						t.Fatalf("new ID %d collided with the imported ID range", entry.ID)
					}
					if scenario.Name == "omitted_defaults" {
						today := createdAt.Format("2006-01-02")
						replaceDate := func(text string) string {
							return strings.ReplaceAll(text, strings.ReplaceAll(want.Date[:10], "-", "/"), strings.ReplaceAll(today, "-", "/"))
						}
						for i := range scenario.Notices {
							scenario.Notices[i].TargetDate = today + "T00:00:00"
							scenario.Notices[i].Message = replaceDate(scenario.Notices[i].Message)
						}
						for i := range scenario.Logs {
							scenario.Logs[i].ScheduledStartTime = today + "T00:00:00"
							scenario.Logs[i].Message = replaceDate(scenario.Logs[i].Message)
						}
						for i := range scenario.Requests {
							scenario.Requests[i].Body = replaceDate(scenario.Requests[i].Body)
						}
						want.Date = today + "T00:00:00"
					}
					location := base + workScheduleTestPath + "/" + strconv.Itoa(entry.ID)
					if got := res.Header.Get("Location"); got != location ||
						!strings.HasSuffix(expected.Location, workScheduleTestPath+"/"+strconv.Itoa(want.ID)) {
						t.Fatalf("Location got %q, want %q", got, location)
					}
					capturedID := want.ID
					want.ID, want.CreatedAt, want.UpdatedAt = entry.ID, entry.CreatedAt, entry.UpdatedAt
					if !reflect.DeepEqual(entry, want) {
						t.Fatalf("POST response got %+v, want %+v", entry, want)
					}
					stored, err := workschedules.Get(ctx, pool, entry.ID)
					if err != nil || stored == nil || !reflect.DeepEqual(*stored, want) {
						t.Fatalf("stored entry = %+v, err = %v", stored, err)
					}
					reader := admin
					if scenario.Actor == "admin" {
						reader = owner
					}
					_, getBody := callWorkScheduleRoute(t, ctx, http.MethodGet, location, nil, reader, "", http.StatusOK)
					if !bytes.Equal(getBody, body) {
						t.Fatalf("GET body = %s, expected POST body %s", getBody, body)
					}
					for i := range scenario.Notices {
						if scenario.Notices[i].TargetWorkScheduleEntryID != capturedID {
							t.Fatal("captured notice did not link to the C# response")
						}
						scenario.Notices[i].TargetWorkScheduleEntryID = entry.ID
					}
					for i := range scenario.Logs {
						row := &scenario.Logs[i]
						if row.WorkScheduleEntryID == nil || *row.WorkScheduleEntryID != capturedID ||
							row.DeliveryKey != fmt.Sprintf("WorkScheduleCreated:work-schedule:%d:user:%d", capturedID, row.TargetUserID) {
							t.Fatal("captured delivery did not use the expected C# key and entry ID")
						}
						row.WorkScheduleEntryID = &entry.ID
						row.DeliveryKey = fmt.Sprintf("WorkScheduleCreated:work-schedule:%d:user:%d", entry.ID, row.TargetUserID)
					}
					if scenario.Name == "external" {
						verifyWorkScheduleDeliveryReplay(t, ctx, pool, fake, server.URL, entry, scenario.Logs[0])
					}
				}
				if got := readWorkScheduleCreationNotices(t, ctx, pool); !reflect.DeepEqual(got, scenario.Notices) {
					t.Fatalf("saved notices = %+v, want %+v", got, scenario.Notices)
				}
				if got := fake.recorded(); !slices.Equal(got, scenario.Requests) {
					t.Fatalf("fake HTTP calls = %+v, want %+v", got, scenario.Requests)
				}
				compareChatworkLogs(t, scenario.Name, scenario.Logs, readChatworkLogs(t, ctx, pool), started, false)
			})
		}
		t.Run("origin_protection", func(t *testing.T) {
			fake.expect(nil, nil)
			callWorkScheduleRoute(t, ctx, http.MethodPost, base+workScheduleTestPath, reference.Cases[0].Request,
				owner, "https://untrusted.example.test", http.StatusForbidden)
			if err := checkWorkScheduleCreationCounts(ctx, pool, 0, 0); err != nil || len(fake.recorded()) != 0 {
				t.Fatalf("untrusted origin had side effects: %v", err)
			}
			fake.expect(nil, func(ctx context.Context) error { return checkWorkScheduleCreationCounts(ctx, pool, 1, 3) })
			res, _ := callWorkScheduleRoute(t, ctx, http.MethodPost, base+workScheduleTestPath, reference.Cases[0].Request,
				owner, "http://127.0.0.1:5168", http.StatusCreated)
			if res.Header.Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5168" || len(fake.recorded()) != 2 {
				t.Fatal("trusted origin did not create and deliver")
			}
		})
	})
	t.Run("disabled_by_default", func(t *testing.T) {
		execWorkScheduleTestSQL(t, ctx, pool, "TRUNCATE work_schedule_entries, notifications, chatwork_delivery_logs")
		fake.expect(nil, nil)
		t.Setenv("CHATWORK_ENABLED", "")
		addr, stop := startAuthTestAPI(t, ctx, dsn)
		defer stop()
		base := "http://" + addr
		callWorkScheduleRoute(t, ctx, http.MethodPost, base+workScheduleTestPath, reference.Cases[0].Request,
			login(t, base, "member"), "", http.StatusCreated)
		if err := checkWorkScheduleCreationCounts(ctx, pool, 1, 3); err != nil {
			t.Fatal(err)
		}
		if len(fake.recorded()) > 0 || len(readChatworkLogs(t, ctx, pool)) > 0 {
			t.Fatal("disabled Chatwork had delivery side effects")
		}
	})
}

func migrateWorkScheduleCreation(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	execWorkScheduleTestSQL(t, ctx, pool, `
		INSERT INTO work_schedule_entries
		    (id,created_by_user_id,type,title,date_ticks,leave_period,participants,created_at_ticks,updated_at_ticks)
		VALUES (7000,2,1,'既存の架空勤務予定',0,0,'',0,0);
		INSERT INTO notifications (user_id,message,created_at_ticks,target_date_ticks,type,target_work_schedule_entry_id)
		VALUES (2,'削除済みの架空勤務予定',0,0,'Info',7100);
		INSERT INTO chatwork_delivery_logs
		    (reservation_id,work_schedule_entry_id,delivery_type,scheduled_start_time_ticks,status,
		     attempted_at_ticks,created_at_ticks,message)
		VALUES (0,7200,'WorkScheduleCreated',0,'Succeeded',0,0,'過去の架空勤務予定');`)
	applyAuthTestSQL(t, ctx, pool, "../../migrations/0010_enable_work_schedule_creation.sql")
	var id int
	if err := pool.QueryRow(ctx, `
		INSERT INTO work_schedule_entries
		    (created_by_user_id,type,title,date_ticks,leave_period,participants,created_at_ticks,updated_at_ticks)
		VALUES (2,1,'新規の架空勤務予定',0,0,'',0,0) RETURNING id`).Scan(&id); err != nil || id != 7201 {
		t.Fatalf("first generated ID = %d, want 7201, err = %v", id, err)
	}
	existing, err := workschedules.Get(ctx, pool, 7000)
	if err != nil || existing == nil || existing.Title != "既存の架空勤務予定" {
		t.Fatalf("migration did not preserve the imported entry: %+v, %v", existing, err)
	}
}

func callWorkScheduleRoute(t *testing.T, ctx context.Context, method, url string, payload []byte, cookie *http.Cookie,
	origin string, status int,
) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != status {
		t.Fatalf("%s %s: status = %d, want %d, body = %s", method, url, res.StatusCode, status, body)
	}
	return res, body
}

func compareWorkScheduleResponse(t *testing.T, expected workScheduleCreationCapture, res *http.Response, body []byte) {
	t.Helper()
	if string(expected.Body) == `""` {
		if len(body) != 0 {
			t.Fatalf("expected an empty response, got %s", body)
		}
		return
	}
	if got := res.Header.Get("Content-Type"); got != expected.ContentType || res.Header.Get("Location") != expected.Location {
		t.Fatalf("headers = %v, expected content type %q and Location %q", res.Header, expected.ContentType, expected.Location)
	}
	var got, want any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected.Body, &want); err != nil {
		t.Fatal(err)
	}
	if problem, ok := want.(map[string]any); ok {
		if fields, ok := problem["errors"].(map[string]any); ok {
			for name, raw := range fields {
				messages := raw.([]any)
				for i, msg := range messages {
					before, _, _ := strings.Cut(msg.(string), " | LineNumber:")
					messages[i] = before
				}
				fields[name] = messages
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %s, want %s", body, expected.Body)
	}
}

func execWorkScheduleTestSQL(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func seedWorkScheduleCreationConflicts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seeds []workScheduleConflictSeed) {
	t.Helper()
	const day = "2026-06-22T"
	date := seriesTestTicks(t, day+"00:00:00")
	toTicks := func(s *string) *int64 {
		if s == nil {
			return nil
		}
		return new(seriesTestTicks(t, day+*s))
	}
	for _, seed := range seeds {
		if seed.Kind == "reservation" {
			execWorkScheduleTestSQL(t, ctx, pool, `
				INSERT INTO reservations
				    (id,user_id,name,room,number_of_people,type,purpose,date_ticks,start_time_ticks,end_time_ticks,
				     participant_ids,participants,repeat_type)
				VALUES ($1,$2,'架空の会議名','大会議室',1,'社内',$3,$4,$5,$6,$7,'','しない')`,
				seed.ID, seed.UserID, seed.Title, date, toTicks(seed.Start), toTicks(seed.End), seed.ParticipantIDs)
		} else {
			execWorkScheduleTestSQL(t, ctx, pool, `
				INSERT INTO work_schedule_entries
				    (id,created_by_user_id,type,title,date_ticks,start_time_ticks,end_time_ticks,
				     participant_ids,leave_period,participants,created_at_ticks,updated_at_ticks)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,'',$5,$5)`, seed.ID, seed.UserID, seed.Type,
				seed.Title, date, toTicks(seed.Start), toTicks(seed.End), seed.ParticipantIDs)
		}
	}
}

func checkWorkScheduleCreationCounts(ctx context.Context, pool *pgxpool.Pool, entries, notices int) error {
	var gotEntries, gotNotices int
	err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM work_schedule_entries), (SELECT count(*) FROM notifications)").
		Scan(&gotEntries, &gotNotices)
	if err != nil {
		return err
	}
	if gotEntries != entries || gotNotices != notices {
		return fmt.Errorf("got %d entries and %d notices, want %d and %d", gotEntries, gotNotices, entries, notices)
	}
	return nil
}

func readWorkScheduleCreationNotices(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []workScheduleCreationNotice {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT user_id,target_work_schedule_entry_id,target_reservation_id,type,message,target_date_ticks,is_read
		FROM notifications ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	notices, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (workScheduleCreationNotice, error) {
		var notice workScheduleCreationNotice
		var date int64
		err := row.Scan(&notice.UserID, &notice.TargetWorkScheduleEntryID, &notice.TargetReservationID, &notice.Type,
			&notice.Message, &date, &notice.IsRead)
		notice.TargetDate = dotnettime.FromTicks(date).Format(dotnettime.Layout)
		return notice, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return notices
}

func verifyWorkScheduleDeliveryReplay(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fake *chatworkHTTPFake,
	fakeURL string, entry workschedules.Entry, expected chatworkTestLog,
) {
	t.Helper()
	sender, err := chatwork.NewSender(pool, fakeChatworkToken, fakeURL+"/v2/", true)
	if err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(dotnettime.Layout, expected.ScheduledStartTime)
	if err != nil {
		t.Fatal(err)
	}
	calls, logs := fake.recorded(), readChatworkLogs(t, ctx, pool)
	sender.SendWorkScheduleCreation(ctx, workschedules.Creation{Entry: entry, ScheduledStart: start, ChatworkBody: expected.Message})
	if !slices.Equal(calls, fake.recorded()) || !reflect.DeepEqual(logs, readChatworkLogs(t, ctx, pool)) {
		t.Fatal("replay duplicated a delivery attempt")
	}
}
