package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/users"
)

const sessionCookieName = "MeetingRoomBooker.GoSession"

type sessionService interface {
	Login(context.Context, string, string, bool, string) (auth.User, auth.Session, error)
	Authenticate(context.Context, string) (auth.User, *auth.Session, error)
	Logout(context.Context, string) error
}

type SessionOptions struct {
	Development     bool
	FrontendOrigins []string
}

type sessionHandler struct {
	service     sessionService
	development bool
}

func RegisterSessionRoutes(mux *http.ServeMux, service sessionService, reservationList http.Handler,
	createReservation reservationCreator, listWorkSchedules workScheduleLister,
	listParticipants func(context.Context) ([]users.Participant, error), options SessionOptions,
) (http.Handler, error) {
	csrf := http.NewCrossOriginProtection()
	origins := make(map[string]bool)
	for _, origin := range options.FrontendOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.User != nil || parsed.ForceQuery {
			return nil, errors.New("invalid FRONTEND_ORIGINS: expected scheme://host[:port]")
		}
		host := parsed.Hostname()
		localHTTP := options.Development && parsed.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback())
		if parsed.Scheme != "https" && !localHTTP {
			return nil, errors.New("invalid FRONTEND_ORIGINS: use HTTPS origins or loopback HTTP in development")
		}
		if err := csrf.AddTrustedOrigin(origin); err != nil {
			return nil, errors.New("invalid FRONTEND_ORIGINS: expected scheme://host[:port]")
		}
		origins[origin] = true
	}

	h := sessionHandler{service: service, development: options.Development}
	mux.Handle("POST /api/Auth/login", csrf.Handler(http.HandlerFunc(h.login)))
	mux.Handle("GET /api/Auth/me", h.requireSession(func(w http.ResponseWriter, _ *http.Request, user auth.User) {
		writeJSON(w, http.StatusOK, user)
	}))
	mux.Handle("POST /api/Auth/logout", csrf.Handler(http.HandlerFunc(h.logout)))
	mux.Handle(ReservationListPattern, h.requireSession(func(w http.ResponseWriter, r *http.Request, user auth.User) {
		r = r.WithContext(WithAuthenticatedUser(r.Context(), user.ID))
		reservationList.ServeHTTP(w, r)
	}))
	mux.Handle(reservationCreatePattern, csrf.Handler(h.requireSession(newReservationCreateHandler(createReservation))))
	mux.Handle(workScheduleListPattern, h.requireSession(newWorkScheduleListHandler(listWorkSchedules)))
	mux.Handle("GET /api/Users/participants", h.requireSession(func(w http.ResponseWriter, r *http.Request, _ auth.User) {
		participants, err := listParticipants(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, participants)
	}))

	if len(origins) == 0 {
		return mux, nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func (h sessionHandler) requireSession(next func(http.ResponseWriter, *http.Request, auth.User)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user, renewed, err := h.service.Authenticate(r.Context(), requestToken(r))
		if err != nil {
			writeSessionError(w, err)
			return
		}
		if renewed != nil {
			h.setCookie(w, r, *renewed)
		}
		next(w, r, user)
	})
}

func (h sessionHandler) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := h.service.Logout(r.Context(), requestToken(r)); err != nil {
		writeSessionError(w, err)
		return
	}
	cookie := h.cookie(r, "")
	cookie.Expires = time.Unix(0, 0).UTC()
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
	w.WriteHeader(http.StatusNoContent)
}

func requestToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h sessionHandler) setCookie(w http.ResponseWriter, r *http.Request, session auth.Session) {
	cookie := h.cookie(r, session.Token)
	if session.RememberMe {
		cookie.Expires = session.ExpiresAt
	}
	http.SetCookie(w, cookie)
}

func (h sessionHandler) cookie(r *http.Request, value string) *http.Cookie {
	return &http.Cookie{
		Name: sessionCookieName, Value: value, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: !h.development || r.TLS != nil,
	}
}

func writeSessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		w.WriteHeader(http.StatusUnauthorized)
	case errors.Is(err, auth.ErrUnavailable):
		w.WriteHeader(http.StatusServiceUnavailable)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, response any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
