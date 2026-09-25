package app

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCulturalImportPreviewPersistsReviewOnlyWorkspaceScopedHints(t *testing.T) {
	fx := newLifecycleFixture(t)
	const startsAt = "2026-11-01T06:30:00Z"
	event := createEvent(t, fx, "Private source event", 1)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+mustString(t, event, "id")+"/occurrences", map[string]any{
		"name": "Night Show", "startsAt": startsAt, "timezone": "America/Chicago", "status": "scheduled",
	}, http.StatusOK)
	other := newLifecycleFixture(t, fx.app)
	otherEvent := createEvent(t, other, "Other workspace event", 1)
	postJSON(t, fx.app, other.ownerCookie, "/api/events/"+mustString(t, otherEvent, "id")+"/occurrences", map[string]any{
		"name": "Night Show", "startsAt": startsAt, "timezone": "America/Chicago", "status": "scheduled",
	}, http.StatusOK)

	input := culturalImportCSV("source-1,Night Show,Allowed description," + startsAt + ",,America/Chicago,scheduled,The Hall,Chicago,Illinois,US")
	response := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/preview", map[string]any{
		"sourceId": "  catalog-2026  ", "sourceName": "  Community calendar  ", "sourceAssertion": "  Operator supplied this source for review  ", "csv": input,
	}, http.StatusCreated)
	if got := response.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	preview := mustObject(t, response.JSON)
	if preview["sourceId"] != "catalog-2026" || preview["sourceName"] != nil || strings.Contains(response.Body, "Operator supplied") {
		t.Fatalf("preview leaked source assertion or did not normalize source ID: %#v", preview)
	}
	candidates, ok := preview["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("candidates = %#v", preview["candidates"])
	}
	candidate := mustObject(t, candidates[0])
	matches, ok := candidate["matches"].([]any)
	if !ok || len(matches) != 1 || candidate["ambiguous"] != false || candidate["matchesTruncated"] != false {
		t.Fatalf("candidate matches = %#v", candidate)
	}

	var sourceID, sourceName, sourceAssertion string
	var headers, candidateRows, matchRows int
	if err := fx.app.db.QueryRow(t.Context(), `
		select source_id, source_name, source_assertion
		from cultural_import_previews where id = $1
	`, preview["id"]).Scan(&sourceID, &sourceName, &sourceAssertion); err != nil {
		t.Fatal(err)
	}
	if sourceID != "catalog-2026" || sourceName != "Community calendar" || sourceAssertion != "Operator supplied this source for review" {
		t.Fatalf("stored assertion = %q, %q, %q", sourceID, sourceName, sourceAssertion)
	}
	if err := fx.app.db.QueryRow(t.Context(), `
		select (select count(*) from cultural_import_previews),
		       (select count(*) from cultural_import_candidates),
		       (select count(*) from cultural_import_candidate_matches)
	`).Scan(&headers, &candidateRows, &matchRows); err != nil {
		t.Fatal(err)
	}
	if headers != 1 || candidateRows != 1 || matchRows != 1 {
		t.Fatalf("stored preview rows = headers:%d candidates:%d matches:%d", headers, candidateRows, matchRows)
	}
	match := mustObject(t, matches[0])
	if _, err := fx.app.db.Exec(t.Context(), `delete from event_occurrences where id = $1`, match["occurrenceId"]); err != nil {
		t.Fatalf("preview hint blocked canonical occurrence deletion: %v", err)
	}
	var occurrenceCleared bool
	var occurrenceSnapshot, nameSnapshot, statusSnapshot string
	var updatedAtSnapshot time.Time
	if err := fx.app.db.QueryRow(t.Context(), `
		select occurrence_id is null, occurrence_id_snapshot::text, name_snapshot, status_snapshot, updated_at_snapshot
		from cultural_import_candidate_matches
	`).Scan(&occurrenceCleared, &occurrenceSnapshot, &nameSnapshot, &statusSnapshot, &updatedAtSnapshot); err != nil {
		t.Fatal(err)
	}
	if !occurrenceCleared || occurrenceSnapshot != match["occurrenceId"] || nameSnapshot != "Night Show" || statusSnapshot != "scheduled" || updatedAtSnapshot.IsZero() {
		t.Fatalf("deleted match provenance = cleared:%v id:%q name:%q status:%q revision:%s", occurrenceCleared, occurrenceSnapshot, nameSnapshot, statusSnapshot, updatedAtSnapshot)
	}
	if strings.Contains(response.Body, input) {
		t.Fatal("response contained raw CSV")
	}
}

func TestCulturalImportPreviewRejectsInvalidAssertionsAndUnderprivilegedRoles(t *testing.T) {
	fx := newLifecycleFixture(t)
	path := "/api/workspaces/" + fx.workspaceID + "/cultural-imports/preview"
	payload := map[string]any{"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": culturalImportCSV("source-1,Title,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US")}
	postJSON(t, fx.app, fx.memberCookie, path, payload, http.StatusForbidden)

	invalid := map[string]any{"sourceId": "", "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": payload["csv"]}
	postJSON(t, fx.app, fx.ownerCookie, path, invalid, http.StatusBadRequest)
	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from cultural_import_previews`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid assertion persisted %d previews", count)
	}
	postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{
		"sourceId": "", "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": strings.Repeat("x", 256*1024+1),
	}, http.StatusBadRequest)
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from cultural_import_previews`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("oversized invalid assertion persisted %d previews", count)
	}
	postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{
		"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": strings.Repeat("x", 256*1024+1),
	}, http.StatusBadRequest)
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from cultural_import_previews`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("oversized valid assertion persisted %d previews", count)
	}

	if _, err := fx.app.db.Exec(t.Context(), `
		update workspace_members set role = 'organizer'
		where workspace_id = $1 and person_id = (select id from people where email = $2)
	`, fx.workspaceID, fx.email("member")); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.memberCookie, path, payload, http.StatusCreated)
	if _, err := fx.app.db.Exec(t.Context(), `
		update workspace_members set revoked_at = now()
		where workspace_id = $1 and person_id = (select id from people where email = $2)
	`, fx.workspaceID, fx.email("member")); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.memberCookie, path, payload, http.StatusForbidden)
}

func TestCulturalImportPreviewPersistsSafeErrorsAndAcceptsEscapedBoundedInput(t *testing.T) {
	fx := newLifecycleFixture(t)
	path := "/api/workspaces/" + fx.workspaceID + "/cultural-imports/preview"
	secret := "do-not-echo-this-csv-cell"
	invalidCSV := culturalImportCSV("source-1," + secret + ",,not-a-time,,UTC,scheduled,,,,US")
	response := postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{
		"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": invalidCSV,
	}, http.StatusCreated)
	if strings.Contains(response.Body, secret) || strings.Contains(response.Body, invalidCSV) {
		t.Fatalf("safe error response echoed CSV: %s", response.Body)
	}
	preview := mustObject(t, response.JSON)
	if candidates, ok := preview["candidates"].([]any); !ok || len(candidates) != 0 {
		t.Fatalf("invalid candidate rows = %#v", preview["candidates"])
	}
	if errors, ok := preview["errors"].([]any); !ok || len(errors) == 0 {
		t.Fatalf("safe errors = %#v", preview["errors"])
	}

	// This decoded CSV is below the parser's 256 KiB cap, but escaping every
	// quote makes its JSON request larger than the former 300 KiB outer cap.
	escapedCSV := strings.Repeat("\"", 200*1024)
	postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{
		"sourceId": "escaped", "sourceName": "Escaped", "sourceAssertion": "reviewed", "csv": escapedCSV,
	}, http.StatusCreated)
}

func TestCulturalImportPreviewBoundsAmbiguousMatchesAndDoesNotApply(t *testing.T) {
	fx := newLifecycleFixture(t)
	const startsAt = "2026-11-01T06:30:00Z"
	for index := 0; index < culturalImportMatchLimit+1; index++ {
		event := createEvent(t, fx, "Match fixture "+string(rune('a'+index)), 1)
		postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+mustString(t, event, "id")+"/occurrences", map[string]any{
			"name": "Crowded Match", "startsAt": startsAt, "timezone": "America/Chicago", "status": "scheduled",
		}, http.StatusOK)
	}
	var eventsBefore, occurrencesBefore, consentsBefore, announcementsBefore int
	if err := fx.app.db.QueryRow(t.Context(), `
		select (select count(*) from events), (select count(*) from event_occurrences),
		       (select count(*) from consent_grants), (select count(*) from announcements)
	`).Scan(&eventsBefore, &occurrencesBefore, &consentsBefore, &announcementsBefore); err != nil {
		t.Fatal(err)
	}
	response := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/preview", map[string]any{
		"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed",
		"csv": culturalImportCSV("source-1,Crowded Match,," + startsAt + ",,America/Chicago,scheduled,,,,US"),
	}, http.StatusCreated)
	candidate := mustObject(t, mustObject(t, response.JSON)["candidates"].([]any)[0])
	matches := candidate["matches"].([]any)
	if len(matches) != culturalImportMatchLimit || candidate["ambiguous"] != true || candidate["matchesTruncated"] != true {
		t.Fatalf("bounded ambiguous matches = %#v", candidate)
	}
	var eventsAfter, occurrencesAfter, consentsAfter, announcementsAfter int
	if err := fx.app.db.QueryRow(t.Context(), `
		select (select count(*) from events), (select count(*) from event_occurrences),
		       (select count(*) from consent_grants), (select count(*) from announcements)
	`).Scan(&eventsAfter, &occurrencesAfter, &consentsAfter, &announcementsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != eventsBefore || occurrencesAfter != occurrencesBefore || consentsAfter != consentsBefore || announcementsAfter != announcementsBefore {
		t.Fatalf("preview changed canonical/consent/announcement state: before=%d/%d/%d/%d after=%d/%d/%d/%d", eventsBefore, occurrencesBefore, consentsBefore, announcementsBefore, eventsAfter, occurrencesAfter, consentsAfter, announcementsAfter)
	}
}

func TestCulturalImportPreviewExcludesDuplicateSourceRowsAndDeniedRoleMatrix(t *testing.T) {
	fx := newLifecycleFixture(t)
	path := "/api/workspaces/" + fx.workspaceID + "/cultural-imports/preview"
	payload := map[string]any{"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": culturalImportCSV(
		"duplicate,One,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US",
		"duplicate,Two,,2026-01-02T10:00:00Z,,UTC,scheduled,,,,US",
	)}
	postJSON(t, fx.app, fx.memberCookie, path, payload, http.StatusForbidden)
	for _, role := range []string{"crew", "door", "finance"} {
		if _, err := fx.app.db.Exec(t.Context(), `
			update workspace_members set role = $1, expires_at = null, revoked_at = null
			where workspace_id = $2 and person_id = (select id from people where email = $3)
		`, role, fx.workspaceID, fx.email("member")); err != nil {
			t.Fatal(err)
		}
		postJSON(t, fx.app, fx.memberCookie, path, payload, http.StatusForbidden)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		update workspace_members set role = 'organizer', expires_at = now() - interval '1 second', revoked_at = null
		where workspace_id = $1 and person_id = (select id from people where email = $2)
	`, fx.workspaceID, fx.email("member")); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.memberCookie, path, payload, http.StatusForbidden)

	response := postJSON(t, fx.app, fx.ownerCookie, path, payload, http.StatusCreated)
	preview := mustObject(t, response.JSON)
	if candidates, ok := preview["candidates"].([]any); !ok || len(candidates) != 0 {
		t.Fatalf("duplicate source candidates = %#v", preview["candidates"])
	}
	if errors, ok := preview["errors"].([]any); !ok || len(errors) != 2 {
		t.Fatalf("duplicate source errors = %#v", preview["errors"])
	}
	var candidates int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from cultural_import_candidates`).Scan(&candidates); err != nil {
		t.Fatal(err)
	}
	if candidates != 0 {
		t.Fatalf("duplicate source rows persisted %d candidates", candidates)
	}
}

func TestCulturalImportPreviewMigrationUpgradesVersionSixteenFixture(t *testing.T) {
	ctx := t.Context()
	db := newMigrationTestPool(t)
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 17 {
		t.Fatalf("migrations = %d, want at least 17", len(migrations))
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `create table schema_migrations(version integer primary key,name text not null,checksum char(64) not null,applied_at timestamptz not null default now())`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:16] {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			t.Fatalf("apply historical migration %d: %v", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version, name, checksum) values ($1, $2, $3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(ctx, db); err != nil {
		t.Fatalf("upgrade version-16 fixture: %v", err)
	}
	version, err := CurrentSchemaVersion(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if version != migrations[len(migrations)-1].Version || version < 17 {
		t.Fatalf("schema version = %d, want current version %d at least 17", version, migrations[len(migrations)-1].Version)
	}
	for _, table := range []string{"cultural_import_previews", "cultural_import_candidates", "cultural_import_candidate_matches", "cultural_import_preview_errors"} {
		var exists bool
		if err := db.QueryRow(ctx, `select to_regclass(current_schema() || '.' || $1) is not null`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("upgraded fixture missing %s", table)
		}
	}
}

func TestCulturalImportApplyRequiresExplicitChoiceAndCanRollbackCleanCreate(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Import target", 1)
	eventID := mustString(t, event, "id")
	preview := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/preview", map[string]any{
		"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed",
		"csv": culturalImportCSV("source-1,Imported show,description,2026-11-01T06:30:00Z,,America/Chicago,scheduled,,,,US"),
	}, http.StatusCreated)
	candidate := mustObject(t, mustObject(t, preview.JSON)["candidates"].([]any)[0])
	applyPath := "/api/workspaces/" + fx.workspaceID + "/cultural-imports/apply"
	postJSON(t, fx.app, fx.ownerCookie, applyPath, map[string]any{"candidateId": candidate["id"], "mode": "create", "eventId": eventID}, http.StatusBadRequest)
	action := postJSON(t, fx.app, fx.ownerCookie, applyPath, map[string]any{"candidateId": candidate["id"], "mode": "create", "eventId": eventID, "selectedFields": []string{"name", "description", "startsAt", "endsAt", "timezone", "status"}}, http.StatusCreated)
	createdID := mustString(t, action.JSON, "createdOccurrenceId")
	var placeID sql.NullString
	if err := fx.app.db.QueryRow(t.Context(), `select place_id from event_occurrences where id=$1`, createdID).Scan(&placeID); err != nil {
		t.Fatal(err)
	}
	if placeID.Valid {
		t.Fatal("import inferred a place")
	}
	postJSON(t, fx.app, fx.ownerCookie, applyPath, map[string]any{"candidateId": candidate["id"], "mode": "create", "eventId": eventID, "selectedFields": []string{"name", "startsAt"}}, http.StatusConflict)
	reloaded := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/"+mustString(t, preview.JSON, "id"), http.StatusOK)
	actions, ok := mustObject(t, reloaded.JSON)["actions"].([]any)
	if !ok || len(actions) != 1 || mustObject(t, actions[0])["id"] != action.JSON.(map[string]any)["id"] {
		t.Fatalf("reloaded action = %#v", reloaded.JSON)
	}
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-import-actions/"+mustString(t, action.JSON, "id")+"/rollback", map[string]any{}, http.StatusOK)
	reloaded = getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/"+mustString(t, preview.JSON, "id"), http.StatusOK)
	actions, ok = mustObject(t, reloaded.JSON)["actions"].([]any)
	if !ok || len(actions) != 1 || mustObject(t, actions[0])["rolledBackAt"] == nil {
		t.Fatalf("reloaded rolled-back action = %#v", reloaded.JSON)
	}
	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_occurrences where id=$1`, createdID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rollback left created occurrence count=%d", count)
	}
}

func TestCulturalImportCorrectionChecksRevisionAndNeverRollsBackCanonicalTarget(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Correction target", 1)
	eventID := mustString(t, event, "id")
	occurrence := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/occurrences", map[string]any{"name": "Original", "startsAt": "2026-11-01T06:30:00Z", "timezone": "America/Chicago", "status": "scheduled"}, http.StatusOK)
	preview := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/preview", map[string]any{"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": culturalImportCSV("source-1,Changed,changed description,2026-11-01T06:30:00Z,,America/Chicago,cancelled,,,,US")}, http.StatusCreated)
	candidate := mustObject(t, mustObject(t, preview.JSON)["candidates"].([]any)[0])
	path := "/api/workspaces/" + fx.workspaceID + "/cultural-imports/apply"
	postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{"candidateId": candidate["id"], "mode": "correction", "eventId": eventID, "occurrenceId": mustString(t, occurrence.JSON, "id"), "selectedFields": []string{"name"}, "expectedUpdatedAt": "2020-01-01T00:00:00Z"}, http.StatusConflict)
	action := postJSON(t, fx.app, fx.ownerCookie, path, map[string]any{"candidateId": candidate["id"], "mode": "correction", "eventId": eventID, "occurrenceId": mustString(t, occurrence.JSON, "id"), "selectedFields": []string{"name", "status"}, "expectedUpdatedAt": mustString(t, occurrence.JSON, "updatedAt")}, http.StatusCreated)
	reloaded := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/"+mustString(t, preview.JSON, "id"), http.StatusOK)
	actions, ok := mustObject(t, reloaded.JSON)["actions"].([]any)
	if !ok || len(actions) != 1 || mustObject(t, actions[0])["id"] != action.JSON.(map[string]any)["id"] || mustObject(t, actions[0])["mode"] != "correction" {
		t.Fatalf("reloaded correction action = %#v", reloaded.JSON)
	}
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-import-actions/"+mustString(t, action.JSON, "id")+"/rollback", map[string]any{}, http.StatusConflict)
	var name string
	if err := fx.app.db.QueryRow(t.Context(), `select name from event_occurrences where id=$1`, mustString(t, occurrence.JSON, "id")).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Changed" {
		t.Fatalf("correction was not retained: %q", name)
	}
}

func TestCulturalImportActionsMigrationUpgradesVersionEighteenFixture(t *testing.T) {
	ctx := t.Context()
	db := newMigrationTestPool(t)
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 19 {
		t.Fatalf("migrations = %d, want at least 19", len(migrations))
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `create table schema_migrations(version integer primary key,name text not null,checksum char(64) not null,applied_at timestamptz not null default now())`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:18] {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			t.Fatalf("apply historical migration %d: %v", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version,name,checksum) values($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(ctx, db); err != nil {
		t.Fatalf("upgrade version-18 fixture: %v", err)
	}
	for _, table := range []string{"cultural_import_actions"} {
		var exists bool
		if err := db.QueryRow(ctx, `select exists(select 1 from information_schema.tables where table_schema=current_schema() and table_name=$1)`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing upgraded table %s", table)
		}
	}
}

func TestCulturalImportApplyAcknowledgementRolesBoundariesAndNoSideEffects(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Action target", 1)
	path := "/api/workspaces/" + fx.workspaceID + "/cultural-imports/apply"
	preview := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/preview", map[string]any{
		"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed",
		"csv": culturalImportCSV("bad,Bad,,not-a-date,,UTC,scheduled,,,,US", "good,Good,,2026-11-01T06:30:00Z,,UTC,scheduled,,,,US"),
	}, http.StatusCreated)
	candidate := mustObject(t, mustObject(t, preview.JSON)["candidates"].([]any)[0])
	payload := map[string]any{"candidateId": candidate["id"], "mode": "create", "eventId": mustString(t, event, "id"), "selectedFields": []string{"name", "startsAt"}}
	var occurrencesBefore, consentBefore, announcementsBefore, paymentsBefore int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from event_occurrences),(select count(*) from consent_grants),(select count(*) from announcements),(select count(*) from payment_checkout_attempts)`).Scan(&occurrencesBefore, &consentBefore, &announcementsBefore, &paymentsBefore); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.ownerCookie, path, payload, http.StatusConflict)
	payload["acknowledgeErrors"] = true
	postJSON(t, fx.app, fx.memberCookie, path, payload, http.StatusForbidden)
	other := newLifecycleFixture(t, fx.app)
	otherEvent := createEvent(t, other, "Other target", 1)
	payload["eventId"] = mustString(t, otherEvent, "id")
	postJSON(t, fx.app, fx.ownerCookie, path, payload, http.StatusBadRequest)
	payload["eventId"] = mustString(t, event, "id")
	postJSON(t, fx.app, fx.ownerCookie, path, payload, http.StatusCreated)
	var occurrencesAfter, consentAfter, announcementsAfter, paymentsAfter int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from event_occurrences),(select count(*) from consent_grants),(select count(*) from announcements),(select count(*) from payment_checkout_attempts)`).Scan(&occurrencesAfter, &consentAfter, &announcementsAfter, &paymentsAfter); err != nil {
		t.Fatal(err)
	}
	if occurrencesAfter != occurrencesBefore+1 || consentAfter != consentBefore || announcementsAfter != announcementsBefore || paymentsAfter != paymentsBefore {
		t.Fatalf("unexpected side effects occurrences %d→%d consent %d→%d announcements %d→%d payments %d→%d", occurrencesBefore, occurrencesAfter, consentBefore, consentAfter, announcementsBefore, announcementsAfter, paymentsBefore, paymentsAfter)
	}
}

func TestCulturalImportApplyRechecksAuthorityAfterEventLock(t *testing.T) {
	fx := newLifecycleFixture(t)
	eventID := mustString(t, createEvent(t, fx, "Import revoke race", 1), "id")
	preview := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/preview", map[string]any{
		"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed",
		"csv": culturalImportCSV("race,Imported,,2026-12-01T06:30:00Z,,UTC,scheduled,,,,US"),
	}, http.StatusCreated)
	candidate := mustObject(t, mustObject(t, preview.JSON)["candidates"].([]any)[0])
	blocker, err := fx.app.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(t.Context()) }()
	if _, err := blocker.Exec(t.Context(), `select id from events where id=$1 for update`, eventID); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		result <- requestTicketCapacityReservation(fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/apply", map[string]any{
			"candidateId": candidate["id"], "mode": "create", "eventId": eventID, "selectedFields": []string{"name", "startsAt"},
		}).status
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at=clock_timestamp() where workspace_id=$1 and person_id=(select person_id from workspace_members where workspace_id=$1 and role='owner' limit 1)`, fx.workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status := <-result; status != http.StatusForbidden {
		t.Fatalf("revoked owner import apply status=%d", status)
	}
	var actions, occurrences int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from cultural_import_actions), (select count(*) from event_occurrences where event_id=$1)`, eventID).Scan(&actions, &occurrences); err != nil {
		t.Fatal(err)
	}
	if actions != 0 || occurrences != 0 {
		t.Fatalf("revoked import apply changed state: actions=%d occurrences=%d", actions, occurrences)
	}
}

func TestCulturalImportCorrectionRejectsStaleCIDAndRollbackBlockers(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Target", 1)
	eventID := mustString(t, event, "id")
	occurrence := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/occurrences", map[string]any{"name": "Target", "startsAt": "2026-11-01T06:30:00Z", "timezone": "UTC", "status": "scheduled"}, http.StatusOK)
	occurrenceID := mustString(t, occurrence.JSON, "id")
	preview := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/preview", map[string]any{"sourceId": "catalog", "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": culturalImportCSV("one,Changed,,2026-11-01T06:30:00Z,,UTC,cancelled,,,,US")}, http.StatusCreated)
	candidate := mustObject(t, mustObject(t, preview.JSON)["candidates"].([]any)[0])
	match := mustObject(t, occurrence.JSON)
	if _, err := fx.app.db.Exec(t.Context(), `update event_occurrences set public_cid='current' where id=$1`, occurrenceID); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/apply", map[string]any{"candidateId": candidate["id"], "mode": "correction", "eventId": eventID, "occurrenceId": occurrenceID, "selectedFields": []string{"name"}, "expectedUpdatedAt": match["updatedAt"], "expectedPublicCid": "stale"}, http.StatusConflict)
	// Each create action below is deliberately made distinct so rollback guards
	// are tested independently instead of being masked by candidate idempotency.
	for _, kind := range []string{"changed", "published", "credited", "later"} {
		p := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/preview", map[string]any{"sourceId": "catalog-" + kind, "sourceName": "Catalog", "sourceAssertion": "reviewed", "csv": culturalImportCSV(kind+",Created,,2026-12-01T06:30:00Z,,UTC,scheduled,,,,US", kind+"-later,Reference,,2026-12-02T06:30:00Z,,UTC,scheduled,,,,US")}, http.StatusCreated)
		candidates := mustObject(t, p.JSON)["candidates"].([]any)
		if len(candidates) != 2 {
			t.Fatalf("%s candidates = %#v", kind, candidates)
		}
		c := mustObject(t, candidates[0])
		action := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-imports/apply", map[string]any{"candidateId": c["id"], "mode": "create", "eventId": eventID, "selectedFields": []string{"name", "startsAt"}}, http.StatusCreated)
		created := mustString(t, action.JSON, "createdOccurrenceId")
		switch kind {
		case "changed":
			if _, err := fx.app.db.Exec(t.Context(), `update event_occurrences set name='edited',updated_at=clock_timestamp() where id=$1`, created); err != nil {
				t.Fatal(err)
			}
		case "published":
			if _, err := fx.app.db.Exec(t.Context(), `update event_occurrences set public_cid='published' where id=$1`, created); err != nil {
				t.Fatal(err)
			}
		case "credited":
			profile := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/profiles", map[string]any{"kind": "collective", "displayName": "Credit " + kind}, http.StatusOK)
			postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/occurrences/"+created+"/credits", map[string]any{"profileId": mustString(t, profile.JSON, "id")}, http.StatusOK)
		case "later":
			laterCandidate := mustObject(t, candidates[1])
			if _, err := fx.app.db.Exec(t.Context(), `insert into cultural_import_actions(workspace_id,import_id,candidate_id,mode,target_event_id,target_occurrence_id,source_id_snapshot,content_sha256_snapshot,candidate_row_snapshot,field_diff,before_snapshot,after_snapshot,applied_by_person_id) select workspace_id,import_id,$3,'correction',target_event_id,$2,source_id_snapshot,content_sha256_snapshot,(select row_number from cultural_import_candidates where id=$3),'{}','{}','{}',applied_by_person_id from cultural_import_actions where id=$1`, mustString(t, action.JSON, "id"), created, laterCandidate["id"]); err != nil {
				t.Fatal(err)
			}
		}
		postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/cultural-import-actions/"+mustString(t, action.JSON, "id")+"/rollback", map[string]any{}, http.StatusConflict)
	}
}

func culturalImportCSV(rows ...string) string {
	return "source_record_id,title,description,starts_at,ends_at,timezone,status,venue_name,locality,region,country\n" + strings.Join(rows, "\n") + "\n"
}
