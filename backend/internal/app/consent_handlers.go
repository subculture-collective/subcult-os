package app

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"
)

// consentDisclosureVersionMinLength is a cheap sanity floor, not a format
// contract: disclosure_version is an opaque operator-supplied label (see
// docs/development/consent.md) identifying which disclosure text the
// recipient agreed to.
const consentDisclosureVersionMinLength = 1

type createConsentGrantRequest struct {
	Channel           string `json:"channel"`
	RecipientAddress  string `json:"recipientAddress"`
	Purpose           string `json:"purpose"`
	Scope             string `json:"scope"`
	DisclosureVersion string `json:"disclosureVersion"`
	Source            string `json:"source"`
}

type consentGrantDTO struct {
	ID                string  `json:"id"`
	Channel           string  `json:"channel"`
	RecipientAddress  string  `json:"recipientAddress"`
	Purpose           string  `json:"purpose"`
	Scope             string  `json:"scope"`
	DisclosureVersion string  `json:"disclosureVersion"`
	Source            string  `json:"source"`
	GrantedAt         string  `json:"grantedAt"`
	VerifiedAt        *string `json:"verifiedAt,omitempty"`
	WithdrawnAt       *string `json:"withdrawnAt,omitempty"`
	WithdrawalReason  *string `json:"withdrawalReason,omitempty"`
}

// handleCreateConsentGrant records an unverified consent grant and enqueues
// a transactional verification email containing the confirm link. Creating
// or verifying a grant is itself a transactional message: it never
// requires a grant (see checkSendPermission), because the grant it is
// establishing does not exist yet.
func (a *App) handleCreateConsentGrant(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	actorID, ok := a.requirePermission(r, workspaceID, permManageConsent)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req createConsentGrantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !consentChannels[req.Channel] {
		writeError(w, http.StatusBadRequest, "channel must be one of: email")
		return
	}
	recipient := normalizeEmail(req.RecipientAddress)
	if recipient == "" || !strings.Contains(recipient, "@") {
		writeError(w, http.StatusBadRequest, "recipientAddress is required")
		return
	}
	// Only announcement grants are meaningful to record: transactional
	// messages never require one (see checkSendPermission), so recording
	// a transactional "grant" here would be a permission that is never
	// consulted, not a real consent record.
	if req.Purpose != consentPurposeAnnouncement {
		writeError(w, http.StatusBadRequest, "purpose must be: announcement")
		return
	}
	if len(strings.TrimSpace(req.DisclosureVersion)) < consentDisclosureVersionMinLength {
		writeError(w, http.StatusBadRequest, "disclosureVersion is required")
		return
	}
	if !consentSources[req.Source] {
		writeError(w, http.StatusBadRequest, "source must be one of: explicit_form, operator_recorded")
		return
	}

	token, hashed, err := newToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate verification token")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var grantID string
	err = tx.QueryRow(r.Context(), `
		insert into consent_grants (
			workspace_id, channel, recipient_address, purpose, scope,
			verification_token_hash, disclosure_version, source, created_by_person_id
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		returning id
	`, workspaceID, req.Channel, recipient, req.Purpose, req.Scope, hashed, req.DisclosureVersion, req.Source, actorID).Scan(&grantID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "an active consent grant already exists for this recipient and purpose")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not record consent grant")
		return
	}

	confirmPath := "/consent/confirm?token=" + token
	subject := "Confirm your subscription"
	body := subject + "\n\n" + strings.TrimRight(a.config.PublicWebURL, "/") + confirmPath
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.enqueueEmail(txCtx, recipient, subject, body, "consent_verification", grantID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not enqueue verification email")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit consent grant")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": grantID, "status": "unverified"})
}

// handleListConsentGrants lists every consent grant recorded for the
// workspace, newest first. verification_token_hash and any raw token are
// never included: the DTO carries only what an operator legitimately
// needs to audit consent state.
func (a *App) handleListConsentGrants(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if _, ok := a.requirePermission(r, workspaceID, permManageConsent); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := a.db.Query(r.Context(), `
		select id, channel, recipient_address, purpose, scope, disclosure_version, source,
		       granted_at, verified_at, withdrawn_at, withdrawal_reason
		from consent_grants
		where workspace_id = $1
		order by granted_at desc
	`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load consent grants")
		return
	}
	defer rows.Close()

	out := make([]consentGrantDTO, 0)
	for rows.Next() {
		var item consentGrantDTO
		var grantedAt time.Time
		var verifiedAt, withdrawnAt sql.NullTime
		var withdrawalReason sql.NullString
		if err := rows.Scan(&item.ID, &item.Channel, &item.RecipientAddress, &item.Purpose, &item.Scope,
			&item.DisclosureVersion, &item.Source, &grantedAt, &verifiedAt, &withdrawnAt, &withdrawalReason); err != nil {
			writeError(w, http.StatusInternalServerError, "could not scan consent grants")
			return
		}
		item.GrantedAt = grantedAt.UTC().Format(time.RFC3339Nano)
		item.VerifiedAt = formatNullableTime(verifiedAt)
		item.WithdrawnAt = formatNullableTime(withdrawnAt)
		if withdrawalReason.Valid {
			item.WithdrawalReason = &withdrawalReason.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read consent grants")
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// handleConfirmConsentGrant is a public, tokenized route: no session is
// required or accepted. It reveals nothing beyond a generic not-found for
// an unrecognized or already-withdrawn token, and never returns the
// recipient address, workspace or any other grant field.
func (a *App) handleConfirmConsentGrant(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	token := r.PathValue("token")
	if token == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	result, err := a.db.Exec(r.Context(), `
		update consent_grants set verified_at = coalesce(verified_at, now())
		where verification_token_hash = $1 and withdrawn_at is null
	`, tokenHash(token))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not confirm consent grant")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

// linkedAnnouncementWithdrawal is a CTE that follows a "target" CTE of
// withdrawn grants. A withdrawal of announcement consent covers the person,
// not one grant row: an address can hold a native grant and Kork-origin
// grants (#220) in the same workspace, and all of them are withdrawn. reason
// is a fixed literal, never input.
func linkedAnnouncementWithdrawal(reason string) string {
	return `linked as (
			update consent_grants g
			set withdrawn_at = now(), withdrawal_reason = '` + reason + `'
			from target t
			where t.purpose = 'announcement' and g.purpose = 'announcement' and g.workspace_id = t.workspace_id
			  and g.channel = t.channel and g.recipient_address = t.recipient_address
			  and g.withdrawn_at is null and g.id not in (select id from target)
			returning g.id
		)`
}

// handleWithdrawConsentGrant is the public, tokenized unsubscribe route
// (POST only, so link-prefetching mail scanners cannot withdraw on the
// recipient's behalf; the emailed link opens the web page that posts).
// Like confirm, it accepts no session and reveals nothing beyond a generic
// not-found; it is idempotent, so submitting an already-used unsubscribe
// twice still succeeds rather than erroring.
func (a *App) handleWithdrawConsentGrant(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	token := r.PathValue("token")
	if token == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	result, err := a.db.Exec(r.Context(), `
		with target as (
			update consent_grants
			set withdrawn_at = coalesce(withdrawn_at, now()),
			    withdrawal_reason = coalesce(withdrawal_reason, 'recipient_requested')
			where verification_token_hash = $1 or id in (
				select grant_id from consent_withdraw_tokens where token_hash = $1
			)
			returning id, workspace_id, channel, recipient_address, purpose
		), `+linkedAnnouncementWithdrawal("recipient_requested")+`
		select 1 from target
	`, tokenHash(token))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not withdraw consent grant")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "withdrawn"})
}

// handleOperatorWithdrawConsentGrant records a withdrawal the workspace
// received out of band (reply email, phone, in person). It needs
// permManageConsent and is scoped to the workspace in the path, so an
// operator can never withdraw another workspace's grant by id.
func (a *App) handleOperatorWithdrawConsentGrant(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	actorID, ok := a.requirePermission(r, workspaceID, permManageConsent)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	grantID := r.PathValue("grantID")
	if !validLinkID(grantID) {
		writeError(w, http.StatusNotFound, "consent grant not found")
		return
	}
	result, err := a.db.Exec(r.Context(), `
		with target as (
			update consent_grants
			set withdrawn_at = coalesce(withdrawn_at, now()),
			    withdrawal_reason = coalesce(withdrawal_reason, 'operator_recorded')
			where id = $1 and workspace_id = $2
			returning id, workspace_id, channel, recipient_address, purpose
		), `+linkedAnnouncementWithdrawal("operator_recorded")+`
		select 1 from target
	`, grantID, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not withdraw consent grant")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "consent grant not found")
		return
	}
	if err := a.audit(r.Context(), actorID, "consent_grant.withdrawn", "consent_grant", grantID, map[string]any{
		"workspaceId": workspaceID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "withdrawn"})
}
