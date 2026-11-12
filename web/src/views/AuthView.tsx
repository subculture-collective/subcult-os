import { useEffect, useMemo, useState } from 'react';
import { Brand } from '../ui/Brand';
import { TicketStub } from '../ui/TicketStub';
import type { FormEvent } from 'react';
import { postJSON } from '../api';
import { Button } from '../ui/Button';
import { Notice } from '../ui/Notice';
import type { CurrentUserDTO, SignupResultDTO } from '../domain';
import { safeReturnPath } from '../modules/auth/returnPath';

type Mode = 'login' | 'signup';

function getMode(): Mode {
  if (typeof window !== 'undefined' && window.location.pathname === '/signup') {
    return 'signup';
  }

  return 'login';
}

function getNextPath() {
  if (typeof window === 'undefined') {
    return '/';
  }

  return safeReturnPath(new URLSearchParams(window.location.search).get('next'));
}

function authHref(mode: Mode) {
  const next = getNextPath();
  return next === '/' ? `/${mode}` : `/${mode}?next=${encodeURIComponent(next)}`;
}

function goToNext(nextPath: string) {
  window.location.href = nextPath;
}

export function AuthView() {
  const mode = getMode();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const nextPath = useMemo(getNextPath, []);
  const title = useMemo(() => (mode === 'signup' ? 'Create account' : 'Sign in'), [mode]);
  useEffect(() => {
    document.title = `${title} | Subcult OS`;
  }, [title]);
  const eyebrow = mode === 'signup' ? 'New account' : 'Operator access';
  const description =
    mode === 'signup'
      ? 'One account works across every workspace you join. If an invite sent you here, use the email it went to.'
      : 'Back on shift. If an invite sent you here, sign in with the email it went to.';
  const invitePrompt = nextPath.startsWith('/invite/')
    ? 'Accepting an invitation? Sign in or create an account with the invited email.'
    : null;

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setNotice(null);

    const trimmedEmail = email.trim();
    if (!trimmedEmail) {
      setError('Enter your email address.');
      return;
    }

    if (password.length < 8) {
      setError('Password must be at least 8 characters.');
      return;
    }

    setLoading(true);

    try {
      const body = {
        email: trimmedEmail,
        password,
        ...(mode === 'signup' ? { displayName: displayName.trim() || undefined } : {}),
      };

      if (mode === 'signup') {
        const result = await postJSON<SignupResultDTO>('/api/auth/signup', body);
        setNotice(`Check ${result.email} for a verification link before signing in.`);
        return;
      }
      await postJSON<CurrentUserDTO>('/api/auth/login', body);
      goToNext(nextPath);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to continue');
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="min-h-screen px-4 py-6 text-fg-primary sm:px-6 lg:px-8">
      <section className="mx-auto flex min-h-[calc(100vh-3rem)] w-full max-w-md flex-col justify-center">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 text-xs uppercase tracking-[0.2em] text-fg-secondary">
          <Brand />
          <a className="text-fg-primary underline underline-offset-4" href="/">
            Workspace
          </a>
        </div>

        <div className="rounded-panel border border-stroke-subtle bg-surface-panel">
          <TicketStub kicker={`Subcult OS / ${eyebrow}`} className="rounded-b-none">
            <h1 className="ticket-stub-title text-5xl">{title}</h1>
          </TicketStub>
          <div className="ticket-perforation" aria-hidden="true" />
          <div className="p-6">
            <p className="text-sm leading-6 text-fg-secondary">{description}</p>

            {invitePrompt ? (
              <div className="mt-5 rounded-2xl border border-status-warning/20 bg-status-surface-warning px-4 py-3 text-sm leading-6 text-status-warning">
                {invitePrompt}
              </div>
            ) : null}

            <nav aria-label="Account" className="mt-6 flex gap-2 text-sm">
              <a
                className={`px-3 py-2 font-bold uppercase tracking-[0.05em] transition ${mode === 'login' ? 'bg-action-primary text-fg-inverse underline decoration-2 underline-offset-4' : 'bg-surface-inset text-fg-secondary'}`}
                href={authHref('login')}
                aria-current={mode === 'login' ? 'page' : undefined}
              >
                Sign in
              </a>
              <a
                className={`px-3 py-2 font-bold uppercase tracking-[0.05em] transition ${mode === 'signup' ? 'bg-action-primary text-fg-inverse underline decoration-2 underline-offset-4' : 'bg-surface-inset text-fg-secondary'}`}
                href={authHref('signup')}
                aria-current={mode === 'signup' ? 'page' : undefined}
              >
                Create account
              </a>
            </nav>

            <form className="mt-6 space-y-4" noValidate onSubmit={handleSubmit}>
              <label className="block space-y-2 text-sm">
                <span className="text-fg-secondary">Email</span>
                <input
                  className="field py-3"
                  type="email"
                  autoComplete="email"
                  required
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                />
              </label>

              {mode === 'signup' ? (
                <label className="block space-y-2 text-sm">
                  <span className="text-fg-secondary">Display name</span>
                  <input
                    className="field py-3"
                    type="text"
                    autoComplete="name"
                    value={displayName}
                    onChange={(event) => setDisplayName(event.target.value)}
                    placeholder="Optional"
                  />
                </label>
              ) : null}

              <label className="block space-y-2 text-sm">
                <span className="text-fg-secondary">Password</span>
                <input
                  className="field py-3"
                  type="password"
                  autoComplete={mode === 'signup' ? 'new-password' : 'current-password'}
                  minLength={8}
                  required
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  aria-describedby="password-rules"
                />
                <span id="password-rules" className="block text-xs text-fg-muted">
                  8+ characters
                </span>
              </label>

              {error ? <Notice tone="danger">{error}</Notice> : null}
              {notice ? <Notice tone="success">{notice}</Notice> : null}

              <Button
                className="w-full"
                type="submit"
                busy={loading}
              >
                {loading ? 'Working…' : title}
              </Button>
              {mode === 'login' ? <a className="block text-center text-sm text-fg-primary underline underline-offset-4" href="/recover">Forgot your password?</a> : null}
            </form>
          </div>
        </div>
      </section>
    </main>
  );
}
