package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
)

type userRegistrationRequest struct {
	Name                 *string
	Email                *string
	Password             *string
	AvatarColor          *string
	ChatworkAccountID    *string
	ChatworkDirectRoomID *string
}

func (h sessionHandler) registerUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !hasJSONContentType(r) {
		writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
		return
	}
	request := &userRegistrationRequest{Email: new(""), AvatarColor: new("")}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBodyBytes))
	err := decoder.Decode(&request)
	if err == io.EOF || (err == nil && request == nil) {
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{
			"": {"A non-empty request body is required."}, "request": {"The request field is required."},
		})
		return
	}
	if err == nil {
		err = decoder.Decode(new(json.RawMessage))
		if err == nil {
			err = errors.New("multiple JSON values")
		}
	}
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	case err != io.EOF:
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"request": {"A JSON request object is required."}})
		return
	}
	if fields := validateUserRegistration(request); len(fields) > 0 {
		writeValidationProblem(w, http.StatusBadRequest, fields)
		return
	}

	// A missing or expired session is allowed to reach the bootstrap check.
	var actorID *int
	if token := requestToken(r); token != "" {
		actor, renewed, err := h.service.Authenticate(r.Context(), token)
		if err == nil {
			actorID = &actor.ID
			if renewed != nil {
				h.setCookie(w, r, *renewed)
			}
		} else if !errors.Is(err, auth.ErrUnauthorized) {
			writeSessionError(w, err)
			return
		}
	}
	user, err := h.service.Register(r.Context(), auth.Registration{
		Name: *request.Name, Email: *request.Email, Password: *request.Password, AvatarColor: *request.AvatarColor,
		ChatworkAccountID: request.ChatworkAccountID, ChatworkDirectRoomID: request.ChatworkDirectRoomID,
	}, actorID)
	switch {
	case errors.Is(err, auth.ErrRegistrationForbidden):
		w.WriteHeader(http.StatusForbidden)
	case errors.Is(err, auth.ErrEmailConflict):
		writeNegotiatedString(w, r, http.StatusConflict, "Email already exists.")
	case err != nil:
		writeSessionError(w, err)
	default:
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		location := url.URL{Scheme: scheme, Host: r.Host, Path: "/api/Users", RawQuery: "id=" + strconv.Itoa(user.ID)}
		w.Header().Set("Location", location.String())
		writeJSON(w, http.StatusCreated, user)
	}
}

func validateUserRegistration(request *userRegistrationRequest) map[string][]string {
	fields := make(map[string][]string)
	for name, value := range map[string]*string{"Name": request.Name, "Email": request.Email, "Password": request.Password} {
		if value == nil || strings.TrimSpace(*value) == "" {
			fields[name] = []string{"The " + name + " field is required."}
		}
	}
	if request.Email != nil && !validEmailAddress(*request.Email) {
		fields["Email"] = append(fields["Email"], "The Email field is not a valid e-mail address.")
	}
	if request.AvatarColor == nil {
		fields["AvatarColor"] = []string{"The AvatarColor field is required."}
	}
	for _, field := range []struct {
		name  string
		value *string
		limit int
	}{
		{"Name", request.Name, 100}, {"Email", request.Email, 256},
		{"ChatworkAccountId", request.ChatworkAccountID, 100}, {"ChatworkDirectRoomId", request.ChatworkDirectRoomID, 100},
	} {
		if field.value != nil && len(utf16.Encode([]rune(*field.value))) > field.limit {
			fields[field.name] = append(fields[field.name],
				fmt.Sprintf("The field %s must be a string with a maximum length of %d.", field.name, field.limit))
		}
	}
	return fields
}
