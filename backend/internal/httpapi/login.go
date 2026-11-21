package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
)

type loginRequest struct {
	Email      *string
	Password   string
	RememberMe json.RawMessage
}

func (h sessionHandler) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/json" && mediaType != "text/json" &&
		!(strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))) {
		writeLoginProblem(w, http.StatusUnsupportedMediaType, nil)
		return
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	request := &loginRequest{Email: new("")}
	err = decoder.Decode(&request)
	if err == io.EOF || (err == nil && request == nil) {
		writeLoginProblem(w, http.StatusBadRequest, map[string][]string{
			"": {"A non-empty request body is required."}, "request": {"The request field is required."},
		})
		return
	}
	if err != nil || decoder.Decode(new(json.RawMessage)) != io.EOF {
		writeLoginProblem(w, http.StatusBadRequest, map[string][]string{"request": {"A JSON request object is required."}})
		return
	}
	fields := validateLogin(request)
	if len(fields) != 0 {
		writeLoginProblem(w, http.StatusBadRequest, fields)
		return
	}

	user, session, err := h.service.Login(r.Context(), strings.ToLower(strings.TrimSpace(*request.Email)),
		request.Password, string(request.RememberMe) == "true", requestToken(r))
	if errors.Is(err, auth.ErrUnauthorized) {
		writeCredentialFailure(w, r)
		return
	}
	if err != nil {
		writeSessionError(w, err)
		return
	}
	h.setCookie(w, r, session)
	writeAuthJSON(w, http.StatusOK, user)
}

func validateLogin(request *loginRequest) map[string][]string {
	fields := make(map[string][]string)
	if request.Email == nil || strings.TrimSpace(*request.Email) == "" {
		fields["Email"] = []string{"The Email field is required."}
	}
	if request.Email != nil {
		email := *request.Email
		at := strings.IndexByte(email, '@')
		if at < 1 || at == len(email)-1 || at != strings.LastIndexByte(email, '@') || strings.ContainsAny(email, "\r\n") {
			fields["Email"] = append(fields["Email"], "The Email field is not a valid e-mail address.")
		}
	}
	if strings.TrimSpace(request.Password) == "" {
		fields["Password"] = []string{"The Password field is required."}
	}
	if remember := string(request.RememberMe); remember != "" && remember != "true" && remember != "false" {
		fields["rememberMe"] = []string{"The rememberMe field must be a boolean."}
	}
	return fields
}

func writeCredentialFailure(w http.ResponseWriter, r *http.Request) {
	const message = "Invalid email or password."
	for _, accept := range strings.Split(r.Header.Get("Accept"), ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(accept))
		if err == nil && mediaType == "application/json" && params["q"] != "0" {
			writeAuthJSON(w, http.StatusUnauthorized, message)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = io.WriteString(w, message)
}

func writeLoginProblem(w http.ResponseWriter, status int, fields map[string][]string) {
	const rfc = "https://tools.ietf.org/html/rfc9110#section-"
	problem := struct {
		Type   string              `json:"type"`
		Title  string              `json:"title"`
		Status int                 `json:"status"`
		Errors map[string][]string `json:"errors,omitempty"`
	}{rfc + "15.5.1", "One or more validation errors occurred.", status, fields}
	if status == http.StatusUnsupportedMediaType {
		problem.Type = rfc + "15.5.16"
		problem.Title = http.StatusText(status)
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}
