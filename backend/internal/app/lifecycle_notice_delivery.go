package app

import "context"

// Recheck the exact approved content and operational relationship immediately
// before each provider call. Database errors preserve the lease for recovery.
func (a *App) lifecycleNoticeSendAllowed(ctx context.Context, outboxID, lease string) (bool, error) {
	var allowed bool
	err := a.db.QueryRow(ctx, `select exists(
 select 1 from email_outbox e
 join lifecycle_notice_recipients nr on nr.id=e.related_id and nr.outbox_id=e.id
 join lifecycle_notices n on n.id=nr.notice_id
 join event_lifecycle_changes c on c.id=n.change_id
 join event_occurrences o on o.id=n.occurrence_id and o.event_id=n.event_id and o.workspace_id=n.workspace_id
 where e.id=$1 and e.lease_token=$2 and e.delivery_status='leased' and e.lease_until>clock_timestamp()
 and e.related_type='lifecycle_notice' and e.purpose='transactional' and e.workspace_id=n.workspace_id
 and e.recipient_email=nr.recipient_email and e.subject=n.subject and e.body=n.body
 and c.status='approved' and c.workspace_id=n.workspace_id and c.event_id=n.event_id
 and c.target_revision::timestamptz=n.occurrence_updated_at
 and c.decision_snapshot->>'occurrenceId'=n.occurrence_id::text
 and c.decision_snapshot->>'expectedPublicCid'=n.public_cid
 and o.updated_at=n.occurrence_updated_at and coalesce(o.public_cid,'')=n.public_cid
 and ((c.kind='cancellation' and o.status='cancelled') or (c.kind='reschedule' and o.status='rescheduled'))
 and exists(select 1 from workspace_members wm where wm.workspace_id=n.workspace_id
   and wm.person_id=c.approved_by_person_id and wm.role='owner' and wm.removed_at is null
   and wm.revoked_at is null and (wm.expires_at is null or wm.expires_at>clock_timestamp()))
 and exists(select 1 from workspace_members wm where wm.workspace_id=n.workspace_id
   and wm.person_id=n.approved_by_person_id and wm.role='owner' and wm.removed_at is null
   and wm.revoked_at is null and (wm.expires_at is null or wm.expires_at>clock_timestamp()))
 and (
 (nr.source_type='ticket' and exists(select 1 from tickets t where t.id=nr.source_id
   and t.event_id=n.event_id and lower(btrim(t.email))=nr.recipient_email
   and t.status in ('reserved','checked_in') and t.payment_status in ('free','pending','paid')))
 or (nr.source_type='crew_person' and exists(select 1 from people p
   join workspace_members wm on wm.person_id=p.id and wm.workspace_id=n.workspace_id
   where p.id=nr.source_id and lower(btrim(p.email))=nr.recipient_email and wm.removed_at is null
   and wm.revoked_at is null and (wm.expires_at is null or wm.expires_at>clock_timestamp())
   and (exists(select 1 from event_staffing_items s where s.assigned_person_id=p.id and s.event_id=n.event_id and s.status in ('open','assigned'))
     or exists(select 1 from event_staffing_claims c join event_staffing_items s on s.id=c.staffing_item_id
       where c.person_id=p.id and c.event_id=n.event_id and c.status='active' and s.status in ('open','assigned')))))
 or (nr.source_type='crew_application' and exists(select 1 from event_role_applications ap
   join event_staffing_items s on s.assigned_application_id=ap.id and s.event_id=n.event_id and s.status in ('open','assigned')
   where ap.id=nr.source_id and ap.event_id=n.event_id and lower(btrim(ap.applicant_email))=nr.recipient_email
   and ap.status in ('accepted','confirmed')))
 ))`, outboxID, lease).Scan(&allowed)
	return allowed, err
}
