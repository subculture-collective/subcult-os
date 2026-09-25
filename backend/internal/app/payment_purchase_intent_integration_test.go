package app

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type countingCheckoutProvider struct {
	calls    int
	response checkoutSessionResponse
}

type blockingCheckoutProvider struct {
	countingCheckoutProvider
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingCheckoutProvider) CreateCheckoutSession(ctx context.Context, request checkoutSessionRequest) (checkoutSessionResponse, error) {
	p.calls++
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		return p.response, nil
	case <-ctx.Done():
		return checkoutSessionResponse{}, ctx.Err()
	}
}

func (p *countingCheckoutProvider) CreateCheckoutSession(context.Context, checkoutSessionRequest) (checkoutSessionResponse, error) {
	p.calls++
	return p.response, nil
}

func TestPaidReservationPurchaseIntentReturnsOriginalReadyCheckout(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Intent Night", 1, "fixed", 1500, "usd")
	provider := &countingCheckoutProvider{response: checkoutSessionResponse{ID: testStripeSessionID(t, "cs_purchase_intent"), URL: "https://checkout.example/purchase-intent"}}
	fx.app.payments = provider
	slug := mustString(t, publishEvent(t, fx, mustString(t, event, "id")), "publicSlug")
	key := uuid.NewString()
	payload := map[string]any{"email": fx.email("intent"), "displayName": "Intent", "purchaseIntentKey": key}
	first := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", payload, http.StatusOK)
	second := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", payload, http.StatusOK)
	if provider.calls != 1 || mustString(t, first.JSON, "ticketId") != mustString(t, second.JSON, "ticketId") || mustString(t, second.JSON, "checkoutUrl") != provider.response.URL || mustString(t, second.JSON, "checkoutStatus") != "ready" {
		t.Fatalf("retry created provider work or changed checkout: calls=%d first=%#v second=%#v", provider.calls, first.JSON, second.JSON)
	}
	var tickets, attempts int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from tickets), (select count(*) from payment_checkout_attempts where purchase_intent_key=$1::uuid)`, key).Scan(&tickets, &attempts); err != nil {
		t.Fatal(err)
	}
	if tickets != 1 || attempts != 1 {
		t.Fatalf("purchase intent rows tickets=%d attempts=%d", tickets, attempts)
	}
}

func TestPaidReservationPurchaseIntentDoesNotRetryUnknownProviderWork(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Unknown Intent", 1, "fixed", 1500, "usd")
	fx.app.payments = failingCheckoutProvider{}
	slug := mustString(t, publishEvent(t, fx, mustString(t, event, "id")), "publicSlug")
	key := uuid.NewString()
	payload := map[string]any{"email": fx.email("unknown-intent"), "displayName": "Unknown", "purchaseIntentKey": key}
	postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", payload, http.StatusBadGateway)
	provider := &countingCheckoutProvider{response: checkoutSessionResponse{ID: testStripeSessionID(t, "cs_should_not_run"), URL: "https://checkout.example/should-not-run"}}
	fx.app.payments = provider
	retry := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", payload, http.StatusAccepted)
	if provider.calls != 0 || mustString(t, retry.JSON, "checkoutStatus") != "pending_reconciliation" {
		t.Fatalf("unknown retry called provider or lost pending status: calls=%d body=%#v", provider.calls, retry.JSON)
	}
}

func TestPaidReservationPurchaseIntentRejectsPresentInvalidKeys(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Intent validation", 2, "fixed", 1500, "usd")
	provider := &countingCheckoutProvider{response: checkoutSessionResponse{ID: testStripeSessionID(t, "cs_intent_validation"), URL: "https://checkout.example/intent-validation"}}
	fx.app.payments = provider
	slug := mustString(t, publishEvent(t, fx, mustString(t, event, "id")), "publicSlug")
	for _, key := range []string{"", "   ", "not-a-uuid"} {
		postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", map[string]any{"email": fx.email("invalid-intent"), "purchaseIntentKey": key}, http.StatusBadRequest)
	}
	if provider.calls != 0 {
		t.Fatalf("invalid purchase keys called provider %d times", provider.calls)
	}
}

func TestPaidReservationPurchaseIntentConcurrentRequestsCreateOneAttemptAndProviderCall(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Concurrent Intent", 1, "fixed", 1500, "usd")
	provider := &blockingCheckoutProvider{countingCheckoutProvider: countingCheckoutProvider{response: checkoutSessionResponse{ID: testStripeSessionID(t, "cs_intent_concurrent"), URL: "https://checkout.example/intent-concurrent"}}, started: make(chan struct{}), release: make(chan struct{})}
	fx.app.payments = provider
	slug := mustString(t, publishEvent(t, fx, mustString(t, event, "id")), "publicSlug")
	payload := map[string]any{"email": fx.email("concurrent-intent"), "displayName": "Concurrent", "purchaseIntentKey": uuid.NewString()}
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", payload, http.StatusOK)
	}()
	select {
	case <-provider.started:
	case <-time.After(10 * time.Second):
		t.Fatal("first purchase never reached provider")
	}
	second := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", payload, http.StatusAccepted)
	if mustString(t, second.JSON, "checkoutStatus") != "pending_reconciliation" || provider.calls != 1 {
		t.Fatalf("concurrent retry=%#v provider calls=%d", second.JSON, provider.calls)
	}
	close(provider.release)
	select {
	case <-firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("first purchase did not finish")
	}
	var tickets, attempts int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from tickets), (select count(*) from payment_checkout_attempts)`).Scan(&tickets, &attempts); err != nil {
		t.Fatal(err)
	}
	if tickets != 1 || attempts != 1 {
		t.Fatalf("concurrent purchase rows tickets=%d attempts=%d", tickets, attempts)
	}
}

func TestPaidReservationPurchaseIntentRejectsPayloadAndCrossEventReuse(t *testing.T) {
	fx := newLifecycleFixture(t)
	firstEvent := createEventWithPricing(t, fx, "First Intent", 2, "fixed", 1500, "usd")
	secondEvent := createEventWithPricing(t, fx, "Second Intent", 2, "fixed", 1500, "usd")
	provider := &countingCheckoutProvider{response: checkoutSessionResponse{ID: testStripeSessionID(t, "cs_intent_conflict"), URL: "https://checkout.example/intent-conflict"}}
	fx.app.payments = provider
	firstSlug := mustString(t, publishEvent(t, fx, mustString(t, firstEvent, "id")), "publicSlug")
	secondSlug := mustString(t, publishEvent(t, fx, mustString(t, secondEvent, "id")), "publicSlug")
	key := uuid.NewString()
	payload := map[string]any{"email": fx.email("conflict-intent"), "displayName": "Original", "purchaseIntentKey": key}
	postJSON(t, fx.app, nil, "/api/public/events/"+firstSlug+"/paid-reservations", payload, http.StatusOK)
	postJSON(t, fx.app, nil, "/api/public/events/"+firstSlug+"/paid-reservations", map[string]any{"email": fx.email("changed-intent"), "displayName": "Original", "purchaseIntentKey": key}, http.StatusConflict)
	postJSON(t, fx.app, nil, "/api/public/events/"+secondSlug+"/paid-reservations", payload, http.StatusConflict)
	if provider.calls != 1 {
		t.Fatalf("conflicting intent invoked provider %d times", provider.calls)
	}
}

func TestPaidReservationPurchaseIntentConcurrentCrossEventCollisionReturnsConflict(t *testing.T) {
	fx := newLifecycleFixture(t)
	firstEvent := createEventWithPricing(t, fx, "First collision", 1, "fixed", 1500, "usd")
	secondEvent := createEventWithPricing(t, fx, "Second collision", 1, "fixed", 1500, "usd")
	provider := &countingCheckoutProvider{response: checkoutSessionResponse{ID: testStripeSessionID(t, "cs_intent_collision"), URL: "https://checkout.example/intent-collision"}}
	fx.app.payments = provider
	firstSlug := mustString(t, publishEvent(t, fx, mustString(t, firstEvent, "id")), "publicSlug")
	secondSlug := mustString(t, publishEvent(t, fx, mustString(t, secondEvent, "id")), "publicSlug")
	blocker, err := fx.app.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(t.Context())
	if _, err := blocker.Exec(t.Context(), `select pg_advisory_xact_lock(2026092501)`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		create function pause_purchase_intent_collision() returns trigger language plpgsql as $$
		begin perform pg_advisory_xact_lock(2026092501); return new; end $$;
		create trigger pause_purchase_intent_collision before insert on payment_checkout_attempts
		for each row execute function pause_purchase_intent_collision()
	`); err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	result := make(chan int, 2)
	for _, request := range []struct{ slug, email string }{{firstSlug, fx.email("collision-one")}, {secondSlug, fx.email("collision-two")}} {
		go func(slug, email string) {
			response := requestTicketCapacityReservation(fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", map[string]any{"email": email, "purchaseIntentKey": key})
			result <- response.status
		}(request.slug, request.email)
	}
	deadline := time.Now().Add(10 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		var waiting int
		if err := fx.app.db.QueryRow(t.Context(), `select count(*) from pg_stat_activity where datname=current_database() and wait_event_type='Lock' and query like '%payment_checkout_attempts%'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= 2 {
			blocked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("timed out waiting for concurrent purchase-intent inserts")
	}
	if err := blocker.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	first, second := <-result, <-result
	if !((first == http.StatusOK && second == http.StatusConflict) || (second == http.StatusOK && first == http.StatusConflict)) || provider.calls != 1 {
		t.Fatalf("cross-event collision statuses=%d/%d provider calls=%d", first, second, provider.calls)
	}
}

func TestPaidReservationPurchaseIntentPreservesLegacyCallersAndTerminalStates(t *testing.T) {
	fx := newLifecycleFixture(t)
	event := createEventWithPricing(t, fx, "Intent states", 5, "fixed", 1500, "usd")
	provider := &countingCheckoutProvider{response: checkoutSessionResponse{ID: testStripeSessionID(t, "cs_intent_states"), URL: "https://checkout.example/intent-states"}}
	fx.app.payments = provider
	slug := mustString(t, publishEvent(t, fx, mustString(t, event, "id")), "publicSlug")
	legacy := map[string]any{"email": fx.email("legacy"), "displayName": "Legacy"}
	firstLegacy := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", legacy, http.StatusOK)
	provider.response.ID = testStripeSessionID(t, "cs_intent_states_second")
	secondLegacy := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", legacy, http.StatusOK)
	if mustString(t, firstLegacy.JSON, "ticketId") == mustString(t, secondLegacy.JSON, "ticketId") {
		t.Fatal("legacy callers unexpectedly shared a purchase intent")
	}
	for _, terminal := range []struct {
		status string
		want   int
		label  string
	}{{"fulfilled", http.StatusOK, "paid"}, {"expired", http.StatusConflict, "expired"}, {"anomalous", http.StatusConflict, "reconciliation_required"}} {
		provider.response.ID = testStripeSessionID(t, "cs_intent_"+terminal.status)
		key := uuid.NewString()
		payload := map[string]any{"email": fx.email(terminal.status), "displayName": terminal.status, "purchaseIntentKey": key}
		postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", payload, http.StatusOK)
		if _, err := fx.app.db.Exec(t.Context(), `update payment_checkout_attempts set status=$2 where purchase_intent_key=$1::uuid`, key, terminal.status); err != nil {
			t.Fatal(err)
		}
		response := postJSON(t, fx.app, nil, "/api/public/events/"+slug+"/paid-reservations", payload, terminal.want)
		if mustString(t, response.JSON, "checkoutStatus") != terminal.label {
			t.Fatalf("terminal %s response=%#v", terminal.status, response.JSON)
		}
	}
}

func TestPaymentPurchaseIntentMigrationUpgradesVersionNineteenFixture(t *testing.T) {
	ctx := t.Context()
	db := newMigrationTestPool(t)
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 20 {
		t.Fatalf("migrations=%d, want at least 20", len(migrations))
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `create table schema_migrations(version integer primary key,name text not null,checksum char(64) not null,applied_at timestamptz not null default now())`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:19] {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			t.Fatalf("apply historical migration %d: %v", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version,name,checksum) values($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(ctx, db); err != nil {
		t.Fatalf("upgrade version-19 fixture: %v", err)
	}
	var keyColumn, urlColumn bool
	if err := db.QueryRow(ctx, `
		select exists(select 1 from information_schema.columns where table_schema=current_schema() and table_name='payment_checkout_attempts' and column_name='purchase_intent_key'),
		       exists(select 1 from information_schema.columns where table_schema=current_schema() and table_name='payment_checkout_attempts' and column_name='provider_checkout_url')
	`).Scan(&keyColumn, &urlColumn); err != nil {
		t.Fatal(err)
	}
	if !keyColumn || !urlColumn {
		t.Fatalf("purchase intent upgrade columns missing key=%v url=%v", keyColumn, urlColumn)
	}
}
