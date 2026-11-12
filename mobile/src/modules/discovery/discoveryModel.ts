import type { PublicEventSummaryDTO } from '@/api/types';

export const discoveryLoadingCopy = 'Loading published events…';
export const discoverySearchLabel = 'Search published events';
export const discoverySearchPlaceholder = 'Title, description or place';
export const discoveryViewEventLabel = 'View event';

export function formatDiscoveryDate(value: string) {
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat([], { month: 'short', day: 'numeric' }).format(date);
}

export function formatDiscoveryTime(value: string) {
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? '' : new Intl.DateTimeFormat([], { timeStyle: 'short' }).format(date);
}

export function discoveryPricingLabel(event: Pick<PublicEventSummaryDTO, 'pricingMode' | 'ticketPriceCents' | 'ticketCurrency'>) {
	if (event.pricingMode === 'free') {
		return 'Free';
	}

	return new Intl.NumberFormat([], { style: 'currency', currency: event.ticketCurrency.toUpperCase() }).format(event.ticketPriceCents / 100);
}

export function discoverySubtitle(event: Pick<PublicEventSummaryDTO, 'publicDescription'>) {
	return event.publicDescription || 'Published event';
}

export function discoveryEmptyTitle(query: string) {
	return query.trim() ? 'No events match that search' : 'No published events yet';
}

export function discoveryEmptyBody(query: string) {
	const trimmed = query.trim();
	return trimmed ? `No published events matched “${trimmed}”. Try another search.` : 'Nothing on the board. Events run on Subcult OS are listed here. When a host publishes one, it shows with its date and place.';
}

export function discoveryErrorCopy(message?: string | null) {
	return message ? `Could not load published events: ${message}` : 'Could not load published events.';
}
