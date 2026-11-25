package workschedules

import (
	"context"
	"errors"
	"maps"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UpdateSeries edits the selected scope. Single delegates to the single-entry transaction.
func UpdateSeries(ctx context.Context, pool *pgxpool.Pool, id int, input EntryInput, actorID int, isAdmin bool, scope string) (UpdateResult, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "", "single":
		return Update(ctx, pool, id, input, actorID, isAdmin)
	case "following", "all":
	default:
		return UpdateResult{}, ValidationError("反映範囲が不正です。")
	}
	if input.Type != externalAppointment {
		return UpdateResult{}, ValidationError("繰り返し社外予定のみまとめて編集できます。")
	}
	if (strings.TrimSpace(input.RepeatType) != "" && input.RepeatType != noRepeatType) || input.RepeatUntil != nil {
		return UpdateResult{}, ValidationError("繰り返し条件の変更は未対応です。")
	}

	var result UpdateResult
	err := mutateWorkScheduleSeries(ctx, pool, id, actorID, isAdmin, scope, func(tx pgx.Tx, previous []storedEntry) error {
		clock := time.Now()
		now := dotnettime.Ticks(dotnettime.OnDate(clock, clock))
		current := make([]storedEntry, 0, len(previous))
		names := make(map[int]string)
		for _, entry := range previous {
			occurrence := input
			occurrence.Date = dotnettime.OnDate(dotnettime.FromTicks(entry.dateTicks), time.Time{})
			if err := occurrence.validateFields(); err != nil {
				return err
			}
			people, err := occurrence.normalize(ctx, tx, entry.CreatedByUserID)
			if err != nil {
				return err
			}
			saved, err := writeWorkScheduleUpdate(ctx, tx, entry.ID, occurrence, entry.dateTicks, now)
			if err != nil {
				return err
			}
			current = append(current, saved)
			maps.Copy(names, people)
		}
		updated, err := notifyWorkScheduleSeriesUpdate(ctx, tx, previous, current, scope, names, now)
		result = updated
		return err
	})
	switch {
	case errors.Is(err, errWorkScheduleNotInSeries):
		return UpdateResult{}, ValidationError("繰り返し社外予定のみまとめて編集できます。")
	case errors.Is(err, errWorkScheduleNoTargets):
		return UpdateResult{}, ValidationError("更新対象の社外予定がありません。")
	case err != nil:
		return UpdateResult{}, err
	}
	if len(result.Deliveries) > 0 {
		result.ChangeID = newWorkScheduleUUID()
	}
	return result, nil
}
