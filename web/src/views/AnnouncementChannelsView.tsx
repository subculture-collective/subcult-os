import { FormEvent, useEffect, useRef, useState } from 'react';
import { ApiError, api, postJSON } from '../api';
import { BacklineHandoffPanel } from '../components/BacklineHandoffPanel';
import type { AnnouncementDTO, BacklinePreviewDTO, ChannelBindingDTO, ChannelHandoffDTO } from '../domain';
import { Button } from '../ui/Button';
import { Notice } from '../ui/Notice';

function message(error: unknown) {
  return error instanceof ApiError ? error.message : 'The request could not be completed. Refresh before trying again.';
}

// AnnouncementChannelsView lets an owner or organizer connect one Backline
// channel and hand an approved announcement to it (issue #221).
export function AnnouncementChannelsView({ workspaceId }: { workspaceId: string }) {
  const generation = useRef(0);
  const [announcements, setAnnouncements] = useState<AnnouncementDTO[]>([]);
  const [bindings, setBindings] = useState<ChannelBindingDTO[]>([]);
  const [selectedId, setSelectedId] = useState('');
  const [preview, setPreview] = useState<BacklinePreviewDTO | null>(null);
  const [handoffs, setHandoffs] = useState<ChannelHandoffDTO[]>([]);
  const [label, setLabel] = useState('');
  const [serverId, setServerId] = useState('');
  const [channelId, setChannelId] = useState('');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const base = `/api/workspaces/${workspaceId}`;
  const active = bindings.find((binding) => binding.active) ?? null;

  function fail(error: unknown) {
    if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
      generation.current += 1;
      setAnnouncements([]); setBindings([]); setPreview(null); setHandoffs([]);
    }
    setNotice(message(error));
  }

  async function loadWorkspace() {
    const current = ++generation.current;
    try {
      const [nextAnnouncements, nextBindings] = await Promise.all([
        api<AnnouncementDTO[]>(`${base}/announcements`),
        api<ChannelBindingDTO[]>(`${base}/backline-bindings`),
      ]);
      if (current !== generation.current) return;
      setAnnouncements(nextAnnouncements); setBindings(nextBindings);
    } catch (error) { if (current === generation.current) fail(error); }
  }

  async function loadSelected(id: string) {
    const current = generation.current;
    if (!id) { setPreview(null); setHandoffs([]); return; }
    try {
      const [nextPreview, announcement] = await Promise.all([
        api<BacklinePreviewDTO>(`${base}/announcements/${id}/backline-preview`),
        api<AnnouncementDTO>(`${base}/announcements/${id}`),
      ]);
      if (current !== generation.current) return;
      setPreview(nextPreview); setHandoffs(announcement.channelHandoffs);
    } catch (error) { if (current === generation.current) fail(error); }
  }

  useEffect(() => { setNotice(null); void loadWorkspace(); return () => { generation.current += 1; }; }, [workspaceId]);
  useEffect(() => { void loadSelected(selectedId); }, [selectedId]);

  async function run(action: () => Promise<unknown>, done: string) {
    if (busy) return false;
    setBusy(true); setNotice(null);
    try {
      await action();
      setNotice(done);
      await loadWorkspace();
      await loadSelected(selectedId);
      return true;
    } catch (error) { fail(error); return false; }
    finally { setBusy(false); }
  }

  function connect(event: FormEvent) {
    event.preventDefault();
    void run(() => postJSON(`${base}/backline-bindings`, { label: label.trim(), serverId: serverId.trim(), channelId: channelId.trim() }), 'Backline channel connected.')
      .then((saved) => { if (saved) { setLabel(''); setServerId(''); setChannelId(''); } });
  }

  function revoke(binding: ChannelBindingDTO) {
    if (typeof window !== 'undefined' && !window.confirm(`Disconnect ${binding.label}? Queued handoffs are cancelled and delivered ones are withdrawn where Backline still can.`)) return;
    void run(() => postJSON(`${base}/backline-bindings/${binding.id}/revoke`, {}), 'Backline channel disconnected. It stays listed for audit.');
  }

  return <main className="mx-auto max-w-4xl space-y-6 px-4 py-10 text-fg-primary">
    <header className="rounded-panel border border-stroke-subtle bg-surface-panel p-6">
      <a className="text-sm text-fg-secondary underline underline-offset-4" href={`/workspace?workspaceId=${workspaceId}`}>Back to workspace</a>
      <p className="mt-5 text-xs uppercase tracking-[0.2em] text-fg-label">Announcements</p>
      <h1 className="mt-2 text-3xl font-bold">Backline channel handoff</h1>
      <p className="mt-2 max-w-2xl text-fg-secondary">Post an approved announcement to one connected Backline channel. Scheduling approves the content; the handoff only adds the channel.</p>
    </header>
    {notice && <Notice>{notice}</Notice>}

    <section aria-labelledby="backline-binding-title" className="space-y-4 rounded-panel border border-stroke-subtle bg-surface-panel p-6">
      <h2 id="backline-binding-title" className="text-xl font-bold">Connected channel</h2>
      {active ? <div className="space-y-3">
        <p>{active.label} <span className="text-fg-secondary">(server {active.serverId}, channel {active.channelId})</span></p>
        <Button variant="danger" busy={busy} onClick={() => revoke(active)}>Disconnect channel</Button>
      </div> : <form onSubmit={connect} className="space-y-3">
        <p className="text-sm text-fg-secondary">Ask the Backline server administrator for the server and channel ids they approved for Subcult OS announcements.</p>
        <label className="block" htmlFor="binding-label">Label</label>
        <input id="binding-label" required maxLength={80} value={label} disabled={busy} onChange={(event) => setLabel(event.target.value)} className="field py-3" />
        <label className="block" htmlFor="binding-server">Backline server id</label>
        <input id="binding-server" required inputMode="numeric" pattern="[0-9]{17,20}" value={serverId} disabled={busy} onChange={(event) => setServerId(event.target.value)} className="field py-3" />
        <label className="block" htmlFor="binding-channel">Channel id</label>
        <input id="binding-channel" required inputMode="numeric" pattern="[0-9]{17,20}" value={channelId} disabled={busy} onChange={(event) => setChannelId(event.target.value)} className="field py-3" />
        <Button type="submit" busy={busy}>Connect channel</Button>
      </form>}
      {bindings.some((binding) => !binding.active) && <details className="text-sm">
        <summary>Disconnected channels</summary>
        <ul className="mt-2 space-y-1 text-fg-secondary">{bindings.filter((binding) => !binding.active).map((binding) =>
          <li key={binding.id}>{binding.label} (channel {binding.channelId}), disconnected {binding.revokedAt ? new Date(binding.revokedAt).toLocaleString() : ''}</li>)}</ul>
      </details>}
    </section>

    <section aria-labelledby="announcement-pick-title" className="space-y-3 rounded-panel border border-stroke-subtle bg-surface-panel p-6">
      <h2 id="announcement-pick-title" className="text-xl font-bold">Announcement</h2>
      {announcements.length === 0 ? <p className="text-fg-secondary">No announcements in this workspace yet.</p> : <>
        <label className="block" htmlFor="announcement-select">Choose an announcement</label>
        <select id="announcement-select" className="field py-3" value={selectedId} onChange={(event) => setSelectedId(event.target.value)}>
          <option value="">Select…</option>
          {announcements.map((announcement) => <option key={announcement.id} value={announcement.id}>{announcement.subject} ({announcement.status})</option>)}
        </select>
      </>}
    </section>

    {preview && <BacklineHandoffPanel preview={preview} handoffs={handoffs} busy={busy}
      onHandoff={() => preview.binding && void run(() => postJSON(`${base}/announcements/${preview.announcementId}/backline-handoff`, { bindingId: preview.binding?.id }), 'Handed off. The worker delivers it to Backline and records the outcome here.')}
      onRetry={(handoff) => void run(() => postJSON(`${base}/announcements/${preview.announcementId}/channel-handoffs/${handoff.id}/retry`, {}), 'Retry queued with the same occurrence key.')} />}
  </main>;
}
