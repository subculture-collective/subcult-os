# Participant portal

Status: implemented PORTALS-01 first slice. The authenticated participant
portal at `/participant` reads `GET /api/me/participant-portal`. It provides a
person's own staffing schedule and event-linked commitments. It does not make a
role application an assignment, create an account from an email address, or
add an application-to-person binding.

## Access boundary

The API identifies the viewer from the current authenticated session. It
returns a staffing item only when `assigned_person_id` equals that person, or
the person holds an active claimed place on it, and the person has an active
workspace membership. Removed, revoked, and expired
memberships are excluded in the query and rechecked before the response is
written. Reassigning an item changes what the next request returns.

`assigned_application_id` is intentionally excluded. An event role application
stores a submitted name, email, message, and review status. Even an accepted
application with text matching an account email does not prove control of that
account and does not authorize portal access.

The response uses `Cache-Control: private, no-store`. The web view clears its
previous portal state before a refresh and after a failed request, so an
authorization failure has no previously loaded assignment content to render.
The server-side rendered regression covers the denied state. A synthetic browser
journey in the installed development preview verified that membership revocation
followed by Refresh removes the loaded assignment and commitment, and logout
followed by Refresh shows an unauthorized error without those records.

## Returned fields

For each person-backed staffing assignment, the API returns only event ID and
title, staffing item ID, title, kind, start/end times, status, and participant
requirements. For a
commitment, it returns only ID, event ID/title, title, due time, and status.
Commitments appear only when their owner is the current person and their event
also has a current person-backed assignment.

The portal does not return staffing notes, application names/emails/messages or
statuses, contacts, commitment descriptions, ticket data, finance/settlement
data, payment-provider references, payout data, membership roles, or audit
metadata. The workspace header links authenticated users to `/participant`.

## Participant requirements

Staffing items have a separate `participant_requirements` field. It defaults to
an empty string, is capped at 2,000 characters, and is written only by an
event owner through the existing staffing create and update routes. Operator
`notes` remain private and are never reused as participant requirements.

The participant portal returns requirements only for an active,
person-backed assignment belonging to the authenticated account. Application
assignments, assignments for another person, revoked memberships, contacts,
ticket and settlement data remain excluded.

An application becomes person-backed only through the session binding
described under open shifts. There is still no flow that claims an existing
anonymous application, and an approved application remains distinct from an
assignment. Vendor fees, invoice generation, payout
records, and payout reminders remain outside this slice until a pilot shows a
need and defines the authority and finance boundaries.

## Verification scope

The backend integration test includes authenticated person assignment and own
commitment visibility, application-only exclusion even when its email matches,
private-field sentinels, reassignment on a later request, revoked membership,
the registered HTTP route, session middleware, and cache control. The web
render test includes populated and denied states. A separate synthetic browser journey verified populated desktop and mobile
layouts, membership revocation and refresh, and logout and refresh in the
installed local preview. This is local browser evidence; it does not qualify
a physical mobile device, a provider flow, or a production deployment.

## Open-shift self-claim

Status: implemented for issue #218. An owner opens one timed shift on one event
to self-claim. Eligible participants claim and release their own places in the
portal under the shift's capacity and release cutoff.

### Shift policy

`PATCH /api/events/{eventID}/staffing/{staffingID}` accepts four owner-only
fields:

- `capacity`: places on the item, 1 to 100. Every item, including tasks and
  items from before migration 31, defaults to 1.
- `claimRoleIds`: the event roles whose approved applicants may claim.
- `releaseCutoffHours`: 0 to 720, default 24. A participant can release until
  `startsAt` minus this many hours. At that instant and later the API refuses.
- `claimsOpen`: requires a shift with start and end times and at least one
  eligible role. The database also rejects an open untimed item.

Reducing capacity below the places held returns 409.

### Who can claim

A person can claim when all of these hold inside the claim transaction:

- They are signed in and hold a current workspace membership: not removed,
  revoked or expired. The membership row is share-locked, so a revocation
  either commits first and denies the claim, or waits for it.
- Their application for an eligible role is `accepted` or `confirmed` and is
  bound to their account. Submitted, under-review and waitlisted applications
  do not qualify.
- The event is published. Draft events and End of Night refuse claims.
- The shift is open to claims, open or assigned, and has not started.

An application is bound to an account only when a signed-in person submits it
with the same email as their verified account email. The submission sets
`applicant_person_id` and the organizer list shows "signed-in account". An
anonymous submission, or a signed-in one with a different email, stays
unbound and cannot claim even when its email text matches an account.
Earlier anonymous applications are not bound retroactively; the organizer can
withdraw one and the person can apply again while signed in.

### One capacity authority

Places held = the legacy single assignee (`assigned_person_id` or
`assigned_application_id`, at most one) plus active rows in
`event_staffing_claims`. Participant claims and releases, organizer additions
and removals, and organizer single-assignee edits all lock the event row and
then the staffing item row before counting. A final place is therefore granted
once. A partial unique index allows one active claim per person per shift.

A retried claim returns the existing place without taking another. A retried
release after success returns the released state and frees nothing further.
The item moves between `open` and `assigned` as places fill and free;
completed and cancelled items keep their status.

Overlap rule: a person cannot hold two places on the same event whose
`[startsAt, endsAt)` windows overlap. Windows that only touch are allowed.
Cancelled and untimed items never conflict. The rule applies to self-claims,
organizer additions and organizer person assignments.

### Organizer override

Owners can add a current workspace member to a place
(`POST .../staffing/{staffingID}/claims`) and remove any place
(`DELETE .../claims/{claimID}`). The override skips the role policy and the
release cutoff. Capacity, duplicate and overlap rules still apply. An addition
queues the existing `staffing.assignment` notification.

### Participant routes and returned fields

- `GET /api/me/open-shifts` lists claimable shifts for the person's eligible
  roles and every shift they currently claim.
- `POST /api/me/open-shifts/{staffingID}/claim` and `.../release`.
- `POST /api/me/open-shifts/{staffingID}/release-request` is the organizer
  contact action after the cutoff. It queues a private notification to each
  workspace owner, at most once per place and owner, and releases nothing.
  The participant never receives an owner's address.

An open shift returns only its ID, event ID and title, title, start and end,
participant requirements, `remainingPlaces`, whether the viewer holds a place,
whether they can still release it, and the release deadline. It never returns
who else holds a place, their contacts, operator notes, application data or
finance data. A held place appears in the existing assignment list with status
`assigned`, also while other places on the shift stay open. Responses use
`Cache-Control: private, no-store`.

### Not in this slice

Reminders still go only to the legacy single assignee, and the event archive
records only that assignee's name. Lifecycle-notice recipients include
claimants. Recurring series and a separate volunteer platform remain out of
scope.

### Verification

`staffing_claims_integration_test.go` covers four racing writers for the final
place (two participant claims, an organizer assignment and an organizer
addition) over four shifts, concurrent claim retries, pending, under-review,
waitlisted, unbound and wrong-role applications, draft and closed events,
membership revocation, the overlap rule, release one microsecond before, at
and after the cutoff, the release request, organizer override and capacity
limits, portal privacy, claimants as lifecycle-notice recipients, and an upgrade from a version-30 database with a
legacy assignee. Web render tests cover the claim, full, release, after-cutoff
and pending states and the organizer panel. The repository has no browser
journey harness, so no browser journey was run for this slice.
