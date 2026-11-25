package auth

import (
	"context"
	"errors"
	"strings"
	"unicode/utf16"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/jackc/pgx/v5"
)

var ErrUserNotFound = errors.New("user not found")

type UserValidationError string

func (e UserValidationError) Error() string { return string(e) }

func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	if s.pool == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.pool.Query(ctx, `SELECT `+userColumns+` FROM users u ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		var user User
		err := user.scan(row)
		return user, err
	})
}

func (s *Service) UpdateUserName(ctx context.Context, id int, name string) error {
	return s.mutateUser(ctx, id, func(tx pgx.Tx, _ bool) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return UserValidationError("Name is required.")
		}
		if len(utf16.Encode([]rune(name))) > 100 {
			return UserValidationError("Name must be 100 characters or less.")
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET name = $2 WHERE id = $1`, id, name); err != nil {
			return err
		}
		return reservations.RenameOwner(ctx, tx, id, name)
	})
}

func (s *Service) UpdateChatworkAccountID(ctx context.Context, id int, value *string) error {
	return s.updateChatworkID(ctx, id, value, `UPDATE users SET chatwork_account_id = $2 WHERE id = $1`,
		"Chatwork account ID must be 100 characters or less.")
}

func (s *Service) UpdateChatworkDirectRoomID(ctx context.Context, id int, value *string) error {
	return s.updateChatworkID(ctx, id, value, `UPDATE users SET chatwork_direct_room_id = $2 WHERE id = $1`,
		"Chatwork direct room ID must be 100 characters or less.")
}

func (s *Service) updateChatworkID(ctx context.Context, id int, value *string, query, limitMessage string) error {
	return s.mutateUser(ctx, id, func(tx pgx.Tx, _ bool) error {
		value = normalizeChatworkID(value)
		if value != nil && len(utf16.Encode([]rune(*value))) > 100 {
			return UserValidationError(limitMessage)
		}
		_, err := tx.Exec(ctx, query, id, value)
		return err
	})
}

func (s *Service) DeleteUser(ctx context.Context, id, actorID int) error {
	return s.mutateUser(ctx, id, func(tx pgx.Tx, isAdmin bool) error {
		if isAdmin {
			return UserValidationError("管理者ユーザーは削除できません。")
		}
		if id == actorID {
			return UserValidationError("自分自身は削除できません。")
		}
		deletedReservations, err := reservations.RemoveUser(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM notifications WHERE user_id = $1 OR target_reservation_id = ANY($2::integer[])`,
			id, deletedReservations); err != nil {
			return err
		}
		// The sessions foreign key revokes all sessions as part of the deletion.
		_, err = tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
		return err
	})
}

func (s *Service) mutateUser(ctx context.Context, id int, mutate func(pgx.Tx, bool) error) error {
	if s.pool == nil {
		return ErrUnavailable
	}
	return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		var isAdmin bool
		err := tx.QueryRow(ctx, `SELECT is_admin FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&isAdmin)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		return mutate(tx, isAdmin)
	})
}
