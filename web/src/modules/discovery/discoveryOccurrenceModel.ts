import type { PublicDiscoveryHandoffDTO, PublicDiscoveryLocationDTO, PublicDiscoveryOccurrenceDTO } from '../../domain';

export const discoveryOccurrencesTitle = 'Cultural discovery';
export const discoveryOccurrencesDescription = 'Public cultural events and where they are. Open one for its details and whether it can be reserved here.';
export const discoveryOccurrencesLoadingCopy = 'Loading discovery occurrences…';
export const discoveryOccurrencesEmptyTitle = 'No cultural events shared yet';
export const discoveryOccurrencesEmptyBody = 'When public cultural events are shared, they show here with their locations.';
export const discoveryOccurrenceDetailCloseLabel = 'Close';
export const discoveryOccurrenceUnavailableLabel = 'This occurrence is no longer available from its source.';
export const discoveryOccurrenceExternalLabel = '(opens an external site)';

export function discoveryOccurrencesErrorCopy(error?: string | null) {
	return error ? `Could not load discovery occurrences: ${error}` : 'Could not load discovery occurrences.';
}

/** True when the record's own projection status means it should never be presented as a live/bookable occurrence. */
export function isOccurrenceUnavailable(occurrence: Pick<PublicDiscoveryOccurrenceDTO, 'projectionStatus'>) {
	return occurrence.projectionStatus !== 'active';
}

export function occurrenceStatusLabel(occurrence: Pick<PublicDiscoveryOccurrenceDTO, 'status' | 'projectionStatus'>) {
	if (occurrence.projectionStatus === 'deleted') {
		return 'No longer published';
	}
	if (occurrence.projectionStatus === 'unavailable') {
		return 'Source unavailable';
	}
	switch (occurrence.status) {
		case 'rescheduled':
			return 'Rescheduled';
		case 'postponed':
			return 'Postponed';
		case 'cancelled':
			return 'Cancelled';
		default:
			return 'Scheduled';
	}
}

export function formatOccurrenceDateTime(startsAt: string, timezone?: string) {
	const date = new Date(startsAt);
	if (Number.isNaN(date.getTime())) {
		return startsAt;
	}
	if (timezone) {
		try {
			const formatted = new Intl.DateTimeFormat([], { dateStyle: 'medium', timeStyle: 'short', timeZone: timezone }).format(date);
			const offset = new Intl.DateTimeFormat([], { timeZone: timezone, timeZoneName: 'shortOffset' })
				.formatToParts(date).find((part) => part.type === 'timeZoneName')?.value;
			return `${formatted} (${timezone}${offset ? `, ${offset}` : ''})`;
		} catch (error) {
			if (!(error instanceof RangeError)) throw error;
			// Projected zone strings are untrusted. Never relabel the viewer's
			// local clock with an invalid or unsupported event zone.
		}
	}
	const formatted = new Intl.DateTimeFormat([], { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' }).format(date);
	return `${formatted} (UTC; event time zone unavailable)`;
}

export function occurrenceLocationSummary(location?: PublicDiscoveryLocationDTO | null) {
	if (!location) {
		return 'Location to be announced';
	}
	const parts = [location.name, location.locality, location.region, location.country].filter(Boolean);
	return parts.join(', ') || 'Location to be announced';
}

export function occurrenceHasPublicCoordinates(location?: PublicDiscoveryLocationDTO | null): location is PublicDiscoveryLocationDTO & { latitude: string; longitude: string } {
	return Boolean(location?.latitude && location?.longitude && !Number.isNaN(Number(location.latitude)) && !Number.isNaN(Number(location.longitude)));
}

export function handoffButtonLabel(handoff: PublicDiscoveryHandoffDTO) {
	if (handoff.kind === 'local') {
		return 'Reserve';
	}
	return 'Reservation unavailable';
}

export function handoffUnavailableReasonCopy(handoff: PublicDiscoveryHandoffDTO) {
	switch (handoff.reason) {
		case 'no_mapping':
			return 'No reservation destination has been linked for this occurrence yet.';
		case 'event_not_published':
			return 'The linked event is not currently published.';
		case 'event_not_found':
			return 'The linked event could not be found.';
		case 'mapping_invalid':
			return 'The reservation mapping is no longer valid.';
		case 'mapping_deleted':
			return 'The reservation mapping was removed.';
		case 'mapping_unavailable':
			return 'The reservation mapping is temporarily unavailable.';
		default:
			return 'No reservation destination is available for this occurrence.';
	}
}

/** Local-only destination path this app can route to directly (never an external URL). */
export function handoffLocalPath(handoff: PublicDiscoveryHandoffDTO) {
	if (handoff.kind === 'local' && handoff.eventSlug) {
		return `/e/${handoff.eventSlug}`;
	}
	return null;
}

export interface DiscoveryPlotPoint {
	uri: string;
	label: string;
	x: number;
	y: number;
}

/**
 * Projects occurrences with public coordinates onto a fixed-size, tile-free
 * coordinate plot (a simple equirectangular projection: longitude maps
 * linearly to x, latitude maps linearly to y). This intentionally adds no
 * mapping/tile dependency; it is a lightweight visual index into the list,
 * not a navigable map.
 */
export function projectOccurrencesToPlot(occurrences: PublicDiscoveryOccurrenceDTO[], width: number, height: number): DiscoveryPlotPoint[] {
	const points: DiscoveryPlotPoint[] = [];
	for (const occurrence of occurrences) {
		if (!occurrenceHasPublicCoordinates(occurrence.location)) {
			continue;
		}
		const lat = Number(occurrence.location.latitude);
		const lon = Number(occurrence.location.longitude);
		const x = ((lon + 180) / 360) * width;
		const y = ((90 - lat) / 180) * height;
		points.push({ uri: occurrence.uri, label: `View ${occurrence.name}: ${formatOccurrenceDateTime(occurrence.startsAt, occurrence.timezone)}`, x, y });
	}
	return points;
}

/** Encodes an at:// URI for the discovery detail path segment (strips the scheme; the backend route re-adds it). */
export function discoveryDetailPathFor(uri: string) {
	return uri.startsWith('at://') ? uri.slice('at://'.length) : uri;
}
