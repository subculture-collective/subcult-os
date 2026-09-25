package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type publicEventDTO struct {
	ID                string  `json:"id"`
	Title             string  `json:"title"`
	StartsAt          string  `json:"startsAt"`
	PublicDescription string  `json:"publicDescription"`
	LocationDisplay   string  `json:"locationDisplay"`
	ImageURL          *string `json:"imageUrl"`
	PricingMode       string  `json:"pricingMode"`
	TicketPriceCents  int     `json:"ticketPriceCents"`
	TicketCurrency    string  `json:"ticketCurrency"`
	Status            string  `json:"status"`
	PublicSlug        string  `json:"publicSlug"`
	PublicURL         string  `json:"publicUrl"`
	RemainingTickets  int     `json:"remainingTickets"`
	IsFull            bool    `json:"isFull"`
}

type ticketDTO struct {
	ID            string  `json:"id"`
	EventID       string  `json:"eventId"`
	Email         string  `json:"email"`
	DisplayName   *string `json:"displayName"`
	Code          string  `json:"code"`
	TicketURL     string  `json:"ticketUrl"`
	Status        string  `json:"status"`
	PaymentStatus string  `json:"paymentStatus"`
	AmountCents   int     `json:"amountCents"`
	Currency      string  `json:"currency"`
	CheckedInAt   *string `json:"checkedInAt"`
}

// doorTicketDTO contains only the fields needed to identify an attendee and
// decide admission at the door. It intentionally omits contact, receipt, and
// financial fields from the operator door endpoints.
type doorTicketDTO struct {
	ID                string  `json:"id"`
	Code              string  `json:"code"`
	DisplayName       *string `json:"displayName"`
	AdmissionEligible bool    `json:"admissionEligible"`
	Status            string  `json:"status"`
	CheckedInAt       *string `json:"checkedInAt"`
}

type reserveTicketRequest struct {
	Email             string  `json:"email"`
	DisplayName       *string `json:"displayName"`
	PurchaseIntentKey *string `json:"purchaseIntentKey"`
}

type createTestTicketRequest struct {
	Email       string  `json:"email"`
	DisplayName *string `json:"displayName"`
}

type ticketRow struct {
	ID            string
	EventID       string
	Email         string
	DisplayName   sql.NullString
	Code          string
	Status        string
	PaymentStatus string
	AmountCents   int
	Currency      string
	CheckedInAt   sql.NullTime
}

type reservedTicketResponse struct {
	ticketDTO
	TicketURL string `json:"ticketUrl"`
}

type paidReservationResponse struct {
	TicketID          string `json:"ticketId"`
	TicketCode        string `json:"ticketCode"`
	TicketURL         string `json:"ticketUrl"`
	CheckoutSessionID string `json:"checkoutSessionId"`
	CheckoutURL       string `json:"checkoutUrl"`
	CheckoutStatus    string `json:"checkoutStatus"`
}

func (a *App) handlePublicEvent(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	event, err := a.loadPublishedEventBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	writeJSON(w, http.StatusOK, a.publicEventDTOFromRow(event))
}

func (a *App) publicEventDTOFromRow(event eventRow) publicEventDTO {
	remaining := ticketJourneyCapacity(event.TicketAllocation, event.ReservedCount)
	slug := event.PublicSlug.String
	return publicEventDTO{
		ID:                event.ID,
		Title:             event.Title,
		StartsAt:          event.StartsAt.UTC().Format(time.RFC3339Nano),
		PublicDescription: event.PublicDescription,
		LocationDisplay:   event.LocationDisplay,
		ImageURL:          nullableString(event.ImageURL),
		PricingMode:       event.PricingMode,
		TicketPriceCents:  event.TicketPriceCents,
		TicketCurrency:    event.TicketCurrency,
		Status:            eventStatusPublished,
		PublicSlug:        slug,
		PublicURL:         a.publicEventURL(slug),
		RemainingTickets:  remaining,
		IsFull:            ticketJourneyIsFull(event.TicketAllocation, event.ReservedCount),
	}
}

func (a *App) handleReserveTicket(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}

	var req reserveTicketRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	email := normalizeEmail(req.Email)
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	displayName := normalizeDisplayName(req.DisplayName)

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var event eventRow
	err = tx.QueryRow(r.Context(), `
		select e.id, e.workspace_id, e.title, e.starts_at, e.public_description, e.location_display,
		       e.ticket_allocation, e.pricing_mode, e.ticket_price_cents, e.ticket_currency, e.status, e.public_slug,
		       (select count(*) from tickets t where t.event_id = e.id and t.payment_status <> 'cancelled') as reserved_count,
		       (select count(*) from tickets t where t.event_id = e.id and t.status = 'checked_in' and t.payment_status <> 'cancelled') as checked_in_count
		from events e
		where e.public_slug = $1
		  and e.status = 'published'
		for update
	`, r.PathValue("slug")).Scan(&event.ID, &event.WorkspaceID, &event.Title, &event.StartsAt, &event.PublicDescription, &event.LocationDisplay, &event.TicketAllocation, &event.PricingMode, &event.TicketPriceCents, &event.TicketCurrency, &event.Status, &event.PublicSlug, &event.ReservedCount, &event.CheckedInCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	if event.ReservedCount, err = loadReservedTicketCount(r.Context(), tx, event.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ticket capacity")
		return
	}
	if ticketJourneyIsFull(event.TicketAllocation, event.ReservedCount) {
		writeError(w, http.StatusConflict, "event is full")
		return
	}
	if !ticketJourneyCanReservePublic(event.PricingMode, event.TicketAllocation, event.ReservedCount) {
		writeError(w, http.StatusConflict, "paid checkout is required for this event")
		return
	}

	code, err := newTicketCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create ticket code")
		return
	}
	var ticket ticketRow
	if err := tx.QueryRow(r.Context(), `
		insert into tickets (event_id, email, display_name, code, status)
		values ($1, $2, $3, $4, 'reserved')
		returning id, event_id, email, display_name, code, status, payment_status, amount_cents, currency, checked_in_at
	`, event.ID, email, displayName, code).Scan(&ticket.ID, &ticket.EventID, &ticket.Email, &ticket.DisplayName, &ticket.Code, &ticket.Status, &ticket.PaymentStatus, &ticket.AmountCents, &ticket.Currency, &ticket.CheckedInAt); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create ticket")
		return
	}

	ticketURL := a.publicTicketURL(ticket.Code)
	body := fmt.Sprintf("Your ticket for %s\n\nView your ticket: %s", event.Title, ticketURL)
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, "", "ticket.reserved", "ticket", ticket.ID, map[string]any{
		"eventId":     event.ID,
		"email":       email,
		"displayName": displayName,
		"ticketUrl":   ticketURL,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := a.enqueueEmail(txCtx, email, "Your ticket for "+event.Title, body, "ticket", ticket.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not enqueue ticket email")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save ticket")
		return
	}

	writeJSON(w, http.StatusOK, reservedTicketResponse{ticketDTO: a.ticketDTOFromRow(ticket), TicketURL: ticketURL})
}

func (a *App) handleCreatePaidReservation(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if a.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "payment provider unavailable")
		return
	}

	var req reserveTicketRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	email := normalizeEmail(req.Email)
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	displayName := normalizeDisplayName(req.DisplayName)
	purchaseIntentKey := uuid.NewString()
	if req.PurchaseIntentKey != nil {
		if strings.TrimSpace(*req.PurchaseIntentKey) == "" {
			writeError(w, http.StatusBadRequest, "invalid purchase intent")
			return
		}
		parsed, err := uuid.Parse(strings.TrimSpace(*req.PurchaseIntentKey))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid purchase intent")
			return
		}
		purchaseIntentKey = parsed.String()
	}
	displayNameValue := ""
	if displayName != nil {
		displayNameValue = *displayName
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var event eventRow
	err = tx.QueryRow(r.Context(), `
		select e.id, e.workspace_id, e.title, e.starts_at, e.public_description, e.location_display,
		       e.ticket_allocation, e.pricing_mode, e.ticket_price_cents, e.ticket_currency, e.status, e.public_slug,
		       (select count(*) from tickets t where t.event_id = e.id and t.payment_status <> 'cancelled') as reserved_count,
		       (select count(*) from tickets t where t.event_id = e.id and t.status = 'checked_in' and t.payment_status <> 'cancelled') as checked_in_count
		from events e
		where e.public_slug = $1
		  and e.status = 'published'
		for update
	`, r.PathValue("slug")).Scan(&event.ID, &event.WorkspaceID, &event.Title, &event.StartsAt, &event.PublicDescription, &event.LocationDisplay, &event.TicketAllocation, &event.PricingMode, &event.TicketPriceCents, &event.TicketCurrency, &event.Status, &event.PublicSlug, &event.ReservedCount, &event.CheckedInCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	if existing, found, err := loadPaidPurchaseIntent(r.Context(), tx, purchaseIntentKey); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load purchase intent")
		return
	} else if found {
		if existing.EventID != event.ID || existing.Email != email || existing.DisplayName != displayNameValue || existing.AmountCents != event.TicketPriceCents || !strings.EqualFold(existing.Currency, event.TicketCurrency) {
			writeError(w, http.StatusConflict, "purchase intent conflicts with an existing checkout")
			return
		}
		status, response := existing.response(a.publicTicketURL(existing.TicketCode))
		writeJSON(w, status, response)
		return
	}
	if event.ReservedCount, err = loadReservedTicketCount(r.Context(), tx, event.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ticket capacity")
		return
	}
	if !ticketJourneyCanCreatePaidReservation(event.PricingMode, event.TicketAllocation, event.ReservedCount) {
		if !strings.EqualFold(strings.TrimSpace(event.PricingMode), "fixed") {
			writeError(w, http.StatusConflict, "paid checkout is only available for fixed-price events")
			return
		}
		writeError(w, http.StatusConflict, "event is full")
		return
	}

	code, err := newTicketCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create ticket code")
		return
	}
	var ticket ticketRow
	if err := tx.QueryRow(r.Context(), `
		insert into tickets (event_id, email, display_name, code, status, payment_status, amount_cents, currency)
		values ($1, $2, $3, $4, 'reserved', 'pending', $5, $6)
		returning id, event_id, email, display_name, code, status, payment_status, amount_cents, currency, checked_in_at
	`, event.ID, email, displayName, code, event.TicketPriceCents, event.TicketCurrency).Scan(&ticket.ID, &ticket.EventID, &ticket.Email, &ticket.DisplayName, &ticket.Code, &ticket.Status, &ticket.PaymentStatus, &ticket.AmountCents, &ticket.Currency, &ticket.CheckedInAt); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create ticket")
		return
	}

	// Commit the local authority to create a checkout before making an
	// irreversible provider call.  This reservation is retained if the call or
	// its follow-up persistence is ambiguous.
	var attemptID, idempotencyKey string
	if err := tx.QueryRow(r.Context(), `
		insert into payment_checkout_attempts (ticket_id, provider, provider_idempotency_key, purchase_intent_key)
		values ($1, 'stripe', 'checkout-attempt-' || gen_random_uuid()::text, $2)
		returning id, provider_idempotency_key
	`, ticket.ID, purchaseIntentKey).Scan(&attemptID, &idempotencyKey); err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			writeError(w, http.StatusConflict, "purchase intent conflicts with an existing checkout")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create checkout attempt")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save ticket")
		return
	}

	ticketURL := a.publicTicketURL(ticket.Code)
	checkout, err := a.payments.CreateCheckoutSession(r.Context(), checkoutSessionRequest{
		TicketID:               ticket.ID,
		EventID:                event.ID,
		CheckoutAttemptID:      attemptID,
		ProviderIdempotencyKey: idempotencyKey,
		EventTitle:             event.Title,
		AmountCents:            event.TicketPriceCents,
		Currency:               event.TicketCurrency,
		SuccessURL:             ticketURL + "?checkout=success",
		CancelURL:              a.publicEventURL(a.publicSlugValue(event)) + "?checkout=cancelled",
	})
	if err != nil || strings.TrimSpace(checkout.ID) == "" || strings.TrimSpace(checkout.URL) == "" {
		a.markCheckoutAttemptUnknown(r.Context(), attemptID, "provider_unknown")
		writeError(w, http.StatusBadGateway, "could not create checkout session")
		return
	}

	bindTx, err := a.db.Begin(r.Context())
	if err != nil {
		a.markCheckoutAttemptUnknown(r.Context(), attemptID, "persistence_failed")
		writeError(w, http.StatusInternalServerError, "could not save checkout session")
		return
	}
	defer func() { _ = bindTx.Rollback(r.Context()) }()
	var lockedTicketID string
	if err := bindTx.QueryRow(r.Context(), `
		select id from tickets
		where id = $1 and payment_status = 'pending' and stripe_checkout_session_id is null
		for update
	`, ticket.ID).Scan(&lockedTicketID); err != nil {
		_ = bindTx.Rollback(r.Context())
		var alreadyPaid bool
		if lookupErr := a.db.QueryRow(r.Context(), `select exists (select 1 from tickets where id = $1 and payment_status = 'paid' and stripe_checkout_session_id = $2)`, ticket.ID, checkout.ID).Scan(&alreadyPaid); lookupErr == nil && alreadyPaid {
			writeJSON(w, http.StatusOK, paidReservationResponse{TicketID: ticket.ID, TicketCode: ticket.Code, TicketURL: ticketURL, CheckoutSessionID: checkout.ID, CheckoutURL: checkout.URL, CheckoutStatus: "paid"})
			return
		}
		a.markCheckoutAttemptUnknown(r.Context(), attemptID, "persistence_failed")
		writeError(w, http.StatusInternalServerError, "could not save checkout session")
		return
	}
	var boundAttemptID string
	err = bindTx.QueryRow(r.Context(), `
		update payment_checkout_attempts a
		set provider_session_id = $2, provider_checkout_url = $3, status = 'ready', ready_at = now(), updated_at = now(), last_error_code = null
		where a.id = $1 and a.ticket_id = $4 and a.provider = 'stripe'
		  and a.status in ('creating', 'unknown')
		returning a.id
	`, attemptID, checkout.ID, checkout.URL, ticket.ID).Scan(&boundAttemptID)
	if err == nil {
		_, err = bindTx.Exec(r.Context(), `update tickets set stripe_checkout_session_id = $2 where id = $1 and stripe_checkout_session_id is null`, ticket.ID, checkout.ID)
	}
	if err != nil {
		_ = bindTx.Rollback(r.Context())
		a.markCheckoutAttemptUnknown(r.Context(), attemptID, "persistence_failed")
		writeError(w, http.StatusInternalServerError, "could not save checkout session")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, bindTx)
	if err := a.audit(txCtx, "", "ticket.payment_started", "ticket", ticket.ID, map[string]any{
		"eventId":               event.ID,
		"email":                 email,
		"displayName":           displayName,
		"amountCents":           event.TicketPriceCents,
		"currency":              event.TicketCurrency,
		"paymentStatus":         "pending",
		"stripeCheckoutSession": checkout.ID,
	}); err != nil {
		_ = bindTx.Rollback(r.Context())
		a.markCheckoutAttemptUnknown(r.Context(), attemptID, "persistence_failed")
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := bindTx.Commit(r.Context()); err != nil {
		a.markCheckoutAttemptUnknown(r.Context(), attemptID, "persistence_failed")
		writeError(w, http.StatusInternalServerError, "could not save ticket")
		return
	}

	writeJSON(w, http.StatusOK, paidReservationResponse{TicketID: ticket.ID, TicketCode: ticket.Code, TicketURL: ticketURL, CheckoutSessionID: checkout.ID, CheckoutURL: checkout.URL, CheckoutStatus: "ready"})
}

type paidPurchaseIntent struct {
	EventID, Email, DisplayName, TicketID, TicketCode, Currency, Status, SessionID, CheckoutURL string
	AmountCents                                                                                 int
}

// loadPaidPurchaseIntent locks an existing purchase command before capacity is
// considered. A retry never invokes the provider: creating and unknown work
// remains pending reconciliation because its external outcome is uncertain.
func loadPaidPurchaseIntent(ctx context.Context, tx pgx.Tx, key string) (paidPurchaseIntent, bool, error) {
	var intent paidPurchaseIntent
	var ticketID string
	err := tx.QueryRow(ctx, `select ticket_id from payment_checkout_attempts where purchase_intent_key = $1`, key).Scan(&ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return paidPurchaseIntent{}, false, nil
	}
	if err != nil {
		return paidPurchaseIntent{}, false, err
	}
	if err := tx.QueryRow(ctx, `
		select event_id, email, coalesce(display_name, ''), id, code, amount_cents, currency
		from tickets where id = $1 for update
	`, ticketID).Scan(&intent.EventID, &intent.Email, &intent.DisplayName, &intent.TicketID, &intent.TicketCode, &intent.AmountCents, &intent.Currency); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paidPurchaseIntent{}, false, nil
		}
		return paidPurchaseIntent{}, false, err
	}
	err = tx.QueryRow(ctx, `
		select status, coalesce(provider_session_id, ''), coalesce(provider_checkout_url, '')
		from payment_checkout_attempts where purchase_intent_key = $1 and ticket_id = $2 for update
	`, key, ticketID).Scan(&intent.Status, &intent.SessionID, &intent.CheckoutURL)
	return intent, err == nil, err
}

func (intent paidPurchaseIntent) response(ticketURL string) (int, paidReservationResponse) {
	response := paidReservationResponse{TicketID: intent.TicketID, TicketCode: intent.TicketCode, TicketURL: ticketURL, CheckoutSessionID: intent.SessionID}
	switch intent.Status {
	case "ready":
		if intent.CheckoutURL != "" {
			response.CheckoutURL, response.CheckoutStatus = intent.CheckoutURL, "ready"
			return http.StatusOK, response
		}
	case "fulfilled":
		response.CheckoutStatus = "paid"
		return http.StatusOK, response
	case "expired":
		response.CheckoutStatus = "expired"
		return http.StatusConflict, response
	case "anomalous":
		response.CheckoutStatus = "reconciliation_required"
		return http.StatusConflict, response
	}
	response.CheckoutStatus = "pending_reconciliation"
	return http.StatusAccepted, response
}

func (a *App) markCheckoutAttemptUnknown(ctx context.Context, attemptID, errorCode string) {
	if a.db == nil || strings.TrimSpace(attemptID) == "" {
		return
	}
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, _ = a.db.Exec(reconcileCtx, `
		update payment_checkout_attempts
		set status = 'unknown', unknown_at = now(), updated_at = now(), last_error_code = $2
		where id = $1 and status in ('creating', 'ready', 'unknown')
	`, attemptID, errorCode)
}

func (a *App) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	ticket, err := a.loadTicketByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load ticket")
		return
	}
	writeJSON(w, http.StatusOK, a.ticketDTOFromRow(ticket))
}

func (a *App) handleCreateTestTicket(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}

	var req createTestTicketRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" {
		email = "test-ticket@subcult.local"
	}
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "email must be valid")
		return
	}
	displayName := normalizeDisplayName(req.DisplayName)
	if displayName == nil {
		value := "Test Ticket"
		displayName = &value
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var event eventRow
	err = tx.QueryRow(r.Context(), `
		select e.id, e.workspace_id, e.title, e.starts_at, e.public_description, e.location_display,
		       e.ticket_allocation, e.pricing_mode, e.ticket_price_cents, e.ticket_currency, e.status, e.public_slug,
		       (select count(*) from tickets t where t.event_id = e.id and t.payment_status <> 'cancelled') as reserved_count,
		       (select count(*) from tickets t where t.event_id = e.id and t.status = 'checked_in' and t.payment_status <> 'cancelled') as checked_in_count
		from events e
		where e.id = $1
		for update
	`, r.PathValue("eventID")).Scan(&event.ID, &event.WorkspaceID, &event.Title, &event.StartsAt, &event.PublicDescription, &event.LocationDisplay, &event.TicketAllocation, &event.PricingMode, &event.TicketPriceCents, &event.TicketCurrency, &event.Status, &event.PublicSlug, &event.ReservedCount, &event.CheckedInCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	if event.ReservedCount, err = loadReservedTicketCount(r.Context(), tx, event.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ticket capacity")
		return
	}
	actorID, _, ok := a.requireWorkspaceRole(r, event.WorkspaceID, "owner", "member")
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if ticketJourneyIsFull(event.TicketAllocation, event.ReservedCount) {
		writeError(w, http.StatusConflict, "event is full")
		return
	}

	code, err := newTicketCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create ticket code")
		return
	}
	var ticket ticketRow
	if err := tx.QueryRow(r.Context(), `
		insert into tickets (event_id, email, display_name, code, status, payment_status, amount_cents, currency)
		values ($1, $2, $3, $4, 'reserved', 'free', 0, $5)
		returning id, event_id, email, display_name, code, status, payment_status, amount_cents, currency, checked_in_at
	`, event.ID, email, displayName, code, event.TicketCurrency).Scan(&ticket.ID, &ticket.EventID, &ticket.Email, &ticket.DisplayName, &ticket.Code, &ticket.Status, &ticket.PaymentStatus, &ticket.AmountCents, &ticket.Currency, &ticket.CheckedInAt); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create test ticket")
		return
	}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "ticket.test_created", "ticket", ticket.ID, map[string]any{
		"eventId": event.ID,
		"email":   email,
		"code":    ticket.Code,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save test ticket")
		return
	}

	writeJSON(w, http.StatusOK, a.ticketDTOFromRow(ticket))
}

func (a *App) handleDoorTicketSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	event, err := a.loadEventDetails(r.Context(), r.PathValue("eventID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	if _, ok := a.requirePermission(r, event.WorkspaceID, permDoor); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("query")))
	if query == "" {
		writeJSON(w, http.StatusOK, []doorTicketDTO{})
		return
	}

	rows, err := a.db.Query(r.Context(), `
		select id, event_id, email, display_name, code, status, payment_status, amount_cents, currency, checked_in_at
		from tickets
		where event_id = $1
		  and payment_status in ('free', 'paid')
		  and (
			lower(email) like '%' || $2 || '%'
			or lower(coalesce(display_name, '')) like '%' || $2 || '%'
			or lower(code) like '%' || $2 || '%'
		  )
		order by created_at desc
	`, event.ID, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not search tickets")
		return
	}
	defer rows.Close()

	tickets := make([]doorTicketDTO, 0)
	for rows.Next() {
		var ticket ticketRow
		if err := rows.Scan(&ticket.ID, &ticket.EventID, &ticket.Email, &ticket.DisplayName, &ticket.Code, &ticket.Status, &ticket.PaymentStatus, &ticket.AmountCents, &ticket.Currency, &ticket.CheckedInAt); err != nil {
			writeError(w, http.StatusInternalServerError, "could not search tickets")
			return
		}
		tickets = append(tickets, a.doorTicketDTOFromRow(ticket))
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not search tickets")
		return
	}
	if _, ok := a.requirePermission(r, event.WorkspaceID, permDoor); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	writeJSON(w, http.StatusOK, tickets)
}

func (a *App) handleDoorCheckIn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.db == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	event, err := a.loadEventDetails(r.Context(), r.PathValue("eventID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	actorID, ok := a.requirePermission(r, event.WorkspaceID, permDoor)
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start transaction")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var ticket ticketRow
	err = tx.QueryRow(r.Context(), `
		select id, event_id, email, display_name, code, status, payment_status, amount_cents, currency, checked_in_at
		from tickets
		where code = $1
		for update
	`, code).Scan(&ticket.ID, &ticket.EventID, &ticket.Email, &ticket.DisplayName, &ticket.Code, &ticket.Status, &ticket.PaymentStatus, &ticket.AmountCents, &ticket.Currency, &ticket.CheckedInAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load ticket")
		return
	}
	if ticket.EventID != event.ID {
		writeError(w, http.StatusConflict, "ticket belongs to a different event")
		return
	}
	if actorID, ok = a.requirePermission(r, event.WorkspaceID, permDoor); !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if !ticketJourneyCanCheckIn(ticket.PaymentStatus) {
		writeError(w, http.StatusConflict, "ticket is not eligible for check-in")
		return
	}
	if ticketJourneyIsCheckedIn(ticket.Status) {
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save check-in")
			return
		}
		writeJSON(w, http.StatusOK, a.doorTicketDTOFromRow(ticket))
		return
	}

	checkedInAt := time.Now().UTC()
	if err := tx.QueryRow(r.Context(), `
		update tickets
		set status = 'checked_in',
		    checked_in_at = $2,
		    checked_in_by_person_id = $3
		where id = $1
		returning checked_in_at
	`, ticket.ID, checkedInAt, actorID).Scan(&ticket.CheckedInAt); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check in ticket")
		return
	}
	ticket.Status = "checked_in"
	if ticket.CheckedInAt.Valid {
		checkedInAt = ticket.CheckedInAt.Time.UTC()
	}
	ticket.CheckedInAt = sql.NullTime{Time: checkedInAt, Valid: true}
	txCtx := context.WithValue(r.Context(), txContextKey{}, tx)
	if err := a.audit(txCtx, actorID, "ticket.checked_in", "ticket", ticket.ID, map[string]any{
		"eventId": event.ID,
		"code":    ticket.Code,
		"email":   ticket.Email,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record audit")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save check-in")
		return
	}

	writeJSON(w, http.StatusOK, a.doorTicketDTOFromRow(ticket))
}

func (a *App) loadPublishedEventBySlug(ctx context.Context, slug string) (eventRow, error) {
	var row eventRow
	if slug == "" {
		return row, pgx.ErrNoRows
	}
	if err := a.db.QueryRow(ctx, `
		select e.id, e.workspace_id, e.title, e.starts_at, e.public_description, e.location_display, e.image_url,
		       e.ticket_allocation, e.pricing_mode, e.ticket_price_cents, e.ticket_currency, e.status, e.public_slug,
		       (select count(*) from tickets t where t.event_id = e.id and t.payment_status <> 'cancelled') as reserved_count,
		       (select count(*) from tickets t where t.event_id = e.id and t.status = 'checked_in' and t.payment_status <> 'cancelled') as checked_in_count
		from events e
		where e.public_slug = $1
		  and e.status = 'published'
	`, slug).Scan(&row.ID, &row.WorkspaceID, &row.Title, &row.StartsAt, &row.PublicDescription, &row.LocationDisplay, &row.ImageURL, &row.TicketAllocation, &row.PricingMode, &row.TicketPriceCents, &row.TicketCurrency, &row.Status, &row.PublicSlug, &row.ReservedCount, &row.CheckedInCount); err != nil {
		return eventRow{}, err
	}
	return row, nil
}

func (a *App) loadTicketByCode(ctx context.Context, code string) (ticketRow, error) {
	var row ticketRow
	if code == "" {
		return row, pgx.ErrNoRows
	}
	if err := a.db.QueryRow(ctx, `
		select id, event_id, email, display_name, code, status, payment_status, amount_cents, currency, checked_in_at
		from tickets
		where code = $1
	`, code).Scan(&row.ID, &row.EventID, &row.Email, &row.DisplayName, &row.Code, &row.Status, &row.PaymentStatus, &row.AmountCents, &row.Currency, &row.CheckedInAt); err != nil {
		return ticketRow{}, err
	}
	return row, nil
}

// loadReservedTicketCount runs after the caller has locked the event row. It
// must be a separate statement: a reservation statement that began before it
// waited on that row lock can otherwise retain a snapshot that predates the
// prior reservation's commit.
func loadReservedTicketCount(ctx context.Context, tx pgx.Tx, eventID string) (int, error) {
	var reserved int
	err := tx.QueryRow(ctx, `
		select count(*)
		from tickets
		where event_id = $1
		  and payment_status <> 'cancelled'
	`, eventID).Scan(&reserved)
	if err != nil {
		return 0, err
	}
	return reserved, nil
}

func (a *App) publicTicketURL(code string) string {
	base := strings.TrimRight(strings.TrimSpace(a.config.PublicWebURL), "/")
	if base == "" {
		return "/tickets/" + code
	}
	return base + "/tickets/" + code
}

func (a *App) ticketDTOFromRow(row ticketRow) ticketDTO {
	dto := ticketDTO{
		ID:            row.ID,
		EventID:       row.EventID,
		Email:         row.Email,
		Code:          row.Code,
		TicketURL:     a.publicTicketURL(row.Code),
		Status:        row.Status,
		PaymentStatus: row.PaymentStatus,
		AmountCents:   row.AmountCents,
		Currency:      row.Currency,
		DisplayName:   nil,
		CheckedInAt:   nil,
	}
	if row.DisplayName.Valid {
		dto.DisplayName = &row.DisplayName.String
	}
	if row.CheckedInAt.Valid {
		value := row.CheckedInAt.Time.UTC().Format(time.RFC3339Nano)
		dto.CheckedInAt = &value
	}
	return dto
}

func (a *App) doorTicketDTOFromRow(row ticketRow) doorTicketDTO {
	return doorTicketDTO{
		ID:                row.ID,
		Code:              row.Code,
		DisplayName:       nullableString(row.DisplayName),
		AdmissionEligible: ticketJourneyCanCheckIn(row.PaymentStatus),
		Status:            row.Status,
		CheckedInAt:       nullableTimeString(row.CheckedInAt),
	}
}

func newTicketCode() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
