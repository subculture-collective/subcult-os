# Participant portal

Status: implemented PORTALS-01 first slice. The authenticated participant
portal at `/participant` reads `GET /api/me/participant-portal`. It provides a
person's own staffing schedule and event-linked commitments. It does not make a
role application an assignment, create an account from an email address, or
add an application-to-person binding.

## Access boundary

The API identifies the viewer from the current authenticated session. It
returns a staffing item only when `assigned_person_id` equals that person and
the person has an active workspace membership. Removed, revoked, and expired
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

There is no verified account-claim flow for existing applications. Any future
binding must require a deliberate, verified claim and preserve that an
application is not an assignment. Vendor fees, invoice generation, payout
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
