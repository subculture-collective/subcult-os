import { useRef, useState, type FormEvent } from 'react';
import { deleteJSON, patchJSON, postJSON } from '../api';
import type { EventRoleDTO, EventStaffingItemDTO } from '../domain';
import { Button } from '../ui/Button';
import { Notice } from '../ui/Notice';

type MemberOption = { value: string; label: string };

type StaffingShiftClaimsPanelProps = {
	eventId: string;
	item: EventStaffingItemDTO;
	roles: EventRoleDTO[];
	memberOptions: MemberOption[];
	onUpdated: (item: EventStaffingItemDTO) => void;
};

/**
 * Owner controls for a shift's places: capacity, release cutoff, which
 * approved roles may self-claim, and the organizer override to add or remove
 * a held place. The server applies one capacity rule to all of these.
 */
export function StaffingShiftClaimsPanel({ eventId, item, roles, memberOptions, onUpdated }: StaffingShiftClaimsPanelProps) {
	const [pending, setPending] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const pendingRef = useRef(false);
	const base = `/api/events/${encodeURIComponent(eventId)}/staffing/${encodeURIComponent(item.id)}`;
	// An API older than this web build omits the place fields.
	const claims = item.claims ?? [];
	const claimRoleIds = item.claimRoleIds ?? [];
	const capacity = item.capacity ?? 1;
	const filledCount = item.filledCount ?? 0;
	const eventRoles = roles.filter((role) => role.eventId === eventId && (role.active || claimRoleIds.includes(role.id)));
	const held = new Set(claims.map((claim) => claim.personId));
	if (item.assignedPersonId) held.add(item.assignedPersonId);
	const addable = memberOptions.filter((option) => !held.has(option.value.replace(/^member:/, '')));
	const timed = Boolean(item.startsAt && item.endsAt);

	async function run(request: () => Promise<EventStaffingItemDTO>) {
		if (pendingRef.current) return;
		pendingRef.current = true;
		setPending(true);
		try {
			const updated = await request();
			if (updated.id === item.id && updated.eventId === eventId) onUpdated(updated);
			setError(null);
		} catch (caught) {
			setError(caught instanceof Error ? caught.message : 'Unable to update shift places');
		} finally {
			pendingRef.current = false;
			setPending(false);
		}
	}

	function handlePolicy(submitEvent: FormEvent<HTMLFormElement>) {
		submitEvent.preventDefault();
		const form = new FormData(submitEvent.currentTarget);
		void run(() => patchJSON<EventStaffingItemDTO>(base, {
			capacity: Number(form.get('capacity')),
			releaseCutoffHours: Number(form.get('releaseCutoffHours')),
			claimRoleIds: form.getAll('claimRoleIds').map(String),
			claimsOpen: form.get('claimsOpen') === 'on',
		}));
	}

	function handleAdd(submitEvent: FormEvent<HTMLFormElement>) {
		submitEvent.preventDefault();
		const personId = String(new FormData(submitEvent.currentTarget).get('personId') ?? '').replace(/^member:/, '');
		if (!personId) return;
		void run(() => postJSON<EventStaffingItemDTO>(`${base}/claims`, { personId }));
	}

	return (
		<div className="space-y-3 rounded-2xl border border-stroke-subtle bg-surface-inset p-4">
			<div className="flex flex-wrap items-center justify-between gap-2">
				<p className="text-xs uppercase tracking-[0.2em] text-fg-muted">Places</p>
				<p className="text-sm font-semibold text-fg-primary">{filledCount} of {capacity} held{item.claimsOpen ? ' · open to self-claim' : ''}</p>
			</div>
			{error ? <Notice tone="danger">{error}</Notice> : null}

			<form key={`${item.id}:${item.updatedAt}`} className="space-y-3" onSubmit={handlePolicy}>
				<div className="grid grid-cols-[repeat(auto-fit,minmax(min(12rem,100%),1fr))] gap-3">
					<label className="block space-y-2 text-sm">
						<span className="text-fg-secondary">Places on this shift</span>
						<input className="field py-3" type="number" name="capacity" min={1} max={100} step={1} required defaultValue={capacity} disabled={pending} />
					</label>
					<label className="block space-y-2 text-sm">
						<span className="text-fg-secondary">Release cutoff (hours before start)</span>
						<input className="field py-3" type="number" name="releaseCutoffHours" min={0} max={720} step={1} required defaultValue={item.releaseCutoffHours ?? 24} disabled={pending} />
					</label>
				</div>
				<fieldset className="space-y-2 text-sm">
					<legend className="text-fg-secondary">Approved roles that may claim</legend>
					{eventRoles.length === 0 ? <p className="text-xs leading-5 text-fg-muted">Add a participation role first.</p> : eventRoles.map((role) => (
						<label key={role.id} className="flex items-center gap-2">
							<input type="checkbox" name="claimRoleIds" value={role.id} defaultChecked={claimRoleIds.includes(role.id)} disabled={pending} />
							<span className="text-fg-primary">{role.name}</span>
						</label>
					))}
				</fieldset>
				<label className="flex items-center gap-2 text-sm">
					<input type="checkbox" name="claimsOpen" defaultChecked={Boolean(item.claimsOpen)} disabled={pending || !timed} />
					<span className="text-fg-primary">Open to self-claim in the participant portal</span>
				</label>
				<p className="text-xs leading-5 text-fg-muted">
					{timed
						? 'Signed-in members with an accepted or confirmed application for a selected role can claim a place. They see the remaining count only, and can release before the cutoff.'
						: 'Add a start and end time before opening this shift to self-claim.'}
				</p>
				<Button variant="secondary" type="submit" busy={pending}>Save places</Button>
			</form>

			{claims.length > 0 ? <ul className="space-y-2">
				{claims.map((claim) => (
					<li key={claim.id} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-stroke-subtle bg-surface-panel px-3 py-2 text-sm">
						<span className="text-fg-primary">{claim.name} <span className="text-fg-muted">· {claim.source === 'self' ? 'claimed in portal' : 'added by organizer'}</span></span>
						<Button variant="danger" className="px-3 py-2" busy={pending} onClick={() => void run(() => deleteJSON<EventStaffingItemDTO>(`${base}/claims/${encodeURIComponent(claim.id)}`))}>
							Remove {claim.name}
						</Button>
					</li>
				))}
			</ul> : null}

			<form className="flex flex-wrap items-end gap-3" onSubmit={handleAdd}>
				<label className="block min-w-64 space-y-2 text-sm">
					<span className="text-fg-secondary">Add a member to a place</span>
					<select className="field py-3" name="personId" defaultValue="" disabled={pending || addable.length === 0}>
						<option value="">Select a member</option>
						{addable.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
					</select>
				</label>
				<Button type="submit" busy={pending} disabled={addable.length === 0 || filledCount >= capacity}>Add to shift</Button>
			</form>
		</div>
	);
}
