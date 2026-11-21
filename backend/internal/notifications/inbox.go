package notifications

import (
	"context"
	"errors"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("notification not found")
	ErrForbidden = errors.New("notification belongs to another user")
)

type Entry struct {
	ID                        int    `json:"id"`
	UserID                    int    `json:"userId"`
	Message                   string `json:"message"`
	CreatedAt                 string `json:"createdAt"`
	TargetDate                string `json:"targetDate"`
	IsRead                    bool   `json:"isRead"`
	Type                      string `json:"type"`
	TargetReservationID       *int   `json:"targetReservationId"`
	TargetWorkScheduleEntryID *int   `json:"targetWorkScheduleEntryId"`
}

type Inbox struct {
	pool *pgxpool.Pool
}

func NewInbox(pool *pgxpool.Pool) *Inbox {
	return &Inbox{pool: pool}
}

func (inbox *Inbox) List(ctx context.Context, userID int) ([]Entry, error) {
	rows, err := inbox.pool.Query(ctx, `
		SELECT id, user_id, message, created_at_ticks, target_date_ticks, is_read, type,
		       target_reservation_id, target_work_schedule_entry_id
		FROM notifications WHERE user_id = $1 ORDER BY created_at_ticks DESC`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) {
		var entry Entry
		var createdAt, targetDate int64
		if err := row.Scan(&entry.ID, &entry.UserID, &entry.Message, &createdAt, &targetDate, &entry.IsRead,
			&entry.Type, &entry.TargetReservationID, &entry.TargetWorkScheduleEntryID); err != nil {
			return Entry{}, err
		}
		entry.CreatedAt = dotnettime.FromTicks(createdAt).Format(dotnettime.Layout)
		entry.TargetDate = dotnettime.FromTicks(targetDate).Format(dotnettime.Layout)
		return entry, nil
	})
}

func (inbox *Inbox) MarkRead(ctx context.Context, id, actorID int, isAdmin bool) error {
	return inbox.mutate(ctx, id, actorID, isAdmin, `UPDATE notifications SET is_read = true WHERE id = $1`)
}

func (inbox *Inbox) Delete(ctx context.Context, id, actorID int, isAdmin bool) error {
	return inbox.mutate(ctx, id, actorID, isAdmin, `DELETE FROM notifications WHERE id = $1`)
}

func (inbox *Inbox) mutate(ctx context.Context, id, actorID int, isAdmin bool, statement string) error {
	return pgx.BeginTxFunc(ctx, inbox.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		// Keep the row stable between checking its owner and applying the mutation.
		var ownerID int
		err := tx.QueryRow(ctx, `SELECT user_id FROM notifications WHERE id = $1 FOR UPDATE`, id).Scan(&ownerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if ownerID != actorID && !isAdmin {
			return ErrForbidden
		}
		_, err = tx.Exec(ctx, statement, id)
		return err
	})
}
