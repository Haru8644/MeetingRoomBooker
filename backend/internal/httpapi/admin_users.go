package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
)

type userAdministration interface {
	ListUsers(context.Context) ([]auth.User, error)
	UpdateUserName(context.Context, int, string) error
	UpdateChatworkAccountID(context.Context, int, *string) error
	UpdateChatworkDirectRoomID(context.Context, int, *string) error
	DeleteUser(context.Context, int, int) error
}

func (h sessionHandler) registerAdminUserRoutes(mux *http.ServeMux, origins *http.CrossOriginProtection) {
	mux.Handle("GET /api/Users", h.requireSession(func(w http.ResponseWriter, r *http.Request, user auth.User) {
		if !user.IsAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		users, err := h.service.ListUsers(r.Context())
		if err != nil {
			writeSessionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, users)
	}))

	mux.Handle("PUT /api/Users/{id}/name", origins.Handler(h.adminUserRoute(func(w http.ResponseWriter, r *http.Request, _ auth.User, id int) {
		request := &struct{ Name *string }{Name: new("")}
		if !readAdminUserUpdate(w, r, &request) {
			return
		}
		if request.Name == nil {
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"Name": {"The Name field is required."}})
			return
		}
		writeAdminUserMutation(w, r, h.service.UpdateUserName(r.Context(), id, *request.Name))
	})))
	mux.Handle("PUT /api/Users/{id}/chatwork-account", origins.Handler(h.adminUserRoute(func(w http.ResponseWriter, r *http.Request, _ auth.User, id int) {
		var request *struct{ ChatworkAccountID *string }
		if readAdminUserUpdate(w, r, &request) {
			writeAdminUserMutation(w, r, h.service.UpdateChatworkAccountID(r.Context(), id, request.ChatworkAccountID))
		}
	})))
	mux.Handle("PUT /api/Users/{id}/chatwork-direct-room", origins.Handler(h.adminUserRoute(func(w http.ResponseWriter, r *http.Request, _ auth.User, id int) {
		var request *struct{ ChatworkDirectRoomID *string }
		if readAdminUserUpdate(w, r, &request) {
			writeAdminUserMutation(w, r, h.service.UpdateChatworkDirectRoomID(r.Context(), id, request.ChatworkDirectRoomID))
		}
	})))
	mux.Handle("DELETE /api/Users/{id}", origins.Handler(h.adminUserRoute(func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		writeAdminUserMutation(w, r, h.service.DeleteUser(r.Context(), id, user.ID))
	})))
}

func (h sessionHandler) adminUserRoute(next func(http.ResponseWriter, *http.Request, auth.User, int)) http.Handler {
	return h.idRoute("id", func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		if !user.IsAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r, user, id)
	})
}

func readAdminUserUpdate[T any](w http.ResponseWriter, r *http.Request, request **T) bool {
	if !hasJSONContentType(r) {
		writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 30_000_000))
	err := decoder.Decode(request)
	empty := err == io.EOF
	if err == nil {
		err = decoder.Decode(new(json.RawMessage))
		empty = *request == nil && err == io.EOF
		if err == nil {
			err = errors.New("expected one JSON object")
		}
	}
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	case empty:
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{
			"": {"A non-empty request body is required."}, "request": {"The request field is required."},
		})
	case err != io.EOF:
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"request": {"A JSON request object is required."}})
	default:
		return true
	}
	return false
}

func writeAdminUserMutation(w http.ResponseWriter, r *http.Request, err error) {
	var validation auth.UserValidationError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, auth.ErrUserNotFound):
		writeProblemDetails(w, problemDetails{
			Type: "https://tools.ietf.org/html/rfc9110#section-15.5.5", Title: "Not Found", Status: http.StatusNotFound,
		})
	case errors.As(err, &validation):
		writeNegotiatedString(w, r, http.StatusBadRequest, validation.Error())
	default:
		writeSessionError(w, err)
	}
}
