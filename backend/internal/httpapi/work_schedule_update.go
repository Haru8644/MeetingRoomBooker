package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

type workScheduleUpdater func(context.Context, int, workschedules.EntryInput, auth.User) (workschedules.Entry, error)

func newWorkScheduleUpdateHandler(update workScheduleUpdater) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		input, ok := decodeWorkScheduleInput(w, r)
		if !ok {
			return
		}
		entry, err := update(r.Context(), id, input, user)
		var invalid workschedules.ValidationError
		switch {
		case errors.As(err, &invalid):
			writeNegotiatedString(w, r, http.StatusBadRequest, invalid.Error())
		case errors.Is(err, workschedules.ErrNotFound):
			writeWorkScheduleNotFound(w)
		case errors.Is(err, workschedules.ErrForbidden):
			w.WriteHeader(http.StatusForbidden)
		case err != nil:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusOK, entry)
		}
	}
}

func writeWorkScheduleNotFound(w http.ResponseWriter) {
	writeProblemDetails(w, problemDetails{
		Type: "https://tools.ietf.org/html/rfc9110#section-15.5.5", Title: "Not Found", Status: http.StatusNotFound,
	})
}
