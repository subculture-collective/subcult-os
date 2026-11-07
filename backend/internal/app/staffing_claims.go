package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Shift places have one capacity authority: the legacy single assignee on
// event_staffing_items plus the active rows in event_staffing_claims, counted
// while the event row and then the staffing item row are locked. Participant
// claims, organizer additions, organizer single-assignee edits and retries
// all take those locks in that order, so a final place is granted once.
const (
	maxStaffingCapacity           = 100
	maxStaffingReleaseCutoffHours = 720
)

type staffingQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type staffingClaimDTO struct {
	ID        string `json:"id"`
	PersonID  string `json:"personId"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
}

// participantOpenShiftDTO is the participant-safe view of a claimable shift.
// It carries a remaining-place count, never the names, contacts or notes of
// anyone else holding a place.
type participantOpenShiftDTO struct {
	StaffingItemID          string `json:"staffingItemId"`
	EventID                 string `json:"eventId"`
	EventTitle              string `json:"eventTitle"`
	Title                   string `json:"title"`
	StartsAt                string `json:"startsAt"`
	EndsAt                  string `json:"endsAt"`
	ParticipantRequirements string `json:"participantRequirements"`
	RemainingPlaces         int    `json:"remainingPlaces"`
	Claimed                 bool   `json:"claimed"`
	CanRelease              bool   `json:"canRelease"`
	ReleaseDeadline         string `json:"releaseDeadline"`
}

type addStaffingClaimRequest struct {
	PersonID string `json:"personId"`
}

type staffingPlace struct {
	EventID                 string
	WorkspaceID             string
	EventTitle              string
	EventStatus             string
	ItemID                  string
	Title                   string
	Kind                    string
	Status                  string
	ParticipantRequirements string
	StartsAt                sql.NullTime
	EndsAt                  sql.NullTime
	Capacity                int
	ReleaseCutoffHours      int
	ClaimsOpen              bool
	LegacyPersonID          sql.NullString
	LegacyApplicationID     sql.NullString
	Filled                  int
}

func (p staffingPlace) remaining() int {
	if remaining := p.Capacity - p.Filled; remaining > 0 {
		return remaining
	}
	return 0
}

func (p staffingPlace) accepting() bool {
	return p.Status == "open" || p.Status == "assigned"
}

func validUUID(value string) bool {
	var id pgtype.UUID
	return value != "" && id.Scan(value) == nil
}

func (a *App) currentTime() time.Time {
	if a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
}

// staffingReleaseDeadline is the first instant at which a participant can no
// longer release their own place: the shift start minus its cutoff hours.
func staffingReleaseDeadline(startsAt time.Time, cutoffHours int) time.Time {
	return startsAt.Add(-time.Duration(cutoffHours) * time.Hour)
}

func staffingReleaseAllowed(now, startsAt time.Time, cutoffHours int) bool {
	return now.Before(staffingReleaseDeadline(startsAt, cutoffHours))
}

// lockStaffingPlace locks the event row, then the staffing item row, and
// counts occupied places. When eventID is empty the item's event is looked up
// first; the item is then re-read under the lock and must still belong to it.
func lockStaffingPlace(ctx context.Context, tx pgx.Tx, eventID, itemID string) (staffingPlace, error) {
	var place staffingPlace
	if !validUUID(itemID) || (eventID != "" && !validUUID(eventID)) {
		return place, pgx.ErrNoRows
	}
	if eventID == "" {
		if err := tx.QueryRow(ctx, `select event_id from event_staffing_items where id = $1`, itemID).Scan(&eventID); err != nil {
			return place, err
		}
	}
	if err := tx.QueryRow(ctx, `
		select id, workspace_id, title, status
		from events
		where id = $1
		for update
	`, eventID).Scan(&place.EventID, &place.WorkspaceID, &place.EventTitle, &place.EventStatus); err != nil {
		return place, err
	}
	if err := tx.QueryRow(ctx, `
		select id, title, kind, status, participant_requirements, starts_at, ends_at,
		       capacity, release_cutoff_hours, claims_open, assigned_person_id, assigned_application_id
		from event_staffing_items
		where id = $1
		  and event_id = $2
		for update
	`, itemID, place.EventID).Scan(&place.ItemID, &place.Title, &place.Kind, &place.Status, &place.ParticipantRequirements,
		&place.StartsAt, &place.EndsAt, &place.Capacity, &place.ReleaseCutoffHours, &place.ClaimsOpen,
		&place.LegacyPersonID, &place.LegacyApplicationID); err != nil {
		return place, err
	}
	claims, err := countActiveStaffingClaims(ctx, tx, place.ItemID)
	if err != nil {
		return place, err
	}
	place.Filled = claims
	if place.LegacyPersonID.Valid || place.LegacyApplicationID.Valid {
		place.Filled++
	}
	return place, nil
}

func countActiveStaffingClaims(ctx context.Context, tx pgx.Tx, itemID string) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `
		select count(*)
		from event_staffing_claims
		where staffing_item_id = $1
		  and status = 'active'
	`, itemID).Scan(&count)
	return count, err
}

func activeStaffingClaimID(ctx context.Context, tx pgx.Tx, itemID, personID string) (string, error) {
	var claimID string
	err := tx.QueryRow(ctx, `
		select id
		from event_staffing_claims
		where staffing_item_id = $1
		  and person_id = $2
		  and status = 'active'
		for update
	`, itemID, personID).Scan(&claimID)
	return claimID, err
}

// lockActiveMembership holds a share lock on the person's current membership
// so a concurrent revocation commits either before this check (and denies it)
// or after this transaction.
func lockActiveMembership(ctx context.Context, tx pgx.Tx, workspaceID, personID string) (string, error) {
	var role string
	err := tx.QueryRow(ctx, `
		select role
		from workspace_members
		where workspace_id = $1
		  and person_id = $2
		  and removed_at is null
		  and revoked_at is null
		  and (expires_at is null or expires_at > now())
		for share
	`, workspaceID, personID).Scan(&role)
	if err != nil {
		return "", ErrMembershipDenied
	}
	return role, nil
}

// eligibleStaffingApplication returns the person's own accepted or confirmed
// application for one of the shift's eligible roles. Submitted, under-review,
// waitlisted and other non-approved statuses do not qualify, and only
// applications bound to the account at submission count.
func eligibleStaffingApplication(ctx context.Context, tx pgx.Tx, eventID, itemID, personID string) (string, error) {
	var applicationID string
	err := tx.QueryRow(ctx, `
		select a.id
		from event_role_applications a
		join event_staffing_claim_roles cr on cr.role_id = a.role_id and cr.staffing_item_id = $2
		where a.event_id = $1
		  and a.applicant_person_id = $3
		  and a.status in ('accepted', 'confirmed')
		order by a.updated_at, a.id
		limit 1
		for share of a
	`, eventID, itemID, personID).Scan(&applicationID)
	return applicationID, err
}

// staffingPersonOverlaps reports whether the person holds another place on
// the same event whose time window overlaps [startsAt, endsAt). Windows that
// only touch (one ends when the other starts) do not overlap. Cancelled
// items and untimed items never conflict.
func staffingPersonOverlaps(ctx context.Context, tx pgx.Tx, eventID, itemID, personID string, startsAt, endsAt time.Time) (bool, error) {
	var overlaps bool
	err := tx.QueryRow(ctx, `
		select exists (
			select 1
			from event_staffing_items o
			where o.event_id = $1
			  and o.id <> $2
			  and o.status <> 'cancelled'
			  and o.starts_at is not null
			  and o.ends_at is not null
			  and o.starts_at < $5
			  and o.ends_at > $4
			  and (
				o.assigned_person_id = $3
				or exists (
					select 1
					from event_staffing_claims c
					where c.staffing_item_id = o.id
					  and c.person_id = $3
					  and c.status = 'active'
				)
			  )
		)
	`, eventID, itemID, personID, startsAt, endsAt).Scan(&overlaps)
	return overlaps, err
}

// syncStaffingFillStatus moves an open or assigned item between open and
// assigned as its places fill and free. Completed and cancelled items keep
// their status.
func syncStaffingFillStatus(ctx context.Context, tx pgx.Tx, itemID string) error {
	_, err := tx.Exec(ctx, `
		with filled as (
			select s.id,
			       case when s.assigned_person_id is not null or s.assigned_application_id is not null then 1 else 0 end
			       + (select count(*) from event_staffing_claims c where c.staffing_item_id = s.id and c.status = 'active') as places
			from event_staffing_items s
			where s.id = $1
		)
		update event_staffing_items esi
		set status = case when filled.places >= esi.capacity then 'assigned' else 'open' end,
		    updated_at = now()
		from filled
		where esi.id = filled.id
		  and esi.status in ('open', 'assigned')
		  and esi.status <> case when filled.places >= esi.capacity then 'assigned' else 'open' end
	`, itemID)
	return err
}

func openShiftDTOFromPlace(place staffingPlace, claimed, ownClaim bool, now time.Time) participantOpenShiftDTO {
	dto := participantOpenShiftDTO{
		StaffingItemID:          place.ItemID,
		EventID:                 place.EventID,
		EventTitle:              place.EventTitle,
		Title:                   place.Title,
		ParticipantRequirements: place.ParticipantRequirements,
		RemainingPlaces:         place.remaining(),
		Claimed:                 claimed,
	}
	if place.StartsAt.Valid {
		dto.StartsAt = place.StartsAt.Time.UTC().Format(time.RFC3339Nano)
		dto.ReleaseDeadline = staffingReleaseDeadline(place.StartsAt.Time, place.ReleaseCutoffHours).UTC().Format(time.RFC3339Nano)
		dto.CanRelease = ownClaim && place.accepting() && staffingReleaseAllowed(now, place.StartsAt.Time, place.ReleaseCutoffHours)
	}
	if place.EndsAt.Valid {
		dto.EndsAt = place.EndsAt.Time.UTC().Format(time.RFC3339Nano)
	}
	return dto
}

// handleListOpenShifts returns claimable shifts for which the signed-in
// person holds an approved, account-bound role application, plus shifts they
// currently claim. It reveals counts only, never other people's places.
func (a *App) handleListOpenShifts(w http.ResponseWriter, r *http.Request) {
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
	now := a.currentTime()
	rows, err := a.db.Query(r.Context(), `
		select e.workspace_id, e.id, e.title, e.status, esi.id, esi.title, esi.kind, esi.status, esi.participant_requirements,
		       esi.starts_at, esi.ends_at, esi.capacity, esi.release_cutoff_hours, esi.claims_open,
		       esi.assigned_person_id, esi.assigned_application_id,
		       (select count(*) from event_staffing_claims c where c.staffing_item_id = esi.id and c.status = 'active') as claim_count,
		       exists (select 1 from event_staffing_claims c where c.staffing_item_id = esi.id and c.person_id = $1 and c.status = 'active') as own_claim
		from event_staffing_items esi
		join events e on e.id = esi.event_id
		join workspace_members wm on wm.workspace_id = e.workspace_id
		where wm.person_id = $1
		  and wm.removed_at is null
		  and wm.revoked_at is null
		  and (wm.expires_at is null or wm.expires_at > now())
		  and esi.claims_open
		  and esi.status in ('open', 'assigned')
		  and e.status = 'published'
		  and (
			exists (select 1 from event_staffing_claims c where c.staffing_item_id = esi.id and c.person_id = $1 and c.status = 'active')
			or (
				esi.starts_at > $2
				and exists (
					select 1
					from event_role_applications a
					join event_staffing_claim_roles cr on cr.role_id = a.role_id and cr.staffing_item_id = esi.id
					where a.event_id = e.id
					  and a.applicant_person_id = $1
					  and a.status in ('accepted', 'confirmed')
				)
			)
		  )
		order by esi.starts_at, e.title, esi.id
	`, personID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load open shifts")
		return
	}
	defer rows.Close()
	shifts := make([]participantOpenShiftDTO, 0)
	workspaceIDs := make(map[string]struct{})
	for rows.Next() {
		var place staffingPlace
		var claimCount int
		var ownClaim bool
		if err := rows.Scan(&place.WorkspaceID, &place.EventID, &place.EventTitle, &place.EventStatus, &place.ItemID, &place.Title,
			&place.Kind, &place.Status, &place.ParticipantRequirements, &place.StartsAt, &place.EndsAt, &place.Capacity,
			&place.ReleaseCutoffHours, &place.ClaimsOpen, &place.LegacyPersonID, &place.LegacyApplicationID, &claimCount, &ownClaim); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load open shifts")
			return
		}
		place.Filled = claimCount
		if place.LegacyPersonID.Valid || place.LegacyApplicationID.Valid {
			place.Filled++
		}
		held := ownClaim || (place.LegacyPersonID.Valid && place.LegacyPersonID.String == personID)
		workspaceIDs[place.WorkspaceID] = struct{}{}
		shifts = append(shifts, openShiftDTOFromPlace(place, held, ownClaim, now))
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load open shifts")
		return
	}
	for workspaceID := range workspaceIDs {
		if _, err := a.activeMembership(r.Context(), personID, workspaceID); err != nil {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	writeJSON(w, http.StatusOK, shifts)
}

func (a *App) handleClaimOpenShift(w http.ResponseWriter, r *http.Request) {
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
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	place, err := lockStaffingPlace(ctx, tx, "", r.PathValue("staffingID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "shift not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load shift")
		return
	}
	if _, err := lockActiveMembership(ctx, tx, place.WorkspaceID, personID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	now := a.currentTime()

	// A retry, or a place the organizer already gave this person, returns the
	// existing hold without consuming another place.
	existingClaimID, err := activeStaffingClaimID(ctx, tx, place.ItemID, personID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "could not load claim")
		return
	}
	if existingClaimID != "" || (place.LegacyPersonID.Valid && place.LegacyPersonID.String == personID) {
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load claim")
			return
		}
		writeJSON(w, http.StatusOK, openShiftDTOFromPlace(place, true, existingClaimID != "", now))
		return
	}

	switch {
	case eventStatusIsClosed(place.EventStatus):
		writeError(w, http.StatusConflict, "event is closed")
		return
	case !eventStatusIsPublished(place.EventStatus):
		writeError(w, http.StatusConflict, "event is not open for shift claims")
		return
	case !place.ClaimsOpen || !place.accepting() || !place.StartsAt.Valid || !place.EndsAt.Valid:
		writeError(w, http.StatusConflict, "shift is not open for claims")
		return
	case !now.Before(place.StartsAt.Time):
		writeError(w, http.StatusConflict, "shift has already started")
		return
	}
	applicationID, err := eligibleStaffingApplication(ctx, tx, place.EventID, place.ItemID, personID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusForbidden, "an approved role application for this shift is required")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not check eligibility")
		return
	}
	overlaps, err := staffingPersonOverlaps(ctx, tx, place.EventID, place.ItemID, personID, place.StartsAt.Time, place.EndsAt.Time)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check schedule")
		return
	}
	if overlaps {
		writeError(w, http.StatusConflict, "shift overlaps another shift you hold for this event")
		return
	}
	if place.Filled >= place.Capacity {
		writeError(w, http.StatusConflict, "shift is full")
		return
	}

	var claimID string
	if err := tx.QueryRow(ctx, `
		insert into event_staffing_claims (staffing_item_id, event_id, person_id, application_id, source, created_by_person_id)
		values ($1, $2, $3, $4, 'self', $3)
		returning id
	`, place.ItemID, place.EventID, personID, applicationID).Scan(&claimID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not claim shift")
		return
	}
	if err := syncStaffingFillStatus(ctx, tx, place.ItemID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update shift")
		return
	}
	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	if err := a.audit(txCtx, personID, "staffing.claimed", "event_staffing_claim", claimID, map[string]any{
		"eventId":        place.EventID,
		"staffingItemId": place.ItemID,
		"source":         "self",
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save claim")
		return
	}
	place.Filled++
	writeJSON(w, http.StatusOK, openShiftDTOFromPlace(place, true, true, now))
}

func (a *App) handleReleaseOpenShift(w http.ResponseWriter, r *http.Request) {
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
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	place, err := lockStaffingPlace(ctx, tx, "", r.PathValue("staffingID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "shift not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load shift")
		return
	}
	if _, err := lockActiveMembership(ctx, tx, place.WorkspaceID, personID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	now := a.currentTime()
	claimID, err := activeStaffingClaimID(ctx, tx, place.ItemID, personID)
	if errors.Is(err, pgx.ErrNoRows) {
		if place.LegacyPersonID.Valid && place.LegacyPersonID.String == personID {
			writeError(w, http.StatusConflict, "this assignment is managed by the organizer; ask the organizer to change it")
			return
		}
		var released bool
		if err := tx.QueryRow(ctx, `
			select exists (
				select 1 from event_staffing_claims
				where staffing_item_id = $1 and person_id = $2 and status <> 'active'
			)
		`, place.ItemID, personID).Scan(&released); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load claim")
			return
		}
		if !released {
			writeError(w, http.StatusConflict, "you do not hold a place on this shift")
			return
		}
		// A retried release after success frees nothing further.
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load claim")
			return
		}
		writeJSON(w, http.StatusOK, openShiftDTOFromPlace(place, false, false, now))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load claim")
		return
	}
	if eventStatusIsClosed(place.EventStatus) {
		writeError(w, http.StatusConflict, "event is closed")
		return
	}
	if !place.accepting() {
		writeError(w, http.StatusConflict, "shift is closed")
		return
	}
	if !place.StartsAt.Valid || !staffingReleaseAllowed(now, place.StartsAt.Time, place.ReleaseCutoffHours) {
		writeError(w, http.StatusConflict, "the release window for this shift has closed; ask the organizer to release you")
		return
	}
	tag, err := tx.Exec(ctx, `
		update event_staffing_claims
		set status = 'released', ended_at = now(), ended_by_person_id = $2
		where id = $1
		  and status = 'active'
	`, claimID, personID)
	if err != nil || tag.RowsAffected() != 1 {
		writeError(w, http.StatusInternalServerError, "could not release shift")
		return
	}
	if err := syncStaffingFillStatus(ctx, tx, place.ItemID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update shift")
		return
	}
	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	if err := a.audit(txCtx, personID, "staffing.released", "event_staffing_claim", claimID, map[string]any{
		"eventId":        place.EventID,
		"staffingItemId": place.ItemID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save release")
		return
	}
	place.Filled--
	writeJSON(w, http.StatusOK, openShiftDTOFromPlace(place, false, false, now))
}

// handleRequestShiftRelease is the organizer contact action shown after the
// release cutoff. It queues a private notification to the workspace owners;
// it never reveals an owner's address to the participant and does not
// release the place by itself.
func (a *App) handleRequestShiftRelease(w http.ResponseWriter, r *http.Request) {
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
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	place, err := lockStaffingPlace(ctx, tx, "", r.PathValue("staffingID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "shift not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load shift")
		return
	}
	if _, err := lockActiveMembership(ctx, tx, place.WorkspaceID, personID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	claimID, err := activeStaffingClaimID(ctx, tx, place.ItemID, personID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "you do not hold a place on this shift")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load claim")
		return
	}
	if eventStatusIsClosed(place.EventStatus) {
		writeError(w, http.StatusConflict, "event is closed")
		return
	}

	var requesterName string
	if err := tx.QueryRow(ctx, `select coalesce(nullif(trim(display_name), ''), 'A participant') from people where id = $1`, personID).Scan(&requesterName); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load participant")
		return
	}
	owners, err := tx.Query(ctx, `
		select p.email
		from workspace_members wm
		join people p on p.id = wm.person_id
		where wm.workspace_id = $1
		  and wm.role = 'owner'
		  and wm.removed_at is null
		  and wm.revoked_at is null
		  and (wm.expires_at is null or wm.expires_at > now())
		order by wm.created_at, wm.id
	`, place.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load organizers")
		return
	}
	ownerEmails := make([]string, 0)
	for owners.Next() {
		var email string
		if err := owners.Scan(&email); err != nil {
			owners.Close()
			writeError(w, http.StatusInternalServerError, "could not load organizers")
			return
		}
		ownerEmails = append(ownerEmails, email)
	}
	owners.Close()
	if err := owners.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load organizers")
		return
	}

	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	for _, email := range ownerEmails {
		recipient := normalizeEmail(email)
		if _, err := a.enqueueNotification(txCtx, tx, enqueueNotificationParams{
			WorkspaceID:       place.WorkspaceID,
			EventID:           place.EventID,
			RecipientEmail:    recipient,
			NotificationType:  "staffing.release_request",
			RelatedType:       "event_staffing_claim",
			RelatedID:         claimID,
			IdempotencyKey:    "staffing_release_request:" + claimID + ":" + recipient,
			Subject:           fmt.Sprintf("%s: release request for %s", place.EventTitle, place.Title),
			Body:              fmt.Sprintf("%s asked to be released from %s at %s. Open the staffing board to release or keep the place.", requesterName, place.Title, place.EventTitle),
			Preview:           fmt.Sprintf("Release request for %s", place.Title),
			CreatedByPersonID: personID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "could not enqueue notification")
			return
		}
	}
	if err := a.audit(txCtx, personID, "staffing.release_requested", "event_staffing_claim", claimID, map[string]any{
		"eventId":        place.EventID,
		"staffingItemId": place.ItemID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save request")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"requested": true})
}

// lockStaffingOwner checks, inside the locked transaction, that the actor is
// a current owner of the event's workspace.
func lockStaffingOwner(ctx context.Context, tx pgx.Tx, workspaceID, actorID string) bool {
	role, err := lockActiveMembership(ctx, tx, workspaceID, actorID)
	return err == nil && role == "owner"
}

// handleAddStaffingClaim is the organizer override: it gives a current
// workspace member a place without the role policy or release cutoff, under
// the same capacity, duplicate and overlap rules as a participant claim.
func (a *App) handleAddStaffingClaim(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	actorID, ok := a.requirePersonID(r)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req addStaffingClaimRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	targetID := strings.TrimSpace(req.PersonID)
	if !validUUID(targetID) {
		writeError(w, http.StatusBadRequest, "personId must be an active workspace member")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	place, err := lockStaffingPlace(ctx, tx, r.PathValue("eventID"), r.PathValue("staffingID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "staffing item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}
	if !lockStaffingOwner(ctx, tx, place.WorkspaceID, actorID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if eventStatusIsClosed(place.EventStatus) {
		writeError(w, http.StatusConflict, "event is closed")
		return
	}
	if !place.accepting() {
		writeError(w, http.StatusConflict, "staffing item is not open")
		return
	}
	if _, err := lockActiveMembership(ctx, tx, place.WorkspaceID, targetID); err != nil {
		writeError(w, http.StatusBadRequest, "personId must be an active workspace member")
		return
	}
	existingClaimID, err := activeStaffingClaimID(ctx, tx, place.ItemID, targetID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "could not load claim")
		return
	}
	if existingClaimID == "" && !(place.LegacyPersonID.Valid && place.LegacyPersonID.String == targetID) {
		if place.StartsAt.Valid && place.EndsAt.Valid {
			overlaps, err := staffingPersonOverlaps(ctx, tx, place.EventID, place.ItemID, targetID, place.StartsAt.Time, place.EndsAt.Time)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not check schedule")
				return
			}
			if overlaps {
				writeError(w, http.StatusConflict, "person holds another overlapping shift for this event")
				return
			}
		}
		if place.Filled >= place.Capacity {
			writeError(w, http.StatusConflict, "shift is full")
			return
		}
		var claimID, recipientEmail string
		if err := tx.QueryRow(ctx, `
			insert into event_staffing_claims (staffing_item_id, event_id, person_id, source, created_by_person_id)
			values ($1, $2, $3, 'organizer', $4)
			returning id, (select email from people where id = $3)
		`, place.ItemID, place.EventID, targetID, actorID).Scan(&claimID, &recipientEmail); err != nil {
			writeError(w, http.StatusInternalServerError, "could not add assignment")
			return
		}
		if err := syncStaffingFillStatus(ctx, tx, place.ItemID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update staffing item")
			return
		}
		txCtx := context.WithValue(ctx, txContextKey{}, tx)
		if recipient := normalizeEmail(recipientEmail); recipient != "" {
			if _, err := a.enqueueNotification(txCtx, tx, enqueueNotificationParams{
				WorkspaceID:       place.WorkspaceID,
				EventID:           place.EventID,
				RecipientEmail:    recipient,
				NotificationType:  "staffing.assignment",
				RelatedType:       "event_staffing_item",
				RelatedID:         place.ItemID,
				IdempotencyKey:    "staffing_assignment:" + place.ItemID + ":" + recipient,
				Subject:           fmt.Sprintf("%s: staffing assignment for %s", place.EventTitle, place.Title),
				Body:              fmt.Sprintf("You have been assigned to %s at %s.", place.Title, place.EventTitle),
				Preview:           fmt.Sprintf("Assignment for %s", place.Title),
				CreatedByPersonID: actorID,
			}); err != nil {
				writeError(w, http.StatusInternalServerError, "could not enqueue notification")
				return
			}
		}
		if err := a.audit(txCtx, actorID, "staffing.claimed", "event_staffing_claim", claimID, map[string]any{
			"eventId":        place.EventID,
			"staffingItemId": place.ItemID,
			"source":         "organizer",
			"personId":       targetID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "could not record audit")
			return
		}
	}
	a.writeLockedStaffingItem(w, r, tx, place)
}

// handleRemoveStaffingClaim lets an owner end a place at any time, including
// after the participant release cutoff. Removing an ended place is a no-op.
func (a *App) handleRemoveStaffingClaim(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	actorID, ok := a.requirePersonID(r)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	place, err := lockStaffingPlace(ctx, tx, r.PathValue("eventID"), r.PathValue("staffingID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "staffing item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}
	if !lockStaffingOwner(ctx, tx, place.WorkspaceID, actorID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if eventStatusIsClosed(place.EventStatus) {
		writeError(w, http.StatusConflict, "event is closed")
		return
	}
	claimID := r.PathValue("claimID")
	if !validUUID(claimID) {
		writeError(w, http.StatusNotFound, "assignment not found")
		return
	}
	var status string
	if err := tx.QueryRow(ctx, `
		select status from event_staffing_claims where id = $1 and staffing_item_id = $2 for update
	`, claimID, place.ItemID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "assignment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load assignment")
		return
	}
	if status == "active" {
		if _, err := tx.Exec(ctx, `
			update event_staffing_claims
			set status = 'removed', ended_at = now(), ended_by_person_id = $2
			where id = $1 and status = 'active'
		`, claimID, actorID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not remove assignment")
			return
		}
		if err := syncStaffingFillStatus(ctx, tx, place.ItemID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update staffing item")
			return
		}
		txCtx := context.WithValue(ctx, txContextKey{}, tx)
		if err := a.audit(txCtx, actorID, "staffing.claim_removed", "event_staffing_claim", claimID, map[string]any{
			"eventId":        place.EventID,
			"staffingItemId": place.ItemID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "could not record audit")
			return
		}
	}
	a.writeLockedStaffingItem(w, r, tx, place)
}

func (a *App) writeLockedStaffingItem(w http.ResponseWriter, r *http.Request, tx pgx.Tx, place staffingPlace) {
	ctx := r.Context()
	row, err := loadEventStaffingItemRow(ctx, tx, place.EventID, place.ItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}
	items := []eventStaffingItemDTO{eventStaffingItemDTOFromRow(row)}
	if err := attachStaffingClaims(ctx, tx, place.EventID, items); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load staffing item")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save staffing item")
		return
	}
	writeJSON(w, http.StatusOK, items[0])
}

// attachStaffingClaims adds the organizer-only place list, eligible-role
// policy and filled count to staffing DTOs of one event.
func attachStaffingClaims(ctx context.Context, q staffingQuerier, eventID string, items []eventStaffingItemDTO) error {
	if len(items) == 0 {
		return nil
	}
	index := make(map[string]int, len(items))
	for i := range items {
		index[items[i].ID] = i
	}
	claims, err := q.Query(ctx, `
		select c.id, c.staffing_item_id, c.person_id,
		       coalesce(nullif(trim(p.display_name), ''), p.email), c.source, c.created_at
		from event_staffing_claims c
		join people p on p.id = c.person_id
		where c.event_id = $1
		  and c.status = 'active'
		order by c.created_at, c.id
	`, eventID)
	if err != nil {
		return err
	}
	for claims.Next() {
		var claim staffingClaimDTO
		var itemID string
		var createdAt time.Time
		if err := claims.Scan(&claim.ID, &itemID, &claim.PersonID, &claim.Name, &claim.Source, &createdAt); err != nil {
			claims.Close()
			return err
		}
		claim.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		if i, ok := index[itemID]; ok {
			items[i].Claims = append(items[i].Claims, claim)
			items[i].FilledCount++
		}
	}
	claims.Close()
	if err := claims.Err(); err != nil {
		return err
	}
	roles, err := q.Query(ctx, `
		select cr.staffing_item_id, cr.role_id
		from event_staffing_claim_roles cr
		join event_staffing_items esi on esi.id = cr.staffing_item_id
		where esi.event_id = $1
		order by cr.created_at, cr.role_id
	`, eventID)
	if err != nil {
		return err
	}
	defer roles.Close()
	for roles.Next() {
		var itemID, roleID string
		if err := roles.Scan(&itemID, &roleID); err != nil {
			return err
		}
		if i, ok := index[itemID]; ok {
			items[i].ClaimRoleIDs = append(items[i].ClaimRoleIDs, roleID)
		}
	}
	return roles.Err()
}
