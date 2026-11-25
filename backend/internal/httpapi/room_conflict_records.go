package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/roomconflicts"
)

const roomConflictPath = "/api/room-conflict-records"

type roomConflictResponse struct {
	roomconflicts.Record
	CanEdit bool `json:"canEdit"`
}

func newRoomConflictResponse(record roomconflicts.Record, user auth.User) roomConflictResponse {
	return roomConflictResponse{record, record.EditableBy(user.ID, user.IsAdmin)}
}

func (h sessionHandler) registerRoomConflictRoutes(mux *http.ServeMux, csrf *http.CrossOriginProtection, records *roomconflicts.Store) {
	mux.Handle("GET "+roomConflictPath, h.requireSession(func(w http.ResponseWriter, r *http.Request, user auth.User) {
		entries, err := records.List(r.Context())
		if err != nil {
			writeRoomConflictError(w, r, err)
			return
		}
		response := make([]roomConflictResponse, len(entries))
		for i, entry := range entries {
			response[i] = newRoomConflictResponse(entry, user)
		}
		writeJSON(w, http.StatusOK, response)
	}))
	mux.Handle("GET "+roomConflictPath+"/summary", h.requireSession(func(w http.ResponseWriter, r *http.Request, _ auth.User) {
		summary, err := records.Summary(r.Context(), time.Now())
		if err != nil {
			writeRoomConflictError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}))
	mux.Handle("GET "+roomConflictPath+"/{id}", h.idRoute("id", func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		record, err := records.Get(r.Context(), id)
		if err != nil {
			writeRoomConflictError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, newRoomConflictResponse(record, user))
	}))
	mux.Handle("POST "+roomConflictPath, csrf.Handler(h.requireSession(func(w http.ResponseWriter, r *http.Request, user auth.User) {
		report, ok := readRoomConflictReport(w, r)
		if !ok {
			return
		}
		record, err := records.Create(r.Context(), report, user.ID)
		if err != nil {
			writeRoomConflictError(w, r, err)
			return
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		location := url.URL{Scheme: scheme, Host: r.Host, Path: roomConflictPath + "/" + strconv.Itoa(record.ID)}
		w.Header().Set("Location", location.String())
		writeJSON(w, http.StatusCreated, newRoomConflictResponse(record, user))
	})))
	mux.Handle("PUT "+roomConflictPath+"/{id}", csrf.Handler(h.idRoute("id", func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		var request *struct {
			roomConflictFields
			Status roomConflictEnum
		}
		if !readJSONRequest(w, r, &request) {
			return
		}
		record, err := records.Update(r.Context(), id, user.ID, user.IsAdmin, request.fields(request.Status))
		if err != nil {
			writeRoomConflictError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, newRoomConflictResponse(record, user))
	})))
}

type roomConflictEnum int32

func (v *roomConflictEnum) UnmarshalJSON(data []byte) error {
	parsed, err := strconv.ParseInt(string(data), 10, 32)
	*v = roomConflictEnum(parsed)
	return err
}

type roomConflictFields struct {
	Impact, Cause           roomConflictEnum
	Description, Resolution string
}

func (f roomConflictFields) fields(status roomConflictEnum) roomconflicts.Fields {
	return roomconflicts.Fields{
		Status: int(status), Impact: int(f.Impact), Cause: int(f.Cause),
		Description: f.Description, Resolution: f.Resolution,
	}
}

func readRoomConflictReport(w http.ResponseWriter, r *http.Request) (roomconflicts.Report, bool) {
	request := &struct {
		roomConflictFields
		OccurredAt                     jsonDateTime
		RoomName                       *string
		ReservationIDA, ReservationIDB *jsonInt32
		Status                         *roomConflictEnum
	}{
		roomConflictFields: roomConflictFields{Impact: roomconflicts.Medium, Cause: roomconflicts.UnknownCause},
		RoomName:           new(""),
	}
	if !readJSONRequest(w, r, &request) {
		return roomconflicts.Report{}, false
	}
	if request.RoomName == nil {
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"RoomName": {"The RoomName field is required."}})
		return roomconflicts.Report{}, false
	}
	status := roomConflictEnum(roomconflicts.Confirmed)
	if request.Status != nil {
		status = *request.Status
	}
	return roomconflicts.Report{
		Fields: request.fields(status), OccurredAt: request.OccurredAt.Time, RoomName: *request.RoomName,
		ReservationIDA: (*int32)(request.ReservationIDA), ReservationIDB: (*int32)(request.ReservationIDB),
	}, true
}

func writeRoomConflictError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid roomconflicts.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeNegotiatedString(w, r, http.StatusBadRequest, invalid.Error())
	case errors.Is(err, roomconflicts.ErrForbidden):
		w.WriteHeader(http.StatusForbidden)
	case errors.Is(err, roomconflicts.ErrNotFound):
		writeProblemDetails(w, problemDetails{
			Type: "https://tools.ietf.org/html/rfc9110#section-15.5.5", Title: "Not Found", Status: http.StatusNotFound,
		})
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}
