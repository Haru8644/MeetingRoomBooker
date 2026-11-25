package workschedules

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
)

func notifyWorkScheduleUpdate(ctx context.Context, tx pgx.Tx, previous, current storedEntry,
	names map[int]string, now int64,
) ([]UpdateDelivery, error) {
	retained, added, removed := updateParticipants(previous.ParticipantIDs, current.ParticipantIDs)
	if len(removed) > 0 {
		removedNames, err := loadParticipantNames(ctx, tx, removed)
		if err != nil {
			return nil, err
		}
		maps.Copy(names, removedNames)
	}
	changes := scheduleChangeLines(previous, current, added, removed, names)
	var conflicts []scheduleConflict
	if current.Type == externalAppointment && len(current.ParticipantIDs) > 0 {
		var err error
		conflicts, err = findScheduleConflicts(ctx, tx, []int{current.ID}, names)
		if err != nil {
			return nil, err
		}
	}
	kind, noticeWarning, dmWarning := conflictWarnings(conflicts, names)
	currentLabel, previousLabel := scheduleTypeLabel(current.Type), scheduleTypeLabel(previous.Type)
	summary := current.summary()
	currentPrefix := currentLabel + "「" + current.Title + "」"
	var deliveries []UpdateDelivery
	notify := func(users []int, entry storedEntry, kind, notice, dmTitle string, dmLines []string) error {
		if len(users) == 0 {
			return nil
		}
		if err := saveWorkScheduleNotices(ctx, tx, entry, users, kind, notice, now); err != nil {
			return err
		}
		message := "[info][title]" + dmTitle + "[/title]\n" + strings.Join(dmLines, "\n") + "\n[/info]"
		deliveries = append(deliveries, UpdateDelivery{users, entry.scheduledStart(), message})
		return nil
	}
	addedDM := append([]string{"内容: " + current.Title}, summary...)
	if err := notify(added, current, kind,
		currentPrefix+"の対象者にあなたが追加されました。"+strings.Join(summary, " / ")+noticeWarning,
		currentLabel+"の対象者に追加されました", append(addedDM, dmWarning...)); err != nil {
		return nil, err
	}

	previousDate := "日付: " + previous.calendarDate()
	if err := notify(removed, previous, "Info",
		previousLabel+"「"+previous.Title+"」の対象者からあなたが削除されました。"+previousDate,
		previousLabel+"の対象者から削除されました", []string{"内容: " + previous.Title, previousDate}); err != nil {
		return nil, err
	}

	if len(changes) == 0 && len(conflicts) == 0 {
		return deliveries, nil
	}

	var noticeChanges string
	retainedDM := append([]string{"内容: " + current.Title}, summary...)
	if len(changes) > 0 {
		noticeChanges = " 変更点: " + strings.Join(changes, "。 ") + "。"
		retainedDM = append(retainedDM, "[hr]", "変更点:")
		for _, line := range changes {
			retainedDM = append(retainedDM, "- "+line)
		}
	}
	if err := notify(retained, current, kind,
		currentPrefix+"が更新されました。"+strings.Join(summary, " / ")+noticeChanges+noticeWarning,
		currentLabel+"が変更されました", append(retainedDM, dmWarning...)); err != nil {
		return nil, err
	}
	// The in-app inbox writes added, removed, retained. Chatwork tries
	// retained recipients first, then added and removed recipients.
	if len(retained) > 0 {
		last := len(deliveries) - 1
		deliveries = slices.Insert(deliveries[:last], 0, deliveries[last])
	}
	return deliveries, nil
}

func updateParticipants(previous, current []int) (retained, added, removed []int) {
	before := make(map[int]bool, len(previous))
	for _, id := range previous {
		before[id] = true
	}
	after := make(map[int]bool, len(current))
	for _, id := range current {
		after[id] = true
		if before[id] {
			retained = append(retained, id)
		} else {
			added = append(added, id)
		}
	}
	for _, id := range previous {
		if id > 0 && before[id] && !after[id] {
			removed = append(removed, id)
			delete(before, id)
		}
	}
	return retained, added, removed
}

func saveWorkScheduleNotices(ctx context.Context, tx pgx.Tx, entry storedEntry, users []int, kind, message string, now int64) error {
	// C# inserts when no message matches among notices of the same kind,
	// user and entry. If any message matches, it keeps the newest candidate
	// and refreshes it only if that candidate is the match.
	_, err := tx.Exec(ctx, `
		WITH locked AS (
		    SELECT id, user_id, message, created_at_ticks
		    FROM notifications
		    WHERE user_id = ANY($1::integer[]) AND target_work_schedule_entry_id = $2 AND type = $3
		    ORDER BY id FOR UPDATE
		), ranked AS (
		    SELECT *, row_number() OVER (PARTITION BY user_id ORDER BY created_at_ticks DESC, id) AS position
		    FROM locked
		), matches AS (
		    SELECT DISTINCT user_id FROM ranked WHERE message = $4
		), refreshed AS (
		    UPDATE notifications n
		    SET created_at_ticks = $5, target_date_ticks = $6, is_read = false
		    FROM ranked r WHERE n.id = r.id AND r.position = 1 AND r.message = $4
		), removed AS (
		    DELETE FROM notifications n USING ranked r JOIN matches m USING (user_id)
		    WHERE n.id = r.id AND r.position > 1
		)
		INSERT INTO notifications
		    (user_id, target_work_schedule_entry_id, type, message, created_at_ticks, target_date_ticks)
		SELECT u.id, $2, $3, $4, $5, $6 FROM unnest($1::integer[]) AS u(id)
		WHERE NOT EXISTS (SELECT 1 FROM matches m WHERE m.user_id = u.id)`,
		users, entry.ID, kind, message, now, entry.dateTicks)
	if err != nil {
		return fmt.Errorf("save work schedule notifications: %w", err)
	}
	return nil
}

func scheduleChangeLines(before, after storedEntry, added, removed []int, names map[int]string) []string {
	var changes []string
	if before.Type != after.Type {
		changes = append(changes, fmt.Sprintf("種別が %s から %s に変更されました", scheduleTypeLabel(before.Type), scheduleTypeLabel(after.Type)))
	}
	if before.Title != after.Title {
		changes = append(changes, fmt.Sprintf("内容が「%s」から「%s」に変更されました", before.Title, after.Title))
	}
	if from, to := before.calendarDate(), after.calendarDate(); from != to {
		changes = append(changes, fmt.Sprintf("日付が %s から %s に変更されました", from, to))
	}
	if !sameScheduleTime(before.startTicks, after.startTicks) || !sameScheduleTime(before.endTicks, after.endTicks) {
		changes = append(changes, fmt.Sprintf("時間が %s から %s に変更されました", before.timeRange(), after.timeRange()))
	}
	if before.LeavePeriod != after.LeavePeriod {
		changes = append(changes, fmt.Sprintf("休暇区分が %s から %s に変更されました", leavePeriodLabel(before.LeavePeriod), leavePeriodLabel(after.LeavePeriod)))
	}
	if display := changedParticipantNames(added, names); display != "" {
		changes = append(changes, "対象者に "+display+" が追加されました")
	}
	if display := changedParticipantNames(removed, names); display != "" {
		changes = append(changes, "対象者から "+display+" が削除されました")
	}
	return changes
}

func changedParticipantNames(ids []int, names map[int]string) string {
	var values []string
	for _, id := range ids {
		name, found := names[id]
		if !found {
			name = fmt.Sprintf("ユーザー%d", id)
		}
		if strings.TrimSpace(name) != "" {
			values = append(values, name)
		}
	}
	return strings.Join(values, "、")
}

func sameScheduleTime(before, after *int64) bool {
	if before == nil || after == nil {
		return before == after
	}
	return *before == *after
}

func (entry storedEntry) calendarDate() string {
	return dotnettime.FromTicks(entry.dateTicks).Format("2006/01/02")
}

func (entry storedEntry) timeRange() string {
	if entry.startTicks == nil || entry.endTicks == nil {
		return "終日"
	}
	return dotnettime.FromTicks(*entry.startTicks).Format("15:04") + "〜" +
		dotnettime.FromTicks(*entry.endTicks).Format("15:04")
}

func (entry storedEntry) scheduledStart() time.Time {
	if entry.startTicks != nil {
		return dotnettime.FromTicks(*entry.startTicks)
	}
	return dotnettime.OnDate(dotnettime.FromTicks(entry.dateTicks), time.Time{})
}

func (entry storedEntry) summary() []string {
	lines := []string{"日付: " + entry.calendarDate()}
	switch entry.Type {
	case externalAppointment:
		lines = append(lines, "時間: "+entry.timeRange())
	case leave:
		lines = append(lines, "休暇区分: "+leavePeriodLabel(entry.LeavePeriod))
	}
	return append(lines, "対象者: "+participantText(entry.Participants))
}
