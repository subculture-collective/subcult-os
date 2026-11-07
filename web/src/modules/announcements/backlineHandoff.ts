import type { BacklineHandoffBlocker, ChannelHandoffDTO } from '../../domain';

const blockerMessages: Record<BacklineHandoffBlocker, string> = {
  announcement_not_approved: 'Schedule the announcement first. Only scheduled or dispatched announcements can go to Backline.',
  no_active_binding: 'No Backline channel is connected to this workspace.',
  content_too_long: 'The subject and body are longer than the Backline channel limit.',
  adapter_not_configured: 'Backline delivery is not configured on this server.',
  already_handed_off: 'This announcement has already been handed off to the connected channel.',
};

export function blockerMessage(blocker: BacklineHandoffBlocker | string) {
  return blockerMessages[blocker as BacklineHandoffBlocker] ?? `Handoff blocked: ${blocker}`;
}

// handoffStateLabel names the current transport state in words; status color
// alone never carries meaning.
export function handoffStateLabel(handoff: ChannelHandoffDTO) {
  const cancel = handoff.operation === 'cancel';
  switch (handoff.status) {
    case 'pending': return cancel ? 'Cancellation queued' : 'Queued for Backline';
    case 'sending': return cancel ? 'Sending cancellation' : 'Sending to Backline';
    case 'accepted':
      if (!cancel) return 'Accepted by Backline';
      return handoff.receiverStatus === 'already_delivered' ? 'Already posted; Backline could not withdraw it' : 'Cancelled at Backline';
    case 'rejected': return 'Rejected by Backline';
    case 'uncertain': return 'Outcome uncertain; held for review';
    case 'failed': return 'Retries exhausted; held for review';
    case 'cancelled': return 'Cancelled before delivery';
    case 'expired': return 'Delivery window closed before handoff';
    default: return handoff.status;
  }
}

export function handoffTone(handoff: ChannelHandoffDTO): 'success' | 'warning' | 'danger' | 'info' {
  if (handoff.status === 'rejected' || handoff.status === 'failed') return 'danger';
  if (handoff.status === 'uncertain' || handoff.status === 'expired') return 'warning';
  if (handoff.status === 'accepted' && handoff.operation === 'publish') return 'success';
  return 'info';
}

export function canRetryHandoff(handoff: ChannelHandoffDTO) {
  return handoff.status === 'uncertain' || handoff.status === 'failed';
}

export const mentionPolicyLabel = 'No mentions. Backline posts the text without notifying users, roles or everyone.';
