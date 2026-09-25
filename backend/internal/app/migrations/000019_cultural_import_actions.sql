-- IMPORT-01: explicit, append-only application decisions. Preview matches are
-- never targets; every action names its event and, for corrections, occurrence.
create table cultural_import_actions (
 id uuid primary key default gen_random_uuid(), workspace_id uuid not null references workspaces(id) on delete cascade,
 import_id uuid not null, candidate_id uuid not null, mode text not null check(mode in ('create','correction')),
 target_event_id uuid not null, target_occurrence_id uuid, created_occurrence_id uuid,
 source_id_snapshot text not null check(length(trim(source_id_snapshot)) between 1 and 200), content_sha256_snapshot char(64) not null,
 candidate_row_snapshot integer not null check(candidate_row_snapshot >= 2), field_diff jsonb not null check(jsonb_typeof(field_diff)='object'),
 before_snapshot jsonb not null check(jsonb_typeof(before_snapshot)='object'), after_snapshot jsonb not null check(jsonb_typeof(after_snapshot)='object'),
 expected_updated_at timestamptz, expected_public_cid text, created_occurrence_updated_at timestamptz,
 errors_acknowledged_at timestamptz, applied_by_person_id uuid not null references people(id), applied_at timestamptz not null default now(),
 rolled_back_at timestamptz, rolled_back_by_person_id uuid references people(id), rollback_outcome text check(rollback_outcome in ('deleted')),
 unique(candidate_id), foreign key(import_id,workspace_id) references cultural_import_previews(id,workspace_id) on delete restrict,
 foreign key(candidate_id,workspace_id) references cultural_import_candidates(id,workspace_id) on delete restrict,
 check((mode='create' and target_occurrence_id is null and created_occurrence_id is not null) or (mode='correction' and target_occurrence_id is not null and created_occurrence_id is null)),
 check((rolled_back_at is null and rolled_back_by_person_id is null and rollback_outcome is null) or (rolled_back_at is not null and rolled_back_by_person_id is not null and rollback_outcome='deleted'))
);
create index cultural_import_actions_workspace_idx on cultural_import_actions(workspace_id, applied_at desc);
