package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

const reservationCreatePattern = "POST /api/Reservations"

type reservationCreator func(context.Context, reservations.ListEntry, bool) (reservations.ListEntry, error)

func newReservationCreateHandler(create reservationCreator) func(http.ResponseWriter, *http.Request, auth.User) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User) {
		if !hasJSONContentType(r) {
			writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 30_000_000))
		input, err := readReservationCreateInput(decoder, time.Now())
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if err == io.EOF {
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{
				"": {"A non-empty request body is required."}, "reservation": {"The reservation field is required."},
			})
			return
		}
		if err != nil {
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"reservation": {"A valid reservation JSON object is required."}})
			return
		}
		allowOverlap, message := readAllowOverlap(r.URL.RawQuery)
		problems := input.validate()
		if message != "" {
			problems["allowOverlap"] = []string{message}
		}
		if len(problems) > 0 {
			writeValidationProblem(w, http.StatusBadRequest, problems)
			return
		}
		entry, message := input.normalize(user)
		if message != "" {
			writeNegotiatedString(w, r, http.StatusBadRequest, message)
			return
		}

		created, err := create(r.Context(), entry, allowOverlap)
		var conflict *reservations.ConflictError
		switch {
		case errors.As(err, &conflict):
			writeNegotiatedString(w, r, http.StatusConflict, conflict.Error())
		case err != nil:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			location := url.URL{Scheme: scheme, Host: r.Host, Path: "/api/Reservations", RawQuery: "id=" + strconv.Itoa(created.ID)}
			w.Header().Set("Location", location.String())
			writeJSON(w, http.StatusCreated, newReservationResponse(created))
		}
	}
}

func readAllowOverlap(query string) (bool, string) {
	value, found, err := firstQueryValue(query, "allowOverlap")
	if err != nil {
		return false, fmt.Sprintf("The value '%s' is not valid.", value)
	}
	if !found {
		return false, ""
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true, ""
	case "false":
		return false, ""
	case "":
		return false, fmt.Sprintf("The value '%s' is invalid.", value)
	default:
		return false, fmt.Sprintf("The value '%s' is not valid.", value)
	}
}
