package chatwork

import (
	"context"
	"log"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const (
	workScheduleUpdateType       = "WorkScheduleUpdated"
	workScheduleSeriesUpdateType = "WorkScheduleSeriesUpdated"
)

// SendWorkScheduleUpdate sends a committed event. Reusing the result keeps its delivery keys.
func (sender *Sender) SendWorkScheduleUpdate(ctx context.Context, result workschedules.UpdateResult) {
	if result.ChangeID == "" {
		return
	}
	ctx, cancel := deliveryContext(ctx)
	defer cancel()
	deliveryType := workScheduleUpdateType
	subject := deliverySubject{workScheduleEntryID: &result.Entry.ID}
	if result.Scope != "" {
		deliveryType = workScheduleSeriesUpdateType
		subject.workScheduleSeriesID = *result.Entry.SeriesID
		subject.workScheduleScope = result.Scope
	}
	var users []int
	for _, delivery := range result.Deliveries {
		users = append(users, delivery.ParticipantIDs...)
	}
	recipients, err := sender.loadRecipients(ctx, users)
	if err != nil {
		log.Printf("Chatwork %s: could not load recipients for work schedule %d", deliveryType, result.Entry.ID)
		return
	}
	for _, delivery := range result.Deliveries {
		subject.scheduledStart = delivery.ScheduledStart
		sender.sendToRecipients(ctx, deliveryType, ":change:"+result.ChangeID,
			subject, delivery.ParticipantIDs, recipients, delivery.Message)
	}
}
