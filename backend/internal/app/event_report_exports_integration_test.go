package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettlementReportExportsFinanceBoundary(t *testing.T) {
	fx := settledCSVExportFixture(t)
	for _, suffix := range []string{"settlement.md", "settlement-print.html"} {
		t.Run(suffix, func(t *testing.T) {
			path := "/api/events/" + fx.eventID + "/exports/" + suffix
			owner := getReportExport(t, fx.fixture.app, fx.fixture.ownerCookie, path, http.StatusOK)
			for _, h := range []string{"Cache-Control", "X-Content-Type-Options", "Content-Disposition"} {
				if owner.Header().Get(h) == "" {
					t.Fatalf("missing %s", h)
				}
			}
			if strings.Contains(owner.Body.String(), fx.fixture.email("paid")) {
				t.Fatal("leaked private ticket email")
			}
			_, member := memberIdentity(t, fx.fixture)
			for _, role := range []string{roleOrganizer, roleCrew, roleDoor} {
				_, _ = fx.fixture.app.db.Exec(t.Context(), `update workspace_members set role=$2, revoked_at=null where id=$1`, member, role)
				getReportExport(t, fx.fixture.app, fx.fixture.memberCookie, path, http.StatusForbidden)
			}
			_, _ = fx.fixture.app.db.Exec(t.Context(), `update workspace_members set role='finance', revoked_at=null where id=$1`, member)
			getReportExport(t, fx.fixture.app, fx.fixture.memberCookie, path, http.StatusOK)
			_, _ = fx.fixture.app.db.Exec(t.Context(), `update workspace_members set revoked_at=now() where id=$1`, member)
			getReportExport(t, fx.fixture.app, fx.fixture.memberCookie, path, http.StatusForbidden)
		})
	}
}

func TestSettlementReportExportsProvenanceAndUnavailableSettlement(t *testing.T) {
	fx := settledCSVExportFixture(t)
	postJSON(t, fx.fixture.app, fx.fixture.ownerCookie, "/api/events/"+fx.eventID+"/settlement/adjustments", map[string]any{
		"amountCents": -75, "label": "Cash correction", "reason": "Drawer reconciliation",
	}, http.StatusOK)
	postJSON(t, fx.fixture.app, fx.fixture.ownerCookie, "/api/events/"+fx.eventID+"/settlement/finalize", map[string]any{}, http.StatusOK)

	var adjustmentID, actorID string
	if err := fx.fixture.app.db.QueryRow(t.Context(), `
		select a.id, a.created_by_person_id
		from event_settlement_adjustments a
		join event_settlements s on s.id = a.settlement_id
		where s.event_id = $1
	`, fx.eventID).Scan(&adjustmentID, &actorID); err != nil {
		t.Fatal(err)
	}

	markdown := getReportExport(t, fx.fixture.app, fx.fixture.ownerCookie, "/api/events/"+fx.eventID+"/exports/settlement.md", http.StatusOK)
	for _, want := range []string{"Status: finalized", "Finalized at (UTC):", "Finalized by person ID:", "Correction ID: " + strings.ReplaceAll(adjustmentID, "-", "\\-"), "Created by person ID: " + strings.ReplaceAll(actorID, "-", "\\-"), "Created at (UTC):", "-USD 0.75"} {
		if !strings.Contains(markdown.Body.String(), want) {
			t.Fatalf("markdown report missing %q: %s", want, markdown.Body.String())
		}
	}
	if got := markdown.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("markdown content type = %q", got)
	}

	printable := getReportExport(t, fx.fixture.app, fx.fixture.ownerCookie, "/api/events/"+fx.eventID+"/exports/settlement-print.html", http.StatusOK)
	for _, want := range []string{"<main>", "<table>", "<ol>", "Correction ID", "Finalized by person ID"} {
		if !strings.Contains(printable.Body.String(), want) {
			t.Fatalf("printable report missing %q: %s", want, printable.Body.String())
		}
	}
	if got := printable.Header().Get("Content-Security-Policy"); got != "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'" {
		t.Fatalf("printable CSP = %q", got)
	}
	if strings.Contains(printable.Body.String(), "<script") || strings.Contains(printable.Body.String(), "http://") || strings.Contains(printable.Body.String(), "https://") {
		t.Fatalf("printable report includes an active or remote asset: %s", printable.Body.String())
	}

	unclosed := createEvent(t, fx.fixture, "No settlement", 1)
	for _, suffix := range []string{"settlement.md", "settlement-print.html"} {
		getReportExport(t, fx.fixture.app, fx.fixture.ownerCookie, "/api/events/"+mustString(t, unclosed, "id")+"/exports/"+suffix, http.StatusNotFound)
	}
}
func getReportExport(t *testing.T, a *App, c *http.Cookie, path string, want int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s=%d %s", path, w.Code, w.Body.String())
	}
	return w
}
