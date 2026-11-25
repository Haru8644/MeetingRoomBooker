package roomconflicts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/jackc/pgx/v5"
)

type detectionCandidate struct {
	reservations.Reservation
	id int
}

// Detect stores the unresolved pairs whose overlap began during the supplied wall-clock window.
func (s *Store) Detect(ctx context.Context, now time.Time, lookback time.Duration) (int, error) {
	// Subtract wall time, as with DateTime.Now - TimeSpan across a DST change.
	now = dotnettime.FromTicks(wallTicks(now))
	from := now.Add(-lookback)
	var created int
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, room, date_ticks, start_time_ticks, end_time_ticks
			FROM reservations WHERE end_time_ticks > $1 AND start_time_ticks <= $2
			ORDER BY start_time_ticks, id`, dotnettime.Ticks(from), dotnettime.Ticks(now))
		if err != nil {
			return err
		}
		candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (detectionCandidate, error) {
			var c detectionCandidate
			var date, start, end int64
			err := row.Scan(&c.id, &c.Room, &date, &start, &end)
			c.Date, c.StartTime, c.EndTime = dotnettime.FromTicks(date), dotnettime.FromTicks(start), dotnettime.FromTicks(end)
			return c, err
		})
		if err != nil {
			return err
		}
		for i, left := range candidates {
			for _, right := range candidates[i+1:] {
				if err := ctx.Err(); err != nil {
					return err
				}
				start := right.StartTime // Candidates are ordered by start time.
				if !start.Before(left.EndTime) {
					break
				}
				if start.Before(from) || !reservations.IsConflicting(left.Reservation, right.Reservation) {
					continue
				}
				end := left.EndTime
				if right.EndTime.Before(end) {
					end = right.EndTime
				}
				a, b := min(left.id, right.id), max(left.id, right.id)
				key := fmt.Sprintf("%s|%s|%d|%d|%s|%s", strings.TrimSpace(left.Room),
					left.Date.Format("20060102"), a, b, start.Format("20060102150405"), end.Format("20060102150405"))
				// Skip existing keys before identity allocation. The unique constraint
				// also handles a concurrent insert between the check and the write.
				result, err := tx.Exec(ctx, `
					INSERT INTO room_conflict_records (type, status, occurred_at_ticks, room_name,
						reservation_id_a, reservation_id_b, impact, cause, description, resolution, detection_key, created_at_ticks)
					SELECT $1,$2,$3,$4,$5,$6,$7,$8,'同じ会議室の予約が未解消のまま重複時間帯を迎えました。','',$9,$10
					WHERE NOT EXISTS (SELECT 1 FROM room_conflict_records WHERE detection_key = $9)
					ON CONFLICT (detection_key) DO NOTHING`, unresolvedReservationOverlap, Detected,
					dotnettime.Ticks(start), left.Room, a, b, Medium, UnknownCause, key, dotnettime.Ticks(now))
				if err != nil {
					return err
				}
				created += int(result.RowsAffected())
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}
