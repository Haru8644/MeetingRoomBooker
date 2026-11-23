package workschedules

import (
	"context"
	"fmt"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReminderEntry struct {
	ID             int
	Title          string
	Date           time.Time
	Start          time.Time
	End            time.Time
	Participants   string
	ParticipantIDs []int
}

// StartingExternalAppointments selects timed external appointments in (after, through].
// The arguments are wall times represented as zone-free .NET ticks.
func StartingExternalAppointments(ctx context.Context, pool *pgxpool.Pool, after, through time.Time) ([]ReminderEntry, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, title, date_ticks, start_time_ticks, end_time_ticks, participants, participant_ids
		FROM work_schedule_entries
		WHERE type = 0 AND start_time_ticks IS NOT NULL AND end_time_ticks IS NOT NULL
		  AND start_time_ticks > $1 AND start_time_ticks <= $2
		ORDER BY start_time_ticks, id`, dotnettime.Ticks(after), dotnettime.Ticks(through))
	if err != nil {
		return nil, fmt.Errorf("query upcoming external appointments: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ReminderEntry, error) {
		var entry ReminderEntry
		var date, start, end int64
		if err := row.Scan(&entry.ID, &entry.Title, &date, &start, &end, &entry.Participants, &entry.ParticipantIDs); err != nil {
			return ReminderEntry{}, err
		}
		entry.Date, entry.Start, entry.End = dotnettime.FromTicks(date), dotnettime.FromTicks(start), dotnettime.FromTicks(end)
		return entry, nil
	})
}
