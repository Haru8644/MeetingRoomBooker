package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const workScheduleListPattern = "GET " + workSchedulePath

type workScheduleLister func(context.Context, time.Time, time.Time) ([]workschedules.Entry, error)

func newWorkScheduleListHandler(list workScheduleLister) func(http.ResponseWriter, *http.Request, auth.User) {
	return func(w http.ResponseWriter, r *http.Request, _ auth.User) {
		from, fromError := workScheduleDate(r.URL.RawQuery, "from")
		to, toError := workScheduleDate(r.URL.RawQuery, "to")
		if fromError != "" || toError != "" {
			problems := make(map[string][]string)
			if fromError != "" {
				problems["from"] = []string{fromError}
			}
			if toError != "" {
				problems["to"] = []string{toError}
			}
			writeValidationProblem(w, http.StatusBadRequest, problems)
			return
		}
		if from != nil && to != nil && from.After(*to) {
			writeNegotiatedString(w, r, http.StatusBadRequest, "取得開始日は取得終了日以前にしてください。")
			return
		}
		fromDate, toDate := workScheduleBounds(from, to, time.Now())
		if fromDate.After(toDate) {
			writeJSON(w, http.StatusOK, []workschedules.Entry{})
			return
		}
		entries, err := list(r.Context(), fromDate, toDate)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, entries)
	}
}

func workScheduleDate(query, name string) (*time.Time, string) {
	value, _, err := firstQueryValue(query, name)
	if err != nil {
		return nil, fmt.Sprintf("The value '%s' is not valid.", value)
	}
	text := strings.TrimSpace(value)
	if text == "" {
		return nil, ""
	}
	for _, layout := range []string{
		time.DateOnly, "2006-1-2", "1/2/2006", "2006/1/2", time.RFC3339Nano,
		"2006-01-02T15:04Z07:00", "2006-01-02T15:04:05", "2006-01-02T15:04",
		time.DateTime, "2006-01-02 15:04",
	} {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			// ASP.NET query binding converts explicit offsets to UTC before taking Date.
			date := dotnettime.OnDate(parsed.UTC(), time.Time{})
			if date.Year() >= 1 && date.Year() <= 9999 {
				return &date, ""
			}
		}
	}
	return nil, fmt.Sprintf("The value '%s' is not valid.", value)
}

func workScheduleBounds(from, to *time.Time, today time.Time) (time.Time, time.Time) {
	midnight := dotnettime.OnDate(today, time.Time{})
	start, end := midnight.AddDate(0, 0, -7), midnight.AddDate(0, 0, 31)
	if from != nil {
		start = *from
	}
	if to != nil {
		end = *to
	}
	return start, end
}
