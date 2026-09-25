package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/subcult-os/internal/culturalimport"
	"github.com/jackc/pgx/v5"
)

const (
	// JSON escaping can expand a CSV substantially (for example quotes and
	// backslashes), so this outer bound is deliberately larger than the
	// parser's decoded 256 KiB CSV bound.
	culturalImportMaxRequestBytes = 2 * 1024 * 1024
	culturalImportMatchLimit      = 20
)

type culturalImportPreviewRequest struct {
	SourceID        string `json:"sourceId"`
	SourceName      string `json:"sourceName"`
	SourceAssertion string `json:"sourceAssertion"`
	CSV             string `json:"csv"`
}

type culturalImportMatchDTO struct {
	OccurrenceID string `json:"occurrenceId"`
	EventID      string `json:"eventId"`
	Name         string `json:"name"`
	StartsAt     string `json:"startsAt"`
	Status       string `json:"status"`
	UpdatedAt    string `json:"updatedAt"`
	updatedAt    time.Time
}

type culturalImportCandidateDTO struct {
	ID               string                   `json:"id"`
	Row              int                      `json:"row"`
	SourceRecordID   string                   `json:"sourceRecordId"`
	Title            string                   `json:"title"`
	Description      string                   `json:"description,omitempty"`
	StartsAt         string                   `json:"startsAt"`
	EndsAt           string                   `json:"endsAt,omitempty"`
	Timezone         string                   `json:"timezone"`
	Status           string                   `json:"status"`
	VenueName        string                   `json:"venueName,omitempty"`
	Locality         string                   `json:"locality,omitempty"`
	Region           string                   `json:"region,omitempty"`
	Country          string                   `json:"country,omitempty"`
	Matches          []culturalImportMatchDTO `json:"matches"`
	Ambiguous        bool                     `json:"ambiguous"`
	MatchesTruncated bool                     `json:"matchesTruncated"`
}

type culturalImportPreviewErrorDTO struct {
	Row   int    `json:"row"`
	Field string `json:"field,omitempty"`
	Code  string `json:"code"`
}

type culturalImportPreviewDTO struct {
	ID            string                          `json:"id"`
	WorkspaceID   string                          `json:"workspaceId"`
	Schema        string                          `json:"schema"`
	SourceID      string                          `json:"sourceId"`
	ContentSHA256 string                          `json:"contentSha256"`
	CreatedAt     string                          `json:"createdAt"`
	Candidates    []culturalImportCandidateDTO    `json:"candidates"`
	Errors        []culturalImportPreviewErrorDTO `json:"errors"`
	Actions       []culturalImportActionDTO       `json:"actions"`
}

func (a *App) handleCreateCulturalImportPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	actorID, ok := a.requirePermission(r, workspaceID, permManageImports)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, culturalImportMaxRequestBytes)
	var req culturalImportPreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid import preview request")
		return
	}
	assertion := culturalimport.SourceAssertion{
		SourceID: strings.TrimSpace(req.SourceID), SourceName: strings.TrimSpace(req.SourceName), Assertion: strings.TrimSpace(req.SourceAssertion),
	}
	if !culturalimport.ValidSourceAssertion(assertion) {
		writeError(w, http.StatusBadRequest, "invalid source assertion")
		return
	}
	preview := culturalimport.PreviewCSV(assertion, []byte(req.CSV))
	if previewHasFatalInputError(preview.Errors) {
		writeError(w, http.StatusBadRequest, "invalid import input")
		return
	}

	tx, err := a.db.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start import preview")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	result, err := persistCulturalImportPreview(r.Context(), tx, workspaceID, actorID, assertion, preview)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save import preview")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save import preview")
		return
	}
	// A role may be revoked while the transaction is running. Do not return a
	// private preview after that change, even though the accepted preview is
	// retained for the authorized actor's workspace provenance.
	if err := a.authorize(r.Context(), actorID, workspaceID, permManageImports); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func previewHasFatalInputError(errors []culturalimport.PreviewError) bool {
	for _, item := range errors {
		switch item.Code {
		case "input_too_large", "invalid_utf8":
			return true
		}
	}
	return false
}

func persistCulturalImportPreview(ctx context.Context, tx pgx.Tx, workspaceID, actorID string, assertion culturalimport.SourceAssertion, preview culturalimport.Preview) (culturalImportPreviewDTO, error) {
	result := culturalImportPreviewDTO{
		WorkspaceID:   workspaceID,
		Schema:        preview.Schema,
		SourceID:      preview.SourceID,
		ContentSHA256: preview.ContentSHA256,
		Candidates:    []culturalImportCandidateDTO{},
		Errors:        []culturalImportPreviewErrorDTO{},
		Actions:       []culturalImportActionDTO{},
	}
	var createdAt time.Time
	err := tx.QueryRow(ctx, `
		insert into cultural_import_previews
			(workspace_id, schema_name, source_id, source_name, source_assertion, content_sha256, created_by_person_id)
		values ($1, $2, $3, $4, $5, $6, $7)
		returning id, created_at
	`, workspaceID, preview.Schema, preview.SourceID, assertion.SourceName, assertion.Assertion, preview.ContentSHA256, actorID).Scan(&result.ID, &createdAt)
	if err != nil {
		return culturalImportPreviewDTO{}, err
	}
	result.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)

	for _, item := range preview.Errors {
		if _, err := tx.Exec(ctx, `
			insert into cultural_import_preview_errors (import_id, row_number, field_name, code)
			values ($1, $2, $3, $4)
		`, result.ID, item.Row, item.Field, item.Code); err != nil {
			return culturalImportPreviewDTO{}, err
		}
		result.Errors = append(result.Errors, culturalImportPreviewErrorDTO{Row: item.Row, Field: item.Field, Code: item.Code})
	}
	for _, candidate := range preview.Candidates {
		startsAt, err := time.Parse(time.RFC3339Nano, candidate.StartsAt)
		if err != nil {
			return culturalImportPreviewDTO{}, err
		}
		var endsAt any
		if candidate.EndsAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, candidate.EndsAt)
			if err != nil {
				return culturalImportPreviewDTO{}, err
			}
			endsAt = parsed
		}
		stored := culturalImportCandidateDTO{
			Row: candidate.Row, SourceRecordID: candidate.SourceRecordID, Title: candidate.Title,
			Description: candidate.Description, StartsAt: candidate.StartsAt, EndsAt: candidate.EndsAt,
			Timezone: candidate.Timezone, Status: candidate.Status, VenueName: candidate.VenueName,
			Locality: candidate.Locality, Region: candidate.Region, Country: candidate.Country,
			Matches: []culturalImportMatchDTO{},
		}
		var matchCount int
		if err := tx.QueryRow(ctx, `
			select count(*)
			from event_occurrences
			where workspace_id = $1
			  and starts_at = $2
			  and lower(btrim(name)) = lower(btrim($3))
		`, workspaceID, startsAt, candidate.Title).Scan(&matchCount); err != nil {
			return culturalImportPreviewDTO{}, err
		}
		stored.Ambiguous = matchCount > 1
		stored.MatchesTruncated = matchCount > culturalImportMatchLimit
		rows, err := tx.Query(ctx, `
			select id, event_id, name, starts_at, status, updated_at
			from event_occurrences
			where workspace_id = $1
			  and starts_at = $2
			  and lower(btrim(name)) = lower(btrim($3))
			order by id
			limit $4
		`, workspaceID, startsAt, candidate.Title, culturalImportMatchLimit)
		if err != nil {
			return culturalImportPreviewDTO{}, err
		}
		matches := make([]culturalImportMatchDTO, 0, min(matchCount, culturalImportMatchLimit))
		for rows.Next() {
			var match culturalImportMatchDTO
			var matchStartsAt time.Time
			if err := rows.Scan(&match.OccurrenceID, &match.EventID, &match.Name, &matchStartsAt, &match.Status, &match.updatedAt); err != nil {
				rows.Close()
				return culturalImportPreviewDTO{}, err
			}
			match.StartsAt = matchStartsAt.UTC().Format(time.RFC3339Nano)
			match.UpdatedAt = match.updatedAt.UTC().Format(time.RFC3339Nano)
			matches = append(matches, match)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return culturalImportPreviewDTO{}, err
		}
		rows.Close()
		if err := tx.QueryRow(ctx, `
			insert into cultural_import_candidates
				(import_id, workspace_id, row_number, source_record_id, title, description, starts_at, ends_at, timezone, status, venue_name, locality, region, country, match_count, matches_truncated)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			returning id
		`, result.ID, workspaceID, candidate.Row, candidate.SourceRecordID, candidate.Title, candidate.Description,
			startsAt, endsAt, candidate.Timezone, candidate.Status, candidate.VenueName, candidate.Locality,
			candidate.Region, candidate.Country, matchCount, stored.MatchesTruncated).Scan(&stored.ID); err != nil {
			return culturalImportPreviewDTO{}, err
		}
		for _, match := range matches {
			if _, err := tx.Exec(ctx, `
				insert into cultural_import_candidate_matches
					(candidate_id, workspace_id, occurrence_id, occurrence_id_snapshot, event_id_snapshot, name_snapshot, starts_at_snapshot, status_snapshot, updated_at_snapshot)
				values ($1, $2, $3, $3, $4, $5, $6, $7, $8)
			`, stored.ID, workspaceID, match.OccurrenceID, match.EventID, match.Name, match.StartsAt, match.Status, match.updatedAt); err != nil {
				return culturalImportPreviewDTO{}, err
			}
		}
		stored.Matches = matches
		result.Candidates = append(result.Candidates, stored)
	}
	return result, nil
}
