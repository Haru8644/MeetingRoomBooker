package workschedules

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
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

const entryColumns = `id, created_by_user_id, type, title, series_id, date_ticks, start_time_ticks, end_time_ticks,
       repeat_type, repeat_until_ticks, leave_period, participant_ids, participants, created_at_ticks, updated_at_ticks`

func List(ctx context.Context, pool *pgxpool.Pool, from, to time.Time) ([]Entry, error) {
	rows, err := pool.Query(ctx, `SELECT `+entryColumns+`
		FROM work_schedule_entries
		WHERE date_ticks >= $1 AND date_ticks <= $2
		ORDER BY date_ticks, COALESCE(start_time_ticks, date_ticks), type, id`, dotnettime.Ticks(from), dotnettime.Ticks(to))
	if err != nil {
		return nil, fmt.Errorf("query work schedule entries: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) {
		return scanEntry(row)
	})
}

func Get(ctx context.Context, pool *pgxpool.Pool, id int) (*Entry, error) {
	entry, err := scanEntry(pool.QueryRow(ctx, `SELECT `+entryColumns+` FROM work_schedule_entries WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &entry, err
}

type storedEntry struct {
	Entry
	dateTicks            int64
	startTicks, endTicks *int64
}

func scanEntry(row pgx.Row) (Entry, error) {
	entry, err := scanStoredEntry(row)
	return entry.Entry, err
}

func scanStoredEntry(row pgx.Row) (storedEntry, error) {
	var entry storedEntry
	var createdAt, updatedAt int64
	var repeatUntil *int64
	err := row.Scan(&entry.ID, &entry.CreatedByUserID, &entry.Type, &entry.Title, &entry.SeriesID,
		&entry.dateTicks, &entry.startTicks, &entry.endTicks, &entry.RepeatType, &repeatUntil, &entry.LeavePeriod, &entry.ParticipantIDs,
		&entry.Participants, &createdAt, &updatedAt)
	if err != nil {
		return storedEntry{}, fmt.Errorf("scan work schedule entry: %w", err)
	}
	entry.Date = dotnettime.FromTicks(entry.dateTicks).Format(dotnettime.Layout)
	entry.StartTime = nullableDateTime(entry.startTicks)
	entry.EndTime = nullableDateTime(entry.endTicks)
	entry.RepeatUntil = nullableDateTime(repeatUntil)
	entry.CreatedAt = dotnettime.FromTicks(createdAt).Format(dotnettime.Layout)
	entry.UpdatedAt = dotnettime.FromTicks(updatedAt).Format(dotnettime.Layout)
	if entry.ParticipantIDs == nil {
		entry.ParticipantIDs = []int{}
	}
	return entry, nil
}

func nullableDateTime(ticks *int64) *string {
	if ticks == nil {
		return nil
	}
	return new(dotnettime.FromTicks(*ticks).Format(dotnettime.Layout))
}
