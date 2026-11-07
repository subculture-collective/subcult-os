package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Issue #221: organizer-facing Backline bindings, preview and handoff. See
// docs/development/backline-handoff.md.

type channelBindingDTO struct {
	ID            string  `json:"id"`
	Channel       string  `json:"channel"`
	Label         string  `json:"label"`
	ServerID      string  `json:"serverId"`
	ChannelID     string  `json:"channelId"`
	MentionPolicy string  `json:"mentionPolicy"`
	Active        bool    `json:"active"`
	CreatedAt     string  `json:"createdAt"`
	RevokedAt     *string `json:"revokedAt,omitempty"`
}

type channelHandoffDTO struct {
	ID                string  `json:"id"`
	BindingID         string  `json:"bindingId"`
	BindingLabel      string  `json:"bindingLabel"`
	OccurrenceKey     string  `json:"occurrenceKey"`
	ContentRevision   string  `json:"contentRevision"`
	Operation         string  `json:"operation"`
	Status            string  `json:"status"`
	AttemptCount      int     `json:"attemptCount"`
	LastError         *string `json:"lastError,omitempty"`
	ReceiverStatus    *string `json:"receiverStatus,omitempty"`
	ReceiverReference *string `json:"receiverReference,omitempty"`
	DeliverAt         string  `json:"deliverAt"`
	ExpiresAt         string  `json:"expiresAt"`
	AcknowledgedAt    *string `json:"acknowledgedAt,omitempty"`
	CancelRequestedAt *string `json:"cancelRequestedAt,omitempty"`
	DeliveryOutcome   *string `json:"deliveryOutcome,omitempty"`
	DeliveryOutcomeAt *string `json:"deliveryOutcomeAt,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
}

type backlinePreviewDTO struct {
	AnnouncementID    string             `json:"announcementId"`
	AnnouncementState string             `json:"announcementStatus"`
	Approved          bool               `json:"approved"`
	Subject           string             `json:"subject"`
	Body              string             `json:"body"`
	ContentRevision   string             `json:"contentRevision"`
	ContentLength     int                `json:"contentLength"`
	ContentLimit      int                `json:"contentLimit"`
	MentionPolicy     string             `json:"mentionPolicy"`
	DeliverAt         string             `json:"deliverAt"`
	ExpiresAt         string             `json:"expiresAt"`
	AdapterConfigured bool               `json:"adapterConfigured"`
	Binding           *channelBindingDTO `json:"binding"`
	Handoff           *channelHandoffDTO `json:"handoff"`
	Blockers          []string           `json:"blockers"`
}

const bindingColumns = `id::text, channel, label, external_server_id, external_channel_id, mention_policy, created_at, revoked_at`

func scanChannelBinding(row pgx.Row) (channelBindingDTO, error) {
	var dto channelBindingDTO
	var createdAt time.Time
	var revokedAt sql.NullTime
	if err := row.Scan(&dto.ID, &dto.Channel, &dto.Label, &dto.ServerID, &dto.ChannelID, &dto.MentionPolicy, &createdAt, &revokedAt); err != nil {
		return dto, err
	}
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	dto.RevokedAt = formatNullableTime(revokedAt)
	dto.Active = !revokedAt.Valid
	return dto, nil
}

const handoffColumns = `h.id::text, h.binding_id::text, b.label, h.occurrence_key, h.content_revision, h.operation, h.status,
	h.attempt_count, h.last_error, h.receiver_status, h.receiver_reference, h.deliver_at, h.expires_at,
	h.acknowledged_at, h.cancel_requested_at, h.delivery_outcome, h.delivery_outcome_at, h.created_at, h.updated_at`

func scanChannelHandoff(row pgx.Row) (channelHandoffDTO, error) {
	var dto channelHandoffDTO
	var lastError, receiverStatus, receiverReference, deliveryOutcome sql.NullString
	var deliverAt, expiresAt, createdAt, updatedAt time.Time
	var acknowledgedAt, cancelRequestedAt, deliveryOutcomeAt sql.NullTime
	if err := row.Scan(&dto.ID, &dto.BindingID, &dto.BindingLabel, &dto.OccurrenceKey, &dto.ContentRevision, &dto.Operation, &dto.Status,
		&dto.AttemptCount, &lastError, &receiverStatus, &receiverReference, &deliverAt, &expiresAt,
		&acknowledgedAt, &cancelRequestedAt, &deliveryOutcome, &deliveryOutcomeAt, &createdAt, &updatedAt); err != nil {
		return dto, err
	}
	nullable := func(value sql.NullString) *string {
		if !value.Valid {
			return nil
		}
		return &value.String
	}
	dto.LastError, dto.ReceiverStatus = nullable(lastError), nullable(receiverStatus)
	dto.ReceiverReference, dto.DeliveryOutcome = nullable(receiverReference), nullable(deliveryOutcome)
	dto.DeliverAt = deliverAt.UTC().Format(time.RFC3339)
	dto.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
	dto.AcknowledgedAt = formatNullableTime(acknowledgedAt)
	dto.CancelRequestedAt = formatNullableTime(cancelRequestedAt)
	dto.DeliveryOutcomeAt = formatNullableTime(deliveryOutcomeAt)
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	dto.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	return dto, nil
}

// loadAnnouncementHandoffs is the channel half of the announcement outcome
// ledger, returned beside the email delivery counts.
func (a *App) loadAnnouncementHandoffs(ctx context.Context, announcementID string) ([]channelHandoffDTO, error) {
	rows, err := a.db.Query(ctx, `
		select `+handoffColumns+`
		from announcement_channel_handoffs h join announcement_channel_bindings b on b.id = h.binding_id
		where h.announcement_id = $1 order by h.created_at
	`, announcementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []channelHandoffDTO{}
	for rows.Next() {
		dto, err := scanChannelHandoff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, rows.Err()
}

func (a *App) handleListChannelBindings(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if _, ok := a.requirePermission(r, workspaceID, permManageAnnouncements); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	rows, err := a.db.Query(r.Context(), `select `+bindingColumns+` from announcement_channel_bindings where workspace_id = $1 and channel = $2 order by created_at desc`, workspaceID, channelBacklineName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load bindings")
		return
	}
	defer rows.Close()
	out := []channelBindingDTO{}
	for rows.Next() {
		dto, err := scanChannelBinding(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load bindings")
			return
		}
		out = append(out, dto)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "could not load bindings")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type createChannelBindingRequest struct {
	Label     string `json:"label"`
	ServerID  string `json:"serverId"`
	ChannelID string `json:"channelId"`
}

// handleCreateChannelBinding records one approved Backline server/channel for
// the workspace. Only one binding may be active; replace it by revoking first.
func (a *App) handleCreateChannelBinding(w http.ResponseWriter, r *http.Request) {
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
	var req createChannelBindingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	label := strings.TrimSpace(req.Label)
	serverID, channelID := strings.TrimSpace(req.ServerID), strings.TrimSpace(req.ChannelID)
	if label == "" || len([]rune(label)) > 80 {
		writeError(w, http.StatusBadRequest, "label must be 1..80 characters")
		return
	}
	if !discordSnowflake.MatchString(serverID) || !discordSnowflake.MatchString(channelID) {
		writeError(w, http.StatusBadRequest, "serverId and channelId must be Discord snowflake ids")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save binding")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	dto, err := scanChannelBinding(tx.QueryRow(r.Context(), `
		insert into announcement_channel_bindings (workspace_id, channel, label, external_server_id, external_channel_id, created_by_person_id)
		values ($1, $2, $3, $4, $5, $6)
		returning `+bindingColumns, workspaceID, channelBacklineName, label, serverID, channelID, actorID))
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "revoke the active Backline binding before connecting another")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save binding")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "announcement.channel_binding.created", "announcement_channel_binding", dto.ID, map[string]any{
		"workspaceId": workspaceID, "channel": channelBacklineName, "serverId": serverID, "channelId": channelID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save binding")
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

// handleRevokeChannelBinding keeps the binding for audit, refuses new
// handoffs to it and withdraws its existing handoffs (see
// requestHandoffCancellation).
func (a *App) handleRevokeChannelBinding(w http.ResponseWriter, r *http.Request) {
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
	dto, err := scanChannelBinding(tx.QueryRow(r.Context(), `
		update announcement_channel_bindings set revoked_at = now(), revoked_by_person_id = $3
		where id = $1 and workspace_id = $2 and channel = $4 and revoked_at is null
		returning `+bindingColumns, bindingID, workspaceID, actorID, channelBacklineName))
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		_ = tx.QueryRow(r.Context(), `select exists(select 1 from announcement_channel_bindings where id = $1 and workspace_id = $2 and channel = $3)`, bindingID, workspaceID, channelBacklineName).Scan(&exists)
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
	if err := requestHandoffCancellation(r.Context(), tx, "binding_id", bindingID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not withdraw handoffs")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "announcement.channel_binding.revoked", "announcement_channel_binding", dto.ID, map[string]any{"workspaceId": workspaceID}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke binding")
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// lockAnnouncementHandoffsForReschedule locks every handoff of the
// announcement and reports whether Backline may already hold one (any row
// other than an unsent publish, or one marked receiver_may_hold) and how many
// unsent pending publishes should follow the new time.
func lockAnnouncementHandoffsForReschedule(ctx context.Context, tx pgx.Tx, announcementID string) (sent bool, pending int, err error) {
	rows, err := tx.Query(ctx, `
		select operation, status, receiver_may_hold from announcement_channel_handoffs
		where announcement_id = $1 for update
	`, announcementID)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var operation, status string
		var mayHold bool
		if err := rows.Scan(&operation, &status, &mayHold); err != nil {
			return false, 0, err
		}
		switch {
		case mayHold || operation != handoffOperationPublish:
			sent = true
		case status == handoffStatusPending:
			pending++
		case status == handoffStatusCancelled || status == handoffStatusRejected || status == handoffStatusExpired:
		default:
			sent = true
		}
	}
	return sent, pending, rows.Err()
}

func announcementApproved(status string) bool {
	return status == announcementStatusScheduled || status == announcementStatusDispatching || status == announcementStatusDispatched
}

// handoffDeliverAt keeps the channel post on the approved schedule; an
// announcement already past its time is delivered as soon as accepted.
func handoffDeliverAt(scheduledFor sql.NullTime, now time.Time) time.Time {
	if scheduledFor.Valid && scheduledFor.Time.After(now) {
		return scheduledFor.Time.UTC().Truncate(time.Second)
	}
	return now.UTC().Truncate(time.Second)
}

// handleBacklinePreview shows exactly what a handoff would carry and why it
// cannot be sent yet, without creating anything.
func (a *App) handleBacklinePreview(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if _, ok := a.requirePermission(r, workspaceID, permManageAnnouncements); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	announcementID := r.PathValue("announcementID")
	if _, err := uuid.Parse(announcementID); err != nil {
		writeError(w, http.StatusNotFound, "announcement not found")
		return
	}
	var subject, body, status string
	var scheduledFor sql.NullTime
	err := a.db.QueryRow(r.Context(), `select subject, body, status, scheduled_for from announcements where id = $1 and workspace_id = $2`,
		announcementID, workspaceID).Scan(&subject, &body, &status, &scheduledFor)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "announcement not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load announcement")
		return
	}
	deliverAt := handoffDeliverAt(scheduledFor, time.Now())
	preview := backlinePreviewDTO{
		AnnouncementID: announcementID, AnnouncementState: status, Approved: announcementApproved(status),
		Subject: subject, Body: body, ContentRevision: announcementContentRevision(subject, body),
		ContentLength: announcementChannelContentLength(subject, body), ContentLimit: backlineMaxContentRunes,
		MentionPolicy: backlineMentionPolicyNone, DeliverAt: deliverAt.Format(time.RFC3339),
		ExpiresAt: deliverAt.Add(backlineDeliveryWindow).Format(time.RFC3339), AdapterConfigured: a.config.backlineHandoffConfigured(),
		Blockers: []string{},
	}
	binding, err := scanChannelBinding(a.db.QueryRow(r.Context(), `
		select `+bindingColumns+` from announcement_channel_bindings
		where workspace_id = $1 and channel = $2 and revoked_at is null`, workspaceID, channelBacklineName))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not load binding")
		return
	default:
		preview.Binding = &binding
		preview.MentionPolicy = binding.MentionPolicy
		handoff, err := scanChannelHandoff(a.db.QueryRow(r.Context(), `
			select `+handoffColumns+`
			from announcement_channel_handoffs h join announcement_channel_bindings b on b.id = h.binding_id
			where h.announcement_id = $1 and h.binding_id = $2`, announcementID, binding.ID))
		if err == nil {
			preview.Handoff = &handoff
		} else if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "could not load handoff")
			return
		}
	}
	if !preview.Approved {
		preview.Blockers = append(preview.Blockers, "announcement_not_approved")
	}
	if preview.Binding == nil {
		preview.Blockers = append(preview.Blockers, "no_active_binding")
	}
	if preview.ContentLength > backlineMaxContentRunes {
		preview.Blockers = append(preview.Blockers, "content_too_long")
	}
	if !preview.AdapterConfigured {
		preview.Blockers = append(preview.Blockers, "adapter_not_configured")
	}
	if preview.Handoff != nil {
		preview.Blockers = append(preview.Blockers, "already_handed_off")
	}
	writeJSON(w, http.StatusOK, preview)
}

type createBacklineHandoffRequest struct {
	BindingID string `json:"bindingId"`
}

// handleCreateBacklineHandoff is the deliberate publish. It reuses the
// announcement's own approval (scheduling by manage_announcements) and
// creates exactly one pending handoff per (announcement, binding).
func (a *App) handleCreateBacklineHandoff(w http.ResponseWriter, r *http.Request) {
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
	var req createBacklineHandoffRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, err := uuid.Parse(announcementID); err != nil {
		writeError(w, http.StatusNotFound, "announcement not found")
		return
	}
	if _, err := uuid.Parse(req.BindingID); err != nil {
		writeError(w, http.StatusBadRequest, "bindingId is required")
		return
	}
	if !a.config.backlineHandoffConfigured() {
		writeError(w, http.StatusConflict, "Backline delivery is not configured")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create handoff")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var subject, body, status string
	var scheduledFor sql.NullTime
	err = tx.QueryRow(r.Context(), `select subject, body, status, scheduled_for from announcements where id = $1 and workspace_id = $2 for update`,
		announcementID, workspaceID).Scan(&subject, &body, &status, &scheduledFor)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "announcement not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load announcement")
		return
	}
	if !announcementApproved(status) {
		writeError(w, http.StatusConflict, "only a scheduled or dispatched announcement can be handed off")
		return
	}
	// Scoping the lookup to this workspace rejects another workspace's binding.
	var revoked bool
	err = tx.QueryRow(r.Context(), `select revoked_at is not null from announcement_channel_bindings where id = $1 and workspace_id = $2 and channel = $3 for share`,
		req.BindingID, workspaceID, channelBacklineName).Scan(&revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load binding")
		return
	}
	if revoked {
		writeError(w, http.StatusConflict, "binding is revoked")
		return
	}
	if announcementChannelContentLength(subject, body) > backlineMaxContentRunes {
		writeError(w, http.StatusUnprocessableEntity, "announcement is longer than the Backline channel limit")
		return
	}
	deliverAt := handoffDeliverAt(scheduledFor, time.Now())
	var handoffID string
	err = tx.QueryRow(r.Context(), `
		insert into announcement_channel_handoffs
			(announcement_id, workspace_id, binding_id, occurrence_key, content_revision, deliver_at, expires_at, created_by_person_id)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (announcement_id, binding_id) do nothing
		returning id::text
	`, announcementID, workspaceID, req.BindingID, backlineOccurrenceKey(announcementID, req.BindingID),
		announcementContentRevision(subject, body), deliverAt, deliverAt.Add(backlineDeliveryWindow), actorID).Scan(&handoffID)
	created := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "could not create handoff")
		return
	}
	if created {
		txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
		if err := a.audit(txCtx, actorID, "announcement.channel_handoff.created", "announcement_channel_handoff", handoffID, map[string]any{
			"workspaceId": workspaceID, "announcementId": announcementID, "bindingId": req.BindingID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "could not record audit")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create handoff")
		return
	}
	dto, err := scanChannelHandoff(a.db.QueryRow(r.Context(), `
		select `+handoffColumns+`
		from announcement_channel_handoffs h join announcement_channel_bindings b on b.id = h.binding_id
		where h.announcement_id = $1 and h.binding_id = $2`, announcementID, req.BindingID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load handoff")
		return
	}
	if created {
		writeJSON(w, http.StatusCreated, dto)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// handleRetryChannelHandoff re-queues a held uncertain or failed handoff. The
// occurrence key and revision are unchanged, so the receiver deduplicates.
func (a *App) handleRetryChannelHandoff(w http.ResponseWriter, r *http.Request) {
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
	announcementID, handoffID := r.PathValue("announcementID"), r.PathValue("handoffID")
	if _, err := uuid.Parse(announcementID); err != nil {
		writeError(w, http.StatusNotFound, "handoff not found")
		return
	}
	if _, err := uuid.Parse(handoffID); err != nil {
		writeError(w, http.StatusNotFound, "handoff not found")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not retry handoff")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	result, err := tx.Exec(r.Context(), `
		update announcement_channel_handoffs h
		set status = 'pending', attempt_count = 0, next_attempt_at = now(), last_error = null, updated_at = now()
		from announcement_channel_bindings b
		where h.id = $1 and h.announcement_id = $2 and h.workspace_id = $3 and b.id = h.binding_id
		  and h.status in ('uncertain', 'failed')
		  and (h.operation = 'cancel' or b.revoked_at is null)
	`, handoffID, announcementID, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not retry handoff")
		return
	}
	if result.RowsAffected() == 0 {
		var exists bool
		_ = tx.QueryRow(r.Context(), `select exists(select 1 from announcement_channel_handoffs where id = $1 and announcement_id = $2 and workspace_id = $3)`,
			handoffID, announcementID, workspaceID).Scan(&exists)
		if exists {
			writeError(w, http.StatusConflict, "only an uncertain or failed handoff can be retried")
			return
		}
		writeError(w, http.StatusNotFound, "handoff not found")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "announcement.channel_handoff.retried", "announcement_channel_handoff", handoffID, map[string]any{"workspaceId": workspaceID}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not retry handoff")
		return
	}
	dto, err := scanChannelHandoff(a.db.QueryRow(r.Context(), `
		select `+handoffColumns+`
		from announcement_channel_handoffs h join announcement_channel_bindings b on b.id = h.binding_id
		where h.id = $1`, handoffID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load handoff")
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// handleBacklineReceipt records the receiver's later channel-delivery
// outcome for one occurrence key. It is authenticated with the shared secret
// under the receipt signature scope.
func (a *App) handleBacklineReceipt(w http.ResponseWriter, r *http.Request) {
	if !a.config.backlineHandoffConfigured() || a.db == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "receipt body too large")
		return
	}
	if verifyBacklineSignature(a.config.BacklineHandoffSecret, backlineSignatureScopeReceipt, r.Header, body, time.Now()) != nil {
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}
	var receipt backlineReceipt
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || receipt.Version != backlineHandoffVersion || receipt.Kind != backlineReceiptKind ||
		len(receipt.ReceiverReference) > 128 {
		writeError(w, http.StatusBadRequest, "invalid receipt")
		return
	}
	switch receipt.Outcome {
	case "delivered", "failed", "uncertain", "cancelled":
	default:
		writeError(w, http.StatusBadRequest, "invalid receipt")
		return
	}
	var revision string
	var current sql.NullString
	err = a.db.QueryRow(r.Context(), `select content_revision, delivery_outcome from announcement_channel_handoffs where occurrence_key = $1`,
		receipt.OccurrenceKey).Scan(&revision, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "unknown occurrence")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record receipt")
		return
	}
	if revision != receipt.ContentRevision {
		writeError(w, http.StatusConflict, "content revision mismatch")
		return
	}
	if current.Valid && current.String == receipt.Outcome {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unchanged"})
		return
	}
	// Only an uncertain outcome may later resolve; delivered, failed and
	// cancelled are final. A final receipt proves Backline holds the
	// occurrence, so an uncertain or failed transport state resolves to
	// accepted and is no longer offered for retry. For a cancel message the
	// receipt also says whether the post went out first.
	result, err := a.db.Exec(r.Context(), `
		update announcement_channel_handoffs
		set delivery_outcome = $2, delivery_outcome_at = now(),
		    receiver_reference = coalesce(nullif($3, ''), receiver_reference),
		    receiver_may_hold = true,
		    receiver_status = case
		      when $2 <> 'uncertain' and status in ('uncertain', 'failed') and operation = 'cancel'
		        then case when $2 = 'delivered' then 'already_delivered' else 'cancelled' end
		      else receiver_status end,
		    last_error = case when $2 <> 'uncertain' and status in ('uncertain', 'failed') then null else last_error end,
		    acknowledged_at = case when $2 <> 'uncertain' and status in ('uncertain', 'failed') then coalesce(acknowledged_at, now()) else acknowledged_at end,
		    status = case when $2 <> 'uncertain' and status in ('uncertain', 'failed') then 'accepted' else status end,
		    updated_at = now()
		where occurrence_key = $1 and (delivery_outcome is null or delivery_outcome = 'uncertain')
	`, receipt.OccurrenceKey, receipt.Outcome, receipt.ReceiverReference)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record receipt")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "outcome already final")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}
