package reservations

import (
	"context"
	"fmt"
	"slices"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func UpdateSeries(ctx context.Context, pool *pgxpool.Pool, id int, template ListEntry, actorID int, isAdmin bool, scope string, notify bool) (UpdateResult, error) {
	var result UpdateResult
	err := mutateReservationSeries(ctx, pool, id, actorID, isAdmin, scope,
		func(tx pgx.Tx, base ListEntry) error {
			var err error
			result, err = updateSingleReservation(ctx, tx, base, template, notify)
			return err
		},
		func(tx pgx.Tx, base ListEntry, targets []ListEntry) error {
			var err error
			result, err = updateRecurringReservations(ctx, tx, base, template, targets, notify)
			return err
		})
	if err != nil {
		return UpdateResult{}, err
	}
	if result.Count > 0 {
		result.ChangeID = newReservationUUID()
	}
	return result, nil
}

func updateRecurringReservations(ctx context.Context, tx pgx.Tx, base, template ListEntry, targets []ListEntry, notify bool) (UpdateResult, error) {
	if err := validateReservationUpdate(template); err != nil {
		return UpdateResult{}, err
	}
	if len(targets) == 0 {
		return UpdateResult{}, nil
	}
	offset := reservationDay(template.Date) - reservationDay(base.Date)
	updated := make([]ListEntry, len(targets))
	for i, previous := range targets {
		current := previous
		current.Room, current.NumberOfPeople, current.Type, current.Purpose = template.Room, template.NumberOfPeople, template.Type, template.Purpose
		current.ParticipantIDs, current.Participants = template.ParticipantIDs, template.Participants
		current.Date = dotnettime.FromTicks((reservationDay(previous.Date) + offset) * dotnettime.TicksPerDay)
		if current.Date.Year() < 1 || current.Date.Year() > 9999 {
			return UpdateResult{}, fmt.Errorf("updated date is out of range for reservation %d", current.ID)
		}
		current.StartTime = dotnettime.OnDate(current.Date, template.StartTime)
		current.EndTime = dotnettime.OnDate(current.Date, template.EndTime)
		if err := validateReservationUpdate(current); err != nil {
			return UpdateResult{}, err
		}
		updated[i] = current
	}
	// Take the same keys as creates, for both the vacated and destination slots.
	if err := lockReservationSlots(ctx, tx, slices.Concat(targets, updated)); err != nil {
		return UpdateResult{}, err
	}
	first := 0
	for i, entry := range updated {
		if err := saveReservationUpdate(ctx, tx, entry); err != nil {
			return UpdateResult{}, err
		}
		if entry.Date.Before(updated[first].Date) || (entry.Date.Equal(updated[first].Date) && entry.ID < updated[first].ID) {
			first = i
		}
	}
	if notify {
		for i, entry := range updated {
			if err := notifyUpdatedParticipants(ctx, tx, targets[i], entry); err != nil {
				return UpdateResult{}, err
			}
		}
	}
	return UpdateResult{Previous: targets[first], Current: updated[first], Count: len(updated)}, nil
}
