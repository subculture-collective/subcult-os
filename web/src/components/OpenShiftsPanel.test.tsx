import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { ParticipantOpenShiftDTO } from '../domain';
import { OpenShiftsPanel } from './OpenShiftsPanel';

const base: ParticipantOpenShiftDTO = {
	staffingItemId: 'shift-1', eventId: 'event-1', eventTitle: 'Night Market', title: 'Door rotation',
	startsAt: '2026-07-01T18:00:00Z', endsAt: '2026-07-01T20:00:00Z', participantRequirements: 'Wear closed shoes.',
	remainingPlaces: 2, claimed: false, canRelease: false, organizerManaged: false, releaseDeadline: '2026-06-30T18:00:00Z',
};

function render(shifts: ParticipantOpenShiftDTO[] | null, extra: Partial<Parameters<typeof OpenShiftsPanel>[0]> = {}) {
	return renderToStaticMarkup(<OpenShiftsPanel shifts={shifts} loading={false} error={null} pendingId={null} notices={{}} onAction={() => {}} {...extra} />);
}

describe('open shifts panel', () => {
	it('offers a claim with a remaining-place count and the release window', () => {
		const html = render([base]);
		expect(html).toContain('Door rotation');
		expect(html).toContain('2 places left');
		expect(html).toContain('Claim a place');
		expect(html).toContain('Wear closed shoes.');
		expect(html).toContain('You can release a claimed place until');
		expect(html).not.toContain('Release my place');
	});

	it('disables a full shift', () => {
		const html = render([{ ...base, remainingPlaces: 0 }]);
		expect(html).toContain('No places left');
		expect(html).toMatch(/<button[^>]*disabled=""[^>]*>Shift full<\/button>/);
		expect(html).not.toContain('Claim a place');
	});

	it('offers release for a held place before the cutoff', () => {
		const html = render([{ ...base, claimed: true, canRelease: true, remainingPlaces: 1 }]);
		expect(html).toContain('Your place');
		expect(html).toContain('Release my place');
		expect(html).not.toContain('Claim a place');
	});

	it('shows the refusal and organizer contact action after the cutoff', () => {
		const html = render([{ ...base, claimed: true, canRelease: false }], {
			notices: { 'shift-1': { tone: 'danger', text: 'the release window for this shift has closed; ask the organizer to release you' } },
		});
		expect(html).toContain('The release window closed');
		expect(html).toContain('Ask the organizer to release me');
		expect(html).toContain('role="alert"');
		expect(html).toContain('ask the organizer to release you');
		expect(html).not.toContain('Release my place');
	});

	it('shows an organizer assignment without a release or release request', () => {
		const html = render([{ ...base, claimed: true, canRelease: false, organizerManaged: true }]);
		expect(html).toContain('Your place');
		expect(html).toContain('Assigned by the organizer; contact them to change it.');
		expect(html).not.toContain('Ask the organizer to release me');
		expect(html).not.toContain('Release my place');
		expect(html).not.toContain('<button');
	});

	it('blocks every action while one request is pending', () => {
		const html = render([base, { ...base, staffingItemId: 'shift-2', title: 'Bar' }], { pendingId: 'shift-1' });
		expect(html).toContain('Claiming…');
		expect(html).toContain('aria-busy="true"');
		expect((html.match(/disabled=""/g) ?? []).length).toBe(2);
	});

	it('renders the empty and denied states without shift data', () => {
		expect(render([])).toContain('No open shifts are available for your approved roles.');
		const denied = render(null, { error: 'forbidden' });
		expect(denied).toContain('forbidden');
		expect(denied).not.toContain('Door rotation');
	});
});
