package reservations

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
)

func notifyUpdatedParticipants(ctx context.Context, tx pgx.Tx, previous, entry ListEntry) error {
	before, after := notifiableParticipants(previous), notifiableParticipants(entry)
	kept, added := splitParticipants(after, before)
	_, removed := splitParticipants(before, after)
	if len(kept)+len(added)+len(removed) == 0 {
		return nil
	}
	warning, err := participantConflictWarning(ctx, tx, entry)
	if err != nil {
		return fmt.Errorf("check participant conflicts: %w", err)
	}
	kind := notificationType(warning)
	if err := saveReservationNotifications(ctx, tx, entry, added, kind, addedParticipantMessage(entry, warning)); err != nil {
		return err
	}
	removedMessage := fmt.Sprintf("%sさんが予約「%s」の参加メンバーからあなたを削除しました。", entry.Name, reservationLabel(entry))
	if err := saveReservationNotifications(ctx, tx, entry, removed, "Info", removedMessage); err != nil {
		return err
	}
	if len(kept) == 0 {
		return nil
	}
	names, err := participantNames(ctx, tx, slices.Concat(added, removed))
	if err != nil {
		return fmt.Errorf("read participant names: %w", err)
	}
	message := updatedReservationMessage(previous, entry, warning, formatParticipantNames(added, names), formatParticipantNames(removed, names))
	if message == "" {
		return nil
	}
	return saveReservationNotifications(ctx, tx, entry, kept, kind, message)
}

func splitParticipants(current, previous []int) (retained, added []int) {
	known := make(map[int]bool, len(previous))
	for _, id := range previous {
		known[id] = true
	}
	for _, id := range current {
		if known[id] {
			retained = append(retained, id)
		} else {
			added = append(added, id)
		}
	}
	return retained, added
}

func updatedReservationMessage(previous, current ListEntry, warning, addedNames, removedNames string) string {
	const dateLayout = "2006/01/02"
	var changes []string
	if from, to := previous.Date.Format(dateLayout), current.Date.Format(dateLayout); from != to {
		changes = append(changes, fmt.Sprintf("利用日が %s から %s に変更されました。", from, to))
	}
	if previous.Room != current.Room {
		changes = append(changes, fmt.Sprintf("会議室が %s から %s に変更されました。", previous.Room, current.Room))
	}
	if previous.Type != current.Type {
		changes = append(changes, fmt.Sprintf("区分が %s から %s に変更されました。", previous.Type, current.Type))
	}
	if timeOfDayTicks(previous.StartTime) != timeOfDayTicks(current.StartTime) ||
		timeOfDayTicks(previous.EndTime) != timeOfDayTicks(current.EndTime) {
		changes = append(changes, fmt.Sprintf("時間が %s から %s に変更されました。",
			reservationTimeRange(previous), reservationTimeRange(current)))
	}
	label, currentLabel := reservationLabel(previous), reservationLabel(current)
	if label != currentLabel {
		changes = append(changes, fmt.Sprintf("予約名が「%s」から「%s」に変更されました。", label, currentLabel))
	}
	if strings.TrimSpace(label) == "" {
		label = currentLabel
	}
	if addedNames != "" {
		changes = append(changes, fmt.Sprintf("参加メンバーに %s が追加されました。", addedNames))
	}
	if removedNames != "" {
		changes = append(changes, fmt.Sprintf("参加メンバーから %s が削除されました。", removedNames))
	}
	if len(changes) != 0 {
		return fmt.Sprintf("予約「%s」が更新されました。%s%s", label, strings.Join(changes, " "), warning)
	}
	if warning != "" {
		return fmt.Sprintf("予約「%s」に参加者の予定重複があります。%s", label, warning)
	}
	return ""
}

func timeOfDayTicks(value time.Time) int64 {
	return dotnettime.Ticks(value) % dotnettime.TicksPerDay
}

func participantNames(ctx context.Context, tx pgx.Tx, ids []int) (map[int]string, error) {
	names := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	rows, err := tx.Query(ctx, `SELECT id, name FROM users WHERE id = ANY($1::integer[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		names[id] = name
	}
	return names, rows.Err()
}

func formatParticipantNames(ids []int, names map[int]string) string {
	var selected []string
	for _, id := range ids {
		name, found := names[id]
		if !found {
			name = fmt.Sprintf("ユーザー%d", id)
		}
		if strings.TrimSpace(name) != "" {
			selected = append(selected, name)
		}
	}
	return strings.Join(selected, "、")
}
