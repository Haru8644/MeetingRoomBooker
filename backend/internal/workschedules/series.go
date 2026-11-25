package workschedules

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errWorkScheduleTargetsChanged = errors.New("work schedule series gained an unlocked target")
	errWorkScheduleNotInSeries    = errors.New("work schedule is outside an external appointment series")
	errWorkScheduleNoTargets      = errors.New("work schedule series scope is empty")
)

// mutateWorkScheduleSeries checks the scope and permissions against locked rows.
// A scope that gains an unlocked row is retried with a fresh transaction.
func mutateWorkScheduleSeries(ctx context.Context, pool *pgxpool.Pool, id, actorID int, isAdmin bool, scope string,
	mutate func(pgx.Tx, []storedEntry) error,
) error {
	for {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
			targets, err := lockWorkScheduleTargets(ctx, tx, id, scope == "following")
			if err != nil {
				return err
			}
			for _, entry := range targets {
				if entry.CreatedByUserID != actorID && !isAdmin {
					return ErrForbidden
				}
			}
			return mutate(tx, targets)
		})
		if !errors.Is(err, errWorkScheduleTargetsChanged) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
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
	// anchor first could deadlock two mutations anchored on different rows.
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
		return nil, errWorkScheduleNotInSeries
	}

	// A single edit may change the anchor's date, start or type while we
	// wait. Re-evaluate the scope and restart to lock any new IDs in order.
	targetIDs, err := workScheduleTargetIDs(ctx, tx, selected, following)
	if err != nil {
		return nil, err
	}
	if len(targetIDs) == 0 {
		return nil, errWorkScheduleNoTargets
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

func compareScheduleOccurrences(a, b storedEntry) int {
	start := func(entry storedEntry) int64 {
		if entry.startTicks == nil {
			return entry.dateTicks
		}
		return *entry.startTicks
	}
	return cmp.Or(cmp.Compare(a.dateTicks, b.dateTicks), cmp.Compare(start(a), start(b)), cmp.Compare(a.ID, b.ID))
}

func workScheduleParticipants(groups ...[]storedEntry) []int {
	var recipients []int
	seen := make(map[int]bool)
	for _, entries := range groups {
		for _, entry := range entries {
			for _, id := range entry.ParticipantIDs {
				if id > 0 && !seen[id] {
					recipients = append(recipients, id)
					seen[id] = true
				}
			}
		}
	}
	return recipients
}

func workScheduleSeriesScopeLabel(scope string) string {
	if scope == "following" {
		return "今回以降"
	}
	return "全て"
}
