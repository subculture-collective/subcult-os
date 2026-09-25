package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Import actions deliberately accept canonical identities only from the
// operator. Preview matches are hints, never an authority to select a record.
type culturalImportApplyRequest struct {
	CandidateID       string   `json:"candidateId"`
	Mode              string   `json:"mode"`
	EventID           string   `json:"eventId"`
	OccurrenceID      string   `json:"occurrenceId"`
	SelectedFields    []string `json:"selectedFields"`
	ExpectedUpdatedAt string   `json:"expectedUpdatedAt"`
	ExpectedPublicCID *string  `json:"expectedPublicCid"`
	AcknowledgeErrors bool     `json:"acknowledgeErrors"`
}

type culturalImportActionDTO struct {
	ID                  string  `json:"id"`
	CandidateID         string  `json:"candidateId"`
	Mode                string  `json:"mode"`
	EventID             string  `json:"eventId"`
	OccurrenceID        *string `json:"occurrenceId,omitempty"`
	CreatedOccurrenceID *string `json:"createdOccurrenceId,omitempty"`
	AppliedAt           string  `json:"appliedAt"`
	RolledBackAt        *string `json:"rolledBackAt,omitempty"`
}

type importCandidateRow struct {
	ID, ImportID, WorkspaceID, SourceRecordID, Title, Description, Timezone, Status string
	Row                                                                             int
	StartsAt                                                                        time.Time
	EndsAt                                                                          sql.NullTime
}

func (a *App) handleGetCulturalImportPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if a.db == nil {
		writeError(w, 500, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	if _, ok := a.requirePermission(r, workspaceID, permManageImports); !ok {
		writeError(w, 403, "forbidden")
		return
	}
	previewID := r.PathValue("importID")
	var out culturalImportPreviewDTO
	var created time.Time
	err := a.db.QueryRow(r.Context(), `select id, workspace_id, schema_name, source_id, content_sha256, created_at from cultural_import_previews where id=$1 and workspace_id=$2`, previewID, workspaceID).Scan(&out.ID, &out.WorkspaceID, &out.Schema, &out.SourceID, &out.ContentSHA256, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "import preview not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load import preview")
		return
	}
	out.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	out.Candidates = []culturalImportCandidateDTO{}
	out.Errors = []culturalImportPreviewErrorDTO{}
	out.Actions = []culturalImportActionDTO{}
	rows, err := a.db.Query(r.Context(), `select id,row_number,source_record_id,title,description,starts_at,ends_at,timezone,status,venue_name,locality,region,country,match_count,matches_truncated from cultural_import_candidates where import_id=$1 order by row_number`, previewID)
	if err != nil {
		writeError(w, 500, "could not load import preview")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c culturalImportCandidateDTO
		var starts time.Time
		var ends sql.NullTime
		var count int
		if err := rows.Scan(&c.ID, &c.Row, &c.SourceRecordID, &c.Title, &c.Description, &starts, &ends, &c.Timezone, &c.Status, &c.VenueName, &c.Locality, &c.Region, &c.Country, &count, &c.MatchesTruncated); err != nil {
			writeError(w, 500, "could not load import preview")
			return
		}
		c.StartsAt = starts.UTC().Format(time.RFC3339Nano)
		if ends.Valid {
			c.EndsAt = ends.Time.UTC().Format(time.RFC3339Nano)
		}
		c.Ambiguous = count > 1
		c.Matches = []culturalImportMatchDTO{}
		out.Candidates = append(out.Candidates, c)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "could not load import preview")
		return
	}
	rows.Close()
	for index := range out.Candidates {
		c := &out.Candidates[index]
		mrs, err := a.db.Query(r.Context(), `select occurrence_id_snapshot,event_id_snapshot,name_snapshot,starts_at_snapshot,status_snapshot,updated_at_snapshot from cultural_import_candidate_matches where candidate_id=$1 order by occurrence_id_snapshot`, c.ID)
		if err != nil {
			writeError(w, 500, "could not load import preview")
			return
		}
		for mrs.Next() {
			var m culturalImportMatchDTO
			var starts, updated time.Time
			if err := mrs.Scan(&m.OccurrenceID, &m.EventID, &m.Name, &starts, &m.Status, &updated); err != nil {
				mrs.Close()
				writeError(w, 500, "could not load import preview")
				return
			}
			m.StartsAt = starts.UTC().Format(time.RFC3339Nano)
			m.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
			c.Matches = append(c.Matches, m)
		}
		if err := mrs.Err(); err != nil {
			mrs.Close()
			writeError(w, 500, "could not load import preview")
			return
		}
		mrs.Close()
	}
	errs, err := a.db.Query(r.Context(), `select row_number,field_name,code from cultural_import_preview_errors where import_id=$1 order by row_number,field_name,code`, previewID)
	if err != nil {
		writeError(w, 500, "could not load import preview")
		return
	}
	defer errs.Close()
	for errs.Next() {
		var x culturalImportPreviewErrorDTO
		if err := errs.Scan(&x.Row, &x.Field, &x.Code); err != nil {
			writeError(w, 500, "could not load import preview")
			return
		}
		out.Errors = append(out.Errors, x)
	}
	if err := errs.Err(); err != nil {
		writeError(w, 500, "could not load import preview")
		return
	}
	actions, err := a.db.Query(r.Context(), `select id,candidate_id,mode,target_event_id,target_occurrence_id,created_occurrence_id,applied_at,rolled_back_at from cultural_import_actions where import_id=$1 order by applied_at desc`, previewID)
	if err != nil {
		writeError(w, 500, "could not load import actions")
		return
	}
	for actions.Next() {
		var x culturalImportActionDTO
		var target, created sql.NullString
		var rolled sql.NullTime
		var applied time.Time
		if err := actions.Scan(&x.ID, &x.CandidateID, &x.Mode, &x.EventID, &target, &created, &applied, &rolled); err != nil {
			actions.Close()
			writeError(w, 500, "could not load import actions")
			return
		}
		if target.Valid {
			x.OccurrenceID = &target.String
		}
		if created.Valid {
			x.CreatedOccurrenceID = &created.String
		}
		x.AppliedAt = applied.UTC().Format(time.RFC3339Nano)
		if rolled.Valid {
			v := rolled.Time.UTC().Format(time.RFC3339Nano)
			x.RolledBackAt = &v
		}
		out.Actions = append(out.Actions, x)
	}
	if err := actions.Err(); err != nil {
		actions.Close()
		writeError(w, 500, "could not load import actions")
		return
	}
	actions.Close()
	actor, ok := a.requirePermission(r, workspaceID, permManageImports)
	if !ok || a.authorize(r.Context(), actor, workspaceID, permManageImports) != nil {
		writeError(w, 403, "forbidden")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, 200, out)
}

func importSelectedFields(values []string) (map[string]bool, error) {
	allowed := map[string]bool{"name": true, "description": true, "startsAt": true, "endsAt": true, "timezone": true, "status": true}
	out := map[string]bool{}
	for _, v := range values {
		if !allowed[v] {
			return nil, errors.New("invalid selectedFields")
		}
		out[v] = true
	}
	if len(out) == 0 {
		return nil, errors.New("selectedFields is required")
	}
	return out, nil
}
func importSnapshot(o eventOccurrenceRow) map[string]any {
	return map[string]any{"name": o.Name, "description": o.Description, "startsAt": o.StartsAt.UTC().Format(time.RFC3339Nano), "endsAt": nullableTimeString(o.EndsAt), "timezone": nullableString(o.Timezone), "status": o.Status, "updatedAt": o.UpdatedAt.UTC().Format(time.RFC3339Nano), "publicCid": nullableString(o.PublicCID)}
}
func nullableAny(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}

func (a *App) handleApplyCulturalImport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if a.db == nil {
		writeError(w, 500, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	actor, ok := a.requirePermission(r, workspaceID, permManageImports)
	if !ok {
		writeError(w, 403, "forbidden")
		return
	}
	var req culturalImportApplyRequest
	if decodeJSON(r, &req) != nil {
		writeError(w, 400, "invalid import action")
		return
	}
	req.CandidateID = strings.TrimSpace(req.CandidateID)
	req.EventID = strings.TrimSpace(req.EventID)
	req.OccurrenceID = strings.TrimSpace(req.OccurrenceID)
	fields, err := importSelectedFields(req.SelectedFields)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if req.CandidateID == "" || req.EventID == "" || (req.Mode != "create" && req.Mode != "correction") {
		writeError(w, 400, "candidateId, eventId, and mode are required")
		return
	}
	if req.Mode == "create" && req.OccurrenceID != "" {
		writeError(w, 400, "create cannot select an occurrence")
		return
	}
	if req.Mode == "create" && (!fields["name"] || !fields["startsAt"]) {
		writeError(w, 400, "create requires name and startsAt")
		return
	}
	if req.Mode == "correction" && (req.OccurrenceID == "" || req.ExpectedUpdatedAt == "") {
		writeError(w, 400, "correction requires occurrenceId and expectedUpdatedAt")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		writeError(w, 500, "could not start import action")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	ctx := context.WithValue(r.Context(), txContextKey{}, tx)
	var c importCandidateRow
	err = tx.QueryRow(ctx, `select id,import_id,workspace_id,row_number,source_record_id,title,description,starts_at,ends_at,timezone,status from cultural_import_candidates where id=$1 and workspace_id=$2 for update`, req.CandidateID, workspaceID).Scan(&c.ID, &c.ImportID, &c.WorkspaceID, &c.Row, &c.SourceRecordID, &c.Title, &c.Description, &c.StartsAt, &c.EndsAt, &c.Timezone, &c.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "import candidate not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not lock import candidate")
		return
	}
	// Recheck authorization at the write boundary; a request authenticated at
	// entry must not retain authority after a concurrent revocation.
	if err := a.authorize(r.Context(), actor, workspaceID, permManageImports); err != nil {
		writeError(w, 403, "forbidden")
		return
	}
	var errorCount int
	if err := tx.QueryRow(ctx, `select count(*) from cultural_import_preview_errors where import_id=$1`, c.ImportID).Scan(&errorCount); err != nil {
		writeError(w, 500, "could not inspect import preview")
		return
	}
	if errorCount > 0 && !req.AcknowledgeErrors {
		writeError(w, 409, "preview errors require acknowledgement")
		return
	}
	var sourceID, digest string
	if err := tx.QueryRow(ctx, `select source_id,content_sha256 from cultural_import_previews where id=$1 and workspace_id=$2 for update`, c.ImportID, workspaceID).Scan(&sourceID, &digest); err != nil {
		writeError(w, 500, "could not lock import preview")
		return
	}
	var eventWorkspace string
	err = tx.QueryRow(ctx, `select workspace_id from events where id=$1 for update`, req.EventID).Scan(&eventWorkspace)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && eventWorkspace != workspaceID) {
		writeError(w, 400, "selected event not found in workspace")
		return
	}
	if err != nil {
		writeError(w, 500, "could not lock event")
		return
	}
	var before map[string]any = map[string]any{}
	var after map[string]any
	var target eventOccurrenceRow
	var created eventOccurrenceRow
	if req.Mode == "correction" {
		target, err = scanOccurrenceRow(tx.QueryRow(ctx, `select `+occurrenceSelectColumns+` from event_occurrences where id=$1 and event_id=$2 and workspace_id=$3 for update`, req.OccurrenceID, req.EventID, workspaceID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 400, "selected occurrence not found in selected event")
			return
		}
		if err != nil {
			writeError(w, 500, "could not lock occurrence")
			return
		}
		expected, err := parseRFC3339Time(req.ExpectedUpdatedAt)
		if err != nil {
			writeError(w, 400, "invalid expectedUpdatedAt")
			return
		}
		if !expected.Equal(target.UpdatedAt) || (req.ExpectedPublicCID != nil && *req.ExpectedPublicCID != target.PublicCID.String) {
			writeError(w, 409, "occurrence changed; reload before correcting")
			return
		}
		if err := a.authorize(r.Context(), actor, workspaceID, permManageImports); err != nil {
			writeError(w, 403, "forbidden")
			return
		}
		before = importSnapshot(target)
		name, description, starts, ends, tz, status := target.Name, target.Description, target.StartsAt, target.EndsAt, target.Timezone, target.Status
		if fields["name"] {
			name = c.Title
		}
		if fields["description"] {
			description = c.Description
		}
		if fields["startsAt"] {
			starts = c.StartsAt
		}
		if fields["endsAt"] {
			ends = c.EndsAt
		}
		if fields["timezone"] {
			tz = sql.NullString{String: c.Timezone, Valid: true}
		}
		if fields["status"] {
			status = c.Status
		}
		if err := validateOccurrenceSchedule(starts, ends, tz); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		target, err = scanOccurrenceRow(tx.QueryRow(ctx, `update event_occurrences set name=$2,description=$3,starts_at=$4,ends_at=$5,timezone=$6,status=$7,updated_at=greatest(clock_timestamp(),updated_at+interval '1 microsecond') where id=$1 and updated_at=$8 and public_cid is not distinct from $9 returning `+occurrenceSelectColumns, target.ID, name, description, starts, ends, tz, status, target.UpdatedAt, target.PublicCID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 409, "occurrence changed; reload before correcting")
			return
		}
		if err != nil {
			writeError(w, 500, "could not correct occurrence")
			return
		}
		after = importSnapshot(target)
	} else {
		if err := a.authorize(r.Context(), actor, workspaceID, permManageImports); err != nil {
			writeError(w, 403, "forbidden")
			return
		}
		name := c.Title
		description := ""
		if fields["description"] {
			description = c.Description
		}
		starts := c.StartsAt
		ends := sql.NullTime{}
		if fields["endsAt"] {
			ends = c.EndsAt
		}
		tz := sql.NullString{}
		if fields["timezone"] {
			tz = sql.NullString{String: c.Timezone, Valid: true}
		}
		status := occurrenceStatusScheduled
		if fields["status"] {
			status = c.Status
		}
		if err := validateOccurrenceSchedule(starts, ends, tz); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		created, err = scanOccurrenceRow(tx.QueryRow(ctx, `insert into event_occurrences(workspace_id,event_id,place_id,name,description,starts_at,ends_at,all_day,timezone,status,created_by_person_id) values($1,$2,null,$3,$4,$5,$6,false,$7,$8,$9) returning `+occurrenceSelectColumns, workspaceID, req.EventID, name, description, starts, ends, tz, status, actor))
		if err != nil {
			writeError(w, 500, "could not create occurrence")
			return
		}
		after = importSnapshot(created)
	}
	fieldDiff := map[string]any{}
	for f := range fields {
		fieldDiff[f] = true
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	diffJSON, _ := json.Marshal(fieldDiff)
	var action culturalImportActionDTO
	var applied time.Time
	var createdID sql.NullString
	var targetID any = nil
	if req.Mode == "correction" {
		targetID = target.ID
	} else {
		createdID = sql.NullString{String: created.ID, Valid: true}
	}
	var acknowledged any = nil
	if errorCount > 0 {
		acknowledged = time.Now().UTC()
	}
	err = tx.QueryRow(ctx, `insert into cultural_import_actions(workspace_id,import_id,candidate_id,mode,target_event_id,target_occurrence_id,created_occurrence_id,source_id_snapshot,content_sha256_snapshot,candidate_row_snapshot,field_diff,before_snapshot,after_snapshot,expected_updated_at,expected_public_cid,created_occurrence_updated_at,errors_acknowledged_at,applied_by_person_id) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) returning id,applied_at`, workspaceID, c.ImportID, c.ID, req.Mode, req.EventID, targetID, createdID, sourceID, digest, c.Row, diffJSON, beforeJSON, afterJSON, nullableExpected(req.ExpectedUpdatedAt), req.ExpectedPublicCID, nullableCreated(created), acknowledged, actor).Scan(&action.ID, &applied)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			writeError(w, 409, "candidate was already applied")
			return
		}
		writeError(w, 500, "could not record import action")
		return
	}
	action.CandidateID = c.ID
	action.Mode = req.Mode
	action.EventID = req.EventID
	action.AppliedAt = applied.UTC().Format(time.RFC3339Nano)
	if req.Mode == "correction" {
		action.OccurrenceID = &target.ID
	} else {
		action.CreatedOccurrenceID = &created.ID
	}
	if err := a.audit(ctx, actor, "cultural_import.applied", "cultural_import_action", action.ID, map[string]any{"workspaceId": workspaceID, "mode": req.Mode, "eventId": req.EventID, "candidateId": c.ID}); err != nil {
		writeError(w, 500, "could not record audit")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, 500, "could not apply import")
		return
	}
	if err := a.authorize(r.Context(), actor, workspaceID, permManageImports); err != nil {
		writeError(w, 403, "forbidden")
		return
	}
	writeJSON(w, 201, action)
}
func nullableExpected(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	x, _ := parseRFC3339Time(v)
	return x
}
func nullableCreated(o eventOccurrenceRow) any {
	if o.ID == "" {
		return nil
	}
	return o.UpdatedAt
}

func (a *App) handleRollbackCulturalImportAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if a.db == nil {
		writeError(w, 500, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	actor, ok := a.requirePermission(r, workspaceID, permManageImports)
	if !ok {
		writeError(w, 403, "forbidden")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		writeError(w, 500, "could not start rollback")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	ctx := context.WithValue(r.Context(), txContextKey{}, tx)
	var mode, createdID string
	var createdRevision sql.NullTime
	var rolled sql.NullTime
	err = tx.QueryRow(ctx, `select mode,coalesce(created_occurrence_id::text,''),created_occurrence_updated_at,rolled_back_at from cultural_import_actions where id=$1 and workspace_id=$2 for update`, r.PathValue("actionID"), workspaceID).Scan(&mode, &createdID, &createdRevision, &rolled)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "import action not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not lock import action")
		return
	}
	if mode != "create" || !createdRevision.Valid {
		writeError(w, 409, "correction actions require a compensating correction")
		return
	}
	if err := a.authorize(r.Context(), actor, workspaceID, permManageImports); err != nil {
		writeError(w, 403, "forbidden")
		return
	}
	if rolled.Valid {
		writeError(w, 409, "import action already rolled back")
		return
	}
	o, err := scanOccurrenceRow(tx.QueryRow(ctx, `select `+occurrenceSelectColumns+` from event_occurrences where id=$1 and workspace_id=$2 for update`, createdID, workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 409, "created occurrence is no longer available for rollback")
		return
	}
	if err != nil {
		writeError(w, 500, "could not lock created occurrence")
		return
	}
	if err := a.authorize(r.Context(), actor, workspaceID, permManageImports); err != nil {
		writeError(w, 403, "forbidden")
		return
	}
	if !o.UpdatedAt.Equal(createdRevision.Time) || o.PublicURI.Valid || o.PublicCID.Valid {
		writeError(w, 409, "created occurrence changed or was published")
		return
	}
	var credits, later int
	if err := tx.QueryRow(ctx, `select count(*) from event_occurrence_profiles where occurrence_id=$1`, createdID).Scan(&credits); err != nil {
		writeError(w, 500, "could not inspect occurrence references")
		return
	}
	if err := tx.QueryRow(ctx, `select count(*) from cultural_import_actions where id<>$1 and rolled_back_at is null and (target_occurrence_id=$2 or created_occurrence_id=$2)`, r.PathValue("actionID"), createdID).Scan(&later); err != nil {
		writeError(w, 500, "could not inspect later actions")
		return
	}
	if credits > 0 || later > 0 {
		writeError(w, 409, "created occurrence has later references")
		return
	}
	tag, err := tx.Exec(ctx, `delete from event_occurrences where id=$1 and updated_at=$2 and public_uri is null and public_cid is null`, createdID, createdRevision.Time)
	if err != nil || tag.RowsAffected() != 1 {
		writeError(w, 409, "created occurrence changed; rollback refused")
		return
	}
	var rolledAt time.Time
	err = tx.QueryRow(ctx, `update cultural_import_actions set rolled_back_at=now(),rolled_back_by_person_id=$2,rollback_outcome='deleted' where id=$1 returning rolled_back_at`, r.PathValue("actionID"), actor).Scan(&rolledAt)
	if err != nil {
		writeError(w, 500, "could not record rollback")
		return
	}
	if err := a.audit(ctx, actor, "cultural_import.rolled_back", "cultural_import_action", r.PathValue("actionID"), map[string]any{"workspaceId": workspaceID, "outcome": "deleted"}); err != nil {
		writeError(w, 500, "could not record audit")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, 500, "could not roll back import")
		return
	}
	if err := a.authorize(r.Context(), actor, workspaceID, permManageImports); err != nil {
		writeError(w, 403, "forbidden")
		return
	}
	writeJSON(w, 200, map[string]any{"id": r.PathValue("actionID"), "rolledBackAt": rolledAt.UTC().Format(time.RFC3339Nano), "outcome": "deleted"})
}
