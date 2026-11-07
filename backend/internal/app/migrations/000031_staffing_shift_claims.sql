-- OPS-218: open-shift self-claim with shift-level capacity.
--
-- Existing staffing items keep their meaning: every row receives capacity 1,
-- stays closed to self-claim, and its existing assigned_person_id or
-- assigned_application_id continues to occupy that single place. New places
-- beyond the legacy assignee are rows in event_staffing_claims. The
-- application counts both sources under one locked transaction.
alter table event_staffing_items
  add column capacity integer not null default 1
    check (capacity between 1 and 100),
  add column claims_open boolean not null default false,
  add column release_cutoff_hours integer not null default 24
    check (release_cutoff_hours between 0 and 720),
  add constraint event_staffing_items_claims_open_timed_shift
    check (not claims_open or (kind = 'shift' and starts_at is not null and ends_at is not null)),
  -- Lets claim rows prove, in the database, that a place and its event agree.
  add constraint event_staffing_items_id_event_key unique (id, event_id);

-- Lets the eligible-role policy prove that a role belongs to the shift's event.
alter table event_roles
  add constraint event_roles_id_event_key unique (id, event_id);

-- A role application becomes person-backed only when a signed-in account with
-- the same verified email submits it. Earlier anonymous applications stay
-- unbound; matching email text alone never binds an account.
alter table event_role_applications
  add column applicant_person_id uuid references people(id) on delete set null;

create index event_role_applications_person_idx
  on event_role_applications (applicant_person_id, event_id)
  where applicant_person_id is not null;

-- Which approved event roles may claim a shift. Both composite keys pin the
-- shift and the role to the same event.
create table event_staffing_claim_roles (
  staffing_item_id uuid not null,
  event_id uuid not null,
  role_id uuid not null,
  created_at timestamptz not null default now(),
  primary key (staffing_item_id, role_id),
  foreign key (staffing_item_id, event_id) references event_staffing_items(id, event_id) on delete cascade,
  foreign key (role_id, event_id) references event_roles(id, event_id) on delete cascade
);

create table event_staffing_claims (
  id uuid primary key default gen_random_uuid(),
  staffing_item_id uuid not null,
  event_id uuid not null references events(id) on delete cascade,
  person_id uuid not null references people(id),
  application_id uuid references event_role_applications(id) on delete set null,
  source text not null check (source in ('self', 'organizer')),
  status text not null default 'active' check (status in ('active', 'released', 'removed')),
  created_by_person_id uuid not null references people(id),
  created_at timestamptz not null default now(),
  ended_at timestamptz,
  ended_by_person_id uuid references people(id),
  check ((status = 'active') = (ended_at is null)),
  foreign key (staffing_item_id, event_id) references event_staffing_items(id, event_id) on delete cascade
);

create unique index event_staffing_claims_one_active_place
  on event_staffing_claims (staffing_item_id, person_id)
  where status = 'active';

create index event_staffing_claims_person_event_idx
  on event_staffing_claims (person_id, event_id)
  where status = 'active';

create index event_staffing_claims_item_idx
  on event_staffing_claims (staffing_item_id)
  where status = 'active';
