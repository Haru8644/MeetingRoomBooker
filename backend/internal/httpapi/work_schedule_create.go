package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf16"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/workschedules"
)

const workSchedulePath = "/api/work-schedule-entries"

type workScheduleCreator func(context.Context, workschedules.EntryInput, int, bool) ([]workschedules.Entry, error)
type workScheduleGetter func(context.Context, int) (*workschedules.Entry, error)

func newWorkScheduleGetHandler(get workScheduleGetter) func(http.ResponseWriter, *http.Request, auth.User, int) {
	return func(w http.ResponseWriter, r *http.Request, _ auth.User, id int) {
		entry, err := get(r.Context(), id)
		switch {
		case err != nil:
			w.WriteHeader(http.StatusInternalServerError)
		case entry == nil:
			writeWorkScheduleNotFound(w)
		default:
			writeJSON(w, http.StatusOK, entry)
		}
	}
}

func newWorkScheduleCreateHandler(create workScheduleCreator, series bool) func(http.ResponseWriter, *http.Request, auth.User) {
	return func(w http.ResponseWriter, r *http.Request, user auth.User) {
		input, ok := decodeWorkScheduleInput(w, r)
		if !ok {
			return
		}

		entries, err := create(r.Context(), input, user.ID, series)
		var invalid workschedules.ValidationError
		switch {
		case errors.As(err, &invalid):
			writeNegotiatedString(w, r, http.StatusBadRequest, invalid.Error())
		case err != nil:
			w.WriteHeader(http.StatusInternalServerError)
		case series:
			writeJSON(w, http.StatusOK, entries)
		default:
			entry := entries[0]
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			location := url.URL{Scheme: scheme, Host: r.Host, Path: workSchedulePath + "/" + strconv.Itoa(entry.ID)}
			w.Header().Set("Location", location.String())
			writeJSON(w, http.StatusCreated, entry)
		}
	}
}

func decodeWorkScheduleInput(w http.ResponseWriter, r *http.Request) (workschedules.EntryInput, bool) {
	if !hasJSONContentType(r) {
		writeValidationProblem(w, http.StatusUnsupportedMediaType, nil)
		return workschedules.EntryInput{}, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 30_000_000))
	input, problems, err := readWorkScheduleInput(decoder, time.Now())
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return workschedules.EntryInput{}, false
	case err != nil:
		writeValidationProblem(w, http.StatusBadRequest, workScheduleBindingProblem(err))
		return workschedules.EntryInput{}, false
	case len(problems) > 0:
		writeValidationProblem(w, http.StatusBadRequest, problems)
		return workschedules.EntryInput{}, false
	}
	return input, true
}

func readWorkScheduleInput(decoder *json.Decoder, now time.Time) (workschedules.EntryInput, map[string][]string, error) {
	request := &struct {
		Type           *int32          `json:"type"`
		Title          *string         `json:"title"`
		Date           json.RawMessage `json:"date"`
		StartTime      json.RawMessage `json:"startTime"`
		EndTime        json.RawMessage `json:"endTime"`
		RepeatType     *string         `json:"repeatType"`
		RepeatUntil    json.RawMessage `json:"repeatUntil"`
		LeavePeriod    *int32          `json:"leavePeriod"`
		ParticipantIDs []jsonInt32     `json:"participantIds"`
		Participants   *string         `json:"participants"`
	}{Type: new(int32), Title: new(""), LeavePeriod: new(int32), ParticipantIDs: []jsonInt32{}, Participants: new("")}
	var input workschedules.EntryInput
	if err := decoder.Decode(&request); err != nil {
		return input, nil, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		if err == nil {
			err = errors.New("expected one JSON object")
		}
		return input, nil, err
	}
	if request == nil {
		return input, nil, io.EOF
	}

	input.Date = dotnettime.OnDate(now, time.Time{})
	for _, field := range []struct {
		name string
		raw  json.RawMessage
		dest **time.Time
	}{
		{"date", request.Date, nil}, {"startTime", request.StartTime, &input.Start},
		{"endTime", request.EndTime, &input.End}, {"repeatUntil", request.RepeatUntil, &input.RepeatUntil},
	} {
		if len(field.raw) == 0 || (field.dest != nil && string(field.raw) == "null") {
			continue
		}
		var parsed jsonDateTime
		if err := json.Unmarshal(field.raw, &parsed); err != nil {
			return input, workScheduleConversionProblem(field.name), nil
		}
		if field.dest == nil {
			input.Date = parsed.Time
		} else {
			*field.dest = &parsed.Time
		}
	}
	if request.Type == nil {
		return input, workScheduleConversionProblem("type"), nil
	}
	if request.LeavePeriod == nil {
		return input, workScheduleConversionProblem("leavePeriod"), nil
	}
	problems := make(map[string][]string)
	if request.Title == nil {
		problems["Title"] = []string{"The Title field is required."}
	} else if len(utf16.Encode([]rune(*request.Title))) > 100 {
		problems["Title"] = []string{"内容は100文字以内で入力してください。"}
	}
	if request.Participants == nil {
		problems["Participants"] = []string{"The Participants field is required."}
	}
	if request.ParticipantIDs == nil {
		problems["ParticipantIds"] = []string{"The ParticipantIds field is required."}
	}
	if len(problems) > 0 {
		return input, problems, nil
	}

	input.Type, input.LeavePeriod = int(*request.Type), int(*request.LeavePeriod)
	input.Title, input.Participants = *request.Title, *request.Participants
	input.ParticipantIDs = make([]int, len(request.ParticipantIDs))
	for i, id := range request.ParticipantIDs {
		input.ParticipantIDs[i] = int(id)
	}
	if request.RepeatType != nil {
		input.RepeatType = *request.RepeatType
	}
	return input, nil, nil
}

func workScheduleBindingProblem(err error) map[string][]string {
	var invalid *json.UnmarshalTypeError
	if errors.As(err, &invalid) && invalid.Field != "" {
		return workScheduleConversionProblem(invalid.Field)
	}
	if err == io.EOF {
		return map[string][]string{"": {"A non-empty request body is required."}, "request": {"The request field is required."}}
	}
	return map[string][]string{"request": {"A valid work schedule JSON object is required."}}
}

func workScheduleConversionProblem(field string) map[string][]string {
	expected := map[string]string{
		"type": "MeetingRoomBooker.Shared.Models.WorkScheduleEntryType", "leavePeriod": "MeetingRoomBooker.Shared.Models.LeavePeriod",
		"date": "System.DateTime", "startTime": "System.Nullable`1[System.DateTime]",
		"endTime": "System.Nullable`1[System.DateTime]", "repeatUntil": "System.Nullable`1[System.DateTime]",
		"title": "System.String", "participants": "System.String", "repeatType": "System.String",
		"participantIds": "System.Collections.Generic.List`1[System.Int32]",
	}[field]
	path := "$." + field
	return map[string][]string{
		"request": {"The request field is required."},
		path:      {"The JSON value could not be converted to " + expected + ". Path: " + path},
	}
}
