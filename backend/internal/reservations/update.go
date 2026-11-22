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
	ErrNotFound   = errors.New("reservation not found")
	ErrForbidden  = errors.New("reservation belongs to another user")
	ErrIDMismatch = errors.New("reservation ID does not match route")
)

type ValidationError string

func (e ValidationError) Error() string { return string(e) }

func Update(ctx context.Context, pool *pgxpool.Pool, entry ListEntry, actorID int, isAdmin, notify bool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		previous, err := lockManagedReservation(ctx, tx, entry.ID, actorID, isAdmin)
		if err != nil {
			return err
		}
		return updateSingleReservation(ctx, tx, previous, entry, notify)
	})
}

func updateSingleReservation(ctx context.Context, tx pgx.Tx, previous, entry ListEntry, notify bool) error {
	if previous.ID != entry.ID {
		return ErrIDMismatch
	}
	entry.UserID, entry.Name, entry.SeriesID = previous.UserID, previous.Name, previous.SeriesID
	if err := validateReservationUpdate(entry); err != nil {
		return err
	}
	// Edits allow overlaps. A create waiting on the destination checks after the edit commits.
	if err := lockReservationSlot(ctx, tx, entry.Room, reservationDay(entry.Date)); err != nil {
		return err
	}
	if err := saveReservationUpdate(ctx, tx, entry); err != nil {
		return err
	}
	if notify {
		return notifyUpdatedParticipants(ctx, tx, previous, entry)
	}
	return nil
}

func validateReservationUpdate(entry ListEntry) error {
	if !entry.StartTime.Before(entry.EndTime) {
		return ValidationError("終了時刻は開始時刻より後にしてください。")
	}
	if isRecurringReservation(entry) && reservationDay(*entry.RepeatUntil) < reservationDay(entry.Date) {
		return ValidationError("繰り返し終了日は予約日以降にしてください。")
	}
	return nil
}

func saveReservationUpdate(ctx context.Context, tx pgx.Tx, entry ListEntry) error {
	var repeatUntil *int64
	if entry.RepeatUntil != nil {
		repeatUntil = new(dotnettime.Ticks(*entry.RepeatUntil))
	}
	_, err := tx.Exec(ctx, `
		UPDATE reservations
		SET room = $2, number_of_people = $3, type = $4, purpose = $5,
		    date_ticks = $6, start_time_ticks = $7, end_time_ticks = $8,
		    participant_ids = $9, participants = $10, repeat_type = $11, repeat_until_ticks = $12
		WHERE id = $1`, entry.ID, entry.Room, entry.NumberOfPeople, entry.Type, entry.Purpose,
		dotnettime.Ticks(entry.Date), dotnettime.Ticks(entry.StartTime), dotnettime.Ticks(entry.EndTime),
		entry.ParticipantIDs, entry.Participants, entry.RepeatType, repeatUntil)
	if err != nil {
		return fmt.Errorf("update reservation: %w", err)
	}
	return nil
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
	return findReservation(ctx, tx, id, true)
}

func findReservation(ctx context.Context, tx pgx.Tx, id int, forUpdate bool) (ListEntry, error) {
	query := reservationSelect + ` WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	entry, err := scanReservation(tx.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ListEntry{}, ErrNotFound
	}
	if err != nil {
		return ListEntry{}, fmt.Errorf("read reservation: %w", err)
	}
	return entry, nil
}
