// Synthetic catalog for scripts/dev-seed.mjs. Every person, venue and act is
// invented; every address uses the reserved example.test domain.

// Concert, club and gallery photos hotlinked from Unsplash (free to use under
// the Unsplash License). Each ID was checked to resolve on 2026-11-12.
const PHOTO_IDS = [
  '1429962714451-bb934ecdc4ec', '1459749411175-04bf5292ceea', '1460723237483-7a6dc9d0b212',
  '1470229722913-7c0e2dbbafd3', '1485872299829-c673f5194813', '1487180144351-b8472da7d491',
  '1493225457124-a3eb161ffa5f', '1496337589254-7e19d01cec44', '1499364615650-ec38552f4f34',
  '1501281668745-f7f57925c3b4', '1501386761578-eac5c94b800a', '1504680177321-2e6a879aac86',
  '1506157786151-b8491531f063', '1508700115892-45ecd05ae2ad', '1511671782779-c97d3d27a1d4',
  '1514320291840-2e0a9bf2a9ae', '1514525253161-7a46d19cd819', '1516450360452-9312f5e86fc7',
  '1519892300165-cb5542fb47c7', '1524368535928-5b5e00ddc76b', '1533174072545-7a4b6ad7a6c3',
  '1540039155733-5bb30b53aa14', '1545128485-c400e7702796', '1563841930606-67e2bce48b78',
  '1574391884720-bbc3740c59d1', '1578736641330-3155e606cd40', '1598387993441-a364f854c3e1',
];

export function photoURL(index) {
  const id = PHOTO_IDS[index % PHOTO_IDS.length];
  return `https://images.unsplash.com/photo-${id}?auto=format&fit=crop&w=1600&h=900&q=80`;
}

export const PASSWORD = 'local-development-only';

// Accepted members get the listed role. `expiresInDays` grants temporary
// access; `revoke` removes the member after joining; `pendingOnly` leaves the
// invitation unaccepted.
export const STAFF = [
  { key: 'rae', name: 'Rae Okafor', role: 'organizer' },
  { key: 'mina', name: 'Mina Castellanos', role: 'finance' },
  { key: 'theo', name: 'Theo Lindqvist', role: 'door' },
  { key: 'jun', name: 'Jun Park', role: 'door' },
  { key: 'sasha', name: 'Sasha Ivanova', role: 'crew' },
  { key: 'dev', name: 'Devika Mehta', role: 'crew' },
  { key: 'lou', name: 'Lou Brennan', role: 'crew' },
  { key: 'pat', name: 'Pat Quinn', role: 'crew', expiresInDays: 30 },
  { key: 'ama', name: 'Ama Mensah', role: 'participant' },
  { key: 'kit', name: 'Kit Navarro', role: 'participant' },
  { key: 'ola', name: 'Ola Bergstrom', role: 'participant' },
  { key: 'remy', name: 'Remy Duval', role: 'crew', revoke: true },
  { key: 'noor', name: 'Noor Haddad', role: 'crew', pendingOnly: true },
].map(person => ({ ...person, email: `${person.key}.staff@example.test` }));

export const VENUES = {
  coil: { name: 'The Coil Room', locality: 'Eastside', region: 'Riverton', streetAddress: '41 Foundry Lane', accessNotes: 'Step-free entrance on the side alley. Accessible toilet by the bar.' },
  dock: { name: 'Dock 9 Warehouse', locality: 'Harbour District', region: 'Riverton', streetAddress: 'Unit 9, Wharf Road', accessNotes: 'Concrete floor, no seating. Ear plugs at the door.' },
  marrow: { name: 'Marrow Gallery', locality: 'Old Town', region: 'Riverton', streetAddress: '7 Tanner Street', accessNotes: 'Two steps at the entrance; ramp on request.' },
  brigid: { name: "St. Brigid's Hall", locality: 'Northfield', region: 'Riverton', streetAddress: '120 Chapel Row', accessNotes: 'Seated hall with a hearing loop. Lift to the upper room.' },
  lantern: { name: 'Lantern Rooftop', locality: 'Midtown', region: 'Riverton', streetAddress: '15th floor, 88 Exchange Place', accessNotes: 'Lift access. Weather call made at noon on the day.' },
  spindle: { name: 'Spindle Records (back room)', locality: 'Eastside', region: 'Riverton', streetAddress: '3 Market Arcade', accessNotes: 'Capacity 40. Narrow corridor through the shop.' },
};

export const ACTS = [
  { key: 'velvet', kind: 'act', displayName: 'Velvet Static', description: 'Four-piece shoegaze band.' },
  { key: 'moth', kind: 'act', displayName: 'Moth Choir', description: 'A cappella drone ensemble.' },
  { key: 'kilo', kind: 'creator', displayName: 'DJ Kilowatt', description: 'Electro and breaks selector.' },
  { key: 'hex', kind: 'act', displayName: 'Hex Garden', description: 'Synth-punk duo.' },
  { key: 'tide', kind: 'creator', displayName: 'Saoirse Tide', description: 'Songwriter, looping violin.' },
  { key: 'brass', kind: 'act', displayName: 'Low Brass Union', description: 'Seven-piece street brass band.' },
  { key: 'paper', kind: 'collective', displayName: 'Paper Lung Press', description: 'Risograph print collective.' },
  { key: 'orbit', kind: 'creator', displayName: 'Orbit Kane', description: 'Ambient and modular synth.' },
  { key: 'gutter', kind: 'act', displayName: 'Gutter Saints', description: 'Hardcore band.' },
  { key: 'yuki', kind: 'creator', displayName: 'Yuki Arai', description: 'Poet and spoken-word host.' },
  { key: 'night', kind: 'collective', displayName: 'Night Shift Sound System', description: 'Dub and soundsystem crew.' },
  { key: 'fern', kind: 'act', displayName: 'Fern & The Fuses', description: 'Garage-pop four-piece.' },
];

const FIRST = ['Alex', 'Bea', 'Cass', 'Dara', 'Eli', 'Fen', 'Gus', 'Hana', 'Ira', 'Jo', 'Kai', 'Lia', 'Max', 'Nia', 'Oz', 'Pia', 'Quin', 'Ren', 'Sol', 'Tam', 'Uma', 'Vic', 'Wren', 'Yara', 'Zed'];
const LAST = ['Abara', 'Bloom', 'Cruz', 'Dube', 'Eriksen', 'Faro', 'Gale', 'Huang', 'Ito', 'Joshi', 'Kowal', 'Lund', 'Mora', 'Nakai', 'Okoro', 'Price', 'Reyes', 'Silva', 'Tran', 'Varga'];

export function guestName(n) {
  return `${FIRST[n % FIRST.length]} ${LAST[Math.floor(n / FIRST.length) % LAST.length]}`;
}

// Each event names its schedule relative to now (`day`, local `hour`, `hours`
// long), its lifecycle `state` (draft, published, closed = end of night),
// ticketing, artwork (photo, poster or none), crew depth, an optional public
// call for roles and optional occurrence statuses beyond one scheduled date.
// `fill` is the share of tickets reserved; `checkIn` the share of those
// scanned at the door.
const E = (title, description, spec) => ({ title, description, ...spec });

export const EVENTS = [
  // Closed: published, run and wrapped up with end of night.
  E('Basement Frequencies 017', 'Monthly electro and breaks night in the Coil Room basement.', { day: -118, hour: 21, hours: 5, venue: 'coil', state: 'closed', alloc: 180, fill: 0.95, checkIn: 0.88, art: 'photo', crew: 'full', acts: ['kilo', 'night'] }),
  E('Risograph Zine Swap', 'Bring zines, leave with zines. Paper Lung Press runs the print corner.', { day: -111, hour: 13, hours: 5, venue: 'marrow', state: 'closed', alloc: 60, fill: 0.7, checkIn: 0.9, art: 'poster', crew: 'light', acts: ['paper'] }),
  E('Velvet Static Album Launch', 'Velvet Static play the new record front to back.', { day: -104, hour: 20, hours: 4, venue: 'dock', state: 'closed', alloc: 300, fill: 1, checkIn: 0.93, art: 'photo', crew: 'full', acts: ['velvet', 'fern'] }),
  E('Moth Choir: Long Tones', 'A seated evening of slow, close-harmony drone.', { day: -97, hour: 19, hours: 3, venue: 'brigid', state: 'closed', alloc: 90, fill: 0.6, checkIn: 0.8, art: 'poster', crew: 'light', acts: ['moth'] }),
  E('Hardcore Matinee', 'All-ages afternoon show. Doors 2pm, done by 6.', { day: -90, hour: 14, hours: 4, venue: 'coil', state: 'closed', alloc: 150, fill: 0.85, checkIn: 0.7, art: 'photo', crew: 'full', acts: ['gutter', 'hex'] }),
  E('Spoken Word Open Mic', 'Five-minute slots, sign up on the night. Hosted by Yuki Arai.', { day: -83, hour: 19, hours: 3, venue: 'spindle', state: 'closed', alloc: 40, fill: 0.9, checkIn: 0.95, art: 'poster', crew: 'light', acts: ['yuki'] }),
  E('Dub Soundsystem Session', 'Night Shift bring the full stack. Bass you can feel.', { day: -76, hour: 22, hours: 5, venue: 'dock', state: 'closed', alloc: 250, fill: 0.75, checkIn: 0.82, art: 'photo', crew: 'full', acts: ['night'] }),
  E('Rooftop Ambient Sunset', 'Orbit Kane plays modular synth as the sun goes down.', { day: -69, hour: 18, hours: 3, venue: 'lantern', state: 'closed', alloc: 80, fill: 1, checkIn: 0.97, art: 'photo', crew: 'light', acts: ['orbit'] }),
  E('Print Fair Afterparty', 'Drinks and records after the print fair closes.', { day: -62, hour: 20, hours: 4, venue: 'marrow', state: 'closed', alloc: 70, fill: 0.5, checkIn: 0.6, art: 'none', crew: 'light', acts: ['paper', 'kilo'] }),
  E('Garage Pop Triple Bill', 'Three short, loud sets.', { day: -55, hour: 20, hours: 4, venue: 'coil', state: 'closed', alloc: 160, fill: 0.8, checkIn: 0.85, art: 'photo', crew: 'full', acts: ['fern', 'hex', 'velvet'] }),
  E('Brass Band Street Party', 'Low Brass Union lead a parade from the hall to the car park.', { day: -48, hour: 15, hours: 4, venue: 'brigid', state: 'closed', alloc: 200, fill: 0.65, checkIn: 0.75, art: 'photo', crew: 'full', acts: ['brass'] }),
  E('Listening Party: Night Signals', 'A new record played once, loud, in the dark. No phones.', { day: -41, hour: 20, hours: 2, venue: 'spindle', state: 'closed', alloc: 40, fill: 1, checkIn: 1, art: 'photo', crew: 'light', acts: ['orbit'] }),
  E('Synth Punk Night', 'Hex Garden headline with two local openers.', { day: -34, hour: 21, hours: 4, venue: 'coil', state: 'closed', alloc: 170, fill: 0.9, checkIn: 0.86, art: 'poster', crew: 'full', acts: ['hex', 'gutter'] }),
  E('Violin Loops & Lanterns', 'Saoirse Tide plays two sets on the roof.', { day: -27, hour: 19, hours: 3, venue: 'lantern', state: 'closed', alloc: 80, fill: 0.55, checkIn: 0.9, art: 'photo', crew: 'light', acts: ['tide'] }),
  E('Members-Only Rehearsal Social', 'Thank-you night for crew and volunteers. Free bar until it runs out.', { day: -20, hour: 19, hours: 4, venue: 'spindle', state: 'closed', alloc: 40, fill: 0.8, checkIn: 0.7, art: 'none', crew: 'light', acts: [] }),
  E('Basement Frequencies 018', 'Monthly electro and breaks night, now in the bigger room.', { day: -13, hour: 21, hours: 5, venue: 'coil', state: 'closed', alloc: 200, fill: 0.88, checkIn: 0.81, art: 'photo', crew: 'full', acts: ['kilo'] }),

  // Past and published, but nobody has run end of night yet.
  E('Noise Tuesday', 'Harsh noise and feedback. Ear plugs provided.', { day: -9, hour: 20, hours: 3, venue: 'spindle', state: 'published', alloc: 40, fill: 0.75, checkIn: 0.8, art: 'poster', crew: 'light', acts: ['hex'] }),
  E('Folk in the Hall', 'Seated folk evening with tea and cake.', { day: -6, hour: 19, hours: 3, venue: 'brigid', state: 'published', alloc: 100, fill: 0.6, checkIn: 0.85, art: 'photo', crew: 'full', acts: ['tide', 'moth'] }),
  E('Warehouse Breaks All-Nighter', 'Ten hours of breaks across two rooms.', { day: -3, hour: 22, hours: 8, venue: 'dock', state: 'published', alloc: 400, fill: 0.7, checkIn: 0.77, art: 'photo', crew: 'full', acts: ['kilo', 'night'] }),
  E('Gallery Late: Open Studios', 'Twelve artists open their studios after hours.', { day: -1, hour: 18, hours: 4, venue: 'marrow', state: 'published', alloc: 120, fill: 0.45, checkIn: 0.6, art: 'poster', crew: 'light', acts: ['paper'] }),

  // Happening now and later today.
  E('Doors Open Now: Hex Garden', 'Hex Garden play a surprise early set. Doors are open.', { day: 0, hour: 'now-1', hours: 4, venue: 'coil', state: 'published', alloc: 150, fill: 0.8, checkIn: 0.4, art: 'photo', crew: 'full', acts: ['hex'] }),
  E('Moth Choir Late Set', 'An extra late set after the sold-out early show.', { day: 0, hour: 'now+4', hours: 2, venue: 'brigid', state: 'published', alloc: 90, fill: 0.5, art: 'poster', crew: 'full', acts: ['moth'] }),

  // Upcoming, free, at different levels of demand.
  E('Basement Frequencies 019', 'Monthly electro and breaks night.', { day: 4, hour: 21, hours: 5, venue: 'coil', state: 'published', alloc: 200, fill: 0.62, art: 'photo', crew: 'full', acts: ['kilo'], roles: true }),
  E('Zine Library Reading Room', 'Quiet afternoon with the full zine archive out on tables.', { day: 6, hour: 13, hours: 4, venue: 'marrow', state: 'published', alloc: 50, fill: 0.2, art: 'poster', crew: 'light', acts: ['paper'] }),
  E('Fern & The Fuses Sold-Out Show', 'Sold out. Returns are released at the door.', { day: 8, hour: 20, hours: 4, venue: 'coil', state: 'published', alloc: 120, fill: 1, art: 'photo', crew: 'full', acts: ['fern'] }),
  E('Poetry & Pints', 'Yuki Arai hosts six readers and an open floor.', { day: 10, hour: 19, hours: 3, venue: 'spindle', state: 'published', alloc: 40, fill: 0.95, art: 'photo', crew: 'light', acts: ['yuki'] }),
  E('Community Soundsystem Build Day', 'Help build and wire the new speaker stack. Lunch provided.', { day: 12, hour: 10, hours: 7, venue: 'dock', state: 'published', alloc: 30, fill: 0, art: 'poster', crew: 'full', acts: ['night'], roles: true }),
  E('Rooftop Sessions: Orbit Kane', 'Sunset modular set, weather permitting.', { day: 15, hour: 18, hours: 3, venue: 'lantern', state: 'published', alloc: 80, fill: 0.35, art: 'photo', crew: 'light', acts: ['orbit'] }),
  E('All-Ages Punk Picnic', 'Afternoon show in the hall car park. Bring a blanket.', { day: 18, hour: 13, hours: 5, venue: 'brigid', state: 'published', alloc: 250, fill: 0.15, art: 'photo', crew: 'full', acts: ['gutter', 'fern', 'brass'], roles: true }),
  E('Saoirse Tide In The Round', 'Seated in the round, no amplification.', { day: 22, hour: 19, hours: 2, venue: 'brigid', state: 'published', alloc: 70, fill: 0.4, art: 'photo', crew: 'light', acts: ['tide'] }),
  E('Night Market & DJs', 'Food stalls, record stalls and DJs until late.', { day: 29, hour: 17, hours: 6, venue: 'dock', state: 'published', alloc: 350, fill: 0.08, art: 'photo', crew: 'full', acts: ['kilo', 'night'], roles: true }),
  E('Secret Show (Location TBA)', 'Line-up and address go to ticket holders the day before.', { day: 33, hour: 21, hours: 3, venue: null, location: 'Riverton, address sent to ticket holders', state: 'published', alloc: 60, fill: 0.7, art: 'poster', crew: 'light', acts: [] }),
  E('Winter Solstice Drone', 'The longest night, the longest notes. Runs until sunrise.', { day: 39, hour: 22, hours: 9, venue: 'brigid', state: 'published', alloc: 100, fill: 0.1, art: 'poster', crew: 'full', acts: ['moth', 'orbit'] }),

  // Upcoming, paid. Public checkout needs Stripe, so these only carry an
  // organizer test ticket.
  E('Velvet Static: Two-Night Run', 'Two nights, two different sets.', { day: 20, hour: 20, hours: 4, venue: 'dock', state: 'published', price: 2200, alloc: 300, art: 'photo', crew: 'full', acts: ['velvet'], occurrences: ['scheduled', 'scheduled'] }),
  E('Low Brass Union Seated Concert', 'A rare indoor, seated set.', { day: 26, hour: 19, hours: 2, venue: 'brigid', state: 'published', price: 1500, alloc: 120, art: 'photo', crew: 'light', acts: ['brass'] }),
  E("New Year's Eve at Dock 9", 'Three rooms, ten DJs, one countdown.', { day: 49, hour: 21, hours: 7, venue: 'dock', state: 'published', price: 3500, alloc: 600, art: 'photo', crew: 'full', acts: ['kilo', 'night', 'orbit'], roles: true }),
  E('Modular Synth Workshop', 'Hands-on patching class. Instruments provided.', { day: 14, hour: 11, hours: 4, venue: 'spindle', state: 'published', price: 4000, alloc: 12, art: 'poster', crew: 'light', acts: ['orbit'] }),
  E('Gutter Saints Farewell Show', 'Their last show. Expect it to sell out.', { day: 44, hour: 20, hours: 4, venue: 'coil', state: 'published', price: 1800, alloc: 200, art: 'photo', crew: 'full', acts: ['gutter', 'hex'] }),

  // Published with a changed occurrence: postponed, rescheduled, cancelled.
  E('Rooftop Jazz Brunch', 'Postponed after the weather call. A new date is coming.', { day: 5, hour: 11, hours: 3, venue: 'lantern', state: 'published', alloc: 80, fill: 0.5, art: 'photo', crew: 'light', acts: ['brass'], occurrences: ['postponed'] }),
  E('Hex Garden Headline', 'Moved one week later because of a venue clash.', { day: 17, hour: 21, hours: 3, venue: 'coil', state: 'published', alloc: 150, fill: 0.55, art: 'poster', crew: 'full', acts: ['hex'], occurrences: ['rescheduled'] }),
  E('Outdoor Cinema: Concert Films', 'Cancelled: the screen hire fell through. Tickets will be refunded.', { day: 9, hour: 19, hours: 3, venue: 'lantern', state: 'published', alloc: 100, fill: 0.3, art: 'photo', crew: 'none', acts: [], occurrences: ['cancelled'] }),
  E('Print Workshop Series', 'Four Saturday workshops. Week three is cancelled.', { day: 3, hour: 12, hours: 3, venue: 'marrow', state: 'published', alloc: 24, fill: 0.9, art: 'poster', crew: 'light', acts: ['paper'], occurrences: ['scheduled', 'scheduled', 'cancelled', 'scheduled'] }),
  E('Choir Residency', 'Three Thursdays. The second date is postponed.', { day: 7, hour: 19, hours: 2, venue: 'brigid', state: 'published', alloc: 90, fill: 0.4, art: 'photo', crew: 'light', acts: ['moth'], occurrences: ['scheduled', 'postponed', 'scheduled'] }),
  E('Riverside Festival Day', 'Called off for the year. We will be back.', { day: 60, hour: 12, hours: 10, venue: 'dock', state: 'published', alloc: 800, fill: 0.05, art: 'photo', crew: 'full', acts: ['velvet', 'brass', 'night'], occurrences: ['cancelled'] }),

  // Drafts at different levels of completeness.
  E('Basement Frequencies 020', 'Monthly electro and breaks night.', { day: 32, hour: 21, hours: 5, venue: 'coil', state: 'draft', alloc: 200, art: 'photo', crew: 'full', acts: ['kilo'] }),
  E('Spring Zine Fest', 'Two-day zine fair. Table applications open soon.', { day: 120, hour: 11, hours: 7, venue: 'marrow', state: 'draft', alloc: 400, art: 'poster', crew: 'light', acts: ['paper'] }),
  E('Untitled Warehouse Party', 'Details to come.', { day: 75, hour: 22, hours: 6, venue: 'dock', state: 'draft', alloc: 0, art: 'none', crew: 'none', acts: [] }),
  E('Charity Gig for the Food Bank', 'All door money goes to the Northfield food bank.', { day: 52, hour: 19, hours: 4, venue: 'brigid', state: 'draft', alloc: 150, art: 'none', crew: 'light', acts: ['fern', 'tide'] }),
  E('Ambient Sleepover', 'Bring a sleeping bag. Music until 8am.', { day: 66, hour: 22, hours: 10, venue: 'marrow', state: 'draft', price: 2500, alloc: 30, art: 'poster', crew: 'full', acts: ['orbit', 'moth'] }),
  E('Record Fair', 'Thirty stalls of vinyl. Early entry ticketed.', { day: 41, hour: 10, hours: 6, venue: 'spindle', state: 'draft', alloc: 0, art: 'photo', crew: 'none', acts: [] }),
  E('Battle of the Bands Heat 1', 'Six bands, twenty minutes each, audience vote.', { day: 88, hour: 19, hours: 4, venue: 'coil', state: 'draft', alloc: 160, art: 'photo', crew: 'full', acts: [] }),
  E('Rooftop Season Opener', 'First rooftop night of the year.', { day: 150, hour: 18, hours: 4, venue: 'lantern', state: 'draft', alloc: 80, art: 'none', crew: 'none', acts: [] }),
  E('Brass Band Workshop', 'Learn a parade tune in an afternoon. Instruments available.', { day: 58, hour: 13, hours: 3, venue: 'brigid', state: 'draft', alloc: 25, art: 'poster', crew: 'light', acts: ['brass'] }),
];

// Generated posters keep to the dark terminal palette from
// contracts/design/tokens.json and are served as data URIs, because the
// development stack has no object store.
const POSTER_COLORS = ['#7c3aed', '#00ffff', '#00ff41', '#ffb000', '#ff6666', '#5b21b6'];

function hash(text) {
  let h = 2166136261;
  for (const char of text) h = Math.imul(h ^ char.codePointAt(0), 16777619);
  return h >>> 0;
}

function escapeXML(text) {
  return text.replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' })[c]);
}

function wrap(text, width) {
  const lines = [];
  for (const word of text.toUpperCase().split(/\s+/)) {
    const last = lines.at(-1);
    if (last && `${last} ${word}`.length <= width) lines[lines.length - 1] = `${last} ${word}`;
    else lines.push(word);
  }
  return lines;
}

export function posterDataURI(title, startsAt, location) {
  let seed = hash(title);
  const next = () => (seed = Math.imul(seed ^ (seed >>> 15), 2246822519) >>> 0) / 4294967296;
  const pick = () => POSTER_COLORS[Math.floor(next() * POSTER_COLORS.length)];
  const accent = pick();
  const shapes = [];
  for (let i = 0; i < 5; i++) {
    const x = Math.round(next() * 1600), y = Math.round(next() * 900), r = Math.round(120 + next() * 360);
    shapes.push(next() < 0.5
      ? `<circle cx="${x}" cy="${y}" r="${r}" fill="${pick()}" opacity="${(0.25 + next() * 0.4).toFixed(2)}"/>`
      : `<rect x="${x - r}" y="${y - r / 3}" width="${r * 2}" height="${Math.round(r / 1.5)}" fill="${pick()}" opacity="${(0.25 + next() * 0.4).toFixed(2)}" transform="rotate(${Math.round(next() * 90 - 45)} ${x} ${y})"/>`);
  }
  const lines = wrap(title, 16).slice(0, 4);
  const size = lines.length > 2 ? 104 : 128;
  const titleSVG = lines.map((line, i) =>
    `<text x="96" y="${330 + i * size * 0.98 - (lines.length - 1) * size * 0.35}" font-family="Impact, 'Arial Black', sans-serif" font-size="${size}" fill="#ffffff">${escapeXML(line)}</text>`).join('');
  const date = new Intl.DateTimeFormat('en-GB', { weekday: 'short', day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit', timeZoneName: 'short' }).format(startsAt).toUpperCase();
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900" width="1600" height="900">
<defs><pattern id="s" width="6" height="6" patternUnits="userSpaceOnUse"><rect width="6" height="2" fill="#000" opacity="0.35"/></pattern></defs>
<rect width="1600" height="900" fill="#0d1117"/>${shapes.join('')}<rect width="1600" height="900" fill="url(#s)"/>
<rect x="64" y="64" width="1472" height="772" fill="none" stroke="${accent}" stroke-width="6"/>
<text x="96" y="140" font-family="ui-monospace, Menlo, monospace" font-size="36" fill="${accent}">&gt; SUBCULT OS // DEV SEED</text>
${titleSVG}
<text x="96" y="760" font-family="ui-monospace, Menlo, monospace" font-size="40" fill="#ffffff">${escapeXML(date)}</text>
<text x="96" y="810" font-family="ui-monospace, Menlo, monospace" font-size="32" fill="#a0aec0">${escapeXML(location.toUpperCase())}</text>
</svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}
