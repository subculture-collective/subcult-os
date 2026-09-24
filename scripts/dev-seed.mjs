// Invoked by dev-env.sh with the current worktree's loopback URL and DB container.
// Uses normal signup/verification APIs; only reads the synthetic user's held mail.
import { execFileSync } from 'node:child_process';

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

const email = 'dev@example.test';
const password = 'local-development-only';
const cookies = new Map();
async function request(path, body, allowed = []) {
  const response = await fetch(`${origin}${path}`, {
    method: body === undefined ? 'GET' : 'POST',
    headers: { 'Content-Type': 'application/json', Cookie: [...cookies].map(([k, v]) => `${k}=${v}`).join('; ') },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(15000),
  });
  for (const cookie of response.headers.getSetCookie()) {
    const pair = cookie.split(';', 1)[0];
    const split = pair.indexOf('=');
    cookies.set(pair.slice(0, split), pair.slice(split + 1));
  }
  if (!response.ok && !allowed.includes(response.status)) {
    throw new Error(`${path} returned ${response.status}`);
  }
  return { status: response.status, data: await response.json() };
}

const signup = await request('/api/auth/signup', { email, password, displayName: 'Dev Operator' }, [409]);
// Also recovers an interrupted seed after signup but before verification.
const message = execFileSync('docker', ['exec', databaseContainer,
  'psql', '-U', 'dev', '-d', 'dev', '-XAt', '-v', 'ON_ERROR_STOP=1', '-c', `
    select o.body from email_outbox o
    join identity_challenges c on c.id = o.related_id
    where o.recipient_email = 'dev@example.test'
      and o.related_type = 'identity_verification' and o.delivery_status = 'held'
      and c.consumed_at is null and c.expires_at > now()
    order by o.created_at desc limit 1`], { encoding: 'utf8' });
if (message.trim()) {
  const link = message.split(/\s+/).find(part => part.startsWith(`${origin}/verify-email?`));
  if (!link) throw new Error('Expected a verification link for this dev origin.');
  await request('/api/auth/verify-email', { token: new URL(link).searchParams.get('token') });
} else if (signup.status !== 409) {
  throw new Error('Expected a held verification message.');
}
await request('/api/auth/login', { email, password });
let workspace = await request('/api/workspaces/current', undefined, [404]);
if (workspace.status === 404) {
  workspace = await request('/api/workspaces', { name: 'Development Collective' });
}
const events = await request(`/api/workspaces/${workspace.data.id}/events`);
if (!events.data.some(event => event.title === 'Development Night')) {
  await request(`/api/workspaces/${workspace.data.id}/events`, {
    title: 'Development Night',
    startsAt: new Date(Date.now() + 7 * 86400000).toISOString(),
    publicDescription: 'Synthetic event for local development.',
    locationDisplay: 'Development Venue',
    ticketAllocation: 100,
  });
}
console.log(`Development data ready. Sign in at ${origin}/login\nEmail: ${email}\nPassword: ${password}`);
