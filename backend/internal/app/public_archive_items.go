package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
)

type publicArchiveItemDTO struct {
	ID                string  `json:"id"`
	EventID           string  `json:"eventId"`
	ReplacesItemID    *string `json:"replacesItemId,omitempty"`
	Kind              string  `json:"kind"`
	Title             string  `json:"title"`
	AttributionName   string  `json:"attributionName"`
	AttributionURL    *string `json:"attributionUrl,omitempty"`
	ExternalURL       *string `json:"externalUrl,omitempty"`
	IntendedUse       string  `json:"intendedUse"`
	RightsAssertion   string  `json:"rightsAssertion"`
	EvidenceReference string  `json:"evidenceReference"`
	Status            string  `json:"status"`
	UnavailableReason string  `json:"unavailableReason"`
	ApprovedAt        string  `json:"approvedAt"`
	CreatedAt         string  `json:"createdAt"`
}
type createPublicArchiveItemRequest struct {
	Kind              string  `json:"kind"`
	Title             string  `json:"title"`
	AttributionName   string  `json:"attributionName"`
	AttributionURL    *string `json:"attributionUrl"`
	ExternalURL       *string `json:"externalUrl"`
	IntendedUse       string  `json:"intendedUse"`
	RightsAssertion   string  `json:"rightsAssertion"`
	EvidenceReference string  `json:"evidenceReference"`
}
type unavailablePublicArchiveItemRequest struct {
	Reason string `json:"reason"`
}

func validArchiveURL(value *string) (*string, bool) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, true
	}
	v := strings.TrimSpace(*value)
	if len(v) > 2000 || strings.IndexFunc(v, unicode.IsControl) >= 0 {
		return nil, false
	}
	u, e := url.Parse(v)
	if e != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return nil, false
	}
	return &v, true
}
func validArchiveChoice(v string, choices ...string) bool {
	for _, c := range choices {
		if v == c {
			return true
		}
	}
	return false
}
func (a *App) handleListPublicArchiveItems(w http.ResponseWriter, r *http.Request) {
	event, ok := a.authorizeEndedArchiveEvent(w, r)
	if !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `select id,event_id,replaces_item_id,kind,title,attribution_name,attribution_url,external_url,intended_use,rights_assertion,evidence_reference,status,unavailable_reason,approved_at,created_at from event_public_archive_items where event_id=$1 order by created_at,id`, event.ID)
	if err != nil {
		writeError(w, 500, "could not load public archive items")
		return
	}
	defer rows.Close()
	out := []publicArchiveItemDTO{}
	for rows.Next() {
		var x publicArchiveItemDTO
		var rep, au, eu sql.NullString
		var approved, created time.Time
		if err := rows.Scan(&x.ID, &x.EventID, &rep, &x.Kind, &x.Title, &x.AttributionName, &au, &eu, &x.IntendedUse, &x.RightsAssertion, &x.EvidenceReference, &x.Status, &x.UnavailableReason, &approved, &created); err != nil {
			writeError(w, 500, "could not load public archive items")
			return
		}
		x.ReplacesItemID = nullableString(rep)
		x.AttributionURL = nullableString(au)
		x.ExternalURL = nullableString(eu)
		x.ApprovedAt = approved.UTC().Format(time.RFC3339Nano)
		x.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "could not load public archive items")
		return
	}
	if _, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, "owner"); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, 200, out)
}
func (a *App) authorizeEndedArchiveEvent(w http.ResponseWriter, r *http.Request) (eventRow, bool) {
	event, err := a.loadEventDetails(r.Context(), r.PathValue("eventID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "event not found")
		} else {
			writeError(w, 500, "could not load event")
		}
		return event, false
	}
	_, role, ok := a.requireWorkspaceRole(r, event.WorkspaceID, "owner")
	if !ok || role != "owner" {
		writeError(w, 403, "forbidden")
		return event, false
	}
	if event.Status != "end_of_night" {
		writeError(w, 409, "event must be ended before approving archive items")
		return event, false
	}
	return event, true
}

// lockEndedArchiveEvent rechecks the actor while holding the event and
// membership rows used by an archive mutation. A later revoke waits for this
// decision instead of invalidating an already-authorized write mid-flight.
func lockEndedArchiveEvent(ctx context.Context, tx pgx.Tx, eventID, actorID string) (eventRow, error) {
	var event eventRow
	err := tx.QueryRow(ctx, `
		select id, workspace_id, status from events where id=$1 for update
	`, eventID).Scan(&event.ID, &event.WorkspaceID, &event.Status)
	if err != nil {
		return event, err
	}
	if event.Status != "end_of_night" {
		return event, ErrPermissionDenied
	}
	var role string
	err = tx.QueryRow(ctx, `
		select role from workspace_members
		where workspace_id=$1 and person_id=$2 and role='owner'
		  and removed_at is null and revoked_at is null
		  and (expires_at is null or expires_at > clock_timestamp())
		for update
	`, event.WorkspaceID, actorID).Scan(&role)
	if err != nil {
		return event, err
	}
	return event, nil
}
func (a *App) handleCreatePublicArchiveItem(w http.ResponseWriter, r *http.Request) {
	event, ok := a.authorizeEndedArchiveEvent(w, r)
	if !ok {
		return
	}
	actor, _, _ := a.requireWorkspaceRole(r, event.WorkspaceID, "owner")
	var q createPublicArchiveItemRequest
	if decodeJSON(r, &q) != nil {
		writeError(w, 400, "invalid json")
		return
	}
	q.Title = strings.TrimSpace(q.Title)
	q.AttributionName = strings.TrimSpace(q.AttributionName)
	q.EvidenceReference = strings.TrimSpace(q.EvidenceReference)
	au, ok1 := validArchiveURL(q.AttributionURL)
	eu, ok2 := validArchiveURL(q.ExternalURL)
	if q.Title == "" || len(q.Title) > 300 || q.AttributionName == "" || len(q.AttributionName) > 300 || len(q.EvidenceReference) > 500 || !ok1 || !ok2 || !validArchiveChoice(q.Kind, "credit", "link") || !validArchiveChoice(q.IntendedUse, "link_only", "display_credit") || !validArchiveChoice(q.RightsAssertion, "owned", "licensed", "permission_asserted", "public_domain") {
		writeError(w, 400, "invalid public archive item")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not start archive approval")
		return
	}
	defer tx.Rollback(r.Context())
	if event, err = lockEndedArchiveEvent(r.Context(), tx, event.ID, actor); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var x publicArchiveItemDTO
	var auOut, euOut sql.NullString
	var approved, created time.Time
	err = tx.QueryRow(r.Context(), `insert into event_public_archive_items(workspace_id,event_id,kind,title,attribution_name,attribution_url,external_url,intended_use,rights_assertion,evidence_reference,approved_by_person_id) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) returning id,event_id,kind,title,attribution_name,attribution_url,external_url,intended_use,rights_assertion,evidence_reference,status,unavailable_reason,approved_at,created_at`, event.WorkspaceID, event.ID, q.Kind, q.Title, q.AttributionName, au, eu, q.IntendedUse, q.RightsAssertion, q.EvidenceReference, actor).Scan(&x.ID, &x.EventID, &x.Kind, &x.Title, &x.AttributionName, &auOut, &euOut, &x.IntendedUse, &x.RightsAssertion, &x.EvidenceReference, &x.Status, &x.UnavailableReason, &approved, &created)
	if err != nil {
		writeError(w, 500, "could not create public archive item")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "could not create public archive item")
		return
	}
	x.AttributionURL = nullableString(auOut)
	x.ExternalURL = nullableString(euOut)
	x.ApprovedAt = approved.UTC().Format(time.RFC3339Nano)
	x.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	writeJSON(w, 201, x)
}

func (a *App) handleUnavailablePublicArchiveItem(w http.ResponseWriter, r *http.Request) {
	event, ok := a.authorizeEndedArchiveEvent(w, r)
	if !ok {
		return
	}
	actor, _ := a.requirePersonID(r)
	var request unavailablePublicArchiveItemRequest
	if decodeJSON(r, &request) != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" || len(reason) > 500 {
		writeError(w, http.StatusBadRequest, "invalid unavailable reason")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not start archive update")
		return
	}
	defer tx.Rollback(r.Context())
	if event, err = lockEndedArchiveEvent(r.Context(), tx, event.ID, actor); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `update event_public_archive_items set status='unavailable', unavailable_reason=$3 where id=$1 and event_id=$2 and workspace_id=$4 and status='approved' returning id`, r.PathValue("itemID"), event.ID, reason, event.WorkspaceID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "approved public archive item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not mark public archive item unavailable")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not mark public archive item unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "unavailable"})
}

func (a *App) handleCorrectPublicArchiveItem(w http.ResponseWriter, r *http.Request) {
	event, ok := a.authorizeEndedArchiveEvent(w, r)
	if !ok {
		return
	}
	actor, _, _ := a.requireWorkspaceRole(r, event.WorkspaceID, "owner")
	var q createPublicArchiveItemRequest
	if decodeJSON(r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	q.Title, q.AttributionName, q.EvidenceReference = strings.TrimSpace(q.Title), strings.TrimSpace(q.AttributionName), strings.TrimSpace(q.EvidenceReference)
	au, goodAU := validArchiveURL(q.AttributionURL)
	eu, goodEU := validArchiveURL(q.ExternalURL)
	if q.Title == "" || len(q.Title) > 300 || q.AttributionName == "" || len(q.AttributionName) > 300 || len(q.EvidenceReference) > 500 || !goodAU || !goodEU || !validArchiveChoice(q.Kind, "credit", "link") || !validArchiveChoice(q.IntendedUse, "link_only", "display_credit") || !validArchiveChoice(q.RightsAssertion, "owned", "licensed", "permission_asserted", "public_domain") {
		writeError(w, http.StatusBadRequest, "invalid public archive item")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not start correction")
		return
	}
	defer tx.Rollback(r.Context())
	if event, err = lockEndedArchiveEvent(r.Context(), tx, event.ID, actor); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var old string
	if err := tx.QueryRow(r.Context(), `update event_public_archive_items set status='corrected' where id=$1 and event_id=$2 and workspace_id=$3 and status='approved' returning id`, r.PathValue("itemID"), event.ID, event.WorkspaceID).Scan(&old); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "approved public archive item not found")
		return
	} else if err != nil {
		writeError(w, 500, "could not correct public archive item")
		return
	}
	var replacement publicArchiveItemDTO
	var auOut, euOut sql.NullString
	var approved, created time.Time
	if err := tx.QueryRow(r.Context(), `insert into event_public_archive_items(workspace_id,event_id,replaces_item_id,kind,title,attribution_name,attribution_url,external_url,intended_use,rights_assertion,evidence_reference,approved_by_person_id) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) returning id,event_id,replaces_item_id,kind,title,attribution_name,attribution_url,external_url,intended_use,rights_assertion,evidence_reference,status,unavailable_reason,approved_at,created_at`, event.WorkspaceID, event.ID, old, q.Kind, q.Title, q.AttributionName, au, eu, q.IntendedUse, q.RightsAssertion, q.EvidenceReference, actor).Scan(&replacement.ID, &replacement.EventID, &replacement.ReplacesItemID, &replacement.Kind, &replacement.Title, &replacement.AttributionName, &auOut, &euOut, &replacement.IntendedUse, &replacement.RightsAssertion, &replacement.EvidenceReference, &replacement.Status, &replacement.UnavailableReason, &approved, &created); err != nil {
		writeError(w, 500, "could not create correction")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "could not save correction")
		return
	}
	replacement.AttributionURL = nullableString(auOut)
	replacement.ExternalURL = nullableString(euOut)
	replacement.ApprovedAt = approved.UTC().Format(time.RFC3339Nano)
	replacement.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	writeJSON(w, http.StatusCreated, replacement)
}
