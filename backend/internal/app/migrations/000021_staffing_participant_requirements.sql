-- OPS-57: participant-visible instructions are separate from operator notes.
alter table event_staffing_items
  add column participant_requirements text not null default ''
    check (length(participant_requirements) <= 2000);
