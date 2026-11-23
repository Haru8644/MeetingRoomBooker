package chatwork

import (
	"context"
	"log"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const (
	workScheduleCreationType       = "WorkScheduleCreated"
	workScheduleSeriesCreationType = "WorkScheduleSeriesCreated"
)

// SendWorkScheduleCreation sends DMs for a committed schedule or series and its in-app notifications.
func (sender *Sender) SendWorkScheduleCreation(ctx context.Context, result workschedules.Creation) {
	ctx, cancel := deliveryContext(ctx)
	defer cancel()
	entry := result.Entries[0]
	subject := deliverySubject{workScheduleEntryID: &entry.ID, scheduledStart: result.ScheduledStart}
	deliveryType := workScheduleCreationType
	if entry.SeriesID != nil {
		subject.workScheduleSeriesID = *entry.SeriesID
		deliveryType = workScheduleSeriesCreationType
	}
	recipients, err := sender.loadRecipients(ctx, entry.ParticipantIDs)
	if err != nil {
		log.Printf("Chatwork %s: could not load recipients for work schedule %d", deliveryType, entry.ID)
		return
	}
	sender.sendToRecipients(ctx, deliveryType, "", subject, entry.ParticipantIDs, recipients, result.ChatworkBody)
}
