package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Issue #220: organizer-facing listmonk bindings, Kork receipt import and
// retry. See docs/development/listmonk-relay.md.

var listmonkSenderAddress = regexp.MustCompile(`^[^@\s<>]+@[^@\s<>]+$`)

type listmonkBindingDTO struct {
	ID                string          `json:"id"`
	Label             string          `json:"label"`
	ListID            int64           `json:"listId"`
	TemplateID        int64           `json:"templateId"`
	SenderAddress     string          `json:"senderAddress"`
	DisclosureVersion string          `json:"disclosureVersion"`
	KorkPolicy        *korkPolicyBody `json:"korkPolicy"`
	Active            bool            `json:"active"`
	CreatedAt         string          `json:"createdAt"`
	RevokedAt         *string         `json:"revokedAt,omitempty"`
}

// korkPolicyBody is the consent-receiver policy for Kork receipts: the exact
// listmonk URL, sender (page title), purpose (block title) and disclosure
// text a Kork receipt must carry to count for this binding.
type korkPolicyBody struct {
	ListURL    string `json:"listUrl"`
	Sender     string `json:"sender"`
	Purpose    string `json:"purpose"`
	Disclosure string `json:"disclosure"`
}

func (b listmonkBinding) dto() listmonkBindingDTO {
	dto := listmonkBindingDTO{ID: b.ID, Label: b.Label, ListID: b.ListID, TemplateID: b.TemplateID, SenderAddress: b.SenderAddress,
		DisclosureVersion: b.DisclosureVersion, Active: b.RevokedAt == nil, CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339Nano)}
	if b.RevokedAt != nil {
		revoked := b.RevokedAt.UTC().Format(time.RFC3339Nano)
		dto.RevokedAt = &revoked
	}
	if b.KorkListURL != nil {
		dto.KorkPolicy = &korkPolicyBody{ListURL: *b.KorkListURL, Sender: *b.KorkSender, Purpose: *b.KorkPurpose, Disclosure: *b.KorkDisclosure}
	}
	return dto
}

func (a *App) handleListListmonkBindings(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if _, ok := a.requirePermission(r, workspaceID, permManageAnnouncements); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select `+listmonkBindingColumns+` from announcement_channel_bindings
		where workspace_id = $1 and channel = $2 order by created_at desc`, workspaceID, channelListmonkName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load bindings")
		return
	}
	defer rows.Close()
	out := []listmonkBindingDTO{}
	for rows.Next() {
		binding, err := scanListmonkBinding(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load bindings")
			return
		}
		out = append(out, binding.dto())
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "could not load bindings")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type createListmonkBindingRequest struct {
	Label             string          `json:"label"`
	ListID            int64           `json:"listId"`
	TemplateID        int64           `json:"templateId"`
	SenderAddress     string          `json:"senderAddress"`
	DisclosureVersion string          `json:"disclosureVersion"`
	KorkPolicy        *korkPolicyBody `json:"korkPolicy"`
}

// handleCreateListmonkBinding binds the workspace to exactly one listmonk
// list, template and sender, plus its consent-receiver policy. listmonk must
// confirm the API user can read the list before the binding is saved.
func (a *App) handleCreateListmonkBinding(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	actorID, ok := a.requirePermission(r, workspaceID, permManageChannelBindings)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req createListmonkBindingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	label, sender, disclosure := strings.TrimSpace(req.Label), normalizeEmail(req.SenderAddress), strings.TrimSpace(req.DisclosureVersion)
	switch {
	case label == "" || len([]rune(label)) > 80:
		writeError(w, http.StatusBadRequest, "label must be 1..80 characters")
		return
	case req.ListID < 1 || req.ListID > 1<<31-1 || req.TemplateID < 1 || req.TemplateID > 1<<31-1:
		writeError(w, http.StatusBadRequest, "listId and templateId must be positive listmonk ids")
		return
	case !listmonkSenderAddress.MatchString(sender):
		writeError(w, http.StatusBadRequest, "senderAddress must be an email address")
		return
	case disclosure == "" || len([]rune(disclosure)) > 80:
		writeError(w, http.StatusBadRequest, "disclosureVersion must be 1..80 characters")
		return
	}
	var kork *korkPolicyBody
	if req.KorkPolicy != nil {
		kork = &korkPolicyBody{ListURL: strings.TrimRight(strings.ToLower(strings.TrimSpace(req.KorkPolicy.ListURL)), "/"),
			Sender: req.KorkPolicy.Sender, Purpose: req.KorkPolicy.Purpose, Disclosure: req.KorkPolicy.Disclosure}
		parsed, err := url.Parse(kork.ListURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			kork.Sender == "" || len(kork.Sender) > 200 || kork.Purpose == "" || len(kork.Purpose) > 200 || kork.Disclosure == "" || len(kork.Disclosure) > 1000 {
			writeError(w, http.StatusBadRequest, "korkPolicy needs an https listUrl and the exact sender, purpose and disclosure text")
			return
		}
	}
	if !a.config.listmonkRelayConfigured() {
		writeError(w, http.StatusConflict, "listmonk relay is not configured")
		return
	}
	check := newListmonkClient(a.config, nil).checkList(r.Context(), req.ListID)
	switch {
	case check.err != nil || check.status >= 500 || check.status == http.StatusTooManyRequests:
		writeError(w, http.StatusBadGateway, "listmonk is unavailable")
		return
	case check.status == http.StatusUnauthorized || check.status == http.StatusForbidden:
		writeError(w, http.StatusBadGateway, "listmonk rejected the configured API user")
		return
	case check.status == http.StatusNotFound:
		writeError(w, http.StatusUnprocessableEntity, "listmonk has no list with that id")
		return
	case check.status != http.StatusOK:
		writeError(w, http.StatusBadGateway, "listmonk could not confirm the list")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save binding")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var korkURL, korkSender, korkPurpose, korkDisclosure *string
	if kork != nil {
		korkURL, korkSender, korkPurpose, korkDisclosure = &kork.ListURL, &kork.Sender, &kork.Purpose, &kork.Disclosure
	}
	binding, err := scanListmonkBinding(tx.QueryRow(r.Context(), `
		insert into announcement_channel_bindings
			(workspace_id, channel, label, listmonk_list_id, listmonk_template_id, sender_address, disclosure_version,
			 kork_list_url, kork_sender, kork_purpose, kork_disclosure, created_by_person_id)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		returning `+listmonkBindingColumns, workspaceID, channelListmonkName, label, req.ListID, req.TemplateID, sender, disclosure,
		korkURL, korkSender, korkPurpose, korkDisclosure, actorID))
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "revoke the active listmonk binding before connecting another")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save binding")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "announcement.channel_binding.created", "announcement_channel_binding", binding.ID, map[string]any{
		"workspaceId": workspaceID, "channel": channelListmonkName, "listId": req.ListID, "templateId": req.TemplateID,
		"acceptsKorkReceipts": kork != nil,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save binding")
		return
	}
	writeJSON(w, http.StatusCreated, binding.dto())
}

// handleRevokeListmonkBinding keeps the binding for audit, withholds its
// unsent deliveries and stops new dispatches from routing to it. Kork-origin
// grants scoped to it authorize nothing afterwards.
func (a *App) handleRevokeListmonkBinding(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	actorID, ok := a.requirePermission(r, workspaceID, permManageChannelBindings)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	bindingID := r.PathValue("bindingID")
	if _, err := uuid.Parse(bindingID); err != nil {
		writeError(w, http.StatusNotFound, "binding not found")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke binding")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	binding, err := scanListmonkBinding(tx.QueryRow(r.Context(), `
		update announcement_channel_bindings set revoked_at = now(), revoked_by_person_id = $3
		where id = $1 and workspace_id = $2 and channel = $4 and revoked_at is null
		returning `+listmonkBindingColumns, bindingID, workspaceID, actorID, channelListmonkName))
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		_ = tx.QueryRow(r.Context(), `select exists(select 1 from announcement_channel_bindings where id = $1 and workspace_id = $2 and channel = $3)`,
			bindingID, workspaceID, channelListmonkName).Scan(&exists)
		if exists {
			writeError(w, http.StatusConflict, "binding is already revoked")
			return
		}
		writeError(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke binding")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		update announcement_listmonk_deliveries set status = 'withheld', last_error = 'binding_revoked', updated_at = now()
		where binding_id = $1 and status in ('held', 'pending')
	`, bindingID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not withhold deliveries")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "announcement.channel_binding.revoked", "announcement_channel_binding", binding.ID, map[string]any{"workspaceId": workspaceID}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke binding")
		return
	}
	writeJSON(w, http.StatusOK, binding.dto())
}

type importKorkGrantRequest struct {
	RecipientAddress string          `json:"recipientAddress"`
	Receipt          json.RawMessage `json:"receipt"`
}

// handleImportKorkGrant records a verified announcement grant from a Kork
// sign-up receipt, scoped to this listmonk binding, only when the receipt
// names exactly the binding's list, sender, purpose and disclosure. Anything
// else is refused and the recipient needs the native confirmation flow.
func (a *App) handleImportKorkGrant(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	actorID, ok := a.requirePermission(r, workspaceID, permManageConsent)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	bindingID := r.PathValue("bindingID")
	if _, err := uuid.Parse(bindingID); err != nil {
		writeError(w, http.StatusNotFound, "binding not found")
		return
	}
	var req importKorkGrantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	recipient := normalizeEmail(req.RecipientAddress)
	if recipient == "" || !strings.Contains(recipient, "@") {
		writeError(w, http.StatusBadRequest, "recipientAddress is required")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record consent grant")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	binding, err := scanListmonkBinding(tx.QueryRow(r.Context(), `select `+listmonkBindingColumns+` from announcement_channel_bindings
		where id = $1 and workspace_id = $2 and channel = $3 for share`, bindingID, workspaceID, channelListmonkName))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load binding")
		return
	}
	if binding.RevokedAt != nil {
		writeError(w, http.StatusConflict, "binding is revoked")
		return
	}
	receipt, code := validateKorkReceipt(binding, req.Receipt, time.Now())
	if code == "binding_accepts_no_kork_receipts" {
		writeError(w, http.StatusConflict, code)
		return
	}
	if code != "" {
		writeError(w, http.StatusUnprocessableEntity, code)
		return
	}
	var suppressed, withdrawnLater bool
	if err := tx.QueryRow(r.Context(), `
		select exists(select 1 from email_suppressions where recipient_email = $1),
		       exists(select 1 from consent_grants where workspace_id = $2 and channel = 'email' and recipient_address = $1
		              and purpose = $3 and withdrawn_at >= $4)
	`, recipient, workspaceID, consentPurposeAnnouncement, receipt.At).Scan(&suppressed, &withdrawnLater); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record consent grant")
		return
	}
	if suppressed {
		writeError(w, http.StatusConflict, "recipient_suppressed")
		return
	}
	// A receipt older than a recorded withdrawal never restores consent.
	if withdrawnLater {
		writeError(w, http.StatusConflict, "grant_withdrawn")
		return
	}
	_, hashed, err := newToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record consent grant")
		return
	}
	stored, _ := json.Marshal(receipt)
	var grantID string
	err = tx.QueryRow(r.Context(), `
		insert into consent_grants (
			workspace_id, channel, recipient_address, purpose, scope, verification_token_hash, verified_at,
			disclosure_version, source, created_by_person_id, listmonk_binding_id, origin_receipt
		)
		values ($1, 'email', $2, $3, '', $4, now(), 'kork:v1', $5, $6, $7, $8::jsonb)
		returning id::text
	`, workspaceID, recipient, consentPurposeAnnouncement, hashed, consentSourceKorkReceipt, actorID, binding.ID, string(stored)).Scan(&grantID)
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "an active consent grant already exists for this recipient and purpose")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record consent grant")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "consent_grant.kork_receipt_imported", "consent_grant", grantID, map[string]any{
		"workspaceId": workspaceID, "bindingId": binding.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record consent grant")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": grantID, "status": "verified"})
}

// handleRetryListmonkDeliveries re-queues an announcement's held uncertain or
// failed listmonk deliveries while their binding is active. listmonk has no
// idempotency key for transactional messages, so an organizer checks
// listmonk's logs before retrying an uncertain one.
func (a *App) handleRetryListmonkDeliveries(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	actorID, ok := a.requirePermission(r, workspaceID, permManageAnnouncements)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	announcementID := r.PathValue("announcementID")
	if _, err := uuid.Parse(announcementID); err != nil {
		writeError(w, http.StatusNotFound, "announcement not found")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not retry deliveries")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var exists bool
	if err := tx.QueryRow(r.Context(), `select exists(select 1 from announcements where id = $1 and workspace_id = $2)`, announcementID, workspaceID).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, "could not retry deliveries")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "announcement not found")
		return
	}
	result, err := tx.Exec(r.Context(), `
		update announcement_listmonk_deliveries d
		set status = 'pending', attempt_count = 0, next_attempt_at = now(), last_error = null, updated_at = now()
		from announcement_channel_bindings b
		where d.announcement_id = $1 and d.workspace_id = $2 and b.id = d.binding_id and b.revoked_at is null
		  and d.status in ('uncertain', 'failed') and d.expires_at > now()
	`, announcementID, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not retry deliveries")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "announcement.listmonk_deliveries.retried", "announcement", announcementID, map[string]any{
		"workspaceId": workspaceID, "count": result.RowsAffected(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not retry deliveries")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"requeued": result.RowsAffected()})
}

// loadListmonkStatusCounts is the listmonk half of the announcement outcome
// ledger: counts by status only.
func (a *App) loadListmonkStatusCounts(ctx context.Context, announcementID string) (map[string]int, error) {
	rows, err := a.db.Query(ctx, `select status, count(*) from announcement_listmonk_deliveries where announcement_id = $1 group by status`, announcementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}
