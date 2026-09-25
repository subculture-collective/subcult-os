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
	Title                   *string `json:"title"`
	Notes                   *string `json:"notes"`
	ParticipantRequirements *string `json:"participantRequirements"`
	StartsAt                *string `json:"startsAt"`
	ClearStartsAt           bool    `json:"clearStartsAt"`
	EndsAt                  *string `json:"endsAt"`
	ClearEndsAt             bool    `json:"clearEndsAt"`
	AssignedPersonID        *string `json:"assignedPersonId"`
	AssignedApplicationID   *string `json:"assignedApplicationId"`
	ClearAssignee           bool    `json:"clearAssignee"`
	Status                  *string `json:"status"`
}

type eventStaffingItemDTO struct {
	ID                      string  `json:"id"`
	EventID                 string  `json:"eventId"`
	Title                   string  `json:"title"`
	Kind                    string  `json:"kind"`
	Notes                   string  `json:"notes"`
	ParticipantRequirements string  `json:"participantRequirements"`
	StartsAt                *string `json:"startsAt,omitempty"`
	EndsAt                  *string `json:"endsAt,omitempty"`
	AssignedPersonID        *string `json:"assignedPersonId,omitempty"`
	AssignedApplicationID   *string `json:"assignedApplicationId,omitempty"`
	AssigneeName            *string `json:"assigneeName,omitempty"`
	Status                  string  `json:"status"`
	CreatedAt               string  `json:"createdAt"`
	UpdatedAt               string  `json:"updatedAt"`
	CompletedAt             *string `json:"completedAt,omitempty"`
	CompletedByPersonID     *string `json:"completedByPersonId,omitempty"`
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
	if _, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, "owner", "member"); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := a.db.Query(r.Context(), `
		select esi.id, esi.event_id, esi.title, esi.kind, esi.notes, esi.participant_requirements, esi.starts_at, esi.ends_at,
		       esi.assigned_person_id, esi.assigned_application_id,
		       coalesce(nullif(trim(p.display_name), ''), p.email, era.applicant_name) as assignee_name,
		       esi.status, esi.created_at, esi.updated_at, esi.completed_at, esi.completed_by_person_id
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
		if err := rows.Scan(&row.ID, &row.EventID, &row.Title, &row.Kind, &row.Notes, &row.ParticipantRequirements, &row.StartsAt, &row.EndsAt, &row.AssignedPersonID, &row.AssignedApplicationID, &row.AssigneeName, &row.Status, &row.CreatedAt, &row.UpdatedAt, &row.CompletedAt, &row.CompletedByPersonID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load staffing")
			return
		}
		items = append(items, eventStaffingItemDTOFromRow(row))
	}
	if err := rows.Err(); err != nil {
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
		          status, created_at, updated_at, completed_at, completed_by_person_id
	`, lockedEventID, title, kind, notes, participantRequirements, startsAtArg, endsAtArg, actorID).Scan(
		&row.ID, &row.EventID, &row.Title, &row.Kind, &row.Notes, &row.ParticipantRequirements, &row.StartsAt, &row.EndsAt,
		&row.AssignedPersonID, &row.AssignedApplicationID, &row.AssigneeName, &row.Status, &row.CreatedAt, &row.UpdatedAt, &row.CompletedAt, &row.CompletedByPersonID,
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
	} else {
		if req.ClearAssignee && current.Status == "assigned" && newRow.Status != "open" {
			newRow.Status = "open"
			changed = true
		} else if (requestedPersonID != nil || requestedApplicationID != nil) && current.Status == "open" && newRow.Status != "assigned" {
			newRow.Status = "assigned"
			changed = true
		}
	}

	if !changed {
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save staffing item")
			return
		}
		writeJSON(w, http.StatusOK, eventStaffingItemDTOFromRow(current))
		return
	}
	if eventStatusIsClosed(eventStatus) {
		writeError(w, http.StatusConflict, "event is closed")
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
		    updated_at = now()
		where event_id = $1
		  and id = $2
	`, lockedEventID, current.ID, newRow.Title, newRow.Notes, newRow.ParticipantRequirements, startsAtArg, endsAtArg, assignedPersonArg, assignedApplicationArg, newRow.Status, completedAtArg, completedByArg); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update staffing item")
		return
	}

	updated, err := loadEventStaffingItemRow(r.Context(), tx, lockedEventID, current.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}

	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
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
	if err := a.audit(txCtx, actorID, "staffing.updated", "event_staffing_item", updated.ID, map[string]any{
		"eventId":               lockedEventID,
		"staffingItemId":        updated.ID,
		"previousStatus":        current.Status,
		"status":                updated.Status,
		"assignedPersonId":      nullableString(updated.AssignedPersonID),
		"assignedApplicationId": nullableString(updated.AssignedApplicationID),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save staffing item")
		return
	}

	writeJSON(w, http.StatusOK, eventStaffingItemDTOFromRow(updated))
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
		       esi.status, esi.created_at, esi.updated_at, esi.completed_at, esi.completed_by_person_id
		from event_staffing_items esi
		left join people p on p.id = esi.assigned_person_id
		left join event_role_applications era on era.id = esi.assigned_application_id
		where esi.event_id = $1
		  and esi.id = $2
		for update of esi
	`, eventID, staffingID).Scan(&row.ID, &row.EventID, &row.Title, &row.Kind, &row.Notes, &row.ParticipantRequirements, &row.StartsAt, &row.EndsAt, &row.AssignedPersonID, &row.AssignedApplicationID, &row.AssigneeName, &row.Status, &row.CreatedAt, &row.UpdatedAt, &row.CompletedAt, &row.CompletedByPersonID); err != nil {
		return eventStaffingItemRow{}, err
	}
	return row, nil
}
