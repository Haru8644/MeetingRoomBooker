package reservations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConflictError struct {
	Reservation
	Label string
}

func (c *ConflictError) Error() string {
	return fmt.Sprintf("この時間帯には既に予約があります。会議室: %s / 利用日: %s / 時間: %s〜%s / 予約: %s",
		c.Room, c.Date.Format("2006/01/02"), c.StartTime.Format("15:04"), c.EndTime.Format("15:04"), c.Label)
}

func Create(ctx context.Context, pool *pgxpool.Pool, entry ListEntry, allowOverlap bool) (ListEntry, error) {
	date := dotnettime.Ticks(entry.Date)
	start := dotnettime.Ticks(entry.StartTime)
	end := dotnettime.Ticks(entry.EndTime)
	day := date / dotnettime.TicksPerDay
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		// Check conflicts in a new statement after obtaining the lock so it sees the previous writer's commit.
		if err := lockReservationSlot(ctx, tx, entry.Room, day); err != nil {
			return err
		}
		if !allowOverlap {
			if err := checkConflict(ctx, tx, entry.Room, day, start, end); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO reservations (user_id, name, room, number_of_people, type, purpose,
			    date_ticks, start_time_ticks, end_time_ticks, participant_ids, participants, repeat_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id`, entry.UserID, entry.Name, entry.Room, entry.NumberOfPeople, entry.Type, entry.Purpose,
			date, start, end, entry.ParticipantIDs, entry.Participants, entry.RepeatType).Scan(&entry.ID); err != nil {
			return fmt.Errorf("insert reservation: %w", err)
		}
		return notifyAddedParticipants(ctx, tx, entry)
	})
	if err != nil {
		return ListEntry{}, err
	}
	return entry, nil
}

func lockReservationSlot(ctx context.Context, tx pgx.Tx, room string, day int64) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), $2::integer)`, room, int32(day)); err != nil {
		return fmt.Errorf("lock room and date: %w", err)
	}
	return nil
}

func checkConflict(ctx context.Context, tx pgx.Tx, room string, day, start, end int64) error {
	var date, existingStart, existingEnd int64
	var name, purpose string
	err := tx.QueryRow(ctx, `
		SELECT date_ticks, start_time_ticks, end_time_ticks, name, purpose
		FROM reservations
		WHERE hashtext(room) = hashtext($1) AND room = $1 AND date_ticks / 864000000000 = $2
		  AND start_time_ticks < $3 AND end_time_ticks > $4
		ORDER BY date_ticks, start_time_ticks, id LIMIT 1`, room, day, end, start).Scan(&date, &existingStart, &existingEnd, &name, &purpose)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check reservation conflicts: %w", err)
	}
	label := purpose
	if strings.TrimSpace(label) == "" {
		label = name
	}
	return &ConflictError{Reservation: Reservation{
		Room: room, Date: dotnettime.FromTicks(date), StartTime: dotnettime.FromTicks(existingStart), EndTime: dotnettime.FromTicks(existingEnd),
	}, Label: label}
}
