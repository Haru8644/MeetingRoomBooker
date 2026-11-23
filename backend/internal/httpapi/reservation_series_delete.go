package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
)

type reservationSeriesDeleter func(context.Context, int, int, bool, string) error

func newReservationSeriesDeleteHandler(remove reservationSeriesDeleter) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		if !hasJSONContentType(r) {
			writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 30_000_000))
		request := &reservationSeriesRequest{Scope: new("single")}
		err := readReservationSeriesJSON(decoder, request)
		problems := make(map[string][]string)
		if err == nil {
			err = request.validate(time.Now(), problems)
		}
		if writeReservationSeriesInputProblem(w, err, problems, "A valid series delete JSON object is required.") {
			return
		}
		writeReservationMutationResult(w, r, remove(r.Context(), id, user.ID, user.IsAdmin, *request.Scope))
	}
}
