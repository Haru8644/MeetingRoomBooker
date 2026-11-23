package chatwork

import (
	"context"
	"log"
	"slices"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

const reminderType = "Reminder10Minutes"

// RunReservationReminders polls immediately, then waits one minute between completed passes.
func (sender *Sender) RunReservationReminders(ctx context.Context) {
	runReminderLoop(ctx, sender.ProcessReservationReminders)
}

func runReminderLoop(ctx context.Context, process func(context.Context, time.Time) error) {
	for ctx.Err() == nil {
		if err := process(ctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Printf("Chatwork reservation reminders: could not process upcoming reservations")
		}
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(time.Minute)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// ProcessReservationReminders runs one pass using the supplied local wall clock.
func (sender *Sender) ProcessReservationReminders(ctx context.Context, now time.Time) error {
	wallTime := dotnettime.OnDate(now, now)
	queryCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	entries, err := reservations.StartingBetween(queryCtx, sender.pool, wallTime, wallTime.Add(10*time.Minute))
	cancel()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		sender.sendReservationReminder(ctx, entry)
	}
	return ctx.Err()
}

func (sender *Sender) sendReservationReminder(ctx context.Context, entry reservations.ListEntry) {
	// Unlike request-triggered DMs, a reminder keeps its worker's cancellation.
	ctx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()
	participants := reservationParticipants([]reservations.ListEntry{entry})
	stakeholders := append(slices.Clone(participants), entry.UserID)
	recipients, err := sender.loadRecipients(ctx, stakeholders)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("Chatwork %s: could not load recipients for reservation %d", reminderType, entry.ID)
		}
		return
	}
	message := stakeholderMessage(entry, participants, stakeholders, recipients, "会議開始10分前です", "10分後に開始します", nil)
	// SQLite reloads the C# start as an Unspecified DateTime. Its round-trip
	// format uses seven fractional digits and omits any timezone suffix.
	const roundTripLayout = "2006-01-02T15:04:05.0000000"
	sender.sendToStakeholders(ctx, reminderType, ":start:"+entry.StartTime.Format(roundTripLayout), entry, stakeholders, recipients, message)
}
