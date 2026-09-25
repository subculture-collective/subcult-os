import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { api, ApiError, postJSON } from '../api';
import type { CulturalImportActionDTO, CulturalImportPreviewDTO, EventDTO } from '../domain';

const selectableFields = ['name', 'description', 'startsAt', 'endsAt', 'timezone', 'status'] as const;
type Occurrence = { id: string; name: string; description: string; startsAt: string; endsAt?: string; timezone?: string; status: string; updatedAt: string; publicCid?: string };

function message(error: unknown) { return error instanceof ApiError ? error.message : 'The import action could not be completed.'; }

export function ImportView({ workspaceId }: { workspaceId: string }) {
  const [sourceId, setSourceId] = useState('');
  const [sourceName, setSourceName] = useState('');
  const [sourceAssertion, setSourceAssertion] = useState('');
  const [csv, setCSV] = useState('');
  const [preview, setPreview] = useState<CulturalImportPreviewDTO | null>(null);
	const [savedPreviewId, setSavedPreviewId] = useState('');
	const [actions, setActions] = useState<CulturalImportActionDTO[]>([]);
	const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
	const generation = useRef(0);
  const currentWorkspace = useRef(workspaceId);
  if (currentWorkspace.current !== workspaceId) { currentWorkspace.current = workspaceId; generation.current += 1; }
  const [events, setEvents] = useState<EventDTO[]>([]);
  const [eventId, setEventId] = useState('');
  const [candidateId, setCandidateId] = useState('');
  const [mode, setMode] = useState<'create' | 'correction'>('create');
  const [occurrenceId, setOccurrenceId] = useState('');
  const [fields, setFields] = useState<string[]>([...selectableFields]);
  const [acknowledgeErrors, setAcknowledgeErrors] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const candidate = useMemo(() => preview?.candidates.find((item) => item.id === candidateId) ?? null, [preview, candidateId]);
  const selectedEvent = useMemo(() => events.find((item) => item.id === eventId) ?? null, [events, eventId]);
	const currentOccurrence = useMemo(() => occurrences.find((item) => item.id === occurrenceId), [occurrences, occurrenceId]);

  function clearPrivateState() {
    setPreview(null); setActions([]); setEvents([]); setOccurrences([]);
    setCandidateId(''); setEventId(''); setOccurrenceId(''); setSavedPreviewId('');
    setSourceId(''); setSourceName(''); setSourceAssertion(''); setCSV('');
    setAcknowledgeErrors(false); setFields([...selectableFields]); setMode('create');
  }
  function fail(error: unknown) {
    if (error instanceof ApiError && (error.status === 401 || error.status === 403)) clearPrivateState();
    setNotice(message(error));
  }
  useEffect(() => {
    let active = true;
    clearPrivateState(); setBusy(false); setNotice(null);
    void api<EventDTO[]>(`/api/workspaces/${workspaceId}/events`)
      .then((next) => { if (active) setEvents(next); })
      .catch((error) => { if (active) fail(error); });
    return () => { active = false; };
  }, [workspaceId]);
  useEffect(() => {
    let active = true;
    setOccurrences([]); setOccurrenceId('');
    if (eventId) void api<Occurrence[]>(`/api/events/${eventId}/occurrences`)
      .then((next) => { if (active) setOccurrences(next); })
      .catch((error) => { if (active) fail(error); });
    return () => { active = false; };
  }, [workspaceId, eventId]);
  function acceptPreview(next: CulturalImportPreviewDTO) {
    setPreview(next); setActions(next.actions); setSavedPreviewId(next.id);
    setCandidateId(''); setOccurrenceId(''); setAcknowledgeErrors(false);
  }
  async function perform(action: () => Promise<() => void>) {
    if (busy) return;
    const current = generation.current;
    setBusy(true); setNotice(null);
    try { const accept = await action(); if (current === generation.current) accept(); }
    catch (error) { if (current === generation.current) fail(error); }
    finally { if (current === generation.current) setBusy(false); }
  }
  async function submitPreview(event: FormEvent) {
    event.preventDefault();
    await perform(async () => {
      const next = await postJSON<CulturalImportPreviewDTO>(`/api/workspaces/${workspaceId}/cultural-imports/preview`, { sourceId, sourceName, sourceAssertion, csv });
      return () => acceptPreview(next);
    });
  }
  async function apply(event: FormEvent) {
    event.preventDefault();
    if (!candidate || !selectedEvent || (mode === 'correction' && !currentOccurrence)) return;
    await perform(async () => {
      const action = await postJSON<CulturalImportActionDTO>(`/api/workspaces/${workspaceId}/cultural-imports/apply`, {
        candidateId, mode, eventId, occurrenceId: mode === 'correction' ? occurrenceId : '', selectedFields: fields,
        expectedUpdatedAt: mode === 'correction' ? currentOccurrence?.updatedAt : '',
        expectedPublicCid: mode === 'correction' ? (currentOccurrence?.publicCid ?? '') : undefined, acknowledgeErrors,
      });
      return () => { setActions((current) => [action, ...current]); setNotice(`Applied ${action.mode} action ${action.id}. Reload the event before another correction.`); setOccurrenceId(''); };
    });
  }
  function toggle(field: string) { setFields((current) => current.includes(field) ? current.filter((item) => item !== field) : [...current, field]); }
  async function reloadPreview() {
    if (!savedPreviewId) return;
    await perform(async () => { const next = await api<CulturalImportPreviewDTO>(`/api/workspaces/${workspaceId}/cultural-imports/${savedPreviewId}`); return () => acceptPreview(next); });
  }
  async function rollback(action: CulturalImportActionDTO) {
    await perform(async () => {
      const result = await postJSON<{ rolledBackAt: string }>(`/api/workspaces/${workspaceId}/cultural-import-actions/${action.id}/rollback`, {});
      return () => { setActions((current) => current.map((item) => item.id === action.id ? { ...item, rolledBackAt: result.rolledBackAt } : item)); setNotice('Created occurrence rolled back.'); };
    });
  }
  function incoming(field: string) {
    if (!candidate) return '';
    return field === 'name' ? candidate.title : candidate[field as 'description' | 'startsAt' | 'endsAt' | 'timezone' | 'status'] ?? '';
  }
  function prior(field: string) { return mode === 'create' ? 'New occurrence' : currentOccurrence?.[field as keyof Occurrence] ?? 'Empty'; }

  return <main className="mx-auto max-w-4xl space-y-8 p-6 text-zinc-100">
    <a className="text-sm text-zinc-400 underline" href={`/workspace?workspaceId=${workspaceId}`}>Back to workspace</a>
    <header><p className="text-xs uppercase tracking-[0.2em] text-amber-300">Cultural imports</p><h1 className="text-3xl font-semibold">Review before changing canonical records</h1><p className="mt-2 max-w-2xl text-zinc-400">A match is only a hint. Choose the existing event and, for a correction, the exact occurrence yourself.</p></header>
    {notice && <p role="status" className="rounded border border-zinc-700 p-3">{notice}</p>}
    <form className="space-y-3 rounded border border-zinc-800 p-4" onSubmit={submitPreview}>
      <h2 className="font-medium">Create private preview</h2><label>Source ID<input required value={sourceId} onChange={(e) => setSourceId(e.target.value)} className="w-full rounded bg-zinc-900 p-2"/></label><label>Source name<input required value={sourceName} onChange={(e) => setSourceName(e.target.value)} className="w-full rounded bg-zinc-900 p-2"/></label><label>Review assertion<textarea required value={sourceAssertion} onChange={(e) => setSourceAssertion(e.target.value)} className="w-full rounded bg-zinc-900 p-2"/></label><label>CSV<textarea required value={csv} onChange={(e) => setCSV(e.target.value)} className="min-h-40 w-full rounded bg-zinc-900 p-2"/></label><button disabled={busy} className="rounded bg-amber-300 px-3 py-2 text-zinc-950">Preview import</button>
    </form>
    <div className="flex flex-wrap gap-2"><label>Saved preview ID<input value={savedPreviewId} onChange={(e)=>setSavedPreviewId(e.target.value)} className="rounded bg-zinc-900 p-2"/></label><button type="button" onClick={reloadPreview} disabled={busy || !savedPreviewId}>Reload saved preview</button></div>
    {preview && <section className="space-y-4 rounded border border-zinc-800 p-4"><h2 className="font-medium">Saved preview</h2><p>Source ID: {preview.sourceId}</p><p className="break-all text-sm text-zinc-400">Digest: {preview.contentSha256}</p>{preview.errors.length > 0 && <label className="block rounded bg-amber-500/10 p-3"><input type="checkbox" checked={acknowledgeErrors} onChange={(e) => setAcknowledgeErrors(e.target.checked)} /> I reviewed these errors: {preview.errors.map((item)=>`row ${item.row}${item.field?` ${item.field}`:''}: ${item.code}`).join('; ')}.</label>}<form className="space-y-3" onSubmit={apply}><select aria-label="Candidate" disabled={busy} required value={candidateId} onChange={(e) => { setCandidateId(e.target.value); setOccurrenceId(''); }} className="w-full rounded bg-zinc-900 p-2"><option value="">Choose candidate</option>{preview.candidates.map((item) => <option key={item.id} value={item.id}>Row {item.row}: {item.title}</option>)}</select><select aria-label="Existing event" disabled={busy} required value={eventId} onChange={(e) => setEventId(e.target.value)} className="w-full rounded bg-zinc-900 p-2"><option value="">Choose existing workspace event</option>{events.map((item) => <option key={item.id} value={item.id}>{item.title}</option>)}</select><label><input type="radio" checked={mode === 'create'} onChange={() => {setMode('create');setFields([...selectableFields]);}} /> Create one occurrence in this event</label><label className="block"><input type="radio" checked={mode === 'correction'} onChange={() => setMode('correction')} /> Correct an explicitly selected occurrence</label>{mode === 'correction' && <select aria-label="Occurrence to correct" disabled={busy} required value={occurrenceId} onChange={(e) => setOccurrenceId(e.target.value)} className="w-full rounded bg-zinc-900 p-2"><option value="">Choose an occurrence in the selected event</option>{occurrences.map((item) => <option key={item.id} value={item.id}>{item.name} — {item.startsAt}</option>)}</select>}<fieldset><legend className="mb-1 text-sm text-zinc-300">Fields to apply</legend>{selectableFields.map((field) => <label key={field} className="mr-3 inline-block"><input disabled={mode==='create'} type="checkbox" checked={fields.includes(field)} onChange={() => toggle(field)} /> {field}</label>)}</fieldset>{candidate && <div className="rounded bg-zinc-900 p-3 text-sm"><p>Candidate: {candidate.title}</p><p>Event: {selectedEvent?.title ?? 'Choose an event'}</p><table className="w-full table-fixed break-words text-left"><caption>Selected changes</caption><thead><tr><th>Field</th><th>Current</th><th>Incoming</th></tr></thead><tbody>{fields.map((field) => <tr key={field}><th>{field}</th><td>{prior(field)}</td><td>{incoming(field) || "Empty"}</td></tr>)}</tbody></table><p>Place is intentionally left empty. This import does not infer a place or create an event.</p></div>}<button disabled={busy || !candidate || !selectedEvent || fields.length === 0 || (mode === 'correction' && !currentOccurrence) || actions.some((action) => action.candidateId === candidateId) || (preview.errors.length > 0 && !acknowledgeErrors)} className="rounded bg-amber-300 px-3 py-2 text-zinc-950">Apply explicit action</button></form>{actions.map((action)=><div key={action.id}><code>{action.id}</code> {action.mode} {action.mode==='create'&&!action.rolledBackAt&&<button disabled={busy} onClick={()=>rollback(action)}>Rollback created occurrence</button>}{action.rolledBackAt&&' rolled back'}</div>)}</section>}
  </main>;
}
