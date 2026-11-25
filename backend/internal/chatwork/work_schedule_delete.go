package chatwork

import (
	"context"
	"log"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const (
	workScheduleDeleteType       = "WorkScheduleDeleted"
	workScheduleSeriesDeleteType = "WorkScheduleSeriesDeleted"
)

// SendWorkScheduleDeletion sends DMs for a committed deletion.
func (sender *Sender) SendWorkScheduleDeletion(ctx context.Context, result workschedules.Deletion) {
	if len(result.ParticipantIDs) == 0 {
		return
	}
	ctx, cancel := deliveryContext(ctx)
	defer cancel()
	deliveryType := workScheduleDeleteType
	subject := deliverySubject{workScheduleEntryID: &result.ID, scheduledStart: result.ScheduledStart}
	if result.Scope != "" {
		deliveryType = workScheduleSeriesDeleteType
		subject.workScheduleSeriesID = result.SeriesID
		subject.workScheduleScope = result.Scope
	}
	recipients, err := sender.loadRecipients(ctx, result.ParticipantIDs)
	if err != nil {
		log.Printf("Chatwork %s: could not load recipients for work schedule %d", deliveryType, result.ID)
		return
	}
	sender.sendToRecipients(ctx, deliveryType, "", subject, result.ParticipantIDs, recipients, result.ChatworkBody)
}
