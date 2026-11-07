package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type createEventStaffingRequest struct {
	Title                   string  `json:"title"`
	Kind                    string  `json:"kind"`
	Notes                   string  `json:"notes"`
	ParticipantRequirements string  `json:"participantRequirements"`
	StartsAt                *string `json:"startsAt"`
	EndsAt                  *string `json:"endsAt"`
}

type updateEventStaffingRequest struct {
	Title                   *string   `json:"title"`
	Notes                   *string   `json:"notes"`
	ParticipantRequirements *string   `json:"participantRequirements"`
	StartsAt                *string   `json:"startsAt"`
	ClearStartsAt           bool      `json:"clearStartsAt"`
	EndsAt                  *string   `json:"endsAt"`
	ClearEndsAt             bool      `json:"clearEndsAt"`
	AssignedPersonID        *string   `json:"assignedPersonId"`
	AssignedApplicationID   *string   `json:"assignedApplicationId"`
	ClearAssignee           bool      `json:"clearAssignee"`
	Status                  *string   `json:"status"`
	Capacity                *int      `json:"capacity"`
	ClaimsOpen              *bool     `json:"claimsOpen"`
	ReleaseCutoffHours      *int      `json:"releaseCutoffHours"`
	ClaimRoleIDs            *[]string `json:"claimRoleIds"`
}

type eventStaffingItemDTO struct {
	ID                      string             `json:"id"`
	EventID                 string             `json:"eventId"`
	Title                   string             `json:"title"`
	Kind                    string             `json:"kind"`
	Notes                   string             `json:"notes"`
	ParticipantRequirements string             `json:"participantRequirements"`
	StartsAt                *string            `json:"startsAt,omitempty"`
	EndsAt                  *string            `json:"endsAt,omitempty"`
	AssignedPersonID        *string            `json:"assignedPersonId,omitempty"`
	AssignedApplicationID   *string            `json:"assignedApplicationId,omitempty"`
	AssigneeName            *string            `json:"assigneeName,omitempty"`
	Status                  string             `json:"status"`
	CreatedAt               string             `json:"createdAt"`
	UpdatedAt               string             `json:"updatedAt"`
	CompletedAt             *string            `json:"completedAt,omitempty"`
	CompletedByPersonID     *string            `json:"completedByPersonId,omitempty"`
	Capacity                int                `json:"capacity"`
	ClaimsOpen              bool               `json:"claimsOpen"`
	ReleaseCutoffHours      int                `json:"releaseCutoffHours"`
	ClaimRoleIDs            []string           `json:"claimRoleIds"`
	FilledCount             int                `json:"filledCount"`
	Claims                  []staffingClaimDTO `json:"claims"`
}

type eventStaffingItemRow struct {
	ID                      string
	EventID                 string
	Title                   string
	Kind                    string
	Notes                   string
	ParticipantRequirements string
	StartsAt                sql.NullTime
	EndsAt                  sql.NullTime
	AssignedPersonID        sql.NullString
	AssignedApplicationID   sql.NullString
	AssigneeName            sql.NullString
	Status                  string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	CompletedAt             sql.NullTime
	CompletedByPersonID     sql.NullString
	Capacity                int
	ClaimsOpen              bool
	ReleaseCutoffHours      int
}

type staffingRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (a *App) handleListEventStaffing(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	event, err := a.loadEventDetails(r.Context(), r.PathValue("eventID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	_, role, ok := a.requireWorkspaceRole(r, event.WorkspaceID, "owner", "member")
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := a.db.Query(r.Context(), `
		select esi.id, esi.event_id, esi.title, esi.kind, esi.notes, esi.participant_requirements, esi.starts_at, esi.ends_at,
		       esi.assigned_person_id, esi.assigned_application_id,
		       coalesce(nullif(trim(p.display_name), ''), p.email, era.applicant_name) as assignee_name,
		       esi.status, esi.created_at, esi.updated_at, esi.completed_at, esi.completed_by_person_id,
		       esi.capacity, esi.claims_open, esi.release_cutoff_hours
		from event_staffing_items esi
		left join people p on p.id = esi.assigned_person_id
		left join event_role_applications era on era.id = esi.assigned_application_id
		where esi.event_id = $1
		order by case esi.status
			when 'open' then 0
			when 'assigned' then 1
			when 'completed' then 2
			when 'cancelled' then 3
			else 4
		end, esi.starts_at nulls last, esi.created_at, esi.id
	`, event.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing")
		return
	}
	defer rows.Close()

	items := make([]eventStaffingItemDTO, 0)
	for rows.Next() {
		var row eventStaffingItemRow
		if err := rows.Scan(&row.ID, &row.EventID, &row.Title, &row.Kind, &row.Notes, &row.ParticipantRequirements, &row.StartsAt, &row.EndsAt, &row.AssignedPersonID, &row.AssignedApplicationID, &row.AssigneeName, &row.Status, &row.CreatedAt, &row.UpdatedAt, &row.CompletedAt, &row.CompletedByPersonID, &row.Capacity, &row.ClaimsOpen, &row.ReleaseCutoffHours); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load staffing")
			return
		}
		items = append(items, eventStaffingItemDTOFromRow(row))
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing")
		return
	}
	rows.Close()
	// Only owners manage staffing, so only they see who holds each place.
	if err := attachStaffingClaims(r.Context(), a.db, event.ID, items, role == roleOwner); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleCreateEventStaffing(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	actorID, ok := a.requirePersonID(r)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var lockedEventID, workspaceID, eventStatus string
	if err := tx.QueryRow(r.Context(), `
		select id, workspace_id, status
		from events
		where id = $1
		for update
	`, r.PathValue("eventID")).Scan(&lockedEventID, &workspaceID, &eventStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not lock event")
		return
	}
	var membershipRole string
	if err := tx.QueryRow(r.Context(), `
		select role
		from workspace_members
		where workspace_id = $1
		  and person_id = $2
		  and removed_at is null
	`, workspaceID, actorID).Scan(&membershipRole); err != nil || membershipRole != "owner" {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if eventStatusIsClosed(eventStatus) {
		writeError(w, http.StatusConflict, "event is closed")
		return
	}

	var req createEventStaffingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	title := strings.TrimSpace(req.Title)
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	notes := strings.TrimSpace(req.Notes)
	participantRequirements := strings.TrimSpace(req.ParticipantRequirements)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	switch kind {
	case "task", "shift":
	default:
		writeError(w, http.StatusBadRequest, "kind must be task or shift")
		return
	}
	if utf8.RuneCountInString(notes) > 2000 {
		writeError(w, http.StatusBadRequest, "notes must be 2000 characters or fewer")
		return
	}
	if utf8.RuneCountInString(participantRequirements) > 2000 {
		writeError(w, http.StatusBadRequest, "participantRequirements must be 2000 characters or fewer")
		return
	}
	startsAt, err := parseOptionalRFC3339Time(req.StartsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid startsAt")
		return
	}
	endsAt, err := parseOptionalRFC3339Time(req.EndsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endsAt")
		return
	}
	if startsAt != nil && endsAt != nil && endsAt.Before(*startsAt) {
		writeError(w, http.StatusBadRequest, "endsAt must be greater than or equal to startsAt")
		return
	}

	var startsAtArg any
	if startsAt != nil {
		startsAtArg = *startsAt
	}
	var endsAtArg any
	if endsAt != nil {
		endsAtArg = *endsAt
	}

	var row eventStaffingItemRow
	if err := tx.QueryRow(r.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, participant_requirements, starts_at, ends_at, status, created_by_person_id
		)
		values ($1, $2, $3, $4, $5, $6, $7, 'open', $8)
		returning id, event_id, title, kind, notes, participant_requirements, starts_at, ends_at,
		          assigned_person_id, assigned_application_id,
		          null as assignee_name,
		          status, created_at, updated_at, completed_at, completed_by_person_id,
		          capacity, claims_open, release_cutoff_hours
	`, lockedEventID, title, kind, notes, participantRequirements, startsAtArg, endsAtArg, actorID).Scan(
		&row.ID, &row.EventID, &row.Title, &row.Kind, &row.Notes, &row.ParticipantRequirements, &row.StartsAt, &row.EndsAt,
		&row.AssignedPersonID, &row.AssignedApplicationID, &row.AssigneeName, &row.Status, &row.CreatedAt, &row.UpdatedAt, &row.CompletedAt, &row.CompletedByPersonID,
		&row.Capacity, &row.ClaimsOpen, &row.ReleaseCutoffHours,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create staffing item")
		return
	}

	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "staffing.created", "event_staffing_item", row.ID, map[string]any{
		"eventId":        row.EventID,
		"staffingItemId": row.ID,
		"kind":           row.Kind,
		"status":         row.Status,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save staffing item")
		return
	}

	writeJSON(w, http.StatusOK, eventStaffingItemDTOFromRow(row))
}

func (a *App) handleUpdateEventStaffing(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	actorID, ok := a.requirePersonID(r)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req updateEventStaffingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.ClearStartsAt && req.StartsAt != nil {
		writeError(w, http.StatusBadRequest, "clearStartsAt conflicts with startsAt")
		return
	}
	if req.ClearEndsAt && req.EndsAt != nil {
		writeError(w, http.StatusBadRequest, "clearEndsAt conflicts with endsAt")
		return
	}
	if req.ClearAssignee && (req.AssignedPersonID != nil || req.AssignedApplicationID != nil) {
		writeError(w, http.StatusBadRequest, "clearAssignee conflicts with assignee")
		return
	}
	if req.AssignedPersonID != nil && req.AssignedApplicationID != nil {
		writeError(w, http.StatusBadRequest, "only one assignee source may be set")
		return
	}

	var requestedTitle *string
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "title is required")
			return
		}
		requestedTitle = &title
	}
	var requestedNotes *string
	if req.Notes != nil {
		notes := strings.TrimSpace(*req.Notes)
		if utf8.RuneCountInString(notes) > 2000 {
			writeError(w, http.StatusBadRequest, "notes must be 2000 characters or fewer")
			return
		}
		requestedNotes = &notes
	}
	var requestedParticipantRequirements *string
	if req.ParticipantRequirements != nil {
		requirements := strings.TrimSpace(*req.ParticipantRequirements)
		if utf8.RuneCountInString(requirements) > 2000 {
			writeError(w, http.StatusBadRequest, "participantRequirements must be 2000 characters or fewer")
			return
		}
		requestedParticipantRequirements = &requirements
	}
	startsAt, err := parseOptionalRFC3339Time(req.StartsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid startsAt")
		return
	}
	endsAt, err := parseOptionalRFC3339Time(req.EndsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endsAt")
		return
	}
	if startsAt != nil && endsAt != nil && endsAt.Before(*startsAt) {
		writeError(w, http.StatusBadRequest, "endsAt must be greater than or equal to startsAt")
		return
	}
	var requestedPersonID *string
	if req.AssignedPersonID != nil {
		personID := strings.TrimSpace(*req.AssignedPersonID)
		if personID == "" {
			writeError(w, http.StatusBadRequest, "assignedPersonId is required")
			return
		}
		requestedPersonID = &personID
	}
	var requestedApplicationID *string
	if req.AssignedApplicationID != nil {
		applicationID := strings.TrimSpace(*req.AssignedApplicationID)
		if applicationID == "" {
			writeError(w, http.StatusBadRequest, "assignedApplicationId is required")
			return
		}
		requestedApplicationID = &applicationID
	}
	var requestedStatus *string
	if req.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*req.Status))
		switch status {
		case "open", "assigned", "completed", "cancelled":
			requestedStatus = &status
		default:
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
	}
	if req.Capacity != nil && (*req.Capacity < 1 || *req.Capacity > maxStaffingCapacity) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("capacity must be between 1 and %d", maxStaffingCapacity))
		return
	}
	if req.ReleaseCutoffHours != nil && (*req.ReleaseCutoffHours < 0 || *req.ReleaseCutoffHours > maxStaffingReleaseCutoffHours) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("releaseCutoffHours must be between 0 and %d", maxStaffingReleaseCutoffHours))
		return
	}
	var requestedRoleIDs []string
	if req.ClaimRoleIDs != nil {
		requestedRoleIDs = make([]string, 0, len(*req.ClaimRoleIDs))
		seen := make(map[string]struct{}, len(*req.ClaimRoleIDs))
		for _, raw := range *req.ClaimRoleIDs {
			roleID := strings.ToLower(strings.TrimSpace(raw))
			if !validUUID(roleID) {
				writeError(w, http.StatusBadRequest, "claimRoleIds must be roles of this event")
				return
			}
			if _, ok := seen[roleID]; ok {
				continue
			}
			seen[roleID] = struct{}{}
			requestedRoleIDs = append(requestedRoleIDs, roleID)
		}
		if len(requestedRoleIDs) > 50 {
			writeError(w, http.StatusBadRequest, "claimRoleIds must contain 50 roles or fewer")
			return
		}
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var lockedEventID, workspaceID, eventTitle, eventStatus string
	if err := tx.QueryRow(r.Context(), `
		select id, workspace_id, title, status
		from events
		where id = $1
		for update
	`, r.PathValue("eventID")).Scan(&lockedEventID, &workspaceID, &eventTitle, &eventStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not lock event")
		return
	}
	var membershipRole string
	if err := tx.QueryRow(r.Context(), `
		select role
		from workspace_members
		where workspace_id = $1
		  and person_id = $2
		  and removed_at is null
	`, workspaceID, actorID).Scan(&membershipRole); err != nil || membershipRole != "owner" {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	itemID := r.PathValue("staffingID")
	current, err := loadEventStaffingItemRow(r.Context(), tx, lockedEventID, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "staffing item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}

	newRow := current
	changed := false
	assignmentChanged := false
	assignmentRecipientEmail := ""

	if requestedTitle != nil && *requestedTitle != current.Title {
		newRow.Title = *requestedTitle
		changed = true
	}
	if requestedNotes != nil && *requestedNotes != current.Notes {
		newRow.Notes = *requestedNotes
		changed = true
	}
	if requestedParticipantRequirements != nil && *requestedParticipantRequirements != current.ParticipantRequirements {
		newRow.ParticipantRequirements = *requestedParticipantRequirements
		changed = true
	}
	if req.ClearStartsAt {
		if current.StartsAt.Valid {
			changed = true
		}
		newRow.StartsAt = sql.NullTime{}
	} else if startsAt != nil {
		candidate := sql.NullTime{Time: *startsAt, Valid: true}
		if !current.StartsAt.Valid || !current.StartsAt.Time.Equal(candidate.Time) {
			changed = true
		}
		newRow.StartsAt = candidate
	}
	if req.ClearEndsAt {
		if current.EndsAt.Valid {
			changed = true
		}
		newRow.EndsAt = sql.NullTime{}
	} else if endsAt != nil {
		candidate := sql.NullTime{Time: *endsAt, Valid: true}
		if !current.EndsAt.Valid || !current.EndsAt.Time.Equal(candidate.Time) {
			changed = true
		}
		newRow.EndsAt = candidate
	}

	if req.ClearAssignee {
		if current.AssignedPersonID.Valid || current.AssignedApplicationID.Valid {
			changed = true
		}
		newRow.AssignedPersonID = sql.NullString{}
		newRow.AssignedApplicationID = sql.NullString{}
	} else if requestedPersonID != nil {
		var assigneeName string
		if err := tx.QueryRow(r.Context(), `
			select coalesce(nullif(trim(p.display_name), ''), p.email), p.email
			from workspace_members wm
			join people p on p.id = wm.person_id
			where wm.workspace_id = $1
			  and wm.person_id = $2
			  and wm.removed_at is null
			  and wm.revoked_at is null
			  and (wm.expires_at is null or wm.expires_at > now())
		`, workspaceID, *requestedPersonID).Scan(&assigneeName, &assignmentRecipientEmail); err != nil {
			writeError(w, http.StatusBadRequest, "assignedPersonId must be an active workspace member")
			return
		}
		_ = assigneeName
		candidate := sql.NullString{String: *requestedPersonID, Valid: true}
		if !current.AssignedPersonID.Valid || current.AssignedPersonID.String != candidate.String || current.AssignedApplicationID.Valid {
			assignmentChanged = true
			changed = true
		}
		newRow.AssignedPersonID = candidate
		newRow.AssignedApplicationID = sql.NullString{}
	} else if requestedApplicationID != nil {
		var assigneeName string
		if err := tx.QueryRow(r.Context(), `
			select applicant_name, applicant_email
			from event_role_applications
			where id = $1
			  and event_id = $2
			  and status in ('accepted', 'confirmed')
		`, *requestedApplicationID, lockedEventID).Scan(&assigneeName, &assignmentRecipientEmail); err != nil {
			writeError(w, http.StatusBadRequest, "assignedApplicationId must be an accepted or confirmed application for this event")
			return
		}
		_ = assigneeName
		candidate := sql.NullString{String: *requestedApplicationID, Valid: true}
		if !current.AssignedApplicationID.Valid || current.AssignedApplicationID.String != candidate.String || current.AssignedPersonID.Valid {
			assignmentChanged = true
			changed = true
		}
		newRow.AssignedApplicationID = candidate
		newRow.AssignedPersonID = sql.NullString{}
	}

	if req.Capacity != nil && *req.Capacity != current.Capacity {
		newRow.Capacity = *req.Capacity
		changed = true
	}
	policyChanged := newRow.Capacity != current.Capacity
	if req.ClaimsOpen != nil && *req.ClaimsOpen != current.ClaimsOpen {
		newRow.ClaimsOpen = *req.ClaimsOpen
		changed = true
	}
	if req.ReleaseCutoffHours != nil && *req.ReleaseCutoffHours != current.ReleaseCutoffHours {
		newRow.ReleaseCutoffHours = *req.ReleaseCutoffHours
		changed = true
	}
	currentRoleIDs, err := loadStaffingClaimRoleIDs(r.Context(), tx, current.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}
	nextRoleIDs := currentRoleIDs
	rolesChanged := false
	if requestedRoleIDs != nil {
		var matched int
		if err := tx.QueryRow(r.Context(), `
			select count(*) from event_roles where event_id = $1 and id = any($2::uuid[])
		`, lockedEventID, requestedRoleIDs).Scan(&matched); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load roles")
			return
		}
		if matched != len(requestedRoleIDs) {
			writeError(w, http.StatusBadRequest, "claimRoleIds must be roles of this event")
			return
		}
		if !sameStringSet(currentRoleIDs, requestedRoleIDs) {
			nextRoleIDs = requestedRoleIDs
			rolesChanged = true
			changed = true
		}
	}
	policyChanged = policyChanged || rolesChanged || newRow.ClaimsOpen != current.ClaimsOpen || newRow.ReleaseCutoffHours != current.ReleaseCutoffHours
	claimCount, err := countActiveStaffingClaims(r.Context(), tx, current.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not count assignments")
		return
	}
	filledOld, filledNew := claimCount, claimCount
	if current.AssignedPersonID.Valid || current.AssignedApplicationID.Valid {
		filledOld++
	}
	if newRow.AssignedPersonID.Valid || newRow.AssignedApplicationID.Valid {
		filledNew++
	}

	if requestedStatus != nil {
		if current.Status != *requestedStatus {
			changed = true
		}
		newRow.Status = *requestedStatus
		if *requestedStatus == "completed" {
			if current.Status != "completed" {
				newRow.CompletedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
				newRow.CompletedByPersonID = sql.NullString{String: actorID, Valid: true}
			}
		} else if current.Status == "completed" {
			if current.CompletedAt.Valid || current.CompletedByPersonID.Valid {
				changed = true
			}
			newRow.CompletedAt = sql.NullTime{}
			newRow.CompletedByPersonID = sql.NullString{}
		}
	} else if (req.ClearAssignee || requestedPersonID != nil || requestedApplicationID != nil || newRow.Capacity != current.Capacity) &&
		(current.Status == "open" || current.Status == "assigned") {
		// Open while places remain, assigned once every place is held. With
		// the legacy capacity of 1 this matches the earlier assign/clear rule.
		fillStatus := "open"
		if filledNew >= newRow.Capacity {
			fillStatus = "assigned"
		}
		if newRow.Status != fillStatus {
			newRow.Status = fillStatus
			changed = true
		}
	}

	if !changed {
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save staffing item")
			return
		}
		items := []eventStaffingItemDTO{eventStaffingItemDTOFromRow(current)}
		if err := attachStaffingClaims(r.Context(), a.db, lockedEventID, items, true); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load staffing item")
			return
		}
		writeJSON(w, http.StatusOK, items[0])
		return
	}
	if eventStatusIsClosed(eventStatus) {
		writeError(w, http.StatusConflict, "event is closed")
		return
	}
	// Cancelling a shift ends its claimed places (below), so they no longer
	// count against its capacity and a reopened shift starts without them.
	cancelling := newRow.Status == "cancelled" && current.Status != "cancelled"
	if cancelling {
		filledNew -= claimCount
	}
	if filledNew > newRow.Capacity && (filledNew > filledOld || newRow.Capacity < current.Capacity) {
		writeError(w, http.StatusConflict, "shift capacity reached")
		return
	}
	// The single assignee is a person directly or through an application bound
	// to their account; either way they cannot hold a second place here or an
	// overlapping one elsewhere on the event.
	newHolderPersonID := ""
	if newRow.AssignedPersonID.Valid {
		newHolderPersonID = newRow.AssignedPersonID.String
	} else if newRow.AssignedApplicationID.Valid {
		var applicantPersonID sql.NullString
		if err := tx.QueryRow(r.Context(), `
			select applicant_person_id from event_role_applications where id = $1
		`, newRow.AssignedApplicationID.String).Scan(&applicantPersonID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not check assignments")
			return
		}
		newHolderPersonID = applicantPersonID.String
	}
	if assignmentChanged && newHolderPersonID != "" {
		var claimHeld bool
		if err := tx.QueryRow(r.Context(), `
			select exists (select 1 from event_staffing_claims where staffing_item_id = $1 and person_id = $2 and status = 'active')
		`, current.ID, newHolderPersonID).Scan(&claimHeld); err != nil {
			writeError(w, http.StatusInternalServerError, "could not check assignments")
			return
		}
		if claimHeld {
			writeError(w, http.StatusConflict, "person already holds a place on this shift")
			return
		}
		if newRow.StartsAt.Valid && newRow.EndsAt.Valid {
			overlaps, err := staffingPersonOverlaps(r.Context(), tx, lockedEventID, current.ID, newHolderPersonID, newRow.StartsAt.Time, newRow.EndsAt.Time)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not check schedule")
				return
			}
			if overlaps {
				writeError(w, http.StatusConflict, "person holds another overlapping shift for this event")
				return
			}
		}
	}
	// Moving a shift, or reopening a completed or cancelled one, must not
	// leave any current holder in two overlapping places. The refusal gives a
	// count only, never who is affected.
	timesChanged := !sameNullTime(current.StartsAt, newRow.StartsAt) || !sameNullTime(current.EndsAt, newRow.EndsAt)
	reopened := !staffingStatusAccepting(current.Status) && staffingStatusAccepting(newRow.Status)
	if (timesChanged || reopened) && staffingStatusAccepting(newRow.Status) && newRow.StartsAt.Valid && newRow.EndsAt.Valid {
		holders, err := staffingHolderPersonIDs(r.Context(), tx, current.ID, newHolderPersonID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not check schedule")
			return
		}
		conflicts := 0
		for _, holderID := range holders {
			overlaps, err := staffingPersonOverlaps(r.Context(), tx, lockedEventID, current.ID, holderID, newRow.StartsAt.Time, newRow.EndsAt.Time)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not check schedule")
				return
			}
			if overlaps {
				conflicts++
			}
		}
		if conflicts > 0 {
			noun := "place holders hold"
			if conflicts == 1 {
				noun = "place holder holds"
			}
			writeError(w, http.StatusConflict, fmt.Sprintf("%d %s another overlapping shift for this event; change their places first", conflicts, noun))
			return
		}
	}
	if newRow.ClaimsOpen && (newRow.Kind != "shift" || !newRow.StartsAt.Valid || !newRow.EndsAt.Valid || len(nextRoleIDs) == 0) {
		writeError(w, http.StatusBadRequest, "open claims require a shift with start and end times and at least one eligible role")
		return
	}

	var startsAtArg any
	if newRow.StartsAt.Valid {
		startsAtArg = newRow.StartsAt.Time
	}
	var endsAtArg any
	if newRow.EndsAt.Valid {
		endsAtArg = newRow.EndsAt.Time
	}
	var assignedPersonArg any
	if newRow.AssignedPersonID.Valid {
		assignedPersonArg = newRow.AssignedPersonID.String
	}
	var assignedApplicationArg any
	if newRow.AssignedApplicationID.Valid {
		assignedApplicationArg = newRow.AssignedApplicationID.String
	}
	var completedAtArg any
	if newRow.CompletedAt.Valid {
		completedAtArg = newRow.CompletedAt.Time
	}
	var completedByArg any
	if newRow.CompletedByPersonID.Valid {
		completedByArg = newRow.CompletedByPersonID.String
	}
	if _, err := tx.Exec(r.Context(), `
		update event_staffing_items
		set title = $3,
		    notes = $4,
		    participant_requirements = $5,
		    starts_at = $6,
		    ends_at = $7,
		    assigned_person_id = $8,
		    assigned_application_id = $9,
		    status = $10,
		    completed_at = $11,
		    completed_by_person_id = $12,
		    capacity = $13,
		    claims_open = $14,
		    release_cutoff_hours = $15,
		    updated_at = now()
		where event_id = $1
		  and id = $2
	`, lockedEventID, current.ID, newRow.Title, newRow.Notes, newRow.ParticipantRequirements, startsAtArg, endsAtArg, assignedPersonArg, assignedApplicationArg, newRow.Status, completedAtArg, completedByArg,
		newRow.Capacity, newRow.ClaimsOpen, newRow.ReleaseCutoffHours); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update staffing item")
		return
	}
	if rolesChanged {
		if _, err := tx.Exec(r.Context(), `delete from event_staffing_claim_roles where staffing_item_id = $1`, current.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update eligible roles")
			return
		}
		if _, err := tx.Exec(r.Context(), `
			insert into event_staffing_claim_roles (staffing_item_id, event_id, role_id)
			select $1, $3, unnest($2::uuid[])
		`, current.ID, nextRoleIDs, lockedEventID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update eligible roles")
			return
		}
	}

	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	endedClaimIDs := []string{}
	if cancelling {
		endedClaimIDs, err = endStaffingClaimsForCancelledShift(txCtx, a, tx, actorID, lockedEventID, current.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not end claimed places")
			return
		}
	}

	updated, err := loadEventStaffingItemRow(r.Context(), tx, lockedEventID, current.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}

	if assignmentChanged && updated.Status == "assigned" {
		recipientEmail := normalizeEmail(assignmentRecipientEmail)
		if recipientEmail != "" {
			params := enqueueNotificationParams{
				WorkspaceID:       workspaceID,
				EventID:           lockedEventID,
				RecipientEmail:    recipientEmail,
				NotificationType:  "staffing.assignment",
				RelatedType:       "event_staffing_item",
				RelatedID:         updated.ID,
				IdempotencyKey:    "staffing_assignment:" + updated.ID + ":" + recipientEmail,
				Subject:           fmt.Sprintf("%s: staffing assignment for %s", eventTitle, updated.Title),
				Body:              fmt.Sprintf("You have been assigned to %s at %s.", updated.Title, eventTitle),
				Preview:           fmt.Sprintf("Assignment for %s", updated.Title),
				CreatedByPersonID: actorID,
			}
			if _, err := a.enqueueNotification(txCtx, tx, params); err != nil {
				writeError(w, http.StatusInternalServerError, "could not enqueue notification")
				return
			}
		}
	}
	auditMetadata := map[string]any{
		"eventId":               lockedEventID,
		"staffingItemId":        updated.ID,
		"previousStatus":        current.Status,
		"status":                updated.Status,
		"assignedPersonId":      nullableString(updated.AssignedPersonID),
		"assignedApplicationId": nullableString(updated.AssignedApplicationID),
	}
	if len(endedClaimIDs) > 0 {
		auditMetadata["endedClaimCount"] = len(endedClaimIDs)
	}
	if policyChanged {
		auditMetadata["capacity"] = updated.Capacity
		auditMetadata["claimsOpen"] = updated.ClaimsOpen
		auditMetadata["releaseCutoffHours"] = updated.ReleaseCutoffHours
		auditMetadata["claimRoleIds"] = nextRoleIDs
	}
	if err := a.audit(txCtx, actorID, "staffing.updated", "event_staffing_item", updated.ID, auditMetadata); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	items := []eventStaffingItemDTO{eventStaffingItemDTOFromRow(updated)}
	if err := attachStaffingClaims(r.Context(), tx, lockedEventID, items, true); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save staffing item")
		return
	}

	writeJSON(w, http.StatusOK, items[0])
}

func sameNullTime(left, right sql.NullTime) bool {
	return left.Valid == right.Valid && (!left.Valid || left.Time.Equal(right.Time))
}

func staffingStatusAccepting(status string) bool {
	return status == "open" || status == "assigned"
}

// staffingHolderPersonIDs lists everyone holding a place on the item: active
// claim holders plus legacyHolderID, the single assignee, when set.
func staffingHolderPersonIDs(ctx context.Context, tx pgx.Tx, itemID, legacyHolderID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select person_id from event_staffing_claims where staffing_item_id = $1 and status = 'active' order by person_id
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	holders := make([]string, 0)
	for rows.Next() {
		var personID string
		if err := rows.Scan(&personID); err != nil {
			return nil, err
		}
		if personID != legacyHolderID {
			holders = append(holders, personID)
		}
	}
	if legacyHolderID != "" {
		holders = append(holders, legacyHolderID)
	}
	return holders, rows.Err()
}

// endStaffingClaimsForCancelledShift ends every active claimed place on a
// shift the organizer is cancelling, recorded as removed by that organizer
// with one audit entry per place. The caller holds the event and item locks.
func endStaffingClaimsForCancelledShift(txCtx context.Context, a *App, tx pgx.Tx, actorID, eventID, itemID string) ([]string, error) {
	rows, err := tx.Query(txCtx, `
		update event_staffing_claims
		set status = 'removed', ended_at = now(), ended_by_person_id = $2
		where staffing_item_id = $1
		  and status = 'active'
		returning id
	`, itemID, actorID)
	if err != nil {
		return nil, err
	}
	ended := make([]string, 0)
	for rows.Next() {
		var claimID string
		if err := rows.Scan(&claimID); err != nil {
			rows.Close()
			return nil, err
		}
		ended = append(ended, claimID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, claimID := range ended {
		if err := a.audit(txCtx, actorID, "staffing.claim_removed", "event_staffing_claim", claimID, map[string]any{
			"eventId":        eventID,
			"staffingItemId": itemID,
			"reason":         "shift_cancelled",
		}); err != nil {
			return nil, err
		}
	}
	return ended, nil
}

func loadStaffingClaimRoleIDs(ctx context.Context, tx pgx.Tx, itemID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select role_id from event_staffing_claim_roles where staffing_item_id = $1 order by created_at, role_id
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roleIDs := make([]string, 0)
	for rows.Next() {
		var roleID string
		if err := rows.Scan(&roleID); err != nil {
			return nil, err
		}
		roleIDs = append(roleIDs, roleID)
	}
	return roleIDs, rows.Err()
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	set := make(map[string]struct{}, len(left))
	for _, value := range left {
		set[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func eventStaffingItemDTOFromRow(row eventStaffingItemRow) eventStaffingItemDTO {
	dto := eventStaffingItemDTO{
		ID:                      row.ID,
		EventID:                 row.EventID,
		Title:                   row.Title,
		Kind:                    row.Kind,
		Notes:                   row.Notes,
		ParticipantRequirements: row.ParticipantRequirements,
		Status:                  row.Status,
		CreatedAt:               row.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:               row.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Capacity:                row.Capacity,
		ClaimsOpen:              row.ClaimsOpen,
		ReleaseCutoffHours:      row.ReleaseCutoffHours,
		ClaimRoleIDs:            []string{},
		Claims:                  []staffingClaimDTO{},
	}
	if row.AssignedPersonID.Valid || row.AssignedApplicationID.Valid {
		dto.FilledCount = 1
	}
	dto.StartsAt = nullableTimeString(row.StartsAt)
	dto.EndsAt = nullableTimeString(row.EndsAt)
	dto.AssignedPersonID = nullableString(row.AssignedPersonID)
	dto.AssignedApplicationID = nullableString(row.AssignedApplicationID)
	dto.AssigneeName = nullableString(row.AssigneeName)
	dto.CompletedAt = nullableTimeString(row.CompletedAt)
	dto.CompletedByPersonID = nullableString(row.CompletedByPersonID)
	return dto
}

func parseOptionalRFC3339Time(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := parseRFC3339Time(*value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func loadEventStaffingItemRow(ctx context.Context, q staffingRowQuerier, eventID, staffingID string) (eventStaffingItemRow, error) {
	var row eventStaffingItemRow
	if err := q.QueryRow(ctx, `
		select esi.id, esi.event_id, esi.title, esi.kind, esi.notes, esi.participant_requirements, esi.starts_at, esi.ends_at,
		       esi.assigned_person_id, esi.assigned_application_id,
		       coalesce(nullif(trim(p.display_name), ''), p.email, era.applicant_name) as assignee_name,
		       esi.status, esi.created_at, esi.updated_at, esi.completed_at, esi.completed_by_person_id,
		       esi.capacity, esi.claims_open, esi.release_cutoff_hours
		from event_staffing_items esi
		left join people p on p.id = esi.assigned_person_id
		left join event_role_applications era on era.id = esi.assigned_application_id
		where esi.event_id = $1
		  and esi.id = $2
		for update of esi
	`, eventID, staffingID).Scan(&row.ID, &row.EventID, &row.Title, &row.Kind, &row.Notes, &row.ParticipantRequirements, &row.StartsAt, &row.EndsAt, &row.AssignedPersonID, &row.AssignedApplicationID, &row.AssigneeName, &row.Status, &row.CreatedAt, &row.UpdatedAt, &row.CompletedAt, &row.CompletedByPersonID, &row.Capacity, &row.ClaimsOpen, &row.ReleaseCutoffHours); err != nil {
		return eventStaffingItemRow{}, err
	}
	return row, nil
}
