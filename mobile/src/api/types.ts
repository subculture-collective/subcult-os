export type PricingMode = 'free' | 'fixed';

export type EventStatus = 'draft' | 'published' | 'end_of_night';

export interface PublicEventSummaryDTO {
  id: string;
  title: string;
  startsAt: string;
  publicDescription: string;
  locationDisplay: string;
  imageUrl?: string;
  workspaceName: string;
  pricingMode: PricingMode;
  ticketPriceCents: number;
  ticketCurrency: string;
  remainingTickets: number;
  isFull: boolean;
  applicationsOpen: boolean;
  status: 'published';
  publicSlug: string;
  publicUrl: string;
}

export interface PublicEventDTO {
  id: string;
  title: string;
  startsAt: string;
  publicDescription: string;
  locationDisplay: string;
  imageUrl: string | null;
  pricingMode: PricingMode;
  ticketPriceCents: number;
  ticketCurrency: string;
  status: 'published';
  publicSlug: string;
  publicUrl: string;
  remainingTickets: number;
  isFull: boolean;
}

export interface PublicDiscoverySourceDTO {
  did: string;
  uri: string;
  handle?: string;
}

export interface PublicDiscoveryLocationDTO {
  name: string;
  locality?: string;
  region?: string;
  country?: string;
  latitude?: string;
  longitude?: string;
}

export interface PublicDiscoveryHandoffDTO {
  kind: 'local' | 'none';
  reason?: string;
  eventSlug?: string;
  reservationPath?: string;
}

export interface PublicDiscoveryOccurrenceDTO {
  uri: string;
  source: PublicDiscoverySourceDTO;
  name: string;
  description?: string;
  startsAt: string;
  endsAt?: string;
  timezone?: string;
  status: string;
  projectionStatus: 'active' | 'deleted' | 'unavailable';
  location?: PublicDiscoveryLocationDTO;
  handoff: PublicDiscoveryHandoffDTO;
}

export interface TicketDTO {
  id: string;
  eventId: string;
  email: string;
  displayName: string | null;
  code: string;
  ticketUrl: string;
  status: 'reserved' | 'checked_in';
  paymentStatus: 'free' | 'pending' | 'paid' | 'cancelled';
  amountCents: number;
  currency: string;
  checkedInAt: string | null;
}

export interface DoorTicketDTO {
  id: string;
  code: string;
  displayName: string | null;
  admissionEligible: boolean;
  status: 'reserved' | 'checked_in';
  checkedInAt: string | null;
}

export interface TicketReservationDTO extends TicketDTO {
  ticketUrl: string;
}

export interface PaidReservationDTO {
  ticketId: string;
  ticketCode: string;
  ticketUrl: string;
  checkoutSessionId: string;
  checkoutUrl: string;
}

export type WorkspaceRole = 'owner' | 'member';

export interface WorkspaceSummaryDTO {
  id: string;
  name: string;
  role: WorkspaceRole;
}

export interface CurrentUserDTO {
  id: string;
  email: string;
  displayName: string | null;
  workspaces: WorkspaceSummaryDTO[];
}

export interface SignupResultDTO {
  verificationRequired: boolean;
  email: string;
}

export interface EventDTO {
  id: string;
  workspaceId: string;
  title: string;
  startsAt: string;
  publicDescription: string;
  locationDisplay: string;
  imageUrl: string | null;
  ticketAllocation: number;
  pricingMode: PricingMode;
  ticketPriceCents: number;
  ticketCurrency: string;
  reservedCount: number;
  checkedInCount: number;
  staffingOpenCount: number;
  staffingAssignedCount: number;
  staffingCompletedCount: number;
  staffingCancelledCount: number;
  status: EventStatus;
  publicSlug: string | null;
  publicUrl: string | null;
}

export interface EventStaffingItemDTO {
  id: string;
  eventId: string;
  title: string;
  kind: 'task' | 'shift';
  notes: string;
  startsAt: string | null;
  endsAt: string | null;
  assignedPersonId: string | null;
  assignedApplicationId: string | null;
  assigneeName: string | null;
  status: 'open' | 'assigned' | 'completed' | 'cancelled';
  createdAt: string;
  updatedAt: string;
  completedAt: string | null;
  completedByPersonId: string | null;
}

export interface EventRoleDTO {
  id: string;
  eventId: string;
  name: string;
  description: string;
  capacity: number;
  public: boolean;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export type EventRoleApplicationStatus = 'submitted' | 'under_review' | 'accepted' | 'waitlisted' | 'rejected' | 'withdrawn' | 'confirmed';

export interface EventRoleApplicationDTO {
  id: string;
  eventId: string;
  roleId: string;
  applicantName: string;
  applicantEmail: string;
  message: string;
  status: EventRoleApplicationStatus;
  reviewedByPersonId?: string;
  reviewedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface PublicArchiveItemDTO {
  id: string;
  eventId: string;
  replacesItemId?: string | null;
  kind: 'credit' | 'link';
  title: string;
  attributionName: string;
  attributionUrl?: string | null;
  externalUrl?: string | null;
  intendedUse: 'link_only' | 'display_credit';
  rightsAssertion: 'owned' | 'licensed' | 'permission_asserted' | 'public_domain';
  evidenceReference: string;
  status: 'approved' | 'corrected' | 'unavailable';
  unavailableReason: string;
  approvedAt: string;
  createdAt: string;
}
