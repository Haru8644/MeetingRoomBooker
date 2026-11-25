package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/chatwork"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/httpapi"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/users"
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

	var chatworkSender *chatwork.Sender
	if setting := os.Getenv("CHATWORK_ENABLED"); setting != "" {
		enabled, err := strconv.ParseBool(setting)
		if err != nil {
			return errors.New("CHATWORK_ENABLED must be a boolean")
		}
		if enabled {
			chatworkSender, err = chatwork.NewSender(pool, os.Getenv("CHATWORK_API_TOKEN"), os.Getenv("CHATWORK_API_BASE_URL"), development)
			if err != nil {
				return err
			}
		}
	}

	reservationList := httpapi.NewReservationListHandler(func(ctx context.Context) ([]reservations.ListEntry, error) {
		return reservations.List(ctx, pool)
	})
	createReservation := func(ctx context.Context, entry reservations.ListEntry, allowOverlap bool) (reservations.ListEntry, error) {
		result, err := reservations.Create(ctx, pool, entry, allowOverlap, chatworkSender != nil)
		if err != nil {
			return reservations.ListEntry{}, err
		}
		if chatworkSender != nil {
			chatworkSender.SendCreation(ctx, result)
		}
		return result.Entries[0], nil
	}
	createReservationSeries := func(ctx context.Context, entries []reservations.ListEntry, allowOverlap bool) ([]reservations.ListEntry, error) {
		result, err := reservations.CreateSeries(ctx, pool, entries, allowOverlap, chatworkSender != nil)
		if err == nil && chatworkSender != nil {
			chatworkSender.SendCreation(ctx, result)
		}
		return result.Entries, err
	}
	updateReservation := func(ctx context.Context, entry reservations.ListEntry, actorID int, isAdmin, notify bool) error {
		result, err := reservations.Update(ctx, pool, entry, actorID, isAdmin, notify)
		if err == nil && chatworkSender != nil {
			chatworkSender.SendUpdate(ctx, result)
		}
		return err
	}
	updateReservationSeries := func(ctx context.Context, id int, entry reservations.ListEntry, actorID int, isAdmin bool, scope string, notify bool) error {
		result, err := reservations.UpdateSeries(ctx, pool, id, entry, actorID, isAdmin, scope, notify)
		if err == nil && chatworkSender != nil {
			chatworkSender.SendUpdate(ctx, result)
		}
		return err
	}
	deleteReservation := func(ctx context.Context, id, actorID int, isAdmin bool) error {
		deleted, err := reservations.Delete(ctx, pool, id, actorID, isAdmin)
		if err == nil && chatworkSender != nil {
			chatworkSender.SendCancellation(ctx, deleted)
		}
		return err
	}
	deleteReservationSeries := func(ctx context.Context, id, actorID int, isAdmin bool, scope string) error {
		deleted, err := reservations.DeleteSeries(ctx, pool, id, actorID, isAdmin, scope)
		if err == nil && chatworkSender != nil {
			chatworkSender.SendCancellation(ctx, deleted)
		}
		return err
	}
	changeParticipation := func(ctx context.Context, id, actorID int, join bool) error {
		return reservations.ChangeParticipation(ctx, pool, id, actorID, join)
	}
	listWorkSchedules := func(ctx context.Context, from, to time.Time) ([]workschedules.Entry, error) {
		return workschedules.List(ctx, pool, from, to)
	}
	getWorkSchedule := func(ctx context.Context, id int) (*workschedules.Entry, error) {
		return workschedules.Get(ctx, pool, id)
	}
	createWorkSchedule := func(ctx context.Context, input workschedules.EntryInput, creatorID int, series bool) ([]workschedules.Entry, error) {
		create := workschedules.Create
		if series {
			create = workschedules.CreateSeries
		}
		result, err := create(ctx, pool, input, creatorID)
		if err == nil && chatworkSender != nil {
			chatworkSender.SendWorkScheduleCreation(ctx, result)
		}
		return result.Entries, err
	}
	updateWorkSchedule := func(ctx context.Context, id int, input workschedules.EntryInput, user auth.User, scope string) (workschedules.Entry, error) {
		result, err := workschedules.UpdateSeries(ctx, pool, id, input, user.ID, user.IsAdmin, scope)
		if err == nil && chatworkSender != nil {
			chatworkSender.SendWorkScheduleUpdate(ctx, result)
		}
		return result.Entry, err
	}
	deleteWorkSchedule := func(ctx context.Context, id int, user auth.User, scope string) error {
		result, err := workschedules.DeleteSeries(ctx, pool, id, user.ID, user.IsAdmin, scope)
		if err == nil && chatworkSender != nil {
			chatworkSender.SendWorkScheduleDeletion(ctx, result)
		}
		return err
	}
	listParticipants := func(ctx context.Context) ([]users.Participant, error) {
		return users.ListParticipants(ctx, pool)
	}
	handler, err := httpapi.RegisterSessionRoutes(httpapi.NewHandler(ping, readinessTimeout), auth.NewService(pool),
		reservationList, createReservation, createReservationSeries, updateReservation, updateReservationSeries,
		deleteReservation, deleteReservationSeries, changeParticipation,
		listWorkSchedules, getWorkSchedule, createWorkSchedule, updateWorkSchedule, deleteWorkSchedule, listParticipants, notifications.NewInbox(pool),
		httpapi.SessionOptions{Development: development, FrontendOrigins: strings.Split(os.Getenv("FRONTEND_ORIGINS"), ",")})
	if err != nil {
		return err
	}

	if chatworkSender != nil {
		workerCtx, stopWorker := context.WithCancel(ctx)
		workerDone := make(chan struct{})
		go func() {
			defer close(workerDone)
			chatworkSender.RunReminders(workerCtx)
		}()
		// Join the worker on shutdown and on listen errors before closing the pool.
		defer func() {
			stopWorker()
			<-workerDone
		}()
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
