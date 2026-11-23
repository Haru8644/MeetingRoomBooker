package chatwork

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

const updateType = "ReservationUpdated"

// SendUpdate sends DMs for a committed update. Reusing the result also reuses
// its event ID and therefore the same per-recipient delivery claims.
func (sender *Sender) SendUpdate(ctx context.Context, result reservations.UpdateResult) {
	if result.Count == 0 || result.ChangeID == "" {
		return
	}
	ctx, cancel := deliveryContext(ctx)
	defer cancel()

	previous, current := result.Previous, result.Current
	before := reservationParticipants([]reservations.ListEntry{previous})
	after := reservationParticipants([]reservations.ListEntry{current})
	// Match C# Union order, including the previous organizer before newly added participants.
	var stakeholders []int
	seen := make(map[int]bool)
	for _, id := range slices.Concat(before, []int{previous.UserID}, after, []int{current.UserID}) {
		if id > 0 && !seen[id] {
			stakeholders = append(stakeholders, id)
			seen[id] = true
		}
	}
	recipients, err := sender.loadRecipients(ctx, stakeholders)
	if err != nil {
		log.Printf("Chatwork %s: could not load recipients for reservation %d", updateType, current.ID)
		return
	}
	changes := reservations.UpdateChangeLines(previous, current, "目的",
		participantDisplayNames(participantDifference(after, before), recipients),
		participantDisplayNames(participantDifference(before, after), recipients))

	current.ParticipantIDs = after
	conflicts, err := reservations.FindParticipantConflicts(ctx, sender.pool, current)
	if err != nil {
		log.Printf("Chatwork %s: could not load appointment conflicts for reservation %d", updateType, current.ID)
		return
	}
	if len(changes) == 0 && conflicts.Count == 0 {
		return
	}

	var extra []string
	if len(changes) > 0 {
		extra = append(extra, "変更点:")
		for _, line := range changes {
			extra = append(extra, "- "+line)
		}
	}
	action := "変更内容を確認してください"
	if result.Count > 1 {
		action = "繰り返し予約がまとめて変更されました"
		if len(extra) > 0 {
			extra = append(extra, "[hr]")
		}
		extra = append(extra, fmt.Sprintf("更新件数: %d件", result.Count))
	}
	extra = appendParticipantConflictDetails(extra, conflicts)
	message := stakeholderMessage(current, after, stakeholders, recipients, "会議予約が変更されました", action, extra)
	sender.sendToStakeholders(ctx, updateType, ":change:"+result.ChangeID, current, stakeholders, recipients, message)
}

func participantDifference(ids, other []int) []int {
	known := make(map[int]bool, len(other))
	for _, id := range other {
		known[id] = true
	}
	return slices.DeleteFunc(slices.Clone(ids), func(id int) bool { return known[id] })
}
