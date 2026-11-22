package reservations

import (
	"context"
	"fmt"
	"slices"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ChangeParticipation(ctx context.Context, pool *pgxpool.Pool, id, actorID int, join bool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		entry, err := lockReservation(ctx, tx, id)
		if err != nil {
			return err
		}
		if entry.UserID == actorID && !join {
			return ValidationError("予約者本人は参加メンバーから外せません。")
		}
		participantIndex := slices.Index(entry.ParticipantIDs, actorID)
		if (participantIndex >= 0) == join {
			return nil
		}
		if join {
			entry.ParticipantIDs = append(entry.ParticipantIDs, actorID)
		} else {
			entry.ParticipantIDs = slices.Delete(entry.ParticipantIDs, participantIndex, participantIndex+1)
		}
		oldStart, oldEnd := entry.StartTime, entry.EndTime
		entry.Normalize()
		if !entry.StartTime.Equal(oldStart) || !entry.EndTime.Equal(oldEnd) {
			day := dotnettime.Ticks(entry.Date) / dotnettime.TicksPerDay
			if err := lockReservationSlot(ctx, tx, entry.Room, day); err != nil {
				return err
			}
		}
		var repeatUntil *int64
		if entry.RepeatUntil != nil {
			repeatUntil = new(dotnettime.Ticks(*entry.RepeatUntil))
		}
		if _, err := tx.Exec(ctx, `
			UPDATE reservations
			SET participant_ids = $2, start_time_ticks = $3, end_time_ticks = $4,
			    repeat_type = $5, repeat_until_ticks = $6, series_id = $7
			WHERE id = $1`, entry.ID, entry.ParticipantIDs, dotnettime.Ticks(entry.StartTime),
			dotnettime.Ticks(entry.EndTime), entry.RepeatType, repeatUntil, entry.SeriesID); err != nil {
			return fmt.Errorf("update reservation participation: %w", err)
		}
		if entry.UserID == actorID {
			return nil
		}
		return notifyOrganizerOfParticipation(ctx, tx, entry, actorID, join)
	})
}

func notifyOrganizerOfParticipation(ctx context.Context, tx pgx.Tx, entry ListEntry, actorID int, join bool) error {
	names, err := participantNames(ctx, tx, []int{actorID})
	if err != nil {
		return fmt.Errorf("read participant name: %w", err)
	}
	name, found := names[actorID]
	if !found {
		name = fmt.Sprintf("ユーザー%d", actorID)
	}
	action := "取り消しました"
	if join {
		action = "追加しました"
	}
	prefix := fmt.Sprintf("%sさんが予約「%s」", name, reservationLabel(entry))
	message := fmt.Sprintf("%sへの参加を%s。利用日: %s / 会議室: %s / 時間: %s",
		prefix, action, entry.Date.Format("2006/01/02"), entry.Room, reservationTimeRange(entry))
	return saveMatchingReservationNotifications(ctx, tx, entry, []int{entry.UserID}, "Info", message, prefix)
}
