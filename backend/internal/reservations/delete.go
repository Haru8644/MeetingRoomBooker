package reservations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Delete(ctx context.Context, pool *pgxpool.Pool, id, actorID int, isAdmin bool) ([]ListEntry, error) {
	return DeleteSeries(ctx, pool, id, actorID, isAdmin, "single")
}

// DeleteSeries returns the locked snapshots of the deleted rows after commit.
func DeleteSeries(ctx context.Context, pool *pgxpool.Pool, id, actorID int, isAdmin bool, scope string) ([]ListEntry, error) {
	var deleted []ListEntry
	err := mutateReservationSeries(ctx, pool, id, actorID, isAdmin, scope,
		func(tx pgx.Tx, base ListEntry) error {
			if err := deleteSingleReservation(ctx, tx, base); err != nil {
				return err
			}
			deleted = []ListEntry{base}
			return nil
		},
		func(tx pgx.Tx, _ ListEntry, targets []ListEntry) error {
			if len(targets) == 0 {
				return nil
			}
			if err := lockReservationSlots(ctx, tx, targets); err != nil {
				return err
			}
			ids := make([]int, len(targets))
			for i, target := range targets {
				ids[i] = target.ID
			}
			if err := deleteReservationRows(ctx, tx, ids); err != nil {
				return err
			}
			deleted = targets
			return nil
		})
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

func deleteSingleReservation(ctx context.Context, tx pgx.Tx, entry ListEntry) error {
	// Use the create/update slot lock so a waiting create can see the freed slot.
	if err := lockReservationSlot(ctx, tx, entry.Room, reservationDay(entry.Date)); err != nil {
		return err
	}
	return deleteReservationRows(ctx, tx, []int{entry.ID})
}

func deleteReservationRows(ctx context.Context, tx pgx.Tx, ids []int) error {
	result, err := tx.Exec(ctx, `DELETE FROM reservations WHERE id = ANY($1::integer[])`, ids)
	if err != nil {
		return fmt.Errorf("delete reservations: %w", err)
	}
	if result.RowsAffected() != int64(len(ids)) {
		return fmt.Errorf("deleted %d of %d locked reservations", result.RowsAffected(), len(ids))
	}
	return nil
}
