-- Issue #221: hand an approved announcement occurrence to one connected
-- Backline server/channel. See docs/development/backline-handoff.md.
--
-- Additive only. No existing row, column or email behavior changes.

-- Composite key so a handoff can carry (announcement_id, workspace_id) and
-- reject a cross-workspace reference at the database level.
alter table announcements add constraint announcements_id_workspace_key unique (id, workspace_id);

-- One row per binding a workspace organizer approved. Revoked rows are kept
-- for audit and never receive new handoffs. Only Backline exists today; the
-- channel column is the seam for a later destination.
create table announcement_channel_bindings (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  channel text not null default 'backline' check (channel in ('backline')),
  label text not null check (length(trim(label)) between 1 and 80),
  external_server_id text not null check (external_server_id ~ '^[0-9]{17,20}$'),
  external_channel_id text not null check (external_channel_id ~ '^[0-9]{17,20}$'),
  mention_policy text not null default 'none' check (mention_policy in ('none')),
  created_by_person_id uuid references people(id),
  created_at timestamptz not null default now(),
  revoked_at timestamptz,
  revoked_by_person_id uuid references people(id),
  unique (id, workspace_id)
);
create unique index announcement_channel_bindings_active_idx
  on announcement_channel_bindings (workspace_id, channel) where revoked_at is null;

-- One durable handoff per (announcement, binding). occurrence_key is the
-- idempotency key the receiver deduplicates on; publish and cancel messages
-- reuse it. status describes the current operation's transport outcome;
-- delivery_outcome is the receiver's later channel-delivery receipt.
create table announcement_channel_handoffs (
  id uuid primary key default gen_random_uuid(),
  announcement_id uuid not null,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  binding_id uuid not null,
  occurrence_key text not null unique check (length(occurrence_key) between 1 and 200),
  content_revision text not null check (content_revision ~ '^sha256:[0-9a-f]{64}$'),
  deliver_at timestamptz not null,
  expires_at timestamptz not null check (expires_at > deliver_at),
  operation text not null default 'publish' check (operation in ('publish', 'cancel')),
  status text not null default 'pending'
    check (status in ('pending', 'sending', 'accepted', 'rejected', 'uncertain', 'failed', 'cancelled', 'expired')),
  attempt_count integer not null default 0 check (attempt_count >= 0),
  next_attempt_at timestamptz not null default now(),
  lease_expires_at timestamptz,
  receiver_status text check (receiver_status is null or receiver_status ~ '^[a-z_]{1,32}$'),
  receiver_reference text check (receiver_reference is null or length(receiver_reference) between 1 and 128),
  last_error text check (last_error is null or last_error ~ '^[a-z0-9_]{1,64}$'),
  acknowledged_at timestamptz,
  cancel_requested_at timestamptz,
  delivery_outcome text check (delivery_outcome is null or delivery_outcome in ('delivered', 'failed', 'uncertain', 'cancelled')),
  delivery_outcome_at timestamptz,
  created_by_person_id uuid references people(id),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (announcement_id, binding_id),
  foreign key (announcement_id, workspace_id) references announcements(id, workspace_id) on delete cascade,
  foreign key (binding_id, workspace_id) references announcement_channel_bindings(id, workspace_id)
);
create index announcement_channel_handoffs_due_idx
  on announcement_channel_handoffs (next_attempt_at) where status in ('pending', 'sending');
create index announcement_channel_handoffs_announcement_idx
  on announcement_channel_handoffs (announcement_id, created_at);
