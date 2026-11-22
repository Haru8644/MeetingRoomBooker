package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

type reservationSeriesUpdater func(context.Context, int, reservations.ListEntry, int, bool, string, bool) error

type reservationSeriesUpdateRequest struct {
	OriginalReservation json.RawMessage
	UpdatedReservation  json.RawMessage
	Scope               *string
	NotifyParticipants  *bool
}

func newReservationSeriesUpdateHandler(update reservationSeriesUpdater) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		if !hasJSONContentType(r) {
			writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 30_000_000))
		request, entry, problems, err := readReservationSeriesUpdate(decoder)
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		case err == io.EOF:
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{
				"": {"A non-empty request body is required."}, "request": {"The request field is required."},
			})
		case err != nil:
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"request": {"A valid series update JSON object is required."}})
		case len(problems) > 0:
			writeValidationProblem(w, http.StatusBadRequest, problems)
		default:
			writeReservationMutationResult(w, r, update(r.Context(), id, entry, user.ID, user.IsAdmin, *request.Scope, *request.NotifyParticipants))
		}
	}
}

func readReservationSeriesUpdate(decoder *json.Decoder) (*reservationSeriesUpdateRequest, reservations.ListEntry, map[string][]string, error) {
	request := &reservationSeriesUpdateRequest{Scope: new("single"), NotifyParticipants: new(true)}
	if err := decoder.Decode(&request); err != nil {
		return nil, reservations.ListEntry{}, nil, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		if err == nil {
			err = errors.New("expected one JSON object")
		}
		return nil, reservations.ListEntry{}, nil, err
	}
	if request == nil {
		return nil, reservations.ListEntry{}, nil, io.EOF
	}
	if request.NotifyParticipants == nil {
		return nil, reservations.ListEntry{}, nil, errors.New("expected a boolean notifyParticipants")
	}
	problems := make(map[string][]string)
	if request.Scope == nil {
		problems["Scope"] = []string{"The Scope field is required."}
	}

	now := time.Now()
	// The original snapshot is model-validated by C#. Target selection uses the
	// stored route ID, so only the updated reservation becomes a write input.
	for _, nested := range []struct {
		name string
		data json.RawMessage
	}{
		{"OriginalReservation", request.OriginalReservation},
		{"UpdatedReservation", request.UpdatedReservation},
	} {
		if bytes.Equal(nested.data, []byte("null")) {
			problems[nested.name] = []string{"The " + nested.name + " field is required."}
			continue
		}
		if nested.data == nil {
			nested.data = json.RawMessage(`{}`)
		}
		input, err := readReservationInput(json.NewDecoder(bytes.NewReader(nested.data)), now, true)
		if err != nil {
			return nil, reservations.ListEntry{}, nil, err
		}
		for field, messages := range input.validate() {
			problems[nested.name+"."+field] = messages
		}
		if nested.name == "UpdatedReservation" && len(problems) == 0 {
			return request, input.entry(int(input.UserID), *input.Name), nil, nil
		}
	}
	return request, reservations.ListEntry{}, problems, nil
}
