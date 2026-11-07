import { useEffect, useState } from 'react';
import { api } from '../api';
import type { ParticipantPortalDTO } from '../domain';
import { OpenShiftsSection } from '../components/OpenShiftsPanel';
import {
  publicCardClass,
  publicEyebrowClass,
  publicMutedTextClass,
  publicPageInnerClass,
  publicPageShellClass,
  publicSecondaryButtonClass,
  publicStatusPillClass,
} from '../modules/publicUi/publicUi';

function formatDateTime(value: string | null | undefined) {
  if (!value) return 'Time to be confirmed';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function statusTone(status: string) {
  if (status === 'completed' || status === 'done') return 'success';
  if (status === 'cancelled') return 'danger';
  return 'warning';
}

export function ParticipantPortalView() {
  const [portal, setPortal] = useState<ParticipantPortalDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshTick, setRefreshTick] = useState(0);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError(null);
		setPortal(null);
      try {
        const loaded = await api<ParticipantPortalDTO>('/api/me/participant-portal');
        if (!cancelled) setPortal(loaded);
      } catch (caught) {
        if (!cancelled) {
			setPortal(null);
			setError(caught instanceof Error ? caught.message : 'Unable to load your assignments');
		}
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => { cancelled = true; };
  }, [refreshTick]);

  return (
    <main className={publicPageShellClass}>
      <section className={`${publicPageInnerClass} max-w-4xl`}>
        <header className="flex flex-wrap items-start justify-between gap-4 rounded-hero border border-stroke-subtle bg-surface-panel p-6">
          <div>
            <p className={publicEyebrowClass}>Participant portal</p>
            <h1 className="mt-2 text-3xl font-bold text-fg-primary sm:text-4xl">Your event work</h1>
            <p className={`mt-2 max-w-2xl ${publicMutedTextClass}`}>This page shows assignments and commitments linked to your signed-in account.</p>
          </div>
          <a className={publicSecondaryButtonClass} href="/">Workspace</a>
        </header>

        <div className="mt-5 flex flex-wrap gap-3">
          <button className={publicSecondaryButtonClass} type="button" disabled={loading} onClick={() => setRefreshTick((value) => value + 1)}>
            {loading ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>

        {loading ? <p className={`mt-5 ${publicMutedTextClass}`}>Loading your assignments…</p> : null}
        {error ? <div role="alert" className="mt-5 border border-status-danger/20 bg-status-surface-danger px-4 py-3 text-sm font-bold text-status-danger">{error}</div> : null}

        {portal ? <div className="mt-5 grid gap-5 lg:grid-cols-2">
          <section className={publicCardClass}>
            <p className={publicEyebrowClass}>Schedule</p>
            <h2 className="mt-2 text-2xl font-bold text-fg-primary">Assignments</h2>
            {portal.assignments.length === 0 ? <p className={`mt-4 ${publicMutedTextClass}`}>No assignments are available.</p> : <ul className="mt-4 space-y-3">
              {portal.assignments.map((assignment) => <li key={assignment.staffingItemId} className="rounded-2xl border border-stroke-subtle bg-surface-inset p-4">
                <div className="flex items-start justify-between gap-3"><div><p className="font-bold text-fg-primary">{assignment.title}</p><p className={publicMutedTextClass}>{assignment.eventTitle} · {assignment.kind}</p></div><span className={publicStatusPillClass(statusTone(assignment.status))}>{assignment.status}</span></div>
                <p className={`mt-3 ${publicMutedTextClass}`}>{formatDateTime(assignment.startsAt)}{assignment.endsAt ? ` – ${formatDateTime(assignment.endsAt)}` : ''}</p>
				{assignment.participantRequirements ? <div className="mt-3 rounded-xl border border-status-info/20 bg-status-surface-info p-3 text-sm leading-6 text-status-info"><p className="font-bold">Requirements</p><p className="mt-1 whitespace-pre-wrap">{assignment.participantRequirements}</p></div> : null}
              </li>)}
            </ul>}
          </section>
          <section className={publicCardClass}>
            <p className={publicEyebrowClass}>Commitments</p>
            <h2 className="mt-2 text-2xl font-bold text-fg-primary">Commitments</h2>
            {portal.commitments.length === 0 ? <p className={`mt-4 ${publicMutedTextClass}`}>No commitments are available.</p> : <ul className="mt-4 space-y-3">
              {portal.commitments.map((commitment) => <li key={commitment.id} className="rounded-2xl border border-stroke-subtle bg-surface-inset p-4">
                <div className="flex items-start justify-between gap-3"><div><p className="font-bold text-fg-primary">{commitment.title}</p><p className={publicMutedTextClass}>{commitment.eventTitle}</p></div><span className={publicStatusPillClass(statusTone(commitment.status))}>{commitment.status}</span></div>
                <p className={`mt-3 ${publicMutedTextClass}`}>Due {formatDateTime(commitment.dueAt)}</p>
              </li>)}
            </ul>}
          </section>
        </div> : null}

        {error ? null : <div className="mt-5"><OpenShiftsSection refreshKey={refreshTick} onChanged={() => setRefreshTick((value) => value + 1)} /></div>}
      </section>
    </main>
  );
}
