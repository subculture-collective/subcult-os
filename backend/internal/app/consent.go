package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Sentinel errors returned by checkSendPermission. Callers that only need
// an allow/deny decision can treat any non-nil error as deny; callers that
// need to distinguish the reason (for example to pick a delivery status
// or an HTTP status code) can use errors.Is.
var (
	// ErrConsentSuppressed means the recipient address is present in
	// email_suppressions (a prior bounce, complaint or explicit
	// suppression) and must never receive another message on this
	// channel, regardless of any grant.
	ErrConsentSuppressed = errors.New("recipient is suppressed and may not receive channel messages")
	// ErrConsentGrantRequired means the purpose requires a verified,
	// unwithdrawn consent grant for this workspace/channel/recipient and
	// none exists — because no grant was ever recorded, the recorded
	// grant is for a different purpose, it is not yet verified, or it has
	// been withdrawn. checkSendPermission deliberately does not
	// distinguish those cases further: an unverified or withdrawn grant
	// authorizes nothing, exactly like no grant at all.
	ErrConsentGrantRequired = errors.New("no verified, unwithdrawn consent grant authorizes this purpose")
)

// purposeTransactional never requires a grant: identity verification and
// recovery, ticket confirmations and workspace invitations are all
// transactional today (see docs/development/consent.md) and must keep
// sending unchanged. purposeAnnouncement requires a verified, unwithdrawn
// grant matching workspace, channel, recipient and purpose.
const (
	consentPurposeTransactional = "transactional"
	consentPurposeAnnouncement  = "announcement"
)

var consentChannels = map[string]bool{"email": true}
var consentSources = map[string]bool{"explicit_form": true, "operator_recorded": true}

// checkSendPermission is the single place that decides whether a message
// may be sent to recipient over channel for purpose, on behalf of
// workspaceID. It must be consulted both when a message is first
// requested and again, unchanged, immediately before a queued message
// actually leaves the system (processEmailDeliveries calls it per row),
// because consent can be withdrawn or a recipient suppressed after a
// message is enqueued and before it is delivered.
//
// It intentionally never joins against tickets, contacts,
// event_role_applications, atproto identity links or workspace_members:
// holding a ticket, being a contact, or being a workspace member never
// implies permission to send an announcement, no matter how well the
// workspace otherwise knows the address. See docs/development/consent.md.
func (a *App) checkSendPermission(ctx context.Context, workspaceID, channel, recipient, purpose string) error {
	return checkSendPermission(ctx, a.db, workspaceID, channel, recipient, purpose)
}

// Reuse the caller's transaction connection during dispatch.
func checkSendPermission(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, workspaceID, channel, recipient, purpose string) error {
	recipient = normalizeEmail(recipient)

	var suppressed bool
	if err := db.QueryRow(ctx, `
		select exists(select 1 from email_suppressions where recipient_email = $1)
	`, recipient).Scan(&suppressed); err != nil {
		return err
	}
	if suppressed {
		return ErrConsentSuppressed
	}

	if purpose == consentPurposeTransactional {
		return nil
	}

	// An announcement can never be authorized without a workspace to scope
	// the grant to. Return the typed denial explicitly rather than letting
	// the query below fail on an empty workspaceID (which pgx would report
	// as a raw driver/encode error indistinguishable from an infra fault).
	if workspaceID == "" {
		return ErrConsentGrantRequired
	}

	var granted bool
	if err := db.QueryRow(ctx, `
		select exists(
			select 1 from consent_grants
			where workspace_id = $1
			  and channel = $2
			  and recipient_address = $3
			  and purpose = $4
			  and verified_at is not null
			  and withdrawn_at is null
			  and listmonk_binding_id is null
		)
	`, workspaceID, channel, recipient, purpose).Scan(&granted); err != nil {
		return err
	}
	if !granted {
		return ErrConsentGrantRequired
	}
	return nil
}
