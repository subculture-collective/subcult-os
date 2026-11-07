import { AuthView } from './views/AuthView';
import { DiscoverView } from './views/DiscoverView';
import { DoorView } from './views/DoorView';
import { EventAccessView, VenueAccessView } from './views/EventAccessView';
import { EventEditorView } from './views/EventEditorView';
import { InviteView } from './views/InviteView';
import { IdentityActionView } from './views/IdentityActionView';
import { ConsentActionView } from './views/ConsentActionView';
import { PublicEventView } from './views/PublicEventView';
import { ParticipantPortalView } from './views/ParticipantPortalView';
import { TicketView } from './views/TicketView';
import { WorkspaceView } from './views/WorkspaceView';
import { MemberRolesView } from './views/MemberRolesView';
import { PublicArchiveItemsPanel } from './components/PublicArchiveItemsPanel';
import { ImportView } from './views/ImportView';
import { LifecycleIntentsView } from './views/LifecycleIntentsView';
import { VenueAccessIndexView } from './views/VenueAccessIndexView';
import { DesignSystemView } from './views/DesignSystemView';
import { AnnouncementChannelsView } from './views/AnnouncementChannelsView';

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

  if (import.meta.env.DEV && pathname === '/design-system') return <DesignSystemView />;

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
    if (pathname.endsWith('/access-info')) return <EventAccessView eventId={getSegment(pathname, 2)} />;
    if (pathname.endsWith('/public-archive')) return <PublicArchiveItemsPanel eventId={getSegment(pathname, 2)} />;
    return <EventEditorView eventId={getSegment(pathname, 2)} />;
  }

  if (/^\/workspace\/[^/]+\/members$/.test(pathname)) return <MemberRolesView key={getSegment(pathname, 2)} workspaceId={getSegment(pathname, 2)} />;
  if (/^\/workspace\/[^/]+\/places\/[^/]+\/access-info$/.test(pathname)) return <VenueAccessView workspaceId={getSegment(pathname, 2)} placeId={getSegment(pathname, 4)} />;
  if (/^\/workspace\/[^/]+\/announcements$/.test(pathname)) return <AnnouncementChannelsView key={getSegment(pathname, 2)} workspaceId={getSegment(pathname, 2)} />;
  if (/^\/workspace\/[^/]+\/venue-access$/.test(pathname)) return <VenueAccessIndexView key={getSegment(pathname, 2)} workspaceId={getSegment(pathname, 2)} />;

  if (pathname.startsWith('/workspace/') && pathname.endsWith('/cultural-imports')) {
    return <ImportView workspaceId={getSegment(pathname, 2)} />;
  }

  if (pathname.startsWith('/workspace/') && pathname.endsWith('/lifecycle-intents')) {
    return <LifecycleIntentsView workspaceId={getSegment(pathname, 2)} />;
  }

  if (pathname === '/workspace' || pathname === '/') {
    return <WorkspaceView />;
  }

  return <WorkspaceView />;
}
