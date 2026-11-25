package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

type workScheduleDeleter func(context.Context, int, auth.User, string) error

func newWorkScheduleDeleteHandler(deleteEntry workScheduleDeleter, series bool) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		var scope string
		if series {
			value, _, err := firstQueryValue(r.URL.RawQuery, "scope")
			if err != nil {
				writeNegotiatedString(w, r, http.StatusBadRequest, "削除範囲が不正です。")
				return
			}
			scope = value
		}
		err := deleteEntry(r.Context(), id, user, scope)
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
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
