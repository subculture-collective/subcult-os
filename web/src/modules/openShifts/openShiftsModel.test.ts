import { describe, expect, it } from 'vitest';
import type { ParticipantOpenShiftDTO } from '../../domain';
import { openShiftAction, remainingPlacesLabel, replaceOpenShift } from './openShiftsModel';

const shift: ParticipantOpenShiftDTO = {
	staffingItemId: 'shift-1', eventId: 'event-1', eventTitle: 'Night Market', title: 'Door', startsAt: '2026-07-01T18:00:00Z', endsAt: '2026-07-01T20:00:00Z',
	participantRequirements: '', remainingPlaces: 2, claimed: false, canRelease: false, releaseDeadline: '2026-06-30T18:00:00Z',
};

describe('open shift actions', () => {
	it('offers a claim only while places remain', () => {
		expect(openShiftAction(shift)).toBe('claim');
		expect(openShiftAction({ ...shift, remainingPlaces: 0 })).toBe('full');
	});
	it('offers release before the cutoff and the organizer contact after it', () => {
		expect(openShiftAction({ ...shift, claimed: true, canRelease: true, remainingPlaces: 0 })).toBe('release');
		expect(openShiftAction({ ...shift, claimed: true, canRelease: false })).toBe('contact-organizer');
	});
	it('labels remaining places as a count', () => {
		expect(remainingPlacesLabel(0)).toBe('No places left');
		expect(remainingPlacesLabel(1)).toBe('1 place left');
		expect(remainingPlacesLabel(4)).toBe('4 places left');
	});
	it('replaces only the matching shift', () => {
		const other = { ...shift, staffingItemId: 'shift-2' };
		const updated = { ...shift, claimed: true, remainingPlaces: 1 };
		expect(replaceOpenShift([shift, other], updated)).toEqual([updated, other]);
		expect(replaceOpenShift(null, updated)).toBeNull();
	});
});
