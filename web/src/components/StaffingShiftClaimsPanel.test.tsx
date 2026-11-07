import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { EventRoleDTO, EventStaffingItemDTO } from '../domain';
import { StaffingShiftClaimsPanel } from './StaffingShiftClaimsPanel';

const role: EventRoleDTO = { id: 'role-a', eventId: 'event-a', name: 'Volunteer', description: '', capacity: 10, public: true, active: true, createdAt: '', updatedAt: '' };
const otherEventRole: EventRoleDTO = { ...role, id: 'role-b', eventId: 'event-b', name: 'Other event role' };
const item: EventStaffingItemDTO = {
	id: 'shift-a', eventId: 'event-a', title: 'Door', kind: 'shift', notes: '', participantRequirements: '',
	startsAt: '2026-07-01T18:00:00Z', endsAt: '2026-07-01T20:00:00Z', status: 'open', createdAt: '', updatedAt: 'v1',
	capacity: 3, claimsOpen: true, releaseCutoffHours: 12, claimRoleIds: ['role-a'], filledCount: 2,
	claims: [
		{ id: 'claim-1', personId: 'person-1', name: 'Ada', source: 'self', createdAt: '' },
		{ id: 'claim-2', personId: 'person-2', name: 'Bo', source: 'organizer', createdAt: '' },
	],
};
const members = [
	{ value: 'member:person-1', label: 'Ada' },
	{ value: 'member:person-3', label: 'Cy' },
];

function render(overrides: Partial<EventStaffingItemDTO> = {}) {
	return renderToStaticMarkup(<StaffingShiftClaimsPanel eventId="event-a" item={{ ...item, ...overrides }} roles={[role, otherEventRole]} memberOptions={members} onUpdated={() => {}} />);
}

describe('staffing shift places', () => {
	it('shows held places, policy and organizer override controls', () => {
		const html = render();
		expect(html).toContain('2 of 3 held · open to self-claim');
		expect(html).toContain('value="3"');
		expect(html).toContain('value="12"');
		expect(html).toContain('checked="" value="role-a"');
		expect(html).not.toContain('Other event role');
		expect(html).toContain('Ada');
		expect(html).toContain('claimed in portal');
		expect(html).toContain('added by organizer');
		expect(html).toContain('Remove Bo');
		// Members already holding a place are not offered again.
		expect(html).toContain('>Cy</option>');
		expect(html).not.toContain('>Ada</option>');
	});

	it('requires times before self-claim and disables adding to a full shift', () => {
		const html = render({ startsAt: null, endsAt: null, claimsOpen: false, filledCount: 3 });
		expect(html).toContain('Add a start and end time before opening this shift to self-claim.');
		expect(html).toContain('disabled="" name="claimsOpen"');
		expect(html).toMatch(/<button[^>]*type="submit"[^>]*disabled=""[^>]*>Add to shift<\/button>/);
	});
});
