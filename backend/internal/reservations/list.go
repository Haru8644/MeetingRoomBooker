package reservations

import (
	"context"
	"fmt"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
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

func List(ctx context.Context, pool *pgxpool.Pool) ([]ListEntry, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, name, room, number_of_people, type, purpose, series_id,
		       date_ticks, start_time_ticks, end_time_ticks, participant_ids,
		       participants, repeat_type, repeat_until_ticks
		FROM reservations`)
	if err != nil {
		return nil, fmt.Errorf("query reservations: %w", err)
	}
	defer rows.Close()

	entries := []ListEntry{}
	for rows.Next() {
		var entry ListEntry
		var date, startTime, endTime int64
		var repeatUntil *int64
		if err := rows.Scan(
			&entry.ID, &entry.UserID, &entry.Name, &entry.Room, &entry.NumberOfPeople,
			&entry.Type, &entry.Purpose, &entry.SeriesID, &date, &startTime, &endTime,
			&entry.ParticipantIDs, &entry.Participants, &entry.RepeatType, &repeatUntil,
		); err != nil {
			return nil, fmt.Errorf("scan reservation: %w", err)
		}
		entry.Date = dotnettime.FromTicks(date)
		entry.StartTime = dotnettime.FromTicks(startTime)
		entry.EndTime = dotnettime.FromTicks(endTime)
		if repeatUntil != nil {
			value := dotnettime.FromTicks(*repeatUntil)
			entry.RepeatUntil = &value
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read reservations: %w", err)
	}
	return entries, nil
}
