package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUnauthorized = errors.New("authentication failed")
	ErrUnavailable  = errors.New("authentication database unavailable")
)

const sessionLifetime = 14 * 24 * time.Hour

type User struct {
	ID                   int     `json:"id"`
	Name                 string  `json:"name"`
	Email                string  `json:"email"`
	AvatarColor          string  `json:"avatarColor"`
	IsAdmin              bool    `json:"isAdmin"`
	ChatworkAccountID    *string `json:"chatworkAccountId"`
	ChatworkDirectRoomID *string `json:"chatworkDirectRoomId"`
}

type Session struct {
	Token      string
	RememberMe bool
	ExpiresAt  time.Time
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

const userColumns = `u.id, u.name, u.email, u.avatar_color, u.is_admin, u.chatwork_account_id, u.chatwork_direct_room_id`

func (u *User) scan(row pgx.Row, extra ...any) error {
	return row.Scan(append([]any{&u.ID, &u.Name, &u.Email, &u.AvatarColor, &u.IsAdmin,
		&u.ChatworkAccountID, &u.ChatworkDirectRoomID}, extra...)...)
}

func (s *Service) Login(ctx context.Context, email, password string, rememberMe bool, previousToken string) (User, Session, error) {
	if s.pool == nil {
		return User{}, Session{}, ErrUnavailable
	}
	var user User
	var passwordHash *string
	err := user.scan(s.pool.QueryRow(ctx, `SELECT `+userColumns+`, u.password_hash FROM users u WHERE u.email = $1`, email), &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, Session{}, err
	}
	if passwordHash == nil || !verifyPassword(*passwordHash, password) {
		return User{}, Session{}, ErrUnauthorized
	}

	var rawToken [32]byte
	rand.Read(rawToken[:])
	digest := sha256.Sum256(rawToken[:])
	previousDigest := sessionDigest(previousToken)
	now := time.Now().UTC()
	session := Session{
		Token: base64.RawURLEncoding.EncodeToString(rawToken[:]), RememberMe: rememberMe, ExpiresAt: now.Add(sessionLifetime),
	}
	_, err = s.pool.Exec(ctx, `
		WITH removed AS (DELETE FROM sessions WHERE token_hash = $1 OR expires_at <= $2)
		INSERT INTO sessions (token_hash, user_id, remember_me, issued_at, expires_at)
		VALUES ($3, $4, $5, $2, $6)`, previousDigest, now, digest[:], user.ID, rememberMe, session.ExpiresAt)
	if err != nil {
		return User{}, Session{}, err
	}
	return user, session, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, *Session, error) {
	digest := sessionDigest(token)
	if digest == nil {
		return User{}, nil, ErrUnauthorized
	}
	if s.pool == nil {
		return User{}, nil, ErrUnavailable
	}
	var user User
	var issuedAt, expiresAt time.Time
	var rememberMe bool
	now := time.Now().UTC()
	err := user.scan(s.pool.QueryRow(ctx, `SELECT `+userColumns+`, s.issued_at, s.expires_at, s.remember_me
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2`, digest, now), &issuedAt, &expiresAt, &rememberMe)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, nil, ErrUnauthorized
	}
	if err != nil {
		return User{}, nil, err
	}
	if now.Sub(issuedAt) <= expiresAt.Sub(now) {
		return user, nil, nil
	}

	newExpiry := now.Add(sessionLifetime)
	// Only one concurrent request renews the session, and a deleted session stays deleted.
	updated, err := s.pool.Exec(ctx, `UPDATE sessions SET issued_at = $3, expires_at = $4
		WHERE token_hash = $1 AND issued_at = $2 AND expires_at > $3`, digest, issuedAt, now, newExpiry)
	if err != nil {
		return User{}, nil, err
	}
	if updated.RowsAffected() == 0 {
		return user, nil, nil
	}
	return user, &Session{Token: token, RememberMe: rememberMe, ExpiresAt: newExpiry}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	digest := sessionDigest(token)
	if digest == nil {
		return ErrUnauthorized
	}
	if s.pool == nil {
		return ErrUnavailable
	}
	deleted, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1 AND expires_at > $2`, digest, time.Now().UTC())
	if err != nil {
		return err
	}
	if deleted.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func sessionDigest(token string) []byte {
	if len(token) != base64.RawURLEncoding.EncodedLen(32) {
		return nil
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return nil
	}
	digest := sha256.Sum256(decoded)
	return digest[:]
}
