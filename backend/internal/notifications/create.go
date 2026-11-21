package notifications

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (inbox *Inbox) Create(ctx context.Context, entry Entry, createdAtTicks, targetDateTicks int64) (int, error) {
	const insert = `
		INSERT INTO notifications (id, user_id, message, created_at_ticks, target_date_ticks,
		                           is_read, type, target_reservation_id, target_work_schedule_entry_id)
		VALUES (COALESCE(NULLIF($1, 0), nextval(pg_get_serial_sequence('notifications', 'id'))),
		        $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`
	args := []any{entry.ID, entry.UserID, entry.Message, createdAtTicks, targetDateTicks,
		entry.IsRead, entry.Type, entry.TargetReservationID, entry.TargetWorkScheduleEntryID}
	var id int
	if entry.ID <= 0 {
		err := inbox.pool.QueryRow(ctx, insert, args...).Scan(&id)
		return id, err
	}

	err := pgx.BeginFunc(ctx, inbox.pool, func(tx pgx.Tx) error {
		// Coordinate explicit positive IDs with the default INSERT path. Take the lock
		// before touching rows so the inbox mutations can finish their row-lock transactions.
		if _, err := tx.Exec(ctx, `LOCK TABLE notifications IN EXCLUSIVE MODE`); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, insert, args...).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			SELECT setval(seq, $1, true)
			FROM (SELECT pg_get_serial_sequence('notifications', 'id')::regclass AS seq) AS identity_sequence
			WHERE COALESCE(pg_sequence_last_value(seq), 0) <= $1`, id)
		return err
	})
	return id, err
}
