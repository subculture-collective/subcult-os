import { useEffect, useRef, useState } from 'react';
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

const publicInputClass =
  'w-full rounded-2xl border border-neutral-200 bg-white px-4 py-3 text-[#171717] outline-none transition placeholder:text-neutral-400 focus:border-neutral-500 focus:ring-2 focus:ring-neutral-200 disabled:cursor-not-allowed disabled:bg-neutral-100 disabled:text-neutral-500';

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
  if (!event || event.pricingMode === 'free') {
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
  const [event, setEvent] = useState<PublicEventDTO | null>(null);
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
  const latestSlug = useRef(slug);
  latestSlug.current = slug;

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setLoading(true);
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
        if (!cancelled) {
          setEvent(loaded);
        }
      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : 'Unable to load public event');
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

    updateRoleDraft(role.id, (current) => ({ ...current, submitting: true, error: null }));

    try {
      const submitted = await postJSON<EventRoleApplicationDTO>(`/api/public/events/${slug}/role-applications`, {
        roleId: role.id,
        applicantName: trimmedName,
        applicantEmail: trimmedEmail,
        message: trimmedMessage,
      });

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

    if (event?.pricingMode === 'fixed' && pendingTicketURL) return;

    const trimmedEmail = email.trim();
    const trimmedDisplayName = displayName.trim();

    if (!trimmedEmail) {
      setError('Please enter the email address where we should send the ticket.');
      return;
    }

    const requestSlug = slug;
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
        if (latestSlug.current !== requestSlug) return;
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

      if (latestSlug.current !== requestSlug) return;
      setReservation(result.ticket);
      setAvailabilityKnown(result.event !== null);
      if (result.event) setEvent(result.event);
    } catch (caught) {
      if (latestSlug.current !== requestSlug) return;
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
      if (latestSlug.current === requestSlug) setReserving(false);
    }
  }

  return (
    <main className={publicPageShellClass}>
      <section className={publicPageInnerClass}>
        <header className={publicHeroCardClass}>
          {event?.imageUrl ? <img className="h-72 w-full object-cover sm:h-96" src={event.imageUrl} alt="" /> : <div className="h-24 bg-neutral-200 sm:h-36" />}

          <div className="p-5 sm:p-7">
            <div className="flex flex-wrap items-center gap-2">
              <span className={publicStatusPillClass()}>{pricingLabel(event)}</span>
              <span className={publicStatusPillClass('success')}>No account needed</span>
              <a className={publicStatusPillClass()} href="/discover">
                Discover more events
              </a>
              {event?.pricingMode === 'fixed' ? <span className={publicStatusPillClass()}>Secure checkout</span> : null}
              <span className={publicStatusPillClass(event?.isFull ? 'danger' : 'success')}>{!availabilityKnown ? 'Availability unavailable' : event?.isFull ? 'Sold out' : `${event?.remainingTickets ?? '—'} remaining`}</span>
            </div>

            <div className="mt-5 grid gap-6 lg:grid-cols-[1.1fr_0.9fr] lg:items-end">
              <div>
                <p className={publicEyebrowClass}>{event ? pricingLabel(event) : 'Free guest reservation'}</p>
                <h1 className="mt-3 text-4xl font-black tracking-tight text-[#171717] sm:text-5xl">{event?.title ?? 'Reserve your free ticket'}</h1>
                <p className="mt-3 max-w-2xl text-base leading-7 text-neutral-600">{availabilityKnown ? publicEventConversionSummary(event, pricingLabel(event)) : 'Your ticket is reserved. Current availability could not be refreshed.'}</p>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <div className="rounded-3xl bg-[#f5f5f5] p-4">
                  <p className={publicEyebrowClass}>Date & time</p>
                  <p className="mt-2 text-sm font-bold text-[#171717]">{event ? formatDateTime(event.startsAt) : 'Loading event…'}</p>
                </div>
                <div className="rounded-3xl bg-[#f5f5f5] p-4">
                  <p className={publicEyebrowClass}>Location</p>
                  <p className="mt-2 text-sm font-bold text-[#171717]">{event?.locationDisplay ?? 'Loading location…'}</p>
                </div>
                <div className="rounded-3xl bg-[#f5f5f5] p-4">
                  <p className={publicEyebrowClass}>Tickets</p>
                  <p className={`mt-2 text-sm font-bold ${event?.isFull ? 'text-rose-700' : 'text-[#171717]'}`}>{!availabilityKnown ? 'Availability unavailable' : event ? (event.isFull ? 'Sold out' : `${event.remainingTickets} left`) : 'Loading availability…'}</p>
                </div>
                <div className="rounded-3xl bg-[#f5f5f5] p-4">
                  <p className={publicEyebrowClass}>Pricing</p>
                  <p className="mt-2 text-sm font-bold text-[#171717]">{event ? pricingLabel(event) : 'Loading pricing…'}</p>
                </div>
              </div>
            </div>
          </div>
        </header>

        {loading ? <div className={publicCardClass}>Loading event…</div> : null}
        {error ? (
          <p aria-live="polite" className="rounded-3xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-semibold text-rose-700">
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
                <p className="mt-3 whitespace-pre-wrap text-sm leading-7 text-neutral-700">{event.publicDescription}</p>
              </section>

              {reservation ? (
                <section className="rounded-[28px] border border-emerald-200 bg-emerald-50 p-5 shadow-sm">
                  <p className={publicEyebrowClass}>Reservation confirmed</p>
                  <h2 className="mt-3 text-2xl font-black text-[#171717]">Your ticket is ready</h2>

                  <div className="mt-5 grid gap-3 text-sm text-neutral-700">
                    <div className="rounded-3xl bg-white p-4">
                      <p className={publicEyebrowClass}>Ticket holder</p>
                      <p className="mt-2 font-bold text-[#171717]">{reservation.displayName ?? '—'}</p>
                      <p className="mt-1 text-neutral-600">{reservation.email}</p>
                    </div>

                    <div className="rounded-3xl bg-white p-4">
                      <p className={publicEyebrowClass}>Access code</p>
                      <div className="mt-3 flex flex-wrap gap-2 text-sm font-bold tracking-[0.35em] text-[#171717]">
                        {chunkCode(reservation.code).map((part, index) => (
                          <span key={`${part}-${index}`} className="rounded-2xl border border-neutral-200 bg-[#f5f5f5] px-3 py-2 font-mono">
                            {part}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>

                  <a className={`${publicPrimaryButtonClass} mt-5 w-full`} href={reservation.ticketUrl}>
                    Open ticket
                  </a>

                  <p className="mt-3 text-sm text-emerald-800">{publicEventReservationSuccessCopy(reservation)}</p>
                </section>
              ) : (
                <form className={publicCardClass} onSubmit={handleSubmit}>
                  <p className={publicEyebrowClass}>{publicEventPrimaryCtaLabel(event, reserving)}</p>
                  <p className={`mt-2 leading-6 ${publicMutedTextClass}`}>
                    {event?.pricingMode === 'fixed'
                      ? 'Email is required for the checkout session. Display name is optional.'
                      : 'Email is required so we can send the ticket. Display name is optional.'}
                  </p>

                  <label className="mt-4 block space-y-2 text-sm font-semibold text-[#171717]">
                    <span>
                      Email <span className="text-rose-600">required</span>
                    </span>
                    <input className={publicInputClass} type="email" autoComplete="email" required value={email} onChange={(event) => { paidIntent.current = null; setEmail(event.target.value); }} disabled={event.isFull || reserving || Boolean(pendingTicketURL)} />
                  </label>

                  <label className="mt-4 block space-y-2 text-sm font-semibold text-[#171717]">
                    <span>
                      Display name <span className="text-neutral-500">optional</span>
                    </span>
                    <input className={publicInputClass} type="text" autoComplete="name" value={displayName} onChange={(event) => { paidIntent.current = null; setDisplayName(event.target.value); }} placeholder="Optional" disabled={event.isFull || reserving || Boolean(pendingTicketURL)} />
                  </label>

                  <button className={`door-action mt-4 w-full ${publicPrimaryButtonClass}`} type="submit" disabled={reserving || event.isFull || Boolean(pendingTicketURL)}>
                    {publicEventPrimaryCtaLabel(event, reserving)}
                  </button>

                  {event.isFull ? (
                    <p className="mt-3 text-sm text-rose-700">This event is sold out. {event?.pricingMode === 'fixed' ? 'Paid checkout is closed.' : 'Reservations are closed.'}</p>
                  ) : (
                    <p className="mt-3 text-sm text-neutral-600">No account needed — just your email.</p>
                  )}
                </form>
              )}
            </div>

            <section className={publicCardClass}>
              <p className={publicEyebrowClass}>Apply to participate</p>
				<p className={`mt-2 leading-6 ${publicMutedTextClass}`}>{publicEventRoleSectionIntro(roles === null ? null : roles.length)}</p>

              {roles === null ? (
                <p className="mt-4 rounded-3xl bg-[#f5f5f5] px-4 py-3 text-sm text-neutral-600">Loading participation roles…</p>
              ) : roles.length === 0 ? (
                <p className="mt-4 rounded-3xl bg-[#f5f5f5] px-4 py-3 text-sm text-neutral-600">No public roles available right now.</p>
              ) : (
                <div className="mt-4 grid gap-4 lg:grid-cols-2">
                  {roles.map((role) => {
                    const draft = applicationDrafts[role.id] ?? emptyRoleApplicationDraft();

                    return (
                      <form
                        key={role.id}
                        className="rounded-3xl border border-neutral-200 bg-[#f5f5f5] p-4"
                        onSubmit={(formEvent) => {
                          void handleRoleSubmit(role, formEvent);
                        }}
                      >
                        <div className="flex items-start justify-between gap-4">
                          <div>
                            <h3 className="text-base font-black text-[#171717]">{role.name}</h3>
                            <p className="mt-1 whitespace-pre-wrap text-sm leading-6 text-neutral-600">{role.description || 'No description provided.'}</p>
                          </div>
                          <span className={publicStatusPillClass()}>{role.capacity > 0 ? `${role.capacity} spots` : 'Open'}</span>
                        </div>

                        <div className="mt-4 grid gap-3">
                          <label className="block space-y-2 text-sm font-semibold text-[#171717]">
                            <span>
                              Applicant name <span className="text-rose-600">required</span>
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

                          <label className="block space-y-2 text-sm font-semibold text-[#171717]">
                            <span>
                              Applicant email <span className="text-rose-600">required</span>
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

                          <label className="block space-y-2 text-sm font-semibold text-[#171717]">
                            <span>
                              Message <span className="text-neutral-500">optional</span>
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
                          <p className="text-xs font-bold uppercase tracking-[0.25em] text-neutral-500">Max 2000 runes</p>
                          <button className={publicSecondaryButtonClass} type="submit" disabled={draft.submitting || draft.submitted}>
                            {draft.submitted ? 'Submitted' : draft.submitting ? 'Submitting…' : 'Submit application'}
                          </button>
                        </div>

                        {draft.error ? <p className="mt-3 rounded-3xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{draft.error}</p> : null}
                        {draft.submitted ? (
                          <p className="mt-3 rounded-3xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800">Application submitted for {role.name}. We received your interest and will follow up privately.</p>
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
