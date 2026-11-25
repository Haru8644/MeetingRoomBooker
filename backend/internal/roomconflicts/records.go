package roomconflicts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	unresolvedReservationOverlap = iota
	actualRoomCollision
)

const (
	Detected = iota
	Confirmed
	FalseAlarm
	Resolved
)

const (
	Low = iota
	Medium
	High
)

const UnknownCause = 98

var (
	ErrNotFound  = errors.New("room conflict record not found")
	ErrForbidden = errors.New("room conflict record cannot be edited by this user")
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type Record struct {
	ID               int     `json:"id"`
	Type             int     `json:"type"`
	Status           int     `json:"status"`
	OccurredAt       string  `json:"occurredAt"`
	RoomName         string  `json:"roomName"`
	ReservationIDA   *int32  `json:"reservationIdA"`
	ReservationIDB   *int32  `json:"reservationIdB"`
	Impact           int     `json:"impact"`
	Cause            int     `json:"cause"`
	Description      string  `json:"description"`
	Resolution       string  `json:"resolution"`
	DetectionKey     *string `json:"detectionKey"`
	ReportedByUserID *int    `json:"reportedByUserId"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        *string `json:"updatedAt"`
}

func (r Record) EditableBy(userID int, admin bool) bool {
	return admin || (r.ReportedByUserID != nil && *r.ReportedByUserID == userID)
}

type Summary struct {
	UnresolvedOverlapsThisMonth  int `json:"unresolvedOverlapsThisMonth"`
	ConfirmedCollisionsThisMonth int `json:"confirmedCollisionsThisMonth"`
	HighImpactConflictsThisMonth int `json:"highImpactConflictsThisMonth"`
	OpenDetectedRecords          int `json:"openDetectedRecords"`
}

const recordColumns = `id, type, status, occurred_at_ticks, room_name, reservation_id_a,
	reservation_id_b, impact, cause, description, resolution, detection_key,
	reported_by_user_id, created_at_ticks, updated_at_ticks`

const selectRecords = `SELECT ` + recordColumns + ` FROM room_conflict_records`

func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.pool.Query(ctx, selectRecords+` ORDER BY occurred_at_ticks DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Record, error) {
		return scanRecord(row)
	})
}

func (s *Store) Get(ctx context.Context, id int) (Record, error) {
	return scanRecord(s.pool.QueryRow(ctx, selectRecords+` WHERE id = $1`, id))
}

func (s *Store) Summary(ctx context.Context, now time.Time) (Summary, error) {
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	from, through := dotnettime.Ticks(month), dotnettime.Ticks(month.AddDate(0, 1, 0))
	var summary Summary
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE in_month AND type = $3),
		       count(*) FILTER (WHERE in_month AND type = $4 AND status = $5),
		       count(*) FILTER (WHERE in_month AND impact = $6),
		       count(*) FILTER (WHERE status = $7)
		FROM (
			SELECT type, status, impact, occurred_at_ticks >= $1 AND occurred_at_ticks < $2 AS in_month
			FROM room_conflict_records
		) records`,
		from, through, unresolvedReservationOverlap, actualRoomCollision, Confirmed, High, Detected,
	).Scan(&summary.UnresolvedOverlapsThisMonth, &summary.ConfirmedCollisionsThisMonth,
		&summary.HighImpactConflictsThisMonth, &summary.OpenDetectedRecords)
	return summary, err
}

func (s *Store) Create(ctx context.Context, report Report, userID int) (Record, error) {
	report.RoomName = strings.TrimSpace(report.RoomName)
	report.Fields.normalizeText()
	occurredAt := wallTicks(report.OccurredAt)
	if err := report.validate(occurredAt); err != nil {
		return Record{}, err
	}
	var created Record
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := validateReservations(ctx, tx, report.ReservationIDA, report.ReservationIDB); err != nil {
			return err
		}
		if report.Status == Detected {
			return ValidationError("Manual conflict reports cannot be created with Detected status.")
		}
		var err error
		created, err = scanRecord(tx.QueryRow(ctx, `
			INSERT INTO room_conflict_records (type, status, occurred_at_ticks, room_name,
				reservation_id_a, reservation_id_b, impact, cause, description, resolution,
				reported_by_user_id, created_at_ticks)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING `+recordColumns, actualRoomCollision, report.Status, occurredAt, report.RoomName,
			report.ReservationIDA, report.ReservationIDB, report.Impact, report.Cause, report.Description,
			report.Resolution, userID, wallTicks(time.Now())))
		return err
	})
	return created, err
}

func (s *Store) Update(ctx context.Context, id, userID int, admin bool, fields Fields) (Record, error) {
	var updated Record
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		previous, err := scanRecord(tx.QueryRow(ctx, selectRecords+` WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if !previous.EditableBy(userID, admin) {
			return ErrForbidden
		}
		fields.normalizeText()
		if err := fields.validateUpdate(); err != nil {
			return err
		}
		updated, err = scanRecord(tx.QueryRow(ctx, `
			UPDATE room_conflict_records
			SET status = $2, impact = $3, cause = $4, description = $5, resolution = $6, updated_at_ticks = $7
			WHERE id = $1 RETURNING `+recordColumns, id, fields.Status, fields.Impact,
			fields.Cause, fields.Description, fields.Resolution, wallTicks(time.Now())))
		return err
	})
	return updated, err
}

func scanRecord(row pgx.Row) (Record, error) {
	var record Record
	var occurredAt, createdAt int64
	var updatedAt *int64
	err := row.Scan(&record.ID, &record.Type, &record.Status, &occurredAt, &record.RoomName,
		&record.ReservationIDA, &record.ReservationIDB, &record.Impact, &record.Cause, &record.Description,
		&record.Resolution, &record.DetectionKey, &record.ReportedByUserID, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	record.OccurredAt = dotnettime.FromTicks(occurredAt).Format(dotnettime.Layout)
	record.CreatedAt = dotnettime.FromTicks(createdAt).Format(dotnettime.Layout)
	if updatedAt != nil {
		record.UpdatedAt = new(dotnettime.FromTicks(*updatedAt).Format(dotnettime.Layout))
	}
	return record, nil
}

func wallTicks(dateTime time.Time) int64 {
	return dotnettime.Ticks(dotnettime.OnDate(dateTime, dateTime))
}
