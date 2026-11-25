package workschedules

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

func notifyWorkScheduleSeriesUpdate(ctx context.Context, tx pgx.Tx, before, after []storedEntry, scope string,
	names map[int]string, now int64,
) (UpdateResult, error) {
	slices.SortFunc(before, compareScheduleOccurrences)
	slices.SortFunc(after, compareScheduleOccurrences)
	first := after[0]
	result := UpdateResult{Entry: first.Entry, Scope: scope}
	recipients := seriesUpdateParticipants(before, after)
	if len(recipients) == 0 {
		return result, nil
	}
	ids := make([]int, len(after))
	for i, entry := range after {
		ids[i] = entry.ID
	}
	conflicts, err := findScheduleConflicts(ctx, tx, ids, names)
	if err != nil {
		return UpdateResult{}, err
	}
	kind, noticeWarning, dmWarning := conflictWarnings(conflicts, names)
	scopeLabel := "全て"
	if scope == "following" {
		scopeLabel = "今回以降"
	}
	summary := []string{
		"反映範囲: " + scopeLabel,
		"期間: " + first.calendarDate() + "〜" + after[len(after)-1].calendarDate(),
		fmt.Sprintf("件数: %d件", len(after)),
		"時間: " + first.timeRange(),
		"対象者: " + participantText(first.Participants),
	}
	label := scheduleTypeLabel(first.Type)
	notice := label + "「" + first.Title + "」をまとめて更新しました。" + strings.Join(summary, " / ")
	body := append([]string{"[info][title]" + label + "をまとめて変更しました[/title]", "内容: " + first.Title}, summary...)
	changes := seriesUpdateChangeLines(before[0], first)
	if len(changes) > 0 {
		notice += " 変更点: " + strings.Join(changes, "。 ") + "。"
		body = append(body, "[hr]", "変更点:")
		for _, line := range changes {
			body = append(body, "- "+line)
		}
	}
	if err := saveWorkScheduleUpdateNotices(ctx, tx, first, recipients, kind, notice+noticeWarning, now); err != nil {
		return UpdateResult{}, err
	}
	body = append(body, dmWarning...)
	result.Deliveries = []UpdateDelivery{{
		ParticipantIDs: recipients, ScheduledStart: first.scheduledStart(),
		Message: strings.Join(append(body, "[/info]"), "\n"),
	}}
	return result, nil
}

func compareScheduleOccurrences(a, b storedEntry) int {
	start := func(entry storedEntry) int64 {
		if entry.startTicks == nil {
			return entry.dateTicks
		}
		return *entry.startTicks
	}
	return cmp.Or(cmp.Compare(a.dateTicks, b.dateTicks), cmp.Compare(start(a), start(b)), cmp.Compare(a.ID, b.ID))
}

func seriesUpdateParticipants(before, after []storedEntry) []int {
	var recipients []int
	seen := make(map[int]bool)
	for _, entries := range [][]storedEntry{before, after} {
		for _, entry := range entries {
			for _, id := range entry.ParticipantIDs {
				if id > 0 && !seen[id] {
					recipients = append(recipients, id)
					seen[id] = true
				}
			}
		}
	}
	return recipients
}

func seriesUpdateChangeLines(before, after storedEntry) []string {
	var changes []string
	if before.Title != after.Title {
		changes = append(changes, fmt.Sprintf("内容が「%s」から「%s」に変更されました", before.Title, after.Title))
	}
	if !sameScheduleTime(before.startTicks, after.startTicks) || !sameScheduleTime(before.endTicks, after.endTicks) {
		changes = append(changes, fmt.Sprintf("時間が %s から %s に変更されました", before.timeRange(), after.timeRange()))
	}
	if !slices.Equal(before.ParticipantIDs, after.ParticipantIDs) {
		changes = append(changes, "対象者が変更されました")
	}
	return changes
}
