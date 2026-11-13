package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const doorLinkHeader = "X-Door-Link"

type doorLinkKey struct{}
type doorLinkDTO struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	ValidFrom      string  `json:"validFrom"`
	ValidUntil     string  `json:"validUntil"`
	RevokedAt      *string `json:"revokedAt"`
	CheckedInCount int     `json:"checkedInCount"`
}
type doorLinkCheckIns struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// A link is independent of a session and is never accepted by operator routes.
// The URL fragment is sent in a header, so neither proxy URLs nor Referrer
// headers contain the bearer value. Only its hash is stored.
func (a *App) withDoorLink(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		token := r.Header.Get(doorLinkHeader)
		if a.db == nil || len(token) < 32 || len(token) > 128 || !validUUID(r.PathValue("eventID")) {
			writeError(w, http.StatusForbidden, "door link unavailable")
			return
		}
		var id string
		var count int
		// Rate accounting is shared by all API processes, and invalid/expired links
		// cannot update it. Search and check-in share a sixty-request minute budget.
		err := a.db.QueryRow(r.Context(), `update event_door_links l set
    rate_count = case when rate_window <= clock_timestamp() - interval '1 minute' then 1 else least(rate_count + 1, 61) end,
    rate_window = case when rate_window <= clock_timestamp() - interval '1 minute' then clock_timestamp() else rate_window end
    from events e where l.event_id = e.id and e.id = $1 and l.token_hash = $2
    and l.revoked_at is null and l.valid_from <= clock_timestamp() and l.valid_until > clock_timestamp()
    and e.status = 'published'
    and not exists (select 1 from event_occurrences o where o.event_id=e.id and o.status='cancelled') returning l.id, l.rate_count`, r.PathValue("eventID"), tokenHash(token)).Scan(&id, &count)
		if err != nil {
			writeError(w, http.StatusForbidden, "door link unavailable")
			return
		}
		if count > 60 {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "too many door requests; try again in a minute")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), doorLinkKey{}, id)))
	}
}

// Recheck after the ticket lock; a queued check-in cannot use an authority
// decision made before revocation, expiry or cancellation. A share lock also
// serializes revocation against the committing check-in.
func (a *App) doorActor(r *http.Request, event eventRow, q queryRower) (personID, linkID string, ok bool) {
	if id, isLink := r.Context().Value(doorLinkKey{}).(string); isLink {
		err := q.QueryRow(r.Context(), `select l.id from event_door_links l join events e on e.id=l.event_id
    where l.id=$1 and l.event_id=$2 and l.revoked_at is null
    and l.valid_from <= clock_timestamp() and l.valid_until > clock_timestamp()
    and e.status='published'
    and not exists (select 1 from event_occurrences o where o.event_id=e.id and o.status='cancelled') for share of l,e`, id, event.ID).Scan(&linkID)
		return "", linkID, err == nil
	}
	personID, ok = a.requirePersonID(r)
	if !ok || authorizeWith(r.Context(), q, personID, event.WorkspaceID, permDoor) != nil {
		return "", "", false
	}
	return personID, "", true
}

func (a *App) doorLinkOwner(w http.ResponseWriter, r *http.Request) (eventRow, string, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.db == nil || !validUUID(r.PathValue("eventID")) {
		writeError(w, http.StatusNotFound, "event not found")
		return eventRow{}, "", false
	}
	event, err := a.loadEventDetails(r.Context(), r.PathValue("eventID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "event not found")
		return eventRow{}, "", false
	}
	actor, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, roleOwner)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return eventRow{}, "", false
	}
	return event, actor, true
}

func (a *App) handleCreateDoorLink(w http.ResponseWriter, r *http.Request) {
	event, actor, ok := a.doorLinkOwner(w, r)
	if !ok {
		return
	}
	var req struct {
		Label      string    `json:"label"`
		ValidFrom  time.Time `json:"validFrom"`
		ValidUntil time.Time `json:"validUntil"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	if utf8.RuneCountInString(req.Label) < 1 || utf8.RuneCountInString(req.Label) > 80 || req.ValidFrom.IsZero() || !req.ValidUntil.After(req.ValidFrom) || req.ValidUntil.Sub(req.ValidFrom) > 7*24*time.Hour || !req.ValidUntil.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "a label and a future time window of at most seven days are required")
		return
	}
	if event.Status != eventStatusPublished {
		writeError(w, http.StatusConflict, "publish the event before creating a door link")
		return
	}
	token, hash, err := newToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create door link")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create door link")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var id string
	if err := tx.QueryRow(r.Context(), `insert into event_door_links(event_id,label,token_hash,valid_from,valid_until,created_by_person_id) values($1,$2,$3,$4,$5,$6) returning id`, event.ID, req.Label, hash, req.ValidFrom, req.ValidUntil, actor).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create door link")
		return
	}
	if _, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, roleOwner); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := a.audit(context.WithValue(r.Context(), txContextKey{}, tx), actor, "door_link.created", "door_link", id, map[string]any{"eventId": event.ID, "label": req.Label}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save door link")
		return
	}
	url := strings.TrimRight(a.config.PublicWebURL, "/") + "/door-link/" + event.ID + "#" + token
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "url": url})
}

func (a *App) handleListDoorLinks(w http.ResponseWriter, r *http.Request) {
	event, _, ok := a.doorLinkOwner(w, r)
	if !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `select l.id,l.label,l.valid_from,l.valid_until,l.revoked_at,(select count(*) from tickets t where t.checked_in_by_door_link_id=l.id) from event_door_links l where l.event_id=$1 order by l.created_at desc`, event.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load door links")
		return
	}
	defer rows.Close()
	links := []doorLinkDTO{}
	for rows.Next() {
		var l doorLinkDTO
		var start, end time.Time
		var revoked sql.NullTime
		if err := rows.Scan(&l.ID, &l.Label, &start, &end, &revoked, &l.CheckedInCount); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load door links")
			return
		}
		l.ValidFrom = start.UTC().Format(time.RFC3339Nano)
		l.ValidUntil = end.UTC().Format(time.RFC3339Nano)
		l.RevokedAt = nullableTimeString(revoked)
		links = append(links, l)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "could not load door links")
		return
	}
	if _, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, roleOwner); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusOK, links)
}
func (a *App) handleRevokeDoorLink(w http.ResponseWriter, r *http.Request) {
	event, actor, ok := a.doorLinkOwner(w, r)
	if !ok {
		return
	}
	if !validUUID(r.PathValue("linkID")) {
		writeError(w, http.StatusNotFound, "door link not found")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke door link")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var id string
	err = tx.QueryRow(r.Context(), `update event_door_links set revoked_at=coalesce(revoked_at,clock_timestamp()) where id=$1 and event_id=$2 returning id`, r.PathValue("linkID"), event.ID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "door link not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke door link")
		return
	}
	if _, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, roleOwner); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := a.audit(context.WithValue(r.Context(), txContextKey{}, tx), actor, "door_link.revoked", "door_link", id, map[string]any{"eventId": event.ID}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke door link")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

func loadDoorLinkCheckIns(ctx context.Context, tx pgx.Tx, eventID string) ([]doorLinkCheckIns, error) {
	rows, err := tx.Query(ctx, `select l.label,count(t.id) from event_door_links l left join tickets t on t.checked_in_by_door_link_id=l.id where l.event_id=$1 group by l.id,l.label order by l.created_at`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []doorLinkCheckIns{}
	for rows.Next() {
		var item doorLinkCheckIns
		if err := rows.Scan(&item.Label, &item.Count); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
