package app

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AnnouncementDispatchReport is the secret-free aggregate output of one
// RunAnnouncementDispatch call: counts only, never a recipient address,
// subject or body. See docs/development/announcements.md, "Dispatch".
type AnnouncementDispatchReport struct {
	Claimed            int `json:"claimed"`
	RecipientsEnqueued int `json:"recipientsEnqueued"`
	RecipientsWithheld int `json:"recipientsWithheld"`
}

// RunAnnouncementDispatch claims every due, scheduled announcement (up to
// limit) with "for update skip locked", and for each one, in a single
// transaction: re-derives the audience fresh from consent_grants (never
// from a cached snapshot taken at schedule time), runs
// checkSendPermission per recipient as the final consent check, enqueues
// one purpose='announcement' email_outbox row per allowed recipient,
// records announcement_deliveries, and sets
// recipient_count/withheld_count/estimated_cost_cents/status=dispatched.
// Actual provider sending stays with RunEmailDeliveries/-send; this
// function never contacts the mail provider.
func RunAnnouncementDispatch(ctx context.Context, config Config, db *pgxpool.Pool, limit int) (AnnouncementDispatchReport, error) {
	report := AnnouncementDispatchReport{}
	if db == nil {
		return report, errors.New("announcement dispatch requires a database")
	}
	if limit < 1 || limit > 100 {
		return report, errors.New("announcement dispatch batch limit must be 1..100")
	}
	if version, err := CurrentSchemaVersion(ctx, db); err != nil || version != minimumSchemaVersion {
		return report, errors.New("announcement dispatch requires current migrations")
	}
	a := &App{db: db, config: config}
	for range limit {
		dispatched, enqueued, withheld, err := a.dispatchOneAnnouncement(ctx)
		if err != nil {
			return report, err
		}
		if !dispatched {
			break
		}
		report.Claimed++
		report.RecipientsEnqueued += enqueued
		report.RecipientsWithheld += withheld
	}
	return report, nil
}

type announcementRecipient struct {
	grantID          string
	recipientAddress string
}

// dispatchOneAnnouncement claims at most one due announcement and fully
// dispatches it, all in one transaction. It returns false (no error) when
// no due announcement remains, so RunAnnouncementDispatch's loop can stop.
func (a *App) dispatchOneAnnouncement(ctx context.Context) (dispatched bool, enqueuedCount, withheldCount int, err error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, 0, 0, errors.New("announcement dispatch transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var announcementID, workspaceID, subject, body string
	err = tx.QueryRow(ctx, `
		with candidate as (
			select id from announcements
			where status = $1 and scheduled_for is not null and scheduled_for <= now()
			order by scheduled_for
			limit 1
			for update skip locked
		)
		update announcements a set status = $2, updated_at = now()
		from candidate where a.id = candidate.id
		returning a.id::text, a.workspace_id::text, a.subject, a.body
	`, announcementStatusScheduled, announcementStatusDispatching).Scan(&announcementID, &workspaceID, &subject, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, 0, nil
	}
	if err != nil {
		return false, 0, 0, errors.New("announcement claim failed")
	}

	// Single-path routing (#220): an active listmonk binding at dispatch time
	// routes every recipient of this occurrence through listmonk; otherwise
	// the native outbox. The path is recorded once and the per-recipient rows
	// of each path reference it, so one occurrence never uses both. The
	// binding row is share-locked so a concurrent revoke waits for this
	// dispatch or is seen by it.
	binding, err := scanListmonkBinding(tx.QueryRow(ctx, `
		select `+listmonkBindingColumns+` from announcement_channel_bindings
		where workspace_id = $1 and channel = $2 and revoked_at is null for share
	`, workspaceID, channelListmonkName))
	path := deliveryPathListmonk
	if errors.Is(err, pgx.ErrNoRows) {
		path = deliveryPathNative
	} else if err != nil {
		return false, 0, 0, errors.New("announcement binding lookup failed")
	}
	if _, err := tx.Exec(ctx, `update announcements set delivery_path = $2 where id = $1`, announcementID, path); err != nil {
		return false, 0, 0, errors.New("announcement routing failed")
	}
	if path == deliveryPathListmonk {
		enqueued, withheld, err := a.dispatchListmonkRecipients(ctx, tx, binding, workspaceID, announcementID, subject, body)
		if err != nil {
			return false, 0, 0, err
		}
		return a.finishAnnouncementDispatch(ctx, tx, announcementID, enqueued, withheld)
	}

	// Re-derive the raw candidate audience fresh, inside this transaction
	// — never from any count computed at preview/schedule time — as
	// one verified grant per address for this workspace/purpose, preferring
	// the active grant over historical withdrawals, regardless of its
	// current withdrawn/suppressed state. checkSendPermission below,
	// called per recipient, is the actual final consent gate: it applies
	// the exact same unwithdrawn-grant and not-suppressed requirements
	// documented in consent.md, so a grant withdrawn (or an address
	// suppressed) since scheduling is counted as withheld here rather
	// than silently vanishing from both counts.
	rows, err := tx.Query(ctx, `
		select distinct on (g.recipient_address) g.id::text, g.recipient_address
		from consent_grants g
		where g.workspace_id = $1
		  and g.channel = 'email'
		  and g.purpose = $2
		  and g.verified_at is not null
		  and g.listmonk_binding_id is null
		order by g.recipient_address, (g.withdrawn_at is null) desc, g.granted_at desc, g.id
	`, workspaceID, consentPurposeAnnouncement)
	if err != nil {
		return false, 0, 0, errors.New("announcement audience query failed")
	}
	var recipients []announcementRecipient
	for rows.Next() {
		var rec announcementRecipient
		if err := rows.Scan(&rec.grantID, &rec.recipientAddress); err != nil {
			rows.Close()
			return false, 0, 0, errors.New("announcement audience scan failed")
		}
		recipients = append(recipients, rec)
	}
	closeErr := rows.Err()
	rows.Close()
	if closeErr != nil {
		return false, 0, 0, errors.New("announcement audience read failed")
	}

	enqueued, withheld := 0, 0
	for _, rec := range recipients {
		// Keep the link bound to the same active grant that authorizes this
		// enqueue. A concurrent withdrawal/regrant must not authorize a row
		// whose link would only withdraw an older grant.
		var active bool
		if err := tx.QueryRow(ctx, `select withdrawn_at is null from consent_grants where id = $1 for share`, rec.grantID).Scan(&active); err != nil {
			return false, 0, 0, errors.New("announcement grant check failed")
		}
		if !active {
			withheld++
			continue
		}
		// Final consent check, immediately before enqueueing: the same
		// function processEmailDeliveries calls again at send time (see
		// consent.md). A denial here (suppressed, or the grant no longer
		// verified/unwithdrawn) withholds this recipient from dispatch
		// entirely rather than enqueueing a row destined to be withheld
		// later.
		if permErr := checkSendPermission(ctx, tx, workspaceID, "email", rec.recipientAddress, consentPurposeAnnouncement); permErr != nil {
			if !errors.Is(permErr, ErrConsentGrantRequired) && !errors.Is(permErr, ErrConsentSuppressed) {
				return false, 0, 0, errors.New("announcement dispatch consent check failed")
			}
			withheld++
			continue
		}

		// Store a fresh hash per message. Existing links remain usable after
		// later dispatches and session-secret rotation.
		withdrawToken, withdrawHash, err := newToken()
		if err != nil {
			return false, 0, 0, errors.New("announcement withdraw token mint failed")
		}
		if _, err := tx.Exec(ctx, `insert into consent_withdraw_tokens(grant_id, token_hash) values ($1, $2)`, rec.grantID, withdrawHash); err != nil {
			return false, 0, 0, errors.New("announcement withdraw token record failed")
		}

		withdrawLink := strings.TrimRight(a.config.PublicWebURL, "/") + "/consent/withdraw?token=" + withdrawToken
		messageBody := body + "\n\nTo stop receiving these messages: " + withdrawLink

		outboxID, err := a.enqueueAnnouncementEmail(ctx, tx, workspaceID, rec.recipientAddress, subject, messageBody, announcementID)
		if err != nil {
			return false, 0, 0, errors.New("announcement enqueue failed")
		}
		if _, err := tx.Exec(ctx, `
			insert into announcement_deliveries (announcement_id, outbox_id, recipient_address)
			values ($1, $2, $3)
		`, announcementID, outboxID, rec.recipientAddress); err != nil {
			return false, 0, 0, errors.New("announcement delivery record failed")
		}
		enqueued++
	}

	return a.finishAnnouncementDispatch(ctx, tx, announcementID, enqueued, withheld)
}

// finishAnnouncementDispatch records the occurrence's counts and commits the
// dispatch transaction for either path.
func (a *App) finishAnnouncementDispatch(ctx context.Context, tx pgx.Tx, announcementID string, enqueued, withheld int) (bool, int, int, error) {
	estimatedCostCents := enqueued * a.config.AnnouncementUnitCostCents
	if _, err := tx.Exec(ctx, `
		update announcements
		set status = $2, dispatched_at = now(), recipient_count = $3, withheld_count = $4,
		    estimated_cost_cents = $5, updated_at = now()
		where id = $1
	`, announcementID, announcementStatusDispatched, enqueued, withheld, estimatedCostCents); err != nil {
		return false, 0, 0, errors.New("announcement finalize failed")
	}

	if err := tx.Commit(ctx); err != nil {
		return false, 0, 0, errors.New("announcement dispatch commit failed")
	}
	return true, enqueued, withheld, nil
}

// enqueueAnnouncementEmail inserts one email_outbox row with
// purpose='announcement' and workspace_id set, so checkSendPermission's
// send-time recheck in processEmailDeliveries has what it needs. It
// deliberately does not reuse enqueueEmail, whose insert defaults
// purpose to 'transactional' and leaves workspace_id null for its four
// existing (transactional) call sites.
func (a *App) enqueueAnnouncementEmail(ctx context.Context, tx pgx.Tx, workspaceID, recipient, subject, body, announcementID string) (string, error) {
	status := "held"
	if a.config.MailDeliveryEnabled {
		status = "pending"
	}
	var outboxID string
	err := tx.QueryRow(ctx, `
		insert into email_outbox
			(recipient_email, subject, body, related_type, related_id, delivery_status,
			 sender_address, reply_to_address, purpose, workspace_id, provider, expires_at)
		values ($1, $2, $3, 'announcement', $4, $5, $6, $7, $8, $9, $10, now() + interval '23 hours')
		returning id::text
	`, recipient, subject, body, announcementID, status, a.config.MailFrom, a.config.MailReplyTo, consentPurposeAnnouncement, workspaceID, a.config.mailProvider()).Scan(&outboxID)
	return outboxID, err
}
