package workschedules

import (
	"context"
	"fmt"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	ID              int     `json:"id"`
	CreatedByUserID int     `json:"createdByUserId"`
	Type            int     `json:"type"`
	Title           string  `json:"title"`
	SeriesID        *string `json:"seriesId"`
	Date            string  `json:"date"`
	StartTime       *string `json:"startTime"`
	EndTime         *string `json:"endTime"`
	RepeatType      *string `json:"repeatType"`
	RepeatUntil     *string `json:"repeatUntil"`
	LeavePeriod     int     `json:"leavePeriod"`
	ParticipantIDs  []int   `json:"participantIds"`
	Participants    string  `json:"participants"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

func List(ctx context.Context, pool *pgxpool.Pool, from, to time.Time) ([]Entry, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, created_by_user_id, type, title, series_id, date_ticks, start_time_ticks, end_time_ticks,
		       repeat_type, repeat_until_ticks, leave_period, participant_ids, participants, created_at_ticks, updated_at_ticks
		FROM work_schedule_entries
		WHERE date_ticks >= $1 AND date_ticks <= $2
		ORDER BY date_ticks, COALESCE(start_time_ticks, date_ticks), type, id`, dotnettime.Ticks(from), dotnettime.Ticks(to))
	if err != nil {
		return nil, fmt.Errorf("query work schedule entries: %w", err)
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		var entry Entry
		var date, createdAt, updatedAt int64
		var start, end, repeatUntil *int64
		if err := rows.Scan(&entry.ID, &entry.CreatedByUserID, &entry.Type, &entry.Title, &entry.SeriesID,
			&date, &start, &end, &entry.RepeatType, &repeatUntil, &entry.LeavePeriod, &entry.ParticipantIDs,
			&entry.Participants, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan work schedule entry: %w", err)
		}
		entry.Date = dotnettime.FromTicks(date).Format(dotnettime.Layout)
		entry.StartTime = nullableDateTime(start)
		entry.EndTime = nullableDateTime(end)
		entry.RepeatUntil = nullableDateTime(repeatUntil)
		entry.CreatedAt = dotnettime.FromTicks(createdAt).Format(dotnettime.Layout)
		entry.UpdatedAt = dotnettime.FromTicks(updatedAt).Format(dotnettime.Layout)
		if entry.ParticipantIDs == nil {
			entry.ParticipantIDs = []int{}
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read work schedule entries: %w", err)
	}
	return entries, nil
}

func nullableDateTime(ticks *int64) *string {
	if ticks == nil {
		return nil
	}
	return new(dotnettime.FromTicks(*ticks).Format(dotnettime.Layout))
}
