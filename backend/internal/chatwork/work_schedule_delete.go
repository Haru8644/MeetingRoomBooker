package chatwork

import (
	"context"
	"log"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const workScheduleDeleteType = "WorkScheduleDeleted"

// SendWorkScheduleDeletion sends DMs for a committed deletion.
func (sender *Sender) SendWorkScheduleDeletion(ctx context.Context, result workschedules.Deletion) {
	if len(result.ParticipantIDs) == 0 {
		return
	}
	ctx, cancel := deliveryContext(ctx)
	defer cancel()
	recipients, err := sender.loadRecipients(ctx, result.ParticipantIDs)
	if err != nil {
		log.Printf("Chatwork %s: could not load recipients for work schedule %d", workScheduleDeleteType, result.ID)
		return
	}
	subject := deliverySubject{workScheduleEntryID: &result.ID, scheduledStart: result.ScheduledStart}
	sender.sendToRecipients(ctx, workScheduleDeleteType, "", subject, result.ParticipantIDs, recipients, result.ChatworkBody)
}
