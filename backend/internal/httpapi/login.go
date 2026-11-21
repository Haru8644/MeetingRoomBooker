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
	if !hasJSONContentType(r) {
		writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
		return
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	request := &loginRequest{Email: new("")}
	err := decoder.Decode(&request)
	if err == io.EOF || (err == nil && request == nil) {
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{
			"": {"A non-empty request body is required."}, "request": {"The request field is required."},
		})
		return
	}
	if err != nil || decoder.Decode(new(json.RawMessage)) != io.EOF {
		writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"request": {"A JSON request object is required."}})
		return
	}
	fields := validateLogin(request)
	if len(fields) != 0 {
		writeValidationProblem(w, http.StatusBadRequest, fields)
		return
	}

	user, session, err := h.service.Login(r.Context(), strings.ToLower(strings.TrimSpace(*request.Email)),
		request.Password, string(request.RememberMe) == "true", requestToken(r))
	if errors.Is(err, auth.ErrUnauthorized) {
		writeNegotiatedString(w, r, http.StatusUnauthorized, "Invalid email or password.")
		return
	}
	if err != nil {
		writeSessionError(w, err)
		return
	}
	h.setCookie(w, r, session)
	writeJSON(w, http.StatusOK, user)
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

func hasJSONContentType(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && (mediaType == "application/json" || mediaType == "text/json" ||
		strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
}

func writeNegotiatedString(w http.ResponseWriter, r *http.Request, status int, message string) {
	for _, accept := range strings.Split(r.Header.Get("Accept"), ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(accept))
		if err == nil && mediaType == "application/json" && params["q"] != "0" {
			writeJSON(w, status, message)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, message)
}

type problemDetails struct {
	Type   string              `json:"type"`
	Title  string              `json:"title"`
	Status int                 `json:"status"`
	Errors map[string][]string `json:"errors,omitempty"`
}

func writeValidationProblem(w http.ResponseWriter, status int, fields map[string][]string) {
	const rfc = "https://tools.ietf.org/html/rfc9110#section-"
	problem := problemDetails{rfc + "15.5.1", "One or more validation errors occurred.", status, fields}
	if status == http.StatusUnsupportedMediaType {
		problem.Type = rfc + "15.5.16"
		problem.Title = http.StatusText(status)
	}
	writeProblemDetails(w, problem)
}

func writeProblemDetails(w http.ResponseWriter, problem problemDetails) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}
