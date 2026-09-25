package app

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSettlementReportEncodingAndTotals(t *testing.T) {
	s := eventSettlementExportSnapshot{EventTitle: "<script>x</script>\n# title", EventID: "event", ReportID: "report", SettlementID: "settle", Currency: "usd", GrossPaidRevenueCents: 12345, StartsAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), GeneratedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Status: "finalized", FinalizedAt: sql.NullTime{Time: time.Date(2026, 1, 2, 4, 5, 6, 0, time.UTC), Valid: true}, FinalizedByPersonID: sql.NullString{String: "person-finalizer", Valid: true}, Adjustments: []eventSettlementAdjustmentRow{{ID: "correction-1", AmountCents: -45, Label: "[label]", Reason: "<img src=x>", CreatedByPersonID: "person-corrector", CreatedAt: time.Date(2026, 1, 2, 5, 6, 7, 0, time.UTC)}}}
	got := reportText(s)
	for _, bad := range []string{"<script>", "<img src=x>", "\n# title"} {
		if strings.Contains(got, bad) {
			t.Fatalf("unescaped %q in %q", bad, got)
		}
	}
	for _, want := range []string{"USD 123.45", "-USD 0.45", "USD 123.00", "Budgets and payables are not tracked", "2026-01-02T03:04:05Z", "Status: finalized", "Finalized by person ID: person\\-finalizer", "Correction ID: correction\\-1", "Created by person ID: person\\-corrector", "Created at (UTC): 2026-01-02T05:06:07Z", "\\<script\\>", "\\<img src\\=x\\>", "  \n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q: %s", want, got)
		}
	}
}

func TestSettlementReportMarkdownFlattensStructuralInjection(t *testing.T) {
	s := eventSettlementExportSnapshot{EventTitle: "title\n- injected list\n| table | column |\n| --- | --- |\n---", EventID: "event", ReportID: "report", SettlementID: "settlement", Currency: "usd", StartsAt: time.Now(), GeneratedAt: time.Now(), Adjustments: []eventSettlementAdjustmentRow{{ID: "correction", AmountCents: 1, Label: "label\n+ injected list", Reason: "reason\n| table | column |\n---", CreatedByPersonID: "person", CreatedAt: time.Now()}}}
	got := reportText(s)
	for _, unwanted := range []string{"\n- injected list", "\n+ injected list", "\n| table", "\n---"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("untrusted Markdown became structural content %q: %s", unwanted, got)
		}
	}
	for _, want := range []string{"title \\- injected list \\| table \\| column \\| \\| \\-\\-\\- \\| \\-\\-\\- \\|", "label \\+ injected list", "reason \\| table \\| column \\| \\-\\-\\-"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing escaped untrusted text %q: %s", want, got)
		}
	}
}

func TestSettlementReportHTMLIsSemanticAndEscaped(t *testing.T) {
	s := eventSettlementExportSnapshot{EventTitle: "<script>alert(1)</script>", EventID: "event", ReportID: "report", SettlementID: "settle", Currency: "usd", Status: "open", GrossPaidRevenueCents: 12345, StartsAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), GeneratedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Adjustments: []eventSettlementAdjustmentRow{{ID: "correction", AmountCents: -45, Label: "<img src=x>", Reason: "reason", CreatedByPersonID: "person", CreatedAt: time.Date(2026, 1, 2, 5, 6, 7, 0, time.UTC)}}}
	w := httptest.NewRecorder()
	if err := settlementReportHTMLTemplate.Execute(w, settlementReportFromSnapshot(s)); err != nil {
		t.Fatal(err)
	}
	body := w.Body.String()
	for _, want := range []string{"<main>", "<section", "<table>", "<ol>", "Correction ID", "USD 123.45"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing semantic report content %q: %s", want, body)
		}
	}
	for _, bad := range []string{"<script>alert", "<img src=x>", "<pre>"} {
		if strings.Contains(body, bad) {
			t.Fatalf("unsafe or non-semantic report content %q: %s", bad, body)
		}
	}
	if !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "&lt;img src=x&gt;") {
		t.Fatalf("expected escaped report text: %s", body)
	}
}

func TestSettlementReportUnavailableDatabaseReturnsServerError(t *testing.T) {
	a := &App{}
	r := httptest.NewRequest("GET", "/api/events/event/exports/settlement.md", nil)
	r.SetPathValue("eventID", "event")
	w := httptest.NewRecorder()
	a.writeSettlementReport(w, r, "markdown")
	if w.Code != 500 || !strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatalf("unexpected unavailable database response: %d %s", w.Code, w.Body.String())
	}
}

func TestSettlementCentsHandlesNegativeExtremes(t *testing.T) {
	if cents(-1, "usd") != "-USD 0.01" {
		t.Fatal(cents(-1, "usd"))
	}
	min := int(^uint(0) >> 1)
	if !strings.HasPrefix(cents(-min-1, "usd"), "-USD ") {
		t.Fatal("minimum int overflow")
	}
}
