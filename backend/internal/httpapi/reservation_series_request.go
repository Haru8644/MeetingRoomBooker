package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

type reservationSeriesRequest struct {
	OriginalReservation json.RawMessage
	Scope               *string
}

func readReservationSeriesJSON[T any](decoder *json.Decoder, request *T) error {
	if err := decoder.Decode(&request); err != nil {
		return err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		if err == nil {
			err = errors.New("expected one JSON object")
		}
		return err
	}
	if request == nil {
		return io.EOF
	}
	return nil
}

func (request reservationSeriesRequest) validate(now time.Time, problems map[string][]string) error {
	if request.Scope == nil {
		problems["Scope"] = []string{"The Scope field is required."}
	}
	// Model validation applies to the original snapshot. The stored route ID
	// determines authorization and series membership.
	_, err := readReservationSeriesSnapshot(request.OriginalReservation, "OriginalReservation", now, problems)
	return err
}

func readReservationSeriesSnapshot(data json.RawMessage, field string, now time.Time, problems map[string][]string) (reservationInput, error) {
	if bytes.Equal(data, []byte("null")) {
		problems[field] = []string{"The " + field + " field is required."}
		return reservationInput{}, nil
	}
	if data == nil {
		data = json.RawMessage(`{}`)
	}
	input, err := readReservationInput(json.NewDecoder(bytes.NewReader(data)), now, true)
	if err != nil {
		return reservationInput{}, err
	}
	for name, messages := range input.validate() {
		problems[field+"."+name] = messages
	}
	return input, nil
}

func writeReservationSeriesInputProblem(w http.ResponseWriter, err error, problems map[string][]string, invalidJSON string) bool {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	case err == io.EOF:
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{
			"": {"A non-empty request body is required."}, "request": {"The request field is required."},
		})
	case err != nil:
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"request": {invalidJSON}})
	case len(problems) > 0:
		writeValidationProblem(w, http.StatusBadRequest, problems)
	default:
		return false
	}
	return true
}
