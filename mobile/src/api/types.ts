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
  checkoutStatus: 'ready' | 'paid' | 'pending_reconciliation' | 'expired' | 'reconciliation_required';
}

export type WorkspaceRole = 'owner' | 'organizer' | 'finance' | 'door' | 'crew' | 'member';

export interface MemberDTO {
  personId?: string;
  id: string;
  email: string;
  displayName: string | null;
  role: WorkspaceRole;
  accessState?: 'active' | 'expired' | 'revoked';
  expiresAt?: string;
  revokedAt?: string;
}

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

export type LifecycleActionKind = 'public_record' | 'provider_ticket' | 'operational_notice' | 'refund';

export interface LifecycleIntentActionDTO {
  id: string;
  actionKind: LifecycleActionKind;
  destination: string;
  status: 'pending' | 'running' | 'succeeded' | 'retryable' | 'unknown' | 'failed' | 'superseded';
  attemptCount: number;
  failureCategory?: string;
  createdAt: string;
  updatedAt: string;
}

export interface LifecycleNoticeDTO {
  id: string;
  changeId: string;
  subject: string;
  body: string;
  queuedAt: string;
  recipients: Array<{ email: string; status: string; attempts: number; feedback: string }>;
}

export interface LifecycleNoticePreviewDTO {
  changeId: string;
  occurrenceId: string;
  revision: string;
  publicCid: string;
  audiences: Array<'ticket_holders' | 'assigned_crew'>;
  subject: string;
  body: string;
  recipients: Array<{ email: string; sourceType: 'ticket' | 'crew_person' | 'crew_application'; suppressed: boolean }>;
  previewHash: string;
}

export interface LifecycleIntentDTO {
  id: string;
  workspaceId: string;
  eventId: string;
  occurrenceId: string;
  kind: 'cancellation' | 'reschedule';
  reason: string;
  targetRevision: string;
  expectedPublicCid: string;
  status: 'approved' | 'superseded';
  approvedByPersonId: string;
  createdAt: string;
  supersededAt?: string;
  actions: LifecycleIntentActionDTO[];
}

export interface EventFinanceLineDTO {
  id: string;
  eventId: string;
  entryType: 'budget' | 'payable' | 'actual_payment';
  direction: 'income' | 'expense';
  amountCents: number;
  currency: string;
  label: string;
  reason: string;
  dueAt?: string;
  occurredAt?: string;
  payableLineId?: string;
  correctsLineId?: string;
  createdByPersonId: string;
  createdAt: string;
}

export interface EventStaffingItemDTO {
  id: string;
  eventId: string;
  title: string;
  kind: 'task' | 'shift';
  notes: string;
  participantRequirements: string;
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

export interface ParticipantOpenShiftDTO {
  staffingItemId: string;
  eventId: string;
  eventTitle: string;
  title: string;
  startsAt: string;
  endsAt: string;
  participantRequirements: string;
  remainingPlaces: number;
  claimed: boolean;
  canRelease: boolean;
  organizerManaged: boolean;
  releaseDeadline: string;
}

export interface ParticipantAssignmentDTO {
  eventId: string;
  eventTitle: string;
  staffingItemId: string;
  title: string;
  kind: 'task' | 'shift';
  startsAt?: string | null;
  endsAt?: string | null;
  status: 'open' | 'assigned' | 'completed';
  participantRequirements: string;
}

export type EventAccessTopic = 'entry' | 'bathrooms' | 'seating' | 'sensory' | 'transit' | 'contact';
export interface AccessInformationRevisionDTO {
  evaluatedAt: string;
  id?: string;
  topic: EventAccessTopic;
  scope: 'event' | 'venue';
  revision: number;
  value: 'unknown' | 'yes' | 'no' | 'available' | 'limited' | 'not_available' | 'known';
  effectiveValue: AccessInformationRevisionDTO['value'];
  needsReview: boolean;
  details: string;
  sourceKind: 'unknown' | 'organizer_assertion' | 'event_observation' | 'venue_observation' | 'external_reference';
  sourceReference: string;
  reviewedAt?: string;
  expiresAt?: string;
  correctionReason: string;
  recordedAt?: string;
}
export interface EventAccessWorksheetDTO {
  eventId: string;
  evaluatedAt: string;
  entries: EventAccessRevisionDTO[];
}
export interface EventAccessHistoryDTO {
  topic: EventAccessTopic;
  revisions: EventAccessRevisionDTO[];
  nextBefore?: number;
}

export interface EventAccessRevisionDTO extends AccessInformationRevisionDTO {
  scope: 'event';
  sourceKind: 'unknown' | 'organizer_assertion' | 'event_observation' | 'external_reference';
}
export interface VenueAccessRevisionDTO extends AccessInformationRevisionDTO {
  scope: 'venue';
  sourceKind: 'unknown' | 'organizer_assertion' | 'venue_observation' | 'external_reference';
}
export interface VenueAccessWorksheetDTO {
  placeId: string;
  placeName: string;
  evaluatedAt: string;
  entries: VenueAccessRevisionDTO[];
}

export interface VenueAccessPlaceDTO {
  id: string;
  name: string;
}
export interface VenueAccessPlaceIndexDTO {
  places: VenueAccessPlaceDTO[];
  nextAfter?: string;
}

export interface AccessComparisonOccurrenceDTO {
  id: string;
  name: string;
  startsAt: string;
  status: 'scheduled' | 'rescheduled' | 'postponed' | 'cancelled';
  updatedAt: string;
  placeId?: string;
}
export interface OccurrenceAccessComparisonDTO {
  eventId: string;
  workspaceId: string;
  evaluatedAt: string;
  occurrence: AccessComparisonOccurrenceDTO;
  event: EventAccessWorksheetDTO;
  venue: VenueAccessWorksheetDTO | null;
}
