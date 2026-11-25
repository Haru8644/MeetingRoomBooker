package auth

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

var (
	ErrRegistrationForbidden = errors.New("registration requires an administrator")
	ErrEmailConflict         = errors.New("email already exists")
)

const defaultAvatarColor = "#58a6ff"

type Registration struct {
	Name                 string
	Email                string
	Password             string
	AvatarColor          string
	ChatworkAccountID    *string
	ChatworkDirectRoomID *string
}

// Register serializes bootstrapping and email admission with the insert.
func (s *Service) Register(ctx context.Context, input Registration, actorID *int) (User, error) {
	if s.pool == nil {
		return User{}, ErrUnavailable
	}
	name := strings.TrimSpace(input.Name)
	email := NormalizeEmail(input.Email)
	avatar := input.AvatarColor
	if strings.TrimSpace(avatar) == "" {
		avatar = defaultAvatarColor
	}
	var user User
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		// This lock conflicts with other registrations and user writes,
		// including when the table is empty. Readers can continue.
		if _, err := tx.Exec(ctx, `LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		var hasUsers, actorIsAdmin, duplicate bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM users),
			       EXISTS (SELECT 1 FROM users WHERE id = $1 AND is_admin),
			       EXISTS (SELECT 1 FROM users WHERE email = $2)`,
			actorID, email).Scan(&hasUsers, &actorIsAdmin, &duplicate)
		if err != nil {
			return err
		}
		if hasUsers && !actorIsAdmin {
			return ErrRegistrationForbidden
		}
		if duplicate {
			return ErrEmailConflict
		}

		passwordHash, err := hashPassword(input.Password)
		if err != nil {
			return err
		}
		return user.scan(tx.QueryRow(ctx, `
			INSERT INTO users AS u (name, email, password_hash, avatar_color, is_admin, chatwork_account_id, chatwork_direct_room_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING `+userColumns, name, email, passwordHash, avatar, !hasUsers,
			normalizeChatworkID(input.ChatworkAccountID), normalizeChatworkID(input.ChatworkDirectRoomID)))
	})
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// NormalizeEmail uses the case mapping applied by ASP.NET's ToLowerInvariant.
func NormalizeEmail(email string) string {
	return strings.Map(func(r rune) rune {
		// .NET invariant casing leaves the Latin capital I with dot unchanged.
		if r == 'İ' {
			return r
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(email))
}

func normalizeChatworkID(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
