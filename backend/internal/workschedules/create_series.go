package workschedules

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	noRepeatType   = "しない"
	daily          = "毎日"
	weekly         = "毎週"
	maxOccurrences = 60
)

// CreateSeries commits the expanded entries and their summary notifications together.
// Requests without recurrence go through ordinary single-entry creation.
func CreateSeries(ctx context.Context, pool *pgxpool.Pool, input CreateInput, creatorID int) (Creation, error) {
	repeat := strings.TrimSpace(input.RepeatType)
	if repeat == "" || repeat == noRepeatType {
		return Create(ctx, pool, input, creatorID)
	}
	dates, err := occurrenceDates(input, repeat)
	if err != nil {
		return Creation{}, err
	}

	var uuid [16]byte
	rand.Read(uuid[:])
	uuid[6] = uuid[6]&0x0f | 0x40
	uuid[8] = uuid[8]&0x3f | 0x80
	seriesID := hex.EncodeToString(uuid[:])
	input.RepeatType, input.Date = repeat, dates[0]
	return createEntries(ctx, pool, input, creatorID, dates, &seriesID)
}

func occurrenceDates(input CreateInput, repeat string) ([]time.Time, error) {
	switch {
	case input.Type != externalAppointment:
		return nil, ValidationError("繰り返し作成は社外予定のみ指定できます。")
	case repeat != daily && repeat != weekly:
		return nil, ValidationError("繰り返しの種類が不正です。")
	case input.RepeatUntil == nil:
		return nil, ValidationError("繰り返し終了日を入力してください。")
	}
	from := dotnettime.OnDate(input.Date, time.Time{})
	until := dotnettime.OnDate(*input.RepeatUntil, time.Time{})
	if until.Before(from) {
		return nil, ValidationError("繰り返し終了日は開始日以降にしてください。")
	}
	empty := ValidationError("作成対象の日付がありません。")
	step := 1
	if repeat == weekly {
		step = 7
		if isWeekend(from) {
			return nil, empty
		}
	}

	var dates []time.Time
	for date := from; !date.After(until); date = date.AddDate(0, 0, step) {
		if isWeekend(date) {
			continue
		}
		if len(dates) == maxOccurrences {
			return nil, ValidationError("繰り返し作成は最大60件までです。")
		}
		dates = append(dates, date)
	}
	if len(dates) == 0 {
		return nil, empty
	}
	return dates, nil
}

func isWeekend(date time.Time) bool {
	return date.Weekday() == time.Saturday || date.Weekday() == time.Sunday
}
