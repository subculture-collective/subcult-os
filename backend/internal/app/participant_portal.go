package app

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

// participantPortalDTO contains only records assigned to the authenticated
// person. An event-role application is deliberately not an identity claim and
// never appears here.
type participantPortalDTO struct {
	Assignments []participantAssignmentDTO `json:"assignments"`
	Commitments []participantCommitmentDTO `json:"commitments"`
}

type participantAssignmentDTO struct {
	EventID                 string  `json:"eventId"`
	EventTitle              string  `json:"eventTitle"`
	StaffingItemID          string  `json:"staffingItemId"`
	Title                   string  `json:"title"`
	Kind                    string  `json:"kind"`
	StartsAt                *string `json:"startsAt,omitempty"`
	EndsAt                  *string `json:"endsAt,omitempty"`
	Status                  string  `json:"status"`
	ParticipantRequirements string  `json:"participantRequirements"`
}

type participantCommitmentDTO struct {
	ID         string  `json:"id"`
	EventID    string  `json:"eventId"`
	EventTitle string  `json:"eventTitle"`
	Title      string  `json:"title"`
	DueAt      *string `json:"dueAt,omitempty"`
	Status     string  `json:"status"`
}

type participantPortalLoad struct {
	portal       participantPortalDTO
	workspaceIDs map[string]struct{}
}

// handleGetParticipantPortal returns a participant's own person-backed
// staffing assignments and commitments. It does not use application names or
// emails as an account identity, and it does not expose operator notes,
// contacts, finance, or payout data.
func (a *App) handleGetParticipantPortal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	personID, ok := a.requirePersonID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	loaded, err := a.loadParticipantPortal(r.Context(), personID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load participant portal")
		return
	}
	// The loader joins active membership. Recheck immediately before emitting
	// private schedule bytes so a revocation during the queries is denied.
	for workspaceID := range loaded.workspaceIDs {
		if _, err := a.activeMembership(r.Context(), personID, workspaceID); err != nil {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	writeJSON(w, http.StatusOK, loaded.portal)
}

func (a *App) loadParticipantPortal(ctx context.Context, personID string) (participantPortalLoad, error) {
	loaded := participantPortalLoad{
		portal: participantPortalDTO{
			Assignments: []participantAssignmentDTO{},
			Commitments: []participantCommitmentDTO{},
		},
		workspaceIDs: make(map[string]struct{}),
	}
	assignments, err := a.db.Query(ctx, `
		select e.workspace_id, e.id, e.title, esi.id, esi.title, esi.kind,
		       esi.starts_at, esi.ends_at, esi.status, esi.participant_requirements
		from event_staffing_items esi
		join events e on e.id = esi.event_id
		join workspace_members wm on wm.workspace_id = e.workspace_id
		where esi.assigned_person_id = $1
		  and wm.person_id = $1
		  and wm.removed_at is null
		  and wm.revoked_at is null
		  and (wm.expires_at is null or wm.expires_at > now())
		  and esi.status <> 'cancelled'
		order by esi.starts_at nulls last, e.starts_at, esi.created_at, esi.id
	`, personID)
	if err != nil {
		return participantPortalLoad{}, err
	}
	defer assignments.Close()
	for assignments.Next() {
		var workspaceID string
		var assignment participantAssignmentDTO
		var startsAt, endsAt sql.NullTime
		if err := assignments.Scan(&workspaceID, &assignment.EventID, &assignment.EventTitle, &assignment.StaffingItemID,
			&assignment.Title, &assignment.Kind, &startsAt, &endsAt, &assignment.Status, &assignment.ParticipantRequirements); err != nil {
			return participantPortalLoad{}, err
		}
		assignment.StartsAt = participantPortalTime(startsAt)
		assignment.EndsAt = participantPortalTime(endsAt)
		loaded.workspaceIDs[workspaceID] = struct{}{}
		loaded.portal.Assignments = append(loaded.portal.Assignments, assignment)
	}
	if err := assignments.Err(); err != nil {
		return participantPortalLoad{}, err
	}

	commitments, err := a.db.Query(ctx, `
		select e.workspace_id, c.id, e.id, e.title, c.title, c.due_at, c.status
		from commitments c
		join events e on e.id = c.event_id and e.workspace_id = c.workspace_id
		join workspace_members wm on wm.workspace_id = e.workspace_id
		where c.owner_person_id = $1
		  and wm.person_id = $1
		  and wm.removed_at is null
		  and wm.revoked_at is null
		  and (wm.expires_at is null or wm.expires_at > now())
		  and exists (
			select 1
			from event_staffing_items esi
			where esi.event_id = e.id
			  and esi.assigned_person_id = $1
			  and esi.status <> 'cancelled'
		  )
		order by c.due_at nulls last, c.created_at, c.id
	`, personID)
	if err != nil {
		return participantPortalLoad{}, err
	}
	defer commitments.Close()
	for commitments.Next() {
		var workspaceID string
		var commitment participantCommitmentDTO
		var dueAt sql.NullTime
		if err := commitments.Scan(&workspaceID, &commitment.ID, &commitment.EventID, &commitment.EventTitle,
			&commitment.Title, &dueAt, &commitment.Status); err != nil {
			return participantPortalLoad{}, err
		}
		commitment.DueAt = participantPortalTime(dueAt)
		loaded.workspaceIDs[workspaceID] = struct{}{}
		loaded.portal.Commitments = append(loaded.portal.Commitments, commitment)
	}
	if err := commitments.Err(); err != nil {
		return participantPortalLoad{}, err
	}
	return loaded, nil
}

func participantPortalTime(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339Nano)
	return &formatted
}
