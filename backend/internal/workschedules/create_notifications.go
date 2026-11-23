package workschedules

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func creationMessages(ctx context.Context, tx pgx.Tx, entries []Entry, dates []time.Time, input CreateInput,
	names map[int]string,
) (notice, kind, dm string, err error) {
	display := input.Participants
	if strings.TrimSpace(display) == "" {
		display = "未設定"
	}
	var conflicts []scheduleConflict
	var hours string
	if input.Type == externalAppointment {
		hours = "時間: " + input.Start.Format("15:04") + "〜" + input.End.Format("15:04")
		conflicts, err = findCreationConflicts(ctx, tx, entries, dates, input, names)
		if err != nil {
			return "", "", "", err
		}
	}

	firstDate := dates[0].Format("2006/01/02")
	participants := "対象者: " + display
	var body []string
	if entries[0].SeriesID != nil {
		period := "期間: " + firstDate + "〜" + dates[len(dates)-1].Format("2006/01/02")
		repeat := "繰り返し: " + input.RepeatType
		count := fmt.Sprintf("件数: %d件", len(entries))
		notice = "社外予定「" + input.Title + "」を繰り返し登録しました。" +
			strings.Join([]string{period, repeat, count, participants}, " / ")
		body = []string{"[info][title]社外予定を繰り返し登録しました[/title]", "内容: " + input.Title,
			period, repeat, count, hours, participants}
	} else {
		label := "社外予定"
		summary := []string{"日付: " + firstDate}
		switch input.Type {
		case externalAppointment:
			summary = append(summary, hours)
		case workFromHome:
			label = "在宅予定"
		case leave:
			label = "休暇予定"
			summary = append(summary, "休暇区分: "+[]string{"", "午前休", "午後休", "休み"}[input.LeavePeriod])
		}
		summary = append(summary, participants)
		notice = label + "「" + input.Title + "」が登録されました。" + strings.Join(summary, " / ")
		body = append([]string{"[info][title]" + label + "が登録されました[/title]", "内容: " + input.Title}, summary...)
	}
	kind = "Info"
	if len(conflicts) > 0 {
		kind = "Warning"
		notice += " 注意: 参加者の予定重複があります。"
		body = append(body, "[hr]", "注意: 参加者の予定重複があります")
		var lines []string
		const maxDetails = 5
		for _, conflict := range conflicts[:min(len(conflicts), maxDetails)] {
			line := fmt.Sprintf("%s: %s「%s」 %s %s〜%s", names[conflict.UserID], conflict.Source, conflict.Title,
				dotnettime.FromTicks(conflict.Date).Format("2006/01/02"), dotnettime.FromTicks(conflict.Start).Format("15:04"),
				dotnettime.FromTicks(conflict.End).Format("15:04"))
			lines = append(lines, line)
			body = append(body, "- "+line)
		}
		notice += strings.Join(lines, " ")
		if len(conflicts) > maxDetails {
			notice += fmt.Sprintf(" ほか%d件", len(conflicts)-maxDetails)
			body = append(body, fmt.Sprintf("- ほか %d件", len(conflicts)-maxDetails))
		}
	}
	return notice, kind, strings.Join(append(body, "[/info]"), "\n"), nil
}

type scheduleConflict struct {
	UserID, ID       int
	Source, Title    string
	DisplayIfNoTitle *string
	Date, Start, End int64
}

func findCreationConflicts(ctx context.Context, tx pgx.Tx, entries []Entry, dates []time.Time, input CreateInput,
	names map[int]string,
) ([]scheduleConflict, error) {
	dateTicks := make([]int64, len(dates))
	entryIDs := make([]int, len(entries))
	for i, date := range dates {
		dateTicks[i], entryIDs[i] = dotnettime.Ticks(date), entries[i].ID
	}
	startOffset := dotnettime.Ticks(*input.Start) - dateTicks[0]
	endOffset := dotnettime.Ticks(*input.End) - dateTicks[0]
	rows, err := tx.Query(ctx, `
		WITH occurrences AS (SELECT * FROM unnest($2::bigint[], $3::integer[]) AS o(day_ticks, entry_id))
		SELECT p.id, r.id, '会議室予約', r.purpose, r.name, o.day_ticks, r.start_time_ticks, r.end_time_ticks
		FROM occurrences o
		JOIN reservations r ON r.date_ticks = o.day_ticks
		  AND r.start_time_ticks < o.day_ticks + $4 AND r.end_time_ticks > o.day_ticks + $5
		JOIN unnest($1::integer[]) AS p(id) ON p.id = r.user_id OR p.id = ANY(r.participant_ids)
		UNION ALL
		SELECT p.id, e.id, '社外予定', e.title, NULL::text, o.day_ticks, e.start_time_ticks, e.end_time_ticks
		FROM occurrences o
		JOIN work_schedule_entries e ON e.date_ticks = o.day_ticks AND e.type = 0 AND e.id <> o.entry_id
		  AND e.start_time_ticks < o.day_ticks + $4 AND e.end_time_ticks > o.day_ticks + $5
		JOIN unnest($1::integer[]) AS p(id) ON p.id = ANY(e.participant_ids)`,
		input.ParticipantIDs, dateTicks, entryIDs, endOffset, startOffset)
	if err != nil {
		return nil, fmt.Errorf("query work schedule conflicts: %w", err)
	}
	conflicts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[scheduleConflict])
	if err != nil {
		return nil, fmt.Errorf("read work schedule conflicts: %w", err)
	}
	for i := range conflicts {
		c := &conflicts[i]
		if c.DisplayIfNoTitle != nil && strings.TrimSpace(c.Title) == "" {
			c.Title = *c.DisplayIfNoTitle
		}
	}
	order := collate.New(language.Und)
	slices.SortStableFunc(conflicts, func(a, b scheduleConflict) int {
		return cmp.Or(cmp.Compare(a.Date, b.Date), cmp.Compare(a.Start, b.Start), order.CompareString(names[a.UserID], names[b.UserID]),
			order.CompareString(a.Source, b.Source), cmp.Compare(a.ID, b.ID))
	})
	return conflicts, nil
}
