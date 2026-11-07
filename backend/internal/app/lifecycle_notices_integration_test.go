package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mailprovider "git.subcult.tv/PatrickFanella/subcult-os/internal/mail"
	"github.com/google/uuid"
)

func addNoticeTicket(t *testing.T, fx lifecycleFixture, eventID, email string) string {
	t.Helper()
	var id string
	if err := fx.app.db.QueryRow(t.Context(), `insert into tickets(event_id,email,code) values($1,$2,$3) returning id`, eventID, email, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func approveNotice(t *testing.T, fx lifecycleFixture, previewPath string) (map[string]any, map[string]any) {
	return approveNoticeAudiences(t, fx, previewPath, []string{"ticket_holders"})
}

func approveNoticeAudiences(t *testing.T, fx lifecycleFixture, previewPath string, audiences []string) (map[string]any, map[string]any) {
	t.Helper()
	preview := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, previewPath, map[string]any{"audiences": audiences}, 200).JSON)
	approval := map[string]any{"requestKey": uuid.NewString(), "previewHash": preview["previewHash"], "audiences": audiences}
	notice := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, strings.TrimSuffix(previewPath, "-preview"), approval, 201).JSON)
	return notice, approval
}

func TestLifecycleNoticeDeliveryRechecksCrewRelationships(t *testing.T) {
	for _, scenario := range []string{"person_unassigned", "person_expired", "application_withdrawn", "application_unassigned"} {
		t.Run(scenario, func(t *testing.T) {
			fx, eventID, _, previewPath := noticePreviewFixture(t)
			ctx := t.Context()
			ownerID := ownerPersonID(t, fx)
			var sourceID, staffingID string
			if strings.HasPrefix(scenario, "person") {
				if err := fx.app.db.QueryRow(ctx, `select id from people where email=$1`, fx.email("member")).Scan(&sourceID); err != nil {
					t.Fatal(err)
				}
				if err := fx.app.db.QueryRow(ctx, `insert into event_staffing_items(event_id,title,kind,status,assigned_person_id,created_by_person_id) values($1,'Door','task','assigned',$2,$3) returning id`, eventID, sourceID, ownerID).Scan(&staffingID); err != nil {
					t.Fatal(err)
				}
			} else {
				var roleID string
				if err := fx.app.db.QueryRow(ctx, `insert into event_roles(event_id,name,created_by_person_id) values($1,'Crew',$2) returning id`, eventID, ownerID).Scan(&roleID); err != nil {
					t.Fatal(err)
				}
				if err := fx.app.db.QueryRow(ctx, `insert into event_role_applications(event_id,role_id,applicant_name,applicant_email,status) values($1,$2,'Crew','crew@example.test','accepted') returning id`, eventID, roleID).Scan(&sourceID); err != nil {
					t.Fatal(err)
				}
				if err := fx.app.db.QueryRow(ctx, `insert into event_staffing_items(event_id,title,kind,status,assigned_application_id,created_by_person_id) values($1,'Sound','task','assigned',$2,$3) returning id`, eventID, sourceID, ownerID).Scan(&staffingID); err != nil {
					t.Fatal(err)
				}
			}
			fx.app.config.MailDeliveryEnabled = true
			approveNoticeAudiences(t, fx, previewPath, []string{"assigned_crew"})
			var err error
			switch scenario {
			case "person_expired":
				_, err = fx.app.db.Exec(ctx, `update workspace_members set expires_at=now()-interval '1 second' where person_id=$1 and workspace_id=$2`, sourceID, fx.workspaceID)
			case "application_withdrawn":
				_, err = fx.app.db.Exec(ctx, `update event_role_applications set status='withdrawn' where id=$1`, sourceID)
			default:
				_, err = fx.app.db.Exec(ctx, `update event_staffing_items set status='open',assigned_person_id=null,assigned_application_id=null where id=$1`, staffingID)
			}
			if err != nil {
				t.Fatal(err)
			}
			report, err := fx.app.processEmailDeliveries(ctx, func(context.Context, mailprovider.Message) (string, error) {
				t.Fatal("stale crew relationship sent")
				return "", nil
			}, 10)
			if err != nil || report.Withheld != 1 {
				t.Fatalf("report=%+v err=%v", report, err)
			}
		})
	}
}

func TestLifecycleNoticeApprovalReplaySuppressionAndStaleDigest(t *testing.T) {
	fx, eventID, occurrence, previewPath := noticePreviewFixture(t)
	path := strings.TrimSuffix(previewPath, "-preview")
	addNoticeTicket(t, fx, eventID, "guest@example.test")
	preview := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, previewPath, map[string]any{"audiences": []string{"ticket_holders"}}, 200).JSON)
	approval := map[string]any{"requestKey": uuid.NewString(), "previewHash": preview["previewHash"], "audiences": []string{"ticket_holders"}}
	postJSON(t, fx.app, fx.memberCookie, path, approval, 403)
	addNoticeTicket(t, fx, eventID, "blocked@example.test")
	postJSON(t, fx.app, fx.ownerCookie, path, approval, 409)
	if _, err := fx.app.db.Exec(t.Context(), `insert into email_suppressions(recipient_email,reason) values('blocked@example.test','email.bounced')`); err != nil {
		t.Fatal(err)
	}
	notice, approval := approveNotice(t, fx, previewPath)
	recipients := notice["recipients"].([]any)
	if len(recipients) != 2 || mustObject(t, recipients[0])["status"] != "suppressed" || mustObject(t, recipients[1])["status"] != "held" {
		t.Fatalf("outcomes=%#v", notice)
	}
	getJSON(t, fx.app, fx.memberCookie, path, 403)
	listed := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, path, 200).JSON)
	if listed["id"] != notice["id"] {
		t.Fatal("notice not durable")
	}
	postJSON(t, fx.app, fx.ownerCookie, path, approval, 200)
	newKey := map[string]any{"requestKey": uuid.NewString(), "previewHash": approval["previewHash"], "audiences": approval["audiences"]}
	postJSON(t, fx.app, fx.ownerCookie, path, newKey, 409)
	changed := map[string]any{"requestKey": approval["requestKey"], "previewHash": strings.Repeat("0", 64), "audiences": approval["audiences"]}
	postJSON(t, fx.app, fx.ownerCookie, path, changed, 409)
	var decisionKey string
	if err := fx.app.db.QueryRow(t.Context(), `select request_key from event_lifecycle_changes where id=$1`, notice["changeId"]).Scan(&decisionKey); err != nil {
		t.Fatal(err)
	}
	intentPayload := lifecycleIntentPayload(occurrence)
	intentPayload["decisionKey"] = decisionKey
	intentPayload["reason"] = "PRIVATE_REASON_DO_NOT_SEND"
	postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/lifecycle-intents", intentPayload, 200)
	intentPayload["decisionKey"] = uuid.NewString()
	other := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, "/api/events/"+eventID+"/lifecycle-intents", intentPayload, 201).JSON)
	otherPreviewPath := "/api/events/" + eventID + "/lifecycle-intents/" + other["id"].(string) + "/notice-preview"
	otherPreview := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, otherPreviewPath, map[string]any{"audiences": []string{"ticket_holders"}}, 200).JSON)
	postJSON(t, fx.app, fx.ownerCookie, strings.TrimSuffix(otherPreviewPath, "-preview"), map[string]any{"requestKey": uuid.NewString(), "previewHash": otherPreview["previewHash"], "audiences": []string{"ticket_holders"}}, 409)
	if _, err := fx.app.db.Exec(t.Context(), `update event_occurrences set updated_at=clock_timestamp(),public_cid='later' where id=$1`, occurrence["id"]); err != nil {
		t.Fatal(err)
	}
	replayed := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, path, approval, 200).JSON)
	if replayed["id"] != notice["id"] {
		t.Fatal("lost response replay created another notice")
	}
	var notices, queued, actions int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from lifecycle_notices),(select count(*) from email_outbox where related_type='lifecycle_notice'),(select count(*) from event_lifecycle_actions where dispatch_approved)`).Scan(&notices, &queued, &actions); err != nil {
		t.Fatal(err)
	}
	if notices != 1 || queued != 1 || actions != 1 {
		t.Fatalf("counts notice/queue/action=%d/%d/%d", notices, queued, actions)
	}
	raw, _ := json.Marshal(notice)
	for _, private := range []string{"providerReference", "provider_message_id", "leaseToken", "requestKey", "PRIVATE_REASON"} {
		if bytes.Contains(raw, []byte(private)) {
			t.Fatalf("private field=%s", private)
		}
	}
}

func TestLifecycleNoticeConcurrentApprovalHasOneBatch(t *testing.T) {
	fx, eventID, _, previewPath := noticePreviewFixture(t)
	addNoticeTicket(t, fx, eventID, "guest@example.test")
	preview := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, previewPath, map[string]any{"audiences": []string{"ticket_holders"}}, 200).JSON)
	results := make(chan int, 2)
	for range 2 {
		go func() {
			body, _ := json.Marshal(map[string]any{"requestKey": uuid.NewString(), "previewHash": preview["previewHash"], "audiences": []string{"ticket_holders"}})
			req := httptest.NewRequest(http.MethodPost, strings.TrimSuffix(previewPath, "-preview"), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(fx.ownerCookie)
			rec := httptest.NewRecorder()
			fx.app.Handler().ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}
	first, second := <-results, <-results
	if !((first == 201 && second == 409) || (first == 409 && second == 201)) {
		t.Fatalf("statuses=%d,%d", first, second)
	}
	var count int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from email_outbox where related_type='lifecycle_notice'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("queue=%d err=%v", count, err)
	}
}

func TestLifecycleNoticeApprovalRollsBackEveryQueueRow(t *testing.T) {
	fx, eventID, _, previewPath := noticePreviewFixture(t)
	addNoticeTicket(t, fx, eventID, "guest@example.test")
	preview := mustObject(t, postJSON(t, fx.app, fx.ownerCookie, previewPath, map[string]any{"audiences": []string{"ticket_holders"}}, 200).JSON)
	approval := map[string]any{"requestKey": uuid.NewString(), "previewHash": preview["previewHash"], "audiences": []string{"ticket_holders"}}
	if _, err := fx.app.db.Exec(t.Context(), `alter table audit_entries add constraint synthetic_notice_audit_failure check(action<>'lifecycle_notice.queued')`); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.ownerCookie, strings.TrimSuffix(previewPath, "-preview"), approval, 500)
	var notices, recipients, outbox, actions int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from lifecycle_notices),(select count(*) from lifecycle_notice_recipients),(select count(*) from email_outbox where related_type='lifecycle_notice'),(select count(*) from event_lifecycle_actions where dispatch_approved)`).Scan(&notices, &recipients, &outbox, &actions); err != nil {
		t.Fatal(err)
	}
	if notices+recipients+outbox+actions != 0 {
		t.Fatalf("partial queue survived rollback: %d/%d/%d/%d", notices, recipients, outbox, actions)
	}
	if _, err := fx.app.db.Exec(t.Context(), `alter table audit_entries drop constraint synthetic_notice_audit_failure`); err != nil {
		t.Fatal(err)
	}
	postJSON(t, fx.app, fx.ownerCookie, strings.TrimSuffix(previewPath, "-preview"), approval, 201)
}

func TestLifecycleNoticeMigrationPreservesLegacyMailAndDrafts(t *testing.T) {
	fx, eventID, _, _ := noticePreviewFixture(t)
	// Reconstruct v25 in this disposable schema. No notice has been approved.
	if _, err := fx.app.db.Exec(t.Context(), `alter table email_outbox drop constraint email_outbox_provider_id_length; alter table email_provider_events drop constraint email_events_provider_id_length; alter table email_outbox drop column provider; alter table email_outbox alter column provider_message_id type uuid using provider_message_id::uuid; alter table email_provider_events alter column provider_message_id type uuid using provider_message_id::uuid; drop table venue_access_place_requests;drop table event_access_revisions; drop table lifecycle_notice_reviews; drop table lifecycle_notice_recipients; drop table lifecycle_notices; alter table email_outbox drop constraint email_outbox_delivery_status_check; alter table email_outbox add constraint email_outbox_delivery_status_check check(delivery_status in ('held','pending','leased','accepted','failed','quarantined','suppressed','withheld_consent')); drop table event_staffing_claims; drop table event_staffing_claim_roles; alter table event_role_applications drop column applicant_person_id; alter table event_staffing_items drop constraint event_staffing_items_claims_open_timed_shift, drop constraint event_staffing_items_id_event_key, drop column capacity, drop column claims_open, drop column release_cutoff_hours; alter table event_roles drop constraint event_roles_id_event_key; delete from schema_migrations where version>=26`); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := fx.app.db.QueryRow(t.Context(), `select count(*) from email_outbox where delivery_status='held'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(t.Context(), fx.app.db); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(t.Context(), fx.app.db); err != nil {
		t.Fatal(err)
	}
	var held, drafts, notices int
	if err := fx.app.db.QueryRow(t.Context(), `select (select count(*) from email_outbox where delivery_status='held'),(select count(*) from event_lifecycle_actions a join event_lifecycle_changes c on c.id=a.change_id where c.event_id=$1 and not a.dispatch_approved and a.attempt_count=0),(select count(*) from lifecycle_notices)`, eventID).Scan(&held, &drafts, &notices); err != nil {
		t.Fatal(err)
	}
	if held != before || drafts != 2 || notices != 0 {
		t.Fatalf("upgrade changed prior rows: held %d->%d drafts=%d notices=%d", before, held, drafts, notices)
	}
}

func TestLifecycleNoticeDeliveryRetryAndFeedbackStaySeparate(t *testing.T) {
	fx, eventID, _, previewPath := noticePreviewFixture(t)
	addNoticeTicket(t, fx, eventID, "guest@example.test")
	fx.app.config.MailDeliveryEnabled = true
	notice, _ := approveNotice(t, fx, previewPath)
	var stableID, stableBody string
	report, err := fx.app.processEmailDeliveries(t.Context(), func(_ context.Context, message mailprovider.Message) (string, error) {
		stableID = message.ID
		stableBody = message.Text
		return "", errors.New("synthetic uncertain reply")
	}, 10)
	if err != nil || report.Retried != 1 {
		t.Fatalf("retry=%+v err=%v", report, err)
	}
	if _, err := fx.app.db.Exec(t.Context(), `update email_outbox set next_attempt_at=now() where id=$1`, stableID); err != nil {
		t.Fatal(err)
	}
	providerID := uuid.NewString()
	report, err = fx.app.processEmailDeliveries(t.Context(), func(_ context.Context, message mailprovider.Message) (string, error) {
		if message.ID != stableID || message.Text != stableBody {
			t.Fatal("retry changed identity/content")
		}
		return providerID, nil
	}, 10)
	if err != nil || report.Accepted != 1 {
		t.Fatalf("accepted=%+v err=%v", report, err)
	}
	result := mustObject(t, getJSON(t, fx.app, fx.ownerCookie, strings.TrimSuffix(previewPath, "-preview"), 200).JSON)
	recipient := mustObject(t, result["recipients"].([]any)[0])
	if recipient["status"] != "accepted" || recipient["feedback"] != "unknown" || recipient["attempts"].(float64) != 2 {
		t.Fatalf("accepted must not imply delivered: %#v", result)
	}
	if _, err := fx.app.db.Exec(t.Context(), `insert into email_provider_events(event_id,provider_message_id,event_type) values($1,$2,'email.delivered')`, uuid.NewString(), providerID); err != nil {
		t.Fatal(err)
	}
	if err := fx.app.reconcileEmailFeedback(t.Context()); err != nil {
		t.Fatal(err)
	}
	result = mustObject(t, getJSON(t, fx.app, fx.ownerCookie, strings.TrimSuffix(previewPath, "-preview"), 200).JSON)
	if result["id"] != notice["id"] || mustObject(t, result["recipients"].([]any)[0])["feedback"] != "delivered" {
		t.Fatalf("feedback=%#v", result)
	}
}

func TestLifecycleNoticeDeliveryWithholdsChangedAuthority(t *testing.T) {
	for _, scenario := range []string{"superseded", "owner_revoked", "listing_revision", "listing_cid", "ticket_cancelled", "ticket_email", "tampered_body", "retry_authority_loss", "suppressed"} {
		t.Run(scenario, func(t *testing.T) {
			fx, eventID, occurrence, previewPath := noticePreviewFixture(t)
			ticketID := addNoticeTicket(t, fx, eventID, "guest@example.test")
			fx.app.config.MailDeliveryEnabled = true
			approveNotice(t, fx, previewPath)
			var outboxID, changeID string
			if err := fx.app.db.QueryRow(t.Context(), `select e.id,n.change_id from lifecycle_notices n join lifecycle_notice_recipients nr on nr.notice_id=n.id join email_outbox e on e.id=nr.outbox_id`).Scan(&outboxID, &changeID); err != nil {
				t.Fatal(err)
			}
			if scenario == "retry_authority_loss" {
				if _, err := fx.app.processEmailDeliveries(t.Context(), func(context.Context, mailprovider.Message) (string, error) { return "", errors.New("uncertain") }, 1); err != nil {
					t.Fatal(err)
				}
				if _, err := fx.app.db.Exec(t.Context(), `update email_outbox set next_attempt_at=now() where id=$1`, outboxID); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			switch scenario {
			case "superseded", "retry_authority_loss":
				_, err = fx.app.db.Exec(t.Context(), `update event_lifecycle_changes set status='superseded' where id=$1`, changeID)
			case "owner_revoked":
				_, err = fx.app.db.Exec(t.Context(), `update workspace_members set revoked_at=now() where workspace_id=$1 and person_id=$2`, fx.workspaceID, ownerPersonID(t, fx))
			case "listing_revision":
				_, err = fx.app.db.Exec(t.Context(), `update event_occurrences set updated_at=clock_timestamp() where id=$1`, occurrence["id"])
			case "listing_cid":
				_, err = fx.app.db.Exec(t.Context(), `update event_occurrences set public_cid='new' where id=$1`, occurrence["id"])
			case "ticket_cancelled":
				_, err = fx.app.db.Exec(t.Context(), `update tickets set payment_status='cancelled' where id=$1`, ticketID)
			case "ticket_email":
				_, err = fx.app.db.Exec(t.Context(), `update tickets set email='another@example.test' where id=$1`, ticketID)
			case "tampered_body":
				_, err = fx.app.db.Exec(t.Context(), `update email_outbox set body='not approved' where id=$1`, outboxID)
			case "suppressed":
				_, err = fx.app.db.Exec(t.Context(), `insert into email_suppressions(recipient_email,reason) values('guest@example.test','email.complained')`)
			}
			if err != nil {
				t.Fatal(err)
			}
			report, err := fx.app.processEmailDeliveries(t.Context(), func(context.Context, mailprovider.Message) (string, error) {
				t.Fatal("changed authority reached provider")
				return "", nil
			}, 10)
			if err != nil {
				t.Fatal(err)
			}
			var status, body string
			if err := fx.app.db.QueryRow(t.Context(), `select delivery_status,body from email_outbox where id=$1`, outboxID).Scan(&status, &body); err != nil {
				t.Fatal(err)
			}
			want := "withheld_authority"
			if scenario == "retry_authority_loss" {
				want = "quarantined"
			}
			if scenario == "suppressed" {
				want = "suppressed"
			}
			if status != want || body != "" {
				t.Fatalf("report=%+v status=%s body retained=%v", report, status, body != "")
			}
		})
	}
}
