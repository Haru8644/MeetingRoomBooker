package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

const noRepeat = "しない"

type reservationInput struct {
	ID             jsonInt32
	UserID         jsonInt32
	Name           *string
	Room           *string
	NumberOfPeople jsonInt32
	Type           *string
	Purpose        *string
	SeriesID       *string
	Date           reservationDateTime
	StartTime      reservationDateTime
	EndTime        reservationDateTime
	ParticipantIDs []jsonInt32
	Participants   *string
	RepeatType     *string
	RepeatUntil    *reservationDateTime
}

func readReservationInput(decoder *json.Decoder, now time.Time, update bool) (reservationInput, error) {
	first, err := decoder.Token()
	if err != nil {
		return reservationInput{}, err
	}
	if first == nil {
		if err := decoder.Decode(new(json.RawMessage)); err != nil {
			return reservationInput{}, err
		}
	}
	if first != json.Delim('{') {
		return reservationInput{}, errors.New("expected a JSON object")
	}

	// C# aliases share setters, so the last occurrence of a field or its alias wins.
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return reservationInput{}, err
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return reservationInput{}, err
		}
		field := strings.ToLower(key.(string))
		switch field {
		case "username":
			field = "name"
		case "roomname":
			field = "room"
		case "participantcount":
			field = "numberofpeople"
		case "isinternal":
			field = "type"
			switch string(value) {
			case "true":
				value = json.RawMessage(`"社内"`)
			case "false":
				value = json.RawMessage(`"来客"`)
			default:
				return reservationInput{}, errors.New("expected a boolean isInternal")
			}
		}
		fields[field] = value
	}
	if _, err := decoder.Token(); err != nil {
		return reservationInput{}, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err == nil {
		return reservationInput{}, errors.New("expected one JSON object")
	} else if err != io.EOF {
		return reservationInput{}, err
	}
	if !update {
		delete(fields, "id")
		delete(fields, "userid")
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return reservationInput{}, err
	}
	input := reservationInput{
		NumberOfPeople: 1, Type: new("社内"), Purpose: new(""), Date: reservationDateTime{dotnettime.OnDate(now, time.Time{})},
		ParticipantIDs: []jsonInt32{}, Participants: new(""), RepeatType: new(noRepeat),
	}
	err = json.Unmarshal(raw, &input)
	return input, err
}

func (input reservationInput) validate() map[string][]string {
	problems := make(map[string][]string)
	if input.Name == nil || strings.TrimSpace(*input.Name) == "" {
		problems["Name"] = []string{"名前は必須です。"}
	} else if len(utf16.Encode([]rune(*input.Name))) > 20 {
		problems["Name"] = []string{"名前は20文字以内で入力してください。"}
	}
	if input.Room == nil || strings.TrimSpace(*input.Room) == "" {
		problems["Room"] = []string{"会議室は必須です。"}
	}
	if input.NumberOfPeople < 1 || input.NumberOfPeople > 20 {
		problems["NumberOfPeople"] = []string{"人数は1～20名の間で設定してください。"}
	}
	if input.SeriesID != nil && len(utf16.Encode([]rune(*input.SeriesID))) > 50 {
		problems["SeriesId"] = []string{"The field SeriesId must be a string with a maximum length of 50."}
	}
	for field, value := range map[string]*string{
		"Type": input.Type, "Purpose": input.Purpose, "Participants": input.Participants, "RepeatType": input.RepeatType,
	} {
		if value == nil {
			problems[field] = []string{"The " + field + " field is required."}
		}
	}
	if input.ParticipantIDs == nil {
		problems["ParticipantIds"] = []string{"The ParticipantIds field is required."}
	}
	return problems
}

func (input reservationInput) normalize(user auth.User) (reservations.ListEntry, string) {
	entry := input.entry(user.ID, user.Name)
	if !entry.StartTime.Before(entry.EndTime) {
		return reservations.ListEntry{}, "終了時刻は開始時刻より後にしてください。"
	}
	if entry.RepeatUntil != nil {
		return reservations.ListEntry{}, "繰り返し予約には対応していません。"
	}
	return entry, ""
}

func (input reservationInput) entry(userID int, name string) reservations.ListEntry {
	date := input.Date.Time
	start := dotnettime.OnDate(date, input.StartTime.Time)
	end := dotnettime.OnDate(date, input.EndTime.Time)
	repeat := *input.RepeatType
	if strings.TrimSpace(repeat) == "" {
		repeat = noRepeat
	}
	var repeatUntil *time.Time
	if repeat != noRepeat && input.RepeatUntil != nil {
		repeatUntil = &input.RepeatUntil.Time
	}
	participants := make([]int, 0, len(input.ParticipantIDs)+1)
	seen := make(map[int]bool, len(input.ParticipantIDs))
	for _, id := range input.ParticipantIDs {
		if id > 0 && !seen[int(id)] {
			participants = append(participants, int(id))
			seen[int(id)] = true
		}
	}
	if userID > 0 && !seen[userID] {
		participants = append([]int{userID}, participants...)
	}
	return reservations.ListEntry{
		Reservation: reservations.Reservation{Room: *input.Room, Date: date, StartTime: start, EndTime: end},
		ID:          int(input.ID), UserID: userID, Name: name, NumberOfPeople: int(input.NumberOfPeople), Type: *input.Type,
		Purpose: *input.Purpose, Participants: *input.Participants, ParticipantIDs: participants, RepeatType: repeat,
		RepeatUntil: repeatUntil,
	}
}

type reservationDateTime struct{ time.Time }

func (value *reservationDateTime) UnmarshalJSON(data []byte) error {
	var parsed jsonDateTime
	if err := parsed.UnmarshalJSON(data); err != nil {
		return err
	}
	value.Time = dotnettime.OnDate(parsed.Time, parsed.Time).Truncate(100 * time.Nanosecond)
	return nil
}
