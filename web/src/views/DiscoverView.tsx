import { useEffect, useRef, useState } from 'react';
import type { FormEvent, KeyboardEvent as ReactKeyboardEvent } from 'react';
import { api } from '../api';
import { TicketStub, indexLabel } from '../ui/TicketStub';
import type { PublicDiscoveryOccurrenceDTO, PublicEventSummaryDTO } from '../domain';
import {
	discoveryBadgeLabel,
	discoveryBrowseLabel,
	discoveryDescription,
	discoveryEmptyBody,
	discoveryEmptyTitle,
	discoveryErrorCopy,
	discoveryLoadingCopy,
	discoveryPricingLabel,
	discoveryRemainingLabel,
	discoveryScopeLabel,
	discoverySearchLabel,
	discoverySearchPlaceholder,
	discoveryTitle,
	discoveryViewEventLabel,
	formatDiscoveryDateTime,
	getRequestedDiscoveryQuery,
} from '../modules/discovery/discoveryModel';
import {
	discoveryOccurrenceDetailCloseLabel,
	discoveryOccurrencesDescription,
	discoveryOccurrencesEmptyBody,
	discoveryOccurrencesEmptyTitle,
	discoveryOccurrencesErrorCopy,
	discoveryOccurrencesLoadingCopy,
	discoveryOccurrencesTitle,
	formatOccurrenceDateTime,
	handoffButtonLabel,
	handoffLocalPath,
	handoffUnavailableReasonCopy,
	isOccurrenceUnavailable,
	occurrenceLocationSummary,
	occurrenceStatusLabel,
	projectOccurrencesToPlot,
} from '../modules/discovery/discoveryOccurrenceModel';
import {
	publicCardClass,
	publicEyebrowClass,
	publicFieldLabelClass,
	publicMutedTextClass,
	publicPageInnerClass,
	publicPageShellClass,
	publicPrimaryButtonClass,
	publicSecondaryButtonClass,
	publicStatusPillClass,
} from '../modules/publicUi/publicUi';

const DISCOVERY_MAP_WIDTH = 320;
const DISCOVERY_MAP_HEIGHT = 160;

export function DiscoveryOccurrencesSection() {
	const [occurrences, setOccurrences] = useState<PublicDiscoveryOccurrenceDTO[] | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);
	const [selectedURI, setSelectedURI] = useState<string | null>(null);

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		setError(null);
		api<PublicDiscoveryOccurrenceDTO[]>('/api/public/discovery/occurrences')
			.then((loaded) => {
				if (!cancelled) setOccurrences(loaded ?? []);
			})
			.catch((caught: unknown) => {
				if (cancelled) return;
				setError(caught instanceof Error ? caught.message : 'Unable to load discovery occurrences');
				setOccurrences([]);
			})
			.finally(() => {
				if (!cancelled) setLoading(false);
			});
		return () => {
			cancelled = true;
		};
	}, []);

	const modalRef = useRef<HTMLDialogElement | null>(null);
	const dialogRef = useRef<HTMLDivElement | null>(null);
	const openerRef = useRef<HTMLElement | SVGElement | null>(null);

	useEffect(() => {
		const modal = modalRef.current;
		if (!selectedURI || !modal) return;
		const opener = openerRef.current;
		if (!modal.open) modal.showModal();
		dialogRef.current?.focus();
		return () => {
			if (modal.open) modal.close();
			if (opener?.isConnected) opener.focus();
		};
	}, [selectedURI]);

	function handleDialogKeyDown(event: ReactKeyboardEvent<HTMLDialogElement>) {
		if (event.key !== 'Tab') return;
		const controls = event.currentTarget.querySelectorAll<HTMLElement>('button:not(:disabled), a[href]');
		const first = controls[0];
		const last = controls[controls.length - 1];
		if (!first || !last) return;
		if (event.shiftKey && (document.activeElement === first || document.activeElement === dialogRef.current)) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && document.activeElement === last) {
			event.preventDefault();
			first.focus();
		}
	}

	function openDetail(uri: string) {
		if (typeof document !== 'undefined' && (document.activeElement instanceof HTMLElement || document.activeElement instanceof SVGElement)) {
			openerRef.current = document.activeElement;
		}
		setSelectedURI(uri);
	}

	function handleCardKeyDown(event: ReactKeyboardEvent<Element>, uri: string) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			openDetail(uri);
		}
	}

	const selected = occurrences?.find((item) => item.uri === selectedURI) ?? null;
	const plotPoints = occurrences ? projectOccurrencesToPlot(occurrences, DISCOVERY_MAP_WIDTH, DISCOVERY_MAP_HEIGHT) : [];

	return (
		<section aria-label={discoveryOccurrencesTitle} className="flex min-w-0 flex-col gap-4 [overflow-wrap:anywhere]">
			<header id="discovery" className="scroll-mt-6 border-t border-stroke-subtle pt-8">
				<p className="index-label">{indexLabel(2, discoveryOccurrencesTitle)}</p>
				<p className="mt-3 max-w-2xl text-base leading-7 text-fg-secondary">{discoveryOccurrencesDescription}</p>
			</header>

			<p role="status" aria-atomic="true" className="sr-only">
				{loading ? discoveryOccurrencesLoadingCopy : error ? discoveryOccurrencesErrorCopy(error) : occurrences ? `${occurrences.length} cultural ${occurrences.length === 1 ? 'occurrence' : 'occurrences'} found.` : ''}
			</p>
			{loading ? <div className={`${publicCardClass} ${publicMutedTextClass}`}>{discoveryOccurrencesLoadingCopy}</div> : null}

			{error ? (
				<p className="border border-status-danger/20 bg-status-surface-danger px-4 py-3 text-sm font-medium text-status-danger">
					{discoveryOccurrencesErrorCopy(error)}
				</p>
			) : null}

			{!loading && !error && occurrences?.length === 0 ? (
				<div className={publicCardClass}>
					<h2 className="text-xl font-bold text-fg-primary">{discoveryOccurrencesEmptyTitle}</h2>
					<p className={`mt-2 ${publicMutedTextClass}`}>{discoveryOccurrencesEmptyBody}</p>
				</div>
			) : null}

			{!loading && !error && plotPoints.length > 0 ? (
				<div className={publicCardClass}>
					<p className={publicEyebrowClass}>Public locations</p>
					<svg
						role="group"
						aria-label="Coordinate plot of discovery occurrences with public locations"
						className="mt-3 w-full bg-surface-inset"
						viewBox={`0 0 ${DISCOVERY_MAP_WIDTH} ${DISCOVERY_MAP_HEIGHT}`}
					>
						{plotPoints.map((point) => (
							<circle
								key={point.uri}
								data-testid={`discovery-map-point-${point.uri}`}
								cx={point.x}
								cy={point.y}
								r={4}
								className="cursor-pointer fill-fg-primary outline-none focus-visible:focus-ring"
								role="button"
								tabIndex={0}
								aria-label={point.label}
								onClick={() => openDetail(point.uri)}
								onKeyDown={(event) => handleCardKeyDown(event, point.uri)}
							/>
						))}
					</svg>
				</div>
			) : null}

			{!loading && !error && occurrences && occurrences.length > 0 ? (
				<ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
					{occurrences.map((occurrence) => (
						<li key={occurrence.uri}>
							<article
								role="button"
								tabIndex={0}
								aria-label={occurrence.name}
								className="flex h-full cursor-pointer flex-col gap-3 border border-stroke-subtle bg-surface-panel p-5 outline-none focus-visible:focus-ring"
								onClick={() => openDetail(occurrence.uri)}
								onKeyDown={(event) => handleCardKeyDown(event, occurrence.uri)}
							>
								<p className={publicEyebrowClass}>{formatOccurrenceDateTime(occurrence.startsAt, occurrence.timezone)}</p>
								<h3 className="text-xl font-bold leading-tight text-fg-primary">{occurrence.name}</h3>
								<span className={publicStatusPillClass(isOccurrenceUnavailable(occurrence) ? 'danger' : 'neutral')}>{occurrenceStatusLabel(occurrence)}</span>
								<p className={publicMutedTextClass}>{occurrenceLocationSummary(occurrence.location)}</p>
							</article>
						</li>
					))}
				</ul>
			) : null}

			{selected ? (
				<dialog
					ref={modalRef}
					onKeyDown={handleDialogKeyDown}
					aria-label={selected.name}
					className="fixed inset-0 m-0 hidden h-full max-h-none w-full max-w-none items-end justify-center border-0 bg-transparent p-4 backdrop:bg-black/40 open:flex sm:items-center"
					onCancel={(event) => {
						event.preventDefault();
						setSelectedURI(null);
					}}
					onClick={() => setSelectedURI(null)}
				>
					<div
						ref={dialogRef}
						tabIndex={-1}
						className={`${publicCardClass} max-h-full w-full max-w-lg overflow-y-auto outline-none`}
						onClick={(event) => event.stopPropagation()}
					>
						<div className="flex items-start justify-between gap-3">
							<h2 className="min-w-0 flex-1 text-2xl font-bold text-fg-primary">{selected.name}</h2>
							<button type="button" className={`${publicSecondaryButtonClass} shrink-0`} onClick={() => setSelectedURI(null)}>
								{discoveryOccurrenceDetailCloseLabel}
							</button>
						</div>

						<dl className="mt-4 grid gap-3 text-sm text-fg-secondary">
							<div className="bg-surface-inset p-4">
								<dt className={publicEyebrowClass}>Source</dt>
								<dd className="mt-2 break-all font-mono text-xs text-fg-secondary">{selected.source.uri}</dd>
							</div>
							<div className="bg-surface-inset p-4">
								<dt className={publicEyebrowClass}>Status</dt>
								<dd className="mt-2 font-bold text-fg-primary">{occurrenceStatusLabel(selected)}</dd>
							</div>
							<div className="bg-surface-inset p-4">
								<dt className={publicEyebrowClass}>When</dt>
								<dd className="mt-2 font-bold text-fg-primary">{formatOccurrenceDateTime(selected.startsAt, selected.timezone)}</dd>
							</div>
							<div className="bg-surface-inset p-4">
								<dt className={publicEyebrowClass}>Location</dt>
								<dd className="mt-2 font-bold text-fg-primary">{occurrenceLocationSummary(selected.location)}</dd>
							</div>
						</dl>

						{isOccurrenceUnavailable(selected) ? (
							<p className="mt-4 border border-status-warning/20 bg-status-surface-warning p-4 text-sm font-medium text-status-warning">This occurrence is no longer available from its source.</p>
						) : selected.handoff.kind === 'local' ? (
							<a className={`${publicPrimaryButtonClass} mt-4 w-full`} href={handoffLocalPath(selected.handoff) ?? '#'}>
								{handoffButtonLabel(selected.handoff)}
							</a>
						) : (
							<p className="mt-4 border border-stroke-subtle bg-surface-inset p-4 text-sm font-medium text-fg-secondary">{handoffUnavailableReasonCopy(selected.handoff)}</p>
						)}
					</div>
				</dialog>
			) : null}
		</section>
	);
}

export function DiscoverView() {
	const [events, setEvents] = useState<PublicEventSummaryDTO[] | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);
	const [searchQuery, setSearchQuery] = useState(() => getRequestedDiscoveryQuery().trim());
	const requestSeq = useRef(0);

	async function loadEvents(nextQuery: string, syncUrl = false) {
		const normalizedQuery = nextQuery.trim();
		const requestID = ++requestSeq.current;

		if (syncUrl && typeof window !== 'undefined') {
			const suffix = normalizedQuery ? `?q=${encodeURIComponent(normalizedQuery)}` : '';
			window.history.pushState({}, '', `/discover${suffix}`);
		}

		setLoading(true);
		setError(null);

		try {
			const path = normalizedQuery ? `/api/public/events?q=${encodeURIComponent(normalizedQuery)}` : '/api/public/events';
			const loaded = await api<PublicEventSummaryDTO[]>(path);
			if (requestSeq.current !== requestID) {
				return;
			}
			setEvents(loaded ?? []);
		} catch (caught) {
			if (requestSeq.current !== requestID) {
				return;
			}
			setError(caught instanceof Error ? caught.message : 'Unable to load published events');
			setEvents([]);
		} finally {
			if (requestSeq.current === requestID) {
				setLoading(false);
			}
		}
	}

	useEffect(() => {
		void loadEvents(searchQuery);
	}, []);

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		const nextQuery = searchQuery.trim();
		setSearchQuery(nextQuery);
		void loadEvents(nextQuery, true);
	}

	function handleReset() {
		setSearchQuery('');
		void loadEvents('', true);
	}

	return (
		<main className={publicPageShellClass}>
			<section className={`${publicPageInnerClass} min-w-0 max-w-6xl [overflow-wrap:anywhere]`}>
				<header className="grid min-w-0 gap-10 pt-4 md:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] md:items-center md:gap-12 md:pt-10">
					<TicketStub kicker={`Subcult OS / ${discoveryBrowseLabel}`} tilt className="mx-2 md:mx-0">
						<h1 className="ticket-stub-title">{discoveryTitle}</h1>
					</TicketStub>

					<div className="min-w-0">
						<div className="flex flex-wrap items-center gap-2">
							<span className={publicStatusPillClass('success')}>{discoveryBadgeLabel}</span>
							<span className={publicStatusPillClass()}>{discoveryScopeLabel}</span>
						</div>
						<p className="mt-5 max-w-md text-base leading-7 text-fg-secondary">{discoveryDescription}</p>
						<nav aria-label="Sections" className="mt-6">
							<ol className="flex flex-col gap-3">
								<li><a className="index-label underline-offset-4 hover:underline" href="#listings">{indexLabel(1, 'Listings')}</a></li>
								<li><a className="index-label underline-offset-4 hover:underline" href="#discovery">{indexLabel(2, 'Discovery')}</a></li>
							</ol>
						</nav>
					</div>

					<form className="flex flex-col gap-3 border-y border-stroke-subtle py-5 sm:flex-row sm:items-end md:col-span-2" onSubmit={handleSubmit}>
						<label className="flex-1 space-y-2">
							<span className={publicEyebrowClass}>{discoverySearchLabel}</span>
							<input
								className="field py-3 text-sm focus-visible:focus-ring"
								type="search"
								value={searchQuery}
								onChange={(event) => setSearchQuery(event.target.value)}
								placeholder={discoverySearchPlaceholder}
							/>
						</label>
						<div className="flex gap-3 sm:shrink-0">
							<button className={publicPrimaryButtonClass} type="submit">
								Search
							</button>
							<button className={publicSecondaryButtonClass} type="button" onClick={() => handleReset()}>
								Reset
							</button>
						</div>
					</form>
				</header>

				<p id="listings" className="index-label scroll-mt-6 pt-4">{indexLabel(1, 'Listings')}</p>

				<p role="status" aria-atomic="true" className="sr-only">
					{loading ? discoveryLoadingCopy : error ? discoveryErrorCopy(error) : events ? `${events.length} published ${events.length === 1 ? 'event' : 'events'} found.` : ''}
				</p>
				{loading ? <div className={`${publicCardClass} ${publicMutedTextClass}`}>{discoveryLoadingCopy}</div> : null}

				{error ? (
					<p className="border border-status-danger/20 bg-status-surface-danger px-4 py-3 text-sm font-medium text-status-danger">
						{discoveryErrorCopy(error)}
					</p>
				) : null}

				{!loading && !error && events?.length === 0 ? (
					<div className={publicCardClass}>
						<h2 className="text-xl font-bold text-fg-primary">{discoveryEmptyTitle(searchQuery)}</h2>
						<p className={`mt-2 ${publicMutedTextClass}`}>{discoveryEmptyBody(searchQuery)}</p>
					</div>
				) : null}

				{!loading && !error && events && events.length > 0 ? (
					<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
						{events.map((event) => (
							<article key={event.id} className="overflow-hidden rounded-panel border border-stroke-subtle bg-surface-panel">
								{event.imageUrl ? <img className="h-48 w-full object-cover" src={event.imageUrl} alt="" /> : null}

								<div className="flex h-full flex-col p-5">
									<div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
										<p className={publicEyebrowClass}>{formatDiscoveryDateTime(event.startsAt)}</p>
										<span className={`${publicStatusPillClass(event.isFull ? 'danger' : 'neutral')} shrink-0`}>{discoveryRemainingLabel(event)}</span>
									</div>
									<h2 className="mt-3 text-3xl leading-none text-fg-primary [overflow-wrap:break-word]">{event.title}</h2>

									<div className="mt-4 grid text-sm text-fg-secondary [&>div]:border-t [&>div]:border-dashed [&>div]:border-stroke-subtle [&>div]:py-3">
										<div>
											<p className={publicFieldLabelClass}>Hosted by</p>
											<p className="mt-1 font-bold text-fg-primary">{event.workspaceName}</p>
										</div>
										<div>
											<p className={publicFieldLabelClass}>Location</p>
											<p className="mt-1 font-bold text-fg-primary">{event.locationDisplay}</p>
										</div>
										<div>
											<p className={publicFieldLabelClass}>Description</p>
											<p className="mt-2 leading-6 text-fg-secondary">{event.publicDescription || 'No public description provided.'}</p>
										</div>
										<div className="grid gap-3 sm:grid-cols-2">
											<div>
												<p className={publicFieldLabelClass}>Pricing</p>
												<p className="mt-1 font-bold text-fg-primary">{discoveryPricingLabel(event)}</p>
											</div>
											<div>
												<p className={publicFieldLabelClass}>Remaining tickets</p>
												<p className={`mt-2 font-bold ${event.isFull ? 'text-status-danger' : 'text-fg-primary'}`}>{event.remainingTickets}</p>
											</div>
										</div>
									{event.applicationsOpen ? (
										<div className="border border-status-success/20 bg-status-surface-success p-4">
											<p className="text-xs font-bold uppercase tracking-[0.2em] text-fg-muted">Applications</p>
											<p className="mt-2 font-bold text-status-success">Applications open</p>
										</div>
									) : null}
								</div>

									<a className={`${publicPrimaryButtonClass} mt-5 w-full`} href={event.publicUrl}>
										{discoveryViewEventLabel}
									</a>
								</div>
							</article>
						))}
					</div>
				) : null}

				<DiscoveryOccurrencesSection />
                {!loading && !error && <footer className="mt-10 border-t border-stroke-subtle pt-5 text-sm text-fg-secondary">
                    Subcult OS is developed by SUBCULT. <a className="underline underline-offset-4" href="https://subcult.tv/support">Support the studio</a>. Contributions are separate from event tickets and donations and do not buy a reservation or product access.
                </footer>}
			</section>
		</main>
	);
}
