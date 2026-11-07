package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mailprovider "git.subcult.tv/PatrickFanella/subcult-os/internal/mail"
)

const (
	syntheticListmonkUser    = "subcult-relay"
	syntheticListmonkToken   = "synthetic-listmonk-token"
	syntheticListmonkList    = 7
	syntheticListmonkKorkURL = "https://lists.example.test"
	syntheticDisclosure      = "subcult-os-listmonk-v1"
)

type fakeListmonkSubscriber struct {
	id      int64
	status  string
	lists   map[int64]string
	created bool
}

// fakeListmonk follows the documented listmonk API shapes the relay uses:
// GET /api/lists/{id}, GET/POST /api/subscribers, PUT /api/subscribers/lists
// and POST /api/tx. It never sends mail; it counts transactional messages.
type fakeListmonk struct {
	mu           sync.Mutex
	subscribers  map[string]*fakeListmonkSubscriber
	nextID       int64
	tx           map[string]int
	txBodies     []map[string]any
	txStatus     []int
	onLookup     func()
	unauthorized bool
}

var fakeListmonkQuery = regexp.MustCompile(`^subscribers\.email = '(.*)'$`)

func (f *fakeListmonk) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reply := func(status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Header.Get("Authorization") != "token "+syntheticListmonkUser+":"+syntheticListmonkToken || f.unauthorized {
		reply(http.StatusForbidden, map[string]string{"message": "invalid API credentials"})
		return
	}
	body, _ := io.ReadAll(r.Body)
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/lists/"):
		if r.URL.Path != "/api/lists/"+strconv.Itoa(syntheticListmonkList) {
			reply(http.StatusNotFound, map[string]string{"message": "list not found"})
			return
		}
		reply(http.StatusOK, map[string]any{"data": map[string]any{"id": syntheticListmonkList, "name": "Synthetic list"}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/subscribers":
		if f.onLookup != nil {
			hook := f.onLookup
			f.onLookup = nil
			hook()
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		match := fakeListmonkQuery.FindStringSubmatch(r.URL.Query().Get("query"))
		results := []any{}
		if match != nil {
			if sub := f.subscribers[strings.ReplaceAll(match[1], "''", "'")]; sub != nil {
				lists := []any{}
				for id, status := range sub.lists {
					lists = append(lists, map[string]any{"id": id, "subscription_status": status})
				}
				results = append(results, map[string]any{"id": sub.id, "status": sub.status, "lists": lists})
			}
		}
		reply(http.StatusOK, map[string]any{"data": map[string]any{"results": results}})
	case r.Method == http.MethodPost && r.URL.Path == "/api/subscribers":
		f.mu.Lock()
		defer f.mu.Unlock()
		var req struct {
			Email      string  `json:"email"`
			Status     string  `json:"status"`
			Lists      []int64 `json:"lists"`
			Preconfirm bool    `json:"preconfirm_subscriptions"`
		}
		_ = json.Unmarshal(body, &req)
		if f.subscribers[req.Email] != nil {
			reply(http.StatusConflict, map[string]string{"message": "E-mail already exists."})
			return
		}
		f.nextID++
		sub := &fakeListmonkSubscriber{id: f.nextID, status: req.Status, lists: map[int64]string{}, created: true}
		for _, id := range req.Lists {
			sub.lists[id] = map[bool]string{true: "confirmed", false: "unconfirmed"}[req.Preconfirm]
		}
		f.subscribers[req.Email] = sub
		reply(http.StatusOK, map[string]any{"data": map[string]any{"id": sub.id}})
	case r.Method == http.MethodPut && r.URL.Path == "/api/subscribers/lists":
		f.mu.Lock()
		defer f.mu.Unlock()
		var req struct {
			IDs     []int64 `json:"ids"`
			Action  string  `json:"action"`
			Targets []int64 `json:"target_list_ids"`
			Status  string  `json:"status"`
		}
		_ = json.Unmarshal(body, &req)
		for _, sub := range f.subscribers {
			for _, id := range req.IDs {
				if sub.id == id && req.Action == "add" {
					for _, list := range req.Targets {
						sub.lists[list] = req.Status
					}
				}
			}
		}
		reply(http.StatusOK, map[string]any{"data": true})
	case r.Method == http.MethodPost && r.URL.Path == "/api/tx":
		f.mu.Lock()
		defer f.mu.Unlock()
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		email, _ := req["subscriber_email"].(string)
		if f.subscribers[email] == nil {
			reply(http.StatusBadRequest, map[string]string{"message": "subscriber not found"})
			return
		}
		status := http.StatusOK
		if len(f.txStatus) > 0 {
			status, f.txStatus = f.txStatus[0], f.txStatus[1:]
		}
		// A 5xx after processing models an uncertain send: the message left.
		if status == http.StatusOK || status >= 500 && status != http.StatusServiceUnavailable {
			f.tx[email]++
			f.txBodies = append(f.txBodies, req)
		}
		if status != http.StatusOK {
			reply(status, map[string]string{"message": "synthetic failure"})
			return
		}
		reply(http.StatusOK, map[string]any{"data": true})
	default:
		reply(http.StatusNotFound, map[string]string{"message": "not found"})
	}
}

func (f *fakeListmonk) sends(email string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tx[normalizeEmail(email)]
}

func (f *fakeListmonk) totalSends() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, n := range f.tx {
		total += n
	}
	return total
}

type listmonkFixture struct {
	lifecycleFixture
	fake      *fakeListmonk
	server    *httptest.Server
	bindingID string
}

func korkPolicy() map[string]any {
	return map[string]any{
		"listUrl": syntheticListmonkKorkURL, "sender": "Signal Collective", "purpose": "Show announcements",
		"disclosure": "Once you confirm, your address is added to the page owner's mailing list at lists.example.test, which they run with listmonk.",
	}
}

// newListmonkFixture configures the relay against the fake and binds the
// workspace to its one list with a Kork receiver policy.
func newListmonkFixture(t *testing.T) listmonkFixture {
	t.Helper()
	fx := consentDeliveryFixture(t)
	fake := &fakeListmonk{subscribers: map[string]*fakeListmonkSubscriber{}, tx: map[string]int{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	fx.app.config.ListmonkURL = server.URL
	fx.app.config.ListmonkAPIUser = syntheticListmonkUser
	fx.app.config.ListmonkAPIToken = syntheticListmonkToken
	binding := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/listmonk-bindings", map[string]any{
		"label": "Synthetic newsletter", "listId": syntheticListmonkList, "templateId": 3, "senderAddress": "news@example.test",
		"disclosureVersion": syntheticDisclosure, "korkPolicy": korkPolicy(),
	}, http.StatusCreated)
	return listmonkFixture{lifecycleFixture: fx, fake: fake, server: server, bindingID: mustString(t, binding.JSON, "id")}
}

func (fx listmonkFixture) path(suffix string) string {
	return "/api/workspaces/" + fx.workspaceID + suffix
}

// grant inserts a verified native grant confirmed under disclosure.
func (fx listmonkFixture) grant(t *testing.T, recipient, disclosure string) string {
	t.Helper()
	id, _ := insertConsentGrant(t, fx.lifecycleFixture, fx.workspaceID, recipient, consentPurposeAnnouncement, "explicit_form", true)
	if _, err := fx.app.db.Exec(t.Context(), `update consent_grants set disclosure_version = $2 where id = $1`, id, disclosure); err != nil {
		t.Fatal(err)
	}
	return id
}

func (fx listmonkFixture) receipt(overrides map[string]any) map[string]any {
	policy := korkPolicy()
	receipt := map[string]any{
		"v": 1, "list": "listmonk:" + syntheticListmonkKorkURL + "#" + strconv.Itoa(syntheticListmonkList),
		"sender": policy["sender"], "page": "https://kork.example.test/signal", "purpose": policy["purpose"],
		"disclosure": policy["disclosure"], "at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano),
	}
	for key, value := range overrides {
		if value == nil {
			delete(receipt, key)
			continue
		}
		receipt[key] = value
	}
	return receipt
}

func (fx listmonkFixture) importKork(t *testing.T, bindingID, recipient string, receipt map[string]any, want int) testResponse {
	t.Helper()
	return postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings/"+bindingID+"/kork-grants"),
		map[string]any{"recipientAddress": recipient, "receipt": receipt}, want)
}

func (fx listmonkFixture) dispatch(t *testing.T) (string, AnnouncementDispatchReport) {
	t.Helper()
	id := draftDueAnnouncement(t, fx.lifecycleFixture)
	report, err := RunAnnouncementDispatch(t.Context(), fx.app.config, fx.app.db, 10)
	if err != nil {
		t.Fatal(err)
	}
	return id, report
}

func (fx listmonkFixture) relay(t *testing.T) ListmonkRelayReport {
	t.Helper()
	report, err := RunListmonkRelay(t.Context(), fx.app.config, fx.app.db, 10, fx.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func (fx listmonkFixture) ledger(t *testing.T, announcementID string) map[string]any {
	t.Helper()
	return mustObject(t, getJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+announcementID), http.StatusOK).JSON)
}

func (fx listmonkFixture) deliveryState(t *testing.T, announcementID, recipient string) (status, code string) {
	t.Helper()
	var lastError *string
	if err := fx.app.db.QueryRow(t.Context(), `select status, last_error from announcement_listmonk_deliveries where announcement_id = $1 and recipient_address = $2`,
		announcementID, normalizeEmail(recipient)).Scan(&status, &lastError); err != nil {
		t.Fatal(err)
	}
	if lastError != nil {
		code = *lastError
	}
	return status, code
}

func countAnnouncementOutbox(t *testing.T, fx lifecycleFixture, announcementID string) int {
	t.Helper()
	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from email_outbox where related_type = 'announcement' and related_id = $1`, announcementID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestListmonkRelayJourney(t *testing.T) {
	fx := newListmonkFixture(t)
	keep := fx.email("listmonk-keep")
	fx.grant(t, keep, syntheticDisclosure)
	// Confirmed under an older disclosure that did not name listmonk: withheld
	// until the person confirms again.
	stale := fx.email("listmonk-stale")
	fx.grant(t, stale, "v0-native-only")

	created := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements"), map[string]any{"subject": "Synthetic lineup", "body": "New dates."}, http.StatusOK)
	id := mustString(t, created.JSON, "id")
	preview := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/preview"), http.StatusOK).JSON)
	if preview["deliveryPath"] != "listmonk" || preview["recipientCount"] != float64(1) || preview["withheldCount"] != float64(1) || preview["listmonkConfigured"] != true {
		t.Fatalf("preview = %#v", preview)
	}
	if raw, _ := json.Marshal(preview); strings.Contains(string(raw), "@example.test") {
		t.Fatalf("preview exposed an address: %s", raw)
	}
	makeAnnouncementDue(t, fx.lifecycleFixture, id)
	report, err := RunAnnouncementDispatch(t.Context(), fx.app.config, fx.app.db, 10)
	if err != nil || report.Claimed != 1 || report.RecipientsEnqueued != 1 || report.RecipientsWithheld != 1 {
		t.Fatalf("dispatch = %+v, %v", report, err)
	}
	// Duplicate dispatch claims nothing and adds no row.
	if again, err := RunAnnouncementDispatch(t.Context(), fx.app.config, fx.app.db, 10); err != nil || again.Claimed != 0 {
		t.Fatalf("second dispatch = %+v, %v", again, err)
	}
	if n := countAnnouncementOutbox(t, fx.lifecycleFixture, id); n != 0 {
		t.Fatalf("listmonk occurrence enqueued %d native outbox rows", n)
	}
	relay := fx.relay(t)
	if relay.Claimed != 1 || relay.Accepted != 1 {
		t.Fatalf("relay = %+v", relay)
	}
	if again := fx.relay(t); again.Claimed != 0 {
		t.Fatalf("second relay = %+v", again)
	}
	if fx.fake.sends(keep) != 1 || fx.fake.sends(stale) != 0 || fx.fake.sends(fx.email("member")) != 0 {
		t.Fatalf("sends = %#v", fx.fake.tx)
	}
	sub := fx.fake.subscribers[normalizeEmail(keep)]
	if sub == nil || sub.lists[syntheticListmonkList] != "confirmed" || sub.status != "enabled" {
		t.Fatalf("subscriber = %#v", sub)
	}
	tx := fx.fake.txBodies[0]
	data, _ := tx["data"].(map[string]any)
	body, _ := data["body"].(string)
	if tx["template_id"] != float64(3) || tx["from_email"] != "news@example.test" || tx["subject"] != "Synthetic lineup" ||
		!strings.Contains(body, "/consent/withdraw?token=") {
		t.Fatalf("tx payload = %#v", tx)
	}
	ledger := fx.ledger(t, id)
	counts, _ := ledger["listmonkStatusCounts"].(map[string]any)
	if ledger["deliveryPath"] != "listmonk" || counts["accepted"] != float64(1) || ledger["recipientCount"] != float64(1) || ledger["withheldCount"] != float64(1) {
		t.Fatalf("ledger = %#v", ledger)
	}
	// The unsubscribe link in the listmonk message withdraws the grant.
	_, link, _ := strings.Cut(body, "/consent/withdraw?token=")
	postJSON(t, fx.app, nil, "/api/public/consent/"+strings.TrimSpace(link)+"/withdraw", map[string]any{}, http.StatusOK)
	var withdrawn bool
	if err := fx.app.db.QueryRow(t.Context(), `select withdrawn_at is not null from consent_grants where recipient_address = $1`, normalizeEmail(keep)).Scan(&withdrawn); err != nil || !withdrawn {
		t.Fatalf("withdraw link did not withdraw the grant: %v", err)
	}
}

func TestListmonkKorkReceiptValidation(t *testing.T) {
	fx := newListmonkFixture(t)
	cases := []struct {
		name      string
		overrides map[string]any
		want      int
		code      string
	}{
		{"wrong sender", map[string]any{"sender": "Another Collective"}, http.StatusUnprocessableEntity, "receipt_wrong_sender"},
		{"wrong purpose", map[string]any{"purpose": "Merch drops"}, http.StatusUnprocessableEntity, "receipt_wrong_purpose"},
		{"wrong list", map[string]any{"list": "listmonk:" + syntheticListmonkKorkURL + "#8"}, http.StatusUnprocessableEntity, "receipt_wrong_list"},
		{"kork-only list", map[string]any{"list": "kork"}, http.StatusUnprocessableEntity, "receipt_wrong_list"},
		{"tampered disclosure", map[string]any{"disclosure": "Your address may be shared with partners."}, http.StatusUnprocessableEntity, "receipt_wrong_disclosure"},
		{"tampered extra field", map[string]any{"authorizes": "subcult-os"}, http.StatusUnprocessableEntity, "receipt_invalid"},
		{"unknown version", map[string]any{"v": 2}, http.StatusUnprocessableEntity, "receipt_invalid"},
		{"missing time", map[string]any{"at": nil}, http.StatusUnprocessableEntity, "receipt_invalid"},
		{"expired", map[string]any{"at": time.Now().Add(-181 * 24 * time.Hour).UTC().Format(time.RFC3339)}, http.StatusUnprocessableEntity, "receipt_expired"},
		{"future", map[string]any{"at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, http.StatusUnprocessableEntity, "receipt_expired"},
	}
	for _, tc := range cases {
		got := fx.importKork(t, fx.bindingID, fx.email("kork-refused"), fx.receipt(tc.overrides), tc.want)
		if mustObject(t, got.JSON)["error"] != tc.code {
			t.Fatalf("%s: got %#v, want %s", tc.name, got.JSON, tc.code)
		}
	}
	var stored int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from consent_grants where workspace_id = $1`, fx.workspaceID).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("refused receipts stored %d grants (%v)", stored, err)
	}

	// The member can't import receipts; consent authority stays with organizers.
	postJSON(t, fx.app, fx.memberCookie, fx.path("/listmonk-bindings/"+fx.bindingID+"/kork-grants"),
		map[string]any{"recipientAddress": fx.email("kork-member"), "receipt": fx.receipt(nil)}, http.StatusForbidden)

	valid := fx.email("kork-valid")
	ok := fx.importKork(t, fx.bindingID, valid, fx.receipt(nil), http.StatusOK)
	if mustObject(t, ok.JSON)["status"] != "verified" {
		t.Fatalf("import = %#v", ok.JSON)
	}
	// A Kork receipt authorizes this listmonk list only, never native email.
	if err := fx.app.checkSendPermission(t.Context(), fx.workspaceID, "email", valid, consentPurposeAnnouncement); err != ErrConsentGrantRequired {
		t.Fatalf("native permission for a Kork-origin grant = %v", err)
	}
	fx.importKork(t, fx.bindingID, valid, fx.receipt(nil), http.StatusConflict)

	// A receipt that predates a recorded withdrawal never restores consent.
	withdrawnAddress := fx.email("kork-withdrawn")
	grantID := fx.grant(t, withdrawnAddress, syntheticDisclosure)
	if _, err := fx.app.db.Exec(t.Context(), `update consent_grants set withdrawn_at = now() - interval '10 minutes' where id = $1`, grantID); err != nil {
		t.Fatal(err)
	}
	got := fx.importKork(t, fx.bindingID, withdrawnAddress, fx.receipt(nil), http.StatusConflict)
	if mustObject(t, got.JSON)["error"] != "grant_withdrawn" {
		t.Fatalf("withdrawn import = %#v", got.JSON)
	}
	// A suppressed address is refused.
	suppressed := fx.email("kork-suppressed")
	if _, err := fx.app.db.Exec(t.Context(), `insert into email_suppressions (recipient_email, reason) values ($1, 'email.complained')`, normalizeEmail(suppressed)); err != nil {
		t.Fatal(err)
	}
	if got := fx.importKork(t, fx.bindingID, suppressed, fx.receipt(nil), http.StatusConflict); mustObject(t, got.JSON)["error"] != "recipient_suppressed" {
		t.Fatalf("suppressed import = %#v", got.JSON)
	}

	// The Kork grant is delivered through listmonk.
	id, report := fx.dispatch(t)
	if report.RecipientsEnqueued != 1 {
		t.Fatalf("dispatch = %+v", report)
	}
	fx.relay(t)
	if status, _ := fx.deliveryState(t, id, valid); status != listmonkStatusAccepted || fx.fake.sends(valid) != 1 {
		t.Fatalf("kork recipient status %s, sends %d", status, fx.fake.sends(valid))
	}
}

func TestListmonkWrongSenderGrantIsRefused(t *testing.T) {
	fx := newListmonkFixture(t)
	korkAddress := fx.email("kork-old-binding")
	fx.importKork(t, fx.bindingID, korkAddress, fx.receipt(nil), http.StatusOK)
	// Replace the binding with one for another sender and no Kork policy.
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusOK)
	replacement := postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings"), map[string]any{
		"label": "Other sender", "listId": syntheticListmonkList, "templateId": 3, "senderAddress": "other@example.test", "disclosureVersion": syntheticDisclosure,
	}, http.StatusCreated)
	replacementID := mustString(t, replacement.JSON, "id")
	got := fx.importKork(t, replacementID, fx.email("kork-new"), fx.receipt(nil), http.StatusConflict)
	if mustObject(t, got.JSON)["error"] != "binding_accepts_no_kork_receipts" {
		t.Fatalf("import without policy = %#v", got.JSON)
	}
	fx.importKork(t, fx.bindingID, fx.email("kork-revoked"), fx.receipt(nil), http.StatusConflict)

	id, report := fx.dispatch(t)
	if report.RecipientsEnqueued != 0 || report.RecipientsWithheld != 1 {
		t.Fatalf("grant scoped to the revoked sender was used: %+v", report)
	}
	fx.relay(t)
	if fx.fake.totalSends() != 0 || countAnnouncementOutbox(t, fx.lifecycleFixture, id) != 0 {
		t.Fatalf("wrong-sender grant reached a path: tx %d", fx.fake.totalSends())
	}
}

func TestListmonkWithdrawalBeforeSend(t *testing.T) {
	fx := newListmonkFixture(t)
	before := fx.email("withdraw-after-dispatch")
	during := fx.email("withdraw-during-send")
	beforeGrant := fx.grant(t, before, syntheticDisclosure)
	duringGrant := fx.grant(t, during, syntheticDisclosure)
	id, report := fx.dispatch(t)
	if report.RecipientsEnqueued != 2 {
		t.Fatalf("dispatch = %+v", report)
	}
	// Withdrawn after dispatch (no schedule-time snapshot authorizes it).
	if _, err := fx.app.db.Exec(t.Context(), `update consent_grants set withdrawn_at = now() where id = $1`, beforeGrant); err != nil {
		t.Fatal(err)
	}
	// Withdrawn while the relay is already talking to listmonk.
	fx.fake.onLookup = func() {
		if _, err := fx.app.db.Exec(t.Context(), `update consent_grants set withdrawn_at = now() where id = $1`, duringGrant); err != nil {
			t.Error(err)
		}
	}
	relay := fx.relay(t)
	if relay.Withheld != 2 || relay.Accepted != 0 {
		t.Fatalf("relay = %+v", relay)
	}
	for _, recipient := range []string{before, during} {
		if status, code := fx.deliveryState(t, id, recipient); status != listmonkStatusWithheld || code != "consent_withdrawn" {
			t.Fatalf("%s: %s/%s", recipient, status, code)
		}
	}
	if fx.fake.totalSends() != 0 {
		t.Fatal("a withdrawn recipient reached /api/tx")
	}
}

func TestListmonkSuppression(t *testing.T) {
	fx := newListmonkFixture(t)
	local := fx.email("suppressed-locally")
	blocked := fx.email("provider-blocklisted")
	left := fx.email("provider-unsubscribed")
	fx.grant(t, local, syntheticDisclosure)
	fx.grant(t, blocked, syntheticDisclosure)
	leftGrant := fx.grant(t, left, syntheticDisclosure)
	fx.fake.subscribers[normalizeEmail(blocked)] = &fakeListmonkSubscriber{id: 90, status: "blocklisted", lists: map[int64]string{syntheticListmonkList: "confirmed"}}
	fx.fake.subscribers[normalizeEmail(left)] = &fakeListmonkSubscriber{id: 91, status: "enabled", lists: map[int64]string{syntheticListmonkList: "unsubscribed"}}
	fx.fake.nextID = 100
	id, _ := fx.dispatch(t)
	if _, err := fx.app.db.Exec(t.Context(), `insert into email_suppressions (recipient_email, reason) values ($1, 'email.bounced')`, normalizeEmail(local)); err != nil {
		t.Fatal(err)
	}
	if relay := fx.relay(t); relay.Suppressed != 3 {
		t.Fatalf("relay = %+v", relay)
	}
	want := map[string]string{local: "suppressed", blocked: "provider_blocklisted", left: "provider_unsubscribed"}
	for recipient, code := range want {
		if status, got := fx.deliveryState(t, id, recipient); status != listmonkStatusSuppressed || got != code {
			t.Fatalf("%s: %s/%s", recipient, status, got)
		}
	}
	if fx.fake.totalSends() != 0 || fx.fake.subscribers[normalizeEmail(left)].lists[syntheticListmonkList] != "unsubscribed" {
		t.Fatal("a suppressed recipient was sent to or resubscribed")
	}
	// The provider unsubscribe is reconciled into a withdrawal.
	var reason string
	if err := fx.app.db.QueryRow(t.Context(), `select withdrawal_reason from consent_grants where id = $1 and withdrawn_at is not null`, leftGrant).Scan(&reason); err != nil || reason != "listmonk_unsubscribed" {
		t.Fatalf("withdrawal reason %q (%v)", reason, err)
	}
}

func TestListmonkSinglePathPerOccurrence(t *testing.T) {
	fx := newListmonkFixture(t)
	both := fx.email("both-paths")
	fx.grant(t, both, syntheticDisclosure)
	id, _ := fx.dispatch(t)
	// The native path cannot gain a row for a listmonk occurrence.
	var outboxID string
	if err := fx.app.db.QueryRow(t.Context(), `
		insert into email_outbox (recipient_email, subject, body, related_type, related_id, delivery_status, purpose, workspace_id, expires_at)
		values ($1, 's', 'b', 'announcement', $2, 'pending', 'announcement', $3, now() + interval '1 hour') returning id::text`,
		normalizeEmail(both), id, fx.workspaceID).Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `insert into announcement_deliveries (announcement_id, outbox_id, recipient_address) values ($1, $2, $3)`,
		id, outboxID, normalizeEmail(both)); err == nil {
		t.Fatal("a native delivery was recorded for a listmonk occurrence")
	}
	if _, err := fx.app.db.Exec(t.Context(), `delete from email_outbox where id = $1`, outboxID); err != nil {
		t.Fatal(err)
	}
	fx.relay(t)
	sent, err := fx.app.processEmailDeliveries(t.Context(), func(_ context.Context, _ mailprovider.Message) (string, error) {
		t.Fatal("native outbox sent a listmonk occurrence")
		return "", nil
	}, 10)
	if err != nil || sent.Accepted != 0 {
		t.Fatalf("native send = %+v, %v", sent, err)
	}
	if fx.fake.sends(both) != 1 {
		t.Fatalf("listmonk sends = %d, want 1", fx.fake.sends(both))
	}

	// After revoking the binding the next occurrence goes native, and the
	// listmonk path cannot gain a row for it.
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusOK)
	native, report := fx.dispatch(t)
	if report.RecipientsEnqueued != 1 || countAnnouncementOutbox(t, fx.lifecycleFixture, native) != 1 || fx.ledger(t, native)["deliveryPath"] != "native" {
		t.Fatalf("native dispatch = %+v", report)
	}
	var grantID string
	if err := fx.app.db.QueryRow(t.Context(), `select id::text from consent_grants where recipient_address = $1`, normalizeEmail(both)).Scan(&grantID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into announcement_listmonk_deliveries (announcement_id, workspace_id, binding_id, grant_id, recipient_address, subject, body, expires_at)
		values ($1, $2, $3, $4, $5, 's', 'b', now() + interval '1 hour')`, native, fx.workspaceID, fx.bindingID, grantID, normalizeEmail(both)); err == nil {
		t.Fatal("a listmonk delivery was recorded for a native occurrence")
	}
}

func TestListmonkUncertainAndRetryOutcomes(t *testing.T) {
	fx := newListmonkFixture(t)
	recipient := fx.email("uncertain")
	fx.grant(t, recipient, syntheticDisclosure)
	id, _ := fx.dispatch(t)
	fx.fake.txStatus = []int{http.StatusInternalServerError}
	if relay := fx.relay(t); relay.Uncertain != 1 {
		t.Fatalf("relay = %+v", relay)
	}
	// Held: never retried automatically.
	if _, err := fx.app.db.Exec(t.Context(), `update announcement_listmonk_deliveries set next_attempt_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	if relay := fx.relay(t); relay.Claimed != 0 || fx.fake.sends(recipient) != 1 {
		t.Fatalf("uncertain row was resent: %+v", relay)
	}
	if counts, _ := fx.ledger(t, id)["listmonkStatusCounts"].(map[string]any); counts["uncertain"] != float64(1) {
		t.Fatalf("ledger counts = %#v", counts)
	}
	postJSON(t, fx.app, fx.memberCookie, fx.path("/announcements/"+id+"/listmonk-deliveries/retry"), map[string]any{}, http.StatusForbidden)
	retried := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/listmonk-deliveries/retry"), map[string]any{}, http.StatusOK)
	if mustObject(t, retried.JSON)["requeued"] != float64(1) {
		t.Fatalf("retry = %#v", retried.JSON)
	}
	if relay := fx.relay(t); relay.Accepted != 1 {
		t.Fatalf("relay after retry = %+v", relay)
	}

	// Throttling retries with backoff; five attempts then held as failed.
	if _, err := fx.app.db.Exec(t.Context(), `update consent_grants set withdrawn_at = now() where recipient_address = $1`, normalizeEmail(recipient)); err != nil {
		t.Fatal(err)
	}
	throttled := fx.email("throttled")
	fx.grant(t, throttled, syntheticDisclosure)
	next, _ := fx.dispatch(t)
	fx.fake.txStatus = []int{503, 503, 503, 503, 503}
	for attempt := 1; attempt <= listmonkMaxAttempts; attempt++ {
		fx.relay(t)
		if _, err := fx.app.db.Exec(t.Context(), `update announcement_listmonk_deliveries set next_attempt_at = now() where announcement_id = $1 and status = 'pending'`, next); err != nil {
			t.Fatal(err)
		}
	}
	if status, code := fx.deliveryState(t, next, throttled); status != listmonkStatusFailed || code != "listmonk_http_503" {
		t.Fatalf("throttled: %s/%s", status, code)
	}
	if fx.fake.sends(throttled) != 0 {
		t.Fatal("a throttled request counted as sent")
	}
}

func TestListmonkBindingAuthorityAndConfiguration(t *testing.T) {
	fx := newListmonkFixture(t)
	body := map[string]any{"label": "x", "listId": syntheticListmonkList, "templateId": 3, "senderAddress": "news@example.test", "disclosureVersion": syntheticDisclosure}
	postJSON(t, fx.app, fx.memberCookie, fx.path("/listmonk-bindings"), body, http.StatusForbidden)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings"), body, http.StatusConflict)
	// The Backline listing never shows a listmonk binding.
	if list := getJSON(t, fx.app, fx.ownerCookie, fx.path("/backline-bindings"), http.StatusOK); len(list.JSON.([]any)) != 0 {
		t.Fatalf("backline bindings = %#v", list.JSON)
	}
	if list := getJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings"), http.StatusOK); len(list.JSON.([]any)) != 1 {
		t.Fatalf("listmonk bindings = %#v", list.JSON)
	}
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusConflict)
	body["listId"] = 99
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings"), body, http.StatusUnprocessableEntity)
	body["listId"] = syntheticListmonkList
	fx.fake.unauthorized = true
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings"), body, http.StatusBadGateway)
	fx.fake.unauthorized = false
	fx.app.config.ListmonkAPIToken = ""
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings"), body, http.StatusConflict)
	if _, err := RunListmonkRelay(t.Context(), fx.app.config, fx.app.db, 10, nil); err == nil {
		t.Fatal("the relay ran without configuration")
	}
}

func TestListmonkHeldWithoutMailDeliveryAndRevocation(t *testing.T) {
	fx := newListmonkFixture(t)
	recipient := fx.email("held")
	fx.grant(t, recipient, syntheticDisclosure)
	fx.app.config.MailDeliveryEnabled = false
	id, _ := fx.dispatch(t)
	if relay := fx.relay(t); relay.Claimed != 0 {
		t.Fatalf("held row was sent: %+v", relay)
	}
	if status, _ := fx.deliveryState(t, id, recipient); status != listmonkStatusHeld {
		t.Fatalf("status = %s", status)
	}
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/listmonk-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusOK)
	if status, code := fx.deliveryState(t, id, recipient); status != listmonkStatusWithheld || code != "binding_revoked" {
		t.Fatalf("after revoke: %s/%s", status, code)
	}
	if fx.fake.totalSends() != 0 {
		t.Fatal("unexpected send")
	}
}
