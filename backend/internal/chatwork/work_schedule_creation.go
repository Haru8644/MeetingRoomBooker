package chatwork

import (
	"context"
	"log"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const workScheduleCreationType = "WorkScheduleCreated"

// SendWorkScheduleCreation sends DMs for a committed schedule and its in-app notifications.
func (sender *Sender) SendWorkScheduleCreation(ctx context.Context, result workschedules.Creation) {
	ctx, cancel := deliveryContext(ctx)
	defer cancel()
	entry := result.Entry
	recipients, err := sender.loadRecipients(ctx, entry.ParticipantIDs)
	if err != nil {
		log.Printf("Chatwork %s: could not load recipients for work schedule %d", workScheduleCreationType, entry.ID)
		return
	}
	subject := deliverySubject{workScheduleEntryID: &entry.ID, scheduledStart: result.ScheduledStart}
	sender.sendToRecipients(ctx, workScheduleCreationType, "", subject, entry.ParticipantIDs, recipients, result.ChatworkBody)
}
