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

func notifyAddedParticipants(ctx context.Context, tx pgx.Tx, entry ListEntry) error {
	recipients := notifiableParticipants(entry)
	if len(recipients) == 0 {
		return nil
	}

	warning, err := participantConflictWarning(ctx, tx, entry)
	if err != nil {
		return fmt.Errorf("check participant conflicts: %w", err)
	}
	return saveReservationNotifications(ctx, tx, entry, recipients, notificationType(warning), addedParticipantMessage(entry, warning))
}

func notifiableParticipants(entry ListEntry) []int {
	recipients := make([]int, 0, len(entry.ParticipantIDs))
	seen := make(map[int]bool, len(entry.ParticipantIDs))
	for _, id := range entry.ParticipantIDs {
		if id > 0 && id != entry.UserID && !seen[id] {
			recipients = append(recipients, id)
			seen[id] = true
		}
	}
	return recipients
}

func notificationType(warning string) string {
	if warning != "" {
		return "Warning"
	}
	return "Info"
}

func reservationLabel(entry ListEntry) string {
	if strings.TrimSpace(entry.Purpose) == "" {
		return entry.Name
	}
	return entry.Purpose
}

func reservationTimeRange(entry ListEntry) string {
	return entry.StartTime.Format("15:04") + "〜" + entry.EndTime.Format("15:04")
}

func addedParticipantMessage(entry ListEntry, warning string) string {
	return fmt.Sprintf("%sさんが予約「%s」の参加メンバーにあなたを追加しました。利用日: %s / 会議室: %s / 時間: %s%s",
		entry.Name, reservationLabel(entry), entry.Date.Format("2006/01/02"), entry.Room, reservationTimeRange(entry), warning)
}

func saveReservationNotifications(ctx context.Context, tx pgx.Tx, entry ListEntry, recipients []int, kind, message string) error {
	return saveMatchingReservationNotifications(ctx, tx, entry, recipients, kind, message, "")
}

func saveMatchingReservationNotifications(ctx context.Context, tx pgx.Tx, entry ListEntry, recipients []int, kind, message, prefix string) error {
	if len(recipients) == 0 {
		return nil
	}
	now := time.Now()

	// Keep the newest matching notice. Participation uses a shared prefix for joins and leaves.
	_, err := tx.Exec(ctx, `
		WITH recipients AS (
		    SELECT user_id FROM unnest($1::integer[]) AS recipient(user_id)
		), matches AS (
		    SELECT n.id, n.user_id, row_number() OVER (
		        PARTITION BY n.user_id ORDER BY n.created_at_ticks DESC, n.id
		    ) AS position
		    FROM notifications n JOIN recipients r ON r.user_id = n.user_id
		    WHERE n.target_reservation_id = $2 AND n.type = $3
		      AND CASE WHEN $7::text = '' THEN n.message = $4 ELSE starts_with(n.message, $7) END
		), refreshed AS (
		    UPDATE notifications n
		    SET message = $4, created_at_ticks = $5, target_date_ticks = $6, is_read = false
		    FROM matches m WHERE n.id = m.id AND m.position = 1
		), removed AS (
		    DELETE FROM notifications n USING matches m WHERE n.id = m.id AND m.position > 1
		)
		INSERT INTO notifications
		    (user_id, target_reservation_id, type, message, created_at_ticks, target_date_ticks)
		SELECT r.user_id, $2, $3, $4, $5, $6 FROM recipients r
		WHERE NOT EXISTS (SELECT 1 FROM matches m WHERE m.user_id = r.user_id)`,
		recipients, entry.ID, kind, message,
		dotnettime.Ticks(dotnettime.OnDate(now, now)), dotnettime.Ticks(entry.Date), prefix)
	if err != nil {
		return fmt.Errorf("save reservation notifications: %w", err)
	}
	return nil
}

func participantConflictWarning(ctx context.Context, tx pgx.Tx, entry ListEntry) (string, error) {
	const externalAppointment = 0
	dateTicks := dotnettime.Ticks(entry.Date)
	midnightTicks := dateTicks - dateTicks%dotnettime.TicksPerDay
	participants := entry.ParticipantIDs
	if entry.UserID > 0 && !slices.Contains(participants, entry.UserID) {
		participants = append(slices.Clone(participants), entry.UserID)
	}
	rows, err := tx.Query(ctx, `
		SELECT COALESCE(u.name, 'ユーザー' || p.id::text) AS participant_name,
		       e.title, e.start_time_ticks, e.end_time_ticks, count(*) OVER ()
		FROM work_schedule_entries e
		JOIN unnest($1::integer[]) AS p(id) ON p.id = ANY(e.participant_ids)
		LEFT JOIN users u ON u.id = p.id
		WHERE e.type = $2 AND e.date_ticks = $3
		  AND e.start_time_ticks < $4 AND e.end_time_ticks > $5
		ORDER BY e.start_time_ticks, participant_name, e.id, p.id
		LIMIT 5`, participants, externalAppointment, midnightTicks,
		dotnettime.Ticks(entry.EndTime), dotnettime.Ticks(entry.StartTime))
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var lines []string
	var total int
	for rows.Next() {
		var participant, title string
		var start, end int64
		if err := rows.Scan(&participant, &title, &start, &end, &total); err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("%s: 社外予定「%s」 %s %s〜%s", participant, title,
			entry.Date.Format("2006/01/02"), dotnettime.FromTicks(start).Format("15:04"), dotnettime.FromTicks(end).Format("15:04")))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", nil
	}
	warning := " 注意: 参加者の予定重複があります。" + strings.Join(lines, " ")
	if total > len(lines) {
		warning += fmt.Sprintf(" ほか%d件", total-len(lines))
	}
	return warning, nil
}
