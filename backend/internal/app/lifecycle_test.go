package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stripe/stripe-go/v85"
)

func TestFirstEventLifecycle(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 2)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")
	guestEmail := fx.email("guest")
	ticket := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": guestEmail, "displayName": "Guest"}, http.StatusOK)
	grantFixtureDoorPermission(t, fx)
	checkIn := postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": mustString(t, ticket.JSON, "code")}, http.StatusOK)
	report := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	reportObj := mustObject(t, report.JSON)

	if int(reportObj["ticketsReserved"].(float64)) != 1 || int(reportObj["ticketsCheckedIn"].(float64)) != 1 || int(reportObj["noShows"].(float64)) != 0 {
		t.Fatalf("unexpected report counts: %#v", report.JSON)
	}
	if mustString(t, checkIn.JSON, "status") != "checked_in" {
		t.Fatalf("unexpected check-in status: %#v", checkIn.JSON)
	}
}

func TestFirstEventLifecycleFreeReportSettlementSummary(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("free"), "Free Guest", "free", 0, "usd")
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("cancelled"), "Cancelled Guest", "cancelled", 1500, "usd")

	report := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	reportObj := mustObject(t, report.JSON)
	summary := mustObject(t, reportObj["settlementSummary"])

	if summary["currency"] != "usd" || int(summary["grossPaidRevenueCents"].(float64)) != 0 || int(summary["paidTicketCount"].(float64)) != 0 || int(summary["pendingTicketCount"].(float64)) != 0 || int(summary["cancelledTicketCount"].(float64)) != 1 || int(summary["freeTicketCount"].(float64)) != 1 || int(summary["reservedCount"].(float64)) != 1 {
		t.Fatalf("unexpected settlement summary: %#v", summary)
	}
}

func TestFirstEventLifecycleSettlementAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("paid"), "Paid Guest", "paid", 1500, "usd")
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("free"), "Free Guest", "free", 0, "usd")
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("cancelled"), "Cancelled Guest", "cancelled", 1500, "usd")

	getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement", http.StatusNotFound)

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)

	ownerSettlement := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement", http.StatusOK)
	getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/settlement", http.StatusForbidden)

	settlement := mustObject(t, ownerSettlement.JSON)
	if settlement["eventId"] != eventID || settlement["currency"] != "usd" || int(settlement["grossPaidRevenueCents"].(float64)) != 1500 || int(settlement["paidTicketCount"].(float64)) != 1 || int(settlement["pendingTicketCount"].(float64)) != 0 || int(settlement["cancelledTicketCount"].(float64)) != 1 || int(settlement["freeTicketCount"].(float64)) != 1 || int(settlement["reservedCount"].(float64)) != 2 || int(settlement["adjustmentTotalCents"].(float64)) != 0 || int(settlement["netTotalCents"].(float64)) != 1500 || settlement["status"] != "open" {
		t.Fatalf("unexpected settlement payload: %#v", settlement)
	}
	if generatedAt, ok := settlement["generatedAt"].(string); !ok || generatedAt == "" {
		t.Fatalf("expected generatedAt timestamp, got %#v", settlement["generatedAt"])
	}
	if adjustments, ok := settlement["adjustments"].([]any); !ok || len(adjustments) != 0 {
		t.Fatalf("expected empty adjustments, got %#v", settlement["adjustments"])
	}

	getJSON(t, fx.app, nil, "/api/events/"+eventID+"/settlement", http.StatusUnauthorized)
	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, otherFx.memberCookie, "/api/events/"+eventID+"/settlement", http.StatusForbidden)
	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/settlement/adjustments", map[string]any{"amountCents": 100, "label": "Member"}, http.StatusForbidden)
	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/settlement/finalize", map[string]any{}, http.StatusForbidden)

	firstFinalize := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement/finalize", map[string]any{}, http.StatusOK)
	firstFinalized := mustObject(t, firstFinalize.JSON)
	if firstFinalized["status"] != "finalized" {
		t.Fatalf("expected finalized settlement status, got %#v", firstFinalize.JSON)
	}
	if finalizedAt, ok := firstFinalized["finalizedAt"].(string); !ok || finalizedAt == "" {
		t.Fatalf("expected finalizedAt timestamp, got %#v", firstFinalized["finalizedAt"])
	}
	if firstFinalized["finalizedByPersonId"] != ownerPersonID(t, fx) {
		t.Fatalf("expected finalizedByPersonId to match owner, got %#v", firstFinalized["finalizedByPersonId"])
	}

	secondFinalize := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement/finalize", map[string]any{}, http.StatusOK)
	if !reflect.DeepEqual(firstFinalize.JSON, secondFinalize.JSON) {
		t.Fatalf("expected finalize to be idempotent: first=%#v second=%#v", firstFinalize.JSON, secondFinalize.JSON)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement/adjustments", map[string]any{"amountCents": 100, "label": "Late adjustment"}, http.StatusConflict)
}

func TestWorkspaceContactsListAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	workspaceID := fx.workspaceID

	empty := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+workspaceID+"/contacts", http.StatusOK)
	if got := empty.JSON.([]any); len(got) != 0 {
		t.Fatalf("expected empty contacts, got %#v", got)
	}

	var contactID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into contacts (workspace_id, display_name, email, phone, notes, tags, created_by_person_id)
		values ($1, 'Mira Door', 'mira@example.test', '+15555550123', 'Prefers late load-in', array['door','trusted'], $2)
		returning id
	`, workspaceID, ownerPersonID(t, fx)).Scan(&contactID); err != nil {
		t.Fatal(err)
	}

	resp := getJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+workspaceID+"/contacts", http.StatusOK)
	contacts := resp.JSON.([]any)
	if len(contacts) != 1 {
		t.Fatalf("expected one contact, got %#v", contacts)
	}
	contact := mustObject(t, contacts[0])
	if contact["id"] != contactID || contact["displayName"] != "Mira Door" || contact["email"] != "mira@example.test" || contact["notes"] != "Prefers late load-in" {
		t.Fatalf("unexpected contact: %#v", contact)
	}
	tags := contact["tags"].([]any)
	if len(tags) != 2 || tags[0] != "door" || tags[1] != "trusted" {
		t.Fatalf("unexpected tags: %#v", tags)
	}

	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, nil, "/api/workspaces/"+workspaceID+"/contacts", http.StatusUnauthorized)
	getJSON(t, fx.app, otherFx.ownerCookie, "/api/workspaces/"+workspaceID+"/contacts", http.StatusForbidden)
}

func TestWorkspaceContactsMutationAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	workspaceID := fx.workspaceID

	created := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+workspaceID+"/contacts", map[string]any{
		"displayName": "  Mira Door  ",
		"email":       " MIRA@EXAMPLE.TEST ",
		"phone":       "  +15555550123 ",
		"notes":       "  Do not publish this note.  ",
		"tags":        []string{" door ", "trusted", "door", ""},
	}, http.StatusOK)
	contact := mustObject(t, created.JSON)
	if contact["displayName"] != "Mira Door" || contact["email"] != "mira@example.test" || contact["phone"] != "+15555550123" || contact["notes"] != "Do not publish this note." {
		t.Fatalf("unexpected normalized contact: %#v", contact)
	}
	tags := contact["tags"].([]any)
	if len(tags) != 2 || tags[0] != "door" || tags[1] != "trusted" {
		t.Fatalf("unexpected normalized tags: %#v", contact["tags"])
	}
	if contact["createdAt"] == "" || contact["updatedAt"] == "" {
		t.Fatalf("expected timestamps in created contact: %#v", contact)
	}
	contactID := mustString(t, created.JSON, "id")

	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+workspaceID+"/contacts", map[string]any{
		"displayName": "Duplicate Mira",
		"email":       "mira@example.test",
	}, http.StatusConflict)

	updated := patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+workspaceID+"/contacts/"+contactID, map[string]any{
		"displayName": "Mira Lead",
		"clearEmail":  true,
		"clearPhone":  true,
		"notes":       "Still private",
		"tags":        []string{"lead"},
	}, http.StatusOK)
	updatedContact := mustObject(t, updated.JSON)
	if updatedContact["displayName"] != "Mira Lead" || updatedContact["email"] != nil || updatedContact["phone"] != nil || updatedContact["notes"] != "Still private" {
		t.Fatalf("unexpected updated contact: %#v", updatedContact)
	}
	updatedTags := updatedContact["tags"].([]any)
	if len(updatedTags) != 1 || updatedTags[0] != "lead" {
		t.Fatalf("unexpected updated tags: %#v", updatedContact["tags"])
	}

	postJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+workspaceID+"/contacts", map[string]any{"displayName": "Member"}, http.StatusForbidden)
	patchJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+workspaceID+"/contacts/"+contactID, map[string]any{"displayName": "Member"}, http.StatusForbidden)
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+workspaceID+"/contacts", map[string]any{"displayName": ""}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+workspaceID+"/contacts/"+contactID, map[string]any{"clearEmail": true, "email": "bad"}, http.StatusBadRequest)

	createdAudit := auditMetadataForAction(t, fx.app.db, "contact.created")
	updatedAudit := auditMetadataForAction(t, fx.app.db, "contact.updated")
	for _, audit := range []string{createdAudit, updatedAudit} {
		if strings.Contains(audit, "Do not publish this note") || strings.Contains(audit, "Still private") || strings.Contains(audit, "mira@example.test") || strings.Contains(audit, "+15555550123") || strings.Contains(audit, "Mira") || strings.Contains(audit, "trusted") {
			t.Fatalf("audit metadata leaked contact details: %s", audit)
		}
	}

	for _, action := range []string{"contact.created", "contact.updated"} {
		var metadataText string
		if err := fx.app.db.QueryRow(t.Context(), `
			select metadata::text
			from audit_entries
			where action = $1 and subject_id = $2
			order by created_at desc
			limit 1
		`, action, contactID).Scan(&metadataText); err != nil {
			t.Fatal(err)
		}
		var metadata map[string]any
		if err := json.Unmarshal([]byte(metadataText), &metadata); err != nil {
			t.Fatal(err)
		}
		if len(metadata) != 2 || metadata["contactId"] != contactID || metadata["workspaceId"] != workspaceID {
			t.Fatalf("unexpected %s audit metadata: %#v", action, metadata)
		}
	}
}

func TestEventTemplatesListAPI(t *testing.T) {
	fx := newLifecycleFixture(t)

	empty := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", http.StatusOK)
	if got := empty.JSON.([]any); len(got) != 0 {
		t.Fatalf("expected empty templates, got %#v", got)
	}

	var templateID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_templates (
			workspace_id, name, title, public_description, location_display,
			ticket_allocation, pricing_mode, ticket_price_cents, ticket_currency,
			private_notes, created_by_person_id
		) values ($1, 'Monthly Market', 'Night Market', 'Public copy', 'The Hall', 40, 'fixed', 1500, 'usd', 'Private run-of-show', $2)
		returning id
	`, fx.workspaceID, ownerPersonID(t, fx)).Scan(&templateID); err != nil {
		t.Fatal(err)
	}

	ownerResp := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", http.StatusOK)
	memberResp := getJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", http.StatusOK)
	for _, resp := range []any{ownerResp.JSON, memberResp.JSON} {
		templates := resp.([]any)
		if len(templates) != 1 {
			t.Fatalf("expected one template, got %#v", templates)
		}
		tmpl := mustObject(t, templates[0])
		if tmpl["id"] != templateID || tmpl["name"] != "Monthly Market" || tmpl["title"] != "Night Market" || tmpl["privateNotes"] != "Private run-of-show" {
			t.Fatalf("unexpected template: %#v", tmpl)
		}
	}

	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, nil, "/api/workspaces/"+fx.workspaceID+"/event-templates", http.StatusUnauthorized)
	getJSON(t, fx.app, otherFx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", http.StatusForbidden)
}

func TestEventTemplatesMutationAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	created := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", map[string]any{
		"name":              "  Monthly Market  ",
		"title":             "  Night Market  ",
		"publicDescription": "  Public copy  ",
		"locationDisplay":   "  The Hall  ",
		"ticketAllocation":  40,
		"pricingMode":       "fixed",
		"ticketPriceCents":  1500,
		"ticketCurrency":    " USD ",
		"privateNotes":      "Private setup note",
	}, http.StatusOK)
	createdTemplate := mustObject(t, created.JSON)
	if createdTemplate["name"] != "Monthly Market" || createdTemplate["title"] != "Night Market" || createdTemplate["publicDescription"] != "Public copy" || createdTemplate["locationDisplay"] != "The Hall" || int(createdTemplate["ticketAllocation"].(float64)) != 40 || createdTemplate["pricingMode"] != "fixed" || int(createdTemplate["ticketPriceCents"].(float64)) != 1500 || createdTemplate["ticketCurrency"] != "usd" || createdTemplate["privateNotes"] != "Private setup note" {
		t.Fatalf("unexpected created template: %#v", createdTemplate)
	}
	templateID := createdTemplate["id"].(string)

	updated := patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates/"+templateID, map[string]any{
		"name":             "  Market v2  ",
		"pricingMode":      "free",
		"ticketPriceCents": 0,
		"privateNotes":     "  Still private  ",
	}, http.StatusOK)
	updatedTemplate := mustObject(t, updated.JSON)
	if updatedTemplate["name"] != "Market v2" || updatedTemplate["pricingMode"] != "free" || int(updatedTemplate["ticketPriceCents"].(float64)) != 0 || updatedTemplate["ticketCurrency"] != "usd" || updatedTemplate["privateNotes"] != "Still private" {
		t.Fatalf("unexpected updated template: %#v", updatedTemplate)
	}

	postJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", map[string]any{"name": "member", "title": "member"}, http.StatusForbidden)
	patchJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates/"+templateID, map[string]any{"name": "member"}, http.StatusForbidden)
	doJSON(t, http.MethodDelete, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates/"+templateID, nil, http.StatusForbidden)

	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", map[string]any{"name": "", "title": "x"}, http.StatusBadRequest)
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", map[string]any{"name": "bad", "title": "bad", "pricingMode": "fixed", "ticketPriceCents": 49}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates/"+templateID, map[string]any{}, http.StatusBadRequest)

	rawAudit := auditMetadataForAction(t, fx.app.db, "event_template.created") + auditMetadataForAction(t, fx.app.db, "event_template.updated")
	if strings.Contains(rawAudit, "Private setup note") || strings.Contains(rawAudit, "Still private") || strings.Contains(rawAudit, "Night Market") || strings.Contains(rawAudit, "Public copy") || strings.Contains(rawAudit, "The Hall") {
		t.Fatalf("audit metadata leaked template content: %s", rawAudit)
	}

	for _, action := range []string{"event_template.created", "event_template.updated"} {
		var metadataText string
		if err := fx.app.db.QueryRow(t.Context(), `
			select metadata::text
			from audit_entries
			where action = $1 and subject_id = $2
			order by created_at desc
			limit 1
		`, action, templateID).Scan(&metadataText); err != nil {
			t.Fatal(err)
		}
		var metadata map[string]any
		if err := json.Unmarshal([]byte(metadataText), &metadata); err != nil {
			t.Fatal(err)
		}
		if len(metadata) != 2 || metadata["templateId"] != templateID || metadata["workspaceId"] != fx.workspaceID {
			t.Fatalf("unexpected %s audit metadata: %#v", action, metadata)
		}
	}

	deleted := doJSON(t, http.MethodDelete, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates/"+templateID, nil, http.StatusNoContent)
	if deleted.Body != "" {
		t.Fatalf("expected empty delete response, got %q", deleted.Body)
	}
	getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", http.StatusOK)

	rawAudit = auditMetadataForAction(t, fx.app.db, "event_template.deleted")
	if strings.Contains(rawAudit, "Private setup note") || strings.Contains(rawAudit, "Still private") || strings.Contains(rawAudit, "Night Market") || strings.Contains(rawAudit, "Public copy") || strings.Contains(rawAudit, "The Hall") {
		t.Fatalf("audit metadata leaked template content: %s", rawAudit)
	}
	var deleteMetadataText string
	if err := fx.app.db.QueryRow(t.Context(), `
		select metadata::text
		from audit_entries
		where action = $1 and subject_id = $2
		order by created_at desc
		limit 1
	`, "event_template.deleted", templateID).Scan(&deleteMetadataText); err != nil {
		t.Fatal(err)
	}
	var deleteMetadata map[string]any
	if err := json.Unmarshal([]byte(deleteMetadataText), &deleteMetadata); err != nil {
		t.Fatal(err)
	}
	if len(deleteMetadata) != 2 || deleteMetadata["templateId"] != templateID || deleteMetadata["workspaceId"] != fx.workspaceID {
		t.Fatalf("unexpected delete audit metadata: %#v", deleteMetadata)
	}
}

func TestEventTemplateApplyToDraftAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	tmpl := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", map[string]any{
		"name":              "Monthly Market",
		"title":             "Template Title",
		"publicDescription": "Template public copy",
		"locationDisplay":   "Template Hall",
		"ticketAllocation":  55,
		"pricingMode":       "fixed",
		"ticketPriceCents":  1500,
		"ticketCurrency":    "usd",
		"privateNotes":      "Do not copy publicly",
	}, http.StatusOK)
	templateID := mustString(t, tmpl.JSON, "id")

	draft := createEvent(t, fx, "Old Title", 5)
	eventID := mustString(t, draft, "id")
	applied := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/apply-template", map[string]any{"templateId": templateID}, http.StatusOK)
	event := mustObject(t, applied.JSON)
	if event["title"] != "Template Title" || event["publicDescription"] != "Template public copy" || event["locationDisplay"] != "Template Hall" || int(event["ticketAllocation"].(float64)) != 55 || event["pricingMode"] != "fixed" || int(event["ticketPriceCents"].(float64)) != 1500 {
		t.Fatalf("template was not applied to draft: %#v", event)
	}
	if raw, _ := json.Marshal(event); strings.Contains(string(raw), "Do not copy publicly") {
		t.Fatalf("private template notes leaked into event DTO: %s", raw)
	}

	reapplied := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/apply-template", map[string]any{"templateId": templateID}, http.StatusOK)
	if mustObject(t, reapplied.JSON)["title"] != "Template Title" {
		t.Fatalf("repeat apply changed event unexpectedly: %#v", reapplied.JSON)
	}

	var applyMetadataText string
	if err := fx.app.db.QueryRow(t.Context(), `
		select metadata::text
		from audit_entries
		where action = $1 and subject_id = $2
		order by created_at desc
		limit 1
	`, "event_template.applied", eventID).Scan(&applyMetadataText); err != nil {
		t.Fatal(err)
	}
	var applyMetadata map[string]any
	if err := json.Unmarshal([]byte(applyMetadataText), &applyMetadata); err != nil {
		t.Fatal(err)
	}
	if len(applyMetadata) != 3 || applyMetadata["templateId"] != templateID || applyMetadata["eventId"] != eventID || applyMetadata["workspaceId"] != fx.workspaceID {
		t.Fatalf("unexpected apply audit metadata: %#v", applyMetadata)
	}
	if strings.Contains(applyMetadataText, "Do not copy publicly") || strings.Contains(applyMetadataText, "Template public copy") {
		t.Fatalf("apply audit leaked private content: %s", applyMetadataText)
	}

	memberDraft := createEvent(t, fx, "Member Draft", 5)
	memberEventID := mustString(t, memberDraft, "id")
	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+memberEventID+"/apply-template", map[string]any{"templateId": templateID}, http.StatusForbidden)

	otherFx := newLifecycleFixture(t, fx.app)
	otherTemplate := postJSON(t, otherFx.app, otherFx.ownerCookie, "/api/workspaces/"+otherFx.workspaceID+"/event-templates", map[string]any{"name": "Other", "title": "Other"}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+memberEventID+"/apply-template", map[string]any{"templateId": mustString(t, otherTemplate.JSON, "id")}, http.StatusNotFound)

	published := publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+mustString(t, published, "id")+"/apply-template", map[string]any{"templateId": templateID}, http.StatusConflict)

	reservedDraft := createEvent(t, fx, "Reserved Draft", 5)
	reservedEventID := mustString(t, reservedDraft, "id")
	insertTicketWithPaymentStatus(t, fx, reservedEventID, fx.email("ticket"), "Reserved Buyer", "pending", 1500, "usd")
	pricingConflict := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+reservedEventID+"/apply-template", map[string]any{"templateId": mustString(t, postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", map[string]any{"name": "Fixed Pricing", "title": "Fixed Pricing", "pricingMode": "fixed", "ticketPriceCents": 2000, "ticketCurrency": "usd", "ticketAllocation": 10}, http.StatusOK).JSON, "id")}, http.StatusConflict)
	if pricingConflict.Status != http.StatusConflict {
		t.Fatalf("expected pricing conflict, got %d", pricingConflict.Status)
	}
	allocationConflict := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+reservedEventID+"/apply-template", map[string]any{"templateId": mustString(t, postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", map[string]any{"name": "Low Allocation", "title": "Low Allocation", "pricingMode": "free", "ticketPriceCents": 0, "ticketAllocation": 0}, http.StatusOK).JSON, "id")}, http.StatusConflict)
	if allocationConflict.Status != http.StatusConflict {
		t.Fatalf("expected allocation conflict, got %d", allocationConflict.Status)
	}
}

func TestCommitmentsAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Benefit Show", 20)
	eventID := mustString(t, event, "id")
	dueAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)

	created := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{
		"eventId":       eventID,
		"title":         "Confirm projector",
		"description":   "Private vendor detail",
		"dueAt":         dueAt,
		"ownerPersonId": ownerPersonID(t, fx),
	}, http.StatusOK)
	commitment := mustObject(t, created.JSON)
	if commitment["title"] != "Confirm projector" || commitment["status"] != "open" || commitment["eventId"] != eventID || commitment["description"] != "Private vendor detail" {
		t.Fatalf("unexpected commitment: %#v", created.JSON)
	}
	if commitment["completedAt"] != nil || commitment["completedByPersonId"] != nil {
		t.Fatalf("expected open commitment to be incomplete: %#v", commitment)
	}
	commitmentID := commitment["id"].(string)

	workspaceList := getJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", http.StatusOK)
	if got := workspaceList.JSON.([]any); len(got) != 1 {
		t.Fatalf("expected one workspace commitment, got %#v", got)
	}
	eventList := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/commitments", http.StatusOK)
	if got := eventList.JSON.([]any); len(got) != 1 {
		t.Fatalf("expected one event commitment, got %#v", got)
	}

	firstDone := patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"status": "done"}, http.StatusOK)
	doneCommitment := mustObject(t, firstDone.JSON)
	if doneCommitment["status"] != "done" || doneCommitment["completedAt"] == nil || doneCommitment["completedByPersonId"] != ownerPersonID(t, fx) {
		t.Fatalf("expected done commitment: %#v", doneCommitment)
	}
	firstCompletedAt := doneCommitment["completedAt"]
	secondDone := patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"status": "done"}, http.StatusOK)
	if mustObject(t, secondDone.JSON)["completedAt"] != firstCompletedAt {
		t.Fatalf("expected done to be idempotent: %#v", secondDone.JSON)
	}
	reopened := patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"status": "open"}, http.StatusOK)
	reopenedCommitment := mustObject(t, reopened.JSON)
	if reopenedCommitment["status"] != "open" || reopenedCommitment["completedAt"] != nil || reopenedCommitment["completedByPersonId"] != nil {
		t.Fatalf("expected reopen to clear completion fields: %#v", reopenedCommitment)
	}
	redone := patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"status": "done"}, http.StatusOK)
	if mustObject(t, redone.JSON)["completedAt"] == nil {
		t.Fatalf("expected commitment to complete again: %#v", redone.JSON)
	}
	cancelled := patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"status": "cancelled"}, http.StatusOK)
	cancelledCommitment := mustObject(t, cancelled.JSON)
	if cancelledCommitment["status"] != "cancelled" || cancelledCommitment["completedAt"] != nil || cancelledCommitment["completedByPersonId"] != nil {
		t.Fatalf("expected cancel to clear completion fields: %#v", cancelledCommitment)
	}

	postJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{"title": "member"}, http.StatusForbidden)
	getJSON(t, fx.app, nil, "/api/workspaces/"+fx.workspaceID+"/commitments", http.StatusUnauthorized)
	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, otherFx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", http.StatusForbidden)

	otherEvent := createEvent(t, otherFx, "Other workspace event", 5)
	otherContact := postJSON(t, otherFx.app, otherFx.ownerCookie, "/api/workspaces/"+otherFx.workspaceID+"/contacts", map[string]any{"displayName": "Other Contact"}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{"title": "bad event", "eventId": mustString(t, otherEvent, "id")}, http.StatusBadRequest)
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{"title": "bad contact", "contactId": mustString(t, otherContact.JSON, "id")}, http.StatusBadRequest)
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{"title": "bad owner", "ownerPersonId": ownerPersonID(t, otherFx)}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"eventId": mustString(t, otherEvent, "id")}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"contactId": mustString(t, otherContact.JSON, "id")}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"ownerPersonId": ownerPersonID(t, otherFx)}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"clearDueAt": true, "dueAt": dueAt}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"clearEvent": true, "eventId": eventID}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"clearContact": true, "contactId": mustString(t, otherContact.JSON, "id")}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+commitmentID, map[string]any{"clearOwner": true, "ownerPersonId": ownerPersonID(t, fx)}, http.StatusBadRequest)

	createdAudit := auditMetadataForAction(t, fx.app.db, "commitment.created")
	updatedAudit := auditMetadataForAction(t, fx.app.db, "commitment.updated")
	for _, audit := range []string{createdAudit, updatedAudit} {
		if strings.Contains(audit, "Private vendor detail") || strings.Contains(audit, "Confirm projector") || strings.Contains(audit, "Benefit Show") {
			t.Fatalf("audit metadata leaked commitment details: %s", audit)
		}
	}

	for _, action := range []string{"commitment.created", "commitment.updated"} {
		var metadataText string
		if err := fx.app.db.QueryRow(t.Context(), `
			select metadata::text
			from audit_entries
			where action = $1 and subject_id = $2
			order by created_at desc
			limit 1
		`, action, commitmentID).Scan(&metadataText); err != nil {
			t.Fatal(err)
		}
		var metadata map[string]any
		if err := json.Unmarshal([]byte(metadataText), &metadata); err != nil {
			t.Fatal(err)
		}
		if len(metadata) != 3 || metadata["commitmentId"] != commitmentID || metadata["workspaceId"] != fx.workspaceID || metadata["status"] == "" {
			t.Fatalf("unexpected %s audit metadata: %#v", action, metadata)
		}
	}
}

func TestFirstEventLifecycleArchiveAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")

	getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusNotFound)

	publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)

	ownerArchive := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	memberArchive := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	if !reflect.DeepEqual(ownerArchive.JSON, memberArchive.JSON) {
		t.Fatalf("expected owner/member archive responses to match: owner=%#v member=%#v", ownerArchive.JSON, memberArchive.JSON)
	}

	archive := mustObject(t, ownerArchive.JSON)
	if archive["eventId"] != eventID || archive["status"] != "private" || int(archive["noteCount"].(float64)) != 0 {
		t.Fatalf("unexpected archive response: %#v", archive)
	}
	if archive["reportId"] == "" || archive["settlementId"] == "" || archive["createdAt"] == "" || archive["updatedAt"] == "" {
		t.Fatalf("archive response missing references/timestamps: %#v", archive)
	}
	notes, ok := archive["notes"].([]any)
	if !ok || len(notes) != 0 {
		t.Fatalf("expected empty notes array, got %#v", archive["notes"])
	}

	getJSON(t, fx.app, nil, "/api/events/"+eventID+"/archive", http.StatusUnauthorized)
	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, otherFx.memberCookie, "/api/events/"+eventID+"/archive", http.StatusForbidden)
}

func TestArchiveCapturesParticipantMemory(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")

	performer := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a 20-minute set.", "capacity": 3, "public": true}, http.StatusOK)
	roleID := mustString(t, performer.JSON, "id")

	accepted := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "  Alex  ", "applicantEmail": "  ALEX@example.com ", "message": "  Bring a keyboard.  "}, http.StatusOK)
	confirmed := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Blair", "applicantEmail": "blair@example.com", "message": "Backup vocals."}, http.StatusOK)
	ignored := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Casey", "applicantEmail": "casey@example.com", "message": "Still interested."}, http.StatusOK)

	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'accepted', updated_at = now()
		where id = $1
	`, mustString(t, accepted.JSON, "id")); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'confirmed', updated_at = now()
		where id = $1
	`, mustString(t, confirmed.JSON, "id")); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'rejected', updated_at = now()
		where id = $1
	`, mustString(t, ignored.JSON, "id")); err != nil {
		t.Fatal(err)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	firstArchive := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	first := mustObject(t, firstArchive.JSON)
	participants, ok := first["participants"].([]any)
	if !ok || len(participants) != 2 {
		t.Fatalf("expected two archived participants, got %#v", firstArchive.JSON)
	}
	firstParticipant := mustObject(t, participants[0])
	secondParticipant := mustObject(t, participants[1])
	if firstParticipant["roleName"] != "Performer" || firstParticipant["participantName"] != "Alex" || firstParticipant["status"] != "accepted" {
		t.Fatalf("unexpected first archived participant: %#v", firstParticipant)
	}
	if secondParticipant["roleName"] != "Performer" || secondParticipant["participantName"] != "Blair" || secondParticipant["status"] != "confirmed" {
		t.Fatalf("unexpected second archived participant: %#v", secondParticipant)
	}
	for _, participant := range participants {
		entry := mustObject(t, participant)
		for _, forbidden := range []string{"applicantEmail", "message"} {
			if _, ok := entry[forbidden]; ok {
				t.Fatalf("archive participant memory must not expose %s: %#v", forbidden, entry)
			}
		}
	}

	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_archive_participants where archive_id = $1`, first["id"]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected two archive participant rows, got %d", count)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	secondArchive := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	if !reflect.DeepEqual(firstArchive.JSON, secondArchive.JSON) {
		t.Fatalf("expected archive retry to be idempotent: first=%#v second=%#v", firstArchive.JSON, secondArchive.JSON)
	}
}

func TestArchiveSnapshotStaysImmutableAfterCreation(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")

	performer := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a 20-minute set.", "capacity": 3, "public": true}, http.StatusOK)
	roleID := mustString(t, performer.JSON, "id")

	first := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": "Bring a keyboard."}, http.StatusOK)
	second := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Blair", "applicantEmail": "blair@example.com", "message": "Backup vocals."}, http.StatusOK)

	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'accepted', updated_at = now()
		where id = $1
	`, mustString(t, first.JSON, "id")); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'confirmed', updated_at = now()
		where id = $1
	`, mustString(t, second.JSON, "id")); err != nil {
		t.Fatal(err)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	archiveBefore := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)

	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Casey", "applicantEmail": "casey@example.com"}, http.StatusNotFound)
	// Directly perturb private source data to prove the archive is a snapshot,
	// without incorrectly admitting a public application to a closed event.
	insertRoleApplicationFixture(t, fx, eventID, roleID, "Casey", "casey@example.com", "accepted")

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	archiveAfter := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)

	if !reflect.DeepEqual(archiveBefore.JSON, archiveAfter.JSON) {
		t.Fatalf("expected archive snapshot to remain immutable after retry: before=%#v after=%#v", archiveBefore.JSON, archiveAfter.JSON)
	}
}

func TestNotificationLedgerAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	ownerID := ownerPersonID(t, fx)
	roleApplicationMessage := "Role application message should stay private"
	staffingNotes := "Staffing notes should stay private"
	archiveNoteBody := "Archive note body should stay private"
	settlementInternal := "Settlement internals should stay private"

	var outboxID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into email_outbox (recipient_email, subject, body, related_type, related_id)
		values ($1, $2, $3, $4, null)
		returning id
	`, "notify@example.test", "Notification subject", "Sensitive message body", "event_notification").Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into notification_events (
			workspace_id, event_id, recipient_email, notification_type, related_type,
			related_id, idempotency_key, email_outbox_id, subject, preview, created_by_person_id
		)
		values ($1, $2, $3, $4, $5, null, $6, $7, $8, $9, $10)
	`, fx.workspaceID, eventID, "notify@example.test", "event.update", "event_notification", "notification-ledger:test", outboxID, "Notification subject", "Sensitive preview", ownerID); err != nil {
		t.Fatal(err)
	}

	ownerResp := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/notifications", http.StatusOK)
	memberResp := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/notifications", http.StatusOK)
	if !reflect.DeepEqual(ownerResp.JSON, memberResp.JSON) {
		t.Fatalf("expected owner/member notifications to match: owner=%#v member=%#v", ownerResp.JSON, memberResp.JSON)
	}

	items := ownerResp.JSON.([]any)
	if len(items) != 1 {
		t.Fatalf("expected one notification event, got %#v", ownerResp.JSON)
	}
	item := mustObject(t, items[0])
	if item["eventId"] != eventID || item["recipientEmail"] != "notify@example.test" || item["notificationType"] != "event.update" || item["relatedType"] != "event_notification" || item["subject"] != "Notification subject" || item["preview"] != "Sensitive preview" || item["status"] != "queued" {
		t.Fatalf("unexpected notification payload: %#v", item)
	}
	if _, ok := item["relatedId"]; ok {
		t.Fatalf("expected relatedId to be omitted when null: %#v", item)
	}
	for _, forbidden := range []string{"message", "notes", "body"} {
		if _, ok := item[forbidden]; ok {
			t.Fatalf("notification ledger leaked %s: %#v", forbidden, item)
		}
	}
	for _, forbidden := range []string{roleApplicationMessage, staffingNotes, archiveNoteBody, settlementInternal, "settlementSummary", "finalizedByPersonId", "adjustments"} {
		if strings.Contains(ownerResp.Body, forbidden) {
			t.Fatalf("notification api leaked %q: %s", forbidden, ownerResp.Body)
		}
	}

	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, otherFx.memberCookie, "/api/events/"+eventID+"/notifications", http.StatusForbidden)
	getJSON(t, fx.app, nil, "/api/events/"+eventID+"/notifications", http.StatusUnauthorized)
}

func TestReminderLedgerAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Benefit Show", 20)
	eventID := mustString(t, event, "id")
	commitment := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{"title": "Seed reminder source"}, http.StatusOK)
	commitmentID := mustString(t, commitment.JSON, "id")

	var reminderID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into reminder_events (
			workspace_id, event_id, source_type, source_id, reminder_type,
			recipient_email, due_at, idempotency_key, status, subject, preview, created_by_person_id
		) values ($1, $2, 'commitment', $3, 'commitment.due', 'owner@example.test', now(), 'test-reminder-key', 'queued', 'Reminder subject', 'Reminder preview', $4)
		returning id
	`, fx.workspaceID, eventID, commitmentID, ownerPersonID(t, fx)).Scan(&reminderID); err != nil {
		t.Fatal(err)
	}

	workspaceResp := getJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/reminders", http.StatusOK)
	reminders := workspaceResp.JSON.([]any)
	if len(reminders) != 1 {
		t.Fatalf("expected one reminder, got %#v", reminders)
	}
	reminder := mustObject(t, reminders[0])
	if reminder["id"] != reminderID || reminder["workspaceId"] != fx.workspaceID || reminder["sourceType"] != "commitment" || reminder["sourceId"] != commitmentID || reminder["reminderType"] != "commitment.due" || reminder["recipientEmail"] != "owner@example.test" || reminder["subject"] != "Reminder subject" || reminder["preview"] != "Reminder preview" || reminder["status"] != "queued" {
		t.Fatalf("unexpected reminder: %#v", reminder)
	}
	if reminder["eventId"] != eventID {
		t.Fatalf("expected eventId to match, got %#v", reminder["eventId"])
	}
	if reminder["notificationEventId"] != nil {
		t.Fatalf("expected notificationEventId to be null, got %#v", reminder["notificationEventId"])
	}

	eventResp := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/reminders", http.StatusOK)
	if got := eventResp.JSON.([]any); len(got) != 1 {
		t.Fatalf("expected one event reminder, got %#v", got)
	}

	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, nil, "/api/workspaces/"+fx.workspaceID+"/reminders", http.StatusUnauthorized)
	getJSON(t, fx.app, otherFx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders", http.StatusForbidden)
	getJSON(t, fx.app, otherFx.memberCookie, "/api/events/"+eventID+"/reminders", http.StatusForbidden)
}

func TestReminderSweepAPIEmptyAndPermissions(t *testing.T) {
	fx := newLifecycleFixture(t)
	resp := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders/sweep", map[string]any{"now": time.Now().UTC().Format(time.RFC3339)}, http.StatusOK)
	result := mustObject(t, resp.JSON)
	if int(result["createdCount"].(float64)) != 0 || int(result["skippedCount"].(float64)) != 0 {
		t.Fatalf("unexpected empty sweep result: %#v", result)
	}

	postJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/reminders/sweep", map[string]any{}, http.StatusForbidden)
	otherFx := newLifecycleFixture(t, fx.app)
	postJSON(t, fx.app, otherFx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders/sweep", map[string]any{}, http.StatusForbidden)
}

func TestReminderSweepCommitments(t *testing.T) {
	fx := newLifecycleFixture(t)
	dueAt := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	var memberPersonID string
	if err := fx.app.db.QueryRow(t.Context(), `
		select person_id
		from workspace_members
		where workspace_id = $1
		  and role = 'member'
		  and removed_at is null
		limit 1
	`, fx.workspaceID).Scan(&memberPersonID); err != nil {
		t.Fatal(err)
	}
	memberEmail := fx.email("member")

	open := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{
		"title":         "Confirm projector",
		"description":   "Private projector vendor note",
		"dueAt":         dueAt,
		"ownerPersonId": memberPersonID,
	}, http.StatusOK)
	openID := mustString(t, open.JSON, "id")

	done := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{
		"title":       "Archive invoices",
		"description": "Private done note",
		"dueAt":       dueAt,
	}, http.StatusOK)
	doneID := mustString(t, done.JSON, "id")
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+doneID, map[string]any{"status": "done"}, http.StatusOK)

	cancelled := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{
		"title":       "Return deposit",
		"description": "Private cancelled note",
		"dueAt":       dueAt,
	}, http.StatusOK)
	cancelledID := mustString(t, cancelled.JSON, "id")
	patchJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments/"+cancelledID, map[string]any{"status": "cancelled"}, http.StatusOK)

	first := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders/sweep", map[string]any{"now": time.Now().UTC().Format(time.RFC3339)}, http.StatusOK)
	firstResult := mustObject(t, first.JSON)
	if int(firstResult["createdCount"].(float64)) != 1 || int(firstResult["skippedCount"].(float64)) != 0 {
		t.Fatalf("expected first sweep to create one reminder: %#v", first.JSON)
	}

	repeated := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders/sweep", map[string]any{"now": time.Now().UTC().Format(time.RFC3339)}, http.StatusOK)
	repeatedResult := mustObject(t, repeated.JSON)
	if int(repeatedResult["createdCount"].(float64)) != 0 || int(repeatedResult["skippedCount"].(float64)) != 1 {
		t.Fatalf("expected repeated sweep to skip duplicate reminder: %#v", repeated.JSON)
	}

	var reminderCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from reminder_events
		where workspace_id = $1
		  and source_type = 'commitment'
	`, fx.workspaceID).Scan(&reminderCount); err != nil {
		t.Fatal(err)
	}
	if reminderCount != 1 {
		t.Fatalf("expected one commitment reminder, got %d", reminderCount)
	}

	var excludedReminderCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from reminder_events
		where source_id in ($1, $2)
	`, doneID, cancelledID).Scan(&excludedReminderCount); err != nil {
		t.Fatal(err)
	}
	if excludedReminderCount != 0 {
		t.Fatalf("expected done/cancelled commitments to be excluded, got %d reminders", excludedReminderCount)
	}

	reminders := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders", http.StatusOK).JSON.([]any)
	if len(reminders) != 1 {
		t.Fatalf("expected one reminder, got %#v", reminders)
	}
	reminder := mustObject(t, reminders[0])
	if reminder["sourceType"] != "commitment" || reminder["sourceId"] != openID || reminder["reminderType"] != "commitment.due" || reminder["recipientEmail"] != memberEmail {
		t.Fatalf("unexpected reminder: %#v", reminder)
	}

	var notificationRecipient, subject, preview, body string
	if err := fx.app.db.QueryRow(t.Context(), `
		select n.recipient_email, n.subject, n.preview, o.body
		from notification_events n
		join email_outbox o on o.id = n.email_outbox_id
		where n.related_id = $1
		  and n.notification_type = 'commitment.due'
		order by n.created_at desc
		limit 1
	`, openID).Scan(&notificationRecipient, &subject, &preview, &body); err != nil {
		t.Fatal(err)
	}
	if notificationRecipient != memberEmail {
		t.Fatalf("expected reminder recipient %q, got %q", memberEmail, notificationRecipient)
	}
	for _, want := range []string{"Confirm projector", dueAt} {
		if !strings.Contains(subject, want) || !strings.Contains(preview, want) || !strings.Contains(body, want) {
			t.Fatalf("reminder copy missing %q: subject=%q preview=%q body=%q", want, subject, preview, body)
		}
	}
	for _, forbidden := range []string{"Private projector vendor note", "Private done note", "Private cancelled note"} {
		if strings.Contains(subject, forbidden) || strings.Contains(preview, forbidden) || strings.Contains(body, forbidden) {
			t.Fatalf("reminder leaked private text %q: subject=%q preview=%q body=%q", forbidden, subject, preview, body)
		}
	}
}

func TestReminderSweepStaffing(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Benefit Show", 20)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")
	startsAt := time.Now().UTC().Add(2 * time.Hour)
	startsAtText := startsAt.Format(time.RFC3339)
	ownerID := ownerPersonID(t, fx)
	memberID := mustString(t, func() map[string]any {
		var personID string
		if err := fx.app.db.QueryRow(t.Context(), `
			select person_id
			from workspace_members
			where workspace_id = $1
			  and role = 'member'
			  and removed_at is null
			limit 1
		`, fx.workspaceID).Scan(&personID); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"id": personID}
	}(), "id")

	role := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a set.", "capacity": 1, "public": true}, http.StatusOK)
	roleID := mustString(t, role.JSON, "id")
	accepted := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Alex Applicant", "applicantEmail": "alex@example.test", "message": "Private application message"}, http.StatusOK)
	acceptedAppID := mustString(t, accepted.JSON, "id")
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'accepted', updated_at = now()
		where id = $1
	`, acceptedAppID); err != nil {
		t.Fatal(err)
	}

	var openID, assignedPersonID, assignedApplicationID, completedID, cancelledID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, starts_at, status, created_by_person_id
		)
		values ($1, 'Open door shift', 'shift', 'Private staffing note', $2, 'open', $3)
		returning id
	`, eventID, startsAtText, ownerID).Scan(&openID); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, starts_at, assigned_person_id, status, created_by_person_id
		)
		values ($1, 'Assigned member shift', 'shift', 'Member staffing note', $2, $3, 'assigned', $4)
		returning id
	`, eventID, startsAtText, memberID, ownerID).Scan(&assignedPersonID); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, starts_at, assigned_application_id, status, created_by_person_id
		)
		values ($1, 'Assigned application shift', 'shift', 'Application staffing note', $2, $3, 'assigned', $4)
		returning id
	`, eventID, startsAtText, acceptedAppID, ownerID).Scan(&assignedApplicationID); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, starts_at, status, created_by_person_id, completed_at, completed_by_person_id
		)
		values ($1, 'Completed task', 'task', 'Completed staffing note', $2, 'completed', $3, now(), $3)
		returning id
	`, eventID, startsAtText, ownerID).Scan(&completedID); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, starts_at, status, created_by_person_id
		)
		values ($1, 'Cancelled task', 'task', 'Cancelled staffing note', $2, 'cancelled', $3)
		returning id
	`, eventID, startsAtText, ownerID).Scan(&cancelledID); err != nil {
		t.Fatal(err)
	}

	draftEvent := createEvent(t, fx, "Draft Night", 10)
	draftEventID := mustString(t, draftEvent, "id")
	var draftStaffingID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, starts_at, status, created_by_person_id
		)
		values ($1, 'Draft task', 'task', 'Draft staffing note', $2, 'open', $3)
		returning id
	`, draftEventID, startsAtText, ownerID).Scan(&draftStaffingID); err != nil {
		t.Fatal(err)
	}

	closedEvent := createEvent(t, fx, "Closed Night", 10)
	closedEventID := mustString(t, closedEvent, "id")
	publishEvent(t, fx, closedEventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+closedEventID+"/end-of-night", map[string]any{}, http.StatusOK)
	var closedStaffingID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, starts_at, status, created_by_person_id
		)
		values ($1, 'Closed task', 'task', 'Closed staffing note', $2, 'open', $3)
		returning id
	`, closedEventID, startsAtText, ownerID).Scan(&closedStaffingID); err != nil {
		t.Fatal(err)
	}

	first := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders/sweep", map[string]any{"now": startsAt.Add(-30 * time.Minute).Format(time.RFC3339)}, http.StatusOK)
	firstResult := mustObject(t, first.JSON)
	if int(firstResult["createdCount"].(float64)) != 3 || int(firstResult["skippedCount"].(float64)) != 0 {
		t.Fatalf("expected first staffing sweep to create three reminders: %#v", first.JSON)
	}

	repeated := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders/sweep", map[string]any{"now": startsAt.Add(-30 * time.Minute).Format(time.RFC3339)}, http.StatusOK)
	repeatedResult := mustObject(t, repeated.JSON)
	if int(repeatedResult["createdCount"].(float64)) != 0 || int(repeatedResult["skippedCount"].(float64)) != 3 {
		t.Fatalf("expected repeated staffing sweep to skip duplicate reminders: %#v", repeated.JSON)
	}

	assertStaffingReminderCounts(t, fx, eventID, 3, 3, 3)

	reminders := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/reminders", http.StatusOK).JSON.([]any)
	if len(reminders) != 3 {
		t.Fatalf("expected three staffing reminders, got %#v", reminders)
	}
	remindersRaw, _ := json.Marshal(reminders)
	for _, forbidden := range []string{"Private staffing note", "Member staffing note", "Application staffing note", "Private application message", "Draft staffing note", "Closed staffing note", "Completed staffing note", "Cancelled staffing note"} {
		if strings.Contains(string(remindersRaw), forbidden) {
			t.Fatalf("staffing reminder api leaked %q: %s", forbidden, remindersRaw)
		}
	}

	assertStaffingReminderNotificationRecord(t, fx, openID, fx.email("owner"), "staffing.unassigned", "staffing_unassigned:"+openID+":"+fx.email("owner"), "Benefit Show", "Open door shift", startsAtText, "Private staffing note")
	assertStaffingReminderNotificationRecord(t, fx, assignedPersonID, fx.email("member"), "staffing.upcoming", "staffing_upcoming:"+assignedPersonID+":"+fx.email("member"), "Benefit Show", "Assigned member shift", startsAtText, "Member staffing note")
	assertStaffingReminderNotificationRecord(t, fx, assignedApplicationID, "alex@example.test", "staffing.upcoming", "staffing_upcoming:"+assignedApplicationID+":alex@example.test", "Benefit Show", "Assigned application shift", startsAtText, "Private application message")

	var excludedCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from reminder_events
		where source_id in ($1, $2, $3, $4)
	`, draftStaffingID, closedStaffingID, completedID, cancelledID).Scan(&excludedCount); err != nil {
		t.Fatal(err)
	}
	if excludedCount != 0 {
		t.Fatalf("expected only published staffing items to create reminders, got %d reminders for excluded sources", excludedCount)
	}
}

func TestPrivateMemoryBoundaries(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 40, "fixed", 1800, "usd")
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")
	ownerID := ownerPersonID(t, fx)

	contactNotes := "Prefers late load-in"
	commitmentDescription := "Private vendor detail"
	templatePrivateNotes := "Template private notes should stay workspace-only"
	reminderSubject := "Reminder subject should stay private"
	reminderPreview := "Reminder preview should stay private"
	reminderRecipient := "reminder@example.test"
	template := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", map[string]any{
		"name":         "Monthly Market",
		"title":        "Night Market",
		"privateNotes": templatePrivateNotes,
	}, http.StatusOK)
	if mustObject(t, template.JSON)["privateNotes"] != templatePrivateNotes {
		t.Fatalf("expected template route to expose private notes: %#v", template.JSON)
	}

	contact := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/contacts", map[string]any{
		"displayName": "Mira Door",
		"email":       "mira@example.test",
		"phone":       "+15555550123",
		"notes":       contactNotes,
		"tags":        []string{"door", "trusted"},
	}, http.StatusOK)
	contactID := mustString(t, contact.JSON, "id")

	commitment := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", map[string]any{
		"eventId":       eventID,
		"contactId":     contactID,
		"title":         "Confirm projector",
		"description":   commitmentDescription,
		"dueAt":         time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		"ownerPersonId": ownerID,
	}, http.StatusOK)

	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusConflict)
	// Payment fulfillment has its own provider-boundary tests. Seed a paid ticket
	// here so privacy checks do not rely on bypassing checkout through free RSVP.
	ticketID := insertTicketWithPaymentStatus(t, fx, eventID, fx.email("guest"), "Guest", "paid", 1800, "usd")
	var ticketCode string
	if err := fx.app.db.QueryRow(t.Context(), `select code from tickets where id = $1`, ticketID).Scan(&ticketCode); err != nil {
		t.Fatal(err)
	}
	openPublicEvent := getJSON(t, fx.app, nil, "/api/public/events/"+slug, http.StatusOK).Body

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)

	var outboxID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into email_outbox (recipient_email, subject, body, related_type, related_id)
		values ($1, $2, $3, $4, null)
		returning id
	`, "notify@example.test", "Notification subject", "Sensitive message body", "event_notification").Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into notification_events (
			workspace_id, event_id, recipient_email, notification_type, related_type,
			related_id, idempotency_key, email_outbox_id, subject, preview, created_by_person_id
		)
		values ($1, $2, $3, $4, $5, null, $6, $7, $8, $9, $10)
	`, fx.workspaceID, eventID, "notify@example.test", "event.update", "event_notification", "notification-ledger:test", outboxID, "Notification subject", "Sensitive preview", ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into reminder_events (
			workspace_id, event_id, source_type, source_id, reminder_type,
			recipient_email, due_at, idempotency_key, status, subject, preview, created_by_person_id
		)
		values ($1, $2, 'commitment', $3, 'commitment.due', $4, now(), 'reminder-ledger:test', 'queued', $5, $6, $7)
	`, fx.workspaceID, eventID, mustString(t, commitment.JSON, "id"), reminderRecipient, reminderSubject, reminderPreview, ownerID); err != nil {
		t.Fatal(err)
	}

	publicResponses := map[string]string{
		"public discovery": getJSON(t, fx.app, nil, "/api/public/events", http.StatusOK).Body,
		"public event":     openPublicEvent,
		"closed event":     getJSON(t, fx.app, nil, "/api/public/events/"+slug, http.StatusNotFound).Body,
		"ticket lookup":    getJSON(t, fx.app, nil, "/api/tickets/"+ticketCode, http.StatusOK).Body,
		"report":           getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/report", http.StatusOK).Body,
		"settlement":       getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement", http.StatusOK).Body,
		"archive summary":  getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK).Body,
		"notifications":    getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/notifications", http.StatusOK).Body,
	}
	for label, body := range publicResponses {
		for _, forbidden := range []string{contactID, "Mira Door", "mira@example.test", "+15555550123", contactNotes, "Confirm projector", commitmentDescription, templatePrivateNotes, reminderSubject, reminderPreview, reminderRecipient} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s leaked %q: %s", label, forbidden, body)
			}
		}
	}

	contactsBody := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/contacts", http.StatusOK).Body
	for _, want := range []string{contactID, "Mira Door", "mira@example.test", "+15555550123", contactNotes} {
		if !strings.Contains(contactsBody, want) {
			t.Fatalf("contacts route missing %q: %s", want, contactsBody)
		}
	}
	if strings.Contains(contactsBody, templatePrivateNotes) {
		t.Fatalf("contacts route leaked template private notes: %s", contactsBody)
	}
	for _, forbidden := range []string{reminderSubject, reminderPreview, reminderRecipient} {
		if strings.Contains(contactsBody, forbidden) {
			t.Fatalf("contacts route leaked reminder data %q: %s", forbidden, contactsBody)
		}
	}

	commitmentsBody := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/commitments", http.StatusOK).Body
	for _, want := range []string{"Confirm projector", commitmentDescription} {
		if !strings.Contains(commitmentsBody, want) {
			t.Fatalf("commitments route missing %q: %s", want, commitmentsBody)
		}
	}
	if strings.Contains(commitmentsBody, templatePrivateNotes) {
		t.Fatalf("commitments route leaked template private notes: %s", commitmentsBody)
	}
	for _, forbidden := range []string{reminderSubject, reminderPreview, reminderRecipient} {
		if strings.Contains(commitmentsBody, forbidden) {
			t.Fatalf("commitments route leaked reminder data %q: %s", forbidden, commitmentsBody)
		}
	}

	templateList := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/event-templates", http.StatusOK).Body
	if !strings.Contains(templateList, templatePrivateNotes) {
		t.Fatalf("template route should expose private notes: %s", templateList)
	}
	for _, forbidden := range []string{reminderSubject, reminderPreview, reminderRecipient} {
		if strings.Contains(templateList, forbidden) {
			t.Fatalf("template route leaked reminder data %q: %s", forbidden, templateList)
		}
	}

	workspaceReminders := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders", http.StatusOK).Body
	if !strings.Contains(workspaceReminders, reminderSubject) || !strings.Contains(workspaceReminders, reminderPreview) || !strings.Contains(workspaceReminders, reminderRecipient) {
		t.Fatalf("workspace reminders route missing reminder data: %s", workspaceReminders)
	}
	eventReminders := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/reminders", http.StatusOK).Body
	if !strings.Contains(eventReminders, reminderSubject) || !strings.Contains(eventReminders, reminderPreview) || !strings.Contains(eventReminders, reminderRecipient) {
		t.Fatalf("event reminders route missing reminder data: %s", eventReminders)
	}
}

func TestPublicEventDiscoveryAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	draft := createEvent(t, fx, "Draft Night", 20)
	published := createEvent(t, fx, "Published Market", 40)
	privateApplications := createEvent(t, fx, "Members Night", 30)
	closed := createEvent(t, fx, "Closed Night", 30)

	publishedID := mustString(t, published, "id")
	privateApplicationsID := mustString(t, privateApplications, "id")
	closedID := mustString(t, closed, "id")
	publishedAfterPublish := publishEvent(t, fx, publishedID)
	publishEvent(t, fx, privateApplicationsID)
	publishEvent(t, fx, closedID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+closedID+"/end-of-night", map[string]any{}, http.StatusOK)

	publishedSlug := mustString(t, publishedAfterPublish, "publicSlug")
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+publishedID+"/roles", map[string]any{"name": "Performer", "description": "Play a 20-minute set.", "capacity": 3, "public": true}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+privateApplicationsID+"/roles", map[string]any{"name": "Backstage", "description": "Private notes.", "capacity": 1, "public": false}, http.StatusOK)
	postJSON(t, fx.app, nil, "/api/public/events/"+publishedSlug+"/reservations", map[string]any{
		"displayName": "Ada",
		"email":       "ada@example.com",
	}, http.StatusOK)

	resp := getJSON(t, fx.app, nil, "/api/public/events", http.StatusOK)
	events := resp.JSON.([]any)
	if len(events) != 2 {
		t.Fatalf("expected two discoverable events, got %#v", events)
	}

	byTitle := map[string]map[string]any{}
	for _, item := range events {
		event := mustObject(t, item)
		byTitle[mustString(t, event, "title")] = event
	}

	publishedEvent := byTitle["Published Market"]
	if publishedEvent["status"] != "published" || publishedEvent["publicUrl"] == nil {
		t.Fatalf("unexpected public event summary: %#v", publishedEvent)
	}
	if publishedEvent["workspaceName"] != "Signal Collective" || publishedEvent["applicationsOpen"] != true {
		t.Fatalf("unexpected trust context: %#v", publishedEvent)
	}
	for _, forbidden := range []string{"applicantEmail", "message", "staffingItems", "settlement", "archive", "notes", "workspaceId", "ticketAllocation", "reservedCount", "checkedInCount", "staffingOpenCount", "staffingAssignedCount", "staffingCompletedCount", "staffingCancelledCount"} {
		if _, ok := publishedEvent[forbidden]; ok {
			t.Fatalf("discovery leaked private field %q: %#v", forbidden, publishedEvent)
		}
	}
	if publishedEvent["settlementSummary"] != nil {
		t.Fatalf("discovery leaked private fields: %#v", publishedEvent)
	}
	if int(publishedEvent["remainingTickets"].(float64)) != 39 || publishedEvent["isFull"].(bool) {
		t.Fatalf("unexpected availability: %#v", publishedEvent)
	}
	if !strings.HasPrefix(publishedEvent["publicUrl"].(string), "http://public.test/e/") {
		t.Fatalf("unexpected public url: %#v", publishedEvent["publicUrl"])
	}

	privateEvent := byTitle["Members Night"]
	if privateEvent["workspaceName"] != "Signal Collective" || privateEvent["applicationsOpen"] != false {
		t.Fatalf("unexpected private-event trust context: %#v", privateEvent)
	}
	for _, forbidden := range []string{"applicantEmail", "message", "staffingItems", "settlement", "archive", "notes", "workspaceId", "ticketAllocation", "reservedCount", "checkedInCount", "staffingOpenCount", "staffingAssignedCount", "staffingCompletedCount", "staffingCancelledCount"} {
		if _, ok := privateEvent[forbidden]; ok {
			t.Fatalf("discovery leaked private field %q: %#v", forbidden, privateEvent)
		}
	}
	if privateEvent["settlementSummary"] != nil || privateEvent["archive"] != nil {
		t.Fatalf("discovery leaked private fields: %#v", privateEvent)
	}

	titles := []string{mustString(t, draft, "title"), "Closed Night"}
	for _, title := range titles {
		if strings.Contains(fmt.Sprintf("%#v", events), title) {
			t.Fatalf("hidden event %q leaked in discovery: %#v", title, events)
		}
	}
	for _, forbidden := range []string{"applicantEmail", "message", "staffingItems", "settlement", "archive", "notes", "workspaceId"} {
		if strings.Contains(fmt.Sprintf("%#v", events), forbidden) {
			t.Fatalf("discovery response leaked %q: %#v", forbidden, events)
		}
	}
}

func TestPublicEventDiscoverySearchAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	createDiscoverEvent := func(title, description, location string) map[string]any {
		t.Helper()
		resp := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/events", map[string]any{
			"title":             title,
			"startsAt":          "2026-07-01T20:00:00Z",
			"publicDescription": description,
			"locationDisplay":   location,
			"ticketAllocation":  20,
			"pricingMode":       "free",
			"ticketPriceCents":  0,
			"ticketCurrency":    "usd",
		}, http.StatusOK)
		return mustObject(t, resp.JSON)
	}

	marketTitle := createDiscoverEvent("Market Title", "Quiet night", "Side Room")
	publishEvent(t, fx, mustString(t, marketTitle, "id"))

	marketDescription := createDiscoverEvent("Quiet Night", "Warm market sounds", "Side Room")
	publishEvent(t, fx, mustString(t, marketDescription, "id"))

	marketLocation := createDiscoverEvent("Open Stage", "Music and food", "Market Hall")
	publishEvent(t, fx, mustString(t, marketLocation, "id"))

	draft := createDiscoverEvent("Draft Market", "Draft market copy", "Draft Hall")
	closed := createDiscoverEvent("Closed Market", "Closed market copy", "Closed Hall")
	publishEvent(t, fx, mustString(t, closed, "id"))
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+mustString(t, closed, "id")+"/end-of-night", map[string]any{}, http.StatusOK)

	percentEvent := createDiscoverEvent("50% Off Night", "Special offer", "Promo Hall")
	publishEvent(t, fx, mustString(t, percentEvent, "id"))

	underscoreEvent := createDiscoverEvent("Underscore_Club", "Special offer", "Promo Hall")
	publishEvent(t, fx, mustString(t, underscoreEvent, "id"))

	resp := getJSON(t, fx.app, nil, "/api/public/events?q="+url.QueryEscape("  MaRkEt  "), http.StatusOK)
	events := resp.JSON.([]any)
	if len(events) != 3 {
		t.Fatalf("expected three discoverable market events, got %#v", events)
	}
	titles := make([]string, 0, len(events))
	for _, item := range events {
		obj := mustObject(t, item)
		titles = append(titles, mustString(t, obj, "title"))
		if obj["status"] != "published" {
			t.Fatalf("expected published event only, got %#v", obj)
		}
	}
	sort.Strings(titles)
	if !reflect.DeepEqual(titles, []string{"Market Title", "Open Stage", "Quiet Night"}) {
		t.Fatalf("unexpected market search results: %#v", titles)
	}

	missing := getJSON(t, fx.app, nil, "/api/public/events?q=missing", http.StatusOK)
	if events, ok := missing.JSON.([]any); !ok || len(events) != 0 {
		t.Fatalf("expected no results for missing query, got %#v", missing.JSON)
	}

	percent := getJSON(t, fx.app, nil, "/api/public/events?q="+url.QueryEscape("%"), http.StatusOK)
	percentEvents := percent.JSON.([]any)
	if len(percentEvents) != 1 || mustString(t, mustObject(t, percentEvents[0]), "title") != "50% Off Night" {
		t.Fatalf("expected literal percent match only, got %#v", percent.JSON)
	}

	underscore := getJSON(t, fx.app, nil, "/api/public/events?q="+url.QueryEscape("_"), http.StatusOK)
	underscoreEvents := underscore.JSON.([]any)
	if len(underscoreEvents) != 1 || mustString(t, mustObject(t, underscoreEvents[0]), "title") != "Underscore_Club" {
		t.Fatalf("expected literal underscore match only, got %#v", underscore.JSON)
	}

	longQuery := strings.Repeat("あ", 121)
	tooLong := getJSON(t, fx.app, nil, "/api/public/events?q="+url.QueryEscape(longQuery), http.StatusBadRequest)
	if mustString(t, tooLong.JSON, "error") != "search query is too long" {
		t.Fatalf("unexpected long-query error: %#v", tooLong.JSON)
	}

	if body := fmt.Sprintf("%#v", resp.JSON); strings.Contains(body, mustString(t, draft, "title")) || strings.Contains(body, mustString(t, closed, "title")) {
		t.Fatalf("hidden draft/closed events leaked into search results: %#v", resp.JSON)
	}
}

func TestEventStaffingListAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")

	ownerEmpty := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", http.StatusOK)
	memberEmpty := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/staffing", http.StatusOK)
	if !reflect.DeepEqual(ownerEmpty.JSON, memberEmpty.JSON) {
		t.Fatalf("expected owner/member staffing responses to match: owner=%#v member=%#v", ownerEmpty.JSON, memberEmpty.JSON)
	}
	if items, ok := ownerEmpty.JSON.([]any); !ok || len(items) != 0 {
		t.Fatalf("expected empty staffing list, got %#v", ownerEmpty.JSON)
	}

	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")
	role := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a set.", "capacity": 1, "public": true}, http.StatusOK)
	roleID := mustString(t, role.JSON, "id")
	application := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Alex Applicant", "applicantEmail": "alex@example.test", "message": "Happy to help."}, http.StatusOK)
	applicationID := mustString(t, application.JSON, "id")

	ownerID := ownerPersonID(t, fx)
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, starts_at, ends_at, assigned_person_id, assigned_application_id, status,
			created_by_person_id, completed_at, completed_by_person_id
		)
		values
			($1, $2, 'task', '', null, null, null, null, 'open', $3, null, null),
			($1, $4, 'shift', 'Door shift', '2026-07-01T20:00:00Z', '2026-07-01T22:00:00Z', $3, null, 'assigned', $3, null, null),
			($1, $5, 'task', 'Volunteer note', null, null, null, $6, 'completed', $3, '2026-07-01T23:30:00Z', $3)
	`, eventID, "Open task", ownerID, "Owner shift", "Application task", applicationID); err != nil {
		t.Fatal(err)
	}

	staffing := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", http.StatusOK)
	items := staffing.JSON.([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 staffing items, got %#v", staffing.JSON)
	}

	byTitle := map[string]map[string]any{}
	for _, item := range items {
		entry := mustObject(t, item)
		byTitle[entry["title"].(string)] = entry
	}

	open := byTitle["Open task"]
	if open["eventId"] != eventID || open["kind"] != "task" || open["notes"] != "" || open["status"] != "open" {
		t.Fatalf("unexpected open staffing item: %#v", open)
	}
	if _, ok := open["startsAt"]; ok {
		t.Fatalf("expected open staffing item to omit startsAt, got %#v", open)
	}
	if _, ok := open["assignedPersonId"]; ok {
		t.Fatalf("expected open staffing item to omit assignee ids, got %#v", open)
	}

	shift := byTitle["Owner shift"]
	if shift["assignedPersonId"] != ownerID || shift["assigneeName"] != "Owner" || shift["startsAt"] != "2026-07-01T20:00:00Z" || shift["endsAt"] != "2026-07-01T22:00:00Z" || shift["status"] != "assigned" {
		t.Fatalf("unexpected assigned staffing item: %#v", shift)
	}
	if _, ok := shift["completedAt"]; ok {
		t.Fatalf("expected assigned staffing item to omit completedAt, got %#v", shift)
	}

	completed := byTitle["Application task"]
	if completed["assignedApplicationId"] != applicationID || completed["assigneeName"] != "Alex Applicant" || completed["status"] != "completed" || completed["completedByPersonId"] != ownerID || completed["completedAt"] != "2026-07-01T23:30:00Z" {
		t.Fatalf("unexpected completed staffing item: %#v", completed)
	}

	getJSON(t, fx.app, nil, "/api/events/"+eventID+"/staffing", http.StatusUnauthorized)
	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, otherFx.memberCookie, "/api/events/"+eventID+"/staffing", http.StatusForbidden)
	getJSON(t, fx.app, fx.ownerCookie, "/api/events/does-not-exist/staffing", http.StatusNotFound)
}

func TestEventStaffingCreateAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")

	ownerList := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", http.StatusOK)
	memberList := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/staffing", http.StatusOK)
	if !reflect.DeepEqual(ownerList.JSON, memberList.JSON) {
		t.Fatalf("expected owner/member staffing lists to match before creation: owner=%#v member=%#v", ownerList.JSON, memberList.JSON)
	}

	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/staffing", map[string]any{
		"title": "Member task",
		"kind":  "task",
	}, http.StatusForbidden)

	createdTask := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", map[string]any{
		"title":                   "  Opening checklist  ",
		"kind":                    "task",
		"notes":                   "  Check lights and radios.  ",
		"participantRequirements": "  Bring a radio and check in with the stage manager.  ",
	}, http.StatusOK)
	task := mustObject(t, createdTask.JSON)
	taskID := mustString(t, createdTask.JSON, "id")
	if task["eventId"] != eventID || task["title"] != "Opening checklist" || task["kind"] != "task" || task["notes"] != "Check lights and radios." || task["participantRequirements"] != "Bring a radio and check in with the stage manager." || task["status"] != "open" || task["createdAt"] == "" || task["updatedAt"] == "" {
		t.Fatalf("unexpected created task response: %#v", task)
	}
	for _, field := range []string{"startsAt", "endsAt", "assignedPersonId", "assignedApplicationId", "assigneeName", "completedAt", "completedByPersonId"} {
		if _, ok := task[field]; ok {
			t.Fatalf("expected created task to omit %s, got %#v", field, task)
		}
	}

	createdShift := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", map[string]any{
		"title":    "Front door shift",
		"kind":     "shift",
		"notes":    "Volunteer at the front door.",
		"startsAt": "2026-07-01T20:00:00Z",
		"endsAt":   "2026-07-01T22:00:00Z",
	}, http.StatusOK)
	shift := mustObject(t, createdShift.JSON)
	if shift["title"] != "Front door shift" || shift["kind"] != "shift" || shift["startsAt"] != "2026-07-01T20:00:00Z" || shift["endsAt"] != "2026-07-01T22:00:00Z" || shift["status"] != "open" {
		t.Fatalf("unexpected created shift response: %#v", shift)
	}

	var auditAction, auditSubjectType, auditSubjectID, metadataText string
	if err := fx.app.db.QueryRow(t.Context(), `
		select action, subject_type, subject_id::text, metadata::text
		from audit_entries
		where action = $1 and subject_id = $2
		order by created_at desc
		limit 1
	`, "staffing.created", taskID).Scan(&auditAction, &auditSubjectType, &auditSubjectID, &metadataText); err != nil {
		t.Fatal(err)
	}
	if auditAction != "staffing.created" || auditSubjectType != "event_staffing_item" || auditSubjectID != taskID {
		t.Fatalf("unexpected staffing audit entry: action=%q subjectType=%q subjectID=%q", auditAction, auditSubjectType, auditSubjectID)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataText), &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 4 || metadata["eventId"] != eventID || metadata["staffingItemId"] != taskID || metadata["kind"] != "task" || metadata["status"] != "open" {
		t.Fatalf("unexpected staffing audit metadata: %#v", metadata)
	}
	for _, forbidden := range []string{"notes"} {
		if _, ok := metadata[forbidden]; ok {
			t.Fatalf("staffing audit metadata must not include %s: %#v", forbidden, metadata)
		}
	}

	for _, payload := range []map[string]any{
		{"title": "   ", "kind": "task"},
		{"title": "Invalid kind", "kind": "job"},
		{"title": "Bad dates", "kind": "shift", "startsAt": "2026-07-01T22:00:00Z", "endsAt": "2026-07-01T20:00:00Z"},
		{"title": "Long notes", "kind": "task", "notes": strings.Repeat("a", 2001)},
		{"title": "Long participant requirements", "kind": "task", "participantRequirements": strings.Repeat("a", 2001)},
	} {
		postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", payload, http.StatusBadRequest)
	}

	publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", map[string]any{
		"title": "After close",
		"kind":  "task",
	}, http.StatusConflict)

	ownerAfter := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", http.StatusOK)
	memberAfter := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/staffing", http.StatusOK)
	if !reflect.DeepEqual(ownerAfter.JSON, memberAfter.JSON) {
		t.Fatalf("expected owner/member staffing lists to match after creation: owner=%#v member=%#v", ownerAfter.JSON, memberAfter.JSON)
	}
	items := ownerAfter.JSON.([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 staffing items, got %#v", ownerAfter.JSON)
	}
	byTitle := map[string]map[string]any{}
	for _, item := range items {
		entry := mustObject(t, item)
		byTitle[entry["title"].(string)] = entry
	}
	if byTitle["Opening checklist"]["notes"] != "Check lights and radios." || byTitle["Opening checklist"]["participantRequirements"] != "Bring a radio and check in with the stage manager." || byTitle["Front door shift"]["notes"] != "Volunteer at the front door." || byTitle["Front door shift"]["participantRequirements"] != "" || byTitle["Front door shift"]["startsAt"] != "2026-07-01T20:00:00Z" {
		t.Fatalf("unexpected staffing list after creation: %#v", ownerAfter.JSON)
	}
}

func TestEventStaffingCreateAPIClosedEventWaitsForLock(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tx, err := fx.app.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(ctx, `
		update events
		set status = 'end_of_night', updated_at = now()
		where id = $1
	`, eventID); err != nil {
		t.Fatal(err)
	}

	done := make(chan testResponse, 1)
	go func() {
		body, _ := json.Marshal(map[string]any{"title": "After close", "kind": "task"})
		req := httptest.NewRequest(http.MethodPost, "/api/events/"+eventID+"/staffing", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(fx.ownerCookie)
		rec := httptest.NewRecorder()
		fx.app.Handler().ServeHTTP(rec, req)
		var decoded any
		if strings.TrimSpace(rec.Body.String()) != "" {
			if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
				done <- testResponse{Status: rec.Code, Body: rec.Body.String()}
				return
			}
		}
		done <- testResponse{Status: rec.Code, JSON: decoded, Body: rec.Body.String()}
	}()

	time.Sleep(50 * time.Millisecond)
	select {
	case resp := <-done:
		t.Fatalf("staffing create finished before lock release: %#v", resp)
	default:
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	resp := <-done
	if resp.Status != http.StatusConflict {
		t.Fatalf("expected closed staffing create to conflict after lock release, got %#v", resp)
	}
	if got := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", http.StatusOK).JSON.([]any); len(got) != 0 {
		t.Fatalf("expected no staffing items to be created, got %#v", got)
	}
}

func TestEventStaffingUpdateAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")

	accepted := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": mustString(t, postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a set.", "capacity": 1, "public": true}, http.StatusOK).JSON, "id"), "applicantName": "Alex Applicant", "applicantEmail": "alex@example.test", "message": "Happy to help."}, http.StatusOK)
	acceptedAppID := mustString(t, accepted.JSON, "id")
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'accepted', updated_at = now()
		where id = $1
	`, acceptedAppID); err != nil {
		t.Fatal(err)
	}

	submitted := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": mustString(t, postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Support", "description": "Help out.", "capacity": 1, "public": true}, http.StatusOK).JSON, "id"), "applicantName": "Sam Submitted", "applicantEmail": "submitted@example.test", "message": "Still waiting."}, http.StatusOK)
	submittedAppID := mustString(t, submitted.JSON, "id")
	rejected := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": mustString(t, postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Runner", "description": "Run errands.", "capacity": 1, "public": true}, http.StatusOK).JSON, "id"), "applicantName": "Riley Rejected", "applicantEmail": "rejected@example.test", "message": "Maybe later."}, http.StatusOK)
	rejectedAppID := mustString(t, rejected.JSON, "id")
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'rejected', updated_at = now()
		where id = $1
	`, rejectedAppID); err != nil {
		t.Fatal(err)
	}

	otherEvent := createEvent(t, fx, "Other Night", 2)
	otherEventID := mustString(t, otherEvent, "id")
	otherPublished := publishEvent(t, fx, otherEventID)
	otherSlug := mustString(t, otherPublished, "publicSlug")
	otherRole := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+otherEventID+"/roles", map[string]any{"name": "Other Performer", "description": "Other set.", "capacity": 1, "public": true}, http.StatusOK)
	otherRoleID := mustString(t, otherRole.JSON, "id")
	otherAccepted := postJSON(t, fx.app, nil, "/api/public/events/"+otherSlug+"/role-applications", map[string]any{"roleId": otherRoleID, "applicantName": "Other Applicant", "applicantEmail": "other@example.test", "message": "Different event."}, http.StatusOK)
	otherAcceptedID := mustString(t, otherAccepted.JSON, "id")
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'accepted', updated_at = now()
		where id = $1
	`, otherAcceptedID); err != nil {
		t.Fatal(err)
	}

	memberID := mustString(t, func() map[string]any {
		var personID string
		if err := fx.app.db.QueryRow(t.Context(), `
			select person_id
			from workspace_members
			where workspace_id = $1
			  and role = 'member'
			  and removed_at is null
			limit 1
		`, fx.workspaceID).Scan(&personID); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"id": personID}
	}(), "id")
	ownerID := ownerPersonID(t, fx)
	memberEmail := fx.email("member")

	item := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", map[string]any{"title": "Opening checklist", "kind": "task", "notes": "Check lights and radios.", "participantRequirements": "Wear closed-toe shoes."}, http.StatusOK)
	taskID := mustString(t, item.JSON, "id")
	if got := mustObject(t, item.JSON)["participantRequirements"]; got != "Wear closed-toe shoes." {
		t.Fatalf("created participant requirements=%#v", got)
	}
	updatedRequirements := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"participantRequirements": "Bring ear protection."}, http.StatusOK)
	if got := mustObject(t, updatedRequirements.JSON)["participantRequirements"]; got != "Bring ear protection." {
		t.Fatalf("updated participant requirements=%#v", got)
	}
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"participantRequirements": strings.Repeat("a", 2001)}, http.StatusBadRequest)

	assignedMember := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"assignedPersonId": memberID}, http.StatusOK)
	assignedMemberObj := mustObject(t, assignedMember.JSON)
	if assignedMemberObj["status"] != "assigned" || assignedMemberObj["assignedPersonId"] != memberID || assignedMemberObj["assigneeName"] != "Door" {
		t.Fatalf("unexpected member assignment response: %#v", assignedMemberObj)
	}
	assertStaffingAssignmentNotificationCounts(t, fx, eventID, 1, 1)
	assertStaffingAssignmentNotificationRecord(t, fx, taskID, memberEmail, "staffing.assignment", "staffing_assignment:"+taskID+":"+memberEmail, "Night Market", "Opening checklist", "Check lights and radios.")

	repeatAssignedMember := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"assignedPersonId": memberID}, http.StatusOK)
	if !reflect.DeepEqual(assignedMember.JSON, repeatAssignedMember.JSON) {
		t.Fatalf("expected repeated member assignment to be idempotent: first=%#v second=%#v", assignedMember.JSON, repeatAssignedMember.JSON)
	}
	assertStaffingAssignmentNotificationCounts(t, fx, eventID, 1, 1)

	var metadataText string
	if err := fx.app.db.QueryRow(t.Context(), `
		select metadata::text
		from audit_entries
		where action = $1
		  and subject_id = $2
		order by created_at desc
		limit 1
	`, "staffing.updated", taskID).Scan(&metadataText); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataText), &metadata); err != nil {
		t.Fatal(err)
	}
	if _, ok := metadata["notes"]; ok {
		t.Fatalf("staffing update audit must not include notes: %#v", metadata)
	}
	if len(metadata) != 6 || metadata["eventId"] != eventID || metadata["staffingItemId"] != taskID || metadata["previousStatus"] != "open" || metadata["status"] != "assigned" || metadata["assignedPersonId"] != memberID || metadata["assignedApplicationId"] != nil {
		t.Fatalf("unexpected staffing update audit metadata: %#v", metadata)
	}

	cleared := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"clearAssignee": true}, http.StatusOK)
	clearedObj := mustObject(t, cleared.JSON)
	if clearedObj["status"] != "open" {
		t.Fatalf("expected cleared assignee to reopen item, got %#v", clearedObj)
	}
	assertStaffingAssignmentNotificationCounts(t, fx, eventID, 1, 1)
	for _, field := range []string{"assignedPersonId", "assignedApplicationId", "assigneeName"} {
		if _, ok := clearedObj[field]; ok {
			t.Fatalf("expected cleared item to omit %s, got %#v", field, clearedObj)
		}
	}

	assignedApplication := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"assignedApplicationId": acceptedAppID}, http.StatusOK)
	assignedApplicationObj := mustObject(t, assignedApplication.JSON)
	if assignedApplicationObj["status"] != "assigned" || assignedApplicationObj["assignedApplicationId"] != acceptedAppID || assignedApplicationObj["assigneeName"] != "Alex Applicant" {
		t.Fatalf("unexpected application assignment response: %#v", assignedApplicationObj)
	}
	assertStaffingAssignmentNotificationCounts(t, fx, eventID, 2, 2)
	assertStaffingAssignmentNotificationRecord(t, fx, taskID, "alex@example.test", "staffing.assignment", "staffing_assignment:"+taskID+":alex@example.test", "Night Market", "Opening checklist", "Happy to help.")

	for _, payload := range []map[string]any{
		{"assignedApplicationId": submittedAppID},
		{"assignedApplicationId": rejectedAppID},
		{"assignedApplicationId": otherAcceptedID},
	} {
		patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, payload, http.StatusBadRequest)
	}

	completed := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"status": "completed"}, http.StatusOK)
	completedObj := mustObject(t, completed.JSON)
	completedAt := completedObj["completedAt"].(string)
	completedBy := completedObj["completedByPersonId"].(string)
	if completedObj["status"] != "completed" || completedAt == "" || completedBy != ownerID {
		t.Fatalf("unexpected completed response: %#v", completedObj)
	}
	assertStaffingAssignmentNotificationCounts(t, fx, eventID, 2, 2)

	var beforeCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from audit_entries
		where action = 'staffing.updated'
		  and subject_id = $1
	`, taskID).Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	completedAgain := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"status": "completed"}, http.StatusOK)
	completedAgainObj := mustObject(t, completedAgain.JSON)
	if completedAgainObj["completedAt"] != completedAt || completedAgainObj["completedByPersonId"] != completedBy || completedAgainObj["status"] != "completed" {
		t.Fatalf("expected completed idempotency, got %#v", completedAgainObj)
	}
	var afterCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from audit_entries
		where action = 'staffing.updated'
		  and subject_id = $1
	`, taskID).Scan(&afterCount); err != nil {
		t.Fatal(err)
	}
	if beforeCount != afterCount {
		t.Fatalf("expected repeated complete to avoid duplicate audit, before=%d after=%d", beforeCount, afterCount)
	}

	cancelled := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"status": "cancelled"}, http.StatusOK)
	cancelledObj := mustObject(t, cancelled.JSON)
	if cancelledObj["status"] != "cancelled" {
		t.Fatalf("unexpected cancelled response: %#v", cancelledObj)
	}
	assertStaffingAssignmentNotificationCounts(t, fx, eventID, 2, 2)
	for _, field := range []string{"completedAt", "completedByPersonId"} {
		if _, ok := cancelledObj[field]; ok {
			t.Fatalf("expected cancelled item to omit %s, got %#v", field, cancelledObj)
		}
	}

	shift := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", map[string]any{"title": "Door shift", "kind": "shift", "notes": "Front door coverage.", "startsAt": "2026-07-01T20:00:00Z", "endsAt": "2026-07-01T22:00:00Z"}, http.StatusOK)
	shiftID := mustString(t, shift.JSON, "id")
	clearedShift := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+shiftID, map[string]any{"clearStartsAt": true, "clearEndsAt": true}, http.StatusOK)
	clearedShiftObj := mustObject(t, clearedShift.JSON)
	for _, field := range []string{"startsAt", "endsAt"} {
		if _, ok := clearedShiftObj[field]; ok {
			t.Fatalf("expected cleared shift to omit %s, got %#v", field, clearedShiftObj)
		}
	}

	patchJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"title": "Member update"}, http.StatusForbidden)

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing/"+taskID, map[string]any{"title": "Closed update"}, http.StatusConflict)
}

func TestArchiveCapturesDuplicateParticipantNamesFromDistinctApplications(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")

	performer := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a 20-minute set.", "capacity": 3, "public": true}, http.StatusOK)
	roleID := mustString(t, performer.JSON, "id")

	first := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Jordan", "applicantEmail": "jordan-a@example.com", "message": "First applicant."}, http.StatusOK)
	second := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Jordan", "applicantEmail": "jordan-b@example.com", "message": "Second applicant."}, http.StatusOK)

	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'accepted', updated_at = now()
		where id = $1
	`, mustString(t, first.JSON, "id")); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'confirmed', updated_at = now()
		where id = $1
	`, mustString(t, second.JSON, "id")); err != nil {
		t.Fatal(err)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	archive := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	participants, ok := mustObject(t, archive.JSON)["participants"].([]any)
	if !ok || len(participants) != 2 {
		t.Fatalf("expected two archived participants, got %#v", archive.JSON)
	}

	seenSourceIDs := map[string]struct{}{}
	for _, participant := range participants {
		entry := mustObject(t, participant)
		if entry["participantName"] != "Jordan" {
			t.Fatalf("unexpected participant name: %#v", entry)
		}
		sourceID := mustString(t, entry, "sourceApplicationId")
		if _, exists := seenSourceIDs[sourceID]; exists {
			t.Fatalf("expected distinct source applications, got duplicate source id %s", sourceID)
		}
		seenSourceIDs[sourceID] = struct{}{}
	}
}

func TestArchiveCapturesStaffingMemoryOnceAndKeepsSnapshotImmutable(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	ownerID := ownerPersonID(t, fx)
	publishEvent(t, fx, eventID)

	first := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", map[string]any{
		"title": "Door shift",
		"kind":  "shift",
		"notes": "Open with the side entrance.",
	}, http.StatusOK)
	second := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/staffing", map[string]any{
		"title": "Door shift",
		"kind":  "task",
		"notes": "Close the side entrance at end of night.",
	}, http.StatusOK)
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_staffing_items
		set status = 'assigned', assigned_person_id = $2, updated_at = now()
		where id = $1
	`, mustString(t, second.JSON, "id"), ownerID); err != nil {
		t.Fatal(err)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	archiveBefore := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	archiveObj := mustObject(t, archiveBefore.JSON)
	staffingItems, ok := archiveObj["staffingItems"].([]any)
	if !ok || len(staffingItems) != 2 {
		t.Fatalf("expected two archived staffing items, got %#v", archiveBefore.JSON)
	}
	seenSourceIDs := map[string]struct{}{}
	for _, item := range staffingItems {
		entry := mustObject(t, item)
		if entry["title"] != "Door shift" {
			t.Fatalf("unexpected staffing snapshot item: %#v", entry)
		}
		if _, ok := entry["notes"]; ok {
			t.Fatalf("archive staffing memory must not expose notes: %#v", entry)
		}
		sourceID := mustString(t, entry, "sourceStaffingItemId")
		if _, exists := seenSourceIDs[sourceID]; exists {
			t.Fatalf("expected distinct source staffing items, got duplicate source id %s", sourceID)
		}
		seenSourceIDs[sourceID] = struct{}{}
	}

	if _, err := fx.app.db.Exec(t.Context(), `
		update event_staffing_items
		set title = 'Changed after closeout', notes = 'mutated source row', updated_at = now()
		where id = $1
	`, mustString(t, first.JSON, "id")); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, status, created_by_person_id
		)
		values ($1, 'Late addition', 'task', 'Should not appear in archive', 'open', $2)
	`, eventID, ownerID); err != nil {
		t.Fatal(err)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	archiveAfter := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	if !reflect.DeepEqual(archiveBefore.JSON, archiveAfter.JSON) {
		t.Fatalf("expected archive staffing snapshot to remain immutable after retry: before=%#v after=%#v", archiveBefore.JSON, archiveAfter.JSON)
	}
}

func TestWorkspaceEventListIncludesStaffingCounts(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	ownerID := ownerPersonID(t, fx)

	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_staffing_items (
			event_id, title, kind, notes, status, created_by_person_id, assigned_person_id, completed_at, completed_by_person_id
		)
		values
			($1, 'Open task', 'task', '', 'open', $2, null, null, null),
			($1, 'Assigned shift', 'shift', '', 'assigned', $2, $2, null, null),
			($1, 'Completed task', 'task', '', 'completed', $2, null, '2026-07-01T22:30:00Z', $2),
			($1, 'Cancelled shift', 'shift', '', 'cancelled', $2, null, null, null)
	`, eventID, ownerID); err != nil {
		t.Fatal(err)
	}

	resp := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/events", http.StatusOK)
	events := resp.JSON.([]any)
	if len(events) != 1 {
		t.Fatalf("expected one event in workspace list, got %#v", resp.JSON)
	}
	listed := mustObject(t, events[0])
	if listed["staffingOpenCount"] != float64(1) || listed["staffingAssignedCount"] != float64(1) || listed["staffingCompletedCount"] != float64(1) || listed["staffingCancelledCount"] != float64(1) {
		t.Fatalf("unexpected workspace staffing counts: %#v", listed)
	}

	detail := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID, http.StatusOK)
	detailObj := mustObject(t, detail.JSON)
	if detailObj["staffingOpenCount"] != listed["staffingOpenCount"] || detailObj["staffingAssignedCount"] != listed["staffingAssignedCount"] || detailObj["staffingCompletedCount"] != listed["staffingCompletedCount"] || detailObj["staffingCancelledCount"] != listed["staffingCancelledCount"] {
		t.Fatalf("expected event detail staffing counts to match list counts: list=%#v detail=%#v", listed, detailObj)
	}
}

func TestLegacyReportCreatesArchiveOnceAndKeepsSnapshotImmutable(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")
	ownerID := ownerPersonID(t, fx)

	performer := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a 20-minute set.", "capacity": 3, "public": true}, http.StatusOK)
	roleID := mustString(t, performer.JSON, "id")

	first := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": "Bring a keyboard."}, http.StatusOK)
	second := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Blair", "applicantEmail": "blair@example.com", "message": "Backup vocals."}, http.StatusOK)

	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'accepted', updated_at = now()
		where id = $1
	`, mustString(t, first.JSON, "id")); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'confirmed', updated_at = now()
		where id = $1
	`, mustString(t, second.JSON, "id")); err != nil {
		t.Fatal(err)
	}

	legacySnapshot := map[string]any{
		"id":                     "legacy-report",
		"eventId":                eventID,
		"title":                  "Night Market",
		"startsAt":               event["startsAt"],
		"publicUrl":              published["publicUrl"],
		"ticketAllocation":       4,
		"ticketsReserved":        0,
		"ticketsCheckedIn":       0,
		"noShows":                0,
		"generatedAt":            time.Now().UTC().Format(time.RFC3339Nano),
		"generatedByMemberEmail": fx.email("owner"),
	}
	payload, err := json.Marshal(legacySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_reports (event_id, generated_by_person_id, snapshot)
		values ($1, $2, $3)
	`, eventID, ownerID, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_settlements (event_id, currency, generated_by_person_id)
		values ($1, 'usd', $2)
	`, eventID, ownerID); err != nil {
		t.Fatal(err)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	archiveBefore := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	if participants, ok := mustObject(t, archiveBefore.JSON)["participants"].([]any); !ok || len(participants) != 2 {
		t.Fatalf("expected two archived participants from legacy report retry, got %#v", archiveBefore.JSON)
	}

	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Casey", "applicantEmail": "casey@example.com"}, http.StatusNotFound)
	insertRoleApplicationFixture(t, fx, eventID, roleID, "Casey", "casey@example.com", "accepted")

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	archiveAfter := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	if !reflect.DeepEqual(archiveBefore.JSON, archiveAfter.JSON) {
		t.Fatalf("expected legacy archive snapshot to remain immutable after retry: before=%#v after=%#v", archiveBefore.JSON, archiveAfter.JSON)
	}
}

func TestEventRoleDefinitionsAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	roleSlug := "event-role-definitions-" + strings.ReplaceAll(fx.suffix, "_", "-")
	if _, err := fx.app.db.Exec(t.Context(), `
		update events
		set public_slug = $2
		where id = $1
	`, eventID, roleSlug); err != nil {
		t.Fatal(err)
	}

	if ownerList := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", http.StatusOK); len(ownerList.JSON.([]any)) != 0 {
		t.Fatalf("expected empty role list, got %#v", ownerList.JSON)
	}
	if memberList := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/roles", http.StatusOK); len(memberList.JSON.([]any)) != 0 {
		t.Fatalf("expected empty member role list, got %#v", memberList.JSON)
	}

	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Denied", "description": "Member", "capacity": 1, "public": true}, http.StatusForbidden)

	performer := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": " Performer ", "description": " Play a 20-minute set. ", "capacity": 3, "public": true}, http.StatusOK)
	performerRole := mustObject(t, performer.JSON)
	if performerRole["eventId"] != eventID || performerRole["name"] != "Performer" || performerRole["description"] != "Play a 20-minute set." || int(performerRole["capacity"].(float64)) != 3 || performerRole["public"] != true || performerRole["active"] != true || performerRole["id"] == "" || performerRole["createdAt"] == "" || performerRole["updatedAt"] == "" {
		t.Fatalf("unexpected role response: %#v", performerRole)
	}
	performerRoleID := mustString(t, performer.JSON, "id")
	patchJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/roles/"+performerRoleID, map[string]any{"name": "Denied"}, http.StatusForbidden)
	updatedPerformer := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles/"+performerRoleID, map[string]any{"name": "Lead Performer", "description": "Updated notes.", "capacity": 2, "public": false, "active": false}, http.StatusOK)
	updatedRole := mustObject(t, updatedPerformer.JSON)
	if updatedRole["name"] != "Lead Performer" || updatedRole["description"] != "Updated notes." || int(updatedRole["capacity"].(float64)) != 2 || updatedRole["public"] != false || updatedRole["active"] != false {
		t.Fatalf("unexpected updated role response: %#v", updatedRole)
	}
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles/"+performerRoleID, map[string]any{}, http.StatusBadRequest)

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": " Host ", "description": "Run the door", "capacity": 0}, http.StatusOK)
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_roles (event_id, name, description, capacity, "public", active, created_by_person_id, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $8)
	`, eventID, "Backstage", "Private notes", 0, false, true, ownerPersonID(t, fx), time.Now().Add(2*time.Minute).UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_roles (event_id, name, description, capacity, "public", active, created_by_person_id, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $8)
	`, eventID, "Disabled", "Inactive role", 0, true, false, ownerPersonID(t, fx), time.Now().Add(3*time.Minute).UTC()); err != nil {
		t.Fatal(err)
	}

	unpublished := getJSON(t, fx.app, nil, "/api/public/events/"+roleSlug+"/roles", http.StatusNotFound)
	if unpublished.Status != http.StatusNotFound {
		t.Fatalf("expected unpublished public roles to 404, got %d", unpublished.Status)
	}

	published := publishEvent(t, fx, eventID)
	if mustString(t, published, "publicSlug") != roleSlug {
		t.Fatalf("expected publish to preserve manual slug, got %#v", published)
	}
	var storedSlug, auditedSlug string
	if err := fx.app.db.QueryRow(t.Context(), `select public_slug from events where id=$1`, eventID).Scan(&storedSlug); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `select metadata->>'publicSlug' from audit_entries where action='event.published' and subject_id=$1`, eventID).Scan(&auditedSlug); err != nil {
		t.Fatal(err)
	}
	if storedSlug != roleSlug || auditedSlug != roleSlug {
		t.Fatalf("publication response, database and audit must agree: stored=%q audited=%q", storedSlug, auditedSlug)
	}

	publicRoles := getJSON(t, fx.app, nil, "/api/public/events/"+roleSlug+"/roles", http.StatusOK)
	publicRoleList := publicRoles.JSON.([]any)
	if len(publicRoleList) != 1 {
		t.Fatalf("expected one public role, got %#v", publicRoles.JSON)
	}
	if mustObject(t, publicRoleList[0])["name"] != "Host" {
		t.Fatalf("expected public roles ordered by creation, got %#v", publicRoles.JSON)
	}

	ownerRoles := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", http.StatusOK).JSON.([]any)
	memberRoles := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/roles", http.StatusOK).JSON.([]any)
	if len(ownerRoles) != 4 || len(memberRoles) != 4 {
		t.Fatalf("expected owner/member to read all roles, owner=%#v member=%#v", ownerRoles, memberRoles)
	}
	if !reflect.DeepEqual(ownerRoles, memberRoles) {
		t.Fatalf("expected owner/member role reads to match, owner=%#v member=%#v", ownerRoles, memberRoles)
	}
	if mustObject(t, ownerRoles[2])["name"] != "Backstage" || mustObject(t, ownerRoles[3])["name"] != "Disabled" {
		t.Fatalf("expected private/inactive roles to be retained privately, got %#v", ownerRoles)
	}
}

func TestPublicRoleApplicationsAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")

	publicRole := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a 20-minute set.", "capacity": 2, "public": true}, http.StatusOK)
	roleID := mustString(t, publicRole.JSON, "id")

	var privateRoleID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_roles (event_id, name, description, capacity, "public", active, created_by_person_id, created_at, updated_at)
		values ($1, $2, $3, $4, false, true, $5, $6, $6)
		returning id
	`, eventID, "Backstage", "Private notes", 0, ownerPersonID(t, fx), time.Now().UTC()).Scan(&privateRoleID); err != nil {
		t.Fatal(err)
	}
	var inactiveRoleID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into event_roles (event_id, name, description, capacity, "public", active, created_by_person_id, created_at, updated_at)
		values ($1, $2, $3, $4, true, false, $5, $6, $6)
		returning id
	`, eventID, "Disabled", "Inactive role", 0, ownerPersonID(t, fx), time.Now().Add(time.Minute).UTC()).Scan(&inactiveRoleID); err != nil {
		t.Fatal(err)
	}

	otherEvent := createEvent(t, fx, "Other Night", 4)
	otherEventID := mustString(t, otherEvent, "id")
	publishEvent(t, fx, otherEventID)
	otherRole := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+otherEventID+"/roles", map[string]any{"name": "Host", "description": "Run the door.", "capacity": 1, "public": true}, http.StatusOK)
	otherRoleID := mustString(t, otherRole.JSON, "id")

	draftEvent := createEvent(t, fx, "Draft Night", 4)
	draftEventID := mustString(t, draftEvent, "id")
	draftSlug := "draft-role-applications-" + strings.ReplaceAll(fx.suffix, "_", "-")
	if _, err := fx.app.db.Exec(t.Context(), `
		update events
		set public_slug = $2
		where id = $1
	`, draftEventID, draftSlug); err != nil {
		t.Fatal(err)
	}
	draftRole := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+draftEventID+"/roles", map[string]any{"name": "Runner", "description": "Draft role.", "capacity": 1, "public": true}, http.StatusOK)
	draftRoleID := mustString(t, draftRole.JSON, "id")

	created := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{
		"roleId":         roleID,
		"applicantName":  "  Alex  ",
		"applicantEmail": "  ALEX@example.com ",
		"message":        "  Bring a keyboard.  ",
	}, http.StatusOK)
	application := mustObject(t, created.JSON)
	applicationID := mustString(t, created.JSON, "id")
	if application["eventId"] != eventID || application["roleId"] != roleID || application["applicantName"] != "Alex" || application["applicantEmail"] != "alex@example.com" || application["message"] != "Bring a keyboard." || application["status"] != "submitted" || application["createdAt"] == "" || application["updatedAt"] == "" {
		t.Fatalf("unexpected application response: %#v", application)
	}

	var auditAction, auditSubjectType, auditSubjectID, metadataText string
	if err := fx.app.db.QueryRow(t.Context(), `
		select action, subject_type, subject_id::text, metadata::text
		from audit_entries
		where action = $1 and subject_id = $2
		order by created_at desc
		limit 1
	`, "role_application.submitted", applicationID).Scan(&auditAction, &auditSubjectType, &auditSubjectID, &metadataText); err != nil {
		t.Fatal(err)
	}
	if auditAction != "role_application.submitted" || auditSubjectType != "event_role_application" || auditSubjectID != applicationID {
		t.Fatalf("unexpected audit entry: action=%q subjectType=%q subjectID=%q", auditAction, auditSubjectType, auditSubjectID)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataText), &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 3 || metadata["eventId"] != eventID || metadata["roleId"] != roleID || metadata["applicationId"] != applicationID {
		t.Fatalf("unexpected audit metadata: %#v", metadata)
	}
	for _, forbidden := range []string{"applicantName", "applicantEmail", "message"} {
		if _, ok := metadata[forbidden]; ok {
			t.Fatalf("audit metadata must not include %s: %#v", forbidden, metadata)
		}
	}

	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": "Follow-up"}, http.StatusConflict)

	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'withdrawn', updated_at = now()
		where id = $1
	`, applicationID); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": "Follow-up"}, http.StatusOK)

	postJSON(t, fx.app, nil, "/api/public/events/"+draftSlug+"/role-applications", map[string]any{"roleId": draftRoleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": "Draft event"}, http.StatusNotFound)
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": privateRoleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": "Private"}, http.StatusNotFound)
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": inactiveRoleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": "Inactive"}, http.StatusNotFound)
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": otherRoleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": "Mismatch"}, http.StatusNotFound)
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "   ", "applicantEmail": "alex@example.com", "message": "Name"}, http.StatusBadRequest)
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Alex", "applicantEmail": "not-an-email", "message": "Email"}, http.StatusBadRequest)
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": "Alex", "applicantEmail": "alex@example.com", "message": strings.Repeat("a", 2001)}, http.StatusBadRequest)
}

func TestEventRoleApplicationReviewAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")

	role := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Performer", "description": "Play a 20-minute set.", "capacity": 2, "public": true}, http.StatusOK)
	roleID := mustString(t, role.JSON, "id")

	applicantIDs := make([]string, 0, 3)
	for _, draft := range []struct {
		name  string
		email string
	}{
		{name: "Alex", email: "alex@example.com"},
		{name: "Brie", email: "brie@example.com"},
		{name: "Casey", email: "casey@example.com"},
	} {
		created := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": roleID, "applicantName": draft.name, "applicantEmail": draft.email, "message": draft.name + " on stage"}, http.StatusOK)
		applicantIDs = append(applicantIDs, mustString(t, created.JSON, "id"))
	}

	ownerList := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications", http.StatusOK)
	memberList := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/role-applications", http.StatusOK)
	if !reflect.DeepEqual(ownerList.JSON, memberList.JSON) {
		t.Fatalf("expected owner/member application lists to match: owner=%#v member=%#v", ownerList.JSON, memberList.JSON)
	}
	applications := ownerList.JSON.([]any)
	if len(applications) != 3 {
		t.Fatalf("expected three applications, got %#v", ownerList.JSON)
	}
	first := mustObject(t, applications[0])
	second := mustObject(t, applications[1])
	third := mustObject(t, applications[2])
	if first["applicantName"] != "Alex" || first["status"] != "submitted" || first["reviewedAt"] != nil {
		t.Fatalf("unexpected first application payload: %#v", first)
	}
	for _, field := range []string{"reviewedByPersonId", "reviewedAt"} {
		if _, ok := first[field]; ok {
			t.Fatalf("did not expect %s before review: %#v", field, first)
		}
	}
	if second["applicantName"] != "Brie" || third["applicantName"] != "Casey" {
		t.Fatalf("unexpected application ordering: %#v", ownerList.JSON)
	}

	patchJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[0], map[string]any{"status": "under_review"}, http.StatusForbidden)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[0], map[string]any{"status": "bogus"}, http.StatusBadRequest)

	underReview := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[2], map[string]any{"status": "under_review"}, http.StatusOK)
	if mustString(t, underReview.JSON, "status") != "under_review" {
		t.Fatalf("unexpected under_review response: %#v", underReview.JSON)
	}
	withdrawn := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[2], map[string]any{"status": "withdrawn"}, http.StatusOK)
	if mustString(t, withdrawn.JSON, "status") != "withdrawn" {
		t.Fatalf("unexpected withdrawn response: %#v", withdrawn.JSON)
	}

	accepted := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[0], map[string]any{"status": "accepted"}, http.StatusOK)
	acceptedObj := mustObject(t, accepted.JSON)
	if acceptedObj["status"] != "accepted" || acceptedObj["reviewedByPersonId"] == nil || acceptedObj["reviewedAt"] == nil {
		t.Fatalf("unexpected accepted response: %#v", acceptedObj)
	}
	assertRoleApplicationNotificationCounts(t, fx, eventID, 1, 1)
	assertRoleApplicationNotificationRecord(t, fx, applicantIDs[0], "role_application.accepted", "role_application:"+applicantIDs[0]+":accepted", mustString(t, event, "title"), "Performer", "Alex on stage")
	confirmed := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[1], map[string]any{"status": "confirmed"}, http.StatusOK)
	if mustString(t, confirmed.JSON, "status") != "confirmed" {
		t.Fatalf("unexpected confirmed response: %#v", confirmed.JSON)
	}
	assertRoleApplicationNotificationCounts(t, fx, eventID, 1, 1)

	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[2], map[string]any{"status": "accepted"}, http.StatusConflict)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[2], map[string]any{"status": "confirmed"}, http.StatusConflict)

	acceptedConfirmed := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[0], map[string]any{"status": "confirmed"}, http.StatusOK)
	if mustString(t, acceptedConfirmed.JSON, "status") != "confirmed" {
		t.Fatalf("unexpected accepted->confirmed response: %#v", acceptedConfirmed.JSON)
	}
	assertRoleApplicationNotificationCounts(t, fx, eventID, 1, 1)
	confirmedAgain := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[0], map[string]any{"status": "confirmed"}, http.StatusOK)
	if mustString(t, confirmedAgain.JSON, "status") != "confirmed" {
		t.Fatalf("unexpected confirmed->confirmed response: %#v", confirmedAgain.JSON)
	}
	assertRoleApplicationNotificationCounts(t, fx, eventID, 1, 1)

	waitlisted := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[2], map[string]any{"status": "waitlisted"}, http.StatusOK)
	if mustString(t, waitlisted.JSON, "status") != "waitlisted" {
		t.Fatalf("unexpected waitlisted response: %#v", waitlisted.JSON)
	}
	assertRoleApplicationNotificationCounts(t, fx, eventID, 2, 2)
	assertRoleApplicationNotificationRecord(t, fx, applicantIDs[2], "role_application.waitlisted", "role_application:"+applicantIDs[2]+":waitlisted", mustString(t, event, "title"), "Performer", "Casey on stage")

	rejected := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications/"+applicantIDs[1], map[string]any{"status": "rejected"}, http.StatusOK)
	if mustString(t, rejected.JSON, "status") != "rejected" {
		t.Fatalf("unexpected rejected response: %#v", rejected.JSON)
	}
	assertRoleApplicationNotificationCounts(t, fx, eventID, 3, 3)
	assertRoleApplicationNotificationRecord(t, fx, applicantIDs[1], "role_application.rejected", "role_application:"+applicantIDs[1]+":rejected", mustString(t, event, "title"), "Performer", "Brie on stage")

	var auditAction, auditSubjectType, auditSubjectID, metadataText string
	if err := fx.app.db.QueryRow(t.Context(), `
		select action, subject_type, subject_id::text, metadata::text
		from audit_entries
		where action = $1
		  and subject_id = $2
		  and metadata->>'previousStatus' = 'accepted'
		  and metadata->>'nextStatus' = 'confirmed'
		order by created_at desc
		limit 1
	`, "role_application.reviewed", applicantIDs[0]).Scan(&auditAction, &auditSubjectType, &auditSubjectID, &metadataText); err != nil {
		t.Fatal(err)
	}
	if auditAction != "role_application.reviewed" || auditSubjectType != "event_role_application" || auditSubjectID != applicantIDs[0] {
		t.Fatalf("unexpected review audit entry: action=%q subjectType=%q subjectID=%q", auditAction, auditSubjectType, auditSubjectID)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataText), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["eventId"] != eventID || metadata["roleId"] != roleID || metadata["applicationId"] != applicantIDs[0] || metadata["previousStatus"] != "accepted" || metadata["nextStatus"] != "confirmed" {
		t.Fatalf("unexpected review audit metadata: %#v", metadata)
	}
	for _, forbidden := range []string{"applicantEmail", "message"} {
		if _, ok := metadata[forbidden]; ok {
			t.Fatalf("review audit metadata must not include %s: %#v", forbidden, metadata)
		}
	}

	updatedList := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/role-applications", http.StatusOK).JSON.([]any)
	updatedFirst := mustObject(t, updatedList[0])
	if updatedFirst["status"] != "confirmed" || updatedFirst["reviewedByPersonId"] == nil || updatedFirst["reviewedAt"] == nil {
		t.Fatalf("expected confirmed application to include review timestamps: %#v", updatedFirst)
	}
	for _, forbidden := range []string{"applicantEmail", "message"} {
		if _, ok := metadata[forbidden]; ok {
			t.Fatalf("review audit metadata must not include %s: %#v", forbidden, metadata)
		}
	}
}

func assertRoleApplicationNotificationCounts(t *testing.T, fx lifecycleFixture, eventID string, wantNotifications, wantOutbox int) {
	t.Helper()
	var notificationCount int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from notification_events where event_id = $1`, eventID).Scan(&notificationCount); err != nil {
		t.Fatal(err)
	}
	if notificationCount != wantNotifications {
		t.Fatalf("unexpected notification count for event %s: got %d want %d", eventID, notificationCount, wantNotifications)
	}

	var outboxCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from email_outbox o
		join notification_events n on n.email_outbox_id = o.id
		where n.event_id = $1
	`, eventID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != wantOutbox {
		t.Fatalf("unexpected outbox count for event %s: got %d want %d", eventID, outboxCount, wantOutbox)
	}
}

func assertRoleApplicationNotificationRecord(t *testing.T, fx lifecycleFixture, applicationID, wantType, wantKey, eventTitle, roleName, applicantMessage string) {
	t.Helper()
	var notificationType, idempotencyKey, subject, preview, body string
	if err := fx.app.db.QueryRow(t.Context(), `
		select n.notification_type, n.idempotency_key, n.subject, n.preview, o.body
		from notification_events n
		join email_outbox o on o.id = n.email_outbox_id
		where n.related_id = $1
		order by n.created_at desc
		limit 1
	`, applicationID).Scan(&notificationType, &idempotencyKey, &subject, &preview, &body); err != nil {
		t.Fatal(err)
	}
	if notificationType != wantType || idempotencyKey != wantKey {
		t.Fatalf("unexpected notification identity for %s: type=%q key=%q", applicationID, notificationType, idempotencyKey)
	}
	if !strings.Contains(subject, eventTitle) || !strings.Contains(subject, roleName) {
		t.Fatalf("notification subject missing event/role: %q", subject)
	}
	if preview != "Application "+strings.TrimPrefix(wantType, "role_application.")+" for "+roleName {
		t.Fatalf("unexpected notification preview: %q", preview)
	}
	if !strings.Contains(body, eventTitle) || !strings.Contains(body, roleName) {
		t.Fatalf("notification body missing event/role: %q", body)
	}
	if strings.Contains(subject, applicantMessage) || strings.Contains(preview, applicantMessage) || strings.Contains(body, applicantMessage) {
		t.Fatalf("notification leaked application message %q: subject=%q preview=%q body=%q", applicantMessage, subject, preview, body)
	}
}

func assertStaffingAssignmentNotificationCounts(t *testing.T, fx lifecycleFixture, eventID string, wantNotifications, wantOutbox int) {
	t.Helper()
	var notificationCount int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from notification_events where event_id = $1`, eventID).Scan(&notificationCount); err != nil {
		t.Fatal(err)
	}
	if notificationCount != wantNotifications {
		t.Fatalf("unexpected staffing notification count for event %s: got %d want %d", eventID, notificationCount, wantNotifications)
	}

	var outboxCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from email_outbox o
		join notification_events n on n.email_outbox_id = o.id
		where n.event_id = $1
	`, eventID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != wantOutbox {
		t.Fatalf("unexpected staffing outbox count for event %s: got %d want %d", eventID, outboxCount, wantOutbox)
	}
}

func assertStaffingAssignmentNotificationRecord(t *testing.T, fx lifecycleFixture, staffingID, wantRecipient, wantType, wantKey, eventTitle, staffingTitle, forbiddenText string) {
	t.Helper()
	var notificationType, idempotencyKey, recipientEmail, subject, preview, body string
	if err := fx.app.db.QueryRow(t.Context(), `
		select n.notification_type, n.idempotency_key, n.recipient_email, n.subject, n.preview, o.body
		from notification_events n
		join email_outbox o on o.id = n.email_outbox_id
		where n.related_id = $1
		order by n.created_at desc
		limit 1
	`, staffingID).Scan(&notificationType, &idempotencyKey, &recipientEmail, &subject, &preview, &body); err != nil {
		t.Fatal(err)
	}
	if notificationType != wantType || idempotencyKey != wantKey || recipientEmail != wantRecipient {
		t.Fatalf("unexpected staffing notification identity for %s: type=%q key=%q recipient=%q", staffingID, notificationType, idempotencyKey, recipientEmail)
	}
	if !strings.Contains(subject, eventTitle) || !strings.Contains(subject, staffingTitle) {
		t.Fatalf("notification subject missing event/staffing: %q", subject)
	}
	if preview != "Assignment for "+staffingTitle {
		t.Fatalf("unexpected staffing notification preview: %q", preview)
	}
	if !strings.Contains(body, eventTitle) || !strings.Contains(body, staffingTitle) {
		t.Fatalf("notification body missing event/staffing: %q", body)
	}
	if strings.Contains(subject, forbiddenText) || strings.Contains(preview, forbiddenText) || strings.Contains(body, forbiddenText) {
		t.Fatalf("notification leaked restricted text %q: subject=%q preview=%q body=%q", forbiddenText, subject, preview, body)
	}
}

func assertStaffingReminderCounts(t *testing.T, fx lifecycleFixture, eventID string, wantReminders, wantNotifications, wantOutbox int) {
	t.Helper()
	var reminderCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from reminder_events
		where event_id = $1
		  and source_type = 'staffing'
	`, eventID).Scan(&reminderCount); err != nil {
		t.Fatal(err)
	}
	if reminderCount != wantReminders {
		t.Fatalf("unexpected staffing reminder count for event %s: got %d want %d", eventID, reminderCount, wantReminders)
	}

	var notificationCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from notification_events
		where event_id = $1
		  and notification_type in ('staffing.unassigned', 'staffing.upcoming')
	`, eventID).Scan(&notificationCount); err != nil {
		t.Fatal(err)
	}
	if notificationCount != wantNotifications {
		t.Fatalf("unexpected staffing notification count for event %s: got %d want %d", eventID, notificationCount, wantNotifications)
	}

	var outboxCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*)
		from email_outbox o
		join notification_events n on n.email_outbox_id = o.id
		where n.event_id = $1
		  and n.notification_type in ('staffing.unassigned', 'staffing.upcoming')
	`, eventID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != wantOutbox {
		t.Fatalf("unexpected staffing outbox count for event %s: got %d want %d", eventID, outboxCount, wantOutbox)
	}
}

func assertStaffingReminderNotificationRecord(t *testing.T, fx lifecycleFixture, staffingID, wantRecipient, wantType, wantKey, eventTitle, staffingTitle, startAt, forbiddenText string) {
	t.Helper()
	var notificationType, idempotencyKey, recipientEmail, subject, preview, body string
	if err := fx.app.db.QueryRow(t.Context(), `
		select n.notification_type, n.idempotency_key, n.recipient_email, n.subject, n.preview, o.body
		from notification_events n
		join email_outbox o on o.id = n.email_outbox_id
		where n.related_id = $1
		  and n.notification_type = $2
		order by n.created_at desc
		limit 1
	`, staffingID, wantType).Scan(&notificationType, &idempotencyKey, &recipientEmail, &subject, &preview, &body); err != nil {
		t.Fatal(err)
	}
	if notificationType != wantType || idempotencyKey != wantKey || recipientEmail != wantRecipient {
		t.Fatalf("unexpected staffing reminder identity for %s: type=%q key=%q recipient=%q", staffingID, notificationType, idempotencyKey, recipientEmail)
	}
	if !strings.Contains(subject, eventTitle) || !strings.Contains(subject, staffingTitle) {
		t.Fatalf("notification subject missing event/staffing: %q", subject)
	}
	if !strings.Contains(preview, eventTitle) || !strings.Contains(preview, staffingTitle) || !strings.Contains(preview, startAt) {
		t.Fatalf("notification preview missing event/staffing/time: %q", preview)
	}
	if !strings.Contains(body, eventTitle) || !strings.Contains(body, staffingTitle) || !strings.Contains(body, startAt) {
		t.Fatalf("notification body missing event/staffing/time: %q", body)
	}
	if forbiddenText != "" && (strings.Contains(subject, forbiddenText) || strings.Contains(preview, forbiddenText) || strings.Contains(body, forbiddenText)) {
		t.Fatalf("notification leaked restricted text %q: subject=%q preview=%q body=%q", forbiddenText, subject, preview, body)
	}
}

func TestEventParticipantRosterAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")

	performer := postJSON(t, fx.app, fx.ownerCookie, `/api/events/`+eventID+`/roles`, map[string]any{"name": "Performer", "description": "Play a 20-minute set.", "capacity": 3, "public": true}, http.StatusOK)
	performerRoleID := mustString(t, performer.JSON, "id")
	backstage := postJSON(t, fx.app, fx.ownerCookie, `/api/events/`+eventID+`/roles`, map[string]any{"name": "Backstage", "description": "Private notes.", "capacity": 1, "public": false}, http.StatusOK)
	backstageRoleID := mustString(t, backstage.JSON, "id")
	support := postJSON(t, fx.app, fx.ownerCookie, `/api/events/`+eventID+`/roles`, map[string]any{"name": "Support", "description": "Runner.", "capacity": 1, "public": false}, http.StatusOK)
	supportRoleID := mustString(t, support.JSON, "id")

	backstageApplications := []struct {
		name   string
		email  string
		status string
	}{
		{name: "Alex", email: "alex@example.com", status: "accepted"},
		{name: "Blair", email: "blair@example.com", status: "confirmed"},
	}
	for _, draft := range backstageApplications {
		postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": backstageRoleID, "applicantName": draft.name, "applicantEmail": draft.email}, http.StatusNotFound)
		insertRoleApplicationFixture(t, fx, eventID, backstageRoleID, draft.name, draft.email, draft.status)
	}
	performerParticipant := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": performerRoleID, "applicantName": "Casey", "applicantEmail": "casey@example.com", "message": "Casey message"}, http.StatusOK)
	if _, err := fx.app.db.Exec(t.Context(), `
		update event_role_applications
		set status = 'accepted', updated_at = now()
		where id = $1
	`, mustString(t, performerParticipant.JSON, "id")); err != nil {
		t.Fatal(err)
	}

	otherStatusApplications := []struct {
		name   string
		email  string
		status string
	}{
		{name: "Drew", email: "drew@example.com", status: "rejected"},
		{name: "Evan", email: "evan@example.com", status: "waitlisted"},
	}
	for _, draft := range otherStatusApplications {
		postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/role-applications", map[string]any{"roleId": supportRoleID, "applicantName": draft.name, "applicantEmail": draft.email}, http.StatusNotFound)
		insertRoleApplicationFixture(t, fx, eventID, supportRoleID, draft.name, draft.email, draft.status)
	}

	participantList := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/participants", http.StatusOK)
	memberList := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/participants", http.StatusOK)
	if !reflect.DeepEqual(participantList.JSON, memberList.JSON) {
		t.Fatalf("expected owner/member participant rosters to match: owner=%#v member=%#v", participantList.JSON, memberList.JSON)
	}

	participants := participantList.JSON.([]any)
	if len(participants) != 3 {
		t.Fatalf("expected three participants, got %#v", participantList.JSON)
	}

	first := mustObject(t, participants[0])
	second := mustObject(t, participants[1])
	third := mustObject(t, participants[2])
	if first["roleName"] != "Backstage" || second["roleName"] != "Backstage" || third["roleName"] != "Performer" {
		t.Fatalf("expected rosters grouped by role name, got %#v", participantList.JSON)
	}
	if first["applicantName"] != "Alex" || second["applicantName"] != "Blair" || third["applicantName"] != "Casey" {
		t.Fatalf("expected applicants ordered by name within role, got %#v", participantList.JSON)
	}
	if first["status"] != "accepted" || second["status"] != "confirmed" || third["status"] != "accepted" {
		t.Fatalf("unexpected participant statuses: %#v", participantList.JSON)
	}
	for _, participant := range participants {
		entry := mustObject(t, participant)
		if entry["applicationId"] == "" || entry["roleId"] == "" || entry["roleName"] == "" || entry["applicantName"] == "" || entry["applicantEmail"] == "" || entry["status"] == "" || entry["updatedAt"] == "" {
			t.Fatalf("participant roster missing required fields: %#v", entry)
		}
		if _, ok := entry["message"]; ok {
			t.Fatalf("participant roster must not expose private message text: %#v", entry)
		}
	}

	getJSON(t, fx.app, nil, "/api/events/"+eventID+"/participants", http.StatusUnauthorized)
	otherFx := newLifecycleFixture(t, fx.app)
	getJSON(t, fx.app, otherFx.memberCookie, "/api/events/"+eventID+"/participants", http.StatusForbidden)
}

func TestWorkspaceArchiveIndexAPI(t *testing.T) {
	fx := newLifecycleFixture(t)
	firstEvent := createEvent(t, fx, "Night Market", 4)
	firstEventID := mustString(t, firstEvent, "id")
	publishEvent(t, fx, firstEventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+firstEventID+"/end-of-night", map[string]any{}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+firstEventID+"/archive/notes", map[string]any{"body": "Move doors earlier."}, http.StatusOK)

	secondEvent := createEvent(t, fx, "Late Market", 4)
	secondEventID := mustString(t, secondEvent, "id")
	publishEvent(t, fx, secondEventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+secondEventID+"/end-of-night", map[string]any{}, http.StatusOK)

	seeded := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+firstEventID+"/archive/seed-draft", map[string]any{}, http.StatusOK)
	seededEventID := mustString(t, seeded.JSON, "id")

	resp := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/archives", http.StatusOK)
	archives, ok := resp.JSON.([]any)
	if !ok || len(archives) != 2 {
		t.Fatalf("expected two archive summaries, got %#v", resp.JSON)
	}

	latest := mustObject(t, archives[0])
	older := mustObject(t, archives[1])
	if latest["eventId"] != secondEventID || latest["title"] != "Late Market" || int(latest["noteCount"].(float64)) != 0 {
		t.Fatalf("unexpected latest archive summary: %#v", latest)
	}
	if latest["seededEventId"] != nil {
		t.Fatalf("did not expect seededEventId on latest archive: %#v", latest)
	}
	if latest["id"] == "" || latest["reportId"] == "" || latest["settlementId"] == "" || latest["locationDisplay"] == "" || latest["createdAt"] == "" || latest["updatedAt"] == "" {
		t.Fatalf("latest archive summary missing required fields: %#v", latest)
	}
	if older["eventId"] != firstEventID || older["title"] != "Night Market" || int(older["noteCount"].(float64)) != 1 {
		t.Fatalf("unexpected older archive summary: %#v", older)
	}
	if older["seededEventId"] != seededEventID {
		t.Fatalf("expected seeded draft id in archive summary, got %#v", older["seededEventId"])
	}

	titleSearch := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/archives?q=night", http.StatusOK)
	titleMatches, ok := titleSearch.JSON.([]any)
	if !ok || len(titleMatches) != 1 || mustObject(t, titleMatches[0])["eventId"] != firstEventID {
		t.Fatalf("expected title search to match first archive only, got %#v", titleSearch.JSON)
	}

	noteSearch := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/archives?q=doors", http.StatusOK)
	noteMatches, ok := noteSearch.JSON.([]any)
	if !ok || len(noteMatches) != 1 || mustObject(t, noteMatches[0])["eventId"] != firstEventID {
		t.Fatalf("expected note-body search to match first archive only, got %#v", noteSearch.JSON)
	}
	if _, ok := mustObject(t, noteMatches[0])["notes"]; ok {
		t.Fatalf("workspace archive summaries must not include note bodies: %#v", noteMatches[0])
	}

	missingSearch := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/archives?q=missing", http.StatusOK)
	if matches, ok := missingSearch.JSON.([]any); !ok || len(matches) != 0 {
		t.Fatalf("expected missing search to return empty array, got %#v", missingSearch.JSON)
	}

	percentSearch := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/archives?q=%25", http.StatusOK)
	if matches, ok := percentSearch.JSON.([]any); !ok || len(matches) != 0 {
		t.Fatalf("expected percent search to return empty array, got %#v", percentSearch.JSON)
	}

	underscoreSearch := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/archives?q=_", http.StatusOK)
	if matches, ok := underscoreSearch.JSON.([]any); !ok || len(matches) != 0 {
		t.Fatalf("expected underscore search to return empty array, got %#v", underscoreSearch.JSON)
	}

	other := newLifecycleFixture(t, fx.app)
	otherEvent := createEvent(t, other, "Night Market", 4)
	otherEventID := mustString(t, otherEvent, "id")
	publishEvent(t, other, otherEventID)
	postJSON(t, other.app, other.ownerCookie, "/api/events/"+otherEventID+"/end-of-night", map[string]any{}, http.StatusOK)
	postJSON(t, other.app, other.ownerCookie, "/api/events/"+otherEventID+"/archive/notes", map[string]any{"body": "Move doors earlier."}, http.StatusOK)

	scopedSearch := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/archives?q=night", http.StatusOK)
	if matches, ok := scopedSearch.JSON.([]any); !ok || len(matches) != 1 || mustObject(t, matches[0])["eventId"] != firstEventID {
		t.Fatalf("expected workspace-scoped search to exclude other workspace archives, got %#v", scopedSearch.JSON)
	}

	tooLongQuery := strings.Repeat("a", 121)
	getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/archives?q="+tooLongQuery, http.StatusBadRequest)

	getJSON(t, fx.app, fx.memberCookie, "/api/workspaces/"+fx.workspaceID+"/archives", http.StatusOK)
	getJSON(t, fx.app, nil, "/api/workspaces/"+fx.workspaceID+"/archives", http.StatusUnauthorized)
	getJSON(t, fx.app, other.memberCookie, "/api/workspaces/"+fx.workspaceID+"/archives", http.StatusForbidden)
}

func TestFirstEventLifecycleArchiveNotes(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive/notes", map[string]any{"body": "Move doors earlier."}, http.StatusOK)
	resp := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive/notes", map[string]any{"body": "Keep card reader charged."}, http.StatusOK)
	archive := mustObject(t, resp.JSON)
	if int(archive["noteCount"].(float64)) != 2 {
		t.Fatalf("expected two notes, got %#v", archive)
	}
	notes := archive["notes"].([]any)
	if mustObject(t, notes[0])["body"] != "Move doors earlier." || mustObject(t, notes[1])["body"] != "Keep card reader charged." {
		t.Fatalf("notes not ordered by creation: %#v", notes)
	}

	reloaded := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK)
	reloadedNotes := mustObject(t, reloaded.JSON)["notes"].([]any)
	if len(reloadedNotes) != 2 || mustObject(t, reloadedNotes[0])["body"] != "Move doors earlier." || mustObject(t, reloadedNotes[1])["body"] != "Keep card reader charged." {
		t.Fatalf("GET archive did not return persisted notes in order: %#v", reloadedNotes)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive/notes", map[string]any{"body": "   "}, http.StatusBadRequest)
	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/archive/notes", map[string]any{"body": "member note"}, http.StatusForbidden)
}

func TestFirstEventLifecycleArchiveSeedsDraftWithoutPrivateData(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 40, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive/notes", map[string]any{"body": "Private lesson"}, http.StatusOK)

	startsAt, err := time.Parse(time.RFC3339Nano, mustString(t, event, "startsAt"))
	if err != nil {
		t.Fatal(err)
	}

	seeded := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive/seed-draft", map[string]any{}, http.StatusOK)
	draft := mustObject(t, seeded.JSON)
	draftID := mustString(t, seeded.JSON, "id")
	expectedStartsAt := startsAt.AddDate(0, 0, 7).UTC().Format(time.RFC3339Nano)
	if draftID == eventID || draft["status"] != "draft" || draft["title"] != event["title"] || draft["workspaceId"] != fx.workspaceID || draft["startsAt"] != expectedStartsAt {
		t.Fatalf("unexpected seeded draft: %#v", draft)
	}
	if draft["publicSlug"] != nil || draft["publicUrl"] != nil {
		t.Fatalf("seeded draft must not have public URLs: %#v", draft)
	}
	archive := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive", http.StatusOK).JSON)
	if archive["seededEventId"] != draftID {
		t.Fatalf("expected archive detail to expose seeded draft id, got %#v", archive["seededEventId"])
	}

	seededAgain := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/archive/seed-draft", map[string]any{}, http.StatusOK)
	if mustString(t, seededAgain.JSON, "id") != draftID {
		t.Fatalf("expected seed draft retry to return same event id: first=%s second=%s", draftID, mustString(t, seededAgain.JSON, "id"))
	}

	var workspaceID, title, publicDescription, locationDisplay, pricingMode, ticketCurrency, status, createdByPersonID string
	var ticketAllocation, ticketPriceCents int
	var seededStartsAt time.Time
	var publicSlug sql.NullString
	var publishedAt, endedAt sql.NullTime
	if err := fx.app.db.QueryRow(t.Context(), `
		select workspace_id, title, starts_at, public_description, location_display,
		       ticket_allocation, pricing_mode, ticket_price_cents, ticket_currency,
		       status, public_slug, published_at, ended_at, created_by_person_id
		from events
		where id = $1
	`, draftID).Scan(&workspaceID, &title, &seededStartsAt, &publicDescription, &locationDisplay, &ticketAllocation, &pricingMode, &ticketPriceCents, &ticketCurrency, &status, &publicSlug, &publishedAt, &endedAt, &createdByPersonID); err != nil {
		t.Fatal(err)
	}
	if workspaceID != fx.workspaceID || title != event["title"] || !seededStartsAt.Equal(startsAt.AddDate(0, 0, 7)) || publicDescription != event["publicDescription"].(string) || locationDisplay != event["locationDisplay"].(string) || ticketAllocation != 40 || pricingMode != "fixed" || ticketPriceCents != 1500 || ticketCurrency != "usd" || status != "draft" || publicSlug.Valid || publishedAt.Valid || endedAt.Valid || createdByPersonID != ownerPersonID(t, fx) {
		t.Fatalf("seeded event row mismatch: workspace=%q title=%q startsAt=%s description=%q location=%q allocation=%d pricing=%q price=%d currency=%q status=%q publicSlug=%v publishedAt=%v endedAt=%v createdBy=%q", workspaceID, title, seededStartsAt.Format(time.RFC3339Nano), publicDescription, locationDisplay, ticketAllocation, pricingMode, ticketPriceCents, ticketCurrency, status, publicSlug, publishedAt, endedAt, createdByPersonID)
	}

	var ticketCount, reportCount, settlementCount, archiveCount int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from tickets where event_id = $1`, draftID).Scan(&ticketCount); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_reports where event_id = $1`, draftID).Scan(&reportCount); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_settlements where event_id = $1`, draftID).Scan(&settlementCount); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_archives where event_id = $1`, draftID).Scan(&archiveCount); err != nil {
		t.Fatal(err)
	}
	if ticketCount != 0 || reportCount != 0 || settlementCount != 0 || archiveCount != 0 {
		t.Fatalf("seeded draft copied private rows: tickets=%d reports=%d settlements=%d archives=%d", ticketCount, reportCount, settlementCount, archiveCount)
	}

	var auditAction, auditSubjectType, auditSubjectID, auditEventID, auditNewEventID, auditWorkspaceID string
	if err := fx.app.db.QueryRow(t.Context(), `
		select action, subject_type, coalesce(subject_id::text, ''), metadata->>'eventId', metadata->>'newEventId', metadata->>'workspaceId'
		from audit_entries
		where action = 'archive.seed_draft_created'
		order by created_at desc
		limit 1
	`).Scan(&auditAction, &auditSubjectType, &auditSubjectID, &auditEventID, &auditNewEventID, &auditWorkspaceID); err != nil {
		t.Fatal(err)
	}
	if auditAction != "archive.seed_draft_created" || auditSubjectType != "event_archive" || auditEventID != eventID || auditNewEventID != draftID || auditWorkspaceID != fx.workspaceID {
		t.Fatalf("unexpected audit entry: action=%q subjectType=%q subjectID=%q eventId=%q newEventId=%q workspaceId=%q", auditAction, auditSubjectType, auditSubjectID, auditEventID, auditNewEventID, auditWorkspaceID)
	}

	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/archive/seed-draft", map[string]any{}, http.StatusForbidden)
	postJSON(t, fx.app, nil, "/api/events/"+eventID+"/archive/seed-draft", map[string]any{}, http.StatusUnauthorized)

	otherEvent := createEvent(t, fx, "Second Night", 10)
	otherEventID := mustString(t, otherEvent, "id")
	publishEvent(t, fx, otherEventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+otherEventID+"/archive/seed-draft", map[string]any{}, http.StatusNotFound)
}

func TestFirstEventLifecycleSettlementAdjustments(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 4, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("paid"), "Paid Guest", "paid", 1500, "usd")
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)

	first := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement/adjustments", map[string]any{
		"amountCents": 500,
		"label":       "Donation",
		"reason":      "  community support  ",
	}, http.StatusOK)
	firstSettlement := mustObject(t, first.JSON)
	if int(firstSettlement["adjustmentTotalCents"].(float64)) != 500 || int(firstSettlement["netTotalCents"].(float64)) != 2000 {
		t.Fatalf("unexpected first adjustment totals: %#v", firstSettlement)
	}
	firstAdjustments := firstSettlement["adjustments"].([]any)
	if len(firstAdjustments) != 1 {
		t.Fatalf("expected one adjustment, got %#v", firstAdjustments)
	}
	firstAdjustment := mustObject(t, firstAdjustments[0])
	if firstAdjustment["amountCents"] != float64(500) || firstAdjustment["label"] != "Donation" || firstAdjustment["reason"] != "community support" || firstAdjustment["createdByPersonId"] != ownerPersonID(t, fx) {
		t.Fatalf("unexpected first adjustment row: %#v", firstAdjustment)
	}
	if firstAdjustment["settlementId"] == "" || firstAdjustment["createdAt"] == "" || firstAdjustment["id"] == "" {
		t.Fatalf("expected first adjustment identifiers: %#v", firstAdjustment)
	}

	time.Sleep(10 * time.Millisecond)
	second := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement/adjustments", map[string]any{
		"amountCents": -200,
		"label":       "Refund",
		"reason":      "oops",
	}, http.StatusOK)
	secondSettlement := mustObject(t, second.JSON)
	if int(secondSettlement["adjustmentTotalCents"].(float64)) != 300 || int(secondSettlement["netTotalCents"].(float64)) != 1800 {
		t.Fatalf("unexpected second adjustment totals: %#v", secondSettlement)
	}
	secondAdjustments := secondSettlement["adjustments"].([]any)
	if len(secondAdjustments) != 2 {
		t.Fatalf("expected two adjustments, got %#v", secondAdjustments)
	}
	if mustObject(t, secondAdjustments[0])["label"] != "Donation" || mustObject(t, secondAdjustments[1])["label"] != "Refund" {
		t.Fatalf("expected adjustments sorted by createdAt asc: %#v", secondAdjustments)
	}
	if int(mustObject(t, secondAdjustments[1])["amountCents"].(float64)) != -200 {
		t.Fatalf("unexpected second adjustment row: %#v", secondAdjustments[1])
	}

	getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/settlement", http.StatusForbidden)
}

func TestFirstEventLifecycleSettlementAdjustmentValidation(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 2)
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("paid"), "Paid Guest", "paid", 1500, "usd")
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement/adjustments", map[string]any{"amountCents": 0, "label": "Zero"}, http.StatusBadRequest)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement/adjustments", map[string]any{"amountCents": 100, "label": "   "}, http.StatusBadRequest)
}

func TestFirstEventLifecycleSettlementAdjustmentRequiresSettlement(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 2)
	eventID := mustString(t, event, "id")
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/settlement/adjustments", map[string]any{"amountCents": 100, "label": "Donation"}, http.StatusNotFound)
}

func TestFirstEventLifecycleEndOfNightRejectsPendingPaidTickets(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("pending"), "Pending Guest", "pending", 1500, "usd")

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusConflict)
}

func TestFirstEventLifecyclePaidReportSettlementSummaryIsIdempotent(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 6, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("paid-a"), "Paid A", "paid", 1500, "usd")
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("paid-b"), "Paid B", "paid", 1500, "usd")
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("cancelled"), "Cancelled Guest", "cancelled", 1500, "usd")
	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("free"), "Free Guest", "free", 0, "usd")

	first := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	firstSummary := mustObject(t, mustObject(t, first.JSON)["settlementSummary"])
	if firstSummary["currency"] != "usd" || int(firstSummary["grossPaidRevenueCents"].(float64)) != 3000 || int(firstSummary["paidTicketCount"].(float64)) != 2 || int(firstSummary["pendingTicketCount"].(float64)) != 0 || int(firstSummary["cancelledTicketCount"].(float64)) != 1 || int(firstSummary["freeTicketCount"].(float64)) != 1 || int(firstSummary["reservedCount"].(float64)) != 3 {
		t.Fatalf("unexpected paid settlement summary: %#v", firstSummary)
	}

	type settlementRow struct {
		Currency              string
		GrossPaidRevenueCents int
		PaidTicketCount       int
		PendingTicketCount    int
		CancelledTicketCount  int
		FreeTicketCount       int
		ReservedCount         int
		Status                string
		GeneratedByPersonID   string
		GeneratedAt           time.Time
		CreatedAt             time.Time
		UpdatedAt             time.Time
	}
	settlement := settlementRow{}
	if err := fx.app.db.QueryRow(t.Context(), `
		select currency, gross_paid_revenue_cents, paid_ticket_count, pending_ticket_count,
		       cancelled_ticket_count, free_ticket_count, reserved_count, status,
		       generated_by_person_id, generated_at, created_at, updated_at
		from event_settlements
		where event_id = $1
	`, eventID).Scan(&settlement.Currency, &settlement.GrossPaidRevenueCents, &settlement.PaidTicketCount, &settlement.PendingTicketCount, &settlement.CancelledTicketCount, &settlement.FreeTicketCount, &settlement.ReservedCount, &settlement.Status, &settlement.GeneratedByPersonID, &settlement.GeneratedAt, &settlement.CreatedAt, &settlement.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if settlement.Currency != "usd" || settlement.GrossPaidRevenueCents != 3000 || settlement.PaidTicketCount != 2 || settlement.PendingTicketCount != 0 || settlement.CancelledTicketCount != 1 || settlement.FreeTicketCount != 1 || settlement.ReservedCount != 3 || settlement.Status != "open" || settlement.GeneratedByPersonID != ownerPersonID(t, fx) {
		t.Fatalf("unexpected settlement row: %#v", settlement)
	}

	insertTicketWithPaymentStatus(t, fx, eventID, fx.email("late-paid"), "Late Paid", "paid", 9999, "usd")
	second := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	if !reflect.DeepEqual(first.JSON, second.JSON) {
		t.Fatalf("expected end-of-night report to be idempotent: first=%#v second=%#v", first.JSON, second.JSON)
	}

	settlementAfter := settlementRow{}
	if err := fx.app.db.QueryRow(t.Context(), `
		select currency, gross_paid_revenue_cents, paid_ticket_count, pending_ticket_count,
		       cancelled_ticket_count, free_ticket_count, reserved_count, status,
		       generated_by_person_id, generated_at, created_at, updated_at
		from event_settlements
		where event_id = $1
	`, eventID).Scan(&settlementAfter.Currency, &settlementAfter.GrossPaidRevenueCents, &settlementAfter.PaidTicketCount, &settlementAfter.PendingTicketCount, &settlementAfter.CancelledTicketCount, &settlementAfter.FreeTicketCount, &settlementAfter.ReservedCount, &settlementAfter.Status, &settlementAfter.GeneratedByPersonID, &settlementAfter.GeneratedAt, &settlementAfter.CreatedAt, &settlementAfter.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(settlement, settlementAfter) {
		t.Fatalf("expected settlement row to remain unchanged: first=%#v second=%#v", settlement, settlementAfter)
	}
}

func TestFirstEventLifecycleCreatesArchiveAtEndOfNight(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 4)
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)

	var archiveID string
	var status string
	var noteCount int
	if err := fx.app.db.QueryRow(t.Context(), `
		select id, status, note_count
		from event_archives
		where event_id = $1
	`, eventID).Scan(&archiveID, &status, &noteCount); err != nil {
		t.Fatal(err)
	}
	if archiveID == "" || status != "private" || noteCount != 0 {
		t.Fatalf("unexpected archive row: id=%q status=%q noteCount=%d", archiveID, status, noteCount)
	}

	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_archives where event_id = $1`, eventID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one archive row, got %d", count)
	}
}

func TestFirstEventLifecycleOldReportSnapshotOmitsSettlementSummary(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 2)
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	ownerID := ownerPersonID(t, fx)
	oldSnapshot := map[string]any{
		"id":                     "legacy-report",
		"eventId":                eventID,
		"title":                  "Night Market",
		"startsAt":               event["startsAt"],
		"publicUrl":              published["publicUrl"],
		"ticketAllocation":       2,
		"ticketsReserved":        0,
		"ticketsCheckedIn":       0,
		"noShows":                0,
		"generatedAt":            time.Now().UTC().Format(time.RFC3339Nano),
		"generatedByMemberEmail": fx.email("owner"),
	}
	payload, err := json.Marshal(oldSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_reports (event_id, generated_by_person_id, snapshot)
		values ($1, $2, $3)
	`, eventID, ownerID, payload); err != nil {
		t.Fatal(err)
	}

	first := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	reportObj := mustObject(t, first.JSON)
	if _, ok := reportObj["settlementSummary"]; ok {
		t.Fatalf("legacy snapshot should not synthesize settlement summary: %#v", reportObj)
	}

	second := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	if !reflect.DeepEqual(first.JSON, second.JSON) {
		t.Fatalf("expected legacy end-of-night retry to return same snapshot: first=%#v second=%#v", first.JSON, second.JSON)
	}

	var archiveCount, settlementCount int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_archives where event_id = $1`, eventID).Scan(&archiveCount); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_settlements where event_id = $1`, eventID).Scan(&settlementCount); err != nil {
		t.Fatal(err)
	}
	if archiveCount != 0 || settlementCount != 0 {
		t.Fatalf("legacy retry should not require archive or settlement: archives=%d settlements=%d", archiveCount, settlementCount)
	}
}

func TestReadyWithDatabase(t *testing.T) {
	fx := newLifecycleFixture(t)
	getJSON(t, fx.app, nil, "/api/ready", http.StatusOK)
}

func TestFirstEventLifecycleFixedPriceCreate(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	if event["pricingMode"] != "fixed" || int(event["ticketPriceCents"].(float64)) != 1500 || event["ticketCurrency"] != "usd" {
		t.Fatalf("unexpected pricing response: %#v", event)
	}
}

func TestFirstEventLifecycleCurrentCreatePublishFreeDoorEndOfNightFlow(t *testing.T) {
	fx := newLifecycleFixture(t)
	assertLocalEventLifecycle(t, fx)
}

func TestATProviderFailureDoesNotInterruptLocalEventLifecycle(t *testing.T) {
	fx := newLifecycleFixture(t)
	flow := &fakeATProtoLinkFlow{startErr: errors.New("synthetic provider unavailable")}
	fx.app.config.ATProtoOAuthEnabled = true
	fx.app.atprotoFlow = flow
	postJSON(t, fx.app, fx.ownerCookie, "/api/v1/auth/atproto/start", map[string]any{"identifier": "alice.example"}, http.StatusBadGateway)
	assertLocalEventLifecycle(t, fx)
	if flow.startCalls != 1 {
		t.Fatalf("local event work unexpectedly called the AT provider: calls=%d", flow.startCalls)
	}
	var links int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from did_links`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Fatalf("failed provider created %d identity links", links)
	}
}

func assertLocalEventLifecycle(t *testing.T, fx lifecycleFixture) {
	t.Helper()
	event := createEvent(t, fx, "Night Market", 2)
	eventID := mustString(t, event, "id")
	slug := mustString(t, publishEvent(t, fx, eventID), "publicSlug")

	reservation := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusOK)
	if mustString(t, reservation.JSON, "status") != "reserved" {
		t.Fatalf("unexpected reservation response: %#v", reservation.JSON)
	}

	code := mustString(t, reservation.JSON, "code")
	grantFixtureDoorPermission(t, fx)
	firstCheckIn := postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": code}, http.StatusOK)
	secondCheckIn := postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": code}, http.StatusOK)
	first := mustObject(t, firstCheckIn.JSON)
	second := mustObject(t, secondCheckIn.JSON)
	if first["status"] != "checked_in" || second["status"] != "checked_in" || first["checkedInAt"] != second["checkedInAt"] {
		t.Fatalf("unexpected duplicate check-in behavior: first=%#v second=%#v", firstCheckIn.JSON, secondCheckIn.JSON)
	}

	report := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	reportObj := mustObject(t, report.JSON)
	if int(reportObj["ticketsReserved"].(float64)) != 1 || int(reportObj["ticketsCheckedIn"].(float64)) != 1 || int(reportObj["noShows"].(float64)) != 0 {
		t.Fatalf("unexpected end-of-night report: %#v", report.JSON)
	}

	var status string
	if err := fx.app.db.QueryRow(t.Context(), `select status from events where id = $1`, eventID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "end_of_night" {
		t.Fatalf("expected event to close out, got %s", status)
	}

	var settlementCount, archiveCount int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_settlements where event_id = $1`, eventID).Scan(&settlementCount); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_archives where event_id = $1`, eventID).Scan(&archiveCount); err != nil {
		t.Fatal(err)
	}
	if settlementCount != 1 || archiveCount != 1 {
		t.Fatalf("expected one settlement and one archive, got settlements=%d archives=%d", settlementCount, archiveCount)
	}
}

func TestTicketReservationCurrentCapacityAndDoorRules(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 1)
	eventID := mustString(t, event, "id")
	slug := mustString(t, publishEvent(t, fx, eventID), "publicSlug")
	const publicImageURL = "https://media.example.test/night-market.jpg"
	if _, err := fx.app.db.Exec(t.Context(), `update events set image_url = $1 where id = $2`, publicImageURL, eventID); err != nil {
		t.Fatal(err)
	}

	firstReservation := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest-a"), "displayName": "Guest One"}, http.StatusOK)
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest-b"), "displayName": "Guest Two"}, http.StatusConflict)

	code := mustString(t, firstReservation.JSON, "code")
	grantFixtureDoorPermission(t, fx)
	firstCheckIn := postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": code}, http.StatusOK)
	secondCheckIn := postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": code}, http.StatusOK)
	if mustString(t, firstCheckIn.JSON, "status") != "checked_in" || mustString(t, secondCheckIn.JSON, "status") != "checked_in" || mustString(t, firstCheckIn.JSON, "checkedInAt") != mustString(t, secondCheckIn.JSON, "checkedInAt") {
		t.Fatalf("unexpected duplicate check-in response: first=%#v second=%#v", firstCheckIn.JSON, secondCheckIn.JSON)
	}

	publicEvent := getJSON(t, fx.app, nil, "/api/public/events/"+slug, http.StatusOK).JSON.(map[string]any)
	if int(publicEvent["remainingTickets"].(float64)) != 0 || !publicEvent["isFull"].(bool) {
		t.Fatalf("expected event to stay full after one reservation: %#v", publicEvent)
	}
	if publicEvent["imageUrl"] != publicImageURL {
		t.Fatalf("expected public event image parity, got %#v", publicEvent["imageUrl"])
	}
	for _, forbidden := range []string{"workspaceId", "ticketAllocation", "reservedCount", "checkedInCount", "staffingOpenCount", "staffingAssignedCount", "staffingCompletedCount", "staffingCancelledCount", "staffingItems", "settlement", "archive", "notes"} {
		if _, ok := publicEvent[forbidden]; ok {
			t.Fatalf("public event detail leaked operational field %q: %#v", forbidden, publicEvent)
		}
	}
}

func TestFirstEventLifecycleRejectsLowFixedPrice(t *testing.T) {
	fx := newLifecycleFixture(t)
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/events", map[string]any{
		"title":             "Night Market",
		"startsAt":          "2026-07-01T20:00:00Z",
		"publicDescription": "Free community event.",
		"locationDisplay":   "Warehouse District",
		"ticketAllocation":  2,
		"pricingMode":       "fixed",
		"ticketPriceCents":  49,
		"ticketCurrency":    "usd",
	}, http.StatusBadRequest)
}

func TestFirstEventLifecycleRejectsPricingChangeAfterReservation(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 1)
	eventID := mustString(t, event, "id")
	slug := mustString(t, publishEvent(t, fx, eventID), "publicSlug")
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusOK)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID, map[string]any{
		"pricingMode":      "fixed",
		"ticketPriceCents": 1500,
		"ticketCurrency":   "usd",
	}, http.StatusConflict)
}

func TestFirstEventLifecycleFixedPriceRequiresPaidCheckout(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	slug := mustString(t, publishEvent(t, fx, mustString(t, event, "id")), "publicSlug")
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusConflict)
}

func TestPaidReservationWithoutPaymentProviderReturns503(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	slug := mustString(t, publishEvent(t, fx, mustString(t, event, "id")), "publicSlug")
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusServiceUnavailable)
}

func TestPaidReservationCreatesPendingTicketAndCheckoutSession(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	fake := &fakePaymentProvider{response: checkoutSessionResponse{ID: "cs_test_" + strings.ReplaceAll(eventID, "-", ""), URL: "https://checkout.example/session"}}
	fx.app.payments = fake
	slug := mustString(t, publishEvent(t, fx, eventID), "publicSlug")
	email := fx.email("guest")
	resp := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", map[string]any{"email": email, "displayName": "Guest"}, http.StatusOK)
	if mustString(t, resp.JSON, "checkoutSessionId") != fake.response.ID || mustString(t, resp.JSON, "checkoutUrl") != "https://checkout.example/session" {
		t.Fatalf("unexpected checkout response: %#v", resp.JSON)
	}

	if fake.request.TicketID == "" || fake.request.EventID != eventID || fake.request.EventTitle != "Night Market" || fake.request.AmountCents != 1500 || fake.request.Currency != "usd" {
		t.Fatalf("unexpected provider request: %#v", fake.request)
	}
	if !strings.Contains(fake.request.SuccessURL, "/tickets/") || !strings.Contains(fake.request.SuccessURL, "checkout=success") {
		t.Fatalf("unexpected success url: %s", fake.request.SuccessURL)
	}
	if !strings.Contains(fake.request.CancelURL, "/e/") || !strings.Contains(fake.request.CancelURL, "checkout=cancelled") {
		t.Fatalf("unexpected cancel url: %s", fake.request.CancelURL)
	}

	var paymentStatus, currency, sessionID, status string
	var amountCents int
	if err := fx.app.db.QueryRow(t.Context(), `
		select payment_status, amount_cents, currency, stripe_checkout_session_id, status
		from tickets
		where event_id = $1 and email = $2
	`, eventID, email).Scan(&paymentStatus, &amountCents, &currency, &sessionID, &status); err != nil {
		t.Fatal(err)
	}
	if paymentStatus != "pending" || amountCents != 1500 || currency != "usd" || sessionID != fake.response.ID || status != "reserved" {
		t.Fatalf("unexpected ticket state: paymentStatus=%s amount=%d currency=%s session=%s status=%s", paymentStatus, amountCents, currency, sessionID, status)
	}
}

func TestFreeReservationStillWorksForFreeEvent(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 2)
	slug := mustString(t, publishEvent(t, fx, mustString(t, event, "id")), "publicSlug")
	resp := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusOK)
	if mustString(t, resp.JSON, "status") != "reserved" {
		t.Fatalf("unexpected reservation response: %#v", resp.JSON)
	}
}

func TestStripeWebhookFulfillmentMarksTicketPaidAndEnqueuesEmail(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	ticketID, sessionID := insertPendingStripeTicket(t, fx, eventID, "guest@example.test", "Guest", testStripeSessionID(t, "cs_test_fulfillment"))
	stripeEventID := testStripeEventID(t, "evt_fulfillment")

	eventPayload := stripeCheckoutSessionEvent(t, stripe.EventTypeCheckoutSessionCompleted, stripeEventID, sessionID, ticketID, eventID, stripe.CheckoutSessionPaymentStatusPaid, 1500, "usd")
	if err := fx.app.processStripeWebhookEvent(t.Context(), eventPayload); err != nil {
		t.Fatal(err)
	}

	assertPaidTicketState(t, fx, ticketID, sessionID)
	assertEmailOutboxCount(t, fx, ticketID, 1)
	assertWebhookEventCount(t, fx, stripeEventID, 1)
}

func TestStripeWebhookFulfillmentIsIdempotentForDuplicateEventID(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	ticketID, sessionID := insertPendingStripeTicket(t, fx, eventID, "guest@example.test", "Guest", testStripeSessionID(t, "cs_test_idempotent"))
	stripeEventID := testStripeEventID(t, "evt_duplicate")

	eventPayload := stripeCheckoutSessionEvent(t, stripe.EventTypeCheckoutSessionCompleted, stripeEventID, sessionID, ticketID, eventID, stripe.CheckoutSessionPaymentStatusPaid, 1500, "usd")
	if err := fx.app.processStripeWebhookEvent(t.Context(), eventPayload); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.processStripeWebhookEvent(t.Context(), eventPayload); err != nil {
		t.Fatal(err)
	}

	assertPaidTicketState(t, fx, ticketID, sessionID)
	assertEmailOutboxCount(t, fx, ticketID, 1)
	assertWebhookEventCount(t, fx, stripeEventID, 1)
}

func TestStripeWebhookCompletedRejectsMismatchedPaymentDetails(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	cases := []struct {
		name          string
		paymentStatus stripe.CheckoutSessionPaymentStatus
		amountTotal   int64
		currency      string
	}{
		{name: "unpaid-status", paymentStatus: stripe.CheckoutSessionPaymentStatusUnpaid, amountTotal: 1500, currency: "usd"},
		{name: "wrong-amount", paymentStatus: stripe.CheckoutSessionPaymentStatusPaid, amountTotal: 1600, currency: "usd"},
		{name: "wrong-currency", paymentStatus: stripe.CheckoutSessionPaymentStatusPaid, amountTotal: 1500, currency: "eur"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stripeEventID := testStripeEventID(t, "evt_"+tc.name)
			ticketID, sessionID := insertPendingStripeTicket(t, fx, eventID, "guest+"+tc.name+"@example.test", "Guest "+tc.name, testStripeSessionID(t, "cs_test_"+tc.name))
			eventPayload := stripeCheckoutSessionEvent(t, stripe.EventTypeCheckoutSessionCompleted, stripeEventID, sessionID, ticketID, eventID, tc.paymentStatus, tc.amountTotal, tc.currency)
			if err := fx.app.processStripeWebhookEvent(t.Context(), eventPayload); err != nil {
				t.Fatal(err)
			}

			var paymentStatus string
			var paidAt sql.NullTime
			if err := fx.app.db.QueryRow(t.Context(), `select payment_status, paid_at from tickets where id = $1`, ticketID).Scan(&paymentStatus, &paidAt); err != nil {
				t.Fatal(err)
			}
			if paymentStatus != "pending" || paidAt.Valid {
				t.Fatalf("ticket should remain pending for %s: status=%s paidAt=%v", tc.name, paymentStatus, paidAt)
			}
			assertEmailOutboxCount(t, fx, ticketID, 0)
			assertWebhookEventCount(t, fx, stripeEventID, 1)
		})
	}
}

func TestStripeWebhookWrongSessionIDDoesNotMarkTicketPaid(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	ticketID, _ := insertPendingStripeTicket(t, fx, eventID, "guest@example.test", "Guest", testStripeSessionID(t, "cs_test_expected"))
	stripeEventID := testStripeEventID(t, "evt_wrong_session")

	eventPayload := stripeCheckoutSessionEvent(t, stripe.EventTypeCheckoutSessionCompleted, stripeEventID, testStripeSessionID(t, "cs_test_wrong"), ticketID, eventID, stripe.CheckoutSessionPaymentStatusPaid, 1500, "usd")
	if err := fx.app.processStripeWebhookEvent(t.Context(), eventPayload); err != nil {
		t.Fatal(err)
	}

	var paymentStatus string
	var paidAt sql.NullTime
	if err := fx.app.db.QueryRow(t.Context(), `select payment_status, paid_at from tickets where id = $1`, ticketID).Scan(&paymentStatus, &paidAt); err != nil {
		t.Fatal(err)
	}
	if paymentStatus != "pending" || paidAt.Valid {
		t.Fatalf("ticket should remain pending on session mismatch: status=%s paidAt=%v", paymentStatus, paidAt)
	}
	assertEmailOutboxCount(t, fx, ticketID, 0)
	assertWebhookEventCount(t, fx, stripeEventID, 1)
}

func TestStripeWebhookExpiredCancelsPendingTicketAndReleasesCapacity(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 1, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")
	ticketID, sessionID := insertPendingStripeTicket(t, fx, eventID, "guest@example.test", "Guest", testStripeSessionID(t, "cs_test_expired"))
	stripeEventID := testStripeEventID(t, "evt_expired")

	publicBefore := getJSON(t, fx.app, nil, "/api/public/events/"+slug, http.StatusOK).JSON.(map[string]any)
	if int(publicBefore["remainingTickets"].(float64)) != 0 {
		t.Fatalf("expected pending ticket to consume capacity before expiration: %#v", publicBefore)
	}

	eventPayload := stripeCheckoutSessionEvent(t, stripe.EventTypeCheckoutSessionExpired, stripeEventID, sessionID, ticketID, eventID, stripe.CheckoutSessionPaymentStatusUnpaid, 1500, "usd")
	if err := fx.app.processStripeWebhookEvent(t.Context(), eventPayload); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.processStripeWebhookEvent(t.Context(), eventPayload); err != nil {
		t.Fatal(err)
	}

	var paymentStatus string
	if err := fx.app.db.QueryRow(t.Context(), `select payment_status from tickets where id = $1`, ticketID).Scan(&paymentStatus); err != nil {
		t.Fatal(err)
	}
	if paymentStatus != "cancelled" {
		t.Fatalf("expected expired ticket to be cancelled, got %s", paymentStatus)
	}
	publicAfter := getJSON(t, fx.app, nil, "/api/public/events/"+slug, http.StatusOK).JSON.(map[string]any)
	if int(publicAfter["remainingTickets"].(float64)) != 1 || publicAfter["isFull"].(bool) != false {
		t.Fatalf("expected capacity to be released after expiration: %#v", publicAfter)
	}
	assertWebhookEventCount(t, fx, stripeEventID, 1)
}

func TestStripeWebhookCompletedDoesNotFulfillAfterEndOfNight(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	ticketID, sessionID := insertPendingStripeTicket(t, fx, eventID, "guest@example.test", "Guest", testStripeSessionID(t, "cs_test_after_close"))
	stripeEventID := testStripeEventID(t, "evt_after_close")

	eventPayload := stripeCheckoutSessionEvent(t, stripe.EventTypeCheckoutSessionCompleted, stripeEventID, sessionID, ticketID, eventID, stripe.CheckoutSessionPaymentStatusPaid, 1500, "usd")
	if err := fx.app.processStripeWebhookEvent(t.Context(), eventPayload); err != nil {
		t.Fatal(err)
	}

	var paymentStatus string
	var paidAt sql.NullTime
	if err := fx.app.db.QueryRow(t.Context(), `select payment_status, paid_at from tickets where id = $1`, ticketID).Scan(&paymentStatus, &paidAt); err != nil {
		t.Fatal(err)
	}
	if paymentStatus != "pending" || paidAt.Valid {
		t.Fatalf("ticket should remain pending after close: status=%s paidAt=%v", paymentStatus, paidAt)
	}
	assertEmailOutboxCount(t, fx, ticketID, 0)
	assertWebhookEventCount(t, fx, stripeEventID, 1)
}

func TestDoorTicketSearchExcludesPendingPaidTicket(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	published := publishEvent(t, fx, eventID)
	slug := mustString(t, published, "publicSlug")
	guestEmail := fx.email("guest-search")
	insertPendingStripeTicket(t, fx, eventID, guestEmail, "Guest Search", testStripeSessionID(t, "cs_test_search"))

	grantFixtureDoorPermission(t, fx)
	search := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/tickets?query=guest", http.StatusOK)
	if len(search.JSON.([]any)) != 0 {
		t.Fatalf("expected pending paid ticket to be excluded from search: %#v", search.JSON)
	}

	publicEvent := getJSON(t, fx.app, nil, "/api/public/events/"+slug, http.StatusOK).JSON.(map[string]any)
	if int(publicEvent["remainingTickets"].(float64)) != 1 {
		t.Fatalf("expected pending ticket to count toward capacity before expiry: %#v", publicEvent)
	}
}

func TestDoorCheckInRejectsPendingPaidTicket(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Night Market", 2, "fixed", 1500, "usd")
	eventID := mustString(t, event, "id")
	publishEvent(t, fx, eventID)
	ticketID, _ := insertPendingStripeTicket(t, fx, eventID, fx.email("guest-checkin"), "Guest Checkin", testStripeSessionID(t, "cs_test_checkin"))
	var code string
	if err := fx.app.db.QueryRow(t.Context(), `select code from tickets where id = $1`, ticketID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	grantFixtureDoorPermission(t, fx)
	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": code}, http.StatusConflict)

	var paymentStatus string
	if err := fx.app.db.QueryRow(t.Context(), `select payment_status from tickets where id = $1`, ticketID).Scan(&paymentStatus); err != nil {
		t.Fatal(err)
	}
	if paymentStatus != "pending" {
		t.Fatalf("expected pending ticket to remain pending after rejected check-in, got %s", paymentStatus)
	}
}

func TestFirstEventLifecycleFullCapacity(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 1)
	eventID := mustString(t, event, "id")
	slug := mustString(t, publishEvent(t, fx, eventID), "publicSlug")

	firstGuest := fx.email("guest-a")
	secondGuest := fx.email("guest-b")
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": firstGuest, "displayName": "Guest One"}, http.StatusOK)
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": secondGuest, "displayName": "Guest Two"}, http.StatusConflict)

	publicEvent := getJSON(t, fx.app, nil, "/api/public/events/"+slug, http.StatusOK)
	if int(publicEvent.JSON.(map[string]any)["remainingTickets"].(float64)) != 0 || publicEvent.JSON.(map[string]any)["isFull"].(bool) != true {
		t.Fatalf("event should remain full with one reservation: %#v", publicEvent.JSON)
	}
}

func TestFirstEventLifecycleDuplicateCheckIn(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 1)
	eventID := mustString(t, event, "id")
	slug := mustString(t, publishEvent(t, fx, eventID), "publicSlug")
	ticket := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusOK)
	code := mustString(t, ticket.JSON, "code")
	grantFixtureDoorPermission(t, fx)
	firstCheckIn := postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": code}, http.StatusOK)
	secondCheckIn := postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": code}, http.StatusOK)

	first := firstCheckIn.JSON.(map[string]any)
	second := secondCheckIn.JSON.(map[string]any)
	if first["status"] != "checked_in" || second["status"] != "checked_in" {
		t.Fatalf("expected checked-in ticket: first=%#v second=%#v", firstCheckIn.JSON, secondCheckIn.JSON)
	}
	if first["checkedInAt"] != second["checkedInAt"] {
		t.Fatalf("expected duplicate check-in to return same checkedInAt, got %v and %v", first["checkedInAt"], second["checkedInAt"])
	}
}

func TestFirstEventLifecycleDraftReservationsBlocked(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 1)
	eventID := mustString(t, event, "id")
	if event["publicSlug"] != nil {
		t.Fatalf("draft event should not have a public slug: %#v", event["publicSlug"])
	}

	getJSON(t, fx.app, nil, "/api/public/events/"+eventID, http.StatusNotFound)
	postJSON(t, fx.app, nil, "/api/public/events/"+eventID+"/reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusNotFound)
}

func TestFirstEventLifecyclePermissions(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEvent(t, fx, "Night Market", 1)
	eventID := mustString(t, event, "id")

	updated := patchJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID, map[string]any{"title": "Night Market Updated"}, http.StatusOK)
	if mustString(t, updated.JSON, "title") != "Night Market Updated" {
		t.Fatalf("member update did not persist: %#v", updated.JSON)
	}

	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/publish", map[string]any{}, http.StatusForbidden)
	published := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/publish", map[string]any{}, http.StatusOK)
	slug := mustString(t, published.JSON, "publicSlug")
	guestTicket := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/reservations", map[string]any{"email": fx.email("guest"), "displayName": "Guest"}, http.StatusOK)
	grantFixtureDoorPermission(t, fx)
	search := getJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/tickets?query=guest", http.StatusOK)
	tickets := search.JSON.([]any)
	if len(tickets) != 1 {
		t.Fatalf("expected one ticket in door search, got %#v", search.JSON)
	}
	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/door/check-ins", map[string]any{"code": mustString(t, guestTicket.JSON, "code")}, http.StatusOK)
	postJSON(t, fx.app, fx.memberCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusForbidden)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/end-of-night", map[string]any{}, http.StatusOK)
}

func TestDevEmailOutbox(t *testing.T) {
	fx := newLifecycleFixture(t)
	inviteEmail := fx.email("invitee")
	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/invitations", map[string]any{"email": inviteEmail}, http.StatusOK)

	outbox := getJSON(t, fx.app, fx.ownerCookie, "/api/dev/email-outbox", http.StatusOK).JSON.([]any)
	if len(outbox) == 0 {
		t.Fatal("expected at least one email in outbox")
	}

	matched := false
	for _, item := range outbox {
		msg := mustObject(t, item)
		if msg["recipientEmail"] == inviteEmail && strings.Contains(msg["body"].(string), "/invite/") {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("expected invitation email for %s in outbox: %#v", inviteEmail, outbox)
	}
}

func TestDevEmailOutboxUnauthorized(t *testing.T) {
	fx := newLifecycleFixture(t)
	getJSON(t, fx.app, nil, "/api/dev/email-outbox", http.StatusUnauthorized)
}

func TestDevEmailOutboxProductionHidden(t *testing.T) {
	fx := newLifecycleFixture(t)
	prodApp := New(Config{AppEnv: "production", PublicWebURL: "http://public.test", SessionSecret: "test-secret"}, fx.app.db)
	getJSON(t, prodApp, fx.ownerCookie, "/api/dev/email-outbox", http.StatusNotFound)
}

func TestGetSpecificWorkspace(t *testing.T) {
	fx := newLifecycleFixture(t)
	second := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces", map[string]any{"name": "Second Room"}, http.StatusOK)
	secondID := mustString(t, second.JSON, "id")

	loaded := getJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+secondID, http.StatusOK).JSON
	if mustString(t, loaded, "id") != secondID || mustString(t, loaded, "name") != "Second Room" {
		t.Fatalf("unexpected workspace response: %#v", loaded)
	}

	otherCookie := signupAndVerifyCookie(t, fx.app, fx.email("other"), "Other")
	getJSON(t, fx.app, otherCookie, "/api/workspaces/"+secondID, http.StatusForbidden)
}

func TestSignupStoresBcryptPasswordHash(t *testing.T) {
	fx := newLifecycleFixture(t)
	email := fx.email("bcrypt")
	postJSON(t, fx.app, nil, "/api/auth/signup", map[string]any{"email": email, "password": "secret1234", "displayName": "Hash"}, http.StatusAccepted)

	var storedHash string
	if err := fx.app.db.QueryRow(t.Context(), `select password_hash from people where email = $1`, email).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(storedHash, passwordScheme+"$") {
		t.Fatalf("expected bcrypt password hash, got %q", storedHash)
	}
	valid, upgraded := verifyPassword("secret1234", storedHash)
	if !valid || upgraded != "" {
		t.Fatalf("expected bcrypt password to verify without upgrade, valid=%v upgraded=%q", valid, upgraded)
	}
}

func TestOriginGuardRejectsCrossSiteCookieMutations(t *testing.T) {
	fx := newLifecycleFixture(t)
	postJSONWithOrigin(t, fx.app, fx.ownerCookie, "/api/workspaces", map[string]any{"name": "Evil"}, "http://evil.example", http.StatusForbidden)
	postJSONWithOrigin(t, fx.app, fx.ownerCookie, "/api/workspaces", map[string]any{"name": "Allowed"}, "http://public.test", http.StatusOK)
}

func TestLoginRateLimit(t *testing.T) {
	fx := newLifecycleFixture(t)
	email := fx.email("missing")
	for range maxLoginFailures {
		postJSON(t, fx.app, nil, "/api/auth/login", map[string]any{"email": email, "password": "wrong-password"}, http.StatusUnauthorized)
	}
	resp := postJSON(t, fx.app, nil, "/api/auth/login", map[string]any{"email": email, "password": "wrong-password"}, http.StatusTooManyRequests)
	if !strings.Contains(resp.Body, "too many login attempts") {
		t.Fatalf("expected rate-limit response, got %s", resp.Body)
	}
}

type testResponse struct {
	Status int
	Cookie *http.Cookie
	JSON   any
	Body   string
	Header http.Header
}

type lifecycleFixture struct {
	app          *App
	ownerCookie  *http.Cookie
	memberCookie *http.Cookie
	workspaceID  string
	suffix       string
}

func (f lifecycleFixture) email(prefix string) string {
	return prefix + "+" + f.suffix + "@example.test"
}

func grantFixtureDoorPermission(t *testing.T, fx lifecycleFixture) {
	t.Helper()
	if _, err := fx.app.db.Exec(t.Context(), `
		update workspace_members
		set role = 'door'
		where workspace_id = $1
		  and person_id = (select id from people where email = $2)
	`, fx.workspaceID, fx.email("member")); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleFixtureSchemaIsolation(t *testing.T) {
	first := newLifecycleFixture(t)
	second := newLifecycleFixture(t)
	var firstSchema, secondSchema string
	if err := first.app.db.QueryRow(t.Context(), `select current_schema()`).Scan(&firstSchema); err != nil {
		t.Fatal(err)
	}
	if err := second.app.db.QueryRow(t.Context(), `select current_schema()`).Scan(&secondSchema); err != nil {
		t.Fatal(err)
	}
	if firstSchema == secondSchema || firstSchema == "public" || secondSchema == "public" {
		t.Fatalf("fixtures must use distinct private schemas: %q, %q", firstSchema, secondSchema)
	}
	otherWorkspace := newLifecycleFixture(t, first.app)
	getJSON(t, first.app, otherWorkspace.ownerCookie, "/api/me", http.StatusOK)
	getJSON(t, first.app, otherWorkspace.ownerCookie, "/api/workspaces/"+first.workspaceID, http.StatusForbidden)
}

func newLifecycleFixture(t *testing.T, shared ...*App) lifecycleFixture {
	t.Helper()
	var app *App
	if len(shared) > 0 {
		// Keep cross-workspace actors in the same database and session authority.
		app = shared[0]
	} else {
		db := newMigrationTestPool(t)
		if err := RunMigrations(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		app = New(Config{AppEnv: "test", PublicWebURL: "http://public.test", SessionSecret: "test-secret"}, db)
	}
	suffix := strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-")) + fmt.Sprintf("-%d", time.Now().UnixNano())
	ownerEmail := "owner+" + suffix + "@example.test"
	memberEmail := "member+" + suffix + "@example.test"

	ownerCookie := signupAndVerifyCookie(t, app, ownerEmail, "Owner")
	workspace := postJSON(t, app, ownerCookie, "/api/workspaces", map[string]any{"name": "Signal Collective"}, http.StatusOK)
	workspaceID := mustString(t, workspace.JSON, "id")
	invite := postJSON(t, app, ownerCookie, "/api/workspaces/"+workspaceID+"/invitations", map[string]any{"email": memberEmail}, http.StatusOK)
	memberCookie := signupAndVerifyCookie(t, app, memberEmail, "Door")
	postJSON(t, app, memberCookie, "/api/invitations/"+mustString(t, invite.JSON, "token")+"/accept", map[string]any{}, http.StatusOK)

	return lifecycleFixture{app: app, ownerCookie: ownerCookie, memberCookie: memberCookie, workspaceID: workspaceID, suffix: suffix}
}

func TestRunMigrationsCreatesEventsTable(t *testing.T) {
	ctx := t.Context()
	db := newMigrationTestPool(t)
	if err := RunMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}

	var exists bool
	if err := db.QueryRow(ctx, `
		select exists (
			select 1
			from information_schema.tables
			where table_schema = current_schema() and table_name = 'events'
		)
	`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected events table to exist after RunMigrations")
	}
}

type fakePaymentProvider struct {
	request  checkoutSessionRequest
	response checkoutSessionResponse
}

func (f *fakePaymentProvider) CreateCheckoutSession(_ context.Context, req checkoutSessionRequest) (checkoutSessionResponse, error) {
	f.request = req
	if f.response.ID == "" {
		f.response.ID = "cs_test_fake"
	}
	if f.response.URL == "" {
		f.response.URL = "https://checkout.example/fake"
	}
	return f.response, nil
}

func createEvent(t *testing.T, fx lifecycleFixture, title string, ticketAllocation int) map[string]any {
	t.Helper()
	return createEventWithPricing(t, fx, title, ticketAllocation, "free", 0, "usd")
}

func createEventWithPricing(t *testing.T, fx lifecycleFixture, title string, ticketAllocation int, pricingMode string, ticketPriceCents int, ticketCurrency string) map[string]any {
	t.Helper()
	resp := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/events", map[string]any{"title": title, "startsAt": "2026-07-01T20:00:00Z", "publicDescription": "Free community event.", "locationDisplay": "Warehouse District", "ticketAllocation": ticketAllocation, "pricingMode": pricingMode, "ticketPriceCents": ticketPriceCents, "ticketCurrency": ticketCurrency}, http.StatusOK)
	return mustObject(t, resp.JSON)
}

func insertPendingStripeTicket(t *testing.T, fx lifecycleFixture, eventID, email, displayName, sessionID string) (string, string) {
	t.Helper()
	code, err := newTicketCode()
	if err != nil {
		t.Fatal(err)
	}
	var ticketID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into tickets (event_id, email, display_name, code, status, payment_status, amount_cents, currency, stripe_checkout_session_id)
		values ($1, $2, $3, $4, 'reserved', 'pending', 1500, 'usd', $5)
		returning id
	`, eventID, email, displayName, code, sessionID).Scan(&ticketID); err != nil {
		t.Fatal(err)
	}
	return ticketID, sessionID
}

func insertRoleApplicationFixture(t *testing.T, fx lifecycleFixture, eventID, roleID, name, email, status string) {
	t.Helper()
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_role_applications (event_id, role_id, applicant_name, applicant_email, message, status)
		values ($1, $2, $3, $4, $5, $6)
	`, eventID, roleID, name, email, name+" private message", status); err != nil {
		t.Fatal(err)
	}
}

func insertTicketWithPaymentStatus(t *testing.T, fx lifecycleFixture, eventID, email, displayName, paymentStatus string, amountCents int, currency string) string {
	t.Helper()
	code, err := newTicketCode()
	if err != nil {
		t.Fatal(err)
	}
	var ticketID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into tickets (event_id, email, display_name, code, status, payment_status, amount_cents, currency)
		values ($1, $2, $3, $4, 'reserved', $5, $6, $7)
		returning id
	`, eventID, email, displayName, code, paymentStatus, amountCents, currency).Scan(&ticketID); err != nil {
		t.Fatal(err)
	}
	return ticketID
}

func ownerPersonID(t *testing.T, fx lifecycleFixture) string {
	t.Helper()
	var personID string
	if err := fx.app.db.QueryRow(t.Context(), `
		select person_id
		from workspace_members
		where workspace_id = $1 and role = 'owner'
		limit 1
	`, fx.workspaceID).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	return personID
}

func stripeCheckoutSessionEvent(t *testing.T, eventType stripe.EventType, eventID, sessionID, ticketID, eventRefID string, paymentStatus stripe.CheckoutSessionPaymentStatus, amountTotal int64, currency string) stripe.Event {
	t.Helper()
	sessionPayload, err := json.Marshal(map[string]any{
		"id":             sessionID,
		"object":         "checkout.session",
		"payment_status": string(paymentStatus),
		"amount_total":   amountTotal,
		"currency":       strings.ToLower(currency),
		"metadata": map[string]string{
			"ticket_id": ticketID,
			"event_id":  eventRefID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stripe.Event{
		ID:   eventID,
		Type: eventType,
		Data: &stripe.EventData{Raw: sessionPayload},
	}
}

func testStripeEventID(t *testing.T, prefix string) string {
	t.Helper()
	return testStripeID(t, prefix)
}

func testStripeSessionID(t *testing.T, prefix string) string {
	t.Helper()
	return testStripeID(t, prefix)
}

func testStripeID(t *testing.T, prefix string) string {
	t.Helper()
	suffix := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	return prefix + "_" + suffix + "_" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func assertPaidTicketState(t *testing.T, fx lifecycleFixture, ticketID, sessionID string) {
	t.Helper()
	var paymentStatus, storedSessionID string
	var paidAt sql.NullTime
	if err := fx.app.db.QueryRow(t.Context(), `
		select payment_status, coalesce(stripe_checkout_session_id, ''), paid_at
		from tickets
		where id = $1
	`, ticketID).Scan(&paymentStatus, &storedSessionID, &paidAt); err != nil {
		t.Fatal(err)
	}
	if paymentStatus != "paid" || storedSessionID != sessionID || !paidAt.Valid {
		t.Fatalf("unexpected paid ticket state: status=%s session=%s paidAt=%v", paymentStatus, storedSessionID, paidAt)
	}
}

func assertEmailOutboxCount(t *testing.T, fx lifecycleFixture, ticketID string, want int) {
	t.Helper()
	var got int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from email_outbox where related_id = $1`, ticketID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("unexpected email count for %s: got %d want %d", ticketID, got, want)
	}
}

func assertWebhookEventCount(t *testing.T, fx lifecycleFixture, eventID string, want int) {
	t.Helper()
	var got int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from payment_webhook_events where id = $1`, eventID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("unexpected webhook event count for %s: got %d want %d", eventID, got, want)
	}
}

func publishEvent(t *testing.T, fx lifecycleFixture, eventID string) map[string]any {
	t.Helper()
	return mustObject(t, publishEventResult(t, fx.app, fx.ownerCookie, eventID).JSON)
}

func publishEventResult(t *testing.T, app *App, cookie *http.Cookie, eventID string) testResponse {
	t.Helper()
	return postJSON(t, app, cookie, "/api/events/"+eventID+"/publish", map[string]any{}, http.StatusOK)
}

func doJSON(t *testing.T, method string, app *App, cookie *http.Cookie, path string, payload any, wantStatus int) testResponse {
	t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s got %d, want %d: %s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	var decoded any
	if strings.TrimSpace(rec.Body.String()) != "" {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatal(err)
		}
	}
	var cookieOut *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName {
			cookieOut = c
		}
	}
	return testResponse{Status: rec.Code, Cookie: cookieOut, JSON: decoded, Body: rec.Body.String(), Header: rec.Header()}
}

func postJSON(t *testing.T, app *App, cookie *http.Cookie, path string, payload any, wantStatus int) testResponse {
	t.Helper()
	return doJSON(t, http.MethodPost, app, cookie, path, payload, wantStatus)
}

func postJSONWithOrigin(t *testing.T, app *App, cookie *http.Cookie, path string, payload any, origin string, wantStatus int) testResponse {
	t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Host = "public.test"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST %s origin %s got %d, want %d: %s", path, origin, rec.Code, wantStatus, rec.Body.String())
	}
	var decoded any
	if strings.TrimSpace(rec.Body.String()) != "" {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatal(err)
		}
	}
	return testResponse{Status: rec.Code, JSON: decoded, Body: rec.Body.String()}
}

func getJSON(t *testing.T, app *App, cookie *http.Cookie, path string, wantStatus int) testResponse {
	t.Helper()
	return doJSON(t, http.MethodGet, app, cookie, path, nil, wantStatus)
}

func patchJSON(t *testing.T, app *App, cookie *http.Cookie, path string, payload any, wantStatus int) testResponse {
	t.Helper()
	return doJSON(t, http.MethodPatch, app, cookie, path, payload, wantStatus)
}

func mustObject(t *testing.T, value any) map[string]any {
	t.Helper()
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected JSON object, got %#v", value)
	}
	return obj
}

func mustString(t *testing.T, value any, key string) string {
	t.Helper()
	obj := mustObject(t, value)
	v, ok := obj[key].(string)
	if !ok {
		t.Fatalf("expected %s string, got %#v", key, obj[key])
	}
	return v
}

func signupAndVerifyCookie(t *testing.T, app *App, email, displayName string) *http.Cookie {
	t.Helper()
	postJSON(t, app, nil, "/api/auth/signup", map[string]any{"email": email, "password": "secret1234", "displayName": displayName}, http.StatusAccepted)
	token := latestIdentityToken(t, app, "identity_verification")
	result := postJSON(t, app, nil, "/api/auth/verify-email", map[string]any{"token": token}, http.StatusOK)
	if result.Cookie == nil {
		t.Fatal("expected verification to issue an access cookie")
	}
	return result.Cookie
}

func auditMetadataForAction(t *testing.T, db *pgxpool.Pool, action string) string {
	t.Helper()
	rows, err := db.Query(t.Context(), `
		select metadata::text
		from audit_entries
		where action = $1
		order by created_at asc
	`, action)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var combined strings.Builder
	for rows.Next() {
		var metadata string
		if err := rows.Scan(&metadata); err != nil {
			t.Fatal(err)
		}
		combined.WriteString(metadata)
		combined.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return combined.String()
}
