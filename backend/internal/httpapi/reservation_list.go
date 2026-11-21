package httpapi

import (
	"context"
	"net/http"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

const ReservationListPattern = "GET /api/Reservations"

type authenticatedUserKey struct{}

// WithAuthenticatedUser records an identity validated by authentication middleware.
func WithAuthenticatedUser(ctx context.Context, userID int) context.Context {
	return context.WithValue(ctx, authenticatedUserKey{}, userID)
}

func NewReservationListHandler(list func(context.Context) ([]reservations.ListEntry, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, authenticated := r.Context().Value(authenticatedUserKey{}).(int); !authenticated {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		entries, err := list(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response := make([]reservationResponse, len(entries))
		for i, entry := range entries {
			response[i] = newReservationResponse(entry)
		}
		writeJSON(w, http.StatusOK, response)
	})
}

type reservationResponse struct {
	ID               int     `json:"id"`
	UserID           int     `json:"userId"`
	Name             string  `json:"name"`
	UserName         string  `json:"userName"`
	Room             string  `json:"room"`
	RoomName         string  `json:"roomName"`
	NumberOfPeople   int     `json:"numberOfPeople"`
	ParticipantCount int     `json:"participantCount"`
	Type             string  `json:"type"`
	IsInternal       bool    `json:"isInternal"`
	Purpose          string  `json:"purpose"`
	SeriesID         *string `json:"seriesId"`
	Date             string  `json:"date"`
	StartTime        string  `json:"startTime"`
	EndTime          string  `json:"endTime"`
	ParticipantIDs   []int   `json:"participantIds"`
	Participants     string  `json:"participants"`
	RepeatType       string  `json:"repeatType"`
	RepeatUntil      *string `json:"repeatUntil"`
}

func newReservationResponse(entry reservations.ListEntry) reservationResponse {
	var repeatUntil *string
	if entry.RepeatUntil != nil {
		value := entry.RepeatUntil.Format(dotnettime.Layout)
		repeatUntil = &value
	}
	participantIDs := entry.ParticipantIDs
	if participantIDs == nil {
		participantIDs = []int{}
	}
	return reservationResponse{
		ID:               entry.ID,
		UserID:           entry.UserID,
		Name:             entry.Name,
		UserName:         entry.Name,
		Room:             entry.Room,
		RoomName:         entry.Room,
		NumberOfPeople:   entry.NumberOfPeople,
		ParticipantCount: entry.NumberOfPeople,
		Type:             entry.Type,
		IsInternal:       entry.Type == "社内",
		Purpose:          entry.Purpose,
		SeriesID:         entry.SeriesID,
		Date:             entry.Date.Format(dotnettime.Layout),
		StartTime:        entry.StartTime.Format(dotnettime.Layout),
		EndTime:          entry.EndTime.Format(dotnettime.Layout),
		ParticipantIDs:   participantIDs,
		Participants:     entry.Participants,
		RepeatType:       entry.RepeatType,
		RepeatUntil:      repeatUntil,
	}
}
