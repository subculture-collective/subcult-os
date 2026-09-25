import { describe, expect, it } from 'vitest';
import type { CommitmentDTO, EventDTO, EventRoleDTO, EventStaffingItemDTO, EventTemplateDTO } from '../../domain';

import {
	buildPayload,
	buildRoleNameById,
	buildStaffingCounts,
	buildStaffingGroups,
	buildCommitmentCounts,
	emptyForm,
	formFromEvent,
	formsMatch,
	fromInputValue,
	formatDateTime,
	pricingSummary,
	reportEndOfNightCopy,
	settlementAdjustmentsEmptyCopy,
	settlementAdjustmentsLockedCopy,
	settlementLockedCopy,
	settlementStatusLabel,
	sortCommitments,
	sortTemplates,
	toInputValue,
} from './eventEditorModel';

function event(overrides: Partial<EventDTO> = {}): EventDTO {
	return {
		id: 'event-1',
		workspaceId: 'workspace-1',
		title: 'Night Market',
		startsAt: '2026-06-18T10:15:00.000Z',
		publicDescription: 'A late set.',
		locationDisplay: 'The Hall',
		imageUrl: null,
		ticketAllocation: 100,
		pricingMode: 'free',
		ticketPriceCents: 0,
		ticketCurrency: 'usd',
		reservedCount: 26,
		checkedInCount: 20,
		staffingOpenCount: 1,
		staffingAssignedCount: 0,
		staffingCompletedCount: 0,
		staffingCancelledCount: 0,
		status: 'draft',
		publicSlug: null,
		publicUrl: null,
		...overrides,
	};
}

function staffingItem(overrides: Partial<EventStaffingItemDTO>): EventStaffingItemDTO {
	return {
		id: 'item-1',
		eventId: 'event-1',
		title: 'Task',
		kind: 'task',
		notes: '',
		participantRequirements: '',
		startsAt: null,
		endsAt: null,
		assignedPersonId: null,
		assignedApplicationId: null,
		assigneeName: null,
		status: 'open',
		createdAt: '2026-06-18T09:00:00.000Z',
		updatedAt: '2026-06-18T09:00:00.000Z',
		completedAt: null,
		completedByPersonId: null,
		...overrides,
	};
}

function commitment(overrides: Partial<CommitmentDTO>): CommitmentDTO {
	return {
		id: 'commitment-1',
		workspaceId: 'workspace-1',
		eventId: 'event-1',
		contactId: null,
		title: 'Check cables',
		description: 'Bring spares',
		dueAt: null,
		status: 'open',
		ownerPersonId: null,
		createdByPersonId: 'person-1',
		completedAt: null,
		completedByPersonId: null,
		createdAt: '2026-06-18T09:00:00.000Z',
		updatedAt: '2026-06-18T09:00:00.000Z',
		...overrides,
	};
}

function template(overrides: Partial<EventTemplateDTO>): EventTemplateDTO {
	return {
		id: 'template-1',
		workspaceId: 'workspace-1',
		name: 'Default',
		title: 'Night Market',
		publicDescription: 'A late set.',
		locationDisplay: 'The Hall',
		ticketAllocation: 100,
		pricingMode: 'free',
		ticketPriceCents: 0,
		ticketCurrency: 'usd',
		privateNotes: '',
		createdAt: '2026-06-18T09:00:00.000Z',
		updatedAt: '2026-06-18T09:00:00.000Z',
		...overrides,
	};
}

describe('event editor model helpers', () => {
	it('builds form state and save payloads without drifting copy', () => {
		const source = event({ pricingMode: 'fixed', ticketPriceCents: 4250 });
		const form = formFromEvent(source);

		expect(fromInputValue(form.startsAt)).toBe(source.startsAt);
		expect(formsMatch(form, { ...form })).toBe(true);
		expect(formsMatch(form, { ...form, title: 'Changed' })).toBe(false);
		expect(buildPayload({ ...form, title: '  Updated title  ', publicDescription: '  Updated copy  ', locationDisplay: '  Upstairs ', ticketAllocation: ' 12 ', pricingMode: 'fixed', ticketPriceDollars: '12.34' })).toEqual({
			title: 'Updated title',
			startsAt: source.startsAt,
			publicDescription: 'Updated copy',
			locationDisplay: 'Upstairs',
			ticketAllocation: 12,
			pricingMode: 'fixed',
			ticketPriceCents: 1234,
			ticketCurrency: 'usd',
		});
		expect(emptyForm().pricingMode).toBe('free');
		expect(toInputValue(source.startsAt)).toBe(form.startsAt);
		expect(pricingSummary(source)).toContain('USD');
		expect(reportEndOfNightCopy()).toBe('This is the end-of-night snapshot for the event.');
	});

	it('keeps settlement and report display copy stable', () => {
		expect(settlementStatusLabel('open')).toBe('open');
		expect(settlementStatusLabel('finalized')).toBe('finalized (locked)');
		expect(settlementLockedCopy('2026-06-18T12:00:00.000Z', 'person-1')).toBe(`Finalized on ${formatDateTime('2026-06-18T12:00:00.000Z')} by person-1.`);
		expect(settlementLockedCopy(null, null)).toBe('Finalized.');
		expect(settlementAdjustmentsEmptyCopy()).toBe('No adjustments yet.');
		expect(settlementAdjustmentsLockedCopy()).toBe('Adjustments are locked after settlement finalization.');
	});

	it('derives role, staffing, and commitment summaries', () => {
		const roles: EventRoleDTO[] = [
			{ id: 'role-2', eventId: 'event-1', name: 'Runner', description: '', capacity: 1, public: true, active: true, createdAt: '2026-06-18T09:00:00.000Z', updatedAt: '2026-06-18T09:00:00.000Z' },
			{ id: 'role-1', eventId: 'event-1', name: 'Host', description: '', capacity: 1, public: true, active: true, createdAt: '2026-06-18T09:00:00.000Z', updatedAt: '2026-06-18T09:00:00.000Z' },
		];

		expect(Array.from(buildRoleNameById(roles).entries())).toEqual([
			['role-2', 'Runner'],
			['role-1', 'Host'],
		]);

		expect(buildStaffingCounts([
			staffingItem({ status: 'open' }),
			staffingItem({ id: 'item-2', status: 'assigned' }),
			staffingItem({ id: 'item-3', status: 'completed' }),
			staffingItem({ id: 'item-4', status: 'cancelled' }),
		])).toEqual({ open: 1, assigned: 1, completed: 1, cancelled: 1 });

		expect(buildStaffingGroups([
			staffingItem({ id: 'task-b', kind: 'task', title: 'B', createdAt: '2026-06-18T10:00:00.000Z' }),
			staffingItem({ id: 'shift-a', kind: 'shift', title: 'A', createdAt: '2026-06-18T08:00:00.000Z' }),
			staffingItem({ id: 'task-a', kind: 'task', title: 'A', createdAt: '2026-06-18T07:00:00.000Z' }),
		]).map((group) => ({ kind: group.kind, label: group.label, ids: group.items.map((item) => item.id) }))).toEqual([
			{ kind: 'task', label: 'Tasks', ids: ['task-a', 'task-b'] },
			{ kind: 'shift', label: 'Shifts', ids: ['shift-a'] },
		]);

		expect(buildCommitmentCounts([
			commitment({ status: 'open' }),
			commitment({ id: 'commitment-2', status: 'done' }),
			commitment({ id: 'commitment-3', status: 'cancelled' }),
		])).toEqual({ open: 1, done: 1, cancelled: 1 });
	});

	it('sorts commitments and templates in the expected order', () => {
		expect(sortCommitments([
			commitment({ id: 'done', status: 'done', dueAt: '2026-06-18T10:00:00.000Z', createdAt: '2026-06-18T08:00:00.000Z' }),
			commitment({ id: 'open-late', status: 'open', dueAt: '2026-06-18T11:00:00.000Z', createdAt: '2026-06-18T10:00:00.000Z' }),
			commitment({ id: 'open-early', status: 'open', dueAt: '2026-06-18T09:00:00.000Z', createdAt: '2026-06-18T09:00:00.000Z' }),
		]).map((item) => item.id)).toEqual(['open-early', 'open-late', 'done']);

		expect(sortTemplates([
			template({ id: 'b', name: 'Beta', createdAt: '2026-06-18T09:00:00.000Z' }),
			template({ id: 'a-new', name: 'Alpha', createdAt: '2026-06-18T10:00:00.000Z' }),
			template({ id: 'a-old', name: 'Alpha', createdAt: '2026-06-18T08:00:00.000Z' }),
		]).map((item) => item.id)).toEqual(['a-old', 'a-new', 'b']);
	});
});
