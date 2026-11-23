package reservations

import (
	"context"
	"fmt"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ListEntry struct {
	Reservation
	ID             int
	UserID         int
	Name           string
	NumberOfPeople int
	Type           string
	Purpose        string
	SeriesID       *string
	ParticipantIDs []int
	Participants   string
	RepeatType     string
	RepeatUntil    *time.Time
}

const reservationSelect = `
	SELECT id, user_id, name, room, number_of_people, type, purpose, series_id,
	       date_ticks, start_time_ticks, end_time_ticks, participant_ids,
	       participants, repeat_type, repeat_until_ticks
	FROM reservations`

// StartingBetween selects live reservations in (after, through] in start order.
// The arguments are wall times represented as zone-free .NET ticks.
func StartingBetween(ctx context.Context, pool *pgxpool.Pool, after, through time.Time) ([]ListEntry, error) {
	rows, err := pool.Query(ctx, reservationSelect+`
		WHERE start_time_ticks > $1 AND start_time_ticks <= $2
		ORDER BY start_time_ticks, id`, dotnettime.Ticks(after), dotnettime.Ticks(through))
	if err != nil {
		return nil, fmt.Errorf("query upcoming reservations: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ListEntry, error) { return scanReservation(row) })
}

func List(ctx context.Context, pool *pgxpool.Pool) ([]ListEntry, error) {
	rows, err := pool.Query(ctx, reservationSelect)
	if err != nil {
		return nil, fmt.Errorf("query reservations: %w", err)
	}
	defer rows.Close()

	entries := []ListEntry{}
	for rows.Next() {
		entry, err := scanReservation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reservation: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read reservations: %w", err)
	}
	return entries, nil
}

func scanReservation(row pgx.Row) (ListEntry, error) {
	var entry ListEntry
	var date, startTime, endTime int64
	var repeatUntil *int64
	if err := row.Scan(
		&entry.ID, &entry.UserID, &entry.Name, &entry.Room, &entry.NumberOfPeople,
		&entry.Type, &entry.Purpose, &entry.SeriesID, &date, &startTime, &endTime,
		&entry.ParticipantIDs, &entry.Participants, &entry.RepeatType, &repeatUntil,
	); err != nil {
		return ListEntry{}, err
	}
	entry.Date = dotnettime.FromTicks(date)
	entry.StartTime = dotnettime.FromTicks(startTime)
	entry.EndTime = dotnettime.FromTicks(endTime)
	if repeatUntil != nil {
		entry.RepeatUntil = new(dotnettime.FromTicks(*repeatUntil))
	}
	return entry, nil
}
