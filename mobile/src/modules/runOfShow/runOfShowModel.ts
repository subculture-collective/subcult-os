import type { EventStaffingItemDTO } from '@/api/types';

export type RunOfShowFormState = {
	title: string;
	kind: EventStaffingItemDTO['kind'];
	notes: string;
	participantRequirements: string;
	startsAt: string;
	endsAt: string;
};

export type RunOfShowCreatePayload = {
	title: string;
	kind: EventStaffingItemDTO['kind'];
	notes: string;
	participantRequirements: string;
	startsAt: string | null;
	endsAt: string | null;
};

export type RunOfShowUpdatePayload = {
	title: string;
	notes: string;
	participantRequirements: string;
	startsAt?: string | null;
	clearStartsAt?: boolean;
	endsAt?: string | null;
	clearEndsAt?: boolean;
};

export function emptyRunOfShowForm(): RunOfShowFormState {
	return { title: '', kind: 'task', notes: '', participantRequirements: '', startsAt: '', endsAt: '' };
}

export function parseOptionalDateTime(value: string): string | null | false {
	const trimmed = value.trim();
	if (!trimmed) return null;
	const date = new Date(trimmed.replace(' ', 'T'));
	if (Number.isNaN(date.getTime())) return false;
	return date.toISOString();
}

export function isRunOfShowTimeRangeValid(startsAt: string | null, endsAt: string | null) {
	if (!startsAt || !endsAt) return true;
	return new Date(endsAt).getTime() >= new Date(startsAt).getTime();
}

export function toRunOfShowInputValue(value: string | null) {
	if (!value) return '';
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return '';
	const offset = date.getTimezoneOffset();
	return new Date(date.getTime() - offset * 60_000).toISOString().slice(0, 16).replace('T', ' ');
}

export function staffingStatusOrder(status: EventStaffingItemDTO['status']) {
	switch (status) {
		case 'open':
			return 0;
		case 'assigned':
			return 1;
		case 'completed':
			return 2;
		case 'cancelled':
			return 3;
	}
}

export function compareRunOfShowItems(left: EventStaffingItemDTO, right: EventStaffingItemDTO) {
	const statusDelta = staffingStatusOrder(left.status) - staffingStatusOrder(right.status);
	if (statusDelta !== 0) return statusDelta;

	const leftStarts = left.startsAt ? new Date(left.startsAt).getTime() : Number.POSITIVE_INFINITY;
	const rightStarts = right.startsAt ? new Date(right.startsAt).getTime() : Number.POSITIVE_INFINITY;
	if (leftStarts !== rightStarts) return leftStarts - rightStarts;

	const createdDelta = new Date(left.createdAt).getTime() - new Date(right.createdAt).getTime();
	if (createdDelta !== 0) return createdDelta;

	return left.id.localeCompare(right.id);
}

export function sortRunOfShowItems(items: EventStaffingItemDTO[]) {
	return [...items].sort(compareRunOfShowItems);
}

export function applyRunOfShowStatusUpdate(items: EventStaffingItemDTO[], updatedItem: EventStaffingItemDTO) {
	return sortRunOfShowItems(items.map((item) => (item.id === updatedItem.id ? updatedItem : item)));
}

export function staffingStatusLabel(status: EventStaffingItemDTO['status']) {
	switch (status) {
		case 'open':
			return 'Open';
		case 'assigned':
			return 'Assigned';
		case 'completed':
			return 'Completed';
		case 'cancelled':
			return 'Cancelled';
}
}

export function staffingKindLabel(kind: EventStaffingItemDTO['kind']) {
	return kind === 'task' ? 'Task' : 'Shift';
}

function formatDateTime(value: string) {
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat([], { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

export function staffingWindowLabel(item: EventStaffingItemDTO) {
	if (item.startsAt && item.endsAt) {
		return `${formatDateTime(item.startsAt)} → ${formatDateTime(item.endsAt)}`;
	}

	if (item.startsAt) {
		return `Starts ${formatDateTime(item.startsAt)}`;
	}

	if (item.endsAt) {
		return `Ends ${formatDateTime(item.endsAt)}`;
	}

	return 'No time window set';
}

export function buildCreateRunOfShowPayload(form: RunOfShowFormState, startsAt: string | null, endsAt: string | null): RunOfShowCreatePayload {
	return { title: form.title.trim(), kind: form.kind, notes: form.notes.trim(), participantRequirements: form.participantRequirements.trim(), startsAt, endsAt };
}

export function buildUpdateRunOfShowPayload(form: Pick<RunOfShowFormState, 'title' | 'notes' | 'participantRequirements'>, startsAt: string | null, endsAt: string | null): RunOfShowUpdatePayload {
	return {
		title: form.title.trim(),
		notes: form.notes.trim(),
		participantRequirements: form.participantRequirements.trim(),
		...(startsAt ? { startsAt } : { clearStartsAt: true }),
		...(endsAt ? { endsAt } : { clearEndsAt: true }),
	};
}
