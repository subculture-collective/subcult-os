import type { ParticipantOpenShiftDTO } from '../../domain';

export type OpenShiftAction = 'claim' | 'full' | 'release' | 'contact-organizer';

/** The one action a participant can take on a shift right now. */
export function openShiftAction(shift: ParticipantOpenShiftDTO): OpenShiftAction {
	if (shift.claimed) return shift.canRelease ? 'release' : 'contact-organizer';
	return shift.remainingPlaces > 0 ? 'claim' : 'full';
}

export function remainingPlacesLabel(count: number) {
	if (count <= 0) return 'No places left';
	return count === 1 ? '1 place left' : `${count} places left`;
}

export function replaceOpenShift(shifts: ParticipantOpenShiftDTO[] | null, updated: ParticipantOpenShiftDTO) {
	if (!shifts) return shifts;
	return shifts.map((shift) => (shift.staffingItemId === updated.staffingItemId ? updated : shift));
}

export function formatShiftTime(value: string | null | undefined) {
	if (!value) return 'Time to be confirmed';
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}
