package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SIGNAL-01 (issue #24): one scoped announcement channel and delivery
// worker, built entirely on the existing CONSENT-01 (consent.md) and
// transactional-email (transactional-email.md) machinery. See
// docs/development/announcements.md for the channel decision, the
// dispatch algorithm and the withdraw-link design.

const (
	announcementStatusDraft       = "draft"
	announcementStatusScheduled   = "scheduled"
	announcementStatusCancelled   = "cancelled"
	announcementStatusDispatching = "dispatching"
	announcementStatusDispatched  = "dispatched"
)

var errAnnouncementNotFound = errors.New("announcement not found")

type announcementDTO struct {
	ID                   string         `json:"id"`
	Subject              string         `json:"subject"`
	Body                 string         `json:"body"`
	Status               string         `json:"status"`
	ScheduledFor         *string        `json:"scheduledFor,omitempty"`
	CancelledAt          *string        `json:"cancelledAt,omitempty"`
	DispatchedAt         *string        `json:"dispatchedAt,omitempty"`
	RecipientCount       int            `json:"recipientCount"`
	WithheldCount        int            `json:"withheldCount"`
	EstimatedCostCents   int            `json:"estimatedCostCents"`
	CreatedAt            string         `json:"createdAt"`
	UpdatedAt            string         `json:"updatedAt"`
	DeliveryStatusCounts map[string]int `json:"deliveryStatusCounts"`
	// ChannelHandoffs is the Backline half of the outcome ledger (#221).
	ChannelHandoffs []channelHandoffDTO `json:"channelHandoffs"`
}

type announcementPreviewDTO struct {
	Subject            string `json:"subject"`
	Body               string `json:"body"`
	RecipientCount     int    `json:"recipientCount"`
	EstimatedCostCents int    `json:"estimatedCostCents"`
}

// announcementAudienceCount counts the workspace's verified, unwithdrawn
// announcement-purpose email grants whose address is not currently
// suppressed. It is the same query dispatch uses to derive the actual
// audience (see RunAnnouncementDispatch), so preview and dispatch never
// disagree about who counts as an eligible recipient. It never returns
// the addresses themselves.
func (a *App) announcementAudienceCount(ctx context.Context, workspaceID string) (int, error) {
	var count int
	err := a.db.QueryRow(ctx, `
		select count(*) from consent_grants g
		where g.workspace_id = $1
		  and g.channel = 'email'
		  and g.purpose = $2
		  and g.verified_at is not null
		  and g.withdrawn_at is null
		  and not exists (
		    select 1 from email_suppressions s where s.recipient_email = g.recipient_address
		  )
	`, workspaceID, consentPurposeAnnouncement).Scan(&count)
	return count, err
}

type createAnnouncementRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// handleCreateAnnouncement drafts a new announcement. Drafting never
// resolves an audience or enqueues anything; that happens at schedule
// time (preview) and dispatch time (send), respectively.
func (a *App) handleCreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	actorID, ok := a.requirePermission(r, workspaceID, permManageAnnouncements)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req createAnnouncementRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	subject := strings.TrimSpace(req.Subject)
	body := strings.TrimSpace(req.Body)
	if subject == "" {
		writeError(w, http.StatusBadRequest, "subject is required")
		return
	}
	if body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	var id string
	if err := a.db.QueryRow(r.Context(), `
		insert into announcements (workspace_id, subject, body, created_by_person_id)
		values ($1, $2, $3, $4)
		returning id::text
	`, workspaceID, subject, body, actorID).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create announcement")
		return
	}

	dto, err := a.loadAnnouncementDTO(r.Context(), workspaceID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load announcement")
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// handlePreviewAnnouncement returns the subject, body, current eligible
// recipient count and estimated cost, without ever returning an address.
func (a *App) handlePreviewAnnouncement(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if _, ok := a.requirePermission(r, workspaceID, permManageAnnouncements); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	announcementID := r.PathValue("announcementID")

	var subject, body string
	if err := a.db.QueryRow(r.Context(), `
		select subject, body from announcements where id = $1 and workspace_id = $2
	`, announcementID, workspaceID).Scan(&subject, &body); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "announcement not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load announcement")
		return
	}

	count, err := a.announcementAudienceCount(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not compute announcement audience")
		return
	}

	writeJSON(w, http.StatusOK, announcementPreviewDTO{
		Subject:            subject,
		Body:               body,
		RecipientCount:     count,
		EstimatedCostCents: count * a.config.AnnouncementUnitCostCents,
	})
}

type scheduleAnnouncementRequest struct {
	ScheduledFor string `json:"scheduledFor"`
}

// handleScheduleAnnouncement requires an explicit future scheduledFor and
// moves the announcement from draft (or an already-scheduled row, to
// reschedule) to scheduled. It never resolves the audience itself: that
// happens again, freshly, at dispatch time, so a grant withdrawn between
// scheduling and dispatch is never counted.
func (a *App) handleScheduleAnnouncement(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if _, ok := a.requirePermission(r, workspaceID, permManageAnnouncements); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	announcementID := r.PathValue("announcementID")

	var req scheduleAnnouncementRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	scheduledFor, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ScheduledFor))
	if err != nil {
		writeError(w, http.StatusBadRequest, "scheduledFor must be an RFC3339 timestamp")
		return
	}
	if !scheduledFor.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "scheduledFor must be in the future")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule announcement")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	result, err := tx.Exec(r.Context(), `
		update announcements set status = $3, scheduled_for = $4, updated_at = now()
		where id = $1 and workspace_id = $2 and status in ($5, $3)
	`, announcementID, workspaceID, announcementStatusScheduled, scheduledFor.UTC(), announcementStatusDraft)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule announcement")
		return
	}
	if result.RowsAffected() == 1 {
		// A Backline handoff the receiver may already hold keeps its time;
		// one it has not seen follows the new schedule.
		var sent bool
		if err := tx.QueryRow(r.Context(), `
			select exists(select 1 from announcement_channel_handoffs where announcement_id = $1
			  and not (operation = 'publish' and status in ('pending', 'cancelled', 'rejected', 'expired')))
		`, announcementID).Scan(&sent); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule announcement")
			return
		}
		if sent {
			writeError(w, http.StatusConflict, "announcement was already handed off to Backline; cancel it and draft a new one")
			return
		}
		if _, err := tx.Exec(r.Context(), `
			update announcement_channel_handoffs
			set deliver_at = $2, expires_at = $2::timestamptz + make_interval(secs => $3), updated_at = now()
			where announcement_id = $1 and operation = 'publish' and status = 'pending'
		`, announcementID, scheduledFor.UTC().Truncate(time.Second), backlineDeliveryWindow.Seconds()); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule announcement")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule announcement")
			return
		}
	}
	if result.RowsAffected() == 0 {
		if _, loadErr := a.loadAnnouncementDTO(r.Context(), workspaceID, announcementID); errors.Is(loadErr, errAnnouncementNotFound) {
			writeError(w, http.StatusNotFound, "announcement not found")
			return
		}
		writeError(w, http.StatusConflict, "announcement cannot be scheduled from its current status")
		return
	}

	dto, err := a.loadAnnouncementDTO(r.Context(), workspaceID, announcementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load announcement")
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// handleCancelAnnouncement is allowed while draft or scheduled, and never
// after dispatch has started (dispatching or dispatched): once dispatch
// has claimed the row, cancellation can no longer prevent anything from
// being enqueued, and the delivery ledger becomes the source of truth.
func (a *App) handleCancelAnnouncement(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	actorID, ok := a.requirePermission(r, workspaceID, permManageAnnouncements)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	announcementID := r.PathValue("announcementID")

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel announcement")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	result, err := tx.Exec(r.Context(), `
		update announcements
		set status = $3, cancelled_at = now(), cancelled_by_person_id = $4, updated_at = now()
		where id = $1 and workspace_id = $2 and status in ($5, $6)
	`, announcementID, workspaceID, announcementStatusCancelled, actorID, announcementStatusDraft, announcementStatusScheduled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel announcement")
		return
	}
	if result.RowsAffected() == 1 {
		// Withdraw Backline handoffs in the same transaction: unsent ones are
		// cancelled locally, sent ones become cancel messages.
		if err := requestHandoffCancellation(r.Context(), tx, "announcement_id", announcementID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not cancel announcement")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "could not cancel announcement")
			return
		}
	}
	if result.RowsAffected() == 0 {
		if _, loadErr := a.loadAnnouncementDTO(r.Context(), workspaceID, announcementID); errors.Is(loadErr, errAnnouncementNotFound) {
			writeError(w, http.StatusNotFound, "announcement not found")
			return
		}
		writeError(w, http.StatusConflict, "announcement can only be cancelled while draft or scheduled")
		return
	}

	dto, err := a.loadAnnouncementDTO(r.Context(), workspaceID, announcementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load announcement")
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (a *App) handleListAnnouncements(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if _, ok := a.requirePermission(r, workspaceID, permManageAnnouncements); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := a.db.Query(r.Context(), `
		select id::text from announcements where workspace_id = $1 order by created_at desc
	`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load announcements")
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "could not load announcements")
			return
		}
		ids = append(ids, id)
	}
	closeErr := rows.Err()
	rows.Close()
	if closeErr != nil {
		writeError(w, http.StatusInternalServerError, "could not load announcements")
		return
	}

	out := make([]announcementDTO, 0, len(ids))
	for _, id := range ids {
		dto, err := a.loadAnnouncementDTO(r.Context(), workspaceID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load announcement")
			return
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) handleGetAnnouncement(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if _, ok := a.requirePermission(r, workspaceID, permManageAnnouncements); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	dto, err := a.loadAnnouncementDTO(r.Context(), workspaceID, r.PathValue("announcementID"))
	if err != nil {
		if errors.Is(err, errAnnouncementNotFound) {
			writeError(w, http.StatusNotFound, "announcement not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load announcement")
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// loadAnnouncementDTO loads one announcement scoped to workspaceID, plus
// its delivery outcome counts joined from email_outbox.delivery_status via
// announcement_deliveries — counts only, never a recipient address.
func (a *App) loadAnnouncementDTO(ctx context.Context, workspaceID, announcementID string) (announcementDTO, error) {
	var dto announcementDTO
	var scheduledFor, cancelledAt, dispatchedAt sql.NullTime
	var createdAt, updatedAt time.Time
	err := a.db.QueryRow(ctx, `
		select id::text, subject, body, status, scheduled_for, cancelled_at, dispatched_at,
		       recipient_count, withheld_count, estimated_cost_cents, created_at, updated_at
		from announcements where id = $1 and workspace_id = $2
	`, announcementID, workspaceID).Scan(
		&dto.ID, &dto.Subject, &dto.Body, &dto.Status, &scheduledFor, &cancelledAt, &dispatchedAt,
		&dto.RecipientCount, &dto.WithheldCount, &dto.EstimatedCostCents, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return announcementDTO{}, errAnnouncementNotFound
		}
		return announcementDTO{}, err
	}
	dto.ScheduledFor = formatNullableTime(scheduledFor)
	dto.CancelledAt = formatNullableTime(cancelledAt)
	dto.DispatchedAt = formatNullableTime(dispatchedAt)
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	dto.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)

	dto.DeliveryStatusCounts = map[string]int{}
	rows, err := a.db.Query(ctx, `
		select o.delivery_status, count(*)
		from announcement_deliveries d
		join email_outbox o on o.id = d.outbox_id
		where d.announcement_id = $1
		group by o.delivery_status
	`, announcementID)
	if err != nil {
		return announcementDTO{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return announcementDTO{}, err
		}
		dto.DeliveryStatusCounts[status] = count
	}
	if err := rows.Err(); err != nil {
		return announcementDTO{}, err
	}
	rows.Close()
	if dto.ChannelHandoffs, err = a.loadAnnouncementHandoffs(ctx, announcementID); err != nil {
		return announcementDTO{}, err
	}
	return dto, nil
}
