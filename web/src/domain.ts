export type WorkspaceRole = 'owner' | 'organizer' | 'finance' | 'door' | 'crew' | 'member';

export type EventStatus = 'draft' | 'published' | 'end_of_night';

export interface CurrentUserDTO {
  id: string;
  email: string;
  displayName: string | null;
  workspaces: WorkspaceSummaryDTO[];
}

export interface ParticipantPortalDTO {
  assignments: ParticipantAssignmentDTO[];
  commitments: ParticipantCommitmentDTO[];
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

export interface ParticipantCommitmentDTO {
  id: string;
  eventId: string;
  eventTitle: string;
  title: string;
  dueAt?: string | null;
  status: 'open' | 'done' | 'cancelled';
}

export interface SignupResultDTO {
  verificationRequired: boolean;
  email: string;
}

export interface WorkspaceSummaryDTO {
  id: string;
  name: string;
  role: WorkspaceRole;
}

export interface WorkspaceDTO {
  id: string;
  name: string;
  role: WorkspaceRole;
}

export interface CurrentWorkspaceDTO extends WorkspaceDTO {
  members: MemberDTO[];
  invitations: InvitationDTO[];
}

export interface MemberDTO {
  id: string;
  email: string;
  displayName: string | null;
  role: WorkspaceRole;
}

export interface ContactDTO {
  id: string;
  workspaceId: string;
  displayName: string;
  email?: string;
  phone?: string;
  notes: string;
  tags: string[];
  createdAt: string;
  updatedAt: string;
}

export interface CreateContactRequestDTO {
  displayName: string;
  email?: string;
  phone?: string;
  notes?: string;
  tags?: string[];
}

export interface UpdateContactRequestDTO {
  displayName?: string | null;
  email?: string | null;
  phone?: string | null;
  notes?: string | null;
  tags?: string[];
  clearEmail?: boolean;
  clearPhone?: boolean;
}

export interface CommitmentDTO {
  id: string;
  workspaceId: string;
  eventId?: string | null;
  contactId?: string | null;
  title: string;
  description: string;
  dueAt?: string | null;
  status: 'open' | 'done' | 'cancelled';
  ownerPersonId?: string | null;
  createdByPersonId: string;
  completedAt?: string | null;
  completedByPersonId?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ReminderEventDTO {
  id: string;
  workspaceId: string;
  eventId?: string | null;
  sourceType: 'commitment' | 'staffing';
  sourceId: string;
  reminderType: 'commitment.due' | 'staffing.upcoming' | 'staffing.unassigned';
  recipientEmail: string;
  dueAt: string;
  notificationEventId?: string | null;
  status: 'queued';
  subject: string;
  preview: string;
  createdAt: string;
}

export interface EventTemplateDTO {
  id: string;
  workspaceId: string;
  name: string;
  title: string;
  publicDescription: string;
  locationDisplay: string;
  ticketAllocation: number;
  pricingMode: 'free' | 'fixed';
  ticketPriceCents: number;
  ticketCurrency: string;
  privateNotes: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateEventTemplateRequestDTO {
  name: string;
  title: string;
  publicDescription?: string;
  locationDisplay?: string;
  ticketAllocation?: number;
  pricingMode?: string;
  ticketPriceCents?: number;
  ticketCurrency?: string;
  privateNotes?: string;
}

export interface UpdateEventTemplateRequestDTO {
  name?: string | null;
  title?: string | null;
  publicDescription?: string | null;
  locationDisplay?: string | null;
  ticketAllocation?: number | null;
  pricingMode?: string | null;
  ticketPriceCents?: number | null;
  ticketCurrency?: string | null;
  privateNotes?: string | null;
}

export interface ApplyEventTemplateRequestDTO {
  templateId: string;
}

export interface CreateCommitmentRequestDTO {
  title: string;
  description?: string;
  dueAt?: string;
  eventId?: string;
  contactId?: string;
  ownerPersonId?: string;
}

export interface UpdateCommitmentRequestDTO {
  title?: string | null;
  description?: string | null;
  dueAt?: string | null;
  eventId?: string | null;
  contactId?: string | null;
  ownerPersonId?: string | null;
  status?: 'open' | 'done' | 'cancelled' | null;
  clearDueAt?: boolean;
  clearEvent?: boolean;
  clearContact?: boolean;
  clearOwner?: boolean;
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
  pricingMode: 'free' | 'fixed';
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

export interface CulturalImportMatchDTO {
  occurrenceId: string;
  eventId: string;
  name: string;
  startsAt: string;
  status: string;
  updatedAt: string;
}

export interface CulturalImportCandidateDTO {
  id: string;
  row: number;
  sourceRecordId: string;
  title: string;
  description?: string;
  startsAt: string;
  endsAt?: string;
  timezone: string;
  status: string;
  matches: CulturalImportMatchDTO[];
  ambiguous: boolean;
  matchesTruncated: boolean;
}

export interface CulturalImportPreviewDTO {
  id: string;
  workspaceId: string;
  schema: string;
  sourceId: string;
  contentSha256: string;
  createdAt: string;
  candidates: CulturalImportCandidateDTO[];
  errors: Array<{ row: number; field?: string; code: string }>;
  actions: CulturalImportActionDTO[];
}

export interface CulturalImportActionDTO {
  id: string;
  candidateId: string;
  mode: 'create' | 'correction';
  eventId: string;
  occurrenceId?: string;
  createdOccurrenceId?: string;
  appliedAt: string;
  rolledBackAt?: string;
}

export interface PublicEventDTO {
  id: string;
  title: string;
  startsAt: string;
  publicDescription: string;
  locationDisplay: string;
  imageUrl: string | null;
  pricingMode: 'free' | 'fixed';
  ticketPriceCents: number;
  ticketCurrency: string;
  status: 'published';
  publicSlug: string;
  publicUrl: string;
  remainingTickets: number;
  isFull: boolean;
}

export interface PublicEventSummaryDTO {
  id: string;
  title: string;
  startsAt: string;
  publicDescription: string;
  locationDisplay: string;
  imageUrl?: string;
  workspaceName: string;
  pricingMode: 'free' | 'fixed';
  ticketPriceCents: number;
  ticketCurrency: string;
  remainingTickets: number;
  isFull: boolean;
  applicationsOpen: boolean;
  status: 'published';
  publicSlug: string;
  publicUrl: string;
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

export interface EventRoleApplicationDTO {
  id: string;
  eventId: string;
  roleId: string;
  applicantName: string;
  applicantEmail: string;
  message: string;
  status: 'submitted' | 'under_review' | 'accepted' | 'waitlisted' | 'rejected' | 'withdrawn' | 'confirmed';
  reviewedByPersonId?: string | null;
  reviewedAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface EventStaffingItemDTO {
  id: string;
  eventId: string;
  title: string;
  kind: 'task' | 'shift';
  notes: string;
  participantRequirements: string;
  startsAt?: string | null;
  endsAt?: string | null;
  assignedPersonId?: string | null;
  assignedApplicationId?: string | null;
  assigneeName?: string | null;
  status: 'open' | 'assigned' | 'completed' | 'cancelled';
  createdAt: string;
  updatedAt: string;
  completedAt?: string | null;
  completedByPersonId?: string | null;
}

export interface CreateEventStaffingRequestDTO {
  title: string;
  kind: 'task' | 'shift';
  notes: string;
  participantRequirements?: string;
  startsAt?: string | null;
  endsAt?: string | null;
}

export interface UpdateEventStaffingRequestDTO {
  title?: string | null;
  notes?: string | null;
  participantRequirements?: string | null;
  startsAt?: string | null;
  clearStartsAt?: boolean;
  endsAt?: string | null;
  clearEndsAt?: boolean;
  assignedPersonId?: string | null;
  assignedApplicationId?: string | null;
  clearAssignee?: boolean;
  status?: 'open' | 'assigned' | 'completed' | 'cancelled' | null;
}

export interface EventParticipantDTO {
  applicationId: string;
  roleId: string;
  roleName: string;
  applicantName: string;
  applicantEmail: string;
  status: 'accepted' | 'confirmed';
  updatedAt: string;
}

export interface NotificationEventDTO {
  id: string;
  eventId?: string;
  recipientEmail: string;
  notificationType: string;
  relatedType: string;
  relatedId?: string;
  subject: string;
  preview: string;
  status: 'queued';
  createdAt: string;
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

export interface PaidReservationDTO {
  ticketId: string;
  ticketCode: string;
  ticketUrl: string;
  checkoutSessionId: string;
  checkoutUrl: string;
  checkoutStatus: 'ready' | 'paid' | 'pending_reconciliation' | 'expired' | 'reconciliation_required';
}

export interface TicketReservationDTO extends TicketDTO {
  ticketUrl: string;
}

export interface InvitationCreatedDTO {
  id: string;
  workspaceId: string;
  email: string;
  role: WorkspaceRole;
  token: string;
}

export interface InvitationDTO {
  id: string;
  email: string;
  displayName?: string | null;
  role: WorkspaceRole;
  token?: string;
  acceptedAt: string | null;
}

export interface EventReportDTO {
  id: string;
  eventId: string;
  title: string;
  startsAt: string;
  publicUrl: string;
  ticketAllocation: number;
  ticketsReserved: number;
  ticketsCheckedIn: number;
  noShows: number;
  settlementSummary?: EventSettlementSummaryDTO;
  generatedAt: string;
  generatedByMemberEmail: string;
}

export interface EventArchiveDTO {
  id: string;
  eventId: string;
  reportId: string;
  settlementId: string;
  seededEventId?: string;
  status: 'private';
  noteCount: number;
  participants: EventArchiveParticipantDTO[];
  staffingItems: EventArchiveStaffingItemDTO[];
  notes: EventArchiveNoteDTO[];
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

export interface EventArchiveParticipantDTO {
  id: string;
  archiveId: string;
  sourceApplicationId: string;
  roleName: string;
  participantName: string;
  status: 'accepted' | 'confirmed';
  createdAt: string;
}

export interface EventArchiveStaffingItemDTO {
  id: string;
  archiveId: string;
  sourceStaffingItemId: string;
  title: string;
  kind: 'task' | 'shift';
  status: 'open' | 'assigned' | 'completed' | 'cancelled';
  assigneeName?: string | null;
  createdAt: string;
}

export interface WorkspaceArchiveSummaryDTO {
  id: string;
  eventId: string;
  title: string;
  startsAt: string;
  locationDisplay: string;
  noteCount: number;
  reportId: string;
  settlementId: string;
  seededEventId?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EventArchiveNoteDTO {
  id: string;
  archiveId: string;
  body: string;
  createdByPersonId: string;
  createdAt: string;
}

export interface EventSettlementSummaryDTO {
  currency: string;
  grossPaidRevenueCents: number;
  paidTicketCount: number;
  pendingTicketCount: number;
  cancelledTicketCount: number;
  freeTicketCount: number;
  reservedCount: number;
}

export interface EventSettlementAdjustmentDTO {
  id: string;
  settlementId: string;
  amountCents: number;
  label: string;
  reason: string;
  createdByPersonId: string;
  createdAt: string;
}

export interface EventSettlementDTO {
  id: string;
  eventId: string;
  currency: string;
  grossPaidRevenueCents: number;
  paidTicketCount: number;
  pendingTicketCount: number;
  cancelledTicketCount: number;
  freeTicketCount: number;
  reservedCount: number;
  adjustmentTotalCents: number;
  netTotalCents: number;
  status: 'open' | 'finalized';
  generatedAt: string;
  finalizedAt?: string;
  finalizedByPersonId?: string;
  adjustments: EventSettlementAdjustmentDTO[];
}

export interface DevEmailOutboxMessageDTO {
  id: string;
  recipientEmail: string;
  subject: string;
  body: string;
  relatedType: string;
  relatedId: string | null;
  createdAt: string;
}
