package reservations

import (
	"strings"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
)

const NoRepeat = "しない"

func (entry *ListEntry) Normalize() {
	participants := make([]int, 0, len(entry.ParticipantIDs)+1)
	seen := make(map[int]bool, len(entry.ParticipantIDs))
	for _, id := range entry.ParticipantIDs {
		if id > 0 && !seen[id] {
			participants = append(participants, id)
			seen[id] = true
		}
	}
	if entry.UserID > 0 && !seen[entry.UserID] {
		participants = append([]int{entry.UserID}, participants...)
	}
	entry.ParticipantIDs = participants
	entry.StartTime = dotnettime.OnDate(entry.Date, entry.StartTime)
	entry.EndTime = dotnettime.OnDate(entry.Date, entry.EndTime)
	if strings.TrimSpace(entry.RepeatType) == "" {
		entry.RepeatType = NoRepeat
	}
	if entry.SeriesID != nil {
		seriesID := strings.TrimSpace(*entry.SeriesID)
		if seriesID == "" {
			entry.SeriesID = nil
		} else {
			entry.SeriesID = &seriesID
		}
	}
	if entry.RepeatType == NoRepeat {
		entry.RepeatUntil = nil
		entry.SeriesID = nil
	}
}
