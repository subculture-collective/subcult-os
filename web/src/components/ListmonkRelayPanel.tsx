import { FormEvent, useState } from 'react';
import type { AnnouncementDeliveryPath, AnnouncementPreviewDTO, ListmonkBindingDTO, ListmonkBindingInput } from '../domain';
import { Button } from '../ui/Button';
import { Notice } from '../ui/Notice';

const statusLabels: Record<string, string> = {
  held: 'Held until mail delivery is enabled',
  pending: 'Queued for listmonk',
  sending: 'Sending',
  accepted: 'Accepted by listmonk',
  rejected: 'Rejected by listmonk',
  uncertain: 'Outcome uncertain; held for review',
  failed: 'Not delivered; held for review',
  withheld: 'Withheld (consent withdrawn or binding revoked)',
  suppressed: 'Suppressed (bounce, complaint, blocklist or unsubscribe)',
  expired: 'Delivery window closed',
};

export function listmonkStatusLabel(status: string) {
  return statusLabels[status] ?? status;
}

// ListmonkRelayPanel shows the workspace's one listmonk binding (or a form to
// create it), the selected announcement's eligible and withheld counts for
// the path dispatch would use, and the listmonk outcome counts. It never
// shows a recipient address; the view owns requests (#220).
export function ListmonkRelayPanel({ binding, preview, counts, deliveryPath, busy, onConnect, onRevoke, onRetry }: {
  binding: ListmonkBindingDTO | null;
  preview: AnnouncementPreviewDTO | null;
  counts: Record<string, number>;
  deliveryPath?: AnnouncementDeliveryPath;
  busy: boolean;
  onConnect: (input: ListmonkBindingInput) => void;
  onRevoke: (binding: ListmonkBindingDTO) => void;
  onRetry: () => void;
}) {
  const [label, setLabel] = useState('');
  const [listId, setListId] = useState('');
  const [templateId, setTemplateId] = useState('');
  const [sender, setSender] = useState('');
  const [disclosure, setDisclosure] = useState('');
  const [acceptKork, setAcceptKork] = useState(false);
  const [kork, setKork] = useState({ listUrl: '', sender: '', purpose: '', disclosure: '' });
  const held = (counts.uncertain ?? 0) + (counts.failed ?? 0);
  const statuses = Object.entries(counts).filter(([, count]) => count > 0);

  function submit(event: FormEvent) {
    event.preventDefault();
    onConnect({
      label: label.trim(), listId: Number(listId), templateId: Number(templateId), senderAddress: sender.trim(),
      disclosureVersion: disclosure.trim(), ...(acceptKork ? { korkPolicy: kork } : {}),
    });
  }

  return <section aria-labelledby="listmonk-relay-title" className="space-y-4 rounded-panel border border-stroke-subtle bg-surface-panel p-6">
    <h2 id="listmonk-relay-title" className="text-xl font-bold">listmonk relay</h2>
    <p className="text-sm text-fg-secondary">While a list is connected, every recipient of a dispatched announcement goes through listmonk instead of Subcult OS email, never both. Only people who confirmed under the bound disclosure version, or whose Kork receipt names this exact list, sender and purpose, are sent to.</p>
    {binding ? <div className="space-y-3">
      <dl className="grid gap-2 text-sm sm:grid-cols-[12rem_1fr]">
        <dt className="text-fg-muted">Connected list</dt><dd>{binding.label} (list {binding.listId}, template {binding.templateId})</dd>
        <dt className="text-fg-muted">Sender</dt><dd>{binding.senderAddress}</dd>
        <dt className="text-fg-muted">Required disclosure</dt><dd>{binding.disclosureVersion}</dd>
        <dt className="text-fg-muted">Kork receipts</dt><dd>{binding.korkPolicy ? `Accepted for "${binding.korkPolicy.purpose}" from ${binding.korkPolicy.sender}` : 'Not accepted'}</dd>
      </dl>
      <Button variant="danger" busy={busy} onClick={() => onRevoke(binding)}>Disconnect list</Button>
    </div> : <form onSubmit={submit} className="space-y-3">
      <label className="block" htmlFor="listmonk-label">Label</label>
      <input id="listmonk-label" required maxLength={80} value={label} disabled={busy} onChange={(event) => setLabel(event.target.value)} className="field py-3" />
      <label className="block" htmlFor="listmonk-list">listmonk list id</label>
      <input id="listmonk-list" required inputMode="numeric" pattern="[0-9]+" value={listId} disabled={busy} onChange={(event) => setListId(event.target.value)} className="field py-3" />
      <label className="block" htmlFor="listmonk-template">Transactional template id</label>
      <input id="listmonk-template" required inputMode="numeric" pattern="[0-9]+" value={templateId} disabled={busy} onChange={(event) => setTemplateId(event.target.value)} className="field py-3" />
      <label className="block" htmlFor="listmonk-sender">Sender address</label>
      <input id="listmonk-sender" required type="email" value={sender} disabled={busy} onChange={(event) => setSender(event.target.value)} className="field py-3" />
      <label className="block" htmlFor="listmonk-disclosure">Required disclosure version</label>
      <input id="listmonk-disclosure" required maxLength={80} value={disclosure} disabled={busy} onChange={(event) => setDisclosure(event.target.value)} className="field py-3" />
      <label className="flex items-center gap-2"><input type="checkbox" checked={acceptKork} disabled={busy} onChange={(event) => setAcceptKork(event.target.checked)} /> Accept Kork sign-up receipts for this list</label>
      {acceptKork && <fieldset className="space-y-3">
        <legend className="text-sm text-fg-secondary">Copy these exactly from the Kork sign-up block. A receipt that differs in any field is refused.</legend>
        {(['listUrl', 'sender', 'purpose', 'disclosure'] as const).map((field) => <div key={field} className="space-y-1">
          <label className="block" htmlFor={`kork-${field}`}>{{ listUrl: 'Kork listmonk URL', sender: 'Kork sender (page title)', purpose: 'Kork purpose (block title)', disclosure: 'Kork disclosure text' }[field]}</label>
          <input id={`kork-${field}`} required value={kork[field]} disabled={busy} onChange={(event) => setKork({ ...kork, [field]: event.target.value })} className="field py-3" />
        </div>)}
      </fieldset>}
      <Button type="submit" busy={busy}>Connect list</Button>
    </form>}
    {preview && <div className="space-y-2">
      <h3 className="text-lg font-bold">Audience for this announcement</h3>
      <dl className="grid gap-2 text-sm sm:grid-cols-[12rem_1fr]">
        <dt className="text-fg-muted">Delivery path</dt><dd>{(deliveryPath ?? preview.deliveryPath) === 'listmonk' ? 'listmonk' : 'Subcult OS email'}{deliveryPath ? ' (recorded at dispatch)' : ''}</dd>
        <dt className="text-fg-muted">Eligible now</dt><dd>{preview.recipientCount}</dd>
        <dt className="text-fg-muted">Withheld now</dt><dd>{preview.withheldCount}</dd>
      </dl>
      <p className="text-sm text-fg-secondary">Counts only. Consent and suppression are checked again for each recipient at dispatch and right before sending.</p>
      {preview.deliveryPath === 'listmonk' && !preview.listmonkConfigured && <Notice tone="warning">listmonk is not configured on this server. Dispatched messages wait until it is.</Notice>}
    </div>}
    {statuses.length > 0 && <div className="space-y-2">
      <h3 className="text-lg font-bold">listmonk outcomes</h3>
      <ul className="space-y-1 text-sm">{statuses.map(([status, count]) => <li key={status}>{listmonkStatusLabel(status)}: {count}</li>)}</ul>
      {held > 0 && binding && <>
        <p className="text-sm text-fg-secondary">Check listmonk's logs first. listmonk cannot deduplicate transactional messages, so retrying an uncertain message can send it twice.</p>
        <Button variant="secondary" busy={busy} onClick={onRetry}>Retry {held} held {held === 1 ? 'message' : 'messages'}</Button>
      </>}
    </div>}
  </section>;
}
