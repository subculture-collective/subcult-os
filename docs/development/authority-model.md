# Workspace authority model (AUTH-01)

Status: permission matrix, membership expiry/revocation, creator delegation
and a publication authorization hook implemented 2026-09-23 (migration
000010). This is evidence toward D8 (publishing actor authority) in
[`decisions.md`](decisions.md); it does not itself accept that decision.
PUB-AUTH (#19) is the future publication outbox that will call
`authorizePublicWrite` before a queued public write executes; no outbox
exists yet.

## Four separate concepts

Authority in this codebase is four distinct things, deliberately kept
separate so that having one never silently implies another:

1. **Account control** — proof that a request comes from a given person:
   verified email identity and rotating session families, added in
   migration 000002 (IDENT-01). This answers "who is making this request",
   nothing more.
2. **Workspace permission** — what a person may do inside one workspace,
   carried by their `workspace_members` row's `role`. This answers "what can
   this person do here", scoped to a single workspace.
3. **Creator delegation** — a cultural profile's scoped, time-boxed grant of
   the right to act for it to a workspace, carried by `creator_delegations`.
   A workspace permission never implies a creator delegation: an owner can
   run a workspace and still have no right to publish on behalf of a
   specific creator profile until that profile's owner grants one. This
   answers "has this specific creator authorized this workspace to act for
   them", independent of who inside the workspace is asking.
4. **Real-world organization claim** — an unverified assertion, e.g. a
   membership row's display name or an invitation email, about someone's
   real-world role (this venue's promoter, this act's manager). This
   codebase does not currently persist a distinct claim record; the point of
   naming it is negative: no such claim, verified or not, confers authority
   on its own. Only workspace permission and creator delegation do. Where a
   future feature wants to record an organization claim (e.g. "verified
   promoter of venue X"), it must not double as a permission grant.

## Workspace permission roles

`workspace_members.role` (migration 000010) allows five roles plus one
legacy alias:

| Role | Intent |
| --- | --- |
| `owner` | Full control: every permission, including managing other members' roles/expiry/revocation and creator delegations. |
| `organizer` | Day-to-day event operations and creator delegation management, not membership or finance. |
| `finance` | Settlement and payment operations, not membership, delegation or publish. |
| `door` | Door check-in operations only. |
| `crew` | Baseline operate-only access: the floor for every active membership. |
| `member` | Legacy alias for `crew`, kept so every pre-AUTH-01 row and every existing literal `"member"` call site in `backend/internal/app` keeps working unmodified. New code assigns `crew`, never `member`; the role-change endpoint's assignable set excludes `member` for the same reason. |

### Least-privilege permission matrix

Permissions are capabilities; roles are named bundles of them
(`backend/internal/app/authority.go`, `rolePermissions`). Every role
includes `operate` (baseline access — anything less isn't membership at
all); everything else is additive per role, not subtractive from `owner`:

| Permission | owner | organizer | finance | door | crew / member |
| --- | --- | --- | --- | --- | --- |
| `operate` (baseline access) | yes | yes | yes | yes | yes |
| `manage_members` (role/expiry/revocation) | yes | no | no | no | no |
| `manage_delegations` (create/revoke delegation) | yes | yes | no | no | no |
| `publish` (authorize a public write) | yes | yes | no | no | no |
| `finance` (settlement operations) | yes | no | yes | no | no |
| `door` (check-in operations) | yes | no | no | yes | no |
| `manage_consent` (create/list/withdraw channel consent grants) | yes | yes | no | no | no |
| `manage_announcements` (create/schedule/cancel/list announcements) | yes | yes | no | no | no |
| `manage_imports` (create review-only cultural import previews) | yes | yes | no | no | no |
| `manage_channel_bindings` (connect/revoke an announcement channel binding) | yes | yes | no | no | no |

`manage_members` is owner-only rather than owner-and-organizer: an organizer
who could grant themselves or an ally the owner role, or revoke the owner,
would defeat the least-privilege boundary this table exists to draw.

`manage_consent` is owner-and-organizer, matching `manage_delegations`
rather than `manage_members`: recording or withdrawing a channel consent
grant does not change anyone's workspace authority, so it carries the same
scope as the other day-to-day operator capability, not the membership
control that is deliberately owner-only. See
[`consent.md`](consent.md).

`manage_announcements` is likewise owner-and-organizer: drafting,
scheduling, cancelling or dispatching an announcement is a day-to-day
operator capability over the workspace's own audience, not a change to
anyone's workspace authority. See
[`announcements.md`](announcements.md).

`manage_imports` is owner-and-organizer. It creates a private review artifact
with a source assertion and normalized cultural fields; it grants neither a
real-world organization claim nor a right to apply, publish, or overwrite a
canonical record. See [`cultural-import.md`](cultural-import.md).

`manage_channel_bindings` is owner-and-organizer. It connects the workspace
to one Backline server channel that its administrator approved, or revokes
that connection. It is separate from `manage_announcements` so the people who
choose an external destination can later be narrowed without changing who
drafts announcements. Handing off still requires `manage_announcements` and an
already scheduled announcement. See
[`backline-handoff.md`](backline-handoff.md).

## Expiry, revocation, owner departure and recovery

- **Expiry**: `workspace_members.expires_at`, nullable. An owner (via
  `PATCH /api/workspaces/{workspaceID}/members/{memberID}`) may set, change
  or clear a member's expiry. Once `expires_at` is in the past, every
  authority check in this package treats that membership as absent — not
  merely restricted — identically to a `removed_at` or `revoked_at` row (see
  `activeMembership` in `authority.go`).
- **Revocation**: `workspace_members.revoked_at` /
  `revoked_by_person_id`, set by
  `POST /api/workspaces/{workspaceID}/members/{memberID}/revoke` and never
  cleared by any endpoint. Revocation is a one-way, audited event distinct
  from removal: removal (the pre-existing `DELETE` endpoint) deletes the
  membership row outright, while revocation preserves the row — and the
  historical fact that this person once held this role — for audit, while
  making every authority check treat them as absent. A second revoke on an
  already-revoked member is a `409`, not a silent success.
- **Owner departure — last-owner guard**: `activeOwnerCountTx` counts active
  (not removed, not revoked, not expired) owners under the same row lock
  used by the mutating handler. Demoting the sole active owner
  (`PATCH .../members/{id}` with a non-owner role), revoking them
  (`POST .../revoke`), or removing them (the pre-existing `DELETE`) are all
  rejected with `409` while they are the only active owner. A workspace can
  only lose its last owner by first promoting a second person to `owner`
  (ownership transfer is "assign another active owner, then step the
  original owner down or out" — there is no separate transfer endpoint,
  because none is needed once two owners can briefly coexist).
- **Recovery**: there is currently no account-recovery flow specific to
  workspace authority beyond the ownership-transfer pattern above. If every
  owner account becomes inaccessible (e.g. lost credentials) with no
  operator-side recovery, the workspace has no in-product recovery path;
  this is a known gap, not a delivered feature, and should be revisited
  before workspaces are relied on for anything the owner cannot afford to
  lose access to.
- **Creator delegation lifecycle**: `creator_delegations.expires_at` /
  `revoked_at` / `revoked_by_person_id` follow the same pattern as
  membership: an expired or revoked delegation is treated as absent by
  `authorizePublicWrite`, and revocation
  (`POST .../delegations/{delegationID}/revoke`) is one-way and audited.
  Unlike membership there is no "last delegation" guard: a cultural profile
  can have zero active delegations, meaning no workspace can currently
  publish on its behalf, which is the safe default, not an error state.

## The central permission function

`func (a *App) authorize(ctx, personID, workspaceID, permission) error`
(`authority.go`) is the single function every authority decision in this
package is built on. It calls `activeMembership`, which treats a removed,
revoked, or expired membership identically to no membership row at all —
returning `ErrMembershipDenied` rather than leaking that a role once
existed — then checks the active role against `rolePermissions`, returning
`ErrPermissionDenied` if the role lacks the requested capability.

`requireWorkspaceRole`, the existing HTTP-facing helper every pre-AUTH-01
handler already calls with literal role strings (e.g.
`a.requireWorkspaceRole(r, workspaceID, "owner")`), is rewritten on top of
`activeMembership` so every existing call site keeps working unmodified
while gaining expiry/revocation enforcement for free. `requirePermission` is
its capability-based sibling for the new endpoints in this slice.

## Publication authorization hook

`func (a *App) authorizePublicWrite(ctx, actorPersonID, workspaceID, profileID) error`
(`authority.go`) is the hook PUB-AUTH (#19) will call from the future
publication outbox before a queued public write executes. It denies, in
order:

1. If the actor's workspace membership is revoked, expired, or absent
   (`ErrMembershipDenied`, via `authorize`).
2. If the actor's active role lacks `publish`
   (`ErrPermissionDenied`, via `authorize`).
3. If no active (not expired, not revoked) creator delegation exists from
   the target cultural profile to the workspace (`ErrDelegationDenied`).

Every reason is a deny, not a partial success — a queued write must never
execute on a stale authorization decision. `authority_integration_test.go`
(`TestAuthorizePublicWriteDeniesEachReason`) exercises all three reasons
independently: missing delegation with an otherwise-authorized actor,
present delegation with an under-permissioned actor, an expired delegation,
a revoked delegation, and a revoked membership overriding an otherwise-valid
delegation.

## Endpoints added in this slice

All restricted to `owner` via `manage_members`, except delegation
management which the matrix also grants to `organizer` via
`manage_delegations`:

- `PATCH /api/workspaces/{workspaceID}/members/{memberID}` — change role
  and/or set/clear `expiresAt`. Enforces the last-owner guard on demotion.
- `POST /api/workspaces/{workspaceID}/members/{memberID}/revoke` — revoke.
  Enforces the last-owner guard; `409` if already revoked.
- `GET /api/workspaces/{workspaceID}/delegations` — list every delegation
  ever granted in the workspace (active or not), for audit.
- `POST /api/workspaces/{workspaceID}/delegations` — create a delegation;
  `404` if the target cultural profile does not belong to this workspace.
- `POST /api/workspaces/{workspaceID}/delegations/{delegationID}/revoke` —
  revoke; `404` if already revoked or absent.

## Proof

`backend/internal/app/authority_integration_test.go`, run against disposable
PostgreSQL:

- `TestAuthorityMigrationWidensRoleAndAddsColumns` — fresh migration reaches
  schema version 10, creates `creator_delegations` and the new
  `workspace_members` columns, and the widened role check still accepts
  every legacy and new role value.
- `TestAuthorityRevokedMemberDeniedEverywhere` — a revoked actor receives
  `403` listing the workspace, event roles, and contacts; `authorize`
  returns `ErrMembershipDenied` directly; `authorizePublicWrite` returns
  `ErrMembershipDenied` even with an active delegation present; a second
  revoke is `409`.
- `TestAuthorityExpiredMembershipDeniedEverywhere` — an expired (not
  revoked) membership is denied identically.
- `TestAuthorityLastOwnerCannotBeRemovedDemotedOrRevoked` — demote, revoke,
  and remove all `409` against the sole active owner; the owner's access is
  unaffected by the rejected attempts.
- `TestAuthorizePublicWriteDeniesEachReason` — each of the three deny
  reasons independently, plus the allowed case.
- `TestUpdateMemberRoleRestrictedToOwner` — a non-owner gets `403`; a
  role change to `organizer` takes effect immediately (subsequent
  `authorize` call for `manage_delegations` succeeds).
- `TestDelegationEndpointsRestrictedAndScoped` — non-owner/non-organizer
  gets `403` on list/create/revoke; a delegation cannot be created against a
  cultural profile belonging to a different workspace (`404`, enforced by
  the composite `(id, workspace_id)` foreign key pattern from migration
  000008).

## Known limits

- **Legacy endpoints are role-name checks, not permission checks.** Every
  pre-AUTH-01 handler calls `requireWorkspaceRole(r, workspaceID, "owner",
  "member")` or `requireWorkspaceRole(r, workspaceID, "owner")`.
  `requireWorkspaceRole` treats the literal `"member"` as "any active role
  that carries `operate`", so a member promoted to `organizer`, `finance`,
  `door` or `crew` keeps baseline access to the existing surface
  (`TestPromotedRolesKeepBaselineAccessOnLegacyRoutes`), and owner-only
  handlers keep denying every non-owner role. Legacy access remains limited
  to routes that only need day-to-day operational access.
  Settlement reads, reports, settlement corrections/finalization, and paid
  event pricing use the `finance` permission. Free-event creation and
  nonpricing event edits remain operational work.
- No account-recovery flow beyond the ownership-transfer-by-promotion
  pattern; see "Recovery" above.
- No API surface yet lists members' delegation-adjacent history beyond the
  raw `GET .../delegations` audit list; no UI consumes any of this.
- `authorizePublicWrite` is unit/integration-tested directly; there is no
  publication outbox yet for it to be wired into (PUB-AUTH, #19).
- Real-world organization claims are intentionally not persisted as a
  distinct record type in this slice; if a future feature adds one, it must
  not be allowed to confer authority by itself.


## Private membership roster status

Current/scoped workspace reads include `accessState` for each retained membership:
active, expired or revoked, derived at database read time. Revocation takes
precedence over expiry; soft-removed rows remain outside the roster. Applicable
`expiresAt`/`revokedAt` instants accompany the state. Both routes return no-store,
including authorization failures. This roster snapshot does not grant authority;
every operation still checks the existing permission/expiry/revocation boundary.

Web role labels distinguish Owner, Organizer, Finance, Door and Crew. The legacy
member alias displays Crew. Expired/revoked status does not imply the configured
role is usable, and old responses without status remain unqualified. Metadata
is private workspace data and is absent from public event reads. See the
[disposable roster rehearsal](../qa/operator-journey-2026-09-30.md) for API,
regression and remaining browser/native evidence.


## Owner role-assignment page

The private Member roles page uses the existing owner-only membership PATCH.
Owners review Owner, Organizer, Finance, Door or Crew capabilities before saving;
the historical member alias defaults to Crew. The payload changes only role,
using the membership row ID, and preserves expiry/revocation. Inactive, unknown
or unresolved rows remain read-only; restoring access has no control in this page.
The server checks permissions and the last-active-owner invariant on every write.

The client reloads the authoritative workspace after a matched receipt. A denied
write clears the roster; an uncertain outcome fences new writes until refresh.
It does not replay uncertain mutations. Status and capability displays remain
snapshots, with browser/keyboard/screen-reader qualification still pending.
