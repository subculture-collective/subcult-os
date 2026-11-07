package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// listmonk announcement relay (issue #220). See
// docs/development/listmonk-relay.md. Each recipient of a listmonk occurrence
// is one transactional message (POST /api/tx) to a subscriber of the bound
// list; the relay never starts a list campaign, so listmonk only ever mails
// addresses Subcult OS authorized for this occurrence.

const (
	channelListmonkName      = "listmonk"
	deliveryPathNative       = "native"
	deliveryPathListmonk     = "listmonk"
	consentSourceKorkReceipt = "kork_receipt"
	korkReceiptVersion       = 1
	korkReceiptMaxAge        = 180 * 24 * time.Hour
	korkReceiptClockSkew     = 5 * time.Minute
	listmonkMaxAttempts      = 5
	listmonkRequestTimeout   = 15 * time.Second
	listmonkLease            = 2 * time.Minute
	listmonkDeliveryWindow   = 23 * time.Hour
	listmonkMaxResponseBytes = 256 << 10
	listmonkStatusHeld       = "held"
	listmonkStatusPending    = "pending"
	listmonkStatusSending    = "sending"
	listmonkStatusAccepted   = "accepted"
	listmonkStatusRejected   = "rejected"
	listmonkStatusUncertain  = "uncertain"
	listmonkStatusFailed     = "failed"
	listmonkStatusWithheld   = "withheld"
	listmonkStatusSuppressed = "suppressed"
	listmonkStatusExpired    = "expired"
)

var listmonkAPIUserPattern = regexp.MustCompile(`^[A-Za-z0-9._@+-]{1,100}$`)

// listmonkRelayConfigured reports whether the opt-in adapter has a URL, API
// user and token. Nothing is sent to listmonk otherwise.
func (c Config) listmonkRelayConfigured() bool {
	return strings.TrimSpace(c.ListmonkURL) != "" && c.ListmonkAPIUser != "" && c.ListmonkAPIToken != ""
}

// listmonkBinding is the relay's view of one listmonk channel binding.
type listmonkBinding struct {
	ID, Label, SenderAddress, DisclosureVersion string
	ListID, TemplateID                          int64
	KorkListURL, KorkSender                     *string
	KorkPurpose, KorkDisclosure                 *string
	CreatedAt                                   time.Time
	RevokedAt                                   *time.Time
}

const listmonkBindingColumns = `id::text, label, sender_address, disclosure_version, listmonk_list_id, listmonk_template_id,
	kork_list_url, kork_sender, kork_purpose, kork_disclosure, created_at, revoked_at`

func scanListmonkBinding(row pgx.Row) (listmonkBinding, error) {
	var b listmonkBinding
	err := row.Scan(&b.ID, &b.Label, &b.SenderAddress, &b.DisclosureVersion, &b.ListID, &b.TemplateID,
		&b.KorkListURL, &b.KorkSender, &b.KorkPurpose, &b.KorkDisclosure, &b.CreatedAt, &b.RevokedAt)
	return b, err
}

// korkReceipt is Kork's version-1 sign-up consent receipt
// (kork internal/server/signup.go signupConsent, stored in
// signup_contacts.consent). Kork does not sign it.
type korkReceipt struct {
	Version    int       `json:"v"`
	List       string    `json:"list"`
	Sender     string    `json:"sender"`
	Page       string    `json:"page"`
	Purpose    string    `json:"purpose"`
	Disclosure string    `json:"disclosure"`
	At         time.Time `json:"at"`
}

// korkListKey is the "list" value Kork writes for a listmonk destination:
// "listmonk:" + the block's listmonk URL (lowercased, no trailing slash) +
// "#" + the list id (kork internal/blocks/newblocks.go Signup.ListKey).
func korkListKey(listURL string, listID int64) string {
	return "listmonk:" + listURL + "#" + strconv.FormatInt(listID, 10)
}

// validateKorkReceipt decides whether a Kork receipt explicitly authorizes
// this binding: the exact list, sender, purpose and disclosure the binding's
// receiver policy names, a known version and a recent, plausible time. It
// returns an error code, or "" when the receipt counts. Any other receipt
// needs fresh confirmation through the native consent flow.
func validateKorkReceipt(binding listmonkBinding, raw json.RawMessage, now time.Time) (korkReceipt, string) {
	var receipt korkReceipt
	if binding.KorkListURL == nil || binding.KorkSender == nil || binding.KorkPurpose == nil || binding.KorkDisclosure == nil {
		return receipt, "binding_accepts_no_kork_receipts"
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || decoder.More() || receipt.Version != korkReceiptVersion || receipt.At.IsZero() {
		return receipt, "receipt_invalid"
	}
	switch {
	case receipt.List != korkListKey(*binding.KorkListURL, binding.ListID):
		return receipt, "receipt_wrong_list"
	case receipt.Sender != *binding.KorkSender:
		return receipt, "receipt_wrong_sender"
	case receipt.Purpose != *binding.KorkPurpose:
		return receipt, "receipt_wrong_purpose"
	case receipt.Disclosure != *binding.KorkDisclosure:
		return receipt, "receipt_wrong_disclosure"
	case receipt.At.After(now.Add(korkReceiptClockSkew)), now.Sub(receipt.At) > korkReceiptMaxAge:
		return receipt, "receipt_expired"
	}
	return receipt, ""
}

// listmonkGrantPolicy is the SQL predicate for a grant row g that a listmonk
// binding b accepts: a native grant confirmed under the binding's disclosure
// version, or a Kork-origin grant this binding's receiver policy accepted.
const listmonkGrantPolicy = `g.channel = 'email' and g.purpose = 'announcement'
	and g.verified_at is not null and g.withdrawn_at is null
	and ((g.listmonk_binding_id is null and g.disclosure_version = b.disclosure_version) or g.listmonk_binding_id = b.id)`

// checkListmonkGrant is the listmonk path's final consent gate, run at
// dispatch and again immediately before each /api/tx call: the grant must
// still be verified, unwithdrawn and accepted by the active binding, and the
// address must not be suppressed. Membership, tickets and contacts are never
// consulted.
func checkListmonkGrant(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, grantID, bindingID, recipient string) error {
	var suppressed bool
	if err := db.QueryRow(ctx, `select exists(select 1 from email_suppressions where recipient_email = $1)`, normalizeEmail(recipient)).Scan(&suppressed); err != nil {
		return err
	}
	if suppressed {
		return ErrConsentSuppressed
	}
	var granted bool
	if err := db.QueryRow(ctx, `
		select exists(
			select 1 from consent_grants g join announcement_channel_bindings b on b.workspace_id = g.workspace_id
			where g.id = $1 and b.id = $2 and b.channel = 'listmonk' and b.revoked_at is null
			  and g.recipient_address = $3 and `+listmonkGrantPolicy+`
		)`, grantID, bindingID, normalizeEmail(recipient)).Scan(&granted); err != nil {
		return err
	}
	if !granted {
		return ErrConsentGrantRequired
	}
	return nil
}

// listmonkClient is a narrow adapter over listmonk's documented REST API
// (https://listmonk.app/docs/apis/apis/): subscriber lookup, create and list
// membership (https://listmonk.app/docs/apis/subscribers/) and transactional
// messages (https://listmonk.app/docs/apis/transactional/).
type listmonkClient struct {
	baseURL, user, token string
	http                 *http.Client
}

func newListmonkClient(config Config, client *http.Client) listmonkClient {
	if client == nil {
		client = &http.Client{Timeout: listmonkRequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return listmonkClient{baseURL: strings.TrimRight(config.ListmonkURL, "/"), user: config.ListmonkAPIUser, token: config.ListmonkAPIToken, http: client}
}

// listmonkResult classifies one HTTP exchange. sent is false only when the
// request certainly never reached listmonk (a refused connection).
type listmonkResult struct {
	status int
	body   []byte
	sent   bool
	err    error
}

func (c listmonkClient) call(ctx context.Context, method, path string, payload any) listmonkResult {
	var reader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return listmonkResult{err: err}
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return listmonkResult{err: err}
	}
	req.Header.Set("Authorization", "token "+c.user+":"+c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var opErr *net.OpError
		return listmonkResult{sent: !(errors.As(err, &opErr) && opErr.Op == "dial"), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, listmonkMaxResponseBytes))
	return listmonkResult{status: resp.StatusCode, body: body, sent: true, err: err}
}

// checkList confirms the API user can read the list.
func (c listmonkClient) checkList(ctx context.Context, listID int64) listmonkResult {
	return c.call(ctx, http.MethodGet, "/api/lists/"+strconv.FormatInt(listID, 10), nil)
}

type listmonkSubscriber struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Lists  []struct {
		ID                 int64  `json:"id"`
		SubscriptionStatus string `json:"subscription_status"`
	} `json:"lists"`
}

// findSubscriber looks one address up. listmonk's query parameter is an SQL
// expression, so the address is a quoted literal with quotes doubled.
func (c listmonkClient) findSubscriber(ctx context.Context, email string) (*listmonkSubscriber, listmonkResult) {
	query := url.Values{}
	query.Set("query", "subscribers.email = '"+strings.ReplaceAll(normalizeEmail(email), "'", "''")+"'")
	query.Set("per_page", "1")
	result := c.call(ctx, http.MethodGet, "/api/subscribers?"+query.Encode(), nil)
	if result.err != nil || result.status != http.StatusOK {
		return nil, result
	}
	var out struct {
		Data struct {
			Results []listmonkSubscriber `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result.body, &out); err != nil {
		result.err = err
		return nil, result
	}
	if len(out.Data.Results) == 0 {
		return nil, result
	}
	return &out.Data.Results[0], result
}

// ListmonkRelayReport is the secret-free aggregate output of one relay
// batch: counts only, never addresses or content.
type ListmonkRelayReport struct {
	Claimed    int `json:"claimed"`
	Accepted   int `json:"accepted"`
	Retrying   int `json:"retrying"`
	Uncertain  int `json:"uncertain"`
	Failed     int `json:"failed"`
	Rejected   int `json:"rejected"`
	Withheld   int `json:"withheld"`
	Suppressed int `json:"suppressed"`
	Expired    int `json:"expired"`
}

// RunListmonkRelay sends due listmonk deliveries. It is opt-in: without
// LISTMONK_URL, LISTMONK_API_USER and LISTMONK_API_TOKEN it refuses to run.
func RunListmonkRelay(ctx context.Context, config Config, db *pgxpool.Pool, limit int, httpClient *http.Client) (ListmonkRelayReport, error) {
	report := ListmonkRelayReport{}
	if db == nil {
		return report, errors.New("listmonk relay requires a database")
	}
	if limit < 1 || limit > 100 {
		return report, errors.New("listmonk relay batch limit must be 1..100")
	}
	if !config.listmonkRelayConfigured() {
		return report, errors.New("listmonk relay requires LISTMONK_URL, LISTMONK_API_USER and LISTMONK_API_TOKEN")
	}
	if version, err := CurrentSchemaVersion(ctx, db); err != nil || version != minimumSchemaVersion {
		return report, errors.New("listmonk relay requires current migrations")
	}
	// A send whose worker vanished may have reached listmonk.
	if _, err := db.Exec(ctx, `
		update announcement_listmonk_deliveries
		set status = 'uncertain', last_error = 'lease_expired', lease_expires_at = null, updated_at = now()
		where status = 'sending' and lease_expires_at < now()
	`); err != nil {
		return report, errors.New("listmonk lease sweep failed")
	}
	expired, err := db.Exec(ctx, `
		update announcement_listmonk_deliveries set status = 'expired', updated_at = now()
		where status in ('held', 'pending') and expires_at <= now()
	`)
	if err != nil {
		return report, errors.New("listmonk expiry sweep failed")
	}
	report.Expired = int(expired.RowsAffected())
	client := newListmonkClient(config, httpClient)
	for range limit {
		claimed, outcome, err := deliverOneListmonk(ctx, db, client)
		if err != nil {
			return report, err
		}
		if !claimed {
			break
		}
		report.Claimed++
		switch outcome {
		case listmonkStatusAccepted:
			report.Accepted++
		case listmonkStatusPending:
			report.Retrying++
		case listmonkStatusUncertain:
			report.Uncertain++
		case listmonkStatusFailed:
			report.Failed++
		case listmonkStatusRejected:
			report.Rejected++
		case listmonkStatusWithheld:
			report.Withheld++
		case listmonkStatusSuppressed:
			report.Suppressed++
		case listmonkStatusExpired:
			report.Expired++
		}
	}
	return report, nil
}

type claimedListmonkDelivery struct {
	id, grantID, bindingID, recipient, subject, body, senderAddress string
	listID, templateID                                              int64
	attempt                                                         int
}

func deliverOneListmonk(ctx context.Context, db *pgxpool.Pool, client listmonkClient) (bool, string, error) {
	var d claimedListmonkDelivery
	err := db.QueryRow(ctx, `
		with candidate as (
			select id from announcement_listmonk_deliveries
			where status = 'pending' and next_attempt_at <= now() and expires_at > now()
			order by next_attempt_at
			limit 1
			for update skip locked
		)
		update announcement_listmonk_deliveries d
		set status = 'sending', attempt_count = d.attempt_count + 1,
		    lease_expires_at = now() + make_interval(secs => $1), updated_at = now()
		from candidate, announcement_channel_bindings b
		where d.id = candidate.id and b.id = d.binding_id
		returning d.id::text, d.grant_id::text, d.binding_id::text, d.recipient_address, d.subject, d.body,
		          b.sender_address, b.listmonk_list_id, b.listmonk_template_id, d.attempt_count
	`, listmonkLease.Seconds()).Scan(&d.id, &d.grantID, &d.bindingID, &d.recipient, &d.subject, &d.body,
		&d.senderAddress, &d.listID, &d.templateID, &d.attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", errors.New("listmonk delivery claim failed")
	}
	status, code := sendOneListmonk(ctx, db, client, d)
	if status == listmonkStatusPending && d.attempt >= listmonkMaxAttempts {
		status = listmonkStatusFailed
	}
	return true, status, recordListmonkOutcome(ctx, db, d, status, code)
}

// sendOneListmonk runs the per-recipient steps. Consent, suppression and the
// binding are checked before anything reaches listmonk and again right before
// the /api/tx call. Only that call can leave an uncertain outcome; failures
// before it are safe to retry because nothing was sent.
func sendOneListmonk(ctx context.Context, db *pgxpool.Pool, client listmonkClient, d claimedListmonkDelivery) (status, code string) {
	if denied, code := listmonkConsentDenied(ctx, db, d); denied != "" {
		return denied, code
	}
	subscriber, result := client.findSubscriber(ctx, d.recipient)
	if status, code := classifyListmonkPreSend(result); status != "" {
		return status, code
	}
	if subscriber != nil {
		if subscriber.Status == "blocklisted" {
			return listmonkStatusSuppressed, "provider_blocklisted"
		}
		onList := false
		for _, list := range subscriber.Lists {
			if list.ID != d.listID {
				continue
			}
			if list.SubscriptionStatus == "unsubscribed" {
				// The person left the bound list in listmonk. Record it as a
				// withdrawal so no later occurrence uses the grant either.
				if _, err := db.Exec(ctx, `
					update consent_grants set withdrawn_at = coalesce(withdrawn_at, now()),
					  withdrawal_reason = coalesce(withdrawal_reason, 'listmonk_unsubscribed')
					where id = $1`, d.grantID); err != nil {
					return listmonkStatusPending, "grant_withdraw_failed"
				}
				return listmonkStatusSuppressed, "provider_unsubscribed"
			}
			onList = true
		}
		if !onList {
			// Subcult OS confirmed this address itself, so it joins the bound
			// list as confirmed. An unsubscribe on that list is never undone.
			result = client.call(ctx, http.MethodPut, "/api/subscribers/lists", map[string]any{
				"ids": []int64{subscriber.ID}, "action": "add", "target_list_ids": []int64{d.listID}, "status": "confirmed",
			})
			if status, code := classifyListmonkPreSend(result); status != "" {
				return status, code
			}
		}
	} else {
		result = client.call(ctx, http.MethodPost, "/api/subscribers", map[string]any{
			"email": normalizeEmail(d.recipient), "name": listmonkSubscriberName(d.recipient), "status": "enabled",
			"lists": []int64{d.listID}, "preconfirm_subscriptions": true,
		})
		if result.err == nil && result.status == http.StatusConflict {
			// Created concurrently; the next attempt looks it up again.
			return listmonkStatusPending, "subscriber_conflict"
		}
		if status, code := classifyListmonkPreSend(result); status != "" {
			return status, code
		}
	}
	// Withdrawal before send: the last check before the message can leave.
	if denied, code := listmonkConsentDenied(ctx, db, d); denied != "" {
		return denied, code
	}
	result = client.call(ctx, http.MethodPost, "/api/tx", map[string]any{
		"subscriber_email": normalizeEmail(d.recipient), "template_id": d.templateID, "from_email": d.senderAddress,
		"subject": d.subject, "data": map[string]any{"body": d.body}, "content_type": "plain",
	})
	return classifyListmonkSend(result)
}

func listmonkConsentDenied(ctx context.Context, db *pgxpool.Pool, d claimedListmonkDelivery) (status, code string) {
	err := checkListmonkGrant(ctx, db, d.grantID, d.bindingID, d.recipient)
	switch {
	case err == nil:
		return "", ""
	case errors.Is(err, ErrConsentSuppressed):
		return listmonkStatusSuppressed, "suppressed"
	case errors.Is(err, ErrConsentGrantRequired):
		return listmonkStatusWithheld, "consent_withdrawn"
	default:
		return listmonkStatusPending, "consent_check_failed"
	}
}

// classifyListmonkPreSend handles a subscriber lookup or upsert. Nothing has
// been sent yet, so transport failures retry; "" means continue.
func classifyListmonkPreSend(result listmonkResult) (status, code string) {
	switch {
	case result.err != nil:
		return listmonkStatusPending, "listmonk_unreachable"
	case result.status >= 200 && result.status < 300:
		return "", ""
	case result.status == http.StatusUnauthorized || result.status == http.StatusForbidden:
		return listmonkStatusFailed, "listmonk_http_" + strconv.Itoa(result.status)
	case result.status == http.StatusTooManyRequests || result.status >= 500:
		return listmonkStatusPending, "listmonk_http_" + strconv.Itoa(result.status)
	default:
		return listmonkStatusRejected, "listmonk_http_" + strconv.Itoa(result.status)
	}
}

// classifyListmonkSend handles the /api/tx call. listmonk has no idempotency
// key for transactional messages, so any answer that might follow a send is
// uncertain and held rather than resent.
func classifyListmonkSend(result listmonkResult) (status, code string) {
	switch {
	case result.err != nil && !result.sent:
		return listmonkStatusPending, "listmonk_unreachable"
	case result.err != nil:
		return listmonkStatusUncertain, "listmonk_no_response"
	case result.status >= 200 && result.status < 300:
		return listmonkStatusAccepted, ""
	case result.status == http.StatusTooManyRequests || result.status == http.StatusServiceUnavailable:
		return listmonkStatusPending, "listmonk_http_" + strconv.Itoa(result.status)
	case result.status >= 500:
		return listmonkStatusUncertain, "listmonk_http_" + strconv.Itoa(result.status)
	case result.status == http.StatusUnauthorized || result.status == http.StatusForbidden || result.status == http.StatusRequestTimeout:
		return listmonkStatusFailed, "listmonk_http_" + strconv.Itoa(result.status)
	default:
		return listmonkStatusRejected, "listmonk_http_" + strconv.Itoa(result.status)
	}
}

func listmonkRetryDelay(attempt int) time.Duration {
	delay := time.Minute
	for i := 1; i < attempt && delay < 30*time.Minute; i++ {
		delay *= 2
	}
	return min(delay, 30*time.Minute)
}

// recordListmonkOutcome writes the result only while the row is still the
// attempt this worker claimed.
func recordListmonkOutcome(ctx context.Context, db *pgxpool.Pool, d claimedListmonkDelivery, status, code string) error {
	_, err := db.Exec(ctx, `
		update announcement_listmonk_deliveries
		set status = $3, last_error = nullif($4, ''),
		    accepted_at = case when $3 = 'accepted' then now() else accepted_at end,
		    next_attempt_at = now() + make_interval(secs => $5), lease_expires_at = null, updated_at = now()
		where id = $1 and status = 'sending' and attempt_count = $2
	`, d.id, d.attempt, status, code, listmonkRetryDelay(d.attempt).Seconds())
	if err != nil {
		return errors.New("listmonk delivery outcome write failed")
	}
	return nil
}

// listmonkSubscriberName uses the local part, which older listmonk versions
// require as a name.
func listmonkSubscriberName(email string) string {
	email = normalizeEmail(email)
	if at := strings.IndexByte(email, '@'); at > 0 {
		return email[:at]
	}
	return email
}

// dispatchListmonkRecipients is the listmonk branch of
// dispatchOneAnnouncement. It runs inside the dispatch transaction with the
// binding row share-locked: one durable row per allowed recipient, nothing
// sent here.
func (a *App) dispatchListmonkRecipients(ctx context.Context, tx pgx.Tx, binding listmonkBinding, workspaceID, announcementID, subject, body string) (enqueued, withheld int, err error) {
	rows, err := tx.Query(ctx, `
		select distinct on (g.recipient_address) g.id::text, g.recipient_address
		from consent_grants g
		where g.workspace_id = $1 and g.channel = 'email' and g.purpose = $2 and g.verified_at is not null
		order by g.recipient_address, (g.withdrawn_at is null) desc,
		         (g.listmonk_binding_id = $3 or (g.listmonk_binding_id is null and g.disclosure_version = $4)) desc nulls last,
		         g.granted_at desc, g.id
	`, workspaceID, consentPurposeAnnouncement, binding.ID, binding.DisclosureVersion)
	if err != nil {
		return 0, 0, errors.New("announcement audience query failed")
	}
	var recipients []announcementRecipient
	for rows.Next() {
		var rec announcementRecipient
		if err := rows.Scan(&rec.grantID, &rec.recipientAddress); err != nil {
			rows.Close()
			return 0, 0, errors.New("announcement audience scan failed")
		}
		recipients = append(recipients, rec)
	}
	closeErr := rows.Err()
	rows.Close()
	if closeErr != nil {
		return 0, 0, errors.New("announcement audience read failed")
	}
	status := listmonkStatusHeld
	if a.config.MailDeliveryEnabled {
		status = listmonkStatusPending
	}
	for _, rec := range recipients {
		if _, err := tx.Exec(ctx, `select 1 from consent_grants where id = $1 for share`, rec.grantID); err != nil {
			return 0, 0, errors.New("announcement grant check failed")
		}
		if permErr := checkListmonkGrant(ctx, tx, rec.grantID, binding.ID, rec.recipientAddress); permErr != nil {
			if !errors.Is(permErr, ErrConsentGrantRequired) && !errors.Is(permErr, ErrConsentSuppressed) {
				return 0, 0, errors.New("announcement dispatch consent check failed")
			}
			withheld++
			continue
		}
		withdrawToken, withdrawHash, err := newToken()
		if err != nil {
			return 0, 0, errors.New("announcement withdraw token mint failed")
		}
		if _, err := tx.Exec(ctx, `insert into consent_withdraw_tokens(grant_id, token_hash) values ($1, $2)`, rec.grantID, withdrawHash); err != nil {
			return 0, 0, errors.New("announcement withdraw token record failed")
		}
		messageBody := body + "\n\nTo stop receiving these messages: " + strings.TrimRight(a.config.PublicWebURL, "/") + "/consent/withdraw?token=" + withdrawToken
		if _, err := tx.Exec(ctx, `
			insert into announcement_listmonk_deliveries
				(announcement_id, workspace_id, binding_id, grant_id, recipient_address, subject, body, status, expires_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, now() + make_interval(secs => $9))
		`, announcementID, workspaceID, binding.ID, rec.grantID, rec.recipientAddress, subject, messageBody, status, listmonkDeliveryWindow.Seconds()); err != nil {
			return 0, 0, errors.New("announcement listmonk delivery record failed")
		}
		enqueued++
	}
	return enqueued, withheld, nil
}
