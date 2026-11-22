package reservations

import (
	"context"
	"errors"
	"fmt"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("reservation not found")
	ErrForbidden = errors.New("reservation belongs to another user")
)

type ValidationError string

func (e ValidationError) Error() string { return string(e) }

func Update(ctx context.Context, pool *pgxpool.Pool, entry ListEntry, actorID int, isAdmin, notify bool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		previous, err := lockManagedReservation(ctx, tx, entry.ID, actorID, isAdmin)
		if err != nil {
			return err
		}
		entry.UserID, entry.Name, entry.SeriesID = previous.UserID, previous.Name, previous.SeriesID
		if !entry.StartTime.Before(entry.EndTime) {
			return ValidationError("終了時刻は開始時刻より後にしてください。")
		}
		date := dotnettime.Ticks(entry.Date)
		day := date / dotnettime.TicksPerDay
		var repeatUntil *int64
		if entry.RepeatUntil != nil {
			ticks := dotnettime.Ticks(*entry.RepeatUntil)
			if ticks/dotnettime.TicksPerDay < day {
				return ValidationError("繰り返し終了日は予約日以降にしてください。")
			}
			repeatUntil = &ticks
		}
		// Edits may overlap in C#. Share the destination lock with creates so a
		// create checks the committed state when this edit holds the lock first.
		if err := lockReservationSlot(ctx, tx, entry.Room, day); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE reservations
			SET room = $2, number_of_people = $3, type = $4, purpose = $5,
			    date_ticks = $6, start_time_ticks = $7, end_time_ticks = $8,
			    participant_ids = $9, participants = $10, repeat_type = $11, repeat_until_ticks = $12
			WHERE id = $1`, entry.ID, entry.Room, entry.NumberOfPeople, entry.Type, entry.Purpose,
			date, dotnettime.Ticks(entry.StartTime), dotnettime.Ticks(entry.EndTime), entry.ParticipantIDs,
			entry.Participants, entry.RepeatType, repeatUntil)
		if err != nil {
			return fmt.Errorf("update reservation: %w", err)
		}
		if notify {
			return notifyUpdatedParticipants(ctx, tx, previous, entry)
		}
		return nil
	})
}

func lockManagedReservation(ctx context.Context, tx pgx.Tx, id, actorID int, isAdmin bool) (ListEntry, error) {
	entry, err := lockReservation(ctx, tx, id)
	if err != nil {
		return ListEntry{}, err
	}
	if entry.UserID != actorID && !isAdmin {
		return ListEntry{}, ErrForbidden
	}
	return entry, nil
}

func lockReservation(ctx context.Context, tx pgx.Tx, id int) (ListEntry, error) {
	entry, err := scanReservation(tx.QueryRow(ctx, reservationSelect+` WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ListEntry{}, ErrNotFound
	}
	if err != nil {
		return ListEntry{}, fmt.Errorf("lock reservation: %w", err)
	}
	return entry, nil
}
