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
const deliveryTimeout = 10 * time.Second

type Sender struct {
	pool    *pgxpool.Pool
	client  *http.Client
	baseURL string
	token   string
}

func NewSender(pool *pgxpool.Pool, token, baseURL string, development bool) (*Sender, error) {
	if pool == nil {
		return nil, errors.New("Chatwork delivery requires DATABASE_URL")
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("Chatwork delivery requires a valid CHATWORK_API_TOKEN")
	}
	if baseURL == "" {
		baseURL = "https://api.chatwork.com/v2/"
	} else {
		endpoint, err := url.Parse(baseURL)
		if err != nil || !development || !isLoopbackEndpoint(endpoint) {
			return nil, errors.New("CHATWORK_API_BASE_URL must be a loopback HTTP or HTTPS URL in development")
		}
	}
	return &Sender{
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

// SendCancellation sends DMs for committed deletions.
func (sender *Sender) SendCancellation(ctx context.Context, deleted []reservations.ListEntry) {
	sender.send(ctx, cancellationType, deleted, nil)
}

func (sender *Sender) send(ctx context.Context, deliveryType string, entries, overlaps []reservations.ListEntry) {
	if len(entries) == 0 {
		return
	}
	ctx, cancel := deliveryContext(ctx)
	defer cancel()
	ordered := slices.Clone(entries)
	slices.SortFunc(ordered, func(a, b reservations.ListEntry) int {
		if order := a.Date.Compare(b.Date); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
	first := ordered[0]
	participants := reservationParticipants(ordered)
	stakeholders := append(slices.Clone(participants), first.UserID)
	recipients, err := sender.loadRecipients(ctx, stakeholders)
	if err != nil {
		log.Printf("Chatwork %s: could not load recipients for reservation %d", deliveryType, first.ID)
		return
	}
	var title, action string
	var extra []string
	if deliveryType == creationType {
		first.ParticipantIDs = participants
		conflicts, err := reservations.FindParticipantConflicts(ctx, sender.pool, first)
		if err != nil {
			log.Printf("Chatwork %s: could not load appointment conflicts for reservation %d", deliveryType, first.ID)
			return
		}
		title, action, extra = creationDetails(len(ordered), overlaps, conflicts)
	} else {
		title, action, extra = cancellationDetails(len(ordered))
	}
	message := stakeholderMessage(first, participants, stakeholders, recipients, title, action, extra)
	sender.sendToStakeholders(ctx, deliveryType, "", first, stakeholders, recipients, message)
}

func deliveryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	// Finish a bounded delivery even if the caller disconnects after commit.
	return context.WithTimeout(context.WithoutCancel(ctx), deliveryTimeout)
}

func (sender *Sender) sendToStakeholders(ctx context.Context, deliveryType, keySuffix string, entry reservations.ListEntry,
	stakeholders []int, recipients map[int]recipient, message string,
) {
	for _, id := range stakeholders {
		if ctx.Err() != nil {
			log.Printf("Chatwork %s: delivery budget expired for reservation %d", deliveryType, entry.ID)
			return
		}
		if target, found := recipients[id]; found {
			key := fmt.Sprintf("%s:reservation:%d:user:%d%s", deliveryType, entry.ID, id, keySuffix)
			sender.deliver(ctx, deliveryType, key, entry, id, target.room, message)
		}
	}
}

func reservationParticipants(entries []reservations.ListEntry) []int {
	var participants []int
	seen := make(map[int]bool)
	organizer := entries[0].UserID
	add := func(id int) {
		if id > 0 && id != organizer && !seen[id] {
			participants = append(participants, id)
			seen[id] = true
		}
	}
	for _, entry := range entries {
		for _, id := range entry.ParticipantIDs {
			add(id)
		}
		add(entry.UserID)
	}
	return participants
}

func (sender *Sender) loadRecipients(ctx context.Context, ids []int) (map[int]recipient, error) {
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

func stakeholderMessage(first reservations.ListEntry, participants, stakeholders []int, recipients map[int]recipient,
	title, action string, extra []string,
) string {
	var targetNames []string
	seenNames := make(map[string]bool)
	for _, id := range stakeholders {
		name := recipients[id].name
		if strings.TrimSpace(name) != "" && !seenNames[name] {
			targetNames = append(targetNames, name)
			seenNames[name] = true
		}
	}
	targets := "関係者"
	if len(targetNames) > 0 {
		targets = strings.Join(targetNames, "、")
	}
	participantList := participantDisplayNames(participants, recipients)
	if participantList == "" {
		participantList = "なし"
	}
	organizer := first.Name
	if target, found := recipients[first.UserID]; found {
		organizer = target.name
	}
	lines := []string{
		"[info][title]" + title + "[/title]", "対象: " + targets, "対応: " + action, "[hr]",
		"目的: " + reservationLabel(first), "利用日: " + first.Date.Format("2006/01/02"), "会議室: " + first.Room,
		"時間: " + reservationTimeRange(first),
		"予約者: " + organizer, "参加者: " + participantList,
	}
	if len(extra) > 0 {
		lines = append(lines, "[hr]")
		lines = append(lines, extra...)
	}
	return strings.Join(append(lines, "[/info]"), "\n")
}

func participantDisplayNames(ids []int, recipients map[int]recipient) string {
	var names []string
	for _, id := range ids {
		target, found := recipients[id]
		name := target.name
		if !found {
			name = fmt.Sprintf("ユーザー%d", id)
		}
		if strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, "、")
}

func cancellationDetails(count int) (title, action string, extra []string) {
	const heading = "会議予約がキャンセルされました"
	if count <= 1 {
		return heading, "この予約は無効です", nil
	}
	return heading, "繰り返し予約がまとめて削除されました", []string{fmt.Sprintf("削除件数: %d件", count)}
}

func reservationLabel(entry reservations.ListEntry) string {
	if strings.TrimSpace(entry.Purpose) == "" {
		return entry.Name
	}
	return entry.Purpose
}

func reservationTimeRange(entry reservations.ListEntry) string {
	return entry.StartTime.Format("15:04") + "〜" + entry.EndTime.Format("15:04")
}

func (sender *Sender) deliver(ctx context.Context, deliveryType, deliveryKey string, first reservations.ListEntry, userID int, room, message string) {
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
		first.ID, deliveryType, deliveryKey, userID, dotnettime.Ticks(first.StartTime), room, status, reason,
		localTicks(), message).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("Chatwork: could not claim delivery %s", deliveryKey)
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
		log.Printf("Chatwork: could not record result for %s", deliveryKey)
	}
}

func localTicks() int64 {
	now := time.Now()
	return dotnettime.Ticks(dotnettime.OnDate(now, now))
}
