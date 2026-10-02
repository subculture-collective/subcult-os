# ADR 0008: Keep Social, Ranking and Reputation Concepts Out of Subcult OS

## Status

Accepted, 2026-10-02. Records the outcome of SOCIAL-01 (Gitea issue #72).
This ADR collects exclusions already accepted in ADR 0003, ADR 0005 and the
extraction documents. It does not change any code, schema or route.

## Context

The older Subcults repository and the early proposals contain alliances,
social posts, a proprietary feed, live audio and streaming, trust ranking and
reputation scoring. Several repository documents already exclude these from
Subcult OS:

- [ADR 0003](0003-no-global-reputation.md) rejects global reputation scores and
  automatic cross-workspace reliability sharing.
- [ADR 0005](0005-subcult-os-platform-core.md) leaves streaming, ranking and
  legacy product surfaces behind and rules out preserving a feature merely
  because code exists.
- The [Subcults source-extraction track](../development/subcults-track.md)
  leaves streaming, ranking, alliances, social posts and general moderation
  behind by default.
- The [extraction manifest](../development/subcults-extraction-manifest.md)
  rejects the legacy indexer mapper, which hard-imports alliance, post and
  scene domains, and the projection repository, which is coupled to trust
  recomputation.
- The [extraction inventory](../development/extraction-inventory.md) excludes
  LiveKit streaming, trust ranking, global reputation, automated moderation,
  alliances and general social-post features from the initial platform core.
- The [development backlog](../development/backlog.md) keeps streaming,
  alliances, posts and reputation out of MODEL-01 and ranking out of DISC-01.

These statements are spread across several files. Issue #72 asked for one
decision record.

## Decision

The following are historical ideas only. They are not planned work in Subcult
OS:

- alliances between collectives or scenes;
- social posts and a proprietary feed;
- trust ranking, global reputation and any cross-workspace reliability score;
- streaming and live audio as part of the core event workflow;
- general automated moderation built to support the above.

ADR 0003 stands. A workspace may keep private factual history about its own
events, such as commitments, disputes, lessons learned and reliability notes.
That history stays inside the workspace and is not shared through Connections
by default. It is not converted into a score, rank or badge, and it is not
compared across workspaces. Recording what happened at an event is in scope;
scoring a person or group from that record is not.

Existing Subcults code for these features is not carried into Subcult OS
because it exists, has tests or was once released. Extraction still follows
ADR 0005: a reviewed product need, a minimal contract, provenance and focused
tests.

Only a new ADR can reopen any of these directions. That ADR must cite dated
evidence of demand from operators or participants. It must also cover privacy
and moderation cost and explain how it avoids the punitive or contextless
scoring that ADR 0003 rejects. Repository volume, old roadmaps and proposal
text do not count as that evidence.

## Consequences

- The proposal and Subcults material describing these features stays readable
  as history. It is not deleted and does not become a backlog item.
- Optional live audio is parked separately as AUDIO-01 (issue #70) in the
  [backlog](../development/backlog.md#parked-options). It stays outside the
  core dependency chain and needs its own demand evidence.
- Reviews of new extraction or feature work can cite this ADR instead of
  repeating the exclusion list.
- No code, migration or public Lexicon changes with this decision.
