package culturalimport

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

var validAssertion = SourceAssertion{
	SourceID: "catalog-2026", SourceName: "Community calendar", Assertion: "Operator supplied this source for review",
}

func TestPreviewCSVNormalizesValidCandidate(t *testing.T) {
	preview := PreviewCSV(validAssertion, csvInput(
		"record-1,  Night Show  ,  A local show  ,2026-11-01T01:30:00-05:00,2026-11-01T01:30:00-06:00,America/Chicago,scheduled,The Hall,Chicago,Illinois,us",
	))
	if len(preview.Errors) != 0 || len(preview.Candidates) != 1 {
		t.Fatalf("preview = %#v, want one valid candidate", preview)
	}
	candidate := preview.Candidates[0]
	if preview.Schema != SchemaV1 || preview.SourceID != validAssertion.SourceID || len(preview.ContentSHA256) != 64 {
		t.Fatalf("preview metadata = %#v", preview)
	}
	if candidate.Row != 2 || candidate.Title != "Night Show" || candidate.Description != "A local show" ||
		candidate.StartsAt != "2026-11-01T06:30:00Z" || candidate.EndsAt != "2026-11-01T07:30:00Z" ||
		candidate.Country != "US" {
		t.Fatalf("candidate = %#v", candidate)
	}
}

func TestPreviewCSVRejectsInvalidSourceOrInput(t *testing.T) {
	tests := []struct {
		name      string
		assertion SourceAssertion
		input     []byte
		wantCode  string
	}{
		{"missing assertion", SourceAssertion{}, csvInput(validRow()), "invalid_source_assertion"},
		{"formula assertion", SourceAssertion{SourceID: "=source", SourceName: "Source", Assertion: "review"}, csvInput(validRow()), "invalid_source_assertion"},
		{"invalid utf8", validAssertion, []byte{0xff}, "invalid_utf8"},
		{"oversized input", validAssertion, []byte(strings.Repeat("x", MaxInputBytes+1)), "input_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preview := PreviewCSV(tt.assertion, tt.input)
			if len(preview.Errors) != 1 || preview.Errors[0].Code != tt.wantCode || len(preview.Candidates) != 0 {
				t.Fatalf("preview = %#v, want %s", preview, tt.wantCode)
			}
		})
	}
}

func TestPreviewCSVRejectsHeadersAndCSVShape(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		code  string
	}{
		{"unknown header", []byte("source_record_id,title,description,starts_at,ends_at,timezone,status,venue_name,locality,region,secret_email\n"), "invalid_headers"},
		{"duplicate header", []byte("source_record_id,title,description,starts_at,ends_at,timezone,status,venue_name,locality,region,region\n"), "invalid_headers"},
		{"extra row column", append(csvInput(validRow()), []byte("record-2,Title,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US,extra\n")...), "invalid_csv"},
		{"trailing malformed data", append(csvInput(validRow()), []byte("\"unterminated\n")...), "invalid_csv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preview := PreviewCSV(validAssertion, tt.input)
			if len(preview.Candidates) != 0 || !hasCode(preview.Errors, tt.code) {
				t.Fatalf("preview = %#v, want %s with no candidates", preview, tt.code)
			}
		})
	}
}

func TestPreviewCSVRejectsRowValidationFailures(t *testing.T) {
	tests := []struct {
		name string
		row  string
		code string
	}{
		{"formula title", "record-1,=SUM(A1),,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US", "invalid_text"},
		{"missing start", "record-1,Title,,,,UTC,scheduled,,,,US", "invalid_timestamp"},
		{"invalid timestamp", "record-1,Title,,not-a-time,,UTC,scheduled,,,,US", "invalid_timestamp"},
		{"invalid end", "record-1,Title,,2026-01-01T10:00:00Z,2026-01-01T10:00:00Z,UTC,scheduled,,,,US", "end_not_after_start"},
		{"invalid timezone", "record-1,Title,,2026-01-01T10:00:00Z,,Not/A_Zone,scheduled,,,,US", "invalid_timezone"},
		{"local timezone", "record-1,Title,,2026-01-01T10:00:00Z,,Local,scheduled,,,,US", "invalid_timezone"},
		{"invalid status", "record-1,Title,,2026-01-01T10:00:00Z,,UTC,published,,,,US", "invalid_status"},
		{"invalid country", "record-1,Title,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,USA", "invalid_country"},
		{"non-letter country", "record-1,Title,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,12", "invalid_country"},
		{"format control", "record-1,\ufeffTitle,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US", "invalid_text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preview := PreviewCSV(validAssertion, csvInput(tt.row))
			if len(preview.Candidates) != 0 || !hasCode(preview.Errors, tt.code) {
				t.Fatalf("preview = %#v, want %s with no candidates", preview, tt.code)
			}
		})
	}
}

func TestPreviewCSVRemovesDuplicateSourceIDsForReview(t *testing.T) {
	preview := PreviewCSV(validAssertion, csvInput(
		validRow(),
		"record-1,Another title,,2026-01-02T10:00:00Z,,UTC,scheduled,,,,US",
	))
	if len(preview.Candidates) != 0 || !hasError(preview.Errors, PreviewError{Row: 2, Field: "source_record_id", Code: "duplicate_source_record_id"}) ||
		!hasError(preview.Errors, PreviewError{Row: 3, Field: "source_record_id", Code: "duplicate_source_record_id"}) {
		t.Fatalf("preview = %#v, want no candidates and explicit duplicate review errors", preview)
	}
}

func TestPreviewCSVDuplicateInvalidThenValidSourceIDRemainsAmbiguous(t *testing.T) {
	preview := PreviewCSV(validAssertion, csvInput(
		"record-1,, ,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US",
		validRow(),
	))
	if len(preview.Candidates) != 0 || !hasCode(preview.Errors, "invalid_text") ||
		!hasError(preview.Errors, PreviewError{Row: 2, Field: "source_record_id", Code: "duplicate_source_record_id"}) ||
		!hasError(preview.Errors, PreviewError{Row: 3, Field: "source_record_id", Code: "duplicate_source_record_id"}) {
		t.Fatalf("preview = %#v, want invalid and duplicate review errors without candidate", preview)
	}
}

func TestPreviewCSVDuplicateErrorsAreDeterministic(t *testing.T) {
	input := csvInput(
		"record-a,First,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US",
		"record-b,Second,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US",
		"record-a,Third,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US",
		"record-b,Fourth,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US",
	)
	first := PreviewCSV(validAssertion, input)
	second := PreviewCSV(validAssertion, input)
	if !reflect.DeepEqual(first.Errors, second.Errors) {
		t.Fatalf("errors differ across identical previews: first=%#v second=%#v", first.Errors, second.Errors)
	}
}

func TestPreviewCSVRejectsExcessRowsAndKeepsErrorsFreeOfInput(t *testing.T) {
	rows := make([]string, MaxRows+1)
	for index := range rows {
		rows[index] = "record-" + strings.Repeat("x", 1) + string(rune('a'+index%26)) + ",Title,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US"
	}
	preview := PreviewCSV(validAssertion, csvInput(rows...))
	if len(preview.Candidates) != 0 || !hasCode(preview.Errors, "row_limit_exceeded") {
		t.Fatalf("preview = %#v, want row limit failure", preview)
	}
	encoded, err := json.Marshal(preview.Errors)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "record-") || strings.Contains(string(encoded), "Title") {
		t.Fatalf("preview errors exposed CSV content: %s", encoded)
	}
}

func csvInput(rows ...string) []byte {
	return []byte(strings.Join(append([]string{strings.Join(headersV1, ",")}, rows...), "\n") + "\n")
}

func validRow() string {
	return "record-1,Title,,2026-01-01T10:00:00Z,,UTC,scheduled,,,,US"
}

func hasCode(errors []PreviewError, want string) bool {
	for _, err := range errors {
		if err.Code == want {
			return true
		}
	}
	return false
}

func hasError(errors []PreviewError, want PreviewError) bool {
	for _, err := range errors {
		if err == want {
			return true
		}
	}
	return false
}
