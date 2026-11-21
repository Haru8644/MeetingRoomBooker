package httpapi

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReservationListPostgres(t *testing.T) {
	databaseURL := os.Getenv("RESERVATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set RESERVATION_TEST_DATABASE_URL to run the PostgreSQL test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid RESERVATION_TEST_DATABASE_URL")
	}
	schema := pgx.Identifier{"reservation_list_test_" + strings.ToLower(rand.Text())}.Sanitize()
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	}()

	mux := reservationListMux(func(ctx context.Context) ([]reservations.ListEntry, error) {
		return reservations.List(ctx, pool)
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer synthetic-test-user" {
			r = r.WithContext(WithAuthenticatedUser(r.Context(), 2))
		}
		mux.ServeHTTP(w, r)
	}))
	defer server.Close()

	t.Run("anonymous", func(t *testing.T) {
		response, body := getReservationList(t, ctx, server, false)
		assertUnauthorized(t, response, body)
	})
	applyReservationSQL(t, ctx, pool, "../../migrations/0001_create_reservations.sql")
	t.Run("empty", func(t *testing.T) {
		response, body := getReservationList(t, ctx, server, true)
		assertReservationList(t, response, body, nil)
	})
	applyReservationSQL(t, ctx, pool, "testdata/reservation_list_seed.sql")
	t.Run("populated", func(t *testing.T) {
		response, body := getReservationList(t, ctx, server, true)
		assertReservationList(t, response, body, csharpReservationList(t))
	})
}

func getReservationList(t *testing.T, ctx context.Context, server *httptest.Server, authenticated bool) (*http.Response, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/Reservations", nil)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated {
		request.Header.Set("Authorization", "Bearer synthetic-test-user")
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, body
}

func applyReservationSQL(t *testing.T, ctx context.Context, pool *pgxpool.Pool, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply %s: %v", path, err)
	}
}
