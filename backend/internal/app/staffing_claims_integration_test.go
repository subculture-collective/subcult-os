package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// claimClock is the fixed wall clock for shift-claim tests; every shift in
// these fixtures starts after it.
var claimClock = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

type claimFixture struct {
	lifecycleFixture
	eventID string
	slug    string
	roleID  string
}

type claimParticipant struct {
	cookie   *http.Cookie
	personID string
	email    string
	name     string
}

func newClaimFixture(t *testing.T) claimFixture {
	t.Helper()
	fx := newLifecycleFixture(t)
	fx.app.now = func() time.Time { return claimClock }
	eventID := mustString(t, createEvent(t, fx, "Shift Claim Event", 50), "id")
	published := publishEvent(t, fx, eventID)
	role := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/roles", map[string]any{"name": "Volunteer", "capacity": 50, "public": true}, http.StatusOK)
	return claimFixture{lifecycleFixture: fx, eventID: eventID, slug: mustString(t, published, "publicSlug"), roleID: mustString(t, role.JSON, "id")}
}

// addClaimParticipant creates a current workspace member. When status is not
// empty, the member also submits a signed-in application for roleID and the
// owner moves it to that status.
func addClaimParticipant(t *testing.T, fx claimFixture, prefix, roleID, status string) claimParticipant {
	t.Helper()
	email := fx.email(prefix)
	name := "Participant " + prefix
	invite := postJSON(t, fx.app, fx.ownerCookie, "/api/workspaces/"+fx.workspaceID+"/invitations", map[string]any{"email": email}, http.StatusOK)
	cookie := signupAndVerifyCookie(t, fx.app, email, name)
	postJSON(t, fx.app, cookie, "/api/invitations/"+mustString(t, invite.JSON, "token")+"/accept", map[string]any{}, http.StatusOK)
	var personID string
	if err := fx.app.db.QueryRow(t.Context(), `select id from people where email = $1`, email).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	participant := claimParticipant{cookie: cookie, personID: personID, email: email, name: name}
	if status != "" {
		applyForClaimRole(t, fx, participant, roleID, status)
	}
	return participant
}

func applyForClaimRole(t *testing.T, fx claimFixture, participant claimParticipant, roleID, status string) string {
	t.Helper()
	application := postJSON(t, fx.app, participant.cookie, "/api/public/events/"+fx.slug+"/role-applications", map[string]any{
		"roleId": roleID, "applicantName": participant.name, "applicantEmail": participant.email, "message": participant.name + " private application message",
	}, http.StatusOK)
	if application.JSON.(map[string]any)["accountLinked"] != true {
		t.Fatalf("signed-in application was not bound: %s", application.Body)
	}
	applicationID := mustString(t, application.JSON, "id")
	if status != "submitted" {
		patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/role-applications/"+applicationID, map[string]any{"status": status}, http.StatusOK)
	}
	return applicationID
}

// createClaimShift creates a timed shift and opens it to the given roles.
func createClaimShift(t *testing.T, fx claimFixture, title string, startsAt, endsAt time.Time, capacity, cutoffHours int, roleIDs ...string) string {
	t.Helper()
	created := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing", map[string]any{
		"title": title, "kind": "shift", "notes": title + " operator-only note", "participantRequirements": "Wear closed shoes.",
		"startsAt": startsAt.Format(time.RFC3339), "endsAt": endsAt.Format(time.RFC3339),
	}, http.StatusOK)
	staffingID := mustString(t, created.JSON, "id")
	if len(roleIDs) == 0 {
		roleIDs = []string{fx.roleID}
	}
	updated := patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing/"+staffingID, map[string]any{
		"capacity": capacity, "releaseCutoffHours": cutoffHours, "claimRoleIds": roleIDs, "claimsOpen": true,
	}, http.StatusOK)
	if obj := mustObject(t, updated.JSON); obj["claimsOpen"] != true || obj["capacity"] != float64(capacity) {
		t.Fatalf("shift not opened: %s", updated.Body)
	}
	return staffingID
}

func claimPath(staffingID, action string) string {
	return "/api/me/open-shifts/" + staffingID + "/" + action
}

// rawJSON sends one request without failing the test, for use in goroutines.
func rawJSON(app *App, cookie *http.Cookie, method, path string, payload any) (int, string) {
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func activeClaimCount(t *testing.T, fx claimFixture, staffingID string) int {
	t.Helper()
	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_staffing_claims where staffing_item_id = $1 and status = 'active'`, staffingID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func staffingItemState(t *testing.T, fx claimFixture, staffingID string) (status string, legacy bool) {
	t.Helper()
	if err := fx.app.db.QueryRow(t.Context(), `
		select status, assigned_person_id is not null or assigned_application_id is not null
		from event_staffing_items where id = $1
	`, staffingID).Scan(&status, &legacy); err != nil {
		t.Fatal(err)
	}
	return status, legacy
}

func openShift(t *testing.T, fx claimFixture, participant claimParticipant, staffingID string) (participantOpenShiftDTO, bool) {
	t.Helper()
	response := getJSON(t, fx.app, participant.cookie, "/api/me/open-shifts", http.StatusOK)
	var shifts []participantOpenShiftDTO
	if err := json.Unmarshal([]byte(response.Body), &shifts); err != nil {
		t.Fatal(err)
	}
	for _, shift := range shifts {
		if shift.StaffingItemID == staffingID {
			return shift, true
		}
	}
	return participantOpenShiftDTO{}, false
}

func TestStaffingShiftClaimFinalPlaceRace(t *testing.T) {
	fx := newClaimFixture(t)
	first := addClaimParticipant(t, fx, "first", fx.roleID, "accepted")
	second := addClaimParticipant(t, fx, "second", fx.roleID, "confirmed")
	third := addClaimParticipant(t, fx, "third", fx.roleID, "accepted")
	memberPersonID, _ := memberIdentity(t, fx.lifecycleFixture)
	ownerID := ownerPersonID(t, fx.lifecycleFixture)

	for round := range 4 {
		starts := claimClock.Add(time.Duration(48+round*10) * time.Hour)
		staffingID := createClaimShift(t, fx, "Door rotation", starts, starts.Add(2*time.Hour), 2, 1)
		// One place is already held, so the race is for the final place.
		postJSON(t, fx.app, third.cookie, claimPath(staffingID, "claim"), nil, http.StatusOK)

		type outcome struct {
			status int
			body   string
		}
		requests := []func() (int, string){
			func() (int, string) {
				return rawJSON(fx.app, first.cookie, http.MethodPost, claimPath(staffingID, "claim"), nil)
			},
			func() (int, string) {
				return rawJSON(fx.app, second.cookie, http.MethodPost, claimPath(staffingID, "claim"), nil)
			},
			func() (int, string) {
				return rawJSON(fx.app, fx.ownerCookie, http.MethodPatch, "/api/events/"+fx.eventID+"/staffing/"+staffingID, map[string]any{"assignedPersonId": memberPersonID})
			},
			func() (int, string) {
				return rawJSON(fx.app, fx.ownerCookie, http.MethodPost, "/api/events/"+fx.eventID+"/staffing/"+staffingID+"/claims", map[string]any{"personId": ownerID})
			},
		}
		results := make([]outcome, len(requests))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, request := range requests {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				status, body := request()
				results[i] = outcome{status, body}
			}()
		}
		close(start)
		wg.Wait()

		successes := 0
		for i, result := range results {
			switch result.status {
			case http.StatusOK:
				successes++
			case http.StatusConflict:
				if !strings.Contains(result.body, "full") && !strings.Contains(result.body, "capacity") && !strings.Contains(result.body, "already") {
					t.Fatalf("round %d request %d conflict body = %s", round, i, result.body)
				}
			default:
				t.Fatalf("round %d request %d status=%d body=%s", round, i, result.status, result.body)
			}
		}
		claims := activeClaimCount(t, fx, staffingID)
		status, legacy := staffingItemState(t, fx, staffingID)
		filled := claims
		if legacy {
			filled++
		}
		if successes != 1 || filled != 2 {
			t.Fatalf("round %d successes=%d filled=%d results=%#v, want exactly one final-place winner and two held places", round, successes, filled, results)
		}
		if status != "assigned" {
			t.Fatalf("round %d full shift status = %q, want assigned", round, status)
		}
	}
}

func TestStaffingShiftClaimRetryIsIdempotent(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "retry", fx.roleID, "accepted")
	starts := claimClock.Add(72 * time.Hour)
	staffingID := createClaimShift(t, fx, "Bar", starts, starts.Add(3*time.Hour), 3, 1)

	start := make(chan struct{})
	var wg sync.WaitGroup
	statuses := make([]int, 3)
	for i := range statuses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			statuses[i], _ = rawJSON(fx.app, participant.cookie, http.MethodPost, claimPath(staffingID, "claim"), nil)
		}()
	}
	close(start)
	wg.Wait()
	for _, status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent retry statuses = %v, want all 200", statuses)
		}
	}
	retry := postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "claim"), nil, http.StatusOK)
	var shift participantOpenShiftDTO
	if err := json.Unmarshal([]byte(retry.Body), &shift); err != nil {
		t.Fatal(err)
	}
	if !shift.Claimed || shift.RemainingPlaces != 2 || activeClaimCount(t, fx, staffingID) != 1 {
		t.Fatalf("retry shift=%#v claims=%d, want one claim and two remaining", shift, activeClaimCount(t, fx, staffingID))
	}
}

func TestStaffingShiftClaimRequiresApprovedBoundRoleAndOpenEvent(t *testing.T) {
	fx := newClaimFixture(t)
	starts := claimClock.Add(48 * time.Hour)
	staffingID := createClaimShift(t, fx, "Coat check", starts, starts.Add(2*time.Hour), 5, 1)

	pending := addClaimParticipant(t, fx, "pending", fx.roleID, "submitted")
	reviewing := addClaimParticipant(t, fx, "reviewing", fx.roleID, "under_review")
	waitlisted := addClaimParticipant(t, fx, "waitlisted", fx.roleID, "waitlisted")
	for _, participant := range []claimParticipant{pending, reviewing, waitlisted} {
		postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "claim"), nil, http.StatusForbidden)
		if _, listed := openShift(t, fx, participant, staffingID); listed {
			t.Fatalf("%s sees a shift it cannot claim", participant.name)
		}
	}

	// An accepted application whose email merely matches the account, but
	// that was submitted without that signed-in account, does not qualify.
	unbound := addClaimParticipant(t, fx, "unbound", fx.roleID, "")
	anonymous := postJSON(t, fx.app, nil, "/api/public/events/"+fx.slug+"/role-applications", map[string]any{
		"roleId": fx.roleID, "applicantName": unbound.name, "applicantEmail": unbound.email,
	}, http.StatusOK)
	if mustObject(t, anonymous.JSON)["accountLinked"] != false {
		t.Fatalf("anonymous application was bound: %s", anonymous.Body)
	}
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/role-applications/"+mustString(t, anonymous.JSON, "id"), map[string]any{"status": "accepted"}, http.StatusOK)
	postJSON(t, fx.app, unbound.cookie, claimPath(staffingID, "claim"), nil, http.StatusForbidden)

	// A signed-in application with a different email stays unbound too.
	other := addClaimParticipant(t, fx, "otheremail", fx.roleID, "")
	mismatched := postJSON(t, fx.app, other.cookie, "/api/public/events/"+fx.slug+"/role-applications", map[string]any{
		"roleId": fx.roleID, "applicantName": other.name, "applicantEmail": "someone-else+" + fx.suffix + "@example.test",
	}, http.StatusOK)
	if mustObject(t, mismatched.JSON)["accountLinked"] != false {
		t.Fatalf("application with another email was bound: %s", mismatched.Body)
	}

	// An approved role that is not eligible for this shift does not qualify.
	otherRole := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/roles", map[string]any{"name": "Performer", "capacity": 50, "public": true}, http.StatusOK)
	performer := addClaimParticipant(t, fx, "performer", mustString(t, otherRole.JSON, "id"), "accepted")
	postJSON(t, fx.app, performer.cookie, claimPath(staffingID, "claim"), nil, http.StatusForbidden)

	approved := addClaimParticipant(t, fx, "approved", fx.roleID, "accepted")
	listed, ok := openShift(t, fx, approved, staffingID)
	if !ok || listed.Claimed || listed.RemainingPlaces != 5 {
		t.Fatalf("approved participant listing = %#v, %v", listed, ok)
	}

	// Closed claims, a started shift and a non-member are refused.
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing/"+staffingID, map[string]any{"claimsOpen": false}, http.StatusOK)
	postJSON(t, fx.app, approved.cookie, claimPath(staffingID, "claim"), nil, http.StatusConflict)
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing/"+staffingID, map[string]any{"claimsOpen": true}, http.StatusOK)
	fx.app.now = func() time.Time { return starts }
	postJSON(t, fx.app, approved.cookie, claimPath(staffingID, "claim"), nil, http.StatusConflict)
	fx.app.now = func() time.Time { return claimClock }
	outsider := newLifecycleFixture(t, fx.app)
	postJSON(t, fx.app, outsider.ownerCookie, claimPath(staffingID, "claim"), nil, http.StatusForbidden)

	// Revoked membership refuses claims and hides the listing.
	var memberRowID string
	if err := fx.app.db.QueryRow(t.Context(), `select id from workspace_members where workspace_id = $1 and person_id = $2`, fx.workspaceID, approved.personID).Scan(&memberRowID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at = now() where id = $1`, memberRowID); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, approved.cookie, claimPath(staffingID, "claim"), nil, http.StatusForbidden)
	if _, listed := openShift(t, fx, approved, staffingID); listed {
		t.Fatal("revoked member still sees the open shift")
	}
	if _, err := fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at = null where id = $1`, memberRowID); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, approved.cookie, claimPath(staffingID, "claim"), nil, http.StatusOK)

	// End of Night closes the event to claims.
	late := addClaimParticipant(t, fx, "late", fx.roleID, "accepted")
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/end-of-night", map[string]any{}, http.StatusOK)
	postJSON(t, fx.app, late.cookie, claimPath(staffingID, "claim"), nil, http.StatusConflict)
	if got := activeClaimCount(t, fx, staffingID); got != 1 {
		t.Fatalf("active claims = %d, want only the approved participant", got)
	}
}

func TestStaffingShiftClaimRefusesDraftEvent(t *testing.T) {
	fx := newClaimFixture(t)
	draftID := mustString(t, createEvent(t, fx.lifecycleFixture, "Draft Event", 10), "id")
	created := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+draftID+"/staffing", map[string]any{
		"title": "Setup", "kind": "shift", "startsAt": claimClock.Add(24 * time.Hour).Format(time.RFC3339), "endsAt": claimClock.Add(26 * time.Hour).Format(time.RFC3339),
	}, http.StatusOK)
	draftRole := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+draftID+"/roles", map[string]any{"name": "Setup crew", "capacity": 50, "public": false}, http.StatusOK)
	staffingID := mustString(t, created.JSON, "id")
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+draftID+"/staffing/"+staffingID, map[string]any{"claimsOpen": true, "claimRoleIds": []string{mustString(t, draftRole.JSON, "id")}}, http.StatusOK)
	member := claimParticipant{cookie: fx.memberCookie}
	postJSON(t, fx.app, member.cookie, claimPath(staffingID, "claim"), nil, http.StatusConflict)
}

func TestStaffingShiftClaimOverlapRule(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "overlap", fx.roleID, "accepted")
	base := claimClock.Add(96 * time.Hour)
	early := createClaimShift(t, fx, "Early door", base, base.Add(2*time.Hour), 2, 1)
	overlapping := createClaimShift(t, fx, "Overlapping bar", base.Add(time.Hour), base.Add(3*time.Hour), 2, 1)
	touching := createClaimShift(t, fx, "Touching close-down", base.Add(2*time.Hour), base.Add(4*time.Hour), 2, 1)

	postJSON(t, fx.app, participant.cookie, claimPath(early, "claim"), nil, http.StatusOK)
	refused := postJSON(t, fx.app, participant.cookie, claimPath(overlapping, "claim"), nil, http.StatusConflict)
	if !strings.Contains(refused.Body, "overlaps") {
		t.Fatalf("overlap refusal = %s", refused.Body)
	}
	postJSON(t, fx.app, participant.cookie, claimPath(touching, "claim"), nil, http.StatusOK)

	// The organizer override follows the same overlap rule.
	patchJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing/"+overlapping, map[string]any{"assignedPersonId": participant.personID}, http.StatusConflict)
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing/"+overlapping+"/claims", map[string]any{"personId": participant.personID}, http.StatusConflict)

	// Releasing the early shift clears the conflict.
	postJSON(t, fx.app, participant.cookie, claimPath(early, "release"), nil, http.StatusOK)
	postJSON(t, fx.app, participant.cookie, claimPath(touching, "release"), nil, http.StatusOK)
	postJSON(t, fx.app, participant.cookie, claimPath(overlapping, "claim"), nil, http.StatusOK)
}

func TestStaffingShiftReleaseCutoffBoundaries(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "release", fx.roleID, "accepted")
	starts := claimClock.Add(48 * time.Hour)
	staffingID := createClaimShift(t, fx, "Merch table", starts, starts.Add(2*time.Hour), 1, 6)
	deadline := starts.Add(-6 * time.Hour)
	claimed := postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "claim"), nil, http.StatusOK)
	var shift participantOpenShiftDTO
	if err := json.Unmarshal([]byte(claimed.Body), &shift); err != nil {
		t.Fatal(err)
	}
	if shift.ReleaseDeadline != deadline.Format(time.RFC3339Nano) || !shift.CanRelease || shift.RemainingPlaces != 0 {
		t.Fatalf("claimed shift = %#v, want deadline %s", shift, deadline)
	}
	if status, _ := staffingItemState(t, fx, staffingID); status != "assigned" {
		t.Fatalf("full shift status = %q", status)
	}

	// Just before the deadline: released once, and the place is free again.
	fx.app.now = func() time.Time { return deadline.Add(-time.Microsecond) }
	released := postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "release"), nil, http.StatusOK)
	if err := json.Unmarshal([]byte(released.Body), &shift); err != nil {
		t.Fatal(err)
	}
	if shift.Claimed || shift.RemainingPlaces != 1 || activeClaimCount(t, fx, staffingID) != 0 {
		t.Fatalf("released shift = %#v", shift)
	}
	if status, _ := staffingItemState(t, fx, staffingID); status != "open" {
		t.Fatalf("released shift status = %q, want open", status)
	}
	// A retried release frees nothing further.
	retry := postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "release"), nil, http.StatusOK)
	if err := json.Unmarshal([]byte(retry.Body), &shift); err != nil {
		t.Fatal(err)
	}
	if shift.RemainingPlaces != 1 || activeClaimCount(t, fx, staffingID) != 0 {
		t.Fatalf("retried release = %#v", shift)
	}

	// At and after the deadline: refused with the organizer contact path.
	postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "claim"), nil, http.StatusOK)
	for _, now := range []time.Time{deadline, deadline.Add(time.Second)} {
		fx.app.now = func() time.Time { return now }
		listed, ok := openShift(t, fx, participant, staffingID)
		if !ok || !listed.Claimed || listed.CanRelease {
			t.Fatalf("listing at %s = %#v, %v; want claimed without release", now, listed, ok)
		}
		refused := postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "release"), nil, http.StatusConflict)
		if !strings.Contains(refused.Body, "ask the organizer") {
			t.Fatalf("cutoff refusal = %s", refused.Body)
		}
	}
	if activeClaimCount(t, fx, staffingID) != 1 {
		t.Fatal("refused release freed the place")
	}
	postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "release-request"), nil, http.StatusOK)
	postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "release-request"), nil, http.StatusOK)
	var requests int
	var subject, body string
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*), min(n.subject), min(o.body)
		from notification_events n
		join email_outbox o on o.id = n.email_outbox_id
		where n.notification_type = 'staffing.release_request' and n.recipient_email = $1
	`, fx.email("owner")).Scan(&requests, &subject, &body); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || !strings.Contains(body, participant.name) || !strings.Contains(subject, "Merch table") {
		t.Fatalf("release requests=%d subject=%q body=%q, want one owner request", requests, subject, body)
	}

	// The organizer override ignores the cutoff and frees the place once.
	item := getStaffingItem(t, fx, staffingID)
	claims := item["claims"].([]any)
	if len(claims) != 1 {
		t.Fatalf("organizer claims = %#v", claims)
	}
	claimID := mustString(t, claims[0], "id")
	removed := doJSON(t, http.MethodDelete, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing/"+staffingID+"/claims/"+claimID, nil, http.StatusOK)
	if obj := mustObject(t, removed.JSON); obj["filledCount"] != float64(0) || obj["status"] != "open" {
		t.Fatalf("removed claim item = %s", removed.Body)
	}
	doJSON(t, http.MethodDelete, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing/"+staffingID+"/claims/"+claimID, nil, http.StatusOK)
	if activeClaimCount(t, fx, staffingID) != 0 {
		t.Fatal("organizer removal did not free the place")
	}
}

func TestStaffingOrganizerOverrideUsesCapacityRules(t *testing.T) {
	fx := newClaimFixture(t)
	memberPersonID, _ := memberIdentity(t, fx.lifecycleFixture)
	ownerID := ownerPersonID(t, fx.lifecycleFixture)
	extra := addClaimParticipant(t, fx, "extra", fx.roleID, "")
	starts := claimClock.Add(30 * time.Hour)
	created := postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing", map[string]any{
		"title": "Load-in", "kind": "shift", "startsAt": starts.Format(time.RFC3339), "endsAt": starts.Add(time.Hour).Format(time.RFC3339),
	}, http.StatusOK)
	staffingID := mustString(t, created.JSON, "id")
	if obj := mustObject(t, created.JSON); obj["capacity"] != float64(1) || obj["claimsOpen"] != false || obj["releaseCutoffHours"] != float64(24) {
		t.Fatalf("new shift defaults = %s", created.Body)
	}
	staffingPath := "/api/events/" + fx.eventID + "/staffing/" + staffingID

	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"capacity": 0}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"capacity": 101}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"releaseCutoffHours": 721}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"claimsOpen": true}, http.StatusBadRequest)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"claimRoleIds": []string{"00000000-0000-0000-0000-000000000000"}}, http.StatusBadRequest)

	// Capacity 1 with the legacy assignee is full for every other path.
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"assignedPersonId": memberPersonID}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, staffingPath+"/claims", map[string]any{"personId": extra.personID}, http.StatusConflict)
	// Retrying the legacy occupant is a no-op success.
	postJSON(t, fx.app, fx.ownerCookie, staffingPath+"/claims", map[string]any{"personId": memberPersonID}, http.StatusOK)

	widened := patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"capacity": 3}, http.StatusOK)
	if obj := mustObject(t, widened.JSON); obj["status"] != "open" || obj["filledCount"] != float64(1) {
		t.Fatalf("widened shift = %s", widened.Body)
	}
	postJSON(t, fx.app, fx.ownerCookie, staffingPath+"/claims", map[string]any{"personId": extra.personID}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, staffingPath+"/claims", map[string]any{"personId": extra.personID}, http.StatusOK)
	added := postJSON(t, fx.app, fx.ownerCookie, staffingPath+"/claims", map[string]any{"personId": ownerID}, http.StatusOK)
	if obj := mustObject(t, added.JSON); obj["status"] != "assigned" || obj["filledCount"] != float64(3) || len(obj["claims"].([]any)) != 2 {
		t.Fatalf("filled shift = %s", added.Body)
	}
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"capacity": 2}, http.StatusConflict)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"assignedPersonId": extra.personID}, http.StatusConflict)

	// Members cannot use the organizer override.
	postJSON(t, fx.app, fx.memberCookie, staffingPath+"/claims", map[string]any{"personId": memberPersonID}, http.StatusForbidden)
	// A non-member cannot be given a place.
	outsider := newLifecycleFixture(t, fx.app)
	outsiderID := ownerPersonID(t, outsider)
	patchJSON(t, fx.app, fx.ownerCookie, staffingPath, map[string]any{"clearAssignee": true}, http.StatusOK)
	postJSON(t, fx.app, fx.ownerCookie, staffingPath+"/claims", map[string]any{"personId": outsiderID}, http.StatusBadRequest)

	var assignments int
	if err := fx.app.db.QueryRow(t.Context(), `
		select count(*) from notification_events where notification_type = 'staffing.assignment' and related_id = $1
	`, staffingID).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if assignments != 3 {
		t.Fatalf("assignment notifications = %d, want one per assigned person", assignments)
	}
}

func TestStaffingShiftClaimPortalShowsOnlyPermittedDetails(t *testing.T) {
	fx := newClaimFixture(t)
	holder := addClaimParticipant(t, fx, "holder", fx.roleID, "accepted")
	viewer := addClaimParticipant(t, fx, "viewer", fx.roleID, "accepted")
	starts := claimClock.Add(50 * time.Hour)
	staffingID := createClaimShift(t, fx, "Sound desk", starts, starts.Add(4*time.Hour), 3, 2)
	postJSON(t, fx.app, holder.cookie, claimPath(staffingID, "claim"), nil, http.StatusOK)

	portal := getParticipantPortal(t, fx.app, holder.cookie, http.StatusOK)
	if len(portal.Assignments) != 1 || portal.Assignments[0].StaffingItemID != staffingID || portal.Assignments[0].Status != "assigned" || portal.Assignments[0].ParticipantRequirements != "Wear closed shoes." {
		t.Fatalf("holder portal assignments = %#v", portal.Assignments)
	}

	response := getJSON(t, fx.app, viewer.cookie, "/api/me/open-shifts", http.StatusOK)
	if got := response.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("cache control = %q", got)
	}
	listed, ok := openShift(t, fx, viewer, staffingID)
	if !ok || listed.RemainingPlaces != 2 || listed.Claimed {
		t.Fatalf("viewer listing = %#v", listed)
	}
	for _, private := range []string{holder.name, holder.email, holder.personID, "operator-only note", "private application message", "amountCents", "applicantEmail", "notes"} {
		if strings.Contains(response.Body, private) {
			t.Fatalf("open shifts exposed %q: %s", private, response.Body)
		}
	}
	viewerPortal := getParticipantPortal(t, fx.app, viewer.cookie, http.StatusOK)
	if len(viewerPortal.Assignments) != 0 {
		t.Fatalf("viewer sees another person's place: %#v", viewerPortal.Assignments)
	}
}

func getStaffingItem(t *testing.T, fx claimFixture, staffingID string) map[string]any {
	t.Helper()
	listed := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+fx.eventID+"/staffing", http.StatusOK)
	for _, raw := range listed.JSON.([]any) {
		if item := mustObject(t, raw); item["id"] == staffingID {
			return item
		}
	}
	t.Fatalf("staffing item %s not listed", staffingID)
	return nil
}

func TestStaffingShiftClaimsMigrationKeepsSingleAssigneeMeaning(t *testing.T) {
	ctx := t.Context()
	pool := newMigrationTestPool(t)
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 31 {
		t.Fatalf("migrations=%d, want version 31", len(migrations))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `create table schema_migrations(version integer primary key,name text not null,checksum char(64) not null,applied_at timestamptz not null default now())`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:30] {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			t.Fatalf("apply historical migration %d: %v", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version,name,checksum) values($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
			t.Fatal(err)
		}
	}
	var personID, workspaceID, eventID, assignedID, openID string
	if err := tx.QueryRow(ctx, `insert into people (email, display_name, password_hash) values ('legacy@example.test', 'Legacy', 'x') returning id`).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `insert into workspaces (name) values ('Legacy') returning id`).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		insert into events (workspace_id, title, starts_at, public_description, location_display, ticket_allocation, created_by_person_id)
		values ($1, 'Legacy event', '2026-07-01T20:00:00Z', 'Legacy', 'Hall', 10, $2) returning id
	`, workspaceID, personID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		insert into event_staffing_items (event_id, title, kind, starts_at, ends_at, assigned_person_id, status, created_by_person_id)
		values ($1, 'Assigned door', 'shift', '2026-07-01T19:00:00Z', '2026-07-01T21:00:00Z', $2, 'assigned', $2) returning id
	`, eventID, personID).Scan(&assignedID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		insert into event_staffing_items (event_id, title, kind, status, created_by_person_id)
		values ($1, 'Open task', 'task', 'open', $2) returning id
	`, eventID, personID).Scan(&openID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("upgrade version-30 fixture: %v", err)
	}

	for _, want := range []struct {
		id     string
		status string
		filled int
	}{{assignedID, "assigned", 1}, {openID, "open", 0}} {
		var capacity, cutoff int
		var claimsOpen bool
		var status string
		var assignee *string
		if err := pool.QueryRow(ctx, `
			select capacity, claims_open, release_cutoff_hours, status, assigned_person_id::text
			from event_staffing_items where id = $1
		`, want.id).Scan(&capacity, &claimsOpen, &cutoff, &status, &assignee); err != nil {
			t.Fatal(err)
		}
		if capacity != 1 || claimsOpen || cutoff != 24 || status != want.status {
			t.Fatalf("migrated item %s capacity=%d claimsOpen=%v cutoff=%d status=%q", want.id, capacity, claimsOpen, cutoff, status)
		}
		if want.filled == 1 && (assignee == nil || *assignee != personID) {
			t.Fatalf("migrated assignee = %v, want %s", assignee, personID)
		}
		lockTx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		place, err := lockStaffingPlace(ctx, lockTx, eventID, want.id)
		_ = lockTx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if place.Filled != want.filled || place.remaining() != 1-want.filled {
			t.Fatalf("migrated place %s filled=%d remaining=%d, want filled %d", want.id, place.Filled, place.remaining(), want.filled)
		}
	}
	var claims int
	if err := pool.QueryRow(ctx, `select count(*) from event_staffing_claims`).Scan(&claims); err != nil || claims != 0 {
		t.Fatalf("migration created claims=%d err=%v", claims, err)
	}
	if _, err := pool.Exec(ctx, `update event_staffing_items set claims_open = true where id = $1`, openID); err == nil {
		t.Fatal("an untimed task was opened to claims")
	}
}

func TestStaffingReleaseDeadlineBoundary(t *testing.T) {
	starts := time.Date(2026, 7, 1, 18, 0, 0, 0, time.UTC)
	deadline := staffingReleaseDeadline(starts, 3)
	if !deadline.Equal(starts.Add(-3 * time.Hour)) {
		t.Fatalf("deadline = %s", deadline)
	}
	if !staffingReleaseAllowed(deadline.Add(-time.Nanosecond), starts, 3) || staffingReleaseAllowed(deadline, starts, 3) || staffingReleaseAllowed(deadline.Add(time.Second), starts, 3) {
		t.Fatal("release must be allowed strictly before the deadline only")
	}
	if !staffingReleaseAllowed(starts.Add(-time.Nanosecond), starts, 0) || staffingReleaseAllowed(starts, starts, 0) {
		t.Fatal("a zero cutoff allows release until the shift starts")
	}
}

func TestStaffingShiftClaimantsReceiveLifecycleNotices(t *testing.T) {
	fx := newClaimFixture(t)
	participant := addClaimParticipant(t, fx, "notice", fx.roleID, "accepted")
	starts := claimClock.Add(40 * time.Hour)
	staffingID := createClaimShift(t, fx, "Runner", starts, starts.Add(time.Hour), 4, 1)
	recipients := func() map[string]string {
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
	if _, ok := recipients()[participant.email]; ok {
		t.Fatal("participant without a place received a crew notice")
	}
	postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "claim"), nil, http.StatusOK)
	if got := recipients()[participant.email]; got != "crew_person:"+participant.personID {
		t.Fatalf("claimant recipient = %q", got)
	}
	postJSON(t, fx.app, participant.cookie, claimPath(staffingID, "release"), nil, http.StatusOK)
	if _, ok := recipients()[participant.email]; ok {
		t.Fatal("released participant still receives crew notices")
	}
}
