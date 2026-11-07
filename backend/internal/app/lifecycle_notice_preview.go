package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const lifecycleNoticeRecipientLimit = 500

type lifecycleNoticeRecipientDTO struct {
	Email      string `json:"email"`
	SourceType string `json:"sourceType"`
	Suppressed bool   `json:"suppressed"`
}

type lifecycleNoticePreviewDTO struct {
	ChangeID     string                        `json:"changeId"`
	OccurrenceID string                        `json:"occurrenceId"`
	Revision     string                        `json:"revision"`
	PublicCID    string                        `json:"publicCid"`
	Audiences    []string                      `json:"audiences"`
	Subject      string                        `json:"subject"`
	Body         string                        `json:"body"`
	Recipients   []lifecycleNoticeRecipientDTO `json:"recipients"`
	PreviewHash  string                        `json:"previewHash"`
}

// A preview is a point-in-time review, never sending authority. Future approval
// must recompute the content, recipients and digest inside its transaction.
func (a *App) handlePreviewLifecycleNotice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.db == nil {
		writeError(w, 500, "database unavailable")
		return
	}
	var req struct {
		Audiences []string `json:"audiences"`
	}
	if decodeJSON(r, &req) != nil || !validLifecycleNoticeAudiences(req.Audiences) {
		writeError(w, 400, "select ticket_holders or assigned_crew, without duplicates")
		return
	}
	if _, err := uuid.Parse(r.PathValue("changeID")); err != nil {
		writeError(w, 400, "invalid lifecycle intent ID")
		return
	}
	event, err := a.loadEventDetails(r.Context(), r.PathValue("eventID"))
	if err != nil {
		lifecycleIntentEventError(w, err)
		return
	}
	if _, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, roleOwner); !ok {
		writeError(w, 403, "forbidden")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(w, 500, "could not preview notice")
		return
	}
	defer tx.Rollback(r.Context())
	ctx := r.Context()
	preview, _, err := buildLifecycleNoticePreview(ctx, tx, event.ID, event.WorkspaceID, r.PathValue("changeID"), req.Audiences, false)
	if err != nil {
		writeNoticeError(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, 500, "could not preview notice")
		return
	}
	if _, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, roleOwner); !ok {
		writeError(w, 403, "forbidden")
		return
	}
	writeJSON(w, 200, preview)
}

type noticeHTTPError struct {
	Status  int
	Message string
}

func (e *noticeHTTPError) Error() string { return e.Message }
func writeNoticeError(w http.ResponseWriter, err error) {
	var detail *noticeHTTPError
	if errors.As(err, &detail) {
		writeError(w, detail.Status, detail.Message)
	} else {
		writeError(w, 500, "could not process listing notice")
	}
}
func buildLifecycleNoticePreview(ctx context.Context, tx pgx.Tx, eventID, workspaceID, changeID string, audiences []string, lockOccurrence bool) (lifecycleNoticePreviewDTO, []string, error) {
	var kind, status, revision string
	var snapshot json.RawMessage
	var owner, noticeAction bool
	err := tx.QueryRow(ctx, `select c.kind,c.status,c.target_revision,c.decision_snapshot,
	  exists(select 1 from workspace_members wm where wm.workspace_id=c.workspace_id
	    and wm.person_id=c.approved_by_person_id and wm.role='owner'
	    and wm.removed_at is null and wm.revoked_at is null
	    and (wm.expires_at is null or wm.expires_at>clock_timestamp())),
	  exists(select 1 from event_lifecycle_actions la where la.change_id=c.id and la.action_kind='operational_notice')
	  from event_lifecycle_changes c where c.id=$1 and c.event_id=$2 and c.workspace_id=$3`,
		changeID, eventID, workspaceID).Scan(&kind, &status, &revision, &snapshot, &owner, &noticeAction)
	if errors.Is(err, pgx.ErrNoRows) {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{404, "lifecycle intent not found"}
	}
	if err != nil {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{500, "could not preview notice"}
	}
	if status != "approved" || !owner || !noticeAction {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "notice requires an active owner decision with an operational notice action"}
	}
	decision, err := lifecycleSnapshotStrings(snapshot)
	if err != nil || decision["occurrenceId"] == "" || decision["expectedUpdatedAt"] != revision {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "intent has no exact listing revision"}
	}
	if _, err := uuid.Parse(decision["occurrenceId"]); err != nil {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "intent has no exact listing revision"}
	}
	// Missing CID must not silently become an explicit empty CID condition.
	var cidCondition struct {
		CID *string `json:"expectedPublicCid"`
	}
	if json.Unmarshal(snapshot, &cidCondition) != nil || cidCondition.CID == nil {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "intent has no exact public revision"}
	}
	lockClause := ""
	if lockOccurrence {
		lockClause = " for update"
	}
	occurrence, err := scanOccurrenceRow(tx.QueryRow(ctx, `select `+occurrenceSelectColumns+` from event_occurrences where id=$1 and event_id=$2 and workspace_id=$3`+lockClause, decision["occurrenceId"], eventID, workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "listing is no longer available"}
	}
	if err != nil {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{500, "could not preview notice"}
	}
	expected, err := time.Parse(time.RFC3339Nano, revision)
	if err != nil || !expected.Equal(occurrence.UpdatedAt) || decision["expectedPublicCid"] != occurrence.PublicCID.String {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "listing changed; record a decision against its current revision"}
	}
	if (kind != "cancellation" || occurrence.Status != "cancelled") && (kind != "reschedule" || occurrence.Status != "rescheduled") {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "save the listing cancellation or reschedule before previewing its notice"}
	}
	sort.Strings(audiences)
	preview := lifecycleNoticePreviewDTO{ChangeID: changeID, OccurrenceID: occurrence.ID, Revision: revision, PublicCID: occurrence.PublicCID.String, Audiences: audiences, Recipients: []lifecycleNoticeRecipientDTO{}}
	preview.Subject, preview.Body = lifecycleNoticeContent(occurrence)
	if len(preview.Subject) > 1000 || len(preview.Body) > 16384 {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "listing content is too long for a notice"}
	}
	rows, err := tx.Query(ctx, lifecycleNoticeRecipientsSQL, eventID, workspaceID, audiences, lifecycleNoticeRecipientLimit+1)
	if err != nil {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{500, "could not preview recipients"}
	}
	defer rows.Close()
	bindings := []string{}
	for rows.Next() {
		var recipient lifecycleNoticeRecipientDTO
		var sourceID string
		if err := rows.Scan(&recipient.Email, &recipient.SourceType, &sourceID, &recipient.Suppressed); err != nil {
			return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{500, "could not preview recipients"}
		}
		address, err := mail.ParseAddress(recipient.Email)
		if err != nil || address.Address != recipient.Email || len(recipient.Email) > 320 {
			return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "an operational recipient has an invalid email address; correct it before review"}
		}
		preview.Recipients = append(preview.Recipients, recipient)
		bindings = append(bindings, sourceID)
	}
	if rows.Err() != nil {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{500, "could not preview recipients"}
	}
	rows.Close()
	if len(preview.Recipients) > lifecycleNoticeRecipientLimit {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{409, "notice exceeds the 500-recipient review limit"}
	}
	canonical, err := json.Marshal(struct {
		Preview  lifecycleNoticePreviewDTO
		Bindings []string
	}{preview, bindings})
	if err != nil {
		return lifecycleNoticePreviewDTO{}, nil, &noticeHTTPError{500, "could not preview notice"}
	}
	digest := sha256.Sum256(canonical)
	preview.PreviewHash = hex.EncodeToString(digest[:])
	return preview, bindings, nil
}

func validLifecycleNoticeAudiences(audiences []string) bool {
	if len(audiences) < 1 || len(audiences) > 2 {
		return false
	}
	seen := map[string]bool{}
	for _, audience := range audiences {
		if (audience != "ticket_holders" && audience != "assigned_crew") || seen[audience] {
			return false
		}
		seen[audience] = true
	}
	return true
}

func lifecycleNoticeContent(o eventOccurrenceRow) (string, string) {
	// Header folding and private decision/staffing text never enter this template.
	name := strings.Join(strings.Fields(o.Name), " ")
	status := "cancelled"
	if o.Status == "rescheduled" {
		status = "rescheduled"
	}
	subject := fmt.Sprintf("Listing %s: %s", status, name)
	body := fmt.Sprintf("The listing for %s has been %s.\n", name, status)
	if status == "rescheduled" {
		body += fmt.Sprintf("Listed start: %s\n", o.StartsAt.UTC().Format(time.RFC3339))
		if o.EndsAt.Valid {
			body += fmt.Sprintf("Listed end: %s\n", o.EndsAt.Time.UTC().Format(time.RFC3339))
		}
		if o.Timezone.Valid {
			body += fmt.Sprintf("Listing timezone: %s\n", o.Timezone.String)
		}
		if o.AllDay {
			body += "This is an all-day listing.\n"
		}
	}
	body += "\nThis notice describes the public listing only. It does not change your ticket, payment, refund or crew assignment. The organizer must separately confirm any changes to the event plan.\n"
	return subject, body
}

// A stable representative source binds each normalized mailbox. This list is
// derived from operational relationships, never contact or marketing consent.
const lifecycleNoticeRecipientsSQL = `with candidates as (
 select lower(btrim(t.email)) email,'ticket' source_type,t.id source_id
 from tickets t where t.event_id=$1 and t.status in ('reserved','checked_in')
   and t.payment_status in ('free','pending','paid') and 'ticket_holders'=any($3::text[])
 union all
 select lower(btrim(p.email)),'crew_person',p.id
 from event_staffing_items s join people p on p.id=s.assigned_person_id
 join workspace_members wm on wm.person_id=p.id and wm.workspace_id=$2
 where s.event_id=$1 and s.status='assigned' and 'assigned_crew'=any($3::text[])
   and wm.removed_at is null and wm.revoked_at is null
   and (wm.expires_at is null or wm.expires_at>clock_timestamp())
 union all
 select lower(btrim(p.email)),'crew_person',p.id
 from event_staffing_claims c join event_staffing_items s on s.id=c.staffing_item_id
 join people p on p.id=c.person_id
 join workspace_members wm on wm.person_id=p.id and wm.workspace_id=$2
 where c.event_id=$1 and c.status='active' and s.status in ('open','assigned') and 'assigned_crew'=any($3::text[])
   and wm.removed_at is null and wm.revoked_at is null
   and (wm.expires_at is null or wm.expires_at>clock_timestamp())
 union all
 select lower(btrim(ap.applicant_email)),'crew_application',ap.id
 from event_staffing_items s join event_role_applications ap on ap.id=s.assigned_application_id and ap.event_id=s.event_id
 where s.event_id=$1 and s.status='assigned' and ap.status in ('accepted','confirmed')
   and 'assigned_crew'=any($3::text[])
), recipients as (select distinct on (email) email,source_type,source_id from candidates order by email,source_type,source_id)
select r.email,r.source_type,r.source_id,exists(select 1 from email_suppressions es where es.recipient_email=r.email)
from recipients r order by r.email limit $4`
