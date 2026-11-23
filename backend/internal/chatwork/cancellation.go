package chatwork

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Haru8644/MeetingRoomBooker/backend/internal/dotnettime"
	"github.com/Haru8644/MeetingRoomBooker/backend/internal/reservations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const cancellationType = "ReservationCanceled"

type CancellationSender struct {
	pool    *pgxpool.Pool
	client  *http.Client
	baseURL string
	token   string
}

func NewCancellationSender(pool *pgxpool.Pool, token, baseURL string, development bool) (*CancellationSender, error) {
	if pool == nil {
		return nil, errors.New("Chatwork cancellation requires DATABASE_URL")
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("Chatwork cancellation requires a valid CHATWORK_API_TOKEN")
	}
	if baseURL == "" {
		baseURL = "https://api.chatwork.com/v2/"
	} else {
		endpoint, err := url.Parse(baseURL)
		if err != nil || !development || !isLoopbackEndpoint(endpoint) {
			return nil, errors.New("CHATWORK_API_BASE_URL must be a loopback HTTP or HTTPS URL in development")
		}
	}
	return &CancellationSender{
		pool: pool, token: token, baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout:       3 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func isLoopbackEndpoint(endpoint *url.URL) bool {
	host := endpoint.Hostname()
	return (endpoint.Scheme == "http" || endpoint.Scheme == "https") &&
		(host == "localhost" || net.ParseIP(host).IsLoopback()) &&
		endpoint.User == nil && !endpoint.ForceQuery && endpoint.RawQuery == "" && endpoint.Fragment == ""
}

type recipient struct {
	name string
	room string
}

// Send sends cancellation DMs for committed deletions. Failures are logged
// independently of the reservation response.
func (sender *CancellationSender) Send(ctx context.Context, deleted []reservations.ListEntry) {
	if len(deleted) == 0 {
		return
	}
	// Finish a bounded delivery even if the caller disconnects after commit.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	ordered := slices.Clone(deleted)
	slices.SortFunc(ordered, func(a, b reservations.ListEntry) int {
		if order := a.Date.Compare(b.Date); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
	first := ordered[0]
	participants := canceledParticipants(ordered)
	stakeholders := append(slices.Clone(participants), first.UserID)
	recipients, err := sender.loadRecipients(ctx, stakeholders)
	if err != nil {
		log.Printf("Chatwork cancellation: could not load recipients for reservation %d", first.ID)
		return
	}
	message := cancellationMessage(first, len(ordered), participants, stakeholders, recipients)
	for _, id := range stakeholders {
		if ctx.Err() != nil {
			log.Printf("Chatwork cancellation: delivery budget expired for reservation %d", first.ID)
			return
		}
		if target, found := recipients[id]; found {
			sender.deliver(ctx, first, id, target.room, message)
		}
	}
}

func canceledParticipants(deleted []reservations.ListEntry) []int {
	var participants []int
	seen := make(map[int]bool)
	organizer := deleted[0].UserID
	add := func(id int) {
		if id > 0 && id != organizer && !seen[id] {
			participants = append(participants, id)
			seen[id] = true
		}
	}
	for _, entry := range deleted {
		for _, id := range entry.ParticipantIDs {
			add(id)
		}
		add(entry.UserID)
	}
	return participants
}

func (sender *CancellationSender) loadRecipients(ctx context.Context, ids []int) (map[int]recipient, error) {
	rows, err := sender.pool.Query(ctx, `
		SELECT id, name, COALESCE(chatwork_direct_room_id, '') FROM users
		WHERE id = ANY($1::integer[]) AND id > 0`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int]recipient)
	for rows.Next() {
		var id int
		var target recipient
		if err := rows.Scan(&id, &target.name, &target.room); err != nil {
			return nil, err
		}
		target.room = strings.TrimSpace(target.room)
		result[id] = target
	}
	return result, rows.Err()
}

func cancellationMessage(first reservations.ListEntry, count int, participants, stakeholders []int, recipients map[int]recipient) string {
	var targetNames, participantNames []string
	seenNames := make(map[string]bool)
	for _, id := range stakeholders {
		name := recipients[id].name
		if strings.TrimSpace(name) != "" && !seenNames[name] {
			targetNames = append(targetNames, name)
			seenNames[name] = true
		}
	}
	for _, id := range participants {
		target, found := recipients[id]
		name := target.name
		if !found {
			name = fmt.Sprintf("ユーザー%d", id)
		}
		if strings.TrimSpace(name) != "" {
			participantNames = append(participantNames, name)
		}
	}
	targets := "関係者"
	if len(targetNames) > 0 {
		targets = strings.Join(targetNames, "、")
	}
	participantList := "なし"
	if len(participantNames) > 0 {
		participantList = strings.Join(participantNames, "、")
	}
	organizer := first.Name
	if target, found := recipients[first.UserID]; found {
		organizer = target.name
	}
	purpose := first.Purpose
	if strings.TrimSpace(purpose) == "" {
		purpose = first.Name
	}
	action := "この予約は無効です"
	if count > 1 {
		action = "繰り返し予約がまとめて削除されました"
	}
	lines := []string{
		"[info][title]会議予約がキャンセルされました[/title]", "対象: " + targets, "対応: " + action, "[hr]",
		"目的: " + purpose, "利用日: " + first.Date.Format("2006/01/02"), "会議室: " + first.Room,
		"時間: " + first.StartTime.Format("15:04") + "〜" + first.EndTime.Format("15:04"),
		"予約者: " + organizer, "参加者: " + participantList,
	}
	if count > 1 {
		lines = append(lines, "[hr]", fmt.Sprintf("削除件数: %d件", count))
	}
	return strings.Join(append(lines, "[/info]"), "\n")
}

func (sender *CancellationSender) deliver(ctx context.Context, first reservations.ListEntry, userID int, room, message string) {
	deliveryKey := fmt.Sprintf("%s:reservation:%d:user:%d", cancellationType, first.ID, userID)
	status, reason := "Unknown", "Delivery claimed. Result not recorded."
	if room == "" {
		status, reason = "Skipped", "ChatworkDirectRoomId is not configured."
	}
	var id int
	err := sender.pool.QueryRow(ctx, `
		INSERT INTO chatwork_delivery_logs (reservation_id, delivery_type, delivery_key, target_user_id,
		    scheduled_start_time_ticks, room_id, status, error_message, attempted_at_ticks, message, created_at_ticks)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $9)
		ON CONFLICT (delivery_key) DO NOTHING RETURNING id`,
		first.ID, cancellationType, deliveryKey, userID, dotnettime.Ticks(first.StartTime), room, status, reason,
		localTicks(), message).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("Chatwork cancellation: could not claim delivery %s", deliveryKey)
		return
	}
	if status == "Skipped" {
		return
	}

	status, reason = sender.postMessage(ctx, room, message)
	var sentAt *int64
	if status == "Succeeded" {
		sentAt = new(localTicks())
	}
	if _, err := sender.pool.Exec(ctx, `
		UPDATE chatwork_delivery_logs SET status = $2, error_message = NULLIF($3, ''), sent_at_ticks = $4
		WHERE id = $1`, id, status, reason, sentAt); err != nil {
		log.Printf("Chatwork cancellation: could not record result for %s", deliveryKey)
	}
}

func localTicks() int64 {
	now := time.Now()
	return dotnettime.Ticks(dotnettime.OnDate(now, now))
}
