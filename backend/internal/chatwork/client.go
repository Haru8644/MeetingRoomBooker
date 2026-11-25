package chatwork

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SendTestMessage sends one explicit test without claiming a delivery log row.
func (sender *Sender) SendTestMessage(ctx context.Context, room, message string) error {
	status, reason := sender.postMessage(ctx, room, message)
	if status == "Succeeded" {
		return nil
	}
	return errors.New(reason)
}

func (sender *Sender) postMessage(ctx context.Context, room, message string) (status, reason string) {
	form := url.Values{"body": {message}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		sender.baseURL+"/rooms/"+url.PathEscape(room)+"/messages", strings.NewReader(form.Encode()))
	if err != nil {
		return "Failed", "Could not construct the Chatwork request."
	}
	request.GetBody = nil // Keep each send to one attempt, including manual test messages.
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-ChatWorkToken", sender.token)
	response, err := sender.client.Do(request)
	if err != nil {
		return unknownResult(err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "Failed", fmt.Sprintf("Failed to send Chatwork message. StatusCode: %d.", response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10)); err != nil {
		return unknownResult(err)
	}
	return "Succeeded", ""
}

func unknownResult(err error) (string, string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return "Unknown", "Chatwork request timed out."
	}
	return "Unknown", "Chatwork response was not confirmed."
}
