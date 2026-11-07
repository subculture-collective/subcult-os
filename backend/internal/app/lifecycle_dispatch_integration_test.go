package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func lifecycleDispatchFixture(t *testing.T) (lifecycleFixture, string, string, map[string]any) {
	t.Helper()
	fx := newLifecycleFixture(t)
	eventID := mustString(t, createEvent(t, fx, "Dispatch fixture", 2), "id")
	occurrence := createLifecycleIntentOccurrence(t, fx, eventID)
	payload := lifecycleIntentPayload(occurrence)
	payload["actionKinds"] = []string{"operational_notice", "provider_ticket", "public_record", "refund"}
	intent := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/lifecycle-intents", payload, http.StatusCreated).JSON)
	return fx, eventID, intent["id"].(string), occurrence
}

func createSyntheticDispatchAction(t *testing.T, fx lifecycleFixture, changeID, destination string) string {
	t.Helper()
	id, err := createLifecycleAction(t.Context(), fx.app.db, lifecycleActionInput{
		ChangeID: changeID, ActionKind: "operational_notice", Destination: destination,
		IdempotencyKey: "synthetic-dispatch:" + changeID + ":" + destination,
		Payload:        json.RawMessage(`{"synthetic":true}`), DispatchApproved: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func syntheticLifecycleAdapter(destination string, execute func(context.Context, lifecycleActionClaim) (lifecycleDispatchOutcome, error)) lifecycleAdapter {
	return lifecycleAdapter{Kind: "operational_notice", Destination: destination, MaxAttempts: 3, Validate: func(payload json.RawMessage) error {
		var document map[string]any
		if json.Unmarshal(payload, &document) != nil || len(document) != 1 || document["synthetic"] != true {
			return errors.New("unapproved synthetic payload")
		}
		return nil
	}, Execute: execute}
}

func TestLifecycleDispatchSelectsApprovedDestinationAndKeepsDraftsUnexecuted(t *testing.T) {
	fx, eventID, changeID, _ := lifecycleDispatchFixture(t)
	calls := 0
	adapter := syntheticLifecycleAdapter("synthetic-notice", func(ctx context.Context, claim lifecycleActionClaim) (lifecycleDispatchOutcome, error) {
		calls++
		if claim.IdempotencyKey != "synthetic-dispatch:"+changeID+":synthetic-notice" || claim.AttemptCount != 1 {
			t.Fatalf("unexpected claim identity: %+v", claim)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 45*time.Second {
			t.Fatal("adapter context is not bounded below the lease")
		}
		return lifecycleDispatchOutcome{Status: "succeeded", ProviderReference: "synthetic-receipt"}, nil
	})
	// An adapter with the same destination as HTTP drafts must not consume them.
	draftAdapter := adapter
	draftAdapter.Destination = "operational_notice"
	if claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, draftAdapter); err != nil || claim != nil || calls != 0 {
		t.Fatalf("draft was dispatched: claim=%+v calls=%d err=%v", claim, calls, err)
	}
	id := createSyntheticDispatchAction(t, fx, changeID, adapter.Destination)
	otherID := createSyntheticDispatchAction(t, fx, changeID, "other-synthetic-notice")
	claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter)
	if err != nil || claim == nil || claim.ID != id || calls != 1 {
		t.Fatalf("dispatch=%+v calls=%d err=%v", claim, calls, err)
	}
	if claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err != nil || claim != nil || calls != 1 {
		t.Fatalf("succeeded action replayed: %+v %v", claim, err)
	}
	var untouched int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_lifecycle_actions where change_id=$1 and status='pending' and attempt_count=0 and (not dispatch_approved or id=$2)`, changeID, otherID).Scan(&untouched); err != nil || untouched != 5 {
		t.Fatalf("unrelated actions changed: count=%d err=%v", untouched, err)
	}
	listed := getJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/lifecycle-intents", http.StatusOK).JSON.([]any)
	actions := mustObject(t, listed[0])["actions"].([]any)
	found := false
	for _, raw := range actions {
		action := mustObject(t, raw)
		for _, field := range []string{"providerReference", "payload", "idempotencyKey", "leaseToken"} {
			if _, exposed := action[field]; exposed {
				t.Fatalf("private worker field %q exposed in worklist", field)
			}
		}
		if action["id"] == id {
			found = true
			if action["dispatchApproved"] != true || action["status"] != "succeeded" || action["finishedAt"] == nil {
				t.Fatalf("outcome not observable: %#v", action)
			}
		}
	}
	var providerReference string
	if err := fx.app.db.QueryRow(t.Context(), `select provider_reference from event_lifecycle_actions where id=$1`, id).Scan(&providerReference); err != nil || providerReference != "synthetic-receipt" {
		t.Fatalf("internal receipt=%q err=%v", providerReference, err)
	}
	if !found {
		t.Fatal("dispatched action missing from owner worklist")
	}
	var queued int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from email_outbox where related_id=$1`, changeID).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("synthetic dispatch queued real email: %d %v", queued, err)
	}
}

func TestLifecycleDispatchRetryKeepsIdentityAndUnknownNeverReplays(t *testing.T) {
	fx, _, changeID, _ := lifecycleDispatchFixture(t)
	id := createSyntheticDispatchAction(t, fx, changeID, "retry-synthetic")
	calls := 0
	var firstIdentity string
	adapter := syntheticLifecycleAdapter("retry-synthetic", func(_ context.Context, claim lifecycleActionClaim) (lifecycleDispatchOutcome, error) {
		calls++
		if calls == 1 {
			firstIdentity = claim.IdempotencyKey
			retryAt := time.Now().Add(time.Hour)
			return lifecycleDispatchOutcome{Status: "retryable", FailureCategory: "provider_unavailable", RetryAt: &retryAt}, nil
		}
		if claim.IdempotencyKey != firstIdentity || claim.AttemptCount != 2 {
			t.Fatalf("retry changed identity/attempt: %+v", claim)
		}
		return lifecycleDispatchOutcome{}, errors.New("provider may have accepted; private response must not be persisted")
	})
	adapter.MaxAttempts = 2
	if _, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err != nil {
		t.Fatal(err)
	}
	intent, err := fx.app.loadLifecycleIntent(t.Context(), fx.workspaceID, changeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range intent.Actions {
		if action.ID == id && (action.NextAttemptAt == nil || action.FinishedAt != nil || action.AttemptCount != 1 || action.FailureCategory == nil || *action.FailureCategory != "provider_unavailable") {
			t.Fatalf("scheduled retry is not observable: %+v", action)
		}
	}
	if claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err != nil || claim != nil || calls != 1 {
		t.Fatalf("retry executed early: %+v %v calls=%d", claim, err, calls)
	}
	if _, err := fx.app.db.Exec(t.Context(), `update event_lifecycle_actions set next_attempt_at=now()-interval '1 second' where id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err != nil {
		t.Fatal(err)
	}
	assertLifecycleDispatchStatus(t, fx, id, "unknown", "transport")
	if claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err != nil || claim != nil || calls != 2 {
		t.Fatalf("unknown action replayed: %+v %v calls=%d", claim, err, calls)
	}
}

func TestLifecycleDispatchRechecksAuthorityRevisionCIDAndPayload(t *testing.T) {
	for _, name := range []string{"revoked", "expired", "removed", "demoted", "revision", "cid", "missing_cid", "null_cid", "legacy_snapshot", "superseded", "payload"} {
		t.Run(name, func(t *testing.T) {
			fx, _, changeID, occurrence := lifecycleDispatchFixture(t)
			id := createSyntheticDispatchAction(t, fx, changeID, "guard-synthetic")
			adapter := syntheticLifecycleAdapter("guard-synthetic", func(context.Context, lifecycleActionClaim) (lifecycleDispatchOutcome, error) {
				t.Fatal("adapter invoked after dispatch precondition failed")
				return lifecycleDispatchOutcome{}, nil
			})
			// Mutate after claim, during local payload validation, so the test
			// proves a dispatch-time check rather than creation-time validation.
			validate := adapter.Validate
			adapter.Validate = func(payload json.RawMessage) error {
				var err error
				switch name {
				case "revoked", "expired", "removed", "demoted":
					set := map[string]string{"revoked": "revoked_at=now()", "expired": "expires_at=now()-interval '1 second'", "removed": "removed_at=now()", "demoted": "role='member'"}[name]
					_, err = fx.app.db.Exec(t.Context(), `update workspace_members set `+set+` where workspace_id=$1 and person_id=$2`, fx.workspaceID, ownerPersonID(t, fx))
				case "revision":
					_, err = fx.app.db.Exec(t.Context(), `update event_occurrences set updated_at=updated_at+interval '1 second' where id=$1`, occurrence["id"])
				case "cid":
					_, err = fx.app.db.Exec(t.Context(), `update event_occurrences set public_cid='bafy-newer' where id=$1`, occurrence["id"])
				case "missing_cid":
					_, err = fx.app.db.Exec(t.Context(), `update event_lifecycle_changes set decision_snapshot=decision_snapshot-'expectedPublicCid' where id=$1`, changeID)
				case "null_cid":
					_, err = fx.app.db.Exec(t.Context(), `update event_lifecycle_changes set decision_snapshot=jsonb_set(decision_snapshot,'{expectedPublicCid}','null') where id=$1`, changeID)
				case "legacy_snapshot":
					_, err = fx.app.db.Exec(t.Context(), `update event_lifecycle_changes set decision_snapshot='{"legacy":true}' where id=$1`, changeID)
				case "superseded":
					err = supersedeLifecycleActions(t.Context(), fx.app.db, changeID)
				case "payload":
					return errors.New("invalid approved content")
				}
				if err != nil {
					t.Fatal(err)
				}
				return validate(payload)
			}
			if _, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err != nil {
				t.Fatal(err)
			}
			category := "permission"
			if name == "revision" || name == "cid" || name == "missing_cid" || name == "null_cid" || name == "legacy_snapshot" || name == "payload" {
				category = "validation"
			}
			assertLifecycleDispatchStatus(t, fx, id, "failed", category)
		})
	}
}

func TestLifecycleDispatchInvalidOutcomeAndExpiredLeaseRequireReconciliation(t *testing.T) {
	for _, name := range []string{"invalid", "accepted_retry", "expired", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			fx, _, changeID, _ := lifecycleDispatchFixture(t)
			id := createSyntheticDispatchAction(t, fx, changeID, "uncertain-synthetic")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			adapter := syntheticLifecycleAdapter("uncertain-synthetic", func(context.Context, lifecycleActionClaim) (lifecycleDispatchOutcome, error) {
				if name == "expired" {
					if _, err := fx.app.db.Exec(t.Context(), `update event_lifecycle_actions set lease_expires_at=now()-interval '1 second' where id=$1`, id); err != nil {
						t.Fatal(err)
					}
				} else if name == "cancelled" {
					cancel()
				} else if name == "accepted_retry" {
					retryAt := time.Now().Add(time.Hour)
					return lifecycleDispatchOutcome{Status: "retryable", FailureCategory: "transport", ProviderReference: "accepted-synthetic", RetryAt: &retryAt}, nil
				} else {
					return lifecycleDispatchOutcome{Status: "retryable", FailureCategory: "transport"}, nil
				}
				return lifecycleDispatchOutcome{Status: "succeeded", ProviderReference: "accepted-synthetic"}, nil
			})
			_, err := dispatchLifecycleAction(ctx, fx.app.db, adapter)
			if name == "expired" {
				if err == nil {
					t.Fatal("expired lease completed")
				}
				if claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err != nil || claim != nil {
					t.Fatalf("expired action replayed: %+v %v", claim, err)
				}
				assertLifecycleDispatchStatus(t, fx, id, "unknown", "lease_expired")
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if name == "cancelled" {
					assertLifecycleDispatchStatus(t, fx, id, "succeeded", "")
				} else {
					assertLifecycleDispatchStatus(t, fx, id, "unknown", "internal")
					if name == "accepted_retry" {
						var reference string
						if err := fx.app.db.QueryRow(t.Context(), `select provider_reference from event_lifecycle_actions where id=$1`, id).Scan(&reference); err != nil || reference != "accepted-synthetic" {
							t.Fatalf("uncertain acceptance reference lost: %q %v", reference, err)
						}
					}
				}
			}
		})
	}
}

func TestLifecycleDispatchApprovalCannotPromoteReplayedDraft(t *testing.T) {
	fx, _, changeID, _ := lifecycleDispatchFixture(t)
	in := lifecycleActionInput{ChangeID: changeID, ActionKind: "operational_notice", Destination: "isolated-draft", IdempotencyKey: "draft:" + changeID, Payload: json.RawMessage(`{"synthetic":true}`)}
	if _, err := createLifecycleAction(t.Context(), fx.app.db, in); err != nil {
		t.Fatal(err)
	}
	in.DispatchApproved = true
	if _, err := createLifecycleAction(t.Context(), fx.app.db, in); err == nil {
		t.Fatal("idempotent replay promoted a draft to dispatch-approved")
	}
}

func TestLifecycleDispatchMigrationKeepsExistingActionsDraftOnly(t *testing.T) {
	fx, _, changeID, _ := lifecycleDispatchFixture(t)
	// Reconstruct the exact v24 lifecycle table and ledger in this fixture's
	// disposable schema, retaining its four existing draft actions.
	if _, err := fx.app.db.Exec(t.Context(), `alter table email_outbox drop constraint email_outbox_provider_id_length; alter table email_provider_events drop constraint email_events_provider_id_length; alter table email_outbox drop column provider; alter table email_outbox alter column provider_message_id type uuid using provider_message_id::uuid; alter table email_provider_events alter column provider_message_id type uuid using provider_message_id::uuid; drop table venue_access_place_requests;drop table event_access_revisions; drop table lifecycle_notice_reviews; drop table lifecycle_notice_recipients; drop table lifecycle_notices; alter table event_lifecycle_actions drop column dispatch_approved; drop table event_staffing_claims; drop table event_staffing_claim_roles; alter table event_role_applications drop column applicant_person_id; alter table event_staffing_items drop constraint event_staffing_items_claims_open_timed_shift, drop column capacity, drop column claims_open, drop column release_cutoff_hours; delete from schema_migrations where version>=25`); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(t.Context(), fx.app.db); err != nil {
		t.Fatal(err)
	}
	var drafts int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from event_lifecycle_actions where change_id=$1 and not dispatch_approved and status='pending' and attempt_count=0`, changeID).Scan(&drafts); err != nil || drafts != 4 {
		t.Fatalf("upgrade changed existing drafts: count=%d err=%v", drafts, err)
	}
}

func assertLifecycleDispatchStatus(t *testing.T, fx lifecycleFixture, id, wantStatus, wantFailure string) {
	t.Helper()
	var status, category string
	var token *string
	if err := fx.app.db.QueryRow(t.Context(), `select status,coalesce(failure_category,''),lease_token::text from event_lifecycle_actions where id=$1`, id).Scan(&status, &category, &token); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || category != wantFailure || token != nil {
		t.Fatalf("status=%q failure=%q lease=%v; want %q/%q and cleared lease", status, category, token, wantStatus, wantFailure)
	}
}

func TestLifecycleDispatchStopsAfterAdapterRetryBudget(t *testing.T) {
	fx, _, changeID, _ := lifecycleDispatchFixture(t)
	id := createSyntheticDispatchAction(t, fx, changeID, "bounded-synthetic")
	calls := 0
	adapter := syntheticLifecycleAdapter("bounded-synthetic", func(context.Context, lifecycleActionClaim) (lifecycleDispatchOutcome, error) {
		calls++
		retryAt := time.Now().Add(time.Hour)
		return lifecycleDispatchOutcome{Status: "retryable", FailureCategory: "provider_unavailable", RetryAt: &retryAt}, nil
	})
	adapter.MaxAttempts = 0
	if claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err == nil || claim != nil || calls != 0 {
		t.Fatalf("unbounded adapter claimed work: %+v err=%v calls=%d", claim, err, calls)
	}
	adapter.MaxAttempts = 2
	for attempt := 1; attempt <= 2; attempt++ {
		claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter)
		if err != nil || claim == nil || claim.AttemptCount != attempt {
			t.Fatalf("attempt %d claim=%+v err=%v", attempt, claim, err)
		}
		if attempt == 1 {
			if _, err := fx.app.db.Exec(t.Context(), `update event_lifecycle_actions set next_attempt_at=now()-interval '1 second' where id=$1`, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertLifecycleDispatchStatus(t, fx, id, "failed", "provider_unavailable")
	if claim, err := dispatchLifecycleAction(t.Context(), fx.app.db, adapter); err != nil || claim != nil || calls != 2 {
		t.Fatalf("retry budget exceeded: %+v err=%v calls=%d", claim, err, calls)
	}
	var retryAt *time.Time
	if err := fx.app.db.QueryRow(t.Context(), `select next_attempt_at from event_lifecycle_actions where id=$1`, id).Scan(&retryAt); err != nil || retryAt != nil {
		t.Fatalf("exhausted action retained retry schedule: %v err=%v", retryAt, err)
	}
}
