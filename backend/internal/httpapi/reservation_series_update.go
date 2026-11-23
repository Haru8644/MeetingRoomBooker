package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

type reservationSeriesUpdater func(context.Context, int, reservations.ListEntry, int, bool, string, bool) error

type reservationSeriesUpdateRequest struct {
	reservationSeriesRequest
	UpdatedReservation json.RawMessage
	NotifyParticipants *bool
}

func newReservationSeriesUpdateHandler(update reservationSeriesUpdater) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		if !hasJSONContentType(r) {
			writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 30_000_000))
		request, entry, problems, err := readReservationSeriesUpdate(decoder)
		if writeReservationSeriesInputProblem(w, err, problems, "A valid series update JSON object is required.") {
			return
		}
		writeReservationMutationResult(w, r, update(r.Context(), id, entry, user.ID, user.IsAdmin, *request.Scope, *request.NotifyParticipants))
	}
}

func readReservationSeriesUpdate(decoder *json.Decoder) (*reservationSeriesUpdateRequest, reservations.ListEntry, map[string][]string, error) {
	request := &reservationSeriesUpdateRequest{
		reservationSeriesRequest: reservationSeriesRequest{Scope: new("single")},
		NotifyParticipants:       new(true),
	}
	if err := readReservationSeriesJSON(decoder, request); err != nil {
		return nil, reservations.ListEntry{}, nil, err
	}
	if request.NotifyParticipants == nil {
		return nil, reservations.ListEntry{}, nil, errors.New("expected a boolean notifyParticipants")
	}
	problems := make(map[string][]string)
	now := time.Now()
	if err := request.validate(now, problems); err != nil {
		return nil, reservations.ListEntry{}, nil, err
	}
	input, err := readReservationSeriesSnapshot(request.UpdatedReservation, "UpdatedReservation", now, problems)
	if err != nil || len(problems) > 0 {
		return nil, reservations.ListEntry{}, problems, err
	}
	return request, input.entry(int(input.UserID), *input.Name), nil, nil
}
