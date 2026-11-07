package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Backline handoff contract, version 1 (issue #221). The producer side lives
// here; docs/development/backline-handoff.md and
// contracts/backline-handoff.v1.schema.json are the written contract.

const (
	backlineHandoffVersion        = 1
	backlineHandoffKind           = "subcult-os.announcement-handoff"
	backlineReceiptKind           = "subcult-os.announcement-receipt"
	backlineTimestampHeader       = "X-Subcult-Handoff-Timestamp"
	backlineSignatureHeader       = "X-Subcult-Handoff-Signature"
	backlineSignaturePrefix       = "v1="
	backlineReplayWindow          = 5 * time.Minute
	backlineDeliveryWindow        = 24 * time.Hour
	backlineMaxContentRunes       = 1800
	backlineMaxAttempts           = 5
	backlineRequestTimeout        = 15 * time.Second
	backlineLease                 = 2 * time.Minute
	backlineMaxResponseBytes      = 64 << 10
	backlineMentionPolicyNone     = "none"
	channelBacklineName           = "backline"
	handoffOperationPublish       = "publish"
	handoffOperationCancel        = "cancel"
	handoffStatusPending          = "pending"
	handoffStatusSending          = "sending"
	handoffStatusAccepted         = "accepted"
	handoffStatusRejected         = "rejected"
	handoffStatusUncertain        = "uncertain"
	handoffStatusFailed           = "failed"
	handoffStatusCancelled        = "cancelled"
	handoffStatusExpired          = "expired"
	backlineSignatureScopeHandoff = "handoff"
	backlineSignatureScopeReceipt = "receipt"
)

var (
	backlineReceiverCode = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	discordSnowflake     = regexp.MustCompile(`^[0-9]{17,20}$`)
)

// backlineHandoffConfigured reports whether the opt-in adapter has both a
// receiver URL and a signing secret. Nothing is sent or accepted otherwise.
func (c Config) backlineHandoffConfigured() bool {
	return strings.TrimSpace(c.BacklineHandoffURL) != "" && len(c.BacklineHandoffSecret) >= 32
}

// backlineOccurrenceKey is the stable idempotency key for one announcement
// occurrence at one binding. Publish, retry and cancel all reuse it.
func backlineOccurrenceKey(announcementID, bindingID string) string {
	return "subcult-os/announcement/" + announcementID + "/binding/" + bindingID
}

// announcementContentRevision digests exactly the content the receiver gets:
// the UTF-8 subject, one NUL byte (PostgreSQL text cannot contain it), then
// the body.
func announcementContentRevision(subject, body string) string {
	digest := sha256.Sum256([]byte(subject + "\x00" + body))
	return "sha256:" + hex.EncodeToString(digest[:])
}

// announcementChannelContentLength counts the runes the receiver renders:
// the subject line, a blank line, then the body.
func announcementChannelContentLength(subject, body string) int {
	return utf8.RuneCountInString(subject) + 2 + utf8.RuneCountInString(body)
}

type backlineBindingRef struct {
	ID        string `json:"id"`
	ServerID  string `json:"serverId"`
	ChannelID string `json:"channelId"`
}

type backlineContent struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// backlineHandoffPayload is the complete request body. It carries content and
// identifiers only: no recipient, consent, membership or email data.
type backlineHandoffPayload struct {
	Version         int                `json:"version"`
	Kind            string             `json:"kind"`
	Operation       string             `json:"operation"`
	OccurrenceKey   string             `json:"occurrenceKey"`
	ContentRevision string             `json:"contentRevision"`
	Binding         backlineBindingRef `json:"binding"`
	DeliverAt       string             `json:"deliverAt"`
	ExpiresAt       string             `json:"expiresAt"`
	MentionPolicy   string             `json:"mentionPolicy"`
	Content         *backlineContent   `json:"content,omitempty"`
	SentAt          string             `json:"sentAt"`
}

type backlineAck struct {
	Version           int    `json:"version"`
	OccurrenceKey     string `json:"occurrenceKey"`
	Status            string `json:"status"`
	ReceiverReference string `json:"receiverReference,omitempty"`
}

type backlineReceipt struct {
	Version           int    `json:"version"`
	Kind              string `json:"kind"`
	OccurrenceKey     string `json:"occurrenceKey"`
	ContentRevision   string `json:"contentRevision"`
	Outcome           string `json:"outcome"`
	ReceiverReference string `json:"receiverReference,omitempty"`
}

// backlineSignature signs scope, timestamp and the exact raw body. The scope
// stops a captured handoff request from being replayed as a receipt.
func backlineSignature(secret, scope string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v1." + scope + "." + strconv.FormatInt(timestamp, 10) + "."))
	mac.Write(body)
	return backlineSignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

var errBacklineSignature = errors.New("invalid backline signature")

// verifyBacklineSignature checks the timestamp window and the HMAC in
// constant time.
func verifyBacklineSignature(secret, scope string, header http.Header, body []byte, now time.Time) error {
	timestamp, err := strconv.ParseInt(strings.TrimSpace(header.Get(backlineTimestampHeader)), 10, 64)
	if err != nil {
		return errBacklineSignature
	}
	skew := now.Sub(time.Unix(timestamp, 0))
	if skew > backlineReplayWindow || skew < -backlineReplayWindow {
		return errBacklineSignature
	}
	want := backlineSignature(secret, scope, timestamp, body)
	if !hmac.Equal([]byte(strings.TrimSpace(header.Get(backlineSignatureHeader))), []byte(want)) {
		return errBacklineSignature
	}
	return nil
}

// BacklineHandoffReport is the secret-free aggregate output of one worker
// batch: counts only, never content or identifiers.
type BacklineHandoffReport struct {
	Claimed   int `json:"claimed"`
	Accepted  int `json:"accepted"`
	Rejected  int `json:"rejected"`
	Retrying  int `json:"retrying"`
	Uncertain int `json:"uncertain"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
	Expired   int `json:"expired"`
}

// RunBacklineHandoffs delivers due pending handoffs to the configured
// receiver. It is opt-in: without BACKLINE_HANDOFF_URL and
// BACKLINE_HANDOFF_SECRET it refuses to run.
func RunBacklineHandoffs(ctx context.Context, config Config, db *pgxpool.Pool, limit int, client *http.Client) (BacklineHandoffReport, error) {
	report := BacklineHandoffReport{}
	if db == nil {
		return report, errors.New("backline handoff requires a database")
	}
	if limit < 1 || limit > 100 {
		return report, errors.New("backline handoff batch limit must be 1..100")
	}
	if !config.backlineHandoffConfigured() {
		return report, errors.New("backline handoff requires BACKLINE_HANDOFF_URL and BACKLINE_HANDOFF_SECRET")
	}
	if version, err := CurrentSchemaVersion(ctx, db); err != nil || version != minimumSchemaVersion {
		return report, errors.New("backline handoff requires current migrations")
	}
	if client == nil {
		client = &http.Client{Timeout: backlineRequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	// A send whose worker vanished may or may not have reached the receiver.
	if _, err := db.Exec(ctx, `
		update announcement_channel_handoffs
		set status = $1, last_error = 'lease_expired', lease_expires_at = null, updated_at = now()
		where status = $2 and lease_expires_at < now()
	`, handoffStatusUncertain, handoffStatusSending); err != nil {
		return report, errors.New("backline lease sweep failed")
	}
	expired, err := db.Exec(ctx, `
		update announcement_channel_handoffs
		set status = $1, updated_at = now()
		where status = $2 and operation = $3 and expires_at <= now()
	`, handoffStatusExpired, handoffStatusPending, handoffOperationPublish)
	if err != nil {
		return report, errors.New("backline expiry sweep failed")
	}
	report.Expired = int(expired.RowsAffected())

	for range limit {
		claimed, outcome, err := deliverOneBacklineHandoff(ctx, config, db, client)
		if err != nil {
			return report, err
		}
		if !claimed {
			break
		}
		report.Claimed++
		switch outcome {
		case handoffStatusAccepted:
			report.Accepted++
		case handoffStatusRejected:
			report.Rejected++
		case handoffStatusPending:
			report.Retrying++
		case handoffStatusUncertain:
			report.Uncertain++
		case handoffStatusFailed:
			report.Failed++
		case handoffStatusCancelled:
			report.Cancelled++
		}
	}
	return report, nil
}

type claimedHandoff struct {
	id, operation, occurrenceKey, contentRevision          string
	bindingID, serverID, channelID, mentionPolicy, subject string
	body, announcementStatus                               string
	deliverAt, expiresAt                                   time.Time
	attempt                                                int
	bindingRevoked                                         bool
}

func deliverOneBacklineHandoff(ctx context.Context, config Config, db *pgxpool.Pool, client *http.Client) (bool, string, error) {
	var h claimedHandoff
	err := db.QueryRow(ctx, `
		with candidate as (
			select id from announcement_channel_handoffs
			where status = $1 and next_attempt_at <= now()
			order by next_attempt_at
			limit 1
			for update skip locked
		)
		update announcement_channel_handoffs h
		set status = $2, attempt_count = h.attempt_count + 1,
		    lease_expires_at = now() + make_interval(secs => $3), updated_at = now()
		from candidate, announcements a, announcement_channel_bindings b
		where h.id = candidate.id and a.id = h.announcement_id and b.id = h.binding_id
		returning h.id::text, h.operation, h.occurrence_key, h.content_revision, h.deliver_at, h.expires_at,
		          h.attempt_count, b.id::text, b.external_server_id, b.external_channel_id, b.mention_policy,
		          b.revoked_at is not null, a.subject, a.body, a.status
	`, handoffStatusPending, handoffStatusSending, backlineLease.Seconds()).Scan(
		&h.id, &h.operation, &h.occurrenceKey, &h.contentRevision, &h.deliverAt, &h.expiresAt,
		&h.attempt, &h.bindingID, &h.serverID, &h.channelID, &h.mentionPolicy,
		&h.bindingRevoked, &h.subject, &h.body, &h.announcementStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", errors.New("backline handoff claim failed")
	}

	if h.operation == handoffOperationPublish {
		// Defensive recheck: revocation and cancellation normally convert the
		// row before a worker sees it.
		if h.bindingRevoked || h.announcementStatus == announcementStatusCancelled || h.announcementStatus == announcementStatusDraft {
			return true, handoffStatusCancelled, recordHandoffOutcome(ctx, db, h, handoffStatusCancelled, "", "", "publish_withdrawn_before_send")
		}
		if announcementContentRevision(h.subject, h.body) != h.contentRevision {
			return true, handoffStatusFailed, recordHandoffOutcome(ctx, db, h, handoffStatusFailed, "", "", "content_revision_changed")
		}
	}

	payload := backlineHandoffPayload{
		Version:         backlineHandoffVersion,
		Kind:            backlineHandoffKind,
		Operation:       h.operation,
		OccurrenceKey:   h.occurrenceKey,
		ContentRevision: h.contentRevision,
		Binding:         backlineBindingRef{ID: h.bindingID, ServerID: h.serverID, ChannelID: h.channelID},
		DeliverAt:       h.deliverAt.UTC().Format(time.RFC3339),
		ExpiresAt:       h.expiresAt.UTC().Format(time.RFC3339),
		MentionPolicy:   h.mentionPolicy,
		SentAt:          time.Now().UTC().Format(time.RFC3339),
	}
	if h.operation == handoffOperationPublish {
		payload.Content = &backlineContent{Subject: h.subject, Body: h.body}
	}
	status, receiverStatus, reference, code := sendBacklineHandoff(ctx, config, client, payload)
	if status == handoffStatusPending && h.attempt >= backlineMaxAttempts {
		status = handoffStatusFailed
	}
	return true, status, recordHandoffOutcome(ctx, db, h, status, receiverStatus, reference, code)
}

// sendBacklineHandoff performs one signed POST and classifies the result:
// accepted, rejected (definitive), pending (receiver said it did not process
// the request, or the connection never opened) or uncertain (the request may
// have been processed; hold for inspection rather than resend blindly).
func sendBacklineHandoff(ctx context.Context, config Config, client *http.Client, payload backlineHandoffPayload) (status, receiverStatus, reference, code string) {
	body, err := json.Marshal(payload)
	if err != nil {
		return handoffStatusFailed, "", "", "payload_encoding_failed"
	}
	timestamp := time.Now().Unix()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.BacklineHandoffURL, bytes.NewReader(body))
	if err != nil {
		return handoffStatusFailed, "", "", "receiver_url_invalid"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", payload.OccurrenceKey)
	req.Header.Set(backlineTimestampHeader, strconv.FormatInt(timestamp, 10))
	req.Header.Set(backlineSignatureHeader, backlineSignature(config.BacklineHandoffSecret, backlineSignatureScopeHandoff, timestamp, body))
	resp, err := client.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return handoffStatusPending, "", "", "receiver_unreachable"
		}
		return handoffStatusUncertain, "", "", "receiver_no_response"
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, backlineMaxResponseBytes))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
		return handoffStatusPending, "", "", "receiver_http_" + strconv.Itoa(resp.StatusCode)
	case resp.StatusCode >= 500:
		return handoffStatusUncertain, "", "", "receiver_http_" + strconv.Itoa(resp.StatusCode)
	case resp.StatusCode >= 300:
		var rejected struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &rejected) == nil && backlineReceiverCode.MatchString(rejected.Error) {
			return handoffStatusRejected, "", "", rejected.Error
		}
		return handoffStatusRejected, "", "", "receiver_http_" + strconv.Itoa(resp.StatusCode)
	}
	if readErr != nil {
		return handoffStatusUncertain, "", "", "receiver_ack_unreadable"
	}
	var ack backlineAck
	if json.Unmarshal(raw, &ack) != nil || ack.Version != backlineHandoffVersion || ack.OccurrenceKey != payload.OccurrenceKey {
		return handoffStatusUncertain, "", "", "receiver_ack_invalid"
	}
	allowed := map[string][]string{
		handoffOperationPublish: {"accepted", "duplicate"},
		handoffOperationCancel:  {"cancelled", "not_found", "already_delivered"},
	}[payload.Operation]
	for _, value := range allowed {
		if ack.Status == value {
			if len(ack.ReceiverReference) > 128 {
				ack.ReceiverReference = ""
			}
			return handoffStatusAccepted, ack.Status, ack.ReceiverReference, ""
		}
	}
	return handoffStatusUncertain, "", "", "receiver_ack_invalid"
}

func backlineRetryDelay(attempt int) time.Duration {
	delay := 30 * time.Second
	for i := 1; i < attempt && delay < 30*time.Minute; i++ {
		delay *= 2
	}
	return min(delay, 30*time.Minute)
}

// recordHandoffOutcome writes the result only while the row is still the
// operation this worker claimed. A cancellation that switched the row to a
// cancel message mid-flight wins.
func recordHandoffOutcome(ctx context.Context, db *pgxpool.Pool, h claimedHandoff, status, receiverStatus, reference, code string) error {
	_, err := db.Exec(ctx, `
		update announcement_channel_handoffs
		set status = $4,
		    receiver_status = coalesce(nullif($5, ''), receiver_status),
		    receiver_reference = coalesce(nullif($6, ''), receiver_reference),
		    last_error = nullif($7, ''),
		    acknowledged_at = case when $4 = 'accepted' then now() else acknowledged_at end,
		    next_attempt_at = now() + make_interval(secs => $8),
		    lease_expires_at = null,
		    updated_at = now()
		where id = $1 and status = $2 and operation = $3 and attempt_count = $9
	`, h.id, handoffStatusSending, h.operation, status, receiverStatus, reference, code, backlineRetryDelay(h.attempt).Seconds(), h.attempt)
	if err != nil {
		return errors.New("backline handoff outcome write failed")
	}
	return nil
}

// requestHandoffCancellation converts the matched handoffs in one
// transaction: a publish the receiver never saw is cancelled locally; one it
// may have seen becomes a cancel message with the same occurrence key.
func requestHandoffCancellation(ctx context.Context, tx pgx.Tx, filter string, arg string) error {
	if _, err := tx.Exec(ctx, `
		update announcement_channel_handoffs
		set status = $1, cancel_requested_at = coalesce(cancel_requested_at, now()), lease_expires_at = null, updated_at = now()
		where `+filter+` = $3 and operation = $2 and status = 'pending'
	`, handoffStatusCancelled, handoffOperationPublish, arg); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		update announcement_channel_handoffs
		set operation = $1, status = 'pending', attempt_count = 0, next_attempt_at = now(), last_error = null,
		    lease_expires_at = null, cancel_requested_at = coalesce(cancel_requested_at, now()), updated_at = now()
		where `+filter+` = $3 and operation = $2 and status in ('sending', 'accepted', 'uncertain', 'failed')
	`, handoffOperationCancel, handoffOperationPublish, arg)
	return err
}
