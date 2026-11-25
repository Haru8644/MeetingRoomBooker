package chatwork

import (
	"context"
	"log"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const workScheduleUpdateType = "WorkScheduleUpdated"

// SendWorkScheduleUpdate sends a committed event. Reusing the result keeps its delivery keys.
func (sender *Sender) SendWorkScheduleUpdate(ctx context.Context, result workschedules.UpdateResult) {
	if result.ChangeID == "" {
		return
	}
	ctx, cancel := deliveryContext(ctx)
	defer cancel()
	var users []int
	for _, delivery := range result.Deliveries {
		users = append(users, delivery.ParticipantIDs...)
	}
	recipients, err := sender.loadRecipients(ctx, users)
	if err != nil {
		log.Printf("Chatwork %s: could not load recipients for work schedule %d", workScheduleUpdateType, result.Entry.ID)
		return
	}
	for _, delivery := range result.Deliveries {
		subject := deliverySubject{workScheduleEntryID: &result.Entry.ID, scheduledStart: delivery.ScheduledStart}
		sender.sendToRecipients(ctx, workScheduleUpdateType, ":change:"+result.ChangeID,
			subject, delivery.ParticipantIDs, recipients, delivery.Message)
	}
}
