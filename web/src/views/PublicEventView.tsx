import { eventMetadata, unavailableEventMetadata } from '../seo';
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { ApiError, api, postJSON } from '../api';
import type { EventRoleApplicationDTO, EventRoleDTO, PaidReservationDTO, PublicEventDTO, TicketReservationDTO } from '../domain';
import { reserveFreeTicket } from '../modules/publicEvent/reservation';
import { checkoutDestination, checkoutTerminalState, nextPurchaseIntent } from '../modules/publicEvent/purchaseIntent';
import {
  publicEventConversionSummary,
  publicEventPrimaryCtaLabel,
  publicEventReservationSuccessCopy,
  publicEventRoleSectionIntro,
} from '../modules/publicEvent/publicEventConversion';
import {
  publicCardClass,
  publicEyebrowClass,
  publicHeroCardClass,
  publicMutedTextClass,
  publicPageInnerClass,
  publicPageShellClass,
  publicPrimaryButtonClass,
  publicSecondaryButtonClass,
  publicStatusPillClass,
} from '../modules/publicUi/publicUi';

const publicInputClass = 'field py-3';

function formatDateTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat([], { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function chunkCode(value: string) {
  return value.match(/.{1,4}/g) ?? [value];
}

function formatCurrency(cents: number, currency: string) {
  return new Intl.NumberFormat([], { style: 'currency', currency: currency.toUpperCase() }).format(cents / 100);
}

function pricingLabel(event: PublicEventDTO | null) {
  if (!event) return 'Pricing unavailable';
  if (event.pricingMode === 'free') {
    return 'Free guest reservation';
  }

  return `${formatCurrency(event.ticketPriceCents, event.ticketCurrency)} ticket`;
}

type RoleApplicationDraft = {
  applicantName: string;
  applicantEmail: string;
  message: string;
  submitting: boolean;
  submitted: boolean;
  error: string | null;
};

function emptyRoleApplicationDraft(): RoleApplicationDraft {
  return {
    applicantName: '',
    applicantEmail: '',
    message: '',
    submitting: false,
    submitted: false,
    error: null,
  };
}

function countRunes(value: string) {
  return Array.from(value).length;
}

export function PublicEventView({ slug }: { slug: string }) {
  return <PublicEventPage key={slug} slug={slug} />;
}

function PublicEventPage({ slug }: { slug: string }) {
  const [loadedEvent, setEvent] = useState<PublicEventDTO | null>(null);
  const [email, setEmail] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [loading, setLoading] = useState(true);
  const [reserving, setReserving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reservation, setReservation] = useState<TicketReservationDTO | null>(null);
  const [roles, setRoles] = useState<EventRoleDTO[] | null>(null);
  const [applicationDrafts, setApplicationDrafts] = useState<Record<string, RoleApplicationDraft>>({});
  const [availabilityKnown, setAvailabilityKnown] = useState(true);
  const [pendingTicketURL, setPendingTicketURL] = useState<string | null>(null);
  const paidIntent = useRef<{ email: string; displayName: string; key: string } | null>(null);
  const event = !loading && loadedEvent?.publicSlug === slug ? loadedEvent : null;
  const active = useRef(true);
  const ticketSubmitting = useRef(false);
  const roleSubmissions = useRef(new Set<string>());

  // Invalidate callbacks in the commit that removes this event's keyed view.
  useLayoutEffect(() => {
    active.current = true;
    return () => { active.current = false; };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setLoading(true);
      setEvent(null);
      setError(null);
      setReservation(null);
      setEmail('');
      setDisplayName('');
      setReserving(false);
      setRoles(null);
      setApplicationDrafts({});
      setAvailabilityKnown(true);
	  setPendingTicketURL(null);
      paidIntent.current = null;

      try {
        const loaded = await api<PublicEventDTO>(`/api/public/events/${slug}`);
        if (loaded.publicSlug !== slug) throw new Error('Event details do not match this page. Return to Discover and try again.');
        if (!cancelled) {
          setEvent(loaded);
          eventMetadata(loaded);
        }
      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : 'Unable to load public event');
          if (caught instanceof ApiError && caught.status === 404) unavailableEventMetadata();
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void load();

    return () => {
      cancelled = true;
    };
  }, [slug]);

  useEffect(() => {
    if (!event || event.publicSlug !== slug) {
      return;
    }

    let cancelled = false;

    async function loadRoles() {
      try {
        const loaded = await api<EventRoleDTO[]>(`/api/public/events/${slug}/roles`);
        if (!cancelled) {
          setRoles(loaded);
        }
      } catch (caught) {
        if (!cancelled) {
          setRoles([]);
          setError(caught instanceof Error ? caught.message : 'Unable to load public roles');
        }
      }
    }

    void loadRoles();

    return () => {
      cancelled = true;
    };
  }, [event, slug]);

  function updateRoleDraft(roleID: string, updater: (draft: RoleApplicationDraft) => RoleApplicationDraft) {
    setApplicationDrafts((current) => {
      const draft = current[roleID] ?? emptyRoleApplicationDraft();
      return {
        ...current,
        [roleID]: updater(draft),
      };
    });
  }

  async function handleRoleSubmit(role: EventRoleDTO, formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    if (!active.current || !event || role.eventId !== event.id || roleSubmissions.current.has(role.id)) return;

    const draft = applicationDrafts[role.id] ?? emptyRoleApplicationDraft();
    const trimmedName = draft.applicantName.trim();
    const trimmedEmail = draft.applicantEmail.trim();
    const trimmedMessage = draft.message.trim();

    if (!trimmedName) {
      updateRoleDraft(role.id, (current) => ({ ...current, error: 'Please enter your name.', submitted: false }));
      return;
    }
    if (!trimmedEmail || !trimmedEmail.includes('@')) {
      updateRoleDraft(role.id, (current) => ({ ...current, error: 'Please enter a valid email address.', submitted: false }));
      return;
    }
    if (countRunes(trimmedMessage) > 2000) {
      updateRoleDraft(role.id, (current) => ({ ...current, error: 'Message must be 2000 characters or fewer.', submitted: false }));
      return;
    }

    if (draft.submitted) return;
    roleSubmissions.current.add(role.id);
    updateRoleDraft(role.id, (current) => ({ ...current, submitting: true, error: null }));

    try {
      const submitted = await postJSON<EventRoleApplicationDTO>(`/api/public/events/${slug}/role-applications`, {
        roleId: role.id,
        applicantName: trimmedName,
        applicantEmail: trimmedEmail,
        message: trimmedMessage,
      });

      if (!active.current) return;
      updateRoleDraft(role.id, (current) => ({
        ...current,
        applicantName: submitted.applicantName,
        applicantEmail: submitted.applicantEmail,
        message: submitted.message,
        submitting: false,
        submitted: true,
        error: null,
      }));
    } catch (caught) {
      if (!active.current) return;
      roleSubmissions.current.delete(role.id);
      updateRoleDraft(role.id, (current) => ({
        ...current,
        submitting: false,
        error: caught instanceof Error ? caught.message : 'Unable to submit application',
        submitted: false,
      }));
    }
  }

  async function handleSubmit(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();

    if (!active.current || !event || event.isFull || ticketSubmitting.current || pendingTicketURL) return;

    const trimmedEmail = email.trim();
    const trimmedDisplayName = displayName.trim();

    if (!trimmedEmail) {
      setError('Please enter the email address where we should send the ticket.');
      return;
    }

    ticketSubmitting.current = true;
    setReserving(true);
    setError(null);
	setPendingTicketURL(null);

    try {
      if (event?.pricingMode === 'fixed') {
        paidIntent.current = nextPurchaseIntent(paidIntent.current, trimmedEmail, trimmedDisplayName, () => crypto.randomUUID());
        const checkout = await postJSON<PaidReservationDTO>(`/api/public/events/${slug}/paid-reservations`, {
          email: trimmedEmail,
          displayName: trimmedDisplayName || undefined,
          purchaseIntentKey: paidIntent.current.key,
        });

        const destination = checkoutDestination(checkout);
        if (!active.current) return;
        if (destination) {
          window.location.href = destination;
          return;
        }
        setPendingTicketURL(checkout.ticketUrl);
        setError('Your checkout is awaiting confirmation. Do not submit another purchase; use your ticket link after the provider confirms it.');
        return;
      }

      const result = await reserveFreeTicket(slug, {
        email: trimmedEmail,
        displayName: trimmedDisplayName || undefined,
      });

      if (!active.current) return;
      setReservation(result.ticket);
      setAvailabilityKnown(result.event?.publicSlug === slug);
      if (result.event?.publicSlug === slug) setEvent(result.event);
    } catch (caught) {
      if (!active.current) return;
      if (caught instanceof ApiError && caught.status === 409) {
        const terminal = checkoutTerminalState(caught.data);
        if (terminal) {
          setPendingTicketURL(terminal.ticketUrl);
          setError(terminal.status === 'expired'
            ? 'This checkout expired before payment completed. Review your ticket status before starting another purchase.'
            : 'This checkout needs reconciliation. Review your ticket status before starting another purchase.');
          return;
        }
      }
      setError(caught instanceof Error ? caught.message : 'Unable to reserve ticket');
    } finally {
      if (active.current) {
        ticketSubmitting.current = false;
        setReserving(false);
      }
    }
  }

  return (
    <main className={publicPageShellClass}>
      <section className={`${publicPageInnerClass} min-w-0 [overflow-wrap:anywhere]`}>
        <header className={publicHeroCardClass}>
          {event?.imageUrl ? <img className="h-72 w-full object-cover sm:h-96" src={event.imageUrl} alt="" /> : null}

          <div className="p-5 sm:p-7">
            <div className="flex flex-wrap items-center gap-2">
              <span className={publicStatusPillClass()}>{pricingLabel(event)}</span>
              <span className={publicStatusPillClass('success')}>No account needed</span>
              {event?.pricingMode === 'fixed' ? <span className={publicStatusPillClass()}>Secure checkout</span> : null}
              <span className={publicStatusPillClass(!event || !availabilityKnown ? 'neutral' : event.isFull ? 'danger' : 'success')}>{!event || !availabilityKnown ? 'Availability unavailable' : event.isFull ? 'Sold out' : `${event.remainingTickets} remaining`}</span>
              <a className="ml-auto text-xs font-bold uppercase tracking-[0.05em] text-fg-primary underline underline-offset-4" href="/discover">
                Discover more events
              </a>
            </div>

            <div className="mt-5 grid gap-6 lg:grid-cols-[1.1fr_0.9fr] lg:items-end">
              <div>
                <p className={publicEyebrowClass}>{event ? pricingLabel(event) : 'Event details'}</p>
                <h1 className="mt-3 text-4xl font-bold tracking-tight text-fg-primary sm:text-5xl">{event?.title ?? (loading ? 'Loading event…' : 'Event unavailable')}</h1>
                <p className="mt-3 max-w-2xl text-base leading-7 text-fg-secondary">{!event ? (loading ? 'Loading event details and availability.' : 'Return to Discover to choose an available event.') : availabilityKnown ? publicEventConversionSummary(event, pricingLabel(event)) : 'Your ticket is reserved. Current availability could not be refreshed.'}</p>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <div className="rounded-3xl bg-surface-inset p-4">
                  <p className={publicEyebrowClass}>Date & time</p>
                  <p className="mt-2 text-sm font-bold text-fg-primary">{event ? formatDateTime(event.startsAt) : loading ? 'Loading event…' : 'Date unavailable'}</p>
                </div>
                <div className="rounded-3xl bg-surface-inset p-4">
                  <p className={publicEyebrowClass}>Location</p>
                  <p className="mt-2 text-sm font-bold text-fg-primary">{event?.locationDisplay ?? (loading ? 'Loading location…' : 'Location unavailable')}</p>
                </div>
                <div className="rounded-3xl bg-surface-inset p-4">
                  <p className={publicEyebrowClass}>Tickets</p>
                  <p className={`mt-2 text-sm font-bold ${event?.isFull ? 'text-status-danger' : 'text-fg-primary'}`}>{!availabilityKnown ? 'Availability unavailable' : event ? (event.isFull ? 'Sold out' : `${event.remainingTickets} left`) : loading ? 'Loading availability…' : 'Availability unavailable'}</p>
                </div>
                <div className="rounded-3xl bg-surface-inset p-4">
                  <p className={publicEyebrowClass}>Pricing</p>
                  <p className="mt-2 text-sm font-bold text-fg-primary">{event ? pricingLabel(event) : loading ? 'Loading pricing…' : 'Pricing unavailable'}</p>
                </div>
              </div>
            </div>
          </div>
        </header>

        {loading ? <div className={publicCardClass}>Loading event…</div> : null}
        {error ? (
          <p aria-live="polite" className="rounded-3xl border border-status-danger/20 bg-status-surface-danger px-4 py-3 text-sm font-semibold text-status-danger">
            {error}
			{pendingTicketURL ? <> <a className="underline" href={pendingTicketURL}>Open ticket status</a>.</> : null}
          </p>
        ) : null}

        {!loading && !event ? <div className={publicCardClass}>Could not load event. Try again from Discover.</div> : null}

        {event ? (
          <div className="space-y-6">
            <div className="grid gap-6 lg:grid-cols-[1.1fr_0.9fr]">
              <section className={publicCardClass}>
                <p className={publicEyebrowClass}>About</p>
                <p className="mt-3 whitespace-pre-wrap text-sm leading-7 text-fg-secondary">{event.publicDescription}</p>
              </section>

              {reservation ? (
                <section className="rounded-panel border border-status-success/20 bg-status-surface-success p-5">
                  <p className={publicEyebrowClass}>Reservation confirmed</p>
                  <h2 className="mt-3 text-2xl font-bold text-fg-primary">Your ticket is ready</h2>

                  <div className="mt-5 grid gap-3 text-sm text-fg-secondary">
                    <div className="rounded-3xl bg-surface-panel p-4">
                      <p className={publicEyebrowClass}>Ticket holder</p>
                      <p className="mt-2 font-bold text-fg-primary">{reservation.displayName ?? '—'}</p>
                      <p className="mt-1 text-fg-secondary">{reservation.email}</p>
                    </div>

                    <div className="rounded-3xl bg-surface-panel p-4">
                      <p className={publicEyebrowClass}>Access code</p>
                      <div className="mt-3 flex flex-wrap gap-2 text-sm font-bold tracking-[0.2em] text-fg-primary">
                        {chunkCode(reservation.code).map((part, index) => (
                          <span key={`${part}-${index}`} className="rounded-2xl border border-stroke-subtle bg-surface-inset px-3 py-2 font-mono">
                            {part}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>

                  <a className={`${publicPrimaryButtonClass} mt-5 w-full`} href={reservation.ticketUrl}>
                    Open ticket
                  </a>

                  <p className="mt-3 text-sm text-status-success">{publicEventReservationSuccessCopy(reservation)}</p>
                </section>
              ) : (
                <form className={publicCardClass} onSubmit={handleSubmit}>
                  <p className={publicEyebrowClass}>{publicEventPrimaryCtaLabel(event, reserving)}</p>
                  <p className={`mt-2 leading-6 ${publicMutedTextClass}`}>
                    {event?.pricingMode === 'fixed'
                      ? 'Email is required for the checkout session. Display name is optional.'
                      : 'Email is required so we can send the ticket. Display name is optional.'}
                  </p>

                  <label className="mt-4 block space-y-2 text-sm font-semibold text-fg-primary">
                    <span>
                      Email <span className="text-status-danger">required</span>
                    </span>
                    <input className={publicInputClass} type="email" autoComplete="email" required value={email} onChange={(event) => { paidIntent.current = null; setEmail(event.target.value); }} disabled={event.isFull || reserving || Boolean(pendingTicketURL)} />
                  </label>

                  <label className="mt-4 block space-y-2 text-sm font-semibold text-fg-primary">
                    <span>
                      Display name <span className="text-fg-muted">optional</span>
                    </span>
                    <input className={publicInputClass} type="text" autoComplete="name" value={displayName} onChange={(event) => { paidIntent.current = null; setDisplayName(event.target.value); }} placeholder="Optional" disabled={event.isFull || reserving || Boolean(pendingTicketURL)} />
                  </label>

                  <button className={`door-action mt-4 w-full ${publicPrimaryButtonClass}`} type="submit" disabled={reserving || event.isFull || Boolean(pendingTicketURL)}>
                    {publicEventPrimaryCtaLabel(event, reserving)}
                  </button>

                  {event.isFull ? (
                    <p className="mt-3 text-sm text-status-danger">This event is sold out. {event?.pricingMode === 'fixed' ? 'Paid checkout is closed.' : 'Reservations are closed.'}</p>
                  ) : (
                    <p className="mt-3 text-sm text-fg-secondary">No account needed — just your email.</p>
                  )}
                </form>
              )}
            </div>

            <section className={publicCardClass}>
              <p className={publicEyebrowClass}>Apply to participate</p>
				<p className={`mt-2 leading-6 ${publicMutedTextClass}`}>{publicEventRoleSectionIntro(roles === null ? null : roles.length)}</p>

              {roles === null ? (
                <p className="mt-4 rounded-3xl bg-surface-inset px-4 py-3 text-sm text-fg-secondary">Loading participation roles…</p>
              ) : roles.length === 0 ? (
                <p className="mt-4 rounded-3xl bg-surface-inset px-4 py-3 text-sm text-fg-secondary">No public roles available right now.</p>
              ) : (
                <div className="mt-4 grid gap-4 lg:grid-cols-2">
                  {roles.map((role) => {
                    const draft = applicationDrafts[role.id] ?? emptyRoleApplicationDraft();

                    return (
                      <form
                        key={role.id}
                        className="rounded-3xl border border-stroke-subtle bg-surface-inset p-4"
                        onSubmit={(formEvent) => {
                          void handleRoleSubmit(role, formEvent);
                        }}
                      >
                        <div className="flex items-start justify-between gap-4">
                          <div className="min-w-0 flex-1">
                            <h3 className="text-base font-bold text-fg-primary">{role.name}</h3>
                            <p className="mt-1 whitespace-pre-wrap text-sm leading-6 text-fg-secondary">{role.description || 'No description provided.'}</p>
                          </div>
                          <span className={`${publicStatusPillClass()} shrink-0`}>{role.capacity > 0 ? `${role.capacity} spots` : 'Open'}</span>
                        </div>

                        <div className="mt-4 grid gap-3">
                          <label className="block space-y-2 text-sm font-semibold text-fg-primary">
                            <span>
                              Applicant name <span className="text-status-danger">required</span>
                            </span>
                            <input
                              className={publicInputClass}
                              type="text"
                              autoComplete="name"
                              required
                              value={draft.applicantName}
                              onChange={(event) => {
                                const value = event.target.value;
                                updateRoleDraft(role.id, (current) => ({ ...current, applicantName: value, submitted: false, error: null }));
                              }}
                              disabled={draft.submitting || draft.submitted}
                            />
                          </label>

                          <label className="block space-y-2 text-sm font-semibold text-fg-primary">
                            <span>
                              Applicant email <span className="text-status-danger">required</span>
                            </span>
                            <input
                              className={publicInputClass}
                              type="email"
                              autoComplete="email"
                              required
                              value={draft.applicantEmail}
                              onChange={(event) => {
                                const value = event.target.value;
                                updateRoleDraft(role.id, (current) => ({ ...current, applicantEmail: value, submitted: false, error: null }));
                              }}
                              disabled={draft.submitting || draft.submitted}
                            />
                          </label>

                          <label className="block space-y-2 text-sm font-semibold text-fg-primary">
                            <span>
                              Message <span className="text-fg-muted">optional</span>
                            </span>
                            <textarea
                              className={`${publicInputClass} min-h-28`}
                              value={draft.message}
                              onChange={(event) => {
                                const value = event.target.value;
                                updateRoleDraft(role.id, (current) => ({ ...current, message: value, submitted: false, error: null }));
                              }}
                              placeholder="Share relevant experience or notes."
                              disabled={draft.submitting || draft.submitted}
                            />
                          </label>
                        </div>

                        <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
                          <p className="text-xs font-bold uppercase tracking-[0.2em] text-fg-muted">Max 2000 runes</p>
                          <button className={publicSecondaryButtonClass} type="submit" disabled={draft.submitting || draft.submitted}>
                            {draft.submitted ? 'Submitted' : draft.submitting ? 'Submitting…' : 'Submit application'}
                          </button>
                        </div>

                        {draft.error ? <p className="mt-3 rounded-3xl border border-status-danger/20 bg-status-surface-danger px-4 py-3 text-sm text-status-danger">{draft.error}</p> : null}
                        {draft.submitted ? (
                          <p className="mt-3 rounded-3xl border border-status-success/20 bg-status-surface-success px-4 py-3 text-sm text-status-success">Application submitted for {role.name}. We received your interest and will follow up privately.</p>
                        ) : null}
                      </form>
                    );
                  })}
                </div>
              )}
            </section>
          </div>
        ) : null}
      </section>
    </main>
  );
}
