package app

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPublicArchiveItemsStayApprovedUnpublishedAndPrivate(t *testing.T) {
	fx := newLifecycleFixture(t)
	eventID := mustString(t, createEvent(t, fx, "Archive", 1), "id")
	path := "/api/events/" + eventID + "/public-archive-items"
	postJSON(t, fx.app, nil, path, map[string]any{}, http.StatusUnauthorized)
	postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{"kind": "link", "title": "Press", "attributionName": "Artist", "externalUrl": "javascript:alert(1)", "intendedUse": "link_only", "rightsAssertion": "permission_asserted", "evidenceReference": "ref"}, http.StatusConflict)
	postJSON(t, fx.app, fx.memberCookie, path, map[string]any{}, http.StatusForbidden)
	postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{}, http.StatusConflict)
	publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	created := postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{"kind": "link", "title": "Press", "attributionName": "Artist", "externalUrl": "https://example.test/press", "intendedUse": "link_only", "rightsAssertion": "permission_asserted", "evidenceReference": "ref"}, http.StatusCreated)
	item := mustObject(t, created.JSON)
	for _, key := range []string{"email", "amountCents", "currency", "settlementId", "reportId", "notes", "participants"} {
		if _, ok := item[key]; ok {
			t.Fatalf("leaked %s", key)
		}
	}
	id := mustString(t, item, "id")
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive/notes", map[string]any{"body": "ARCHIVE_PRIVATE_NOTE_SENTINEL"}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, path+"/"+id+"/correct", map[string]any{"kind": "credit", "title": "Correct", "attributionName": "Artist", "intendedUse": "display_credit", "rightsAssertion": "owned", "evidenceReference": "ref"}, http.StatusCreated)
	listResponse := getJSON(t, fx.app, fx.ownerCookie, path, http.StatusOK)
	if strings.Contains(listResponse.Body, "ARCHIVE_PRIVATE_NOTE_SENTINEL") {
		t.Fatalf("public archive ledger leaked a private archive note: %s", listResponse.Body)
	}
	listed := listResponse.JSON.([]any)
	if len(listed) != 2 {
		t.Fatalf("items=%#v", listed)
	}
}

func TestPublicArchiveItemsOwnerEndedRoleAndWorkspaceBoundary(t *testing.T) {
	fx := newLifecycleFixture(t)
	eventID := mustString(t, createEvent(t, fx, "Archive authority", 1), "id")
	path := "/api/events/" + eventID + "/public-archive-items"
	payload := map[string]any{"kind": "link", "title": "Press", "attributionName": "Artist", "externalUrl": "https://example.test/press", "intendedUse": "link_only", "rightsAssertion": "permission_asserted", "evidenceReference": "owner-led record"}
	publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.memberCookie, path, nil, http.StatusForbidden)
	postJSON(t, fx.app, fx.ownerCookie, path, payload, http.StatusConflict)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)

	for _, raw := range []string{"javascript:alert(1)", "https://user:password@example.test/work", "https://example.test/one\nsecond", "ftp://example.test/work"} {
		bad := map[string]any{"kind": "link", "title": "Bad", "attributionName": "Artist", "externalUrl": raw, "intendedUse": "link_only", "rightsAssertion": "permission_asserted", "evidenceReference": ""}
		postJSON(t, fx.app, fx.ownerCookie, path, bad, http.StatusBadRequest)
	}
	created := postJSON(t, fx.app, fx.ownerCookie, path, payload, http.StatusCreated)
	itemID := mustString(t, created.JSON, "id")
	other := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, other.ownerCookie, path, http.StatusForbidden)
	postJSON(t, fx.app, other.ownerCookie, path+"/"+itemID+"/unavailable", map[string]any{"reason": "wrong workspace"}, http.StatusForbidden)

	var ownerID string
	if err := fx.app.db.QueryRow(t.Context(), `select person_id from workspace_members where workspace_id=$1 and role='owner'`, fx.workspaceID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at=clock_timestamp() where workspace_id=$1 and person_id=$2`, fx.workspaceID, ownerID); err != nil {
		t.Fatal(err)
	}
	getJSON(t, fx.app, fx.ownerCookie, path, http.StatusForbidden)
}

func TestPublicArchiveCreateRechecksOwnerAfterEventLock(t *testing.T) {
	fx := newLifecycleFixture(t)
	eventID := mustString(t, createEvent(t, fx, "Archive revoke race", 1), "id")
	publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	blocker, err := fx.app.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(t.Context())
	if _, err := blocker.Exec(t.Context(), `select id from events where id=$1 for update`, eventID); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		result <- requestTicketCapacityReservation(fx.app, fx.ownerCookie, "/api/events/"+eventID+"/public-archive-items", map[string]any{"kind": "link", "title": "Race", "attributionName": "Artist", "intendedUse": "link_only", "rightsAssertion": "owned"}).status
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at=clock_timestamp() where workspace_id=$1 and person_id=(select person_id from workspace_members where workspace_id=$1 and role='owner' limit 1)`, fx.workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status := <-result; status != http.StatusForbidden {
		t.Fatalf("revoked owner mutation status=%d", status)
	}
}

func TestPublicArchiveCorrectionUnavailableAndRollback(t *testing.T) {
	fx := newLifecycleFixture(t)
	eventID := mustString(t, createEvent(t, fx, "Archive corrections", 1), "id")
	publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	path := "/api/events/" + eventID + "/public-archive-items"
	created := postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{"kind": "link", "title": "Original", "attributionName": "Artist", "intendedUse": "link_only", "rightsAssertion": "permission_asserted", "evidenceReference": "email"}, http.StatusCreated)
	originalID := mustString(t, created.JSON, "id")
	// Validation happens before the transaction mutates the approved predecessor.
	postJSON(t, fx.app, fx.ownerCookie, path+"/"+originalID+"/correct", map[string]any{"kind": "link", "title": "Broken", "attributionName": "Artist", "externalUrl": "https://user:password@example.test", "intendedUse": "link_only", "rightsAssertion": "owned"}, http.StatusBadRequest)
	var originalStatus string
	if err := fx.app.db.QueryRow(t.Context(), `select status from event_public_archive_items where id=$1`, originalID).Scan(&originalStatus); err != nil {
		t.Fatal(err)
	}
	if originalStatus != "approved" {
		t.Fatalf("invalid correction changed predecessor to %q", originalStatus)
	}
	replacement := postJSON(t, fx.app, fx.ownerCookie, path+"/"+originalID+"/correct", map[string]any{"kind": "credit", "title": "Corrected", "attributionName": "Artist", "intendedUse": "display_credit", "rightsAssertion": "owned", "evidenceReference": "signed release"}, http.StatusCreated)
	replacementObj := mustObject(t, replacement.JSON)
	if replacementObj["replacesItemId"] != originalID || replacementObj["status"] != "approved" {
		t.Fatalf("unexpected replacement: %#v", replacementObj)
	}
	otherEventID := mustString(t, createEvent(t, fx, "Other archive scope", 1), "id")
	publishEvent(t, fx, otherEventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+otherEventID+"/end-of-night", map[string]any{}, http.StatusOK)
	var ownerID string
	if err := fx.app.db.QueryRow(t.Context(), `select person_id from workspace_members where workspace_id=$1 and role='owner'`, fx.workspaceID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `insert into event_public_archive_items(workspace_id,event_id,replaces_item_id,kind,title,attribution_name,intended_use,rights_assertion,approved_by_person_id) values($1,$2,$3,'link','Wrong scope','Artist','link_only','owned',$4)`, fx.workspaceID, otherEventID, originalID, ownerID); err == nil {
		t.Fatal("replacement cross-event foreign key was accepted")
	}
	postJSON(t, fx.app, fx.ownerCookie, path+"/"+originalID+"/correct", map[string]any{"kind": "credit", "title": "Again", "attributionName": "Artist", "intendedUse": "display_credit", "rightsAssertion": "owned"}, http.StatusNotFound)
	replacementID := mustString(t, replacement.JSON, "id")
	postJSON(t, fx.app, fx.ownerCookie, path+"/"+replacementID+"/unavailable", map[string]any{"reason": "Contributor withdrew the public link"}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, path+"/"+replacementID+"/unavailable", map[string]any{"reason": "again"}, http.StatusNotFound)
	listed := getJSON(t, fx.app, fx.ownerCookie, path, http.StatusOK).JSON.([]any)
	if len(listed) != 2 {
		t.Fatalf("items=%#v", listed)
	}
	first, second := mustObject(t, listed[0]), mustObject(t, listed[1])
	if first["status"] != "corrected" || second["status"] != "unavailable" || second["unavailableReason"] != "Contributor withdrew the public link" {
		t.Fatalf("unexpected immutable chain: %#v", listed)
	}
}

func TestPublicArchiveMigrationUpgradesVersionSeventeen(t *testing.T) {
	ctx := t.Context()
	db := newMigrationTestPool(t)
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 18 {
		t.Fatalf("migrations=%d, need version 18", len(migrations))
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `create table schema_migrations(version integer primary key,name text not null,checksum char(64) not null,applied_at timestamptz not null default now())`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:17] {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			t.Fatalf("apply migration %d: %v", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version,name,checksum) values($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := db.QueryRow(ctx, `select max(version) from schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 18 {
		t.Fatalf("version=%d, want at least 18", version)
	}
	var exists bool
	if err := db.QueryRow(ctx, `select to_regclass('event_public_archive_items') is not null`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("version 18 did not create the public archive ledger")
	}
}
