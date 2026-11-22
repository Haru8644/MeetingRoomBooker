package reservations

import (
	"context"
	"fmt"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Delete(ctx context.Context, pool *pgxpool.Pool, id, actorID int, isAdmin bool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		entry, err := lockManagedReservation(ctx, tx, id, actorID, isAdmin)
		if err != nil {
			return err
		}
		day := dotnettime.Ticks(entry.Date) / dotnettime.TicksPerDay
		// Use the create/update slot lock so a waiting create can see the freed slot.
		if err := lockReservationSlot(ctx, tx, entry.Room, day); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM reservations WHERE id = $1`, entry.ID); err != nil {
			return fmt.Errorf("delete reservation: %w", err)
		}
		return nil
	})
}
