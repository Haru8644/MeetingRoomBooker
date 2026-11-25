package workschedules

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errWorkScheduleTargetsChanged = errors.New("work schedule series gained an unlocked target")

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

	for {
		var result UpdateResult
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
			previous, err := lockWorkScheduleTargets(ctx, tx, id, scope == "following")
			if err != nil {
				return err
			}
			for _, entry := range previous {
				if entry.CreatedByUserID != actorID && !isAdmin {
					return ErrForbidden
				}
			}

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
			result, err = notifyWorkScheduleSeriesUpdate(ctx, tx, previous, current, scope, names, now)
			return err
		})
		switch {
		case errors.Is(err, errWorkScheduleTargetsChanged):
			if ctx.Err() != nil {
				return UpdateResult{}, ctx.Err()
			}
		case err != nil:
			return UpdateResult{}, err
		default:
			if len(result.Deliveries) > 0 {
				result.ChangeID = newWorkScheduleUUID()
			}
			return result, nil
		}
	}
}

func lockWorkScheduleTargets(ctx context.Context, tx pgx.Tx, id int, following bool) ([]storedEntry, error) {
	const selectEntries = `SELECT ` + entryColumns + ` FROM work_schedule_entries`
	selected, err := scanStoredEntry(tx.QueryRow(ctx, selectEntries+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	ids := []int{id}
	if workScheduleSeriesKey(selected) != "" {
		targetIDs, err := workScheduleTargetIDs(ctx, tx, selected, following)
		if err != nil {
			return nil, err
		}
		ids = append(ids, targetIDs...)
	}
	// Take the selected row with the candidates in one order. Locking the
	// anchor first could deadlock two updates anchored on different rows.
	rows, err := tx.Query(ctx, selectEntries+` WHERE id = ANY($1::integer[]) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return nil, fmt.Errorf("lock work schedule series: %w", err)
	}
	locked, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedEntry, error) { return scanStoredEntry(row) })
	if err != nil {
		return nil, err
	}
	byID := make(map[int]storedEntry, len(locked))
	for _, entry := range locked {
		byID[entry.ID] = entry
	}
	selected, ok := byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	if workScheduleSeriesKey(selected) == "" {
		return nil, ValidationError("繰り返し社外予定のみまとめて編集できます。")
	}

	// A single edit may change the anchor's date, start or type while we
	// wait. Re-evaluate the scope and restart to lock any new IDs in order.
	targetIDs, err := workScheduleTargetIDs(ctx, tx, selected, following)
	if err != nil {
		return nil, err
	}
	if len(targetIDs) == 0 {
		return nil, ValidationError("更新対象の社外予定がありません。")
	}
	targets := make([]storedEntry, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		entry, ok := byID[targetID]
		if !ok {
			return nil, errWorkScheduleTargetsChanged
		}
		targets = append(targets, entry)
	}
	return targets, nil
}

func workScheduleSeriesKey(entry storedEntry) string {
	if entry.Type != externalAppointment || entry.SeriesID == nil {
		return ""
	}
	return strings.TrimSpace(*entry.SeriesID)
}

func workScheduleTargetIDs(ctx context.Context, tx pgx.Tx, selected storedEntry, following bool) ([]int, error) {
	day := selected.dateTicks - selected.dateTicks%dotnettime.TicksPerDay
	start := day
	if selected.startTicks != nil {
		start = *selected.startTicks
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM work_schedule_entries
		WHERE series_id = $1 AND type = $2
		  AND (NOT $3::boolean OR date_ticks >= $4 + $5::bigint
		       OR (date_ticks >= $4 AND COALESCE(start_time_ticks, $4) >= $6))
		ORDER BY id`, workScheduleSeriesKey(selected), externalAppointment, following, day, dotnettime.TicksPerDay, start)
	if err != nil {
		return nil, fmt.Errorf("query work schedule series targets: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[int])
}
