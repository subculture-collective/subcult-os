import { api, patchJSON, postForm, postJSON } from '@/api/client';
import type { EventDTO, EventRoleApplicationDTO, EventRoleApplicationStatus, EventRoleDTO, EventStaffingItemDTO, TicketDTO } from '@/api/types';

export interface EventWritePayload {
  title: string;
  startsAt: string;
  publicDescription: string;
  locationDisplay: string;
  imageUrl?: string;
  ticketAllocation: number;
  pricingMode: EventDTO['pricingMode'];
  ticketPriceCents: number;
  ticketCurrency: string;
}

export function listWorkspaceEvents(workspaceID: string) {
  return api<EventDTO[]>(`/api/workspaces/${encodeURIComponent(workspaceID)}/events`);
}

export function getEvent(eventID: string) {
  return api<EventDTO>(`/api/events/${encodeURIComponent(eventID)}`);
}

export function createEvent(workspaceID: string, body: EventWritePayload) {
  return postJSON<EventDTO>(`/api/workspaces/${encodeURIComponent(workspaceID)}/events`, body);
}

export function updateEvent(eventID: string, body: EventWritePayload) {
  return patchJSON<EventDTO>(`/api/events/${encodeURIComponent(eventID)}`, body);
}

export function publishEvent(eventID: string) {
  return postJSON<EventDTO>(`/api/events/${encodeURIComponent(eventID)}/publish`, {});
}

export function createTestTicket(eventID: string, body: { email?: string; displayName?: string } = {}) {
  return postJSON<TicketDTO>(`/api/events/${encodeURIComponent(eventID)}/test-ticket`, body);
}

export function uploadEventImage(eventID: string, image: { uri: string; fileName?: string | null; mimeType?: string | null; file?: Blob | null }) {
  const body = new FormData();
  if (image.file) {
    body.append('image', image.file, image.fileName ?? 'event-image.jpg');
  } else {
    body.append('image', {
      uri: image.uri,
      name: image.fileName ?? 'event-image.jpg',
      type: image.mimeType ?? 'image/jpeg',
    } as unknown as Blob);
  }
  return postForm<EventDTO>(`/api/events/${encodeURIComponent(eventID)}/image`, body);
}

export function listEventStaffing(eventID: string) {
  return api<EventStaffingItemDTO[]>(`/api/events/${encodeURIComponent(eventID)}/staffing`);
}

export interface CreateEventStaffingPayload {
  title: string;
  kind: EventStaffingItemDTO['kind'];
  notes: string;
  participantRequirements?: string;
  startsAt?: string | null;
  endsAt?: string | null;
}

export function createEventStaffing(eventID: string, body: CreateEventStaffingPayload) {
  return postJSON<EventStaffingItemDTO>(`/api/events/${encodeURIComponent(eventID)}/staffing`, body);
}

export interface UpdateEventStaffingPayload {
  title?: string;
  notes?: string;
  participantRequirements?: string;
  startsAt?: string | null;
  clearStartsAt?: boolean;
  endsAt?: string | null;
  clearEndsAt?: boolean;
  status?: EventStaffingItemDTO['status'];
}

export function updateEventStaffing(eventID: string, staffingID: string, body: UpdateEventStaffingPayload) {
  return patchJSON<EventStaffingItemDTO>(`/api/events/${encodeURIComponent(eventID)}/staffing/${encodeURIComponent(staffingID)}`, body);
}

export function updateEventStaffingStatus(eventID: string, staffingID: string, status: EventStaffingItemDTO['status']) {
  return updateEventStaffing(eventID, staffingID, { status });
}

export function listEventRoles(eventID: string) {
  return api<EventRoleDTO[]>(`/api/events/${encodeURIComponent(eventID)}/roles`);
}

export function createEventRole(eventID: string, body: { name: string; description: string; capacity: number; public: boolean }) {
  return postJSON<EventRoleDTO>(`/api/events/${encodeURIComponent(eventID)}/roles`, body);
}

export function updateEventRole(eventID: string, roleID: string, body: { name?: string; description?: string; capacity?: number; public?: boolean; active?: boolean }) {
  return patchJSON<EventRoleDTO>(`/api/events/${encodeURIComponent(eventID)}/roles/${encodeURIComponent(roleID)}`, body);
}

export function listEventRoleApplications(eventID: string) {
  return api<EventRoleApplicationDTO[]>(`/api/events/${encodeURIComponent(eventID)}/role-applications`);
}

export function reviewEventRoleApplication(eventID: string, applicationID: string, status: EventRoleApplicationStatus) {
  return patchJSON<EventRoleApplicationDTO>(`/api/events/${encodeURIComponent(eventID)}/role-applications/${encodeURIComponent(applicationID)}`, { status });
}
