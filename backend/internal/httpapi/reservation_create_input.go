package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/auth"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
)

const noRepeat = "しない"

type reservationCreateInput struct {
	Name           *string
	Room           *string
	NumberOfPeople reservationInt
	Type           *string
	Purpose        *string
	SeriesID       *string
	Date           reservationDateTime
	StartTime      reservationDateTime
	EndTime        reservationDateTime
	ParticipantIDs []reservationInt
	Participants   *string
	RepeatType     *string
	RepeatUntil    *reservationDateTime
}

func readReservationCreateInput(decoder *json.Decoder, now time.Time) (reservationCreateInput, error) {
	first, err := decoder.Token()
	if err != nil {
		return reservationCreateInput{}, err
	}
	if first == nil {
		if err := decoder.Decode(new(json.RawMessage)); err != nil {
			return reservationCreateInput{}, err
		}
	}
	if first != json.Delim('{') {
		return reservationCreateInput{}, errors.New("expected a JSON object")
	}

	// C# aliases share setters, so the last occurrence of a field or its alias wins.
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return reservationCreateInput{}, err
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return reservationCreateInput{}, err
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
				return reservationCreateInput{}, errors.New("expected a boolean isInternal")
			}
		}
		fields[field] = value
	}
	if _, err := decoder.Token(); err != nil {
		return reservationCreateInput{}, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err == nil {
		return reservationCreateInput{}, errors.New("expected one JSON object")
	} else if err != io.EOF {
		return reservationCreateInput{}, err
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return reservationCreateInput{}, err
	}
	input := reservationCreateInput{
		NumberOfPeople: 1, Type: new("社内"), Purpose: new(""), Date: reservationDateTime{timeOnDate(now, time.Time{})},
		ParticipantIDs: []reservationInt{}, Participants: new(""), RepeatType: new(noRepeat),
	}
	err = json.Unmarshal(raw, &input)
	return input, err
}

func (input reservationCreateInput) validate() map[string][]string {
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

func (input reservationCreateInput) normalize(user auth.User) (reservations.ListEntry, string) {
	date := input.Date.Time
	start := timeOnDate(date, input.StartTime.Time)
	end := timeOnDate(date, input.EndTime.Time)
	if !start.Before(end) {
		return reservations.ListEntry{}, "終了時刻は開始時刻より後にしてください。"
	}
	repeat := *input.RepeatType
	if strings.TrimSpace(repeat) == "" {
		repeat = noRepeat
	}
	if repeat != noRepeat && input.RepeatUntil != nil {
		return reservations.ListEntry{}, "繰り返し予約には対応していません。"
	}
	participants := make([]int, 0, len(input.ParticipantIDs)+1)
	seen := make(map[int]bool, len(input.ParticipantIDs))
	for _, id := range input.ParticipantIDs {
		if id > 0 && !seen[int(id)] {
			participants = append(participants, int(id))
			seen[int(id)] = true
		}
	}
	if user.ID > 0 && !seen[user.ID] {
		participants = append([]int{user.ID}, participants...)
	}
	return reservations.ListEntry{
		Reservation: reservations.Reservation{Room: *input.Room, Date: date, StartTime: start, EndTime: end},
		UserID:      user.ID, Name: user.Name, NumberOfPeople: int(input.NumberOfPeople), Type: *input.Type,
		Purpose: *input.Purpose, Participants: *input.Participants, ParticipantIDs: participants, RepeatType: repeat,
	}, ""
}

type reservationInt int32

func (value *reservationInt) UnmarshalJSON(data []byte) error {
	number := string(data)
	if strings.HasPrefix(number, `"`) {
		if err := json.Unmarshal(data, &number); err != nil {
			return err
		}
	}
	parsed, err := strconv.ParseInt(number, 10, 32)
	*value = reservationInt(parsed)
	return err
}

type reservationDateTime struct{ time.Time }

func (value *reservationDateTime) UnmarshalJSON(data []byte) error {
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
		if strings.HasSuffix(layout, "Z07:00") && !strings.HasSuffix(text, "Z") {
			parsed = parsed.In(time.Local)
		}
		if parsed.Year() < 1 || parsed.Year() > 9999 {
			break
		}
		value.Time = timeOnDate(parsed, parsed).Truncate(100 * time.Nanosecond)
		return nil
	}
	return errors.New("expected an ISO 8601 date or date-time")
}

// Store DateTime wall-clock fields without a zone, as in the reservation list.
func timeOnDate(date, clock time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), clock.Second(), clock.Nanosecond(), time.UTC)
}
