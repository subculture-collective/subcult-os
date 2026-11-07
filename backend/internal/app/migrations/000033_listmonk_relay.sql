-- Issue #220: relay an approved announcement to one explicitly bound listmonk
-- list. See docs/development/listmonk-relay.md.
--
-- Additive except for two widened constraints: announcement bindings admit a
-- second channel, and consent grants admit a Kork-origin source. No existing
-- row, Backline handoff or native email behavior changes.

-- Bindings reuse the channel seam from 000032. A listmonk binding names one
-- list, one transactional template, one sender address and the disclosure
-- version a native grant must carry to count for it. The kork_* columns are
-- the optional receiver policy for Kork consent receipts: all four or none.
alter table announcement_channel_bindings drop constraint announcement_channel_bindings_channel_check;
alter table announcement_channel_bindings add constraint announcement_channel_bindings_channel_check
  check (channel in ('backline', 'listmonk'));
alter table announcement_channel_bindings
  alter column external_server_id drop not null,
  alter column external_channel_id drop not null,
  add column listmonk_list_id integer check (listmonk_list_id > 0),
  add column listmonk_template_id integer check (listmonk_template_id > 0),
  add column sender_address text check (sender_address is null or sender_address ~ '^[^@[:space:]<>]+@[^@[:space:]<>]+$'),
  add column disclosure_version text check (disclosure_version is null or length(trim(disclosure_version)) between 1 and 80),
  add column kork_list_url text check (kork_list_url is null or kork_list_url ~ '^https://[^[:space:]#?]+$'),
  add column kork_sender text check (kork_sender is null or length(kork_sender) between 1 and 200),
  add column kork_purpose text check (kork_purpose is null or length(kork_purpose) between 1 and 200),
  add column kork_disclosure text check (kork_disclosure is null or length(kork_disclosure) between 1 and 1000),
  add constraint announcement_channel_bindings_shape check (
    (channel = 'backline' and external_server_id is not null and external_channel_id is not null
      and num_nonnulls(listmonk_list_id, listmonk_template_id, sender_address, disclosure_version,
                       kork_list_url, kork_sender, kork_purpose, kork_disclosure) = 0)
    or (channel = 'listmonk' and external_server_id is null and external_channel_id is null
      and num_nulls(listmonk_list_id, listmonk_template_id, sender_address, disclosure_version) = 0
      and num_nulls(kork_list_url, kork_sender, kork_purpose, kork_disclosure) in (0, 4))
  );

-- A Kork-origin grant is scoped to the listmonk binding whose receiver policy
-- accepted its receipt, and keeps that receipt. It never authorizes native
-- email or another binding.
alter table consent_grants drop constraint consent_grants_source_check;
alter table consent_grants add constraint consent_grants_source_check
  check (source in ('explicit_form', 'operator_recorded', 'kork_receipt'));
alter table consent_grants
  add column listmonk_binding_id uuid,
  add column origin_receipt jsonb,
  add constraint consent_grants_listmonk_binding_fkey foreign key (listmonk_binding_id, workspace_id)
    references announcement_channel_bindings(id, workspace_id),
  add constraint consent_grants_kork_receipt_check check (
    (source = 'kork_receipt') = (listmonk_binding_id is not null)
    and (source = 'kork_receipt') = (origin_receipt is not null)
  );

-- Single-path routing. Dispatch records the occurrence's path once. Each
-- path's per-recipient rows carry a constant path and a foreign key to
-- (announcement, path), so one occurrence can never have both native outbox
-- rows and listmonk rows.
alter table announcements add column delivery_path text check (delivery_path in ('native', 'listmonk'));
update announcements set delivery_path = 'native' where status in ('dispatching', 'dispatched');
alter table announcements add constraint announcements_id_delivery_path_key unique (id, delivery_path);
alter table announcement_deliveries
  add column delivery_path text not null default 'native' check (delivery_path = 'native'),
  add constraint announcement_deliveries_path_fkey foreign key (announcement_id, delivery_path)
    references announcements(id, delivery_path) on delete cascade;

-- One durable row per recipient of a listmonk occurrence. status follows the
-- Backline handoff vocabulary plus withheld (consent or binding gone before
-- send) and suppressed (OS or provider suppression). Only the /api/tx call can
-- leave an uncertain outcome; uncertain and failed rows are held for review.
create table announcement_listmonk_deliveries (
  id uuid primary key default gen_random_uuid(),
  announcement_id uuid not null,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  delivery_path text not null default 'listmonk' check (delivery_path = 'listmonk'),
  binding_id uuid not null,
  grant_id uuid not null references consent_grants(id) on delete cascade,
  recipient_address text not null check (trim(recipient_address) <> ''),
  subject text not null,
  body text not null,
  status text not null default 'pending'
    check (status in ('held', 'pending', 'sending', 'accepted', 'rejected', 'uncertain', 'failed', 'withheld', 'suppressed', 'expired')),
  attempt_count integer not null default 0 check (attempt_count >= 0),
  next_attempt_at timestamptz not null default now(),
  lease_expires_at timestamptz,
  last_error text check (last_error is null or last_error ~ '^[a-z0-9_]{1,64}$'),
  accepted_at timestamptz,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (announcement_id, recipient_address),
  foreign key (announcement_id, delivery_path) references announcements(id, delivery_path) on delete cascade,
  foreign key (announcement_id, workspace_id) references announcements(id, workspace_id) on delete cascade,
  foreign key (binding_id, workspace_id) references announcement_channel_bindings(id, workspace_id)
);
create index announcement_listmonk_deliveries_due_idx
  on announcement_listmonk_deliveries (next_attempt_at) where status in ('pending', 'sending');
create index announcement_listmonk_deliveries_binding_idx
  on announcement_listmonk_deliveries (binding_id) where status in ('held', 'pending');
