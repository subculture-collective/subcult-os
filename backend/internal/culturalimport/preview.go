// Package culturalimport parses the bounded, preview-only cultural occurrence
// CSV dialect. It has no database, network, or application dependencies.
package culturalimport

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"io"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"
)

const (
	// SchemaV1 identifies the exact header contract accepted by PreviewCSV.
	SchemaV1 = "subcult-occurrence-csv/v1"
	// MaxInputBytes bounds a complete preview input before CSV parsing.
	MaxInputBytes = 256 * 1024
	// MaxRows bounds data rows, excluding the required header row.
	MaxRows = 200
)

var headersV1 = []string{
	"source_record_id", "title", "description", "starts_at", "ends_at",
	"timezone", "status", "venue_name", "locality", "region", "country",
}

// SourceAssertion identifies the source that an operator asserts they may use.
// The parser records no claim about whether that assertion is true.
type SourceAssertion struct {
	SourceID   string
	SourceName string
	Assertion  string
}

// Candidate is a normalized, non-persisted occurrence proposal.
type Candidate struct {
	Row            int    `json:"row"`
	SourceRecordID string `json:"sourceRecordId"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	StartsAt       string `json:"startsAt"`
	EndsAt         string `json:"endsAt,omitempty"`
	Timezone       string `json:"timezone"`
	Status         string `json:"status"`
	VenueName      string `json:"venueName,omitempty"`
	Locality       string `json:"locality,omitempty"`
	Region         string `json:"region,omitempty"`
	Country        string `json:"country,omitempty"`
}

// PreviewError identifies a safe location and code without echoing CSV input.
type PreviewError struct {
	Row   int    `json:"row"`
	Field string `json:"field,omitempty"`
	Code  string `json:"code"`
}

// Preview is the complete result of one non-persisting CSV parse.
type Preview struct {
	Schema        string         `json:"schema"`
	SourceID      string         `json:"sourceId,omitempty"`
	ContentSHA256 string         `json:"contentSha256,omitempty"`
	Candidates    []Candidate    `json:"candidates"`
	Errors        []PreviewError `json:"errors"`
}

// PreviewCSV validates and normalizes one CSV input. It never writes data,
// looks up canonical records, fetches a URL, or derives consent.
func PreviewCSV(assertion SourceAssertion, input []byte) Preview {
	preview := Preview{Schema: SchemaV1, Candidates: []Candidate{}, Errors: []PreviewError{}}
	if len(input) > MaxInputBytes {
		return preview.withError(0, "", "input_too_large")
	}
	if !utf8.Valid(input) {
		return preview.withError(0, "", "invalid_utf8")
	}
	if !validSourceAssertion(assertion) {
		return preview.withError(0, "source_assertion", "invalid_source_assertion")
	}
	preview.SourceID = strings.TrimSpace(assertion.SourceID)
	digest := sha256.Sum256(input)
	preview.ContentSHA256 = hex.EncodeToString(digest[:])

	reader := csv.NewReader(strings.NewReader(string(input)))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return preview.withError(1, "", "invalid_csv")
	}
	if !sameHeaders(header, headersV1) {
		return preview.withError(1, "", "invalid_headers")
	}
	reader.FieldsPerRecord = len(headersV1)
	sourceRows := make(map[string][]int)
	for row := 2; ; row++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return preview.invalidBatch(row, "invalid_csv")
		}
		if row-1 > MaxRows {
			return preview.invalidBatch(row, "row_limit_exceeded")
		}
		sourceRecordID := strings.TrimSpace(record[0])
		if sourceRecordID != "" {
			sourceRows[sourceRecordID] = append(sourceRows[sourceRecordID], row)
		}
		candidate, validationErrors := candidateFromRecord(row, record)
		if len(validationErrors) != 0 {
			preview.Errors = append(preview.Errors, validationErrors...)
			continue
		}
		preview.Candidates = append(preview.Candidates, candidate)
	}
	duplicateIDs := make(map[string]struct{})
	for sourceRecordID, rows := range sourceRows {
		if len(rows) < 2 {
			continue
		}
		duplicateIDs[sourceRecordID] = struct{}{}
		for _, row := range rows {
			preview.Errors = append(preview.Errors, PreviewError{Row: row, Field: "source_record_id", Code: "duplicate_source_record_id"})
		}
	}
	if len(duplicateIDs) != 0 {
		candidates := make([]Candidate, 0, len(preview.Candidates))
		for _, candidate := range preview.Candidates {
			if _, duplicate := duplicateIDs[candidate.SourceRecordID]; !duplicate {
				candidates = append(candidates, candidate)
			}
		}
		preview.Candidates = candidates
	}
	sort.Slice(preview.Errors, func(left, right int) bool {
		if preview.Errors[left].Row != preview.Errors[right].Row {
			return preview.Errors[left].Row < preview.Errors[right].Row
		}
		if preview.Errors[left].Field != preview.Errors[right].Field {
			return preview.Errors[left].Field < preview.Errors[right].Field
		}
		return preview.Errors[left].Code < preview.Errors[right].Code
	})
	return preview
}

func (p Preview) withError(row int, field, code string) Preview {
	p.Errors = append(p.Errors, PreviewError{Row: row, Field: field, Code: code})
	return p
}

func (p Preview) invalidBatch(row int, code string) Preview {
	p.Candidates = []Candidate{}
	p.Errors = append(p.Errors, PreviewError{Row: row, Code: code})
	return p
}

func validSourceAssertion(assertion SourceAssertion) bool {
	return validText(assertion.SourceID, 200, true, false) &&
		validText(assertion.SourceName, 400, true, false) &&
		validText(assertion.Assertion, 1000, true, false)
}

func sameHeaders(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}

func candidateFromRecord(row int, record []string) (Candidate, []PreviewError) {
	values := make(map[string]string, len(headersV1))
	for index, header := range headersV1 {
		values[header] = strings.TrimSpace(record[index])
	}
	if errors := validateTextFields(row, values); len(errors) != 0 {
		return Candidate{}, errors
	}
	startsAt, startOK := parseRequiredRFC3339(values["starts_at"])
	endsAt, endOK := parseRFC3339(values["ends_at"])
	errors := make([]PreviewError, 0)
	if !startOK {
		errors = append(errors, PreviewError{Row: row, Field: "starts_at", Code: "invalid_timestamp"})
	}
	if values["ends_at"] != "" && !endOK {
		errors = append(errors, PreviewError{Row: row, Field: "ends_at", Code: "invalid_timestamp"})
	}
	if values["ends_at"] != "" && startOK && endOK && !endsAt.After(startsAt) {
		errors = append(errors, PreviewError{Row: row, Field: "ends_at", Code: "end_not_after_start"})
	}
	if values["timezone"] == "" || values["timezone"] == "Local" || !validTimezone(values["timezone"]) {
		errors = append(errors, PreviewError{Row: row, Field: "timezone", Code: "invalid_timezone"})
	}
	if !validStatus(values["status"]) {
		errors = append(errors, PreviewError{Row: row, Field: "status", Code: "invalid_status"})
	}
	if !validCountry(values["country"]) {
		errors = append(errors, PreviewError{Row: row, Field: "country", Code: "invalid_country"})
	}
	if len(errors) != 0 {
		return Candidate{}, errors
	}
	return Candidate{
		Row: row, SourceRecordID: values["source_record_id"], Title: values["title"], Description: values["description"],
		StartsAt: startsAt.UTC().Format(time.RFC3339Nano), EndsAt: formatOptionalTime(values["ends_at"], endsAt),
		Timezone: values["timezone"], Status: values["status"], VenueName: values["venue_name"],
		Locality: values["locality"], Region: values["region"], Country: strings.ToUpper(values["country"]),
	}, nil
}

func validateTextFields(row int, values map[string]string) []PreviewError {
	fields := []struct {
		name     string
		limit    int
		required bool
	}{
		{name: "source_record_id", limit: 200, required: true},
		{name: "title", limit: 1000, required: true},
		{name: "description", limit: 12000},
		{name: "venue_name", limit: 600},
		{name: "locality", limit: 400},
		{name: "region", limit: 400},
	}
	errors := make([]PreviewError, 0)
	for _, field := range fields {
		if !validText(values[field.name], field.limit, field.required, field.name == "description") {
			errors = append(errors, PreviewError{Row: row, Field: field.name, Code: "invalid_text"})
		}
	}
	return errors
}

func validText(value string, limit int, required, allowMultiline bool) bool {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return false
	}
	if utf8.RuneCountInString(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.Is(unicode.Cf, r) || (unicode.IsControl(r) && !(allowMultiline && (r == '\n' || r == '\r' || r == '\t'))) {
			return false
		}
	}
	if value == "" {
		return true
	}
	switch value[0] {
	case '=', '+', '-', '@':
		return false
	default:
		return true
	}
}

func validCountry(value string) bool {
	if value == "" {
		return true
	}
	if !validText(value, 2, false, false) || utf8.RuneCountInString(value) != 2 {
		return false
	}
	for _, r := range value {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}

func parseRequiredRFC3339(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	return parseRFC3339(value)
}

func parseRFC3339(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, true
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed, err == nil
}

func formatOptionalTime(value string, parsed time.Time) string {
	if value == "" {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

func validTimezone(value string) bool {
	_, err := time.LoadLocation(value)
	return err == nil
}

func validStatus(value string) bool {
	switch value {
	case "scheduled", "rescheduled", "postponed", "cancelled":
		return true
	default:
		return false
	}
}
