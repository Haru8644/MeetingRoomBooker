package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

type reservationSeriesCreator func(context.Context, []reservations.ListEntry, bool) ([]reservations.ListEntry, error)

func newReservationSeriesCreateHandler(create reservationSeriesCreator) func(http.ResponseWriter, *http.Request, auth.User) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User) {
		if !hasJSONContentType(r) {
			writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 30_000_000))
		inputs, err := readReservationSeriesInput(decoder)
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		case err == io.EOF:
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"": {"A non-empty request body is required."}})
			return
		case err != nil:
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"reservations": {"A valid reservation JSON array is required."}})
			return
		}
		allowOverlap, message := readReservationBoolean(r.URL.RawQuery, "allowOverlap", false)
		problems := make(map[string][]string)
		if message != "" {
			problems["allowOverlap"] = []string{message}
		}
		for i, input := range inputs {
			for field, errors := range input.validate() {
				problems[fmt.Sprintf("[%d].%s", i, field)] = errors
			}
		}
		if len(problems) > 0 {
			writeValidationProblem(w, http.StatusBadRequest, problems)
			return
		}
		if len(inputs) == 0 {
			writeNegotiatedString(w, r, http.StatusBadRequest, "登録する予約がありません。")
			return
		}

		// C# orders the supplied Date and StartTime before normalizing the times of day.
		slices.SortStableFunc(inputs, func(left, right reservationInput) int {
			if order := left.Date.Compare(right.Date.Time); order != 0 {
				return order
			}
			return left.StartTime.Compare(right.StartTime.Time)
		})
		entries := make([]reservations.ListEntry, len(inputs))
		for i, input := range inputs {
			entries[i] = input.entry(user.ID, user.Name)
		}
		created, err := create(r.Context(), entries, allowOverlap)
		var conflict *reservations.ConflictError
		var invalid reservations.ValidationError
		switch {
		case errors.As(err, &conflict):
			writeNegotiatedString(w, r, http.StatusConflict, conflict.Error())
		case errors.As(err, &invalid):
			writeNegotiatedString(w, r, http.StatusBadRequest, invalid.Error())
		case err != nil:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.Header().Set("Location", reservationListLocation(r, ""))
			response := make([]reservationResponse, len(created))
			for i, entry := range created {
				response[i] = newReservationResponse(entry)
			}
			writeJSON(w, http.StatusCreated, response)
		}
	}
}

func readReservationSeriesInput(decoder *json.Decoder) ([]reservationInput, error) {
	var elements []json.RawMessage
	if err := decoder.Decode(&elements); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		if err == nil {
			err = errors.New("expected one JSON array")
		}
		return nil, err
	}
	inputs := make([]reservationInput, len(elements))
	now := time.Now()
	for i, element := range elements {
		input, err := readReservationInput(json.NewDecoder(bytes.NewReader(element)), now, false)
		if err != nil {
			return nil, fmt.Errorf("read reservation at index %d: %w", i, err)
		}
		inputs[i] = input
	}
	return inputs, nil
}
