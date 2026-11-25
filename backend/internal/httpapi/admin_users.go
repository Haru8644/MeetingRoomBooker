package httpapi

import (
	"context"
	"errors"
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
	mux.Handle("GET /api/Users", h.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		users, err := h.service.ListUsers(r.Context())
		if err != nil {
			writeSessionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, users)
	})))

	mux.Handle("PUT /api/Users/{id}/name", origins.Handler(h.adminUserRoute(func(w http.ResponseWriter, r *http.Request, _ auth.User, id int) {
		request := &struct{ Name *string }{Name: new("")}
		if !readJSONRequest(w, r, &request) {
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
		if readJSONRequest(w, r, &request) {
			writeAdminUserMutation(w, r, h.service.UpdateChatworkAccountID(r.Context(), id, request.ChatworkAccountID))
		}
	})))
	mux.Handle("PUT /api/Users/{id}/chatwork-direct-room", origins.Handler(h.adminUserRoute(func(w http.ResponseWriter, r *http.Request, _ auth.User, id int) {
		var request *struct{ ChatworkDirectRoomID *string }
		if readJSONRequest(w, r, &request) {
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

func (h sessionHandler) requireAdmin(next http.Handler) http.Handler {
	return h.requireSession(func(w http.ResponseWriter, r *http.Request, user auth.User) {
		if !user.IsAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
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
