package workschedules

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeleteSeries deletes the selected scope and saves its summary notices before commit.
func DeleteSeries(ctx context.Context, pool *pgxpool.Pool, id, actorID int, isAdmin bool, scope string) (Deletion, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "", "single":
		return Delete(ctx, pool, id, actorID, isAdmin)
	case "following", "all":
	default:
		return Deletion{}, ValidationError("削除範囲が不正です。")
	}

	var result Deletion
	err := mutateWorkScheduleSeries(ctx, pool, id, actorID, isAdmin, scope, func(tx pgx.Tx, entries []storedEntry) error {
		ids := make([]int, len(entries))
		for i, entry := range entries {
			ids[i] = entry.ID
		}
		if err := deleteLockedWorkSchedules(ctx, tx, ids); err != nil {
			return err
		}

		slices.SortFunc(entries, compareScheduleOccurrences)
		representative := entries[0]
		recipients := workScheduleParticipants(entries)
		result = Deletion{
			ID: representative.ID, SeriesID: workScheduleSeriesKey(representative), Scope: scope,
			ParticipantIDs: recipients, ScheduledStart: representative.scheduledStart(),
		}
		if len(recipients) == 0 {
			return nil
		}

		summary := []string{
			"削除範囲: " + workScheduleSeriesScopeLabel(scope),
			"期間: " + representative.calendarDate() + "〜" + entries[len(entries)-1].calendarDate(),
			fmt.Sprintf("件数: %d件", len(entries)),
		}
		participants := "対象者: " + participantText(representative.Participants)
		label := scheduleTypeLabel(representative.Type)
		notice := label + "「" + representative.Title + "」をまとめて削除しました。" +
			strings.Join(summary, " / ") + " / " + participants
		clock := time.Now()
		now := dotnettime.Ticks(dotnettime.OnDate(clock, clock))
		if err := saveWorkScheduleNotices(ctx, tx, representative, recipients, "Info", notice, now); err != nil {
			return err
		}

		body := []string{"[info][title]" + label + "をまとめて削除しました[/title]", "内容: " + representative.Title}
		body = append(body, summary...)
		body = append(body, "時間: "+representative.timeRange(), participants, "[/info]")
		result.ChatworkBody = strings.Join(body, "\n")
		return nil
	})
	switch {
	case errors.Is(err, errWorkScheduleNotInSeries):
		return Deletion{}, ValidationError("繰り返し社外予定のみまとめて削除できます。")
	case errors.Is(err, errWorkScheduleNoTargets):
		return Deletion{}, ValidationError("削除対象の社外予定がありません。")
	case err != nil:
		return Deletion{}, err
	default:
		return result, nil
	}
}
