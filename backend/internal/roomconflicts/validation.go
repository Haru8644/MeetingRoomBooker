package roomconflicts

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jackc/pgx/v5"
)

type ValidationError string

func (e ValidationError) Error() string { return string(e) }

type Fields struct {
	Status, Impact, Cause   int
	Description, Resolution string
}

type Report struct {
	Fields
	OccurredAt                     time.Time
	RoomName                       string
	ReservationIDA, ReservationIDB *int32
}

func (f *Fields) normalizeText() {
	f.Description = strings.TrimSpace(f.Description)
	f.Resolution = strings.TrimSpace(f.Resolution)
}

func (r Report) validate(occurredAt int64) error {
	switch {
	case occurredAt == 0:
		return ValidationError("OccurredAt is required.")
	case r.RoomName == "":
		return ValidationError("RoomName is required.")
	case len(utf16.Encode([]rune(r.RoomName))) > 100:
		return ValidationError("RoomName must be 100 characters or less.")
	case !validImpact(r.Impact):
		return ValidationError("Impact is invalid.")
	case !validCause(r.Cause):
		return ValidationError("Cause is invalid.")
	case !validStatus(r.Status):
		return ValidationError("Status is invalid.")
	default:
		return r.validateText()
	}
}

func (f Fields) validateUpdate() error {
	switch {
	case !validStatus(f.Status):
		return ValidationError("Status is invalid.")
	case !validImpact(f.Impact):
		return ValidationError("Impact is invalid.")
	case !validCause(f.Cause):
		return ValidationError("Cause is invalid.")
	default:
		return f.validateText()
	}
}

func (f Fields) validateText() error {
	switch {
	case len(utf16.Encode([]rune(f.Description))) > 1000:
		return ValidationError("Description must be 1000 characters or less.")
	case len(utf16.Encode([]rune(f.Resolution))) > 1000:
		return ValidationError("Resolution must be 1000 characters or less.")
	default:
		return nil
	}
}

func validStatus(status int) bool { return status >= Detected && status <= Resolved }

func validImpact(impact int) bool { return impact >= Low && impact <= High }

func validCause(cause int) bool {
	return cause >= 0 && cause <= 5 || cause == UnknownCause || cause == 99
}

func validateReservations(ctx context.Context, tx pgx.Tx, a, b *int32) error {
	if a != nil && b != nil && *a == *b {
		return ValidationError("ReservationIdA and ReservationIdB must be different.")
	}
	ids := make([]int32, 0, 2)
	for _, id := range []*int32{a, b} {
		if id != nil {
			ids = append(ids, *id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	// Keep the referenced rows present through the insert without a persistent foreign key.
	rows, err := tx.Query(ctx, `SELECT id FROM reservations WHERE id = ANY($1::integer[]) ORDER BY id FOR KEY SHARE`, ids)
	if err != nil {
		return err
	}
	existing, err := pgx.CollectRows(rows, pgx.RowTo[int32])
	if err != nil {
		return err
	}
	var missing []string
	for _, id := range ids {
		if !slices.Contains(existing, id) {
			missing = append(missing, strconv.FormatInt(int64(id), 10))
		}
	}
	if len(missing) > 0 {
		return ValidationError(fmt.Sprintf("Reservation was not found: %s.", strings.Join(missing, ", ")))
	}
	return nil
}
