package workschedules

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	externalAppointment = iota
	workFromHome
	leave
)

type CreateInput struct {
	Type, LeavePeriod int
	Title             string
	Date              time.Time
	Start, End        *time.Time
	RepeatType        string
	RepeatUntil       *time.Time
	ParticipantIDs    []int
	Participants      string
}

type ValidationError string

func (e ValidationError) Error() string { return string(e) }

type Creation struct {
	Entry          Entry
	ScheduledStart time.Time
	ChatworkBody   string
}

// Create commits the entry and its in-app notifications together. The caller
// can send the returned Chatwork message once the transaction has committed.
func Create(ctx context.Context, pool *pgxpool.Pool, input CreateInput, creatorID int) (Creation, error) {
	if err := input.validate(); err != nil {
		return Creation{}, err
	}
	var created Creation
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		names, err := input.normalize(ctx, tx, creatorID)
		if err != nil {
			return err
		}
		clock := time.Now()
		now := dotnettime.Ticks(dotnettime.OnDate(clock, clock))
		date := dotnettime.Ticks(input.Date)
		var start, end *int64
		scheduledStart := input.Date
		if input.Start != nil {
			start, end = new(dotnettime.Ticks(*input.Start)), new(dotnettime.Ticks(*input.End))
			scheduledStart = *input.Start
		}
		entry, err := scanEntry(tx.QueryRow(ctx, `
			INSERT INTO work_schedule_entries
			    (created_by_user_id, type, title, date_ticks, start_time_ticks, end_time_ticks,
			     leave_period, participant_ids, participants, created_at_ticks, updated_at_ticks)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
			RETURNING `+entryColumns, creatorID, input.Type, input.Title, date, start, end, input.LeavePeriod,
			input.ParticipantIDs, input.Participants, now))
		if err != nil {
			return err
		}
		notice, noticeType, dm, err := creationMessages(ctx, tx, entry.ID, input, names)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications
			    (user_id, message, created_at_ticks, target_date_ticks, type, target_work_schedule_entry_id)
			SELECT user_id, $2, $3, $4, $5, $6 FROM unnest($1::integer[]) AS recipient(user_id)`,
			entry.ParticipantIDs, notice, now, date, noticeType, entry.ID); err != nil {
			return fmt.Errorf("save work schedule notifications: %w", err)
		}
		created = Creation{Entry: entry, ScheduledStart: scheduledStart, ChatworkBody: dm}
		return nil
	})
	if err != nil {
		return Creation{}, err
	}
	return created, nil
}

func (input CreateInput) validate() error {
	noRepeat := strings.TrimSpace(input.RepeatType) == "" || input.RepeatType == "しない"
	switch {
	case input.Type != externalAppointment && (!noRepeat || input.RepeatUntil != nil):
		return ValidationError("繰り返し作成は社外予定のみ指定できます。")
	case !noRepeat:
		return ValidationError("繰り返し作成は社外予定の一括作成APIを使用してください。")
	case input.Type < externalAppointment || input.Type > leave:
		return ValidationError("予定種別が不正です。")
	case input.LeavePeriod < 0 || input.LeavePeriod > 3:
		return ValidationError("休暇区分が不正です。")
	case dotnettime.OnDate(input.Date, input.Date).IsZero():
		return ValidationError("日付を入力してください。")
	default:
		return nil
	}
}

func (input *CreateInput) normalize(ctx context.Context, tx pgx.Tx, creatorID int) (map[int]string, error) {
	ids := make([]int, 0, len(input.ParticipantIDs)+1)
	seen := make(map[int]bool)
	for _, id := range input.ParticipantIDs {
		if id > 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if creatorID > 0 && !seen[creatorID] {
		ids = slices.Insert(ids, 0, creatorID)
	}
	rows, err := tx.Query(ctx, `SELECT id, name FROM users WHERE id = ANY($1::integer[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("read work schedule participants: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct {
		ID   int
		Name string
	}])
	if err != nil {
		return nil, fmt.Errorf("read work schedule participant names: %w", err)
	}
	byID := make(map[int]string, len(names))
	for _, user := range names {
		byID[user.ID] = user.Name
	}
	var display, missing []string
	for _, id := range ids {
		if name, found := byID[id]; found {
			display = append(display, name)
		} else {
			missing = append(missing, strconv.Itoa(id))
		}
	}
	if len(missing) > 0 {
		return nil, ValidationError("存在しない参加者が含まれています: " + strings.Join(missing, ", "))
	}
	input.ParticipantIDs = ids
	input.Participants = strings.TrimSpace(input.Participants)
	if input.Participants == "" {
		input.Participants = strings.Join(display, "、")
	}
	if len(utf16.Encode([]rune(input.Participants))) > 500 {
		return nil, ValidationError("参加者は500文字以内で入力してください。")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Date = dotnettime.OnDate(input.Date, time.Time{})

	switch input.Type {
	case externalAppointment:
		if input.Title == "" {
			return nil, ValidationError("社外予定の内容を入力してください。")
		}
		if input.Start == nil || input.End == nil {
			return nil, ValidationError("社外予定の開始時刻と終了時刻を入力してください。")
		}
		start := dotnettime.OnDate(input.Date, *input.Start).Truncate(100 * time.Nanosecond)
		end := dotnettime.OnDate(input.Date, *input.End).Truncate(100 * time.Nanosecond)
		if !start.Before(end) {
			return nil, ValidationError("終了時刻は開始時刻より後にしてください。")
		}
		input.Start, input.End, input.LeavePeriod = &start, &end, 0
	case workFromHome:
		if input.Title == "" {
			input.Title = "在宅"
		}
		input.Start, input.End, input.LeavePeriod = nil, nil, 0
	case leave:
		if input.LeavePeriod == 0 {
			return nil, ValidationError("休暇区分を選択してください。")
		}
		if input.Title == "" {
			input.Title = "休暇"
		}
		input.Start, input.End = nil, nil
	}
	return byID, nil
}
