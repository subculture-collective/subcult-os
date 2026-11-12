import { Children, isValidElement } from 'react';
import type { ReactElement, ReactNode } from 'react';
import { renderToString } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DiscoveryOccurrencesSection } from './DiscoverView';
import type { PublicDiscoveryOccurrenceDTO } from '../domain';

// This repository's convention for interactive-component tests (see
// IdentityActionView.test.tsx) mocks React's hooks so a function component
// can be invoked directly as a plain function and its returned element tree
// walked for handler props, without a jsdom/testing-library dependency.
// Native modal focus containment and Escape require a real browser. These
// tests cover markup and React state handlers, not browser modality.
const SKIP = Symbol('skip-state');

function makeUseStateImplementation(values: unknown[] = []) {
	const setters: Array<ReturnType<typeof vi.fn>> = [];
	const impl = ((initial: unknown) => {
		const setter = vi.fn();
		setters.push(setter);
		if (values.length > 0) {
			const next = values.shift();
			if (next === SKIP) {
				return [typeof initial === 'function' ? (initial as () => unknown)() : initial, setter];
			}
			return [next, setter];
		}
		return [typeof initial === 'function' ? (initial as () => unknown)() : initial, setter];
	}) as unknown as typeof import('react').useState;
	return { impl, setters };
}

let currentUseState: typeof import('react').useState = ((initial: unknown) => [initial, vi.fn()]) as never;

vi.mock('react', async () => {
	const actual = await vi.importActual<typeof import('react')>('react');
	return {
		...actual,
		useState: (initial: unknown) => currentUseState(initial),
		useEffect: () => undefined,
		useRef: (initial: unknown) => ({ current: initial }),
	};
});

vi.mock('../api', () => ({ api: vi.fn() }));

afterEach(() => {
	vi.unstubAllGlobals();
	vi.clearAllMocks();
});

type AnyElement = ReactElement<Record<string, unknown>>;

function findAll(node: ReactNode, predicate: (el: AnyElement) => boolean, out: AnyElement[] = []): AnyElement[] {
	for (const child of Children.toArray(node)) {
		if (!isValidElement<{ children?: ReactNode }>(child)) continue;
		const element = child as AnyElement;
		if (predicate(element)) out.push(element);
		if (element.props.children) findAll(element.props.children as ReactNode, predicate, out);
	}
	return out;
}

function findOne(node: ReactNode, predicate: (el: AnyElement) => boolean): AnyElement {
	const found = findAll(node, predicate);
	if (found.length === 0) throw new Error('element not found');
	return found[0];
}

function renderSection(stateValues: unknown[]) {
	const { impl, setters } = makeUseStateImplementation([...stateValues]);
	currentUseState = impl;
	const element = DiscoveryOccurrencesSection();
	return { element, setters };
}

function occurrence(overrides: Partial<PublicDiscoveryOccurrenceDTO> = {}): PublicDiscoveryOccurrenceDTO {
	return {
		uri: 'at://did:plc:abc/tv.subcult.event.occurrence/1',
		source: { did: 'did:plc:abc', uri: 'at://did:plc:abc/tv.subcult.event.occurrence/1' },
		name: 'Test Occurrence',
		startsAt: '2026-10-01T20:00:00.000Z',
		status: 'scheduled',
		projectionStatus: 'active',
		handoff: { kind: 'none', reason: 'no_mapping' },
		...overrides,
	};
}

describe('DiscoveryOccurrencesSection states', () => {
	it('renders the loading state', () => {
		const { element } = renderSection([null, true, null, null]);
		const html = renderToString(element);
		expect(html).toContain('Loading discovery occurrences');
	});

	it('renders the empty state', () => {
		const { element } = renderSection([[], false, null, null]);
		const html = renderToString(element);
		expect(html).toContain('No cultural events shared yet');
	});

	it('renders the error state', () => {
		const { element } = renderSection([[], false, 'Network down', null]);
		const html = renderToString(element);
		expect(html).toContain('Could not load discovery occurrences: Network down');
	});

	it('renders occurrence cards in a mobile-first single-column layout', () => {
		const list = [occurrence({ uri: 'uri-a', name: 'Alpha Night' }), occurrence({ uri: 'uri-b', name: 'Beta Night' })];
		const { element } = renderSection([list, false, null, null]);
		const html = renderToString(element);
		expect(html).toContain('Alpha Night');
		expect(html).toContain('Beta Night');
		// Base class is single-column (no explicit column count until the
		// sm: breakpoint), matching the 360px narrow-screen requirement;
		// full media-query evaluation still needs a real browser (see the
		// "Known limits" note in the test file header).
		expect(html).toContain('grid-cols-1');
		expect(html).toContain('sm:grid-cols-2');
	});

	it('opens the detail view on Enter/Space and never crashes on other keys', () => {
		const list = [occurrence({ uri: 'uri-a', name: 'Alpha Night' })];
		const { element, setters } = renderSection([list, false, null, null]);
		const card = findOne(element, (el) => el.type === 'article' && el.props.role === 'button');
		const onKeyDown = card.props.onKeyDown as (event: { key: string; preventDefault: () => void }) => void;

		onKeyDown({ key: 'Tab', preventDefault: vi.fn() });
		const selectedURISetter = setters[3];
		expect(selectedURISetter).not.toHaveBeenCalled();

		onKeyDown({ key: 'Enter', preventDefault: vi.fn() });
		expect(selectedURISetter).toHaveBeenCalledWith('uri-a');

		onKeyDown({ key: ' ', preventDefault: vi.fn() });
		expect(selectedURISetter).toHaveBeenCalledWith('uri-a');
	});

	it('opens the detail view when a card is clicked', () => {
		const list = [occurrence({ uri: 'uri-a', name: 'Alpha Night' })];
		const { element, setters } = renderSection([list, false, null, null]);
		const card = findOne(element, (el) => el.type === 'article' && el.props.role === 'button');
		(card.props.onClick as () => void)();
		expect(setters[3]).toHaveBeenCalledWith('uri-a');
	});

	it('shows the local reservation handoff for a bookable occurrence', () => {
		const list = [occurrence({ uri: 'uri-a', name: 'Alpha Night', handoff: { kind: 'local', eventSlug: 'alpha-night' } })];
		const { element } = renderSection([list, false, null, 'uri-a']);
		const html = renderToString(element);
		expect(html).toContain('href="/e/alpha-night"');
		expect(html).toContain('Reserve');
	});

	it('shows a clear unavailable state when handoff is none', () => {
		const list = [occurrence({ uri: 'uri-a', name: 'Alpha Night', handoff: { kind: 'none', reason: 'no_mapping' } })];
		const { element } = renderSection([list, false, null, 'uri-a']);
		const html = renderToString(element);
		expect(html).toContain('No reservation destination has been linked for this occurrence yet.');
		expect(html).not.toContain('href="/e/');
	});

	it('shows a clear unavailable state when the projection marked the occurrence deleted/unavailable', () => {
		const list = [occurrence({ uri: 'uri-a', name: 'Alpha Night', projectionStatus: 'deleted', handoff: { kind: 'local', eventSlug: 'alpha-night' } })];
		const { element } = renderSection([list, false, null, 'uri-a']);
		const html = renderToString(element);
		expect(html).toContain('This occurrence is no longer available from its source.');
		expect(html).not.toContain('href="/e/alpha-night"');
	});

	it('closes the detail view when the close button is activated', () => {
		const list = [occurrence({ uri: 'uri-a', name: 'Alpha Night' })];
		const { element, setters } = renderSection([list, false, null, 'uri-a']);
		const closeButton = findOne(element, (el) => el.type === 'button' && el.props.children === 'Close');
		(closeButton.props.onClick as () => void)();
		expect(setters[3]).toHaveBeenCalledWith(null);
	});

	it('uses a named native dialog and synchronizes cancellation with selection', () => {
		const list = [occurrence({ uri: 'uri-a', name: 'Alpha Night' })];
		const { element, setters } = renderSection([list, false, null, 'uri-a']);
		const dialog = findOne(element, (el) => el.type === 'dialog');
		expect(dialog.props['aria-label']).toBe('Alpha Night');
		expect(dialog.props.tabIndex).toBeUndefined();
		const preventDefault = vi.fn();
		(dialog.props.onCancel as (event: { preventDefault: () => void }) => void)({ preventDefault });
		expect(preventDefault).toHaveBeenCalledOnce();
		expect(setters[3]).toHaveBeenCalledWith(null);
	});

	it('names each map control and groups interactive points instead of flattening them as an image', () => {
		const list = [
			occurrence({ uri: 'uri-a', name: 'Alpha Night', timezone: 'America/Chicago', location: { name: 'Alpha venue', latitude: '41', longitude: '-87' } }),
			occurrence({ uri: 'uri-b', name: 'Beta Night', timezone: 'Asia/Tokyo', location: { name: 'Beta venue', latitude: '35', longitude: '139' } }),
		];
		const { element } = renderSection([list, false, null, null]);
		const plot = findOne(element, (el) => el.type === 'svg');
		expect(plot.props.role).toBe('group');
		const points = findAll(plot, (el) => el.type === 'circle');
		expect(points[0].props['aria-label']).toContain('View Alpha Night:');
		expect(points[0].props['aria-label']).toContain('America/Chicago');
		expect(points[1].props['aria-label']).toContain('View Beta Night:');
		expect(points[1].props['aria-label']).toContain('Asia/Tokyo');
	});

	it('keeps a polite atomic result status present while the occurrence list loads and resolves', () => {
		const loading = renderSection([null, true, null, null]).element;
		const loaded = renderSection([[occurrence()], false, null, null]).element;
		const empty = renderSection([[], false, null, null]).element;
		const failed = renderSection([[], false, 'Network down', null]).element;
		for (const element of [loading, loaded, empty, failed]) {
			const status = findOne(element, (el) => el.props.role === 'status');
			expect(status.props['aria-atomic']).toBe('true');
		}
		expect(renderToString(loading)).toContain('Loading discovery occurrences');
		expect(renderToString(loaded)).toContain('1 cultural occurrence found.');
		expect(renderToString(empty)).toContain('0 cultural occurrences found.');
		const failedStatus = findOne(failed, (el) => el.props.role === 'status');
		expect(failedStatus.props.children).toBe('Could not load discovery occurrences: Network down');
	});

	it('plots occurrences with public coordinates and omits ones without', () => {
		const list = [
			occurrence({ uri: 'uri-with-location', location: { name: 'Venue', latitude: '41.8', longitude: '-87.6' } }),
			occurrence({ uri: 'uri-without-location' }),
		];
		const { element } = renderSection([list, false, null, null]);
		const html = renderToString(element);
		expect(html).toContain('discovery-map-point-uri-with-location');
		expect(html).not.toContain('discovery-map-point-uri-without-location');
	});
});
