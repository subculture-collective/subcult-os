package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const syntheticBacklineSecret = "synthetic-backline-receiver-secret-0123456789"

// syntheticBackline is a fake receiver following the v1 contract: it verifies
// signatures, deduplicates on the occurrence key, keeps a tombstone for a
// cancel that arrives first, and counts scheduled channel posts.
type syntheticBackline struct {
	mu                  sync.Mutex
	states              map[string]string
	posts               int
	requests            []map[string]any
	failAfterProcessing int
}

func (s *syntheticBackline) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	reply := func(status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if verifyBacklineSignature(syntheticBacklineSecret, backlineSignatureScopeHandoff, r.Header, body, time.Now()) != nil {
		reply(http.StatusUnauthorized, map[string]string{"error": "invalid_signature"})
		return
	}
	var raw map[string]any
	var payload backlineHandoffPayload
	if json.Unmarshal(body, &raw) != nil || json.Unmarshal(body, &payload) != nil || payload.Version != 1 {
		reply(http.StatusBadRequest, map[string]string{"error": "invalid_payload"})
		return
	}
	s.requests = append(s.requests, raw)
	status := ""
	switch payload.Operation {
	case handoffOperationPublish:
		switch s.states[payload.OccurrenceKey] {
		case "cancelled":
			reply(http.StatusConflict, map[string]string{"error": "occurrence_cancelled"})
			return
		case "scheduled":
			status = "duplicate"
		default:
			s.states[payload.OccurrenceKey] = "scheduled"
			s.posts++
			status = "accepted"
		}
	case handoffOperationCancel:
		status = "not_found"
		if s.states[payload.OccurrenceKey] == "scheduled" {
			status = "cancelled"
		}
		s.states[payload.OccurrenceKey] = "cancelled"
	}
	if s.failAfterProcessing > 0 {
		s.failAfterProcessing--
		reply(http.StatusInternalServerError, map[string]string{"error": "late_failure"})
		return
	}
	reply(http.StatusOK, backlineAck{Version: 1, OccurrenceKey: payload.OccurrenceKey, Status: status, ReceiverReference: "synthetic-job"})
}

type backlineHandoffFixture struct {
	lifecycleFixture
	receiver  *syntheticBackline
	server    *httptest.Server
	bindingID string
}

func newBacklineHandoffFixture(t *testing.T) backlineHandoffFixture {
	t.Helper()
	fx := newLifecycleFixture(t)
	receiver := &syntheticBackline{states: map[string]string{}}
	server := httptest.NewServer(receiver)
	t.Cleanup(server.Close)
	fx.app.config.BacklineHandoffURL = server.URL
	fx.app.config.BacklineHandoffSecret = syntheticBacklineSecret
	binding := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/backline-bindings",
		map[string]any{"label": "Synthetic #announcements", "serverId": "112233445566778899", "channelId": "998877665544332211"}, http.StatusCreated)
	return backlineHandoffFixture{lifecycleFixture: fx, receiver: receiver, server: server, bindingID: mustString(t, binding.JSON, "id")}
}

func (fx backlineHandoffFixture) path(suffix string) string {
	return "/api/workspaces/" + fx.workspaceID + suffix
}

func (fx backlineHandoffFixture) scheduledAnnouncement(t *testing.T) string {
	t.Helper()
	created := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements"), map[string]any{"subject": "Doors move to 8pm", "body": "Synthetic channel announcement."}, http.StatusOK)
	id := mustString(t, created.JSON, "id")
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/schedule"), map[string]any{"scheduledFor": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, http.StatusOK)
	return id
}

func (fx backlineHandoffFixture) runWorker(t *testing.T) BacklineHandoffReport {
	t.Helper()
	report, err := RunBacklineHandoffs(t.Context(), fx.app.config, fx.app.db, 10, fx.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func (fx backlineHandoffFixture) handoff(t *testing.T, announcementID string) map[string]any {
	t.Helper()
	got := getJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+announcementID), http.StatusOK)
	handoffs, ok := mustObject(t, got.JSON)["channelHandoffs"].([]any)
	if !ok || len(handoffs) != 1 {
		t.Fatalf("expected one channel handoff in the outcome ledger, got %#v", mustObject(t, got.JSON)["channelHandoffs"])
	}
	return mustObject(t, handoffs[0])
}

func (fx backlineHandoffFixture) postReceipt(t *testing.T, secret string, receipt backlineReceipt, want int) {
	t.Helper()
	body, _ := json.Marshal(receipt)
	fx.postSignedReceipt(t, body, secret, backlineSignatureScopeReceipt, time.Now().Unix(), want)
}

func (fx backlineHandoffFixture) postSignedReceipt(t *testing.T, body []byte, secret, scope string, timestamp int64, want int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/backline/receipts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(backlineTimestampHeader, strconv.FormatInt(timestamp, 10))
	req.Header.Set(backlineSignatureHeader, backlineSignature(secret, scope, timestamp, body))
	rec := httptest.NewRecorder()
	fx.app.Handler().ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("receipt status %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
}

// uncertainThenRetried leaves a handoff that Backline stored but whose
// acknowledgement was lost, then retried by an operator: pending again, with
// receiver_may_hold set.
func (fx backlineHandoffFixture) uncertainThenRetried(t *testing.T) (announcementID, handoffID string) {
	t.Helper()
	announcementID = fx.scheduledAnnouncement(t)
	created := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+announcementID+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	handoffID = mustString(t, created.JSON, "id")
	fx.receiver.failAfterProcessing = 1
	if report := fx.runWorker(t); report.Uncertain != 1 {
		t.Fatalf("a 500 after processing must be uncertain: %+v", report)
	}
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+announcementID+"/channel-handoffs/"+handoffID+"/retry"), map[string]any{}, http.StatusOK)
	if handoff := fx.handoff(t, announcementID); handoff["status"] != handoffStatusPending || handoff["operation"] != handoffOperationPublish {
		t.Fatalf("retry must queue the publish again: %#v", handoff)
	}
	return announcementID, handoffID
}

// assertWithdrawnAtReceiver checks that the receiver got a same-key cancel
// after the stored publish and holds the occurrence as cancelled.
func (fx backlineHandoffFixture) assertWithdrawnAtReceiver(t *testing.T, announcementID string) {
	t.Helper()
	key := backlineOccurrenceKey(announcementID, fx.bindingID)
	last := fx.receiver.requests[len(fx.receiver.requests)-1]
	if last["operation"] != handoffOperationCancel || last["occurrenceKey"] != key || fx.receiver.states[key] != "cancelled" {
		t.Fatalf("Backline still holds the occurrence: state=%q requests=%v", fx.receiver.states[key], fx.receiver.requests)
	}
	for _, request := range fx.receiver.requests[1:] {
		if request["operation"] == handoffOperationPublish {
			t.Fatalf("a withdrawn publish was sent again: %v", fx.receiver.requests)
		}
	}
	if handoff := fx.handoff(t, announcementID); handoff["operation"] != handoffOperationCancel || handoff["status"] != handoffStatusAccepted || handoff["receiverStatus"] != "cancelled" {
		t.Fatalf("unexpected cancel outcome: %#v", handoff)
	}
}

// TestBacklineHandoffJourney is the organizer journey from issue #221:
// preview, deliberate handoff, signed delivery, acknowledgement, and a signed
// delivery receipt read back through the announcement outcome ledger.
func TestBacklineHandoffJourney(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	created := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements"), map[string]any{"subject": "Doors move to 8pm", "body": "Synthetic channel announcement."}, http.StatusOK)
	id := mustString(t, created.JSON, "id")

	draft := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-preview"), http.StatusOK).JSON)
	if blockers := draft["blockers"].([]any); len(blockers) != 1 || blockers[0] != "announcement_not_approved" {
		t.Fatalf("a draft must be blocked only by approval, got %#v", blockers)
	}
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusConflict)

	scheduledFor := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/schedule"), map[string]any{"scheduledFor": scheduledFor.Format(time.RFC3339)}, http.StatusOK)
	preview := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-preview"), http.StatusOK).JSON)
	binding := mustObject(t, preview["binding"])
	if len(preview["blockers"].([]any)) != 0 || binding["label"] != "Synthetic #announcements" || preview["mentionPolicy"] != "none" ||
		preview["subject"] != "Doors move to 8pm" || preview["deliverAt"] != scheduledFor.Format(time.RFC3339) || preview["approved"] != true {
		t.Fatalf("unexpected approved preview: %#v", preview)
	}

	first := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	again := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusOK)
	if mustString(t, first.JSON, "id") != mustString(t, again.JSON, "id") || mustString(t, first.JSON, "status") != handoffStatusPending {
		t.Fatalf("a repeated publish must return the same pending handoff: %v / %v", first.JSON, again.JSON)
	}
	var rows int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from announcement_channel_handoffs where announcement_id = $1`, id).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("expected exactly one durable handoff, got %d (%v)", rows, err)
	}

	if report := fx.runWorker(t); report.Accepted != 1 || report.Claimed != 1 {
		t.Fatalf("unexpected worker report: %+v", report)
	}
	if fx.receiver.posts != 1 || len(fx.receiver.requests) != 1 {
		t.Fatalf("receiver must schedule exactly one post, got %d posts from %d requests", fx.receiver.posts, len(fx.receiver.requests))
	}
	sent := fx.receiver.requests[0]
	encoded, _ := json.Marshal(sent)
	for _, forbidden := range []string{fx.email("owner"), fx.email("member"), "recipient", "audience", "workspaceId", "consent"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("handoff payload leaks %q: %s", forbidden, encoded)
		}
	}
	if sent["deliverAt"] != scheduledFor.Format(time.RFC3339) || mustObject(t, sent["binding"])["channelId"] != "998877665544332211" ||
		sent["occurrenceKey"] != backlineOccurrenceKey(id, fx.bindingID) {
		t.Fatalf("unexpected handoff payload: %s", encoded)
	}

	handoff := fx.handoff(t, id)
	if handoff["status"] != handoffStatusAccepted || handoff["receiverStatus"] != "accepted" || handoff["acknowledgedAt"] == nil {
		t.Fatalf("acknowledgement must reach the ledger: %#v", handoff)
	}
	if report := fx.runWorker(t); report.Claimed != 0 {
		t.Fatalf("an acknowledged handoff must not be resent: %+v", report)
	}

	receipt := backlineReceipt{Version: 1, Kind: backlineReceiptKind, OccurrenceKey: backlineOccurrenceKey(id, fx.bindingID),
		ContentRevision: handoff["contentRevision"].(string), Outcome: "delivered", ReceiverReference: "discord-message"}
	fx.postReceipt(t, "wrong-secret-wrong-secret-wrong-secret", receipt, http.StatusUnauthorized)
	mismatched := receipt
	mismatched.ContentRevision = "sha256:" + strings.Repeat("0", 64)
	fx.postReceipt(t, syntheticBacklineSecret, mismatched, http.StatusConflict)
	fx.postReceipt(t, syntheticBacklineSecret, receipt, http.StatusOK)
	fx.postReceipt(t, syntheticBacklineSecret, receipt, http.StatusOK)
	conflicting := receipt
	conflicting.Outcome = "failed"
	fx.postReceipt(t, syntheticBacklineSecret, conflicting, http.StatusConflict)
	if handoff := fx.handoff(t, id); handoff["deliveryOutcome"] != "delivered" || handoff["receiverReference"] != "discord-message" {
		t.Fatalf("delivery receipt must be read back: %#v", handoff)
	}
}

func TestBacklineHandoffReceiverRejectsBadSignature(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	config := fx.app.config
	config.BacklineHandoffSecret = "a-different-secret-than-the-receiver-holds"
	report, err := RunBacklineHandoffs(t.Context(), config, fx.app.db, 10, fx.server.Client())
	if err != nil || report.Failed != 1 {
		t.Fatalf("a badly signed handoff must be held as failed: %+v %v", report, err)
	}
	if fx.receiver.posts != 0 || len(fx.receiver.requests) != 0 {
		t.Fatal("receiver processed an unauthenticated handoff")
	}
	handoff := fx.handoff(t, id)
	if handoff["status"] != handoffStatusFailed || handoff["lastError"] != "invalid_signature" {
		t.Fatalf("the refusal must be visible: %#v", handoff)
	}
	if report := fx.runWorker(t); report.Claimed != 0 {
		t.Fatalf("a refused handoff must wait for an operator: %+v", report)
	}
	// After the secret is fixed, an operator retry delivers it.
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/channel-handoffs/"+mustString(t, handoff, "id")+"/retry"), map[string]any{}, http.StatusOK)
	if report := fx.runWorker(t); report.Accepted != 1 || fx.receiver.posts != 1 {
		t.Fatalf("the retried handoff must be accepted once: %+v posts=%d", report, fx.receiver.posts)
	}
}

// TestBacklineHandoffCancelAfterRetriedUncertainSendsCancel covers a cancel
// that lands after an operator retried an uncertain publish but before the
// worker resent it. Backline stored the first attempt, so a local
// cancellation alone would let it post at deliverAt.
func TestBacklineHandoffCancelAfterRetriedUncertainSendsCancel(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id, _ := fx.uncertainThenRetried(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/cancel"), map[string]any{}, http.StatusOK)
	if handoff := fx.handoff(t, id); handoff["operation"] != handoffOperationCancel || handoff["status"] != handoffStatusPending {
		t.Fatalf("a publish Backline may hold must become a cancel message: %#v", handoff)
	}
	if report := fx.runWorker(t); report.Accepted != 1 {
		t.Fatalf("the cancel message must be acknowledged: %+v", report)
	}
	fx.assertWithdrawnAtReceiver(t, id)
}

func TestBacklineRevocationAfterRetriedUncertainSendsCancel(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id, _ := fx.uncertainThenRetried(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/backline-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusOK)
	if report := fx.runWorker(t); report.Accepted != 1 {
		t.Fatalf("the cancel message must be acknowledged: %+v", report)
	}
	fx.assertWithdrawnAtReceiver(t, id)
}

// TestBacklineHandoffWorkerWithdrawsHeldPublishWithCancel covers the
// worker's own recheck: a claimed publish whose announcement was withdrawn
// behind the API (here by direct SQL) is turned into a cancel when Backline
// may hold an earlier attempt.
func TestBacklineHandoffWorkerWithdrawsHeldPublishWithCancel(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id, _ := fx.uncertainThenRetried(t)
	if _, err := fx.app.db.Exec(t.Context(), `update announcements set status = 'cancelled' where id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if report := fx.runWorker(t); report.Retrying != 1 || report.Accepted != 1 {
		t.Fatalf("the worker must convert the publish and send a cancel: %+v", report)
	}
	fx.assertWithdrawnAtReceiver(t, id)
}

// TestBacklineHandoffFinalReceiptResolvesUncertain: a delivered receipt
// proves Backline holds the occurrence, so the held uncertain state resolves
// and no retry is offered.
func TestBacklineHandoffFinalReceiptResolvesUncertain(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	created := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	fx.receiver.failAfterProcessing = 1
	fx.runWorker(t)
	receipt := backlineReceipt{Version: 1, Kind: backlineReceiptKind, OccurrenceKey: backlineOccurrenceKey(id, fx.bindingID),
		ContentRevision: mustString(t, created.JSON, "contentRevision"), Outcome: "delivered", ReceiverReference: "discord-message"}
	fx.postReceipt(t, syntheticBacklineSecret, receipt, http.StatusOK)
	handoff := fx.handoff(t, id)
	if handoff["status"] != handoffStatusAccepted || handoff["lastError"] != nil || handoff["acknowledgedAt"] == nil || handoff["deliveryOutcome"] != "delivered" {
		t.Fatalf("a delivered receipt must resolve the uncertain transport state: %#v", handoff)
	}
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/channel-handoffs/"+mustString(t, handoff, "id")+"/retry"), map[string]any{}, http.StatusConflict)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/schedule"), map[string]any{"scheduledFor": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)}, http.StatusConflict)
}

func TestBacklineReceiptRejectsStaleAndCrossScopeSignatures(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	created := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	body, _ := json.Marshal(backlineReceipt{Version: 1, Kind: backlineReceiptKind, OccurrenceKey: backlineOccurrenceKey(id, fx.bindingID),
		ContentRevision: mustString(t, created.JSON, "contentRevision"), Outcome: "delivered"})
	stale := time.Now().Add(-backlineReplayWindow - time.Minute).Unix()
	fx.postSignedReceipt(t, body, syntheticBacklineSecret, backlineSignatureScopeReceipt, stale, http.StatusUnauthorized)
	future := time.Now().Add(backlineReplayWindow + time.Minute).Unix()
	fx.postSignedReceipt(t, body, syntheticBacklineSecret, backlineSignatureScopeReceipt, future, http.StatusUnauthorized)
	// A body signed for the handoff scope, as a captured handoff request
	// would be, is not a valid receipt.
	fx.postSignedReceipt(t, body, syntheticBacklineSecret, backlineSignatureScopeHandoff, time.Now().Unix(), http.StatusUnauthorized)
	if handoff := fx.handoff(t, id); handoff["deliveryOutcome"] != nil {
		t.Fatalf("a refused receipt must record nothing: %#v", handoff)
	}
	fx.postSignedReceipt(t, body, syntheticBacklineSecret, backlineSignatureScopeReceipt, time.Now().Unix(), http.StatusOK)
}

// TestBacklineHandoffUncertainRetryDeliversOnce covers a receiver that
// processed the request and then failed to answer: the handoff is held as
// uncertain, an explicit retry reuses the key, and the receiver posts once.
func TestBacklineHandoffUncertainRetryDeliversOnce(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	created := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	handoffID := mustString(t, created.JSON, "id")
	fx.receiver.failAfterProcessing = 1
	if report := fx.runWorker(t); report.Uncertain != 1 {
		t.Fatalf("a 500 after processing must be uncertain: %+v", report)
	}
	if report := fx.runWorker(t); report.Claimed != 0 {
		t.Fatalf("an uncertain handoff must be held, not retried automatically: %+v", report)
	}
	if handoff := fx.handoff(t, id); handoff["status"] != handoffStatusUncertain || handoff["lastError"] != "receiver_http_500" {
		t.Fatalf("uncertainty must stay visible: %#v", handoff)
	}
	postJSON(t, fx.app, fx.memberCookie, fx.path("/announcements/"+id+"/channel-handoffs/"+handoffID+"/retry"), map[string]any{}, http.StatusForbidden)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/channel-handoffs/"+handoffID+"/retry"), map[string]any{}, http.StatusOK)
	if report := fx.runWorker(t); report.Accepted != 1 {
		t.Fatalf("the explicit retry must be acknowledged: %+v", report)
	}
	if fx.receiver.posts != 1 || len(fx.receiver.requests) != 2 {
		t.Fatalf("duplicate retry must produce one post: posts=%d requests=%d", fx.receiver.posts, len(fx.receiver.requests))
	}
	if fx.receiver.requests[0]["occurrenceKey"] != fx.receiver.requests[1]["occurrenceKey"] {
		t.Fatal("retry must reuse the occurrence key")
	}
	if handoff := fx.handoff(t, id); handoff["status"] != handoffStatusAccepted || handoff["receiverStatus"] != "duplicate" {
		t.Fatalf("unexpected retried handoff: %#v", handoff)
	}
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/channel-handoffs/"+handoffID+"/retry"), map[string]any{}, http.StatusConflict)
}

func TestBacklineHandoffCancelBeforeDeliverySendsNothing(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/cancel"), map[string]any{}, http.StatusOK)
	if report := fx.runWorker(t); report.Claimed != 0 {
		t.Fatalf("a cancelled handoff must not be claimed: %+v", report)
	}
	if len(fx.receiver.requests) != 0 {
		t.Fatal("cancellation before delivery must send nothing")
	}
	if handoff := fx.handoff(t, id); handoff["status"] != handoffStatusCancelled || handoff["cancelRequestedAt"] == nil {
		t.Fatalf("unexpected cancelled handoff: %#v", handoff)
	}
}

func TestBacklineHandoffCancelAfterDeliverySendsCancelWithSameKey(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	fx.runWorker(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/cancel"), map[string]any{}, http.StatusOK)
	if report := fx.runWorker(t); report.Accepted != 1 {
		t.Fatalf("the cancel message must be acknowledged: %+v", report)
	}
	if len(fx.receiver.requests) != 2 {
		t.Fatalf("expected publish then cancel, got %d requests", len(fx.receiver.requests))
	}
	cancel := fx.receiver.requests[1]
	if cancel["operation"] != handoffOperationCancel || cancel["occurrenceKey"] != fx.receiver.requests[0]["occurrenceKey"] || cancel["content"] != nil {
		t.Fatalf("cancel must reuse the key and carry no content: %#v", cancel)
	}
	if handoff := fx.handoff(t, id); handoff["operation"] != handoffOperationCancel || handoff["status"] != handoffStatusAccepted || handoff["receiverStatus"] != "cancelled" {
		t.Fatalf("unexpected cancel outcome: %#v", handoff)
	}
}

func TestBacklineBindingRevocationRefusesNewHandoffs(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	pending := fx.scheduledAnnouncement(t)
	delivered := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+delivered+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	fx.runWorker(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+pending+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)

	postJSON(t, fx.app, fx.memberCookie, fx.path("/backline-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusForbidden)
	revoked := postJSON(t, fx.app, fx.ownerCookie, fx.path("/backline-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusOK)
	if mustObject(t, revoked.JSON)["active"] != false || mustObject(t, revoked.JSON)["revokedAt"] == nil {
		t.Fatalf("revoked binding must be kept and marked: %#v", revoked.JSON)
	}
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/backline-bindings/"+fx.bindingID+"/revoke"), map[string]any{}, http.StatusConflict)

	if handoff := fx.handoff(t, pending); handoff["status"] != handoffStatusCancelled {
		t.Fatalf("an unsent handoff must be cancelled on revocation: %#v", handoff)
	}
	fx.runWorker(t)
	if fx.receiver.posts != 1 || len(fx.receiver.requests) != 2 || fx.receiver.requests[1]["operation"] != handoffOperationCancel {
		t.Fatalf("revocation must withdraw only the delivered handoff: posts=%d requests=%v", fx.receiver.posts, fx.receiver.requests)
	}

	third := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+third+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusConflict)
	preview := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+third+"/backline-preview"), http.StatusOK).JSON)
	if preview["binding"] != nil || preview["blockers"].([]any)[0] != "no_active_binding" {
		t.Fatalf("preview must show no destination after revocation: %#v", preview)
	}
	listed := getJSON(t, fx.app, fx.ownerCookie, fx.path("/backline-bindings"), http.StatusOK).JSON.([]any)
	if len(listed) != 1 || mustObject(t, listed[0])["active"] != false {
		t.Fatalf("revoked binding must remain listed for audit: %#v", listed)
	}
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/backline-bindings"),
		map[string]any{"label": "Replacement", "serverId": "112233445566778899", "channelId": "112233445566778800"}, http.StatusCreated)
}

func TestBacklineHandoffAuthorityAndScope(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)

	// Crew holds neither manage_announcements nor manage_channel_bindings.
	getJSON(t, fx.app, fx.memberCookie, fx.path("/backline-bindings"), http.StatusForbidden)
	postJSON(t, fx.app, fx.memberCookie, fx.path("/backline-bindings"), map[string]any{"label": "x", "serverId": "112233445566778899", "channelId": "998877665544332211"}, http.StatusForbidden)
	getJSON(t, fx.app, fx.memberCookie, fx.path("/announcements/"+id+"/backline-preview"), http.StatusForbidden)
	postJSON(t, fx.app, fx.memberCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusForbidden)

	postJSON(t, fx.app, fx.ownerCookie, fx.path("/backline-bindings"), map[string]any{"label": "Second", "serverId": "112233445566778899", "channelId": "998877665544332200"}, http.StatusConflict)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/backline-bindings"), map[string]any{"label": "Bad", "serverId": "guild", "channelId": "998877665544332200"}, http.StatusBadRequest)

	// Another workspace's binding is not a valid destination here.
	other := newLifecycleFixture(t, fx.app)
	foreign := postJSON(t, fx.app, other.ownerCookie, "/api/workspaces/"+other.workspaceID+"/backline-bindings",
		map[string]any{"label": "Foreign", "serverId": "112233445566778899", "channelId": "998877665544332299"}, http.StatusCreated)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": mustString(t, foreign.JSON, "id")}, http.StatusNotFound)
	postJSON(t, fx.app, other.ownerCookie, "/api/workspaces/"+other.workspaceID+"/announcements/"+id+"/backline-handoff", map[string]any{"bindingId": mustString(t, foreign.JSON, "id")}, http.StatusNotFound)

	long := postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements"), map[string]any{"subject": "Long", "body": strings.Repeat("x", backlineMaxContentRunes)}, http.StatusOK)
	longID := mustString(t, long.JSON, "id")
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+longID+"/schedule"), map[string]any{"scheduledFor": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+longID+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusUnprocessableEntity)

	// Organizers hold both permissions.
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set role = 'organizer' where workspace_id = $1 and person_id = (select id from people where email = $2)`,
		fx.workspaceID, fx.email("member")); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.memberCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)

	fx.app.config.BacklineHandoffSecret = ""
	another := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+another+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusConflict)
	if _, err := RunBacklineHandoffs(t.Context(), fx.app.config, fx.app.db, 10, nil); err == nil {
		t.Fatal("the worker must refuse to run without a configured receiver")
	}
}

func TestBacklineHandoffRescheduleFollowsUnsentAndBlocksSent(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	later := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/schedule"), map[string]any{"scheduledFor": later.Format(time.RFC3339)}, http.StatusOK)
	if handoff := fx.handoff(t, id); handoff["deliverAt"] != later.Format(time.RFC3339) {
		t.Fatalf("an unsent handoff must follow the new schedule: %#v", handoff)
	}
	fx.runWorker(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/schedule"), map[string]any{"scheduledFor": later.Add(time.Hour).Format(time.RFC3339)}, http.StatusConflict)
}

// A pending publish that Backline may already hold (retried after an
// uncertain attempt) keeps its time: the receiver deduplicates on the key and
// would ignore a new deliverAt.
func TestBacklineHandoffRescheduleRefusesRetriedUncertain(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id, _ := fx.uncertainThenRetried(t)
	before := fx.handoff(t, id)["deliverAt"]
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/schedule"), map[string]any{"scheduledFor": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339)}, http.StatusConflict)
	if after := fx.handoff(t, id)["deliverAt"]; after != before {
		t.Fatalf("a refused reschedule must not move the handoff: %v -> %v", before, after)
	}
}

// TestBacklineHandoffRescheduleWaitsForConcurrentClaim holds the handoff row
// the way the worker's claim does, lets the reschedule start, then commits
// the claim. The reschedule must see the claim and refuse, rather than
// report success while Backline receives the old deliverAt.
func TestBacklineHandoffRescheduleWaitsForConcurrentClaim(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	claim, err := fx.app.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = claim.Rollback(t.Context()) }()
	if _, err := claim.Exec(t.Context(), `update announcement_channel_handoffs set status = 'sending', attempt_count = 1, lease_expires_at = now() + interval '2 minutes' where announcement_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"scheduledFor": time.Now().Add(4 * time.Hour).UTC().Format(time.RFC3339)})
	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, fx.path("/announcements/"+id+"/schedule"), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(fx.ownerCookie)
		rec := httptest.NewRecorder()
		fx.app.Handler().ServeHTTP(rec, req)
		done <- rec.Code
	}()
	// Wait until the reschedule is blocked on the claimed row's lock.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		if err := fx.app.db.QueryRow(t.Context(), `select exists(select 1 from pg_stat_activity where datname = current_database() and pid <> pg_backend_pid()
			  and wait_event_type = 'Lock' and query like '%announcement_channel_handoffs%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case code := <-done:
			t.Fatalf("the reschedule finished with %d without waiting for the claimed row", code)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the reschedule never waited on the claimed handoff")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := claim.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != http.StatusConflict {
		t.Fatalf("a reschedule racing the worker's claim must be refused, got %d", code)
	}
	var announcementTime, deliverAt time.Time
	if err := fx.app.db.QueryRow(t.Context(), `select a.scheduled_for, h.deliver_at from announcements a join announcement_channel_handoffs h on h.announcement_id = a.id where a.id = $1`, id).Scan(&announcementTime, &deliverAt); err != nil {
		t.Fatal(err)
	}
	if !announcementTime.Equal(deliverAt) {
		t.Fatalf("the refused reschedule must leave the announcement and handoff times equal: %v vs %v", announcementTime, deliverAt)
	}
}

func TestBacklineHandoffBoundedRetriesThenFail(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer busy.Close()
	config := fx.app.config
	config.BacklineHandoffURL = busy.URL
	for attempt := 1; attempt <= backlineMaxAttempts; attempt++ {
		if _, err := fx.app.db.Exec(t.Context(), `update announcement_channel_handoffs set next_attempt_at = now() where announcement_id = $1`, id); err != nil {
			t.Fatal(err)
		}
		report, err := RunBacklineHandoffs(t.Context(), config, fx.app.db, 10, busy.Client())
		if err != nil || report.Claimed != 1 {
			t.Fatalf("attempt %d: %+v %v", attempt, report, err)
		}
		if attempt < backlineMaxAttempts && report.Retrying != 1 || attempt == backlineMaxAttempts && report.Failed != 1 {
			t.Fatalf("attempt %d: unexpected report %+v", attempt, report)
		}
	}
	if handoff := fx.handoff(t, id); handoff["status"] != handoffStatusFailed || handoff["attemptCount"] != float64(backlineMaxAttempts) {
		t.Fatalf("retries must stop at the bound and stay visible: %#v", handoff)
	}
}

func TestBacklineHandoffExpiresUnsentOccurrence(t *testing.T) {
	fx := newBacklineHandoffFixture(t)
	id := fx.scheduledAnnouncement(t)
	postJSON(t, fx.app, fx.ownerCookie, fx.path("/announcements/"+id+"/backline-handoff"), map[string]any{"bindingId": fx.bindingID}, http.StatusCreated)
	if _, err := fx.app.db.Exec(t.Context(), `update announcement_channel_handoffs set deliver_at = now() - interval '2 days', expires_at = now() - interval '1 day' where announcement_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if report := fx.runWorker(t); report.Expired != 1 || report.Claimed != 0 {
		t.Fatalf("an expired handoff must not be sent: %+v", report)
	}
	if len(fx.receiver.requests) != 0 {
		t.Fatal("expired handoff reached the receiver")
	}
}
