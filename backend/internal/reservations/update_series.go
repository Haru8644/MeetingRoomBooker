package reservations

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errSeriesTargetsChanged = errors.New("series gained an unlocked reservation")

func UpdateSeries(ctx context.Context, pool *pgxpool.Pool, id int, template ListEntry, actorID int, isAdmin bool, scope string, notify bool) error {
	for {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
			if scope != "all" && scope != "following" {
				base, err := lockManagedReservation(ctx, tx, id, actorID, isAdmin)
				if err != nil {
					return err
				}
				return updateSingleReservation(ctx, tx, base, template, notify)
			}
			base, targets, err := lockSeriesUpdateTargets(ctx, tx, id, actorID, isAdmin, scope == "following")
			if err != nil {
				return err
			}
			if !isRecurringReservation(base) {
				return updateSingleReservation(ctx, tx, base, template, notify)
			}
			return updateRecurringReservations(ctx, tx, base, template, targets, notify)
		})
		if !errors.Is(err, errSeriesTargetsChanged) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func lockSeriesUpdateTargets(ctx context.Context, tx pgx.Tx, id, actorID int, isAdmin, following bool) (ListEntry, []ListEntry, error) {
	base, err := findReservation(ctx, tx, id, false)
	if err != nil {
		return ListEntry{}, nil, err
	}
	if base.UserID != actorID && !isAdmin {
		return ListEntry{}, nil, ErrForbidden
	}
	var ids []int
	if isRecurringReservation(base) {
		ids, err = seriesTargetIDs(ctx, tx, base, following)
		if err != nil {
			return ListEntry{}, nil, err
		}
	}
	// Include the base even when its padded key does not match its stored value.
	// All rows precede slot locks, and all multi-row writers take rows by ID.
	rows, err := tx.Query(ctx, reservationSelect+` WHERE id = ANY($1::integer[]) ORDER BY id FOR UPDATE`, append(ids, id))
	if err != nil {
		return ListEntry{}, nil, fmt.Errorf("lock series reservations: %w", err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ListEntry, error) { return scanReservation(row) })
	if err != nil {
		return ListEntry{}, nil, fmt.Errorf("read locked series reservations: %w", err)
	}
	locked := make(map[int]ListEntry, len(entries))
	for _, entry := range entries {
		locked[entry.ID] = entry
	}
	base, found := locked[id]
	if !found {
		return ListEntry{}, nil, ErrNotFound
	}
	if base.UserID != actorID && !isAdmin {
		return ListEntry{}, nil, ErrForbidden
	}
	if !isRecurringReservation(base) {
		return base, nil, nil
	}
	// A writer may have committed while we waited. Resolve the scope again
	// using the locked base and retry if it now requires any unlocked rows.
	ids, err = seriesTargetIDs(ctx, tx, base, following)
	if err != nil {
		return ListEntry{}, nil, err
	}
	targets := make([]ListEntry, 0, len(ids))
	for _, targetID := range ids {
		entry, found := locked[targetID]
		if !found {
			return ListEntry{}, nil, errSeriesTargetsChanged
		}
		targets = append(targets, entry)
	}
	return base, targets, nil
}

func seriesTargetIDs(ctx context.Context, tx pgx.Tx, base ListEntry, following bool) ([]int, error) {
	const query = `SELECT id FROM reservations WHERE user_id = $1 AND date_ticks >= $2`
	var startTicks int64
	if following {
		startTicks = reservationDay(base.Date) * dotnettime.TicksPerDay
	}
	args := []any{base.UserID, startTicks}
	var match string
	if base.SeriesID != nil && strings.TrimSpace(*base.SeriesID) != "" {
		match = ` AND series_id = $3`
		args = append(args, strings.TrimSpace(*base.SeriesID))
	} else {
		match = ` AND name = $3 AND room = $4 AND type = $5 AND purpose = $6 AND repeat_type = $7
			AND repeat_until_ticks / 864000000000 = $8
			AND start_time_ticks % 864000000000 = $9 AND end_time_ticks % 864000000000 = $10`
		args = append(args, base.Name, base.Room, base.Type, base.Purpose, base.RepeatType,
			reservationDay(*base.RepeatUntil), timeOfDayTicks(base.StartTime), timeOfDayTicks(base.EndTime))
	}
	rows, err := tx.Query(ctx, query+match+` ORDER BY date_ticks, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("query series targets: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, fmt.Errorf("read series targets: %w", err)
	}
	return ids, nil
}

func updateRecurringReservations(ctx context.Context, tx pgx.Tx, base, template ListEntry, targets []ListEntry, notify bool) error {
	if err := validateReservationUpdate(template); err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	offset := reservationDay(template.Date) - reservationDay(base.Date)
	updated := make([]ListEntry, len(targets))
	for i, previous := range targets {
		current := previous
		current.Room, current.NumberOfPeople, current.Type, current.Purpose = template.Room, template.NumberOfPeople, template.Type, template.Purpose
		current.ParticipantIDs, current.Participants = template.ParticipantIDs, template.Participants
		current.Date = dotnettime.FromTicks((reservationDay(previous.Date) + offset) * dotnettime.TicksPerDay)
		if current.Date.Year() < 1 || current.Date.Year() > 9999 {
			return fmt.Errorf("updated date is out of range for reservation %d", current.ID)
		}
		current.StartTime = dotnettime.OnDate(current.Date, template.StartTime)
		current.EndTime = dotnettime.OnDate(current.Date, template.EndTime)
		if err := validateReservationUpdate(current); err != nil {
			return err
		}
		updated[i] = current
	}
	// Take the same keys as creates, for both the vacated and destination slots.
	if err := lockReservationSlots(ctx, tx, slices.Concat(targets, updated)); err != nil {
		return err
	}
	for _, entry := range updated {
		if err := saveReservationUpdate(ctx, tx, entry); err != nil {
			return err
		}
	}
	if notify {
		for i, entry := range updated {
			if err := notifyUpdatedParticipants(ctx, tx, targets[i], entry); err != nil {
				return err
			}
		}
	}
	return nil
}
