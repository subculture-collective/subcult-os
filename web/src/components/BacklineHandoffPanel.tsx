import type { BacklinePreviewDTO, ChannelHandoffDTO } from '../domain';
import { blockerMessage, canRetryHandoff, handoffStateLabel, handoffTone, mentionPolicyLabel } from '../modules/announcements/backlineHandoff';
import { Button } from '../ui/Button';
import { Notice } from '../ui/Notice';

function when(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

// BacklineHandoffPanel shows the approved content, destination and mention
// policy before the organizer hands an announcement to Backline, then the
// handoff outcome ledger. It only renders; the view owns requests.
export function BacklineHandoffPanel({ preview, handoffs, busy, onHandoff, onRetry }: {
  preview: BacklinePreviewDTO;
  handoffs: ChannelHandoffDTO[];
  busy: boolean;
  onHandoff: () => void;
  onRetry: (handoff: ChannelHandoffDTO) => void;
}) {
  const ready = preview.blockers.length === 0 && preview.binding !== null;
  return <section aria-labelledby="backline-preview-title" className="space-y-4 rounded-panel border border-stroke-subtle bg-surface-panel p-6">
    <h2 id="backline-preview-title" className="text-xl font-bold">Backline channel preview</h2>
    <dl className="grid gap-3 text-sm sm:grid-cols-[10rem_1fr]">
      <dt className="text-fg-muted">Destination</dt>
      <dd>{preview.binding ? <>{preview.binding.label} <span className="text-fg-secondary">(server {preview.binding.serverId}, channel {preview.binding.channelId})</span></> : 'No connected channel'}</dd>
      <dt className="text-fg-muted">Mentions</dt>
      <dd>{mentionPolicyLabel}</dd>
      <dt className="text-fg-muted">Approval</dt>
      <dd>{preview.approved ? `Approved (${preview.announcementStatus})` : `Not approved (${preview.announcementStatus})`}</dd>
      <dt className="text-fg-muted">Posts at</dt>
      <dd>{when(preview.deliverAt)}; Backline must not post after {when(preview.expiresAt)}</dd>
      <dt className="text-fg-muted">Length</dt>
      <dd>{preview.contentLength} of {preview.contentLimit} characters</dd>
    </dl>
    <figure className="rounded-control border border-stroke-subtle bg-surface-inset p-4">
      <figcaption className="text-xs uppercase tracking-[0.2em] text-fg-muted">Content sent to the channel</figcaption>
      <p className="mt-2 font-bold">{preview.subject}</p>
      <p className="mt-2 whitespace-pre-wrap text-fg-secondary">{preview.body}</p>
    </figure>
    <p className="text-sm text-fg-secondary">Backline receives the subject, body, destination and identifiers only. No email addresses, audience or membership data leave Subcult OS.</p>
    {preview.blockers.map((blocker) => <Notice key={blocker} tone="warning">{blockerMessage(blocker)}</Notice>)}
    {ready && <Button busy={busy} onClick={onHandoff}>Hand off to {preview.binding?.label}</Button>}
    <div className="space-y-2">
      <h3 className="text-lg font-bold">Channel outcomes</h3>
      {handoffs.length === 0 ? <p className="text-sm text-fg-secondary">Nothing handed off yet.</p> : <ul className="space-y-2">
        {handoffs.map((handoff) => <li key={handoff.id} className="space-y-2 rounded-control border border-stroke-subtle p-3 text-sm">
          <p><span className="font-bold">{handoff.bindingLabel}</span> · attempt {handoff.attemptCount}</p>
          <Notice tone={handoffTone(handoff)}>{handoffStateLabel(handoff)}{handoff.lastError ? ` (${handoff.lastError})` : ''}</Notice>
          {handoff.deliveryOutcome && <p>Channel delivery receipt: {handoff.deliveryOutcome}</p>}
          {canRetryHandoff(handoff) && <>
            <p className="text-fg-secondary">Check the Backline channel before retrying. A retry reuses the same occurrence key, so Backline posts it at most once.</p>
            <Button variant="secondary" busy={busy} onClick={() => onRetry(handoff)}>Retry with the same key</Button>
          </>}
        </li>)}
      </ul>}
    </div>
  </section>;
}
