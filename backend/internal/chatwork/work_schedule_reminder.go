package chatwork

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const workScheduleReminderType = "WorkScheduleReminder10Minutes"

func (sender *Sender) processWorkScheduleReminders(ctx context.Context, now time.Time) error {
	after, through := reminderWindow(now)
	queryCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	entries, err := workschedules.StartingExternalAppointments(queryCtx, sender.pool, after, through)
	cancel()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		sender.sendWorkScheduleReminder(ctx, entry)
	}
	return ctx.Err()
}

func (sender *Sender) sendWorkScheduleReminder(ctx context.Context, entry workschedules.ReminderEntry) {
	ctx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()
	var participants []int
	seen := make(map[int]bool)
	for _, id := range entry.ParticipantIDs {
		if id > 0 && !seen[id] {
			participants = append(participants, id)
			seen[id] = true
		}
	}
	if len(participants) == 0 {
		return
	}
	recipients, err := sender.loadRecipients(ctx, participants)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("Chatwork %s: could not load recipients for work schedule %d", workScheduleReminderType, entry.ID)
		}
		return
	}
	display := entry.Participants
	if strings.TrimSpace(display) == "" {
		display = "未設定"
	}
	message := strings.Join([]string{
		"[info][title]社外予定リマインド[/title]",
		"10分後に開始します。",
		"内容: " + entry.Title,
		"日付: " + entry.Date.Format("2006/01/02"),
		"時間: " + entry.Start.Format("15:04") + "〜" + entry.End.Format("15:04"),
		"対象者: " + display,
		"[/info]",
	}, "\n")
	subject := deliverySubject{workScheduleEntryID: &entry.ID, scheduledStart: entry.Start}
	sender.sendToRecipients(ctx, workScheduleReminderType, reminderStartKey(entry.Start), subject, participants, recipients, message)
}
