import { useEffect, useRef, useState } from 'react';
import { api, ApiError, postJSON } from '../api';
import type { ParticipantOpenShiftDTO } from '../domain';
import { formatShiftTime, openShiftAction, remainingPlacesLabel, replaceOpenShift } from '../modules/openShifts/openShiftsModel';
import { publicCardClass, publicEyebrowClass, publicMutedTextClass, publicStatusPillClass } from '../modules/publicUi/publicUi';
import { Button } from '../ui/Button';
import { Notice } from '../ui/Notice';

export type OpenShiftNotice = { tone: 'success' | 'danger'; text: string };
type ShiftRequest = 'claim' | 'release' | 'release-request';

type OpenShiftsPanelProps = {
	shifts: ParticipantOpenShiftDTO[] | null;
	loading: boolean;
	error: string | null;
	pendingId: string | null;
	notices: Record<string, OpenShiftNotice>;
	onAction: (staffingItemId: string, request: ShiftRequest) => void;
};

/**
 * Lists claimable shifts with a remaining-place count only. It never renders
 * who else holds a place.
 */
export function OpenShiftsPanel({ shifts, loading, error, pendingId, notices, onAction }: OpenShiftsPanelProps) {
	return (
		<section className={publicCardClass} aria-labelledby="open-shifts-heading">
			<p className={publicEyebrowClass}>Open shifts</p>
			<h2 id="open-shifts-heading" className="mt-2 text-2xl font-bold text-fg-primary">Shifts you can claim</h2>
			<p className={`mt-2 ${publicMutedTextClass}`}>Shifts open to a role you were approved for. Other people's places stay private.</p>
			{loading && !shifts ? <p className={`mt-4 ${publicMutedTextClass}`}>Loading open shifts…</p> : null}
			{error ? <div className="mt-4"><Notice tone="danger">{error}</Notice></div> : null}
			{shifts && shifts.length === 0 ? <p className={`mt-4 ${publicMutedTextClass}`}>No open shifts are available for your approved roles.</p> : null}
			{shifts && shifts.length > 0 ? <ul className="mt-4 space-y-3">
				{shifts.map((shift) => {
					const action = openShiftAction(shift);
					const busy = pendingId === shift.staffingItemId;
					const blocked = pendingId !== null;
					const notice = notices[shift.staffingItemId];
					return <li key={shift.staffingItemId} className="rounded-2xl border border-stroke-subtle bg-surface-inset p-4">
						<div className="flex flex-wrap items-start justify-between gap-3">
							<div><p className="font-bold text-fg-primary">{shift.title}</p><p className={publicMutedTextClass}>{shift.eventTitle}</p></div>
							<span className={publicStatusPillClass(shift.claimed ? 'success' : shift.remainingPlaces > 0 ? 'warning' : 'neutral')}>
								{shift.claimed ? 'Your place' : remainingPlacesLabel(shift.remainingPlaces)}
							</span>
						</div>
						<p className={`mt-3 ${publicMutedTextClass}`}>{formatShiftTime(shift.startsAt)} – {formatShiftTime(shift.endsAt)}</p>
						{shift.participantRequirements ? <div className="mt-3 rounded-xl border border-status-info/20 bg-status-surface-info p-3 text-sm leading-6 text-status-info"><p className="font-bold">Requirements</p><p className="mt-1 whitespace-pre-wrap">{shift.participantRequirements}</p></div> : null}
						<p className={`mt-3 ${publicMutedTextClass}`}>
							{action === 'contact-organizer'
								? `The release window closed ${formatShiftTime(shift.releaseDeadline)}. Only the organizer can change this place now.`
								: `You can release a claimed place until ${formatShiftTime(shift.releaseDeadline)}.`}
						</p>
						{notice ? <div className="mt-3"><Notice tone={notice.tone}>{notice.text}</Notice></div> : null}
						<div className="mt-4 flex flex-wrap gap-3">
							{action === 'claim' ? <Button busy={busy} disabled={blocked} onClick={() => onAction(shift.staffingItemId, 'claim')}>{busy ? 'Claiming…' : 'Claim a place'}</Button> : null}
							{action === 'full' ? <Button variant="secondary" disabled>Shift full</Button> : null}
							{action === 'release' ? <Button variant="secondary" busy={busy} disabled={blocked} onClick={() => onAction(shift.staffingItemId, 'release')}>{busy ? 'Releasing…' : 'Release my place'}</Button> : null}
							{action === 'contact-organizer' ? <Button variant="secondary" busy={busy} disabled={blocked} onClick={() => onAction(shift.staffingItemId, 'release-request')}>{busy ? 'Sending…' : 'Ask the organizer to release me'}</Button> : null}
						</div>
					</li>;
				})}
			</ul> : null}
		</section>
	);
}

const successText: Record<ShiftRequest, string> = {
	claim: 'Place claimed. It now appears in your assignments.',
	release: 'Place released.',
	'release-request': 'Release request sent to the organizer. Your place stays held until they change it.',
};

/** Loads the signed-in person's open shifts and performs claim and release requests. */
export function OpenShiftsSection({ refreshKey, onChanged }: { refreshKey: number; onChanged: () => void }) {
	const [shifts, setShifts] = useState<ParticipantOpenShiftDTO[] | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);
	const [pendingId, setPendingId] = useState<string | null>(null);
	const [notices, setNotices] = useState<Record<string, OpenShiftNotice>>({});
	const [reloadTick, setReloadTick] = useState(0);
	// Guards against a second request before the disabled buttons render.
	const pendingRef = useRef<string | null>(null);

	useEffect(() => {
		let cancelled = false;
		async function load() {
			setLoading(true);
			setError(null);
			try {
				const loaded = await api<ParticipantOpenShiftDTO[]>('/api/me/open-shifts');
				if (!cancelled) setShifts(loaded);
			} catch (caught) {
				if (!cancelled) {
					setShifts(null);
					setError(caught instanceof Error ? caught.message : 'Unable to load open shifts');
				}
			} finally {
				if (!cancelled) setLoading(false);
			}
		}
		void load();
		return () => { cancelled = true; };
	}, [refreshKey, reloadTick]);

	async function handleAction(staffingItemId: string, request: ShiftRequest) {
		if (pendingRef.current) return;
		pendingRef.current = staffingItemId;
		setPendingId(staffingItemId);
		try {
			const path = `/api/me/open-shifts/${encodeURIComponent(staffingItemId)}/${request}`;
			if (request === 'release-request') {
				await postJSON<{ requested: boolean }>(path, {});
			} else {
				const updated = await postJSON<ParticipantOpenShiftDTO>(path, {});
				setShifts((current) => replaceOpenShift(current, updated));
				onChanged();
			}
			setNotices((current) => ({ ...current, [staffingItemId]: { tone: 'success', text: successText[request] } }));
		} catch (caught) {
			const message = caught instanceof Error ? caught.message : 'Request failed';
			if (caught instanceof ApiError && (caught.status === 401 || caught.status === 403) && request !== 'claim') {
				setShifts(null);
				setError(message);
			} else {
				setNotices((current) => ({ ...current, [staffingItemId]: { tone: 'danger', text: message } }));
				// Reload so the shift shows its current places and release window.
				setReloadTick((value) => value + 1);
			}
		} finally {
			pendingRef.current = null;
			setPendingId(null);
		}
	}

	return <OpenShiftsPanel shifts={shifts} loading={loading} error={error} pendingId={pendingId} notices={notices} onAction={(id, request) => void handleAction(id, request)} />;
}
