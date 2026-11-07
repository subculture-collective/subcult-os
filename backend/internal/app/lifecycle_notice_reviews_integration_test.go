package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLifecycleNoticeReviewPrivacyReplayAndFrozenObservation(t *testing.T) {
	fx, eventID, _, previewPath := noticePreviewFixture(t)
	addNoticeTicket(t, fx, eventID, "guest@example.test")
	notice, _ := approveNotice(t, fx, previewPath)
	path := strings.TrimSuffix(previewPath, "-preview")
	req := map[string]any{"requestKey": uuid.NewString(), "note": "Check provider records before considering a correction."}
	postJSON(t, fx.app, fx.memberCookie, path+"/reviews", req, 403)
	getJSON(t, fx.app, fx.memberCookie, path, 403)
	postJSON(t, fx.app, fx.ownerCookie, path+"/reviews", map[string]any{"requestKey": uuid.NewString(), "note": "   "}, 400)
	postJSON(t, fx.app, fx.ownerCookie, path+"/reviews", map[string]any{"requestKey": uuid.NewString(), "note": strings.Repeat("x", 2001)}, 400)
	var before string
	if err := fx.app.db.QueryRow(t.Context(), `select jsonb_agg(to_jsonb(e) order by e.id)::text from email_outbox e`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	created := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, path+"/reviews", req, 201).JSON)
	first := mustObject(t, created["reviews"].([]any)[0])
	observed := mustObject(t, first["recipients"].([]any)[0])
	if observed["status"] != "held" || observed["feedback"] != "unknown" || observed["attempts"] != float64(0) {
		t.Fatalf("observation=%v", observed)
	}
	var after string
	if err := fx.app.db.QueryRow(t.Context(), `select jsonb_agg(to_jsonb(e) order by e.id)::text from email_outbox e`).Scan(&after); err != nil || before != after {
		t.Fatalf("review mutated mail: err=%v", err)
	}
	// Synthetic later outcome: the observation must remain held on exact replay.
	if _, err := fx.app.db.Exec(t.Context(), `update email_outbox set delivery_status='accepted',attempts=1,feedback_rank=1 where id in (select outbox_id from lifecycle_notice_recipients where notice_id=$1)`, notice["id"]); err != nil {
		t.Fatal(err)
	}
	replayed := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, path+"/reviews", req, 200).JSON)
	reviews := replayed["reviews"].([]any)
	if len(reviews) != 1 || mustObject(t, reviews[0])["id"] != first["id"] || mustObject(t, mustObject(t, reviews[0])["recipients"].([]any)[0])["status"] != "held" {
		t.Fatalf("replay changed observation: %v", reviews)
	}
	if mustObject(t, replayed["recipients"].([]any)[0])["feedback"] != "delivered" {
		t.Fatal("live feedback did not advance independently")
	}
	req["note"] = "Changed binding"
	postJSON(t, fx.app, fx.ownerCookie, path+"/reviews", req, 409)
	encoded, _ := json.Marshal(replayed)
	for _, private := range []string{"requestKey", "recorded_by_person_id", "provider_message_id", "lease_token"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private field leaked: %s", private)
		}
	}
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at=now() where workspace_id=$1 and person_id=$2`, fx.workspaceID, ownerPersonID(t, fx)); err != nil {
		t.Fatal(err)
	}
	getJSON(t, fx.app, fx.ownerCookie, path, 403)
	postJSON(t, fx.app, fx.ownerCookie, path+"/reviews", req, 403)
}

func TestLifecycleNoticeReviewConcurrentReplayAndAuditRollback(t *testing.T) {
	fx, eventID, _, previewPath := noticePreviewFixture(t)
	addNoticeTicket(t, fx, eventID, "guest@example.test")
	approveNotice(t, fx, previewPath)
	path := strings.TrimSuffix(previewPath, "-preview") + "/reviews"
	req := map[string]any{"requestKey": uuid.NewString(), "note": "Owner follow-up"}
	if _, err := fx.app.db.Exec(t.Context(), `alter table audit_entries add constraint synthetic_review_audit_failure check(action<>'lifecycle_notice.reviewed')`); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.ownerCookie, path, req, 500)
	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from lifecycle_notice_reviews`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("review survived failed audit: %d %v", count, err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `alter table audit_entries drop constraint synthetic_review_audit_failure`); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(req)
	results := make(chan int, 2)
	for range 2 {
		go func() {
			r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(fx.ownerCookie)
			w := httptest.NewRecorder()
			fx.app.Handler().ServeHTTP(w, r)
			results <- w.Code
		}()
	}
	first, second := <-results, <-results
	if !((first == 201 && second == 200) || (first == 200 && second == 201)) {
		t.Fatalf("statuses=%d,%d", first, second)
	}
	var audit int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from lifecycle_notice_reviews),(select count(*) from audit_entries where action='lifecycle_notice.reviewed')`).Scan(&count, &audit); err != nil || count != 1 || audit != 1 {
		t.Fatalf("review/audit=%d/%d err=%v", count, audit, err)
	}
}

func TestLifecycleNoticeReviewMigrationPreservesApprovedQueue(t *testing.T) {
	fx, eventID, _, previewPath := noticePreviewFixture(t)
	addNoticeTicket(t, fx, eventID, "guest@example.test")
	approveNotice(t, fx, previewPath)
	var before string
	if err := fx.app.db.QueryRow(t.Context(), `select jsonb_agg(to_jsonb(e) order by e.id)::text from email_outbox e`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `alter table email_outbox drop constraint email_outbox_provider_id_length; alter table email_provider_events drop constraint email_events_provider_id_length; alter table email_outbox drop column provider; alter table email_outbox alter column provider_message_id type uuid using provider_message_id::uuid; alter table email_provider_events alter column provider_message_id type uuid using provider_message_id::uuid; drop table venue_access_place_requests;drop table event_access_revisions; drop table lifecycle_notice_reviews; drop table event_staffing_claims; drop table event_staffing_claim_roles; alter table event_role_applications drop column applicant_person_id; alter table event_staffing_items drop constraint event_staffing_items_claims_open_timed_shift, drop column capacity, drop column claims_open, drop column release_cutoff_hours; delete from schema_migrations where version>=27`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := RunMigrations(t.Context(), fx.app.db); err != nil {
			t.Fatal(err)
		}
	}
	var after string
	var notices, reviews int
	if err := fx.app.db.QueryRow(t.Context(), `select (select jsonb_agg(to_jsonb(e) order by e.id)::text from email_outbox e),(select count(*) from lifecycle_notices),(select count(*) from lifecycle_notice_reviews)`).Scan(&after, &notices, &reviews); err != nil || before != after || notices != 1 || reviews != 0 {
		t.Fatalf("migration changed queue: notices=%d reviews=%d err=%v", notices, reviews, err)
	}
}
