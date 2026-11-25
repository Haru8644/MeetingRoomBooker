package reservations

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// RenameOwner updates the display name on every reservation the user owns.
func RenameOwner(ctx context.Context, tx pgx.Tx, userID int, name string) error {
	entries, err := lockUserReservations(ctx, tx, userID, false)
	if err != nil {
		return err
	}
	ids := make([]int, 0, len(entries))
	for _, entry := range entries {
		if entry.Name != name {
			ids = append(ids, entry.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE reservations SET name = $2 WHERE id = ANY($1::integer[])`, ids, name)
	return err
}

// RemoveUser deletes owned reservations and removes the user from other
// reservations. Administrative cleanup does not issue cancellation notices.
func RemoveUser(ctx context.Context, tx pgx.Tx, userID int) ([]int, error) {
	entries, err := lockUserReservations(ctx, tx, userID, true)
	if err != nil {
		return nil, err
	}
	owned := []ListEntry{}
	ownedIDs := []int{}
	for _, entry := range entries {
		if entry.UserID == userID {
			owned = append(owned, entry)
			ownedIDs = append(ownedIDs, entry.ID)
		}
	}
	if len(owned) != 0 {
		if err := lockReservationSlots(ctx, tx, owned); err != nil {
			return nil, err
		}
		if err := deleteReservationRows(ctx, tx, ownedIDs); err != nil {
			return nil, err
		}
	}
	for _, entry := range entries {
		if entry.UserID == userID {
			continue
		}
		remaining := make([]int, 0, len(entry.ParticipantIDs))
		seen := map[int]bool{userID: true}
		for _, id := range entry.ParticipantIDs {
			if !seen[id] {
				seen[id] = true
				remaining = append(remaining, id)
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE reservations SET participant_ids = $2, number_of_people = cardinality($2::integer[])
			WHERE id = $1`, entry.ID, remaining); err != nil {
			return nil, err
		}
	}
	return ownedIDs, nil
}

func lockUserReservations(ctx context.Context, tx pgx.Tx, userID int, includeParticipation bool) ([]ListEntry, error) {
	rows, err := tx.Query(ctx, reservationSelect+`
		WHERE user_id = $1 OR ($2::boolean AND $1 = ANY(participant_ids))
		ORDER BY id FOR UPDATE`, userID, includeParticipation)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ListEntry, error) {
		return scanReservation(row)
	})
}
