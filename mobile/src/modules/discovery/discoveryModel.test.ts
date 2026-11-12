import { describe, expect, it } from 'vitest';

import {
	discoveryEmptyBody,
	discoveryEmptyTitle,
	discoveryErrorCopy,
	discoveryLoadingCopy,
	discoveryPricingLabel,
	discoverySubtitle,
	discoverySearchLabel,
	discoverySearchPlaceholder,
	discoveryViewEventLabel,
} from './discoveryModel';

describe('discoveryModel', () => {
	it('exposes stable copy', () => {
		expect(discoveryLoadingCopy).toBe('Loading published events…');
		expect(discoverySearchLabel).toBe('Search published events');
		expect(discoverySearchPlaceholder).toBe('Title, description or place');
		expect(discoveryViewEventLabel).toBe('View event');
		expect(discoveryEmptyTitle('')).toBe('No published events yet');
		expect(discoveryEmptyTitle('noise')).toBe('No events match that search');
		expect(discoveryEmptyBody('')).toBe('Nothing on the board. Events run on Subcult OS are listed here. When a host publishes one, it shows with its date and place.');
		expect(discoveryEmptyBody('noise')).toBe('No published events matched “noise”. Try another search.');
		expect(discoveryErrorCopy('offline')).toBe('Could not load published events: offline');
	});

	it('formats pricing and subtitles', () => {
		expect(discoveryPricingLabel({ pricingMode: 'free', ticketPriceCents: 0, ticketCurrency: 'usd' })).toBe('Free');
		expect(discoveryPricingLabel({ pricingMode: 'fixed', ticketPriceCents: 2500, ticketCurrency: 'usd' })).toBe(new Intl.NumberFormat([], { style: 'currency', currency: 'USD' }).format(25));
		expect(discoverySubtitle({ publicDescription: '' })).toBe('Published event');
	});
});
