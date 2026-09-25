-- COMMERCE-01: bind a browser retry to its original durable checkout attempt.
-- This is an identity/recovery ledger only. It neither retries provider work
-- nor creates a refund, charge, cancellation, or reconciliation decision.
alter table payment_checkout_attempts
  add column purchase_intent_key uuid unique,
  add column provider_checkout_url text check (
    provider_checkout_url is null
    or (length(provider_checkout_url) between 1 and 4000 and provider_checkout_url ~ '^https://')
  );
