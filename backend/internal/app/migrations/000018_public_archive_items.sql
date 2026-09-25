create table event_public_archive_items (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  event_id uuid not null,
  replaces_item_id uuid,
  kind text not null check (kind in ('credit','link')),
  title text not null check (length(trim(title)) between 1 and 300),
  attribution_name text not null check (length(trim(attribution_name)) between 1 and 300),
  attribution_url text,
  external_url text,
  intended_use text not null check (intended_use in ('link_only','display_credit')),
  rights_assertion text not null check (rights_assertion in ('owned','licensed','permission_asserted','public_domain')),
  evidence_reference text not null default '' check (length(evidence_reference) <= 500),
  status text not null default 'approved' check (status in ('approved','corrected','unavailable')),
  unavailable_reason text not null default '' check (length(unavailable_reason) <= 500),
  approved_by_person_id uuid not null references people(id),
  approved_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  unique (id, workspace_id, event_id),
  foreign key (event_id, workspace_id) references events(id, workspace_id) on delete cascade,
  foreign key (replaces_item_id, workspace_id, event_id) references event_public_archive_items(id, workspace_id, event_id)
);
create index event_public_archive_items_event_idx on event_public_archive_items(event_id, created_at);
