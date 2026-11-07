package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func accessFixture(t *testing.T) (lifecycleFixture, string, string) {
	t.Helper()
	fx := newLifecycleFixture(t)
	id := mustString(t, createEvent(t, fx, "Synthetic access worksheet", 10), "id")
	return fx, id, "/api/events/" + id + "/access-info"
}
func accessPayload(revision int) map[string]any {
	return map[string]any{"requestKey": uuid.NewString(), "expectedRevision": revision, "value": "yes", "details": "Synthetic entry route", "sourceKind": "organizer_assertion", "sourceReference": "PRIVATE_ACCESS_SOURCE", "reviewedAt": "2026-01-01T00:00:00Z", "correctionReason": "Initial synthetic assertion"}
}
func TestEventAccessPrivacyUnknownAndPublicAbsence(t *testing.T) {
	fx, id, path := accessFixture(t)
	getJSON(t, fx.app, nil, path, 401)
	getJSON(t, fx.app, fx.memberCookie, path, 403)
	outsider := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, outsider.ownerCookie, path, 403)
	worksheet := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, path, 200).JSON)
	entries := worksheet["entries"].([]any)
	if len(entries) != 6 {
		t.Fatal("missing unknown topics")
	}
	for _, raw := range entries {
		e := mustObject(t, raw)
		if e["value"] != "unknown" || e["effectiveValue"] != "unknown" || e["revision"] != float64(0) || e["scope"] != "event" {
			t.Fatalf("default is an assertion: %v", e)
		}
	}
	req := accessPayload(0)
	postJSON(t, fx.app, fx.memberCookie, path+"/entry", req, 403)
	created := postJSON(t, fx.app, fx.ownerCookie, path+"/entry", req, 201)
	for _, suffix := range []string{"", "/entry/history"} {
		getJSON(t, fx.app, fx.memberCookie, path+suffix, 403)
	}
	data, _ := json.Marshal(created.JSON)
	for _, field := range []string{"requestKey", "request_fingerprint", "recorded_by_person_id", "recordedByPersonId"} {
		if strings.Contains(string(data), field) {
			t.Fatal("private identity leaked")
		}
	}
	published := publishEvent(t, fx, id)
	slug := mustString(t, published, "publicSlug")
	for _, publicPath := range []string{"/api/public/events", "/api/public/events/" + slug} {
		body, _ := json.Marshal(getJSON(t, fx.app, nil, publicPath, 200).JSON)
		for _, private := range []string{"PRIVATE_ACCESS_SOURCE", "accessInfo", "sourceKind", "correctionReason"} {
			if strings.Contains(string(body), private) {
				t.Fatal("private worksheet leaked publicly")
			}
		}
	}
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at=now() where workspace_id=$1 and person_id=$2`, fx.workspaceID, ownerPersonID(t, fx)); err != nil {
		t.Fatal(err)
	}
	getJSON(t, fx.app, fx.ownerCookie, path, 403)
	getJSON(t, fx.app, fx.ownerCookie, path+"/entry/history", 403)
	postJSON(t, fx.app, fx.ownerCookie, path+"/entry", req, 403)
}
func TestEventAccessCorrectionExpiryAndReplay(t *testing.T) {
	fx, _, path := accessFixture(t)
	req := accessPayload(0)
	req["expiresAt"] = "2026-02-01T00:00:00Z"
	first := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, path+"/entry", req, 201).JSON)
	if first["value"] != "yes" || first["effectiveValue"] != "unknown" || first["needsReview"] != true {
		t.Fatal("expired assertion remained effective")
	}
	correction := accessPayload(1)
	correction["value"] = "no"
	correction["sourceKind"] = "event_observation"
	correction["correctionReason"] = "Different entrance selected"
	second := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, path+"/entry", correction, 201).JSON)
	if second["effectiveValue"] != "no" {
		t.Fatal("old review without expiry was incorrectly expired")
	}
	replay := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, path+"/entry", req, 200).JSON)
	if replay["id"] != first["id"] || replay["revision"] != float64(1) {
		t.Fatal("replay replaced original revision")
	}
	req["details"] = "Different binding"
	postJSON(t, fx.app, fx.ownerCookie, path+"/entry", req, 409)
	postJSON(t, fx.app, fx.ownerCookie, path+"/entry", accessPayload(1), 409)
	withdraw := map[string]any{"requestKey": uuid.NewString(), "expectedRevision": 2, "value": "unknown", "sourceKind": "unknown", "correctionReason": "Disputed; needs a new check"}
	postJSON(t, fx.app, fx.ownerCookie, path+"/entry", withdraw, 201)
	history := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, path+"/entry/history", 200).JSON)["revisions"].([]any)
	if len(history) != 3 || mustObject(t, history[0])["value"] != "unknown" || mustObject(t, history[2])["id"] != first["id"] {
		t.Fatal("correction lost history")
	}
	latest := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, path, 200).JSON)["entries"].([]any)
	if mustObject(t, latest[0])["value"] != "unknown" {
		t.Fatal("withdrawn assertion still current")
	}
	future := accessPayload(0)
	future["reviewedAt"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	postJSON(t, fx.app, fx.ownerCookie, path+"/bathrooms", future, 400)
}
func accessRequestStatus(fx lifecycleFixture, path string, req map[string]any) int {
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(fx.ownerCookie)
	w := httptest.NewRecorder()
	fx.app.Handler().ServeHTTP(w, r)
	return w.Code
}
func TestEventAccessConcurrentCorrectionAndAuditRollback(t *testing.T) {
	fx, _, path := accessFixture(t)
	req := accessPayload(0)
	if _, err := fx.app.db.Exec(t.Context(), `alter table audit_entries add constraint synthetic_access_audit_failure check(action<>'event_access.revised')`); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.ownerCookie, path+"/entry", req, 500)
	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_access_revisions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("revision survived failed audit")
	}
	if _, err := fx.app.db.Exec(t.Context(), `alter table audit_entries drop constraint synthetic_access_audit_failure`); err != nil {
		t.Fatal(err)
	}
	statuses := make(chan int, 2)
	for range 2 {
		go func() { statuses <- accessRequestStatus(fx, path+"/entry", req) }()
	}
	a, b := <-statuses, <-statuses
	if !((a == 201 && b == 200) || (a == 200 && b == 201)) {
		t.Fatalf("replay statuses %d/%d", a, b)
	}
	first, second := accessPayload(1), accessPayload(1)
	second["value"] = "no"
	go func() { statuses <- accessRequestStatus(fx, path+"/entry", first) }()
	go func() { statuses <- accessRequestStatus(fx, path+"/entry", second) }()
	a, b = <-statuses, <-statuses
	if !((a == 201 && b == 409) || (a == 409 && b == 201)) {
		t.Fatalf("correction statuses %d/%d", a, b)
	}
	var audits int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from event_access_revisions),(select count(*) from audit_entries where action='event_access.revised')`).Scan(&count, &audits); err != nil || count != 2 || audits != 2 {
		t.Fatalf("revision/audit %d/%d %v", count, audits, err)
	}
}
func TestEventAccessHistoryPaginationAndKeyBinding(t *testing.T) {
	fx, _, path := accessFixture(t)
	first := accessPayload(0)
	postJSON(t, fx.app, fx.ownerCookie, path+"/entry", first, 201)
	postJSON(t, fx.app, fx.ownerCookie, path+"/bathrooms", first, 409)
	other := mustString(t, createEvent(t, fx, "Other access event", 10), "id")
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+other+"/access-info/entry", first, 409)
	for revision := 1; revision < 23; revision++ {
		postJSON(t, fx.app, fx.ownerCookie, path+"/entry", accessPayload(revision), 201)
	}
	page := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, path+"/entry/history", 200).JSON)
	if len(page["revisions"].([]any)) != 20 || page["nextBefore"] != float64(4) {
		t.Fatal("unbounded history page")
	}
	older := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, path+"/entry/history?before=4", 200).JSON)
	if len(older["revisions"].([]any)) != 3 || older["nextBefore"] != nil {
		t.Fatal("history cursor lost revisions")
	}
	getJSON(t, fx.app, fx.ownerCookie, path+"/entry/history?before=-1", 400)
	getJSON(t, fx.app, fx.ownerCookie, path+"/other/history", 400)
}
func TestEventAccessRechecksOwnerAfterEventLock(t *testing.T) {
	fx, id, path := accessFixture(t)
	tx, err := fx.app.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err = tx.Exec(t.Context(), `select id from events where id=$1 for update`, id); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() { result <- accessRequestStatus(fx, path+"/entry", accessPayload(0)) }()
	deadline := time.Now().Add(10 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var n int
		if err = fx.app.db.QueryRow(t.Context(), `select count(*) from pg_stat_activity where datname=current_database() and wait_event_type='Lock' and query like '%select id from events%for update%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("request did not wait on event lock")
	}
	if _, err = fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at=now() where workspace_id=$1 and person_id=$2`, fx.workspaceID, ownerPersonID(t, fx)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-result:
		if status != 403 {
			t.Fatalf("stale owner admitted %d", status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("request did not finish")
	}
	var n int
	if err = fx.app.db.QueryRow(t.Context(), `select count(*) from event_access_revisions`).Scan(&n); err != nil || n != 0 {
		t.Fatal("revoked owner wrote revision")
	}
}
func TestEventAccessMigrationPreservesExistingState(t *testing.T) {
	fx, id, _ := accessFixture(t)
	var before string
	if err := fx.app.db.QueryRow(t.Context(), `select to_jsonb(e)::text from events e where id=$1`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `alter table email_outbox drop constraint email_outbox_provider_id_length; alter table email_provider_events drop constraint email_events_provider_id_length; alter table email_outbox drop column provider; alter table email_outbox alter column provider_message_id type uuid using provider_message_id::uuid; alter table email_provider_events alter column provider_message_id type uuid using provider_message_id::uuid; drop table venue_access_place_requests;drop table event_access_revisions;drop table event_staffing_claims; drop table event_staffing_claim_roles; alter table event_role_applications drop column applicant_person_id; alter table event_staffing_items drop constraint event_staffing_items_claims_open_timed_shift, drop column capacity, drop column claims_open, drop column release_cutoff_hours; delete from schema_migrations where version>=28`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := RunMigrations(t.Context(), fx.app.db); err != nil {
			t.Fatal(err)
		}
	}
	var after string
	if err := fx.app.db.QueryRow(t.Context(), `select to_jsonb(e)::text from events e where id=$1`, id).Scan(&after); err != nil || after != before {
		t.Fatal("migration changed existing event")
	}
	if version, err := CurrentSchemaVersion(t.Context(), fx.app.db); err != nil || version != minimumSchemaVersion {
		t.Fatal("migration not applied")
	}
}
