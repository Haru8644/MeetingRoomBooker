package workschedules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deletion struct {
	ID             int
	ParticipantIDs []int
	ScheduledStart time.Time
	ChatworkBody   string
}

// Delete removes one locked entry and saves its in-app notices in the same transaction.
func Delete(ctx context.Context, pool *pgxpool.Pool, id, actorID int, isAdmin bool) (Deletion, error) {
	var result Deletion
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		entry, err := lockManagedWorkSchedule(ctx, tx, id, actorID, isAdmin)
		if err != nil {
			return err
		}
		deleted, err := tx.Exec(ctx, `DELETE FROM work_schedule_entries WHERE id = $1`, entry.ID)
		if err != nil {
			return fmt.Errorf("delete work schedule: %w", err)
		}
		if deleted.RowsAffected() != 1 {
			return fmt.Errorf("work schedule %d was not deleted", entry.ID)
		}

		var recipients []int
		seen := make(map[int]bool, len(entry.ParticipantIDs))
		for _, userID := range entry.ParticipantIDs {
			if userID > 0 && !seen[userID] {
				recipients = append(recipients, userID)
				seen[userID] = true
			}
		}
		result = Deletion{ID: entry.ID, ParticipantIDs: recipients, ScheduledStart: entry.scheduledStart()}
		if len(recipients) == 0 {
			return nil
		}

		label := scheduleTypeLabel(entry.Type)
		notice := label + "「" + entry.Title + "」が削除されました。日付: " + entry.calendarDate()
		clock := time.Now()
		now := dotnettime.Ticks(dotnettime.OnDate(clock, clock))
		if err := saveWorkScheduleNotices(ctx, tx, entry, recipients, "Info", notice, now); err != nil {
			return err
		}

		body := []string{"[info][title]" + label + "が削除されました[/title]", "内容: " + entry.Title}
		body = append(body, entry.summary()...)
		result.ChatworkBody = strings.Join(append(body, "[/info]"), "\n")
		return nil
	})
	if err != nil {
		return Deletion{}, err
	}
	return result, nil
}
