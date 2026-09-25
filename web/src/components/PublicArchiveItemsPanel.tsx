import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { api, postJSON } from '../api';
import type { PublicArchiveItemDTO } from '../domain';

type ArchiveDraft = {
  kind: PublicArchiveItemDTO['kind']; title: string; attributionName: string; attributionUrl: string; externalUrl: string;
  intendedUse: PublicArchiveItemDTO['intendedUse']; rightsAssertion: PublicArchiveItemDTO['rightsAssertion']; evidenceReference: string;
};
const emptyDraft = (): ArchiveDraft => ({ kind: 'link', title: '', attributionName: '', attributionUrl: '', externalUrl: '', intendedUse: 'link_only', rightsAssertion: 'permission_asserted', evidenceReference: '' });
const archivePayload = (draft: ArchiveDraft) => ({ ...draft, attributionUrl: draft.attributionUrl.trim() || null, externalUrl: draft.externalUrl.trim() || null });

export function PublicArchiveItemsPanel({ eventId }: { eventId: string }) {
  const [items, setItems] = useState<PublicArchiveItemDTO[]>([]);
  const [draft, setDraft] = useState<ArchiveDraft>(emptyDraft);
  const [correctionFor, setCorrectionFor] = useState<PublicArchiveItemDTO | null>(null);
  const [unavailableFor, setUnavailableFor] = useState<PublicArchiveItemDTO | null>(null);
  const [unavailableReason, setUnavailableReason] = useState('');
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const eventGeneration = useRef(0);

  useEffect(() => {
    let active = true;
	const generation = ++eventGeneration.current;
    setLoading(true); setError(null); setItems([]); setDraft(emptyDraft()); setCorrectionFor(null); setUnavailableFor(null); setUnavailableReason(''); setSubmitting(false);
    api<PublicArchiveItemDTO[]>(`/api/events/${eventId}/public-archive-items`)
      .then((loaded) => { if (active && eventGeneration.current === generation) setItems(loaded); })
      .catch((caught) => { if (active && eventGeneration.current === generation) { setItems([]); setError(caught instanceof Error ? caught.message : 'Unable to load archive approvals'); } })
      .finally(() => { if (active && eventGeneration.current === generation) setLoading(false); });
    return () => { active = false; };
  }, [eventId]);

  const updateDraft = <K extends keyof ArchiveDraft>(key: K, value: ArchiveDraft[K]) => setDraft((current) => ({ ...current, [key]: value }));
  const startCorrection = (item: PublicArchiveItemDTO) => {
    setCorrectionFor(item); setUnavailableFor(null);
    setDraft({ kind: item.kind, title: item.title, attributionName: item.attributionName, attributionUrl: item.attributionUrl ?? '', externalUrl: item.externalUrl ?? '', intendedUse: item.intendedUse, rightsAssertion: item.rightsAssertion, evidenceReference: item.evidenceReference });
  };
  async function submitApproval(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault(); setSubmitting(true); setError(null);
    const generation = eventGeneration.current;
    try { const item = await postJSON<PublicArchiveItemDTO>(`/api/events/${eventId}/public-archive-items`, archivePayload(draft)); if (eventGeneration.current === generation) { setItems((current) => [...current, item]); setDraft(emptyDraft()); } }
    catch (caught) { if (eventGeneration.current === generation) setError(caught instanceof Error ? caught.message : 'Unable to save archive approval'); } finally { if (eventGeneration.current === generation) setSubmitting(false); }
  }
  async function submitCorrection(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault(); if (!correctionFor) return; setSubmitting(true); setError(null);
    const generation = eventGeneration.current;
    try { const replacement = await postJSON<PublicArchiveItemDTO>(`/api/events/${eventId}/public-archive-items/${correctionFor.id}/correct`, archivePayload(draft)); if (eventGeneration.current === generation) { setItems((current) => current.map((item) => item.id === correctionFor.id ? { ...item, status: 'corrected' as const } : item).concat(replacement)); setCorrectionFor(null); setDraft(emptyDraft()); } }
    catch (caught) { if (eventGeneration.current === generation) setError(caught instanceof Error ? caught.message : 'Unable to save correction'); } finally { if (eventGeneration.current === generation) setSubmitting(false); }
  }
  async function markUnavailable() {
    if (!unavailableFor || !unavailableReason.trim()) return; setSubmitting(true); setError(null);
    const generation = eventGeneration.current;
    try { await postJSON(`/api/events/${eventId}/public-archive-items/${unavailableFor.id}/unavailable`, { reason: unavailableReason.trim() }); if (eventGeneration.current === generation) { setItems((current) => current.map((item) => item.id === unavailableFor.id ? { ...item, status: 'unavailable' as const, unavailableReason: unavailableReason.trim() } : item)); setUnavailableFor(null); setUnavailableReason(''); } }
    catch (caught) { if (eventGeneration.current === generation) setError(caught instanceof Error ? caught.message : 'Unable to mark archive item unavailable'); } finally { if (eventGeneration.current === generation) setSubmitting(false); }
  }
  const formTitle = correctionFor ? 'Correct approved item' : 'Approve an archive credit or link';
  const submitLabel = correctionFor ? 'Save correction' : 'Approve for future archive';
  return <main className="min-h-screen px-4 py-6 text-zinc-100 sm:px-6 lg:px-8"><section className="mx-auto w-full max-w-3xl space-y-6">
    <header className="rounded-[1.75rem] border border-violet-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30"><p className="text-xs uppercase tracking-[0.3em] text-violet-300">Event archive</p><h1 className="mt-2 text-3xl font-semibold text-white">Approved for a future public archive</h1><p className="mt-3 text-sm leading-6 text-zinc-300">This is a private owner ledger. Every approval remains unpublished until a separate public archive is built. A rights assertion records what the owner was told; it does not prove rights or authorize copying, uploads, rehosting, or media delivery.</p><a className="mt-4 inline-block rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-sm font-medium text-zinc-100 transition hover:bg-white/10" href={`/events/${eventId}`}>Back to event archive and editor</a></header>
    {error ? <p role="alert" className="rounded-2xl border border-rose-400/30 bg-rose-400/10 p-4 text-sm text-rose-100">{error}</p> : null}
    <form className="space-y-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/90 p-6" onSubmit={correctionFor ? submitCorrection : submitApproval}><div><p className="text-xs uppercase tracking-[0.25em] text-zinc-500">{correctionFor ? 'Replacement creates an immutable correction chain' : 'Private, unpublished approval'}</p><h2 className="mt-2 text-xl font-semibold text-white">{formTitle}</h2></div><label className="block text-sm"><span>Item type</span><select className="mt-2 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={draft.kind} onChange={(event) => updateDraft('kind', event.target.value as ArchiveDraft['kind'])}><option value="link">External link</option><option value="credit">Display credit</option></select></label><label className="block text-sm"><span>Title</span><input required maxLength={300} className="mt-2 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={draft.title} onChange={(event) => updateDraft('title', event.target.value)} /></label><label className="block text-sm"><span>Attribution name</span><input required maxLength={300} className="mt-2 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={draft.attributionName} onChange={(event) => updateDraft('attributionName', event.target.value)} /></label><div className="grid gap-4 sm:grid-cols-2"><label className="block text-sm"><span>Attribution URL (optional)</span><input type="url" maxLength={2000} placeholder="https://example.org/artist" className="mt-2 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={draft.attributionUrl} onChange={(event) => updateDraft('attributionUrl', event.target.value)} /></label><label className="block text-sm"><span>External URL (optional)</span><input type="url" maxLength={2000} placeholder="https://example.org/work" className="mt-2 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={draft.externalUrl} onChange={(event) => updateDraft('externalUrl', event.target.value)} /></label></div><div className="grid gap-4 sm:grid-cols-2"><label className="block text-sm"><span>Intended public use</span><select className="mt-2 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={draft.intendedUse} onChange={(event) => updateDraft('intendedUse', event.target.value as ArchiveDraft['intendedUse'])}><option value="link_only">Link only</option><option value="display_credit">Display credit</option></select></label><label className="block text-sm"><span>Rights assertion</span><select className="mt-2 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={draft.rightsAssertion} onChange={(event) => updateDraft('rightsAssertion', event.target.value as ArchiveDraft['rightsAssertion'])}><option value="permission_asserted">Permission asserted</option><option value="owned">Owned</option><option value="licensed">Licensed</option><option value="public_domain">Public domain</option></select></label></div><label className="block text-sm"><span>Evidence reference (optional)</span><input maxLength={500} className="mt-2 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={draft.evidenceReference} onChange={(event) => updateDraft('evidenceReference', event.target.value)} /></label><div className="flex flex-wrap gap-3"><button className="rounded-2xl bg-violet-300 px-4 py-3 font-medium text-zinc-950 disabled:opacity-60" type="submit" disabled={submitting}>{submitting ? 'Saving…' : submitLabel}</button>{correctionFor ? <button className="rounded-2xl border border-white/10 px-4 py-3 text-sm" type="button" onClick={() => { setCorrectionFor(null); setDraft(emptyDraft()); }}>Cancel correction</button> : null}</div></form>
    <section className="rounded-[1.75rem] border border-white/10 bg-zinc-950/90 p-6"><h2 className="text-xl font-semibold text-white">Approval ledger</h2>{loading ? <p className="mt-4 text-sm text-zinc-400">Loading approvals…</p> : items.length === 0 ? <p className="mt-4 text-sm text-zinc-400">No archive items are approved yet.</p> : <ul className="mt-4 space-y-3">{items.map((item) => <li key={item.id} className="rounded-2xl border border-white/10 bg-white/5 p-4"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="font-medium text-white">{item.title}</p><p className="mt-1 text-sm text-zinc-400">{item.attributionName} · {item.intendedUse === 'link_only' ? 'Link only' : 'Display credit'} · {item.status}</p>{item.unavailableReason ? <p className="mt-2 text-sm text-amber-200">Unavailable: {item.unavailableReason}</p> : null}</div>{item.status === 'approved' ? <div className="flex gap-2"><button className="rounded-xl border border-white/10 px-3 py-2 text-sm" type="button" onClick={() => startCorrection(item)}>Correct</button><button className="rounded-xl border border-amber-400/30 px-3 py-2 text-sm text-amber-100" type="button" onClick={() => { setUnavailableFor(item); setCorrectionFor(null); }}>Unavailable</button></div> : null}</div></li>)}</ul>}</section>
    {unavailableFor ? <section className="rounded-[1.75rem] border border-amber-400/30 bg-amber-400/10 p-6"><h2 className="text-xl font-semibold text-white">Mark unavailable</h2><p className="mt-2 text-sm text-zinc-300">Keep the approval record and record a short public-safe reason. This does not remove or publish anything.</p><label className="mt-4 block text-sm"><span>Reason</span><textarea required maxLength={500} className="mt-2 min-h-28 w-full rounded-xl border border-white/10 bg-zinc-900 p-3" value={unavailableReason} onChange={(event) => setUnavailableReason(event.target.value)} /></label><div className="mt-4 flex gap-3"><button className="rounded-2xl bg-amber-300 px-4 py-3 font-medium text-zinc-950 disabled:opacity-60" type="button" onClick={() => void markUnavailable()} disabled={submitting || !unavailableReason.trim()}>Mark unavailable</button><button className="rounded-2xl border border-white/10 px-4 py-3" type="button" onClick={() => { setUnavailableFor(null); setUnavailableReason(''); }}>Cancel</button></div></section> : null}
  </section></main>;
}
