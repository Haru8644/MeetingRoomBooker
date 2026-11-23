package chatwork

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

const creationType = "ReservationCreated"

// SendCreation sends DMs for reservations and in-app notices that have committed.
func (sender *Sender) SendCreation(ctx context.Context, result reservations.Creation) {
	sender.send(ctx, creationType, result.Entries, result.Overlaps)
}

func creationDetails(count int, overlaps []reservations.ListEntry, conflicts reservations.ParticipantConflicts) (title, action string, extra []string) {
	const maxDetails = 5
	action = "内容を確認してください"
	if count > 1 {
		action = "繰り返し予約がまとめて作成されました"
		extra = append(extra, fmt.Sprintf("作成件数: %d件", count))
	}
	sorted := slices.DeleteFunc(slices.Clone(overlaps), func(entry reservations.ListEntry) bool { return entry.ID <= 0 })
	slices.SortFunc(sorted, func(a, b reservations.ListEntry) int {
		return cmp.Or(a.Date.Compare(b.Date), a.StartTime.Compare(b.StartTime), cmp.Compare(a.ID, b.ID))
	})
	if len(sorted) > 0 {
		if len(extra) > 0 {
			extra = append(extra, "[hr]")
		}
		extra = append(extra, "注意: この予約は既存予約と重複しています", fmt.Sprintf("重複件数: %d件", len(sorted)), "重複している既存予約:")
		for _, overlap := range sorted[:min(len(sorted), maxDetails)] {
			extra = append(extra, fmt.Sprintf("- %s %s %s / %s", overlap.Date.Format("2006/01/02"),
				reservationTimeRange(overlap), overlap.Room, reservationLabel(overlap)))
		}
		if len(sorted) > maxDetails {
			extra = append(extra, fmt.Sprintf("- ほか %d件", len(sorted)-maxDetails))
		}
	}
	if conflicts.Count > 0 {
		if len(extra) > 0 {
			extra = append(extra, "[hr]")
		}
		extra = append(extra, "注意: 参加者の予定が社外予定と重複しています", fmt.Sprintf("重複件数: %d件", conflicts.Count))
		for _, line := range conflicts.Lines {
			extra = append(extra, "- "+line)
		}
		if conflicts.Count > len(conflicts.Lines) {
			extra = append(extra, fmt.Sprintf("- ほか %d件", conflicts.Count-len(conflicts.Lines)))
		}
	}
	return "会議予約を受け付けました", action, extra
}
