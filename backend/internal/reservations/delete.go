package reservations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Delete(ctx context.Context, pool *pgxpool.Pool, id, actorID int, isAdmin bool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		entry, err := lockManagedReservation(ctx, tx, id, actorID, isAdmin)
		if err != nil {
			return err
		}
		return deleteSingleReservation(ctx, tx, entry)
	})
}

func DeleteSeries(ctx context.Context, pool *pgxpool.Pool, id, actorID int, isAdmin bool, scope string) error {
	return mutateReservationSeries(ctx, pool, id, actorID, isAdmin, scope,
		func(tx pgx.Tx, base ListEntry) error {
			return deleteSingleReservation(ctx, tx, base)
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
			return deleteReservationRows(ctx, tx, ids)
		})
}

func deleteSingleReservation(ctx context.Context, tx pgx.Tx, entry ListEntry) error {
	// Use the create/update slot lock so a waiting create can see the freed slot.
	if err := lockReservationSlot(ctx, tx, entry.Room, reservationDay(entry.Date)); err != nil {
		return err
	}
	return deleteReservationRows(ctx, tx, []int{entry.ID})
}

func deleteReservationRows(ctx context.Context, tx pgx.Tx, ids []int) error {
	if _, err := tx.Exec(ctx, `DELETE FROM reservations WHERE id = ANY($1::integer[])`, ids); err != nil {
		return fmt.Errorf("delete reservations: %w", err)
	}
	return nil
}
