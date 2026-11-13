// Invoked by dev-env.sh with the current worktree's loopback URL and DB container.
// Uses normal signup/verification APIs; only reads synthetic users' held mail.
// Safe to re-run: existing people, venues, acts and events (by title) are kept.
import { execFileSync } from 'node:child_process';
import { ACTS, EVENTS, PASSWORD, STAFF, VENUES, guestName, photoURL, posterDataURI } from './dev-seed-catalog.mjs';

const [origin, databaseContainer, project] = process.argv.slice(2);
const parsed = new URL(origin);
if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1'
    || parsed.username || parsed.password || parsed.pathname !== '/'
    || !/^subcult-dev-[a-f0-9]{16}$/.test(project)) {
  throw new Error('Seed requires the worktree development environment.');
}
const [container] = JSON.parse(execFileSync('docker', ['inspect', databaseContainer], { encoding: 'utf8' }));
if (container.Config.Labels['com.docker.compose.project'] !== project
    || container.Config.Labels['com.docker.compose.service'] !== 'postgres'
    || !container.Config.Env.includes('POSTGRES_DB=dev')) {
  throw new Error('Database does not belong to this development environment.');
}

class Session {
  cookies = new Map();

  async request(method, path, body, allowed = []) {
    const response = await fetch(`${origin}${path}`, {
      method,
      headers: { 'Content-Type': 'application/json', Cookie: [...this.cookies].map(([k, v]) => `${k}=${v}`).join('; ') },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(15000),
    });
    for (const cookie of response.headers.getSetCookie()) {
      const pair = cookie.split(';', 1)[0];
      const split = pair.indexOf('=');
      this.cookies.set(pair.slice(0, split), pair.slice(split + 1));
    }
    const text = await response.text();
    if (!response.ok && !allowed.includes(response.status)) {
      throw new Error(`${method} ${path} returned ${response.status}: ${text.slice(0, 200)}`);
    }
    return { status: response.status, data: text ? JSON.parse(text) : null };
  }

  get(path, allowed) { return this.request('GET', path, undefined, allowed); }
  post(path, body = {}, allowed) { return this.request('POST', path, body, allowed); }
  patch(path, body, allowed) { return this.request('PATCH', path, body, allowed); }
}

// Runs SQL against this environment's dev database. Values are passed as psql
// variables, never interpolated into the SQL text.
function psql(sql, vars = {}) {
  const args = ['exec', '-i', databaseContainer, 'psql', '-U', 'dev', '-d', 'dev', '-XAqt', '-v', 'ON_ERROR_STOP=1'];
  for (const [name, value] of Object.entries(vars)) args.push('-v', `${name}=${value}`);
  return execFileSync('docker', args, { encoding: 'utf8', input: sql });
}

// Signs up (or reuses) a synthetic account, completing verification from the
// held outbox row, and returns a signed-in session.
async function account(email, displayName) {
  const session = new Session();
  const signup = await session.post('/api/auth/signup', { email, password: PASSWORD, displayName }, [409]);
  // Also recovers an interrupted seed after signup but before verification.
  const message = psql(`select o.body from email_outbox o
      join identity_challenges c on c.id = o.related_id
      where o.recipient_email = :'email'
        and o.related_type = 'identity_verification' and o.delivery_status = 'held'
        and c.consumed_at is null and c.expires_at > now()
      order by o.created_at desc limit 1;`, { email });
  if (message.trim()) {
    const link = message.split(/\s+/).find(part => part.startsWith(`${origin}/verify-email?`));
    if (!link) throw new Error('Expected a verification link for this dev origin.');
    await session.post('/api/auth/verify-email', { token: new URL(link).searchParams.get('token') });
  } else if (signup.status !== 409) {
    throw new Error(`Expected a held verification message for ${email}.`);
  }
  await session.post('/api/auth/login', { email, password: PASSWORD });
  return session;
}

// Runs fn over items with a few requests in flight.
async function each(items, fn, limit = 8) {
  let next = 0;
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (next < items.length) {
      const index = next++;
      await fn(items[index], index);
    }
  }));
}

const iso = ms => new Date(ms).toISOString();
const HOUR = 3600000;
const DAY = 24 * HOUR;

// ---- Owner and workspace -------------------------------------------------

const ownerEmail = 'dev@example.test';
const owner = await account(ownerEmail, 'Dev Operator');
let workspace = await owner.get('/api/workspaces/current', [404]);
if (workspace.status === 404) {
  await owner.post('/api/workspaces', { name: 'Development Collective' });
  workspace = await owner.get('/api/workspaces/current');
}
const workspaceID = workspace.data.id;
const ws = `/api/workspaces/${workspaceID}`;

// ---- Staff: invited, accepted, given a role -----------------------------

const people = {};
for (const person of STAFF) {
  const current = (await owner.get('/api/workspaces/current')).data;
  const member = current.members.find(m => m.email === person.email);
  if (member) {
    people[person.key] = { ...person, personID: member.personId };
    continue;
  }
  if (current.invitations.some(i => i.email === person.email && !i.acceptedAt)) continue;
  const invitationRole = person.role === 'participant' ? 'participant' : 'crew';
  const session = await account(person.email, person.name);
  const invitation = await owner.post(`${ws}/invitations`, { email: person.email, role: invitationRole });
  if (person.pendingOnly) continue;
  await session.post(`/api/invitations/${invitation.data.token}/accept`);
  const joined = (await owner.get('/api/workspaces/current')).data.members.find(m => m.email === person.email);
  const change = {};
  if (person.role !== invitationRole) change.role = person.role;
  if (person.expiresInDays) change.expiresAt = iso(Date.now() + person.expiresInDays * DAY).replace(/\.\d+Z$/, 'Z');
  if (Object.keys(change).length) await owner.patch(`${ws}/members/${joined.id}`, change);
  if (person.revoke) await owner.post(`${ws}/members/${joined.id}/revoke`);
  people[person.key] = { ...person, personID: joined.personId };
}
const active = key => (people[key] && !people[key].revoke ? people[key].personID : undefined);
const doorSessions = [];
for (const key of ['theo', 'jun']) {
  if (people[key]) doorSessions.push(await account(people[key].email, people[key].name));
}
if (!doorSessions.length) doorSessions.push(owner);

// ---- Venues and acts ------------------------------------------------------

const places = {};
const existingPlaces = (await owner.get(`${ws}/places`)).data ?? [];
for (const [key, venue] of Object.entries(VENUES)) {
  const found = existingPlaces.find(p => p.name === venue.name);
  places[key] = found ? found.id : (await owner.post(`${ws}/places`, { ...venue, coordinatesPublic: false })).data.id;
}
const acts = {};
const existingProfiles = (await owner.get(`${ws}/profiles`)).data ?? [];
for (const act of ACTS) {
  const found = existingProfiles.find(p => p.displayName === act.displayName);
  acts[act.key] = { kind: act.kind, id: found ? found.id : (await owner.post(`${ws}/profiles`, { kind: act.kind, displayName: act.displayName, description: act.description })).data.id };
}

// ---- Events ---------------------------------------------------------------

function startTime(spec) {
  const now = new Date();
  if (typeof spec.hour === 'string') {
    const offset = Number(spec.hour.slice(3));
    return Math.floor((now.getTime() + offset * HOUR) / (15 * 60000)) * 15 * 60000;
  }
  // Hours are wall-clock in the host's time zone, so a 9pm show reads 9pm.
  return new Date(now.getFullYear(), now.getMonth(), now.getDate() + spec.day, spec.hour).getTime();
}

const crewKeys = ['sasha', 'dev', 'lou', 'pat'];
const participantKeys = ['ama', 'kit', 'ola'];

async function seedEvent(spec, index) {
  const starts = startTime(spec);
  const ends = starts + spec.hours * HOUR;
  const location = spec.venue ? VENUES[spec.venue].name : spec.location;
  const imageUrl = spec.art === 'photo' ? photoURL(index)
    : spec.art === 'poster' ? posterDataURI(spec.title, starts, location) : '';
  const price = spec.price ?? 0;
  const event = (await owner.post(`${ws}/events`, {
    title: spec.title,
    publicDescription: spec.description,
    locationDisplay: location,
    startsAt: iso(starts),
    endsAt: iso(ends),
    imageUrl,
    ticketAllocation: spec.alloc,
    pricingMode: price ? 'fixed' : 'free',
    ticketPriceCents: price,
    ticketCurrency: 'usd',
  })).data;
  const ev = `/api/events/${event.id}`;
  const past = ends < Date.now();
  const live = spec.state !== 'draft';

  // Occurrences carry the venue, credits and any postponed, rescheduled or
  // cancelled dates. Drafts have none yet.
  if (live) {
    const statuses = spec.occurrences ?? ['scheduled'];
    for (const [i, status] of statuses.entries()) {
      const occurrence = (await owner.post(`${ev}/occurrences`, {
        name: statuses.length > 1 ? `${spec.title} (${i + 1} of ${statuses.length})` : spec.title,
        description: spec.description,
        startsAt: iso(starts + i * 7 * DAY),
        endsAt: iso(ends + i * 7 * DAY),
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        status,
        ...(spec.venue ? { placeId: places[spec.venue] } : {}),
      })).data;
      for (const [order, key] of spec.acts.entries()) {
        const act = acts[key];
        await owner.post(`${ev}/occurrences/${occurrence.id}/credits`, {
          profileId: act.id, role: act.kind === 'collective' ? 'collective' : order === 0 ? 'host' : 'performer', sortOrder: order,
        });
      }
    }
  }

  // Roles and applications. Applications need the public slug, so they come
  // after publishing.
  let roles = null;
  if (spec.roles) {
    roles = {
      volunteer: (await owner.post(`${ev}/roles`, { name: 'Volunteer door host', description: 'Greet guests, check bags, hand out ear plugs.', capacity: 3, public: true })).data.id,
      support: (await owner.post(`${ev}/roles`, { name: 'Open-call support act', description: 'One 25-minute opening slot for a local act.', capacity: 1, public: true })).data.id,
      stagehand: (await owner.post(`${ev}/roles`, { name: 'Stagehand', description: 'Crew only: changeovers and load-out.', capacity: 2, public: false })).data.id,
    };
  }

  if (live) {
    const published = (await owner.post(`${ev}/publish`)).data;
    event.publicSlug = published.publicSlug;
  }

  let volunteerApplication;
  if (roles && live && !past) {
    const outcomes = ['submitted', 'under_review', 'accepted', 'waitlisted', 'rejected'];
    for (const [i, status] of outcomes.entries()) {
      const name = guestName(index * 7 + i + 500);
      const application = (await owner.post(`/api/public/events/${event.publicSlug}/role-applications`, {
        roleId: i === 4 ? roles.support : roles.volunteer,
        applicantName: name,
        applicantEmail: `applicant-${index}-${i}@example.test`,
        message: i === 4 ? 'We are a three-piece and can bring our own backline.' : 'Happy to help on the door.',
      })).data;
      if (status !== 'submitted') {
        await owner.patch(`${ev}/role-applications/${application.id}`, { status });
      }
      if (status === 'accepted') volunteerApplication = application.id;
    }
  }

  // Run of show and crew assignments.
  const items = [];
  const add = async (item, assign) => {
    const created = (await owner.post(`${ev}/staffing`, {
      title: item.title, kind: item.kind, notes: item.notes ?? '',
      startsAt: item.startsAt ? iso(item.startsAt) : null, endsAt: item.endsAt ? iso(item.endsAt) : null,
    })).data;
    if (assign && spec.state !== 'draft') await owner.patch(`${ev}/staffing/${created.id}`, assign);
    items.push({ id: created.id, assigned: Boolean(assign && spec.state !== 'draft'), title: item.title });
  };
  const crew = crewKeys[index % crewKeys.length];
  const door = index % 2 ? 'theo' : 'jun';
  const person = key => (active(key) ? { assignedPersonId: active(key) } : undefined);
  if (spec.crew === 'full') {
    await add({ title: 'Load-in and sound check', kind: 'shift', startsAt: starts - 3 * HOUR, endsAt: starts, notes: 'Backline arrives two hours before doors.' }, person(crew));
    await add({ title: 'Door and guest list', kind: 'shift', startsAt: starts - HOUR / 2, endsAt: starts + 3 * HOUR, notes: 'Scanner, float and the printed guest list.' }, person(door));
    await add({ title: 'Runner', kind: 'shift', startsAt: starts - HOUR, endsAt: ends, notes: 'Water for the stage, messages for the bar.' }, person(participantKeys[index % participantKeys.length]));
    await add({ title: 'Print set times and signage', kind: 'task', notes: 'Set times by the stage and the door.' }, person('rae'));
    await add({ title: 'Settle with venue', kind: 'task', notes: 'Bar split and door count, signed by both sides.' }, person('mina'));
    await add({ title: 'Load-out', kind: 'shift', startsAt: ends, endsAt: ends + 1.5 * HOUR, notes: 'Everyone still on site helps.' });
    if (volunteerApplication) {
      await add({ title: 'Volunteer briefing', kind: 'task', notes: 'Walk the accepted volunteer through the door.' }, { assignedApplicationId: volunteerApplication });
    }
  } else if (spec.crew === 'light') {
    await add({ title: 'Door and guest list', kind: 'shift', startsAt: starts - HOUR / 2, endsAt: starts + 2 * HOUR }, person(door));
    await add({ title: 'Print set times and signage', kind: 'task' }, person('rae'));
    await add({ title: 'Settle with venue', kind: 'task' }, person('mina'));
  }
  if (roles && live && !past) {
    const merch = (await owner.post(`${ev}/staffing`, { title: 'Merch table', kind: 'shift', startsAt: iso(starts), endsAt: iso(ends), notes: 'Card reader is in the cash box.' })).data;
    await owner.patch(`${ev}/staffing/${merch.id}`, { capacity: 2, claimsOpen: true, claimRoleIds: [roles.volunteer] });
  }

  // Tickets: free events take public reservations; paid events need Stripe
  // checkout, so they only get an organizer test ticket.
  const codes = [];
  if (live && price) {
    codes.push((await owner.post(`${ev}/test-ticket`)).data.code);
  } else if (live) {
    const count = Math.floor(spec.alloc * (spec.fill ?? 0));
    const guests = Array.from({ length: count }, (_, i) => i);
    await each(guests, async i => {
      const ticket = (await new Session().post(`/api/public/events/${event.publicSlug}/reservations`, {
        email: `guest-${index}-${i}@example.test`, displayName: guestName(index * 31 + i),
      })).data;
      codes.push(ticket.code);
    });
  }

  // Door check-ins by door staff for events that have started.
  if (live && starts < Date.now() && spec.checkIn) {
    const door = doorSessions[index % doorSessions.length];
    await each(codes.slice(0, Math.floor(codes.length * spec.checkIn)), code => door.post(`${ev}/door/check-ins`, { code }));
  }

  // Staffing outcomes follow the event's state.
  const cancelledOccurrence = spec.occurrences?.length === 1 && spec.occurrences[0] === 'cancelled';
  for (const item of items) {
    let status;
    if (cancelledOccurrence) status = 'cancelled';
    else if (spec.state === 'closed') status = item.assigned ? 'completed' : 'cancelled';
    else if (past) status = item.assigned && item.title !== 'Settle with venue' ? 'completed' : undefined;
    else if (starts < Date.now() && item.title === 'Load-in and sound check') status = 'completed';
    else if (index % 3 === 0 && item.title === 'Print set times and signage' && item.assigned) status = 'completed';
    if (status) await owner.patch(`${ev}/staffing/${item.id}`, { status });
  }

  if (spec.state === 'closed') await owner.post(`${ev}/end-of-night`);
  return { id: event.id, title: spec.title, state: spec.state, tickets: codes.length };
}

// An event counts as seeded only once every step has finished. The record
// lives in its own schema in the dev database, outside the application's
// migrations, so it survives reruns and disappears with `down`.
psql(`set client_min_messages = warning;
  create schema if not exists dev_seed;
  create table if not exists dev_seed.completed_events (
    event_id uuid primary key, title text not null, completed_at timestamptz not null default now());`);
const completed = new Set(psql('select title from dev_seed.completed_events;').split('\n').filter(Boolean));
const listed = (await owner.get(`${ws}/events`)).data;
const existing = new Set(listed.map(event => event.title));
// A catalog event that exists without a completion record was interrupted.
// Its title is released (drafts only; published titles are locked) and the
// event is seeded again from the start.
for (const event of listed) {
  if (completed.has(event.title) || !EVENTS.some(spec => spec.title === event.title)) continue;
  const released = event.status === 'draft'
    && (await owner.patch(`/api/events/${event.id}`, { title: `${event.title} (interrupted seed)` }, [400, 409])).status === 200;
  console.warn(released
    ? `Renamed interrupted seed event "${event.title}" and seeding it again.`
    : `"${event.title}" (${event.status}) was interrupted mid-seed and is left as is; seeding a complete copy.`);
  existing.delete(event.title);
}
// The original single development event stays for older notes and tests.
if (!existing.has('Development Night')) {
  await owner.post(`${ws}/events`, {
    title: 'Development Night',
    startsAt: iso(Date.now() + 7 * DAY),
    publicDescription: 'Synthetic event for local development.',
    locationDisplay: 'Development Venue',
    ticketAllocation: 100,
  });
}
const pending = EVENTS.map((spec, index) => ({ spec, index })).filter(({ spec }) => !existing.has(spec.title));
const results = [];
await each(pending, async ({ spec, index }) => {
  const result = await seedEvent(spec, index);
  psql('insert into dev_seed.completed_events (event_id, title) values (:\'id\', :\'title\');', { id: result.id, title: result.title });
  results.push(result);
  process.stdout.write(`\rEvents seeded: ${results.length}/${pending.length}`);
}, 1);
if (pending.length) process.stdout.write('\n');

const byState = results.reduce((counts, r) => ({ ...counts, [r.state]: (counts[r.state] ?? 0) + 1 }), {});
console.log(`Created ${results.length} events (${Object.entries(byState).map(([k, v]) => `${v} ${k}`).join(', ') || 'none new'}), ${results.reduce((n, r) => n + r.tickets, 0)} tickets.`);
console.log(`Development data ready. Sign in at ${origin}/login
Owner: ${ownerEmail}
Staff: ${STAFF.filter(p => !p.pendingOnly && !p.revoke).map(p => `${p.email} (${p.role})`).join(', ')}
Password for every account: ${PASSWORD}`);
