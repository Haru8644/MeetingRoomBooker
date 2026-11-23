package workschedules

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func creationMessages(ctx context.Context, tx pgx.Tx, id int, entry CreateInput, names map[int]string) (notice, kind, dm string, err error) {
	label := "社外予定"
	date := entry.Date.Format("2006/01/02")
	display := entry.Participants
	if strings.TrimSpace(display) == "" {
		display = "未設定"
	}
	summary := []string{"日付: " + date}
	var conflicts []scheduleConflict
	switch entry.Type {
	case externalAppointment:
		summary = append(summary, "時間: "+entry.Start.Format("15:04")+"〜"+entry.End.Format("15:04"))
		conflicts, err = findCreationConflicts(ctx, tx, id, entry, names)
		if err != nil {
			return "", "", "", err
		}
	case workFromHome:
		label = "在宅予定"
	case leave:
		label = "休暇予定"
		summary = append(summary, "休暇区分: "+[]string{"", "午前休", "午後休", "休み"}[entry.LeavePeriod])
	}
	summary = append(summary, "対象者: "+display)
	notice = label + "「" + entry.Title + "」が登録されました。" + strings.Join(summary, " / ")
	body := append([]string{"[info][title]" + label + "が登録されました[/title]", "内容: " + entry.Title}, summary...)
	kind = "Info"
	if len(conflicts) > 0 {
		kind = "Warning"
		notice += " 注意: 参加者の予定重複があります。"
		body = append(body, "[hr]", "注意: 参加者の予定重複があります")
		var lines []string
		const maxDetails = 5
		for _, conflict := range conflicts[:min(len(conflicts), maxDetails)] {
			line := fmt.Sprintf("%s: %s「%s」 %s %s〜%s", names[conflict.UserID], conflict.Source, conflict.Title,
				date, dotnettime.FromTicks(conflict.Start).Format("15:04"),
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
	Start, End       int64
}

func findCreationConflicts(ctx context.Context, tx pgx.Tx, id int, entry CreateInput, names map[int]string) ([]scheduleConflict, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, r.id, '会議室予約', r.purpose, r.name, r.start_time_ticks, r.end_time_ticks
		FROM reservations r
		JOIN unnest($1::integer[]) AS p(id) ON p.id = r.user_id OR p.id = ANY(r.participant_ids)
		WHERE r.date_ticks = $2 AND r.start_time_ticks < $3 AND r.end_time_ticks > $4
		UNION ALL
		SELECT p.id, e.id, '社外予定', e.title, NULL::text, e.start_time_ticks, e.end_time_ticks
		FROM work_schedule_entries e
		JOIN unnest($1::integer[]) AS p(id) ON p.id = ANY(e.participant_ids)
		WHERE e.type = 0 AND e.date_ticks = $2 AND e.start_time_ticks < $3 AND e.end_time_ticks > $4
		  AND e.id <> $5`, entry.ParticipantIDs, dotnettime.Ticks(entry.Date),
		dotnettime.Ticks(*entry.End), dotnettime.Ticks(*entry.Start), id)
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
		return cmp.Or(cmp.Compare(a.Start, b.Start), order.CompareString(names[a.UserID], names[b.UserID]),
			order.CompareString(a.Source, b.Source), cmp.Compare(a.ID, b.ID))
	})
	return conflicts, nil
}
