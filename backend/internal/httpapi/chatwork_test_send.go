package httpapi

import (
	"context"
	"net/http"
	"strings"
)

type chatworkTestResponse struct {
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func newChatworkTestHandler(defaultRoom string, send func(context.Context, string, string) error) http.HandlerFunc {
	defaultRoom = strings.TrimSpace(defaultRoom)
	return func(w http.ResponseWriter, r *http.Request) {
		request := &struct {
			Message *string
			RoomID  string
		}{Message: new("")}
		if !readJSONRequest(w, r, &request) {
			return
		}
		if request.Message == nil {
			writeValidationProblem(w, http.StatusBadRequest, map[string][]string{"Message": {"The Message field is required."}})
			return
		}
		if strings.TrimSpace(*request.Message) == "" {
			writeNegotiatedString(w, r, http.StatusBadRequest, "Message は必須です。")
			return
		}
		if send == nil {
			writeChatworkTestFailure(w, http.StatusServiceUnavailable, "Chatwork delivery is disabled.")
			return
		}
		room := strings.TrimSpace(request.RoomID)
		if room == "" {
			room = defaultRoom
		}
		if room == "" {
			writeChatworkTestFailure(w, http.StatusInternalServerError, "Chatwork default room ID is not configured.")
			return
		}
		if err := send(r.Context(), room, *request.Message); err != nil {
			writeChatworkTestFailure(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, chatworkTestResponse{Message: "Chatworkへのテスト送信に成功しました。"})
	}
}

func writeChatworkTestFailure(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, chatworkTestResponse{Message: "Chatworkへのテスト送信に失敗しました。", Detail: detail})
}
