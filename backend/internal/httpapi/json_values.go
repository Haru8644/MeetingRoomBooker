package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ASP.NET's web JSON options accept quoted numbers for integer fields.
type jsonInt32 int32

func (value *jsonInt32) UnmarshalJSON(data []byte) error {
	number := string(data)
	if strings.HasPrefix(number, `"`) {
		if err := json.Unmarshal(data, &number); err != nil {
			return err
		}
	}
	parsed, err := strconv.ParseInt(number, 10, 32)
	*value = jsonInt32(parsed)
	return err
}

type jsonDateTime struct {
	time.Time
	suffix string
}

func (value *jsonDateTime) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	for _, layout := range []string{
		time.RFC3339Nano, "2006-01-02T15:04Z07:00", "2006-01-02T15:04:05", "2006-01-02T15:04", time.DateOnly,
	} {
		parsed, err := time.Parse(layout, text)
		if err != nil {
			continue
		}
		var suffix string
		if strings.HasSuffix(layout, "Z07:00") {
			if strings.HasSuffix(text, "Z") {
				suffix = "Z"
			} else {
				parsed = parsed.In(time.Local)
				suffix = parsed.Format("-07:00")
			}
		}
		if parsed.Year() < 1 || parsed.Year() > 9999 {
			break
		}
		*value = jsonDateTime{Time: parsed, suffix: suffix}
		return nil
	}
	return errors.New("expected an ISO 8601 date or date-time")
}

func readJSONRequest[T any](w http.ResponseWriter, r *http.Request, request **T) bool {
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
