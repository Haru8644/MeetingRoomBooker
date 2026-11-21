package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/notifications"
)

type notificationCreateInput struct {
	ID                        jsonInt32
	UserID                    jsonInt32
	Message                   *string
	CreatedAt                 jsonDateTime
	TargetDate                jsonDateTime
	IsRead                    *bool
	Type                      *string
	TargetReservationID       *jsonInt32
	TargetWorkScheduleEntryID *jsonInt32
}

func newAdminNotificationCreateHandler(inbox notificationInbox) func(http.ResponseWriter, *http.Request, auth.User) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User) {
		if !user.IsAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if !hasJSONContentType(r) {
			writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 30_000_000))
		input, err := readNotificationCreateInput(decoder)
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		case err == io.EOF:
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{
				"": {"A non-empty request body is required."}, "notification": {"The notification field is required."},
			})
			return
		case err != nil:
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{
				"notification": {"A valid notification JSON object is required."},
			})
			return
		}
		if problems := input.validate(); len(problems) > 0 {
			writeValidationProblem(w, http.StatusBadRequest, problems)
			return
		}
		entry := input.entry()
		entry.ID, err = inbox.Create(r.Context(), entry,
			dotnettime.Ticks(dotnettime.OnDate(input.CreatedAt.Time, input.CreatedAt.Time)),
			dotnettime.Ticks(dotnettime.OnDate(input.TargetDate.Time, input.TargetDate.Time)))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		location := url.URL{Scheme: scheme, Host: r.Host, Path: "/api/Notifications/user/" + strconv.Itoa(entry.UserID)}
		w.Header().Set("Location", location.String())
		writeJSON(w, http.StatusCreated, entry)
	}
}

func readNotificationCreateInput(decoder *json.Decoder) (*notificationCreateInput, error) {
	localDate := func() jsonDateTime {
		now := time.Now()
		return jsonDateTime{Time: now, suffix: now.Format("-07:00")}
	}
	input := &notificationCreateInput{
		Message: new(""), Type: new("Info"), IsRead: new(false),
		CreatedAt: localDate(), TargetDate: localDate(),
	}
	if err := decoder.Decode(&input); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		if err == nil {
			err = errors.New("expected one JSON object")
		}
		return nil, err
	}
	if input == nil {
		return nil, io.EOF
	}
	if input.IsRead == nil {
		return nil, errors.New("expected a boolean isRead")
	}
	return input, nil
}

func (input *notificationCreateInput) validate() map[string][]string {
	problems := make(map[string][]string)
	if input.Message == nil {
		problems["Message"] = []string{"The Message field is required."}
	}
	if input.Type == nil {
		problems["Type"] = []string{"The Type field is required."}
	}
	return problems
}

func (input *notificationCreateInput) entry() notifications.Entry {
	entry := notifications.Entry{
		ID: int(input.ID), UserID: int(input.UserID), Message: *input.Message, Type: *input.Type, IsRead: *input.IsRead,
		CreatedAt:  input.CreatedAt.Format(dotnettime.Layout) + input.CreatedAt.suffix,
		TargetDate: input.TargetDate.Format(dotnettime.Layout) + input.TargetDate.suffix,
	}
	if input.TargetReservationID != nil {
		entry.TargetReservationID = new(int(*input.TargetReservationID))
	}
	if input.TargetWorkScheduleEntryID != nil {
		entry.TargetWorkScheduleEntryID = new(int(*input.TargetWorkScheduleEntryID))
	}
	return entry
}
