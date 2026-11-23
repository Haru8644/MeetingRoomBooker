package reservations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateSeries takes normalized occurrences in the order determined from the input dates and start times.
func CreateSeries(ctx context.Context, pool *pgxpool.Pool, entries []ListEntry, allowOverlap, captureOverlaps bool) (Creation, error) {
	if len(entries) == 0 {
		return Creation{}, ValidationError("登録する予約がありません。")
	}
	entries = slices.Clone(entries)
	seriesID := newReservationUUID()

	var overlaps []ListEntry
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		if err := lockReservationSlots(ctx, tx, entries); err != nil {
			return err
		}
		earlier := make(map[reservationSlot][]ListEntry)
		seenOverlaps := make(map[int]bool)
		for i, entry := range entries {
			if err := validateSeriesOccurrence(entry); err != nil {
				return err
			}
			entries[i].SeriesID = nil
			if entry.RepeatUntil != nil {
				entries[i].SeriesID = &seriesID
			}
			if !allowOverlap {
				slot := reservationSlot{entry.Room, reservationDay(entry.Date)}
				for _, previous := range earlier[slot] {
					if IsConflicting(previous.Reservation, entry.Reservation) {
						return reservationConflict(previous)
					}
				}
				earlier[slot] = append(earlier[slot], entry)
			}
			matches, err := checkRoomConflicts(ctx, tx, entry, allowOverlap, captureOverlaps)
			if err != nil {
				return err
			}
			for _, existing := range matches {
				if !seenOverlaps[existing.ID] {
					overlaps = append(overlaps, existing)
					seenOverlaps[existing.ID] = true
				}
			}
		}
		for i := range entries {
			if err := insertReservation(ctx, tx, &entries[i]); err != nil {
				return err
			}
		}
		return notifyAddedSeriesParticipants(ctx, tx, entries)
	})
	if err != nil {
		return Creation{}, err
	}
	return Creation{Entries: entries, Overlaps: overlaps}, nil
}

func newReservationUUID() string {
	var uuid [16]byte
	rand.Read(uuid[:])
	uuid[6] = uuid[6]&0x0f | 0x40
	uuid[8] = uuid[8]&0x3f | 0x80
	return hex.EncodeToString(uuid[:])
}

func validateSeriesOccurrence(entry ListEntry) error {
	if entry.RepeatUntil != nil && (entry.Date.Weekday() == time.Saturday || entry.Date.Weekday() == time.Sunday) {
		return ValidationError("繰り返し予約では土日を登録できません。")
	}
	if !entry.StartTime.Before(entry.EndTime) {
		return ValidationError("終了時刻は開始時刻より後にしてください。")
	}
	if entry.RepeatUntil != nil && reservationDay(*entry.RepeatUntil) < reservationDay(entry.Date) {
		return ValidationError("繰り返し終了日は予約日以降にしてください。")
	}
	return nil
}

type reservationSlot struct {
	Room string
	Day  int64
}

func reservationDay(date time.Time) int64 {
	return dotnettime.Ticks(date) / dotnettime.TicksPerDay
}

func lockReservationSlots(ctx context.Context, tx pgx.Tx, entries []ListEntry) error {
	rooms := make([]string, len(entries))
	days := make([]int32, len(entries))
	for i, entry := range entries {
		rooms[i] = entry.Room
		days[i] = int32(reservationDay(entry.Date))
	}
	// Sort the actual lock keys so that even distinct room names that hash to
	// the same key cannot lead two series to take the locks in opposite orders.
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (hashtext(room), day) room, day
		FROM unnest($1::text[], $2::integer[]) AS slots(room, day)
		ORDER BY hashtext(room), day`, rooms, days)
	if err != nil {
		return fmt.Errorf("query series lock keys: %w", err)
	}
	slots, err := pgx.CollectRows(rows, pgx.RowToStructByPos[reservationSlot])
	if err != nil {
		return fmt.Errorf("read series lock keys: %w", err)
	}
	for _, slot := range slots {
		if err := lockReservationSlot(ctx, tx, slot.Room, slot.Day); err != nil {
			return err
		}
	}
	return nil
}
