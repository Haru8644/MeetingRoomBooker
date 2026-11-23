package reservations

import (
	"context"
	"fmt"

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

// Creation describes committed reservations and their pre-insert room overlaps.
type Creation struct {
	Entries  []ListEntry
	Overlaps []ListEntry
}

func Create(ctx context.Context, pool *pgxpool.Pool, entry ListEntry, allowOverlap, captureOverlaps bool) (Creation, error) {
	var overlaps []ListEntry
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		// Check conflicts in a new statement after obtaining the lock so it sees the previous writer's commit.
		if err := lockReservationSlot(ctx, tx, entry.Room, reservationDay(entry.Date)); err != nil {
			return err
		}
		var err error
		overlaps, err = checkRoomConflicts(ctx, tx, entry, allowOverlap, captureOverlaps)
		if err != nil {
			return err
		}
		if err := insertReservation(ctx, tx, &entry); err != nil {
			return err
		}
		return notifyAddedParticipants(ctx, tx, entry)
	})
	if err != nil {
		return Creation{}, err
	}
	return Creation{Entries: []ListEntry{entry}, Overlaps: overlaps}, nil
}

func insertReservation(ctx context.Context, tx pgx.Tx, entry *ListEntry) error {
	var repeatUntil *int64
	if entry.RepeatUntil != nil {
		repeatUntil = new(dotnettime.Ticks(*entry.RepeatUntil))
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO reservations (user_id, name, room, number_of_people, type, purpose,
		    date_ticks, start_time_ticks, end_time_ticks, participant_ids, participants, repeat_type,
		    series_id, repeat_until_ticks)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id`, entry.UserID, entry.Name, entry.Room, entry.NumberOfPeople, entry.Type, entry.Purpose,
		dotnettime.Ticks(entry.Date), dotnettime.Ticks(entry.StartTime), dotnettime.Ticks(entry.EndTime),
		entry.ParticipantIDs, entry.Participants, entry.RepeatType, entry.SeriesID, repeatUntil).Scan(&entry.ID)
	if err != nil {
		return fmt.Errorf("insert reservation: %w", err)
	}
	return nil
}

func lockReservationSlot(ctx context.Context, tx pgx.Tx, room string, day int64) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), $2::integer)`, room, int32(day)); err != nil {
		return fmt.Errorf("lock room and date: %w", err)
	}
	return nil
}

func checkRoomConflicts(ctx context.Context, tx pgx.Tx, entry ListEntry, allowOverlap, captureOverlaps bool) ([]ListEntry, error) {
	if allowOverlap && !captureOverlaps {
		return nil, nil
	}
	var limit *int
	if !allowOverlap {
		limit = new(1)
	}
	rows, err := tx.Query(ctx, reservationSelect+`
		WHERE hashtext(room) = hashtext($1) AND room = $1 AND date_ticks / 864000000000 = $2
		  AND start_time_ticks < $3 AND end_time_ticks > $4
		ORDER BY date_ticks, start_time_ticks, id LIMIT $5`, entry.Room, reservationDay(entry.Date),
		dotnettime.Ticks(entry.EndTime), dotnettime.Ticks(entry.StartTime), limit)
	if err != nil {
		return nil, fmt.Errorf("query reservation conflicts: %w", err)
	}
	overlaps, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ListEntry, error) { return scanReservation(row) })
	if err != nil {
		return nil, fmt.Errorf("read reservation conflicts: %w", err)
	}
	if !allowOverlap && len(overlaps) > 0 {
		return nil, reservationConflict(overlaps[0])
	}
	return overlaps, nil
}

func reservationConflict(entry ListEntry) *ConflictError {
	return &ConflictError{Reservation: entry.Reservation, Label: reservationLabel(entry)}
}
