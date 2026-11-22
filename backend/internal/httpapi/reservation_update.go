package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

type reservationUpdater func(context.Context, reservations.ListEntry, int, bool, bool) error

func newReservationUpdateHandler(update reservationUpdater) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		input, ok := decodeReservationInput(w, r, true)
		if !ok {
			return
		}
		notify, message := readReservationBoolean(r.URL.RawQuery, "notifyParticipants", true)
		problems := input.validate()
		if message != "" {
			problems["notifyParticipants"] = []string{message}
		}
		if len(problems) != 0 {
			writeValidationProblem(w, http.StatusBadRequest, problems)
			return
		}
		if int(input.ID) != id {
			writeProblemDetails(w, problemDetails{
				Type: "https://tools.ietf.org/html/rfc9110#section-15.5.1", Title: "Bad Request", Status: http.StatusBadRequest,
			})
			return
		}
		writeReservationMutationResult(w, r, update(r.Context(), input.entry(int(input.UserID), *input.Name), user.ID, user.IsAdmin, notify))
	}
}

func writeReservationMutationResult(w http.ResponseWriter, r *http.Request, err error) {
	var invalid reservations.ValidationError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, reservations.ErrNotFound):
		writeProblemDetails(w, problemDetails{
			Type: "https://tools.ietf.org/html/rfc9110#section-15.5.5", Title: "Not Found", Status: http.StatusNotFound,
		})
	case errors.Is(err, reservations.ErrForbidden):
		w.WriteHeader(http.StatusForbidden)
	case errors.As(err, &invalid):
		writeNegotiatedString(w, r, http.StatusBadRequest, invalid.Error())
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}
