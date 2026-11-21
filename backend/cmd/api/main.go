package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/httpapi"
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

	var ping func(context.Context) error
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		config, err := pgxpool.ParseConfig(databaseURL)
		if err != nil {
			return errors.New("invalid DATABASE_URL")
		}
		config.ConnConfig.ConnectTimeout = readinessTimeout
		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			return errors.New("could not initialize PostgreSQL connection pool")
		}
		defer pool.Close()
		ping = pool.Ping
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(ping, readinessTimeout),
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
