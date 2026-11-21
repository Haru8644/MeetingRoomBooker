package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/httpapi"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultAddr      = "127.0.0.1:8081"
	readinessTimeout = time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	development := os.Getenv("APP_ENV") == "development"
	if development {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || (host != "localhost" && !net.ParseIP(host).IsLoopback()) {
			return errors.New("APP_ENV=development requires a loopback HTTP_ADDR")
		}
	}

	var pool *pgxpool.Pool
	var ping func(context.Context) error
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		config, err := pgxpool.ParseConfig(databaseURL)
		if err != nil {
			return errors.New("invalid DATABASE_URL")
		}
		config.ConnConfig.ConnectTimeout = readinessTimeout
		pool, err = pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			return errors.New("could not initialize PostgreSQL connection pool")
		}
		defer pool.Close()
		ping = pool.Ping
	}

	reservationList := httpapi.NewReservationListHandler(func(ctx context.Context) ([]reservations.ListEntry, error) {
		return reservations.List(ctx, pool)
	})
	createReservation := func(ctx context.Context, entry reservations.ListEntry, allowOverlap bool) (reservations.ListEntry, error) {
		return reservations.Create(ctx, pool, entry, allowOverlap)
	}
	listWorkSchedules := func(ctx context.Context, from, to time.Time) ([]workschedules.Entry, error) {
		return workschedules.List(ctx, pool, from, to)
	}
	handler, err := httpapi.RegisterSessionRoutes(httpapi.NewHandler(ping, readinessTimeout), auth.NewService(pool), reservationList, createReservation, listWorkSchedules,
		httpapi.SessionOptions{Development: development, FrontendOrigins: strings.Split(os.Getenv("FRONTEND_ORIGINS"), ",")})
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()
	log.Printf("Go API starting on %s", addr)

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
