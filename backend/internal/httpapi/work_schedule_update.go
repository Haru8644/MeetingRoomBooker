package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

type workScheduleUpdater func(context.Context, int, workschedules.EntryInput, auth.User, string) (workschedules.Entry, error)

func newWorkScheduleUpdateHandler(update workScheduleUpdater, series bool) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		input, ok := decodeWorkScheduleInput(w, r)
		if !ok {
			return
		}
		var scope string
		if series {
			value, _, err := firstQueryValue(r.URL.RawQuery, "scope")
			if err != nil {
				writeNegotiatedString(w, r, http.StatusBadRequest, "反映範囲が不正です。")
				return
			}
			scope = value
		}
		entry, err := update(r.Context(), id, input, user, scope)
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
		case series:
			w.WriteHeader(http.StatusNoContent)
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
