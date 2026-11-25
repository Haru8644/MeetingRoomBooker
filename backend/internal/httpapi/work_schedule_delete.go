package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

type workScheduleDeleter func(context.Context, int, auth.User) error

func newWorkScheduleDeleteHandler(deleteEntry workScheduleDeleter) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		err := deleteEntry(r.Context(), id, user)
		switch {
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
