# Stripe Local Testing Runbook

Use this runbook to validate paid ticketing locally without browser automation.

## Purpose

Verify the Stripe-backed paid reservation path, webhook fulfillment, and local environment wiring.

## Preconditions

- `subcult-os` stack is running locally.
- `STRIPE_SECRET_KEY` is set in the shell that launches the app or QA command.
- Stripe CLI is installed and authenticated.

## Manual webhook flow

Forward Stripe events to the local API webhook endpoint:

```bash
stripe listen --forward-to localhost:38080/api/stripe/webhook
```

Copy the printed `whsec_...` value into `STRIPE_WEBHOOK_SECRET`.

## Paid checkout test

1. Configure a fixed-price paid Event.
2. Start a paid reservation with `make alpha-qa-paid` or the public UI.
3. Complete Checkout with the Stripe test card:

```text
4242 4242 4242 4242
```

4. Confirm the ticket becomes paid only after the webhook arrives.

## Rules

- `make alpha-qa` remains the free-ticket QA path.
- `make alpha-qa-paid` skips cleanly when Stripe env vars are missing.
- Do not treat the browser success redirect as fulfillment; the signed webhook is the source of truth.
- A paid reservation first records a local checkout attempt and its Stripe idempotency key. If the provider request times out or saving its response fails, the ticket stays pending and the attempt is marked `unknown`; do not retry by creating a second checkout session. Reconcile the existing attempt with Stripe using its stored idempotency key, then replay the signed webhook.
- The public checkout form sends one opaque purchase-intent key while its purchaser details stay unchanged. A repeat request finds the same durable attempt: a ready attempt returns its saved Checkout URL, while `creating` or `unknown` returns a pending-reconciliation result and makes no provider request. Changing the form starts a new intent. This is not an automatic provider retry or a recovery-window policy.
- The browser keeps that key only for its current page session. Reload recovery, provider idempotency retention windows, and an operator recovery policy remain separate work; a reload must not be presented as a safe automatic retry.
- A signed callback is accepted only when its ticket, checkout attempt, provider session, amount, and currency agree with the durable record. The webhook ledger retains an `anomalous` outcome for mismatches.
- This runbook uses Stripe test mode only. Automated tests use fakes and must never invoke Stripe, the Stripe CLI, or a retained application database.

## Expected result

Paid reservations should return a Checkout URL, and ticket fulfillment should appear only after Stripe delivers the webhook event.
