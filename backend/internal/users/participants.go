package users

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Participant struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	AvatarColor string `json:"avatarColor"`
}

func ListParticipants(ctx context.Context, pool *pgxpool.Pool) ([]Participant, error) {
	rows, err := pool.Query(ctx, `SELECT id, name, avatar_color FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Participant])
}
