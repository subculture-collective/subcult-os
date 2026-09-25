import { AuthView } from './views/AuthView';
import { DiscoverView } from './views/DiscoverView';
import { DoorView } from './views/DoorView';
import { EventEditorView } from './views/EventEditorView';
import { InviteView } from './views/InviteView';
import { IdentityActionView } from './views/IdentityActionView';
import { ConsentActionView } from './views/ConsentActionView';
import { PublicEventView } from './views/PublicEventView';
import { ParticipantPortalView } from './views/ParticipantPortalView';
import { TicketView } from './views/TicketView';
import { WorkspaceView } from './views/WorkspaceView';
import { PublicArchiveItemsPanel } from './components/PublicArchiveItemsPanel';

function getPathname() {
  if (typeof window === 'undefined') {
    return '/';
  }

  return window.location.pathname;
}

function getSegment(pathname: string, index: number) {
  return pathname.split('/')[index] ?? '';
}

export default function App() {
  const pathname = getPathname();

  if (pathname === '/login' || pathname === '/signup' || pathname === '/auth') {
    return <AuthView />;
  }

  if (pathname === '/verify-email') return <IdentityActionView action="verify" />;
  if (pathname === '/recover') return <IdentityActionView action="request-recovery" />;
  if (pathname === '/recover-password') return <IdentityActionView action="complete-recovery" />;
  if (pathname === '/consent/confirm') return <ConsentActionView action="confirm" />;
  if (pathname === '/consent/withdraw') return <ConsentActionView action="withdraw" />;

  if (pathname === '/discover') {
    return <DiscoverView />;
  }

  if (pathname === '/participant') return <ParticipantPortalView />;

  if (pathname.startsWith('/e/')) {
    return <PublicEventView slug={getSegment(pathname, 2)} />;
  }

  if (pathname.startsWith('/public/events/')) {
    return <PublicEventView slug={getSegment(pathname, 3)} />;
  }

  if (pathname.startsWith('/tickets/')) {
    return <TicketView code={getSegment(pathname, 2)} />;
  }

	if (pathname === '/door' || pathname.startsWith('/door/')) {
		return <DoorView eventId={getSegment(pathname, 2)} />;
	}

  if (pathname.startsWith('/invite/')) {
    return <InviteView token={getSegment(pathname, 2)} />;
  }

  if (pathname.startsWith('/events/')) {
    if (pathname.endsWith('/public-archive')) return <PublicArchiveItemsPanel eventId={getSegment(pathname, 2)} />;
    return <EventEditorView eventId={getSegment(pathname, 2)} />;
  }

  if (pathname === '/workspace' || pathname === '/') {
    return <WorkspaceView />;
  }

  return <WorkspaceView />;
}
