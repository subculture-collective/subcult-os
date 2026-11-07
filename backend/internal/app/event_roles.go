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

type eventRoleDTO struct {
	ID          string `json:"id"`
	EventID     string `json:"eventId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Capacity    int    `json:"capacity"`
	Public      bool   `json:"public"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type eventRoleRow struct {
	ID                string
	EventID           string
	Name              string
	Description       string
	Capacity          int
	Public            bool
	Active            bool
	CreatedByPersonID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type createEventRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Capacity    int    `json:"capacity"`
	Public      *bool  `json:"public"`
}

type updateEventRoleRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Capacity    *int    `json:"capacity"`
	Public      *bool   `json:"public"`
	Active      *bool   `json:"active"`
}

type eventRoleApplicationDTO struct {
	ID                 string  `json:"id"`
	EventID            string  `json:"eventId"`
	RoleID             string  `json:"roleId"`
	ApplicantName      string  `json:"applicantName"`
	ApplicantEmail     string  `json:"applicantEmail"`
	Message            string  `json:"message"`
	Status             string  `json:"status"`
	ReviewedByPersonID *string `json:"reviewedByPersonId,omitempty"`
	ReviewedAt         *string `json:"reviewedAt,omitempty"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
	// AccountLinked reports that a signed-in account with this verified email
	// submitted the application, so an approval can let it claim shifts.
	AccountLinked bool `json:"accountLinked"`
}

type eventParticipantDTO struct {
	ApplicationID  string `json:"applicationId"`
	RoleID         string `json:"roleId"`
	RoleName       string `json:"roleName"`
	ApplicantName  string `json:"applicantName"`
	ApplicantEmail string `json:"applicantEmail"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updatedAt"`
}

type eventRoleApplicationRow struct {
	ID                 string
	EventID            string
	RoleID             string
	ApplicantName      string
	ApplicantEmail     string
	Message            string
	Status             string
	ReviewedByPersonID sql.NullString
	ReviewedAt         sql.NullTime
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ApplicantPersonID  sql.NullString
}

type submitPublicRoleApplicationRequest struct {
	RoleID         string `json:"roleId"`
	ApplicantName  string `json:"applicantName"`
	ApplicantEmail string `json:"applicantEmail"`
	Message        string `json:"message"`
}

type reviewEventRoleApplicationRequest struct {
	Status string `json:"status"`
}

var validReviewApplicationStatuses = map[string]struct{}{
	"submitted":    {},
	"under_review": {},
	"accepted":     {},
	"waitlisted":   {},
	"rejected":     {},
	"withdrawn":    {},
	"confirmed":    {},
}

var capacityConsumingApplicationStatuses = map[string]struct{}{
	"accepted":  {},
	"confirmed": {},
}

func (a *App) handleListEventRoles(w http.ResponseWriter, r *http.Request) {
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

	roles, err := a.loadEventRoles(r.Context(), event.ID, false, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load roles")
		return
	}
	writeJSON(w, http.StatusOK, eventRoleDTOsFromRows(roles))
}

func (a *App) handleCreateEventRole(w http.ResponseWriter, r *http.Request) {
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
	actorID, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, "owner")
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req createEventRoleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Capacity < 0 {
		writeError(w, http.StatusBadRequest, "capacity must be non-negative")
		return
	}
	rolePublic := true
	if req.Public != nil {
		rolePublic = *req.Public
	}

	var role eventRoleRow
	if err := a.db.QueryRow(r.Context(), `
		insert into event_roles (event_id, name, description, capacity, "public", active, created_by_person_id)
		values ($1, $2, $3, $4, $5, true, $6)
		returning id, event_id, name, description, capacity, "public", active, created_by_person_id, created_at, updated_at
	`, event.ID, name, description, req.Capacity, rolePublic, actorID).Scan(&role.ID, &role.EventID, &role.Name, &role.Description, &role.Capacity, &role.Public, &role.Active, &role.CreatedByPersonID, &role.CreatedAt, &role.UpdatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create role")
		return
	}

	writeJSON(w, http.StatusOK, eventRoleDTOFromRow(role))
}

func (a *App) handleUpdateEventRole(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	actorID, ok := a.requirePersonID(r)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req updateEventRoleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name == nil && req.Description == nil && req.Capacity == nil && req.Public == nil && req.Active == nil {
		writeError(w, http.StatusBadRequest, "no changes provided")
		return
	}

	var requestedName *string
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		requestedName = &name
	}
	var requestedDescription *string
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if utf8.RuneCountInString(description) > 2000 {
			writeError(w, http.StatusBadRequest, "description must be 2000 characters or fewer")
			return
		}
		requestedDescription = &description
	}
	if req.Capacity != nil && *req.Capacity < 0 {
		writeError(w, http.StatusBadRequest, "capacity must be non-negative")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var role eventRoleRow
	var workspaceID, eventStatus string
	if err := tx.QueryRow(r.Context(), `
		select r.id, r.event_id, r.name, r.description, r.capacity, r."public", r.active,
		       r.created_by_person_id, r.created_at, r.updated_at, e.workspace_id, e.status
		from event_roles r
		join events e on e.id = r.event_id
		where e.id = $1
		  and r.id = $2
		for update of r
	`, r.PathValue("eventID"), r.PathValue("roleID")).Scan(&role.ID, &role.EventID, &role.Name, &role.Description, &role.Capacity, &role.Public, &role.Active, &role.CreatedByPersonID, &role.CreatedAt, &role.UpdatedAt, &workspaceID, &eventStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "role not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load role")
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

	nextName := role.Name
	if requestedName != nil {
		nextName = *requestedName
	}
	nextDescription := role.Description
	if requestedDescription != nil {
		nextDescription = *requestedDescription
	}
	nextCapacity := role.Capacity
	if req.Capacity != nil {
		nextCapacity = *req.Capacity
	}
	nextPublic := role.Public
	if req.Public != nil {
		nextPublic = *req.Public
	}
	nextActive := role.Active
	if req.Active != nil {
		nextActive = *req.Active
	}

	if nextCapacity < role.Capacity {
		var activeCount int
		if err := tx.QueryRow(r.Context(), `
			select count(*)
			from event_role_applications
			where role_id = $1
			  and status in ('accepted', 'confirmed')
		`, role.ID).Scan(&activeCount); err != nil {
			writeError(w, http.StatusInternalServerError, "could not count applications")
			return
		}
		if activeCount > nextCapacity {
			writeError(w, http.StatusConflict, "capacity is below accepted applications")
			return
		}
	}

	if err := tx.QueryRow(r.Context(), `
		update event_roles
		set name = $3,
		    description = $4,
		    capacity = $5,
		    "public" = $6,
		    active = $7,
		    updated_at = now()
		where event_id = $1
		  and id = $2
		returning id, event_id, name, description, capacity, "public", active, created_by_person_id, created_at, updated_at
	`, role.EventID, role.ID, nextName, nextDescription, nextCapacity, nextPublic, nextActive).Scan(&role.ID, &role.EventID, &role.Name, &role.Description, &role.Capacity, &role.Public, &role.Active, &role.CreatedByPersonID, &role.CreatedAt, &role.UpdatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update role")
		return
	}

	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "event_role.updated", "event_role", role.ID, map[string]any{
		"eventId":  role.EventID,
		"roleId":   role.ID,
		"name":     role.Name,
		"capacity": role.Capacity,
		"public":   role.Public,
		"active":   role.Active,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save role")
		return
	}

	writeJSON(w, http.StatusOK, eventRoleDTOFromRow(role))
}

func (a *App) handleListPublicEventRoles(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	event, err := a.loadPublishedEventBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}

	roles, err := a.loadEventRoles(r.Context(), event.ID, true, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load roles")
		return
	}
	writeJSON(w, http.StatusOK, eventRoleDTOsFromRows(roles))
}

func (a *App) handleListEventRoleApplications(w http.ResponseWriter, r *http.Request) {
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

	applications, err := a.loadEventRoleApplications(r.Context(), event.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load applications")
		return
	}
	writeJSON(w, http.StatusOK, eventRoleApplicationDTOsFromRows(applications))
}

func (a *App) handleListEventParticipants(w http.ResponseWriter, r *http.Request) {
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

	participants, err := a.loadEventParticipants(r.Context(), event.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load participants")
		return
	}
	writeJSON(w, http.StatusOK, eventParticipantDTOsFromRows(participants))
}

func (a *App) handleReviewEventRoleApplication(w http.ResponseWriter, r *http.Request) {
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
	actorID, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, "owner")
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req reviewEventRoleApplicationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	nextStatus := strings.TrimSpace(req.Status)
	if _, ok := validReviewApplicationStatuses[nextStatus]; !ok {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	applicationID := r.PathValue("applicationID")
	var workspaceID string
	var eventTitle string
	var roleID string
	var roleName string
	var capacity int
	var applicantEmail string
	if err := tx.QueryRow(r.Context(), `
		select e.workspace_id, e.title, r.id, r.name, r.capacity, a.applicant_email
		from events e
		join event_roles r on r.event_id = e.id
		join event_role_applications a on a.role_id = r.id and a.event_id = e.id
		where e.id = $1
		  and a.id = $2
		for update of r, a
	`, event.ID, applicationID).Scan(&workspaceID, &eventTitle, &roleID, &roleName, &capacity, &applicantEmail); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "application not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load role")
		return
	}

	var application eventRoleApplicationRow
	if err := tx.QueryRow(r.Context(), `
		select id, event_id, role_id, applicant_name, applicant_email, message, status, reviewed_by_person_id, reviewed_at, created_at, updated_at, applicant_person_id
		from event_role_applications
		where event_id = $1
		  and id = $2
		for update
	`, event.ID, applicationID).Scan(&application.ID, &application.EventID, &application.RoleID, &application.ApplicantName, &application.ApplicantEmail, &application.Message, &application.Status, &application.ReviewedByPersonID, &application.ReviewedAt, &application.CreatedAt, &application.UpdatedAt, &application.ApplicantPersonID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "application not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load application")
		return
	}
	previousStatus := application.Status

	if _, ok := capacityConsumingApplicationStatuses[nextStatus]; ok {
		var activeCount int
		if err := tx.QueryRow(r.Context(), `
			select count(*)
			from event_role_applications
			where role_id = $1
			  and id <> $2
			  and status in ('accepted', 'confirmed')
		`, roleID, applicationID).Scan(&activeCount); err != nil {
			writeError(w, http.StatusInternalServerError, "could not count applications")
			return
		}
		if activeCount+1 > capacity {
			writeError(w, http.StatusConflict, "role capacity reached")
			return
		}
	}

	now := time.Now().UTC()
	if err := tx.QueryRow(r.Context(), `
		update event_role_applications
		set status = $1,
		    reviewed_by_person_id = $2,
		    reviewed_at = $3,
		    updated_at = $3
		where id = $4
		returning id, event_id, role_id, applicant_name, applicant_email, message, status, reviewed_by_person_id, reviewed_at, created_at, updated_at, applicant_person_id
	`, nextStatus, actorID, now, applicationID).Scan(&application.ID, &application.EventID, &application.RoleID, &application.ApplicantName, &application.ApplicantEmail, &application.Message, &application.Status, &application.ReviewedByPersonID, &application.ReviewedAt, &application.CreatedAt, &application.UpdatedAt, &application.ApplicantPersonID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not review application")
		return
	}

	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if isRoleApplicationOutcomeStatus(nextStatus) && previousStatus != nextStatus {
		params := enqueueNotificationParams{
			WorkspaceID:       workspaceID,
			EventID:           event.ID,
			RecipientEmail:    applicantEmail,
			NotificationType:  "role_application." + nextStatus,
			RelatedType:       "event_role_application",
			RelatedID:         application.ID,
			IdempotencyKey:    "role_application:" + application.ID + ":" + nextStatus,
			Subject:           fmt.Sprintf("%s: application update for %s", eventTitle, roleName),
			Body:              fmt.Sprintf("Your application for %s at %s was %s.", roleName, eventTitle, nextStatus),
			Preview:           fmt.Sprintf("Application %s for %s", nextStatus, roleName),
			CreatedByPersonID: actorID,
		}
		if _, err := a.enqueueNotification(txCtx, tx, params); err != nil {
			writeError(w, http.StatusInternalServerError, "could not enqueue notification")
			return
		}
	}
	if err := a.audit(txCtx, actorID, "role_application.reviewed", "event_role_application", application.ID, map[string]any{
		"eventId":        application.EventID,
		"roleId":         roleID,
		"applicationId":  application.ID,
		"previousStatus": previousStatus,
		"nextStatus":     nextStatus,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save application")
		return
	}

	writeJSON(w, http.StatusOK, eventRoleApplicationDTOFromRow(application))
}

func (a *App) handleSubmitPublicRoleApplication(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}

	var req submitPublicRoleApplicationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	event, err := a.loadPublishedEventBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}

	roleID := strings.TrimSpace(req.RoleID)
	role, err := a.loadPublicActiveEventRole(r.Context(), event.ID, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "role not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load role")
		return
	}

	applicantName := strings.TrimSpace(req.ApplicantName)
	applicantEmail := normalizeEmail(req.ApplicantEmail)
	message := strings.TrimSpace(req.Message)
	if applicantName == "" {
		writeError(w, http.StatusBadRequest, "applicant name is required")
		return
	}
	if !basicEmail(applicantEmail) {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if utf8.RuneCountInString(message) > 2000 {
		writeError(w, http.StatusBadRequest, "message must be 2000 characters or fewer")
		return
	}

	// A signed-in submitter whose verified account email equals the submitted
	// email binds the application to that account. Any other submission stays
	// an unbound email record, as before.
	var applicantPersonID any
	if personID, ok := a.requirePersonID(r); ok {
		var accountEmail string
		if err := a.db.QueryRow(r.Context(), `
			select p.email
			from people p
			join email_identities ei on ei.person_id = p.id
			where p.id = $1
			  and ei.verified_at is not null
		`, personID).Scan(&accountEmail); err == nil && normalizeEmail(accountEmail) == applicantEmail {
			applicantPersonID = personID
		}
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var application eventRoleApplicationRow
	if err := tx.QueryRow(r.Context(), `
		insert into event_role_applications (event_id, role_id, applicant_name, applicant_email, message, status, applicant_person_id)
		values ($1, $2, $3, $4, $5, 'submitted', $6)
		returning id, event_id, role_id, applicant_name, applicant_email, message, status, created_at, updated_at, applicant_person_id
	`, event.ID, role.ID, applicantName, applicantEmail, message, applicantPersonID).Scan(&application.ID, &application.EventID, &application.RoleID, &application.ApplicantName, &application.ApplicantEmail, &application.Message, &application.Status, &application.CreatedAt, &application.UpdatedAt, &application.ApplicantPersonID); err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "application already submitted")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create application")
		return
	}

	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, "", "role_application.submitted", "event_role_application", application.ID, map[string]any{
		"eventId":       application.EventID,
		"roleId":        application.RoleID,
		"applicationId": application.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save application")
		return
	}

	writeJSON(w, http.StatusOK, eventRoleApplicationDTOFromRow(application))
}

func (a *App) loadEventRoles(ctx context.Context, eventID string, publicOnly, activeOnly bool) ([]eventRoleRow, error) {
	query := `
		select id, event_id, name, description, capacity, "public", active, created_by_person_id, created_at, updated_at
		from event_roles
		where event_id = $1`
	if publicOnly {
		query += ` and "public" = true`
	}
	if activeOnly {
		query += ` and active = true`
	}
	query += ` order by created_at asc, name asc, id asc`

	rows, err := a.db.Query(ctx, query, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]eventRoleRow, 0)
	for rows.Next() {
		var role eventRoleRow
		if err := rows.Scan(&role.ID, &role.EventID, &role.Name, &role.Description, &role.Capacity, &role.Public, &role.Active, &role.CreatedByPersonID, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

func (a *App) loadPublicActiveEventRole(ctx context.Context, eventID, roleID string) (eventRoleRow, error) {
	var role eventRoleRow
	if roleID == "" {
		return role, pgx.ErrNoRows
	}
	if err := a.db.QueryRow(ctx, `
		select id, event_id, name, description, capacity, "public", active, created_by_person_id, created_at, updated_at
		from event_roles
		where event_id = $1
		  and id = $2
		  and "public" = true
		  and active = true
	`, eventID, roleID).Scan(&role.ID, &role.EventID, &role.Name, &role.Description, &role.Capacity, &role.Public, &role.Active, &role.CreatedByPersonID, &role.CreatedAt, &role.UpdatedAt); err != nil {
		return eventRoleRow{}, err
	}
	return role, nil
}

func (a *App) loadEventRoleApplications(ctx context.Context, eventID string) ([]eventRoleApplicationRow, error) {
	rows, err := a.db.Query(ctx, `
		select id, event_id, role_id, applicant_name, applicant_email, message, status, reviewed_by_person_id, reviewed_at, created_at, updated_at, applicant_person_id
		from event_role_applications
		where event_id = $1
		order by created_at asc, id asc
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applications := make([]eventRoleApplicationRow, 0)
	for rows.Next() {
		var application eventRoleApplicationRow
		if err := rows.Scan(&application.ID, &application.EventID, &application.RoleID, &application.ApplicantName, &application.ApplicantEmail, &application.Message, &application.Status, &application.ReviewedByPersonID, &application.ReviewedAt, &application.CreatedAt, &application.UpdatedAt, &application.ApplicantPersonID); err != nil {
			return nil, err
		}
		applications = append(applications, application)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applications, nil
}

func (a *App) loadEventParticipants(ctx context.Context, eventID string) ([]eventParticipantRow, error) {
	rows, err := a.db.Query(ctx, `
		select a.id, a.event_id, a.role_id, r.name, a.applicant_name, a.applicant_email, a.status, a.updated_at
		from event_role_applications a
		join event_roles r on r.id = a.role_id and r.event_id = a.event_id
		where a.event_id = $1
		  and a.status in ('accepted', 'confirmed')
		order by r.name asc, a.applicant_name asc, a.updated_at asc, a.id asc
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	participants := make([]eventParticipantRow, 0)
	for rows.Next() {
		var participant eventParticipantRow
		if err := rows.Scan(&participant.ApplicationID, &participant.EventID, &participant.RoleID, &participant.RoleName, &participant.ApplicantName, &participant.ApplicantEmail, &participant.Status, &participant.UpdatedAt); err != nil {
			return nil, err
		}
		participants = append(participants, participant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return participants, nil
}

func eventRoleDTOFromRow(row eventRoleRow) eventRoleDTO {
	return eventRoleDTO{
		ID:          row.ID,
		EventID:     row.EventID,
		Name:        row.Name,
		Description: row.Description,
		Capacity:    row.Capacity,
		Public:      row.Public,
		Active:      row.Active,
		CreatedAt:   row.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:   row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func eventRoleDTOsFromRows(rows []eventRoleRow) []eventRoleDTO {
	roles := make([]eventRoleDTO, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, eventRoleDTOFromRow(row))
	}
	return roles
}

func eventRoleApplicationDTOFromRow(row eventRoleApplicationRow) eventRoleApplicationDTO {
	return eventRoleApplicationDTO{
		ID:                 row.ID,
		EventID:            row.EventID,
		RoleID:             row.RoleID,
		ApplicantName:      row.ApplicantName,
		ApplicantEmail:     row.ApplicantEmail,
		Message:            row.Message,
		Status:             row.Status,
		ReviewedByPersonID: nullableString(row.ReviewedByPersonID),
		ReviewedAt:         nullableTimeString(row.ReviewedAt),
		CreatedAt:          row.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          row.UpdatedAt.UTC().Format(time.RFC3339Nano),
		AccountLinked:      row.ApplicantPersonID.Valid,
	}
}

func eventRoleApplicationDTOsFromRows(rows []eventRoleApplicationRow) []eventRoleApplicationDTO {
	applications := make([]eventRoleApplicationDTO, 0, len(rows))
	for _, row := range rows {
		applications = append(applications, eventRoleApplicationDTOFromRow(row))
	}
	return applications
}

type eventParticipantRow struct {
	ApplicationID  string
	EventID        string
	RoleID         string
	RoleName       string
	ApplicantName  string
	ApplicantEmail string
	Status         string
	UpdatedAt      time.Time
}

func eventParticipantDTOFromRow(row eventParticipantRow) eventParticipantDTO {
	return eventParticipantDTO{
		ApplicationID:  row.ApplicationID,
		RoleID:         row.RoleID,
		RoleName:       row.RoleName,
		ApplicantName:  row.ApplicantName,
		ApplicantEmail: row.ApplicantEmail,
		Status:         row.Status,
		UpdatedAt:      row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func eventParticipantDTOsFromRows(rows []eventParticipantRow) []eventParticipantDTO {
	participants := make([]eventParticipantDTO, 0, len(rows))
	for _, row := range rows {
		participants = append(participants, eventParticipantDTOFromRow(row))
	}
	return participants
}

func isCapacityConsumingApplicationStatus(status string) bool {
	_, ok := capacityConsumingApplicationStatuses[status]
	return ok
}

func isValidReviewApplicationStatus(status string) bool {
	_, ok := validReviewApplicationStatuses[status]
	return ok
}

func isRoleApplicationOutcomeStatus(status string) bool {
	switch status {
	case "accepted", "waitlisted", "rejected":
		return true
	default:
		return false
	}
}

func basicEmail(email string) bool {
	if email == "" || strings.Contains(email, " ") {
		return false
	}
	local, domain, ok := strings.Cut(email, "@")
	return ok && local != "" && domain != ""
}
