package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
)

type notificationInbox interface {
	List(ctx context.Context, userID int) ([]notifications.Entry, error)
	MarkRead(ctx context.Context, id, actorID int, isAdmin bool) error
	Delete(ctx context.Context, id, actorID int, isAdmin bool) error
}

func (h sessionHandler) registerNotificationRoutes(mux *http.ServeMux, origins *http.CrossOriginProtection, inbox notificationInbox) {
	mux.Handle("GET /api/Notifications/user/{userId}", h.notificationRoute("userId", func(w http.ResponseWriter, r *http.Request, user auth.User, userID int) {
		if user.ID != userID && !user.IsAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		entries, err := inbox.List(r.Context(), userID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, entries)
	}))
	mux.Handle("PUT /api/Notifications/{id}/read", origins.Handler(h.notificationRoute("id", func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		writeNotificationMutation(w, inbox.MarkRead(r.Context(), id, user.ID, user.IsAdmin))
	})))
	mux.Handle("DELETE /api/Notifications/{id}", origins.Handler(h.notificationRoute("id", func(w http.ResponseWriter, r *http.Request, user auth.User, id int) {
		writeNotificationMutation(w, inbox.Delete(r.Context(), id, user.ID, user.IsAdmin))
	})))
}

func (h sessionHandler) notificationRoute(parameter string, next func(http.ResponseWriter, *http.Request, auth.User, int)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue(parameter)), 10, 32)
		if err != nil {
			// ASP.NET's int route constraint returns 404 before performing authentication.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.requireSession(func(w http.ResponseWriter, r *http.Request, user auth.User) {
			next(w, r, user, int(id))
		}).ServeHTTP(w, r)
	})
}

func writeNotificationMutation(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, notifications.ErrForbidden):
		w.WriteHeader(http.StatusForbidden)
	case errors.Is(err, notifications.ErrNotFound):
		writeProblemDetails(w, problemDetails{
			Type: "https://tools.ietf.org/html/rfc9110#section-15.5.5", Title: "Not Found", Status: http.StatusNotFound,
		})
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}
