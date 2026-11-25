package workschedules

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("work schedule entry not found")
	ErrForbidden = errors.New("work schedule entry is owned by another user")
)

type UpdateDelivery struct {
	ParticipantIDs []int
	ScheduledStart time.Time
	Message        string
}

type UpdateResult struct {
	Entry      Entry
	Scope      string
	ChangeID   string
	Deliveries []UpdateDelivery
}

// Update edits one locked entry and saves its in-app notifications in the same transaction.
func Update(ctx context.Context, pool *pgxpool.Pool, id int, input EntryInput, actorID int, isAdmin bool) (UpdateResult, error) {
	noRepeat := strings.TrimSpace(input.RepeatType) == "" || input.RepeatType == noRepeatType
	switch {
	case input.Type != externalAppointment && (!noRepeat || input.RepeatUntil != nil):
		return UpdateResult{}, ValidationError("繰り返し作成は社外予定のみ指定できます。")
	case !noRepeat:
		return UpdateResult{}, ValidationError("繰り返し予定の一括更新は未対応です。")
	}

	var result UpdateResult
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		previous, err := lockManagedWorkSchedule(ctx, tx, id, actorID, isAdmin)
		if err != nil {
			return err
		}
		if err := input.validateFields(); err != nil {
			return err
		}
		names, err := input.normalize(ctx, tx, previous.CreatedByUserID)
		if err != nil {
			return err
		}
		clock := time.Now()
		now := dotnettime.Ticks(dotnettime.OnDate(clock, clock))
		current, err := writeWorkScheduleUpdate(ctx, tx, id, input, dotnettime.Ticks(input.Date), now)
		if err != nil {
			return err
		}
		result.Entry = current.Entry
		result.Deliveries, err = notifyWorkScheduleUpdate(ctx, tx, previous, current, names, now)
		return err
	})
	if err != nil {
		return UpdateResult{}, err
	}
	if len(result.Deliveries) > 0 {
		result.ChangeID = newWorkScheduleUUID()
	}
	return result, nil
}

func lockManagedWorkSchedule(ctx context.Context, tx pgx.Tx, id, actorID int, isAdmin bool) (storedEntry, error) {
	entry, err := scanStoredEntry(tx.QueryRow(ctx, `SELECT `+entryColumns+` FROM work_schedule_entries WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return storedEntry{}, ErrNotFound
	}
	if err != nil {
		return storedEntry{}, err
	}
	if entry.CreatedByUserID != actorID && !isAdmin {
		return storedEntry{}, ErrForbidden
	}
	return entry, nil
}

func writeWorkScheduleUpdate(ctx context.Context, tx pgx.Tx, id int, input EntryInput, dateTicks, now int64) (storedEntry, error) {
	var start, end *int64
	if input.Start != nil {
		start, end = new(dotnettime.Ticks(*input.Start)), new(dotnettime.Ticks(*input.End))
	}
	return scanStoredEntry(tx.QueryRow(ctx, `
			UPDATE work_schedule_entries
			SET type=$2, title=$3, date_ticks=$4, start_time_ticks=$5, end_time_ticks=$6,
			    leave_period=$7, participant_ids=$8, participants=$9, updated_at_ticks=$10
			WHERE id=$1
			RETURNING `+entryColumns, id, input.Type, input.Title, dateTicks, start, end,
		input.LeavePeriod, input.ParticipantIDs, input.Participants, now))
}
