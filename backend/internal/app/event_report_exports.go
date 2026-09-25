package app

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// cents formats stored integer minor units without floating-point conversion.
func cents(c int, currency string) string { return cents64(int64(c), currency) }

func cents64(value int64, currency string) string {
	sign := ""
	var magnitude uint64
	if value < 0 {
		sign = "-"
		magnitude = uint64(-(value + 1)) + 1 // safe for math.MinInt64
	} else {
		magnitude = uint64(value)
	}
	return sign + strings.ToUpper(currency) + " " + strconv.FormatUint(magnitude/100, 10) + "." + fmt.Sprintf("%02d", magnitude%100)
}

// markdownEscape makes untrusted values inline-only. The report controls every
// Markdown line break and marker, so a title or correction cannot become a
// heading, list, table, link, or thematic break.
func markdownEscape(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	var escaped strings.Builder
	escaped.Grow(len(value) * 2)
	for _, r := range value {
		if r == '\n' {
			escaped.WriteByte(' ')
			continue
		}
		if r >= '!' && r <= '~' && !(r >= '0' && r <= '9') && !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z') {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(r)
	}
	return escaped.String()
}

func reportTimestamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

type settlementReportView struct {
	EventTitle, EventID, ReportID, SettlementID string
	StartsAt, GeneratedAt, Status               string
	FinalizedAt, FinalizedBy, Currency          string
	GrossPaid, AdjustmentTotal, NetTotal        string
	Corrections                                 []settlementReportCorrection
}

type settlementReportCorrection struct {
	ID, Amount, Label, Reason, ActorID, CreatedAt string
}

func settlementReportFromSnapshot(snapshot eventSettlementExportSnapshot) settlementReportView {
	var adjustmentTotal int64
	corrections := make([]settlementReportCorrection, 0, len(snapshot.Adjustments))
	for _, adjustment := range snapshot.Adjustments {
		adjustmentTotal += int64(adjustment.AmountCents)
		corrections = append(corrections, settlementReportCorrection{
			ID: adjustment.ID, Amount: cents(adjustment.AmountCents, snapshot.Currency),
			Label: adjustment.Label, Reason: adjustment.Reason, ActorID: adjustment.CreatedByPersonID,
			CreatedAt: reportTimestamp(adjustment.CreatedAt),
		})
	}
	finalizedAt, finalizedBy := "Not finalized", "Not finalized"
	if snapshot.FinalizedAt.Valid {
		finalizedAt = reportTimestamp(snapshot.FinalizedAt.Time)
	}
	if snapshot.FinalizedByPersonID.Valid {
		finalizedBy = snapshot.FinalizedByPersonID.String
	}
	return settlementReportView{
		EventTitle: snapshot.EventTitle, EventID: snapshot.EventID, ReportID: snapshot.ReportID, SettlementID: snapshot.SettlementID,
		StartsAt: reportTimestamp(snapshot.StartsAt), GeneratedAt: reportTimestamp(snapshot.GeneratedAt), Status: snapshot.Status,
		FinalizedAt: finalizedAt, FinalizedBy: finalizedBy, Currency: strings.ToUpper(snapshot.Currency),
		GrossPaid: cents(snapshot.GrossPaidRevenueCents, snapshot.Currency), AdjustmentTotal: cents64(adjustmentTotal, snapshot.Currency),
		NetTotal: cents64(int64(snapshot.GrossPaidRevenueCents)+adjustmentTotal, snapshot.Currency), Corrections: corrections,
	}
}

func reportText(snapshot eventSettlementExportSnapshot) string {
	report := settlementReportFromSnapshot(snapshot)
	var b strings.Builder
	fmt.Fprintf(&b, "# Settlement report\n\nEvent: %s\n\nEvent ID: %s  \nReport ID: %s  \nSettlement ID: %s  \nEvent starts (UTC): %s  \nSettlement generated (UTC): %s\n\n", markdownEscape(report.EventTitle), markdownEscape(report.EventID), markdownEscape(report.ReportID), markdownEscape(report.SettlementID), report.StartsAt, report.GeneratedAt)
	fmt.Fprintf(&b, "## Stored settlement\n\nStatus: %s  \nFinalized at (UTC): %s  \nFinalized by person ID: %s  \nCurrency: %s  \nPaid revenue: %s  \nAdjustments: %s  \nNet total: %s\n\nBudgets and payables are not tracked in this report.\n\n## Corrections\n\n", markdownEscape(report.Status), markdownEscape(report.FinalizedAt), markdownEscape(report.FinalizedBy), markdownEscape(report.Currency), report.GrossPaid, report.AdjustmentTotal, report.NetTotal)
	if len(report.Corrections) == 0 {
		b.WriteString("No corrections recorded.\n")
		return b.String()
	}
	for _, correction := range report.Corrections {
		fmt.Fprintf(&b, "- Correction ID: %s  \n  Amount: %s  \n  Label: %s  \n  Reason: %s  \n  Created by person ID: %s  \n  Created at (UTC): %s\n", markdownEscape(correction.ID), correction.Amount, markdownEscape(correction.Label), markdownEscape(correction.Reason), markdownEscape(correction.ActorID), correction.CreatedAt)
	}
	return b.String()
}

var settlementReportHTMLTemplate = template.Must(template.New("settlement-report").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Settlement report</title><style>
body{font:16px system-ui,sans-serif;line-height:1.45;max-width:48rem;margin:2rem auto;padding:0 1rem;color:#111}h1,h2{line-height:1.15}dl{display:grid;grid-template-columns:max-content 1fr;gap:.35rem 1rem}dt{font-weight:700}dd{margin:0;overflow-wrap:anywhere}table{border-collapse:collapse;width:100%}th,td{border:1px solid #777;padding:.45rem;text-align:left;vertical-align:top}th{background:#eee}.preserve-lines{white-space:pre-line}@media print{body{max-width:none;margin:0}}
</style></head><body><main><header><h1>Settlement report</h1><p class="preserve-lines">{{.EventTitle}}</p></header><section aria-labelledby="identifiers"><h2 id="identifiers">Report identity</h2><dl><dt>Event ID</dt><dd>{{.EventID}}</dd><dt>Report ID</dt><dd>{{.ReportID}}</dd><dt>Settlement ID</dt><dd>{{.SettlementID}}</dd><dt>Event starts (UTC)</dt><dd>{{.StartsAt}}</dd><dt>Settlement generated (UTC)</dt><dd>{{.GeneratedAt}}</dd></dl></section><section aria-labelledby="settlement"><h2 id="settlement">Stored settlement</h2><dl><dt>Status</dt><dd>{{.Status}}</dd><dt>Finalized at (UTC)</dt><dd>{{.FinalizedAt}}</dd><dt>Finalized by person ID</dt><dd>{{.FinalizedBy}}</dd><dt>Currency</dt><dd>{{.Currency}}</dd></dl><table><caption>Stored monetary totals</caption><thead><tr><th scope="col">Paid revenue</th><th scope="col">Adjustment total</th><th scope="col">Net total</th></tr></thead><tbody><tr><td>{{.GrossPaid}}</td><td>{{.AdjustmentTotal}}</td><td>{{.NetTotal}}</td></tr></tbody></table><p>Budgets and payables are not tracked in this report.</p></section><section aria-labelledby="corrections"><h2 id="corrections">Corrections</h2>{{if .Corrections}}<ol>{{range .Corrections}}<li><dl><dt>Correction ID</dt><dd>{{.ID}}</dd><dt>Amount</dt><dd>{{.Amount}}</dd><dt>Label</dt><dd class="preserve-lines">{{.Label}}</dd><dt>Reason</dt><dd class="preserve-lines">{{.Reason}}</dd><dt>Created by person ID</dt><dd>{{.ActorID}}</dd><dt>Created at (UTC)</dt><dd>{{.CreatedAt}}</dd></dl></li>{{end}}</ol>{{else}}<p>No corrections recorded.</p>{{end}}</section></main></body></html>`))

func (a *App) handleSettlementMarkdown(w http.ResponseWriter, r *http.Request) {
	a.writeSettlementReport(w, r, "markdown")
}
func (a *App) handleSettlementPrint(w http.ResponseWriter, r *http.Request) {
	a.writeSettlementReport(w, r, "html")
}

func (a *App) writeSettlementReport(w http.ResponseWriter, r *http.Request, kind string) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	event, err := a.loadEventDetails(r.Context(), r.PathValue("eventID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "settlement export unavailable")
		} else {
			writeError(w, http.StatusInternalServerError, "could not load event")
		}
		return
	}
	if _, ok := a.requirePermission(r, event.WorkspaceID, permFinance); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	snapshot, err := a.loadEventSettlementExport(r.Context(), event.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "settlement export unavailable")
		} else {
			writeError(w, http.StatusInternalServerError, "could not load settlement export")
		}
		return
	}
	if _, ok := a.requirePermission(r, event.WorkspaceID, permFinance); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if kind == "markdown" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="event-settlement.md"`)
		_, _ = w.Write([]byte(reportText(snapshot)))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="event-settlement-print.html"`)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'")
	_ = settlementReportHTMLTemplate.Execute(w, settlementReportFromSnapshot(snapshot))
}
