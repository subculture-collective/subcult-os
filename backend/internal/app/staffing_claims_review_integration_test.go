package app

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Tests for the PR #233 review findings on shift self-claim.

func staffingPath(fx claimFixture, staffingID string) string {
	return "/api/events/" + fx.eventID + "/staffing/" + staffingID
}

func noticeCrewRecipients(t *testing.T, fx claimFixture) map[string]string {
	t.Helper()
	rows, err := fx.app.db.Query(t.Context(), lifecycleNoticeRecipientsSQL, fx.eventID, fx.workspaceID, []string{"assigned_crew"}, 501)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := map[string]string{}
	for rows.Next() {
		var email, sourceType, sourceID string
		var suppressed bool
		if err := rows.Scan(&email, &sourceType, &sourceID, &suppressed); err != nil {
			t.Fatal(err)
		}
		found[email] = sourceType + ":" + sourceID
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return found
}

// reminderTypesFor returns reminder_type by recipient for one staffing item.
func reminderTypesFor(t *testing.T, fx claimFixture, staffingID string) map[string]string {
	t.Helper()
	rows, err := fx.app.db.Query(t.Context(), `
		select recipient_email, reminder_type from reminder_events where source_type = 'staffing' and source_id = $1
	`, staffingID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := map[string]string{}
	for rows.Next() {
		var email, reminderType string
		if err := rows.Scan(&email, &reminderType); err != nil {
			t.Fatal(err)
		}
		found[email] = reminderType
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return found
}

// Finding 1: raising capacity on a shift with a legacy assignee leaves it
// open, and the assignee must still get reminders and lifecycle notices.
// Claim holders are reminded instead of the owners.
func TestStaffingHoldersStayRemindedAndNotifiedWhenPlacesRemain(t *testing.T) {
	fx := newClaimFixture(t)
	memberPersonID, _ := memberIdentity(t, fx.lifecycleFixture)
	memberEmail := fx.email("member")
	ownerEmail := fx.email("owner")
	participant := addClaimParticipant(t, fx, "holder", fx.roleID, "accepted")
	starts := claimClock.Add(30 * time.Hour)

	// Legacy assignee on a shift raised from one place to three.
	created := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing", map[string]any{
		"title": "Legacy door", "kind": "shift", "startsAt": starts.Format(time.RFC3339), "endsAt": starts.Add(time.Hour).Format(time.RFC3339),
	}, http.StatusOK)
	legacyID := mustString(t, created.JSON, "id")
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, legacyID), map[string]any{"assignedPersonId": memberPersonID}, http.StatusOK)
	widened := patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, legacyID), map[string]any{"capacity": 3}, http.StatusOK)
	if mustObject(t, widened.JSON)["status"] != "open" {
		t.Fatalf("widened shift = %s", widened.Body)
	}

	// A claim-only shift that is full, and a claim-only shift with places left.
	fullID := createClaimShift(t, fx, "Full bar", starts.Add(2*time.Hour), starts.Add(3*time.Hour), 1, 1)
	partialID := createClaimShift(t, fx, "Partial cloak", starts.Add(4*time.Hour), starts.Add(5*time.Hour), 3, 1)
	emptyID := createClaimShift(t, fx, "Empty merch", starts.Add(6*time.Hour), starts.Add(7*time.Hour), 2, 1)
	postJSON(t, fx.app, participant.cookie, claimPath(fullID, "claim"), nil, http.StatusOK)
	postJSON(t, fx.app, participant.cookie, claimPath(partialID, "claim"), nil, http.StatusOK)

	postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/reminders/sweep", map[string]any{"now": claimClock.Add(12 * time.Hour).Format(time.RFC3339)}, http.StatusOK)

	for _, check := range []struct {
		id        string
		recipient string
	}{{legacyID, memberEmail}, {fullID, participant.email}, {partialID, participant.email}} {
		got := reminderTypesFor(t, fx, check.id)
		if got[check.recipient] != "staffing.upcoming" || len(got) != 1 {
			t.Fatalf("item %s reminders = %#v, want only %s upcoming", check.id, got, check.recipient)
		}
	}
	if got := reminderTypesFor(t, fx, emptyID); got[ownerEmail] != "staffing.unassigned" || len(got) != 1 {
		t.Fatalf("unheld shift reminders = %#v, want the owner's unassigned reminder", got)
	}

	recipients := noticeCrewRecipients(t, fx)
	if recipients[memberEmail] != "crew_person:"+memberPersonID {
		t.Fatalf("legacy assignee on an open shift missing from notices: %#v", recipients)
	}
	if recipients[participant.email] != "crew_person:"+participant.personID {
		t.Fatalf("claim holder missing from notices: %#v", recipients)
	}
}

// Finding 2: an application-based single assignment is the bound account's
// person for duplicate and overlap checks.
func TestStaffingApplicationAssigneeCountsAsThatPerson(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "appassignee", fx.roleID, "")
	applicationID := applyForClaimRole(t, fx, participant, fx.roleID, "accepted")
	base := claimClock.Add(72 * time.Hour)
	shiftID := createClaimShift(t, fx, "Front door", base, base.Add(2*time.Hour), 2, 1)
	overlapID := createClaimShift(t, fx, "Overlapping bar", base.Add(time.Hour), base.Add(3*time.Hour), 2, 1)
	laterID := createClaimShift(t, fx, "Later cloak", base.Add(5*time.Hour), base.Add(6*time.Hour), 2, 1)

	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, shiftID), map[string]any{"assignedApplicationId": applicationID}, http.StatusOK)

	// Self-claim returns the existing hold without taking a second place.
	retry := postJSON(t, fx.app, participant.cookie, claimPath(shiftID, "claim"), nil, http.StatusOK)
	if obj := mustObject(t, retry.JSON); obj["claimed"] != true || obj["organizerManaged"] != true || obj["remainingPlaces"] != float64(1) {
		t.Fatalf("claim by application assignee = %s", retry.Body)
	}
	// The organizer addition is a no-op for the same person.
	postJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, shiftID)+"/claims", map[string]any{"personId": participant.personID}, http.StatusOK)
	if got := activeClaimCount(t, fx, shiftID); got != 0 {
		t.Fatalf("application assignee holds %d extra claims", got)
	}

	// Overlap counts the application assignment: self-claim, organizer
	// addition and person assignment are all refused.
	postJSON(t, fx.app, participant.cookie, claimPath(overlapID, "claim"), nil, http.StatusConflict)
	postJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, overlapID)+"/claims", map[string]any{"personId": participant.personID}, http.StatusConflict)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, overlapID), map[string]any{"assignedPersonId": participant.personID}, http.StatusConflict)

	// Assigning the application where the person already claimed a place is
	// refused as a duplicate.
	postJSON(t, fx.app, participant.cookie, claimPath(laterID, "claim"), nil, http.StatusOK)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, shiftID), map[string]any{"clearAssignee": true}, http.StatusOK)
	duplicate := patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, laterID), map[string]any{"assignedApplicationId": applicationID}, http.StatusConflict)
	if !strings.Contains(duplicate.Body, "already holds") {
		t.Fatalf("duplicate application assignment = %s", duplicate.Body)
	}
	// An application assignment overlapping a claimed place is refused too.
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, overlapID), map[string]any{"assignedApplicationId": applicationID}, http.StatusOK)
	postJSON(t, fx.app, participant.cookie, claimPath(shiftID, "claim"), nil, http.StatusConflict)

	// A revoked member can no longer be given the single assignment.
	revoked := addClaimParticipant(t, fx, "revokedassignee", fx.roleID, "")
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at = now() where workspace_id = $1 and person_id = $2`, fx.workspaceID, revoked.personID); err != nil {
		t.Fatal(err)
	}
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, laterID), map[string]any{"capacity": 3}, http.StatusOK)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, laterID), map[string]any{"assignedPersonId": revoked.personID}, http.StatusBadRequest)
}

// Finding 3: only owners, who manage staffing, see who holds each place,
// and nobody sees a holder's email.
func TestStaffingClaimHoldersVisibleOnlyToOwners(t *testing.T) {
	fx := newClaimFixture(t)
	holder := addClaimParticipant(t, fx, "visible", fx.roleID, "accepted")
	if _, err := fx.app.db.Exec(t.Context(), `update people set display_name = '' where id = $1`, holder.personID); err != nil {
		t.Fatal(err)
	}
	crew := addClaimParticipant(t, fx, "crewviewer", fx.roleID, "")
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set role = 'crew' where workspace_id = $1 and person_id = $2`, fx.workspaceID, crew.personID); err != nil {
		t.Fatal(err)
	}
	starts := claimClock.Add(60 * time.Hour)
	shiftID := createClaimShift(t, fx, "Stage door", starts, starts.Add(2*time.Hour), 3, 1)
	postJSON(t, fx.app, holder.cookie, claimPath(shiftID, "claim"), nil, http.StatusOK)

	for name, cookie := range map[string]*http.Cookie{"crew": crew.cookie, "member": fx.memberCookie} {
		listed := getJSON(t, fx.app, cookie, "/api/events/"+fx.eventID+"/staffing", http.StatusOK)
		for _, private := range []string{holder.email, holder.personID} {
			if strings.Contains(listed.Body, private) {
				t.Fatalf("%s staffing response exposed %q: %s", name, private, listed.Body)
			}
		}
		for _, raw := range listed.JSON.([]any) {
			item := mustObject(t, raw)
			if item["id"] != shiftID {
				continue
			}
			if claims := item["claims"].([]any); len(claims) != 0 || item["filledCount"] != float64(1) {
				t.Fatalf("%s sees claims=%#v filled=%v, want a count only", name, claims, item["filledCount"])
			}
		}
	}

	owner := getStaffingItem(t, fx, shiftID)
	claims := owner["claims"].([]any)
	if len(claims) != 1 || mustString(t, claims[0], "personId") != holder.personID || mustString(t, claims[0], "name") != "Member" {
		t.Fatalf("owner claims = %#v, want the holder named without email", claims)
	}
	if body := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing", http.StatusOK).Body; strings.Contains(body, holder.email) {
		t.Fatalf("owner staffing response exposed the holder email: %s", body)
	}
}

// Finding 4: moving or reopening a held shift rechecks every holder for
// overlap, and cancelling a shift ends its claimed places.
func TestStaffingShiftMoveCancelAndReopenKeepHoldersConsistent(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "mover", fx.roleID, "accepted")
	other := addClaimParticipant(t, fx, "mover2", fx.roleID, "accepted")
	base := claimClock.Add(80 * time.Hour)
	firstID := createClaimShift(t, fx, "First set", base, base.Add(2*time.Hour), 2, 1)
	secondID := createClaimShift(t, fx, "Second set", base.Add(3*time.Hour), base.Add(5*time.Hour), 3, 1)
	postJSON(t, fx.app, participant.cookie, claimPath(firstID, "claim"), nil, http.StatusOK)
	postJSON(t, fx.app, participant.cookie, claimPath(secondID, "claim"), nil, http.StatusOK)
	postJSON(t, fx.app, other.cookie, claimPath(secondID, "claim"), nil, http.StatusOK)

	// Moving the second shift onto the first is refused with a count only.
	refused := patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, secondID), map[string]any{"startsAt": base.Add(time.Hour).Format(time.RFC3339)}, http.StatusConflict)
	if !strings.Contains(refused.Body, "1 place holder holds another overlapping shift") {
		t.Fatalf("move refusal = %s", refused.Body)
	}
	for _, private := range []string{participant.name, participant.email, participant.personID} {
		if strings.Contains(refused.Body, private) {
			t.Fatalf("move refusal exposed %q: %s", private, refused.Body)
		}
	}
	// A move that keeps every holder clear is accepted.
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, secondID), map[string]any{"startsAt": base.Add(2 * time.Hour).Format(time.RFC3339)}, http.StatusOK)

	// Cancelling ends both claimed places, with an audit entry for each.
	cancelled := patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, secondID), map[string]any{"status": "cancelled"}, http.StatusOK)
	if obj := mustObject(t, cancelled.JSON); obj["filledCount"] != float64(0) || len(obj["claims"].([]any)) != 0 {
		t.Fatalf("cancelled shift = %s", cancelled.Body)
	}
	if got := activeClaimCount(t, fx, secondID); got != 0 {
		t.Fatalf("cancelled shift still has %d active claims", got)
	}
	var removed, audited int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*) from event_staffing_claims where staffing_item_id = $1 and status = 'removed' and ended_by_person_id is not null
	`, secondID).Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*) from audit_entries where action = 'staffing.claim_removed' and metadata->>'staffingItemId' = $1 and metadata->>'reason' = 'shift_cancelled'
	`, secondID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if removed != 2 || audited != 2 {
		t.Fatalf("cancel ended %d claims with %d audit entries, want 2 and 2", removed, audited)
	}
	// Reopening starts with no claimed places, so it can even overlap.
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, secondID), map[string]any{"startsAt": base.Add(time.Hour).Format(time.RFC3339)}, http.StatusOK)
	reopened := patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, secondID), map[string]any{"status": "open"}, http.StatusOK)
	if obj := mustObject(t, reopened.JSON); obj["filledCount"] != float64(0) {
		t.Fatalf("reopened shift = %s", reopened.Body)
	}

	// Reopening a shift whose single assignee now overlaps another place is
	// refused.
	thirdID := createClaimShift(t, fx, "Third set", base.Add(6*time.Hour), base.Add(7*time.Hour), 1, 1)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, thirdID), map[string]any{"assignedPersonId": participant.personID}, http.StatusOK)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, thirdID), map[string]any{"status": "cancelled"}, http.StatusOK)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, thirdID), map[string]any{"startsAt": base.Add(time.Hour).Format(time.RFC3339)}, http.StatusOK)
	blocked := patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, thirdID), map[string]any{"status": "open"}, http.StatusConflict)
	if !strings.Contains(blocked.Body, "overlapping") {
		t.Fatalf("reopen refusal = %s", blocked.Body)
	}
}

// Finding 5: a legacy assignee sees an organizer-managed place with no
// release or release request, and the request route refuses it.
func TestStaffingOrganizerAssignmentIsMarkedOrganizerManaged(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "managed", fx.roleID, "accepted")
	starts := claimClock.Add(50 * time.Hour)
	shiftID := createClaimShift(t, fx, "Lights", starts, starts.Add(2*time.Hour), 2, 1)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, shiftID), map[string]any{"assignedPersonId": participant.personID}, http.StatusOK)

	listed, ok := openShift(t, fx, participant, shiftID)
	if !ok || !listed.Claimed || !listed.OrganizerManaged || listed.CanRelease {
		t.Fatalf("legacy assignee listing = %#v, %v", listed, ok)
	}
	postJSON(t, fx.app, participant.cookie, claimPath(shiftID, "release-request"), nil, http.StatusConflict)

	selfClaimed := createClaimShift(t, fx, "Sound", starts.Add(3*time.Hour), starts.Add(4*time.Hour), 2, 1)
	postJSON(t, fx.app, participant.cookie, claimPath(selfClaimed, "claim"), nil, http.StatusOK)
	if own, _ := openShift(t, fx, participant, selfClaimed); own.OrganizerManaged || !own.CanRelease {
		t.Fatalf("self-claimed listing = %#v", own)
	}
}

// Finding 6: release requests are only for after the deadline, and repeats
// neither notify nor audit again.
func TestStaffingReleaseRequestOnlyAfterDeadline(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "requester", fx.roleID, "accepted")
	starts := claimClock.Add(48 * time.Hour)
	shiftID := createClaimShift(t, fx, "Coat check", starts, starts.Add(2*time.Hour), 2, 6)
	postJSON(t, fx.app, participant.cookie, claimPath(shiftID, "claim"), nil, http.StatusOK)

	early := postJSON(t, fx.app, participant.cookie, claimPath(shiftID, "release-request"), nil, http.StatusConflict)
	if !strings.Contains(early.Body, "release the place yourself") {
		t.Fatalf("early release request = %s", early.Body)
	}
	fx.app.now = func() time.Time { return starts.Add(-6 * time.Hour) }
	for range 3 {
		postJSON(t, fx.app, participant.cookie, claimPath(shiftID, "release-request"), nil, http.StatusOK)
	}
	var notifications, audits int
	if err := fx.app.db.QueryRow(t.Context(), `
		select (select count(*) from notification_events where notification_type = 'staffing.release_request' and event_id = $1),
		       (select count(*) from audit_entries where action = 'staffing.release_requested' and metadata->>'staffingItemId' = $2)
	`, fx.eventID, shiftID).Scan(&notifications, &audits); err != nil {
		t.Fatal(err)
	}
	if notifications != 1 || audits != 1 {
		t.Fatalf("release requests notifications=%d audits=%d, want 1 and 1", notifications, audits)
	}
}

// Finding 8: the database refuses claim rows and eligible-role rows that
// mix events.
func TestStaffingClaimRowsMustMatchTheirEvent(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "integrity", fx.roleID, "accepted")
	starts := claimClock.Add(40 * time.Hour)
	shiftID := createClaimShift(t, fx, "Bar", starts, starts.Add(time.Hour), 2, 1)
	otherEventID := mustString(t, createEvent(t, fx.lifecycleFixture, "Other Event", 10), "id")
	otherRole := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+otherEventID+"/roles", map[string]any{"name": "Elsewhere", "capacity": 5, "public": false}, http.StatusOK)

	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_staffing_claims (staffing_item_id, event_id, person_id, source, created_by_person_id)
		values ($1, $2, $3, 'organizer', $3)
	`, shiftID, otherEventID, participant.personID); err == nil {
		t.Fatal("a claim was stored against another event")
	}
	if _, err := fx.app.db.Exec(t.Context(), `
		insert into event_staffing_claim_roles (staffing_item_id, event_id, role_id) values ($1, $2, $3)
	`, shiftID, fx.eventID, mustString(t, otherRole.JSON, "id")); err == nil {
		t.Fatal("a role of another event was made eligible for this shift")
	}
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath(fx, shiftID), map[string]any{"claimRoleIds": []string{mustString(t, otherRole.JSON, "id")}}, http.StatusBadRequest)
}
