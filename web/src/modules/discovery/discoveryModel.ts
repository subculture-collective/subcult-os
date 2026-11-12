import type { PublicEventSummaryDTO } from '../../domain';

export const discoveryBrowseLabel = 'Public listings';
export const discoveryBadgeLabel = 'Public';
export const discoveryScopeLabel = 'Published only';
export const discoveryTitle = 'What’s on';
// Facts follow the Studio claims register entry C-OS and match the page's meta description.
export const discoveryProductLine = 'Subcult OS is event software in development. Hosts plan the night, staff it, take free ticket reservations, check guests in at the door and get a report at close-out.';
export const discoveryDescription = 'Events that hosts have published. Open one for the details and its tickets.';
export const discoverySearchLabel = 'Search published events';
export const discoverySearchPlaceholder = 'Title, description or place';
export const discoveryLoadingCopy = 'Loading published events…';
export const discoveryViewEventLabel = 'View event';

export function getRequestedDiscoveryQuery() {
	if (typeof window === 'undefined') {
		return '';
	}

	return new URLSearchParams(window.location.search).get('q') ?? '';
}

export function formatDiscoveryDateTime(value: string) {
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat([], { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

export function formatDiscoveryCurrency(cents: number, currency: string) {
	return new Intl.NumberFormat([], { style: 'currency', currency: currency.toUpperCase() }).format(cents / 100);
}

export function discoveryPricingLabel(event: Pick<PublicEventSummaryDTO, 'pricingMode' | 'ticketPriceCents' | 'ticketCurrency'>) {
	return event.pricingMode === 'free' ? 'Free' : formatDiscoveryCurrency(event.ticketPriceCents, event.ticketCurrency);
}

export function discoveryRemainingLabel(event: Pick<PublicEventSummaryDTO, 'isFull' | 'remainingTickets'>) {
	if (event.isFull) {
		return 'Sold out';
	}

	return event.remainingTickets === 1 ? '1 ticket left' : `${event.remainingTickets} tickets left`;
}

export function discoveryEmptyStateCopy(searchQuery: string) {
	return discoveryEmptyBody(searchQuery);
}

export function discoveryEmptyTitle(searchQuery: string) {
	return searchQuery.trim() ? 'No events match that search' : 'No published events yet';
}

export function discoveryEmptyBody(searchQuery: string) {
	const query = searchQuery.trim();
	return query ? `No published events matched “${query}”. Try another search.` : 'Nothing on the board. Events run on Subcult OS are listed here. When a host publishes one, it shows with its date, its place and how many tickets are left.';
}

export function discoveryErrorCopy(error?: string | null) {
	return error ? `Could not load published events: ${error}` : 'Could not load published events.';
}
