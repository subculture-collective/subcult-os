import { describe, expect, it } from 'vitest';
import type { EventStaffingItemDTO } from '../../domain';

import {
	buildCreateRunOfShowPayload,
	buildUpdateRunOfShowPayload,
	emptyRunOfShowForm,
	isRunOfShowTimeRangeValid,
	parseOptionalDateTime,
	staffingKindLabel,
	staffingStatusLabel,
	staffingWindowLabel,
	sortRunOfShowItems,
	toDateTimeLocalInputValue,
} from './runOfShowModel';

function staffingItem(overrides: Partial<EventStaffingItemDTO>): EventStaffingItemDTO {
	return {
		id: 'item',
		eventId: 'event',
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
		createdAt: '2026-06-18T09:00:00Z',
		updatedAt: '2026-06-18T09:00:00Z',
		completedAt: null,
		completedByPersonId: null,
		...overrides,
	};
}

describe('run of show helpers', () => {
	it('normalizes copy helpers', () => {
		expect(emptyRunOfShowForm().kind).toBe('task');
		expect(staffingStatusLabel('assigned')).toBe('Assigned');
		expect(staffingKindLabel('shift')).toBe('Shift');
		expect(
			staffingWindowLabel(staffingItem({ id: '1', startsAt: '2026-06-18T10:00:00Z' })),
		).toContain('Starts');
	});

	it('parses and converts local time values', () => {
		const localValue = toDateTimeLocalInputValue('2026-06-18T10:15:00.000Z');
		expect(localValue).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);
		expect(parseOptionalDateTime(localValue)).toBe('2026-06-18T10:15:00.000Z');
		expect(parseOptionalDateTime('')).toBeNull();
		expect(parseOptionalDateTime('not a time')).toBe(false);
	});

	it('sorts by status, start time, created time, then id', () => {
		const items = sortRunOfShowItems([
			staffingItem({ id: 'completed-earliest', status: 'completed', startsAt: '2026-06-18T08:00:00Z', createdAt: '2026-06-18T08:00:00Z' }),
			staffingItem({ id: 'assigned-earliest', status: 'assigned', startsAt: '2026-06-18T07:00:00Z', createdAt: '2026-06-18T07:00:00Z' }),
			staffingItem({ id: 'open-no-start', status: 'open', startsAt: null, createdAt: '2026-06-18T09:00:00Z' }),
			staffingItem({ id: 'open-later', status: 'open', startsAt: '2026-06-18T10:00:00Z', createdAt: '2026-06-18T08:00:00Z' }),
			staffingItem({ id: 'open-earlier-created-b', status: 'open', startsAt: '2026-06-18T09:00:00Z', createdAt: '2026-06-18T08:00:00Z' }),
			staffingItem({ id: 'open-earlier-created-a', status: 'open', startsAt: '2026-06-18T09:00:00Z', createdAt: '2026-06-18T07:00:00Z' }),
			staffingItem({ id: 'open-same-a', status: 'open', startsAt: '2026-06-18T09:00:00Z', createdAt: '2026-06-18T07:00:00Z' }),
		]);

		expect(items.map((item) => item.id)).toEqual([
			'open-earlier-created-a',
			'open-same-a',
			'open-earlier-created-b',
			'open-later',
			'open-no-start',
			'assigned-earliest',
			'completed-earliest',
		]);
	});

	it('builds payloads without imposing web time-range validation', () => {
		expect(isRunOfShowTimeRangeValid('2026-06-18T10:00:00.000Z', '2026-06-18T11:00:00.000Z')).toBe(true);
		expect(isRunOfShowTimeRangeValid('2026-06-18T11:00:00.000Z', '2026-06-18T10:00:00.000Z')).toBe(false);
		expect(buildCreateRunOfShowPayload({ title: '  Call sheet ', kind: 'shift', notes: '  Bring radios ', participantRequirements: '  Wear black ', startsAt: '', endsAt: '' }, null, null)).toEqual({ title: 'Call sheet', kind: 'shift', notes: 'Bring radios', participantRequirements: 'Wear black', startsAt: null, endsAt: null });
		expect(buildCreateRunOfShowPayload({ title: ' Reverse ', kind: 'task', notes: '', participantRequirements: '', startsAt: '', endsAt: '' }, '2026-06-18T11:00:00.000Z', '2026-06-18T10:00:00.000Z')).toEqual({ title: 'Reverse', kind: 'task', notes: '', participantRequirements: '', startsAt: '2026-06-18T11:00:00.000Z', endsAt: '2026-06-18T10:00:00.000Z' });
		expect(buildUpdateRunOfShowPayload({ title: '  Update ', notes: '  Keep ', participantRequirements: '  Bring ID ' }, '2026-06-18T10:00:00.000Z', null)).toEqual({ title: 'Update', notes: 'Keep', participantRequirements: 'Bring ID', startsAt: '2026-06-18T10:00:00.000Z', clearEndsAt: true });
	});
});
