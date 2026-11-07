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
    case 'failed': return 'Not delivered; held for review';
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

// A failed handoff is either out of automatic attempts or was refused by
// Backline in a way an operator can fix (signature, clock, access). Both
// stay retryable with the same key.
export function canRetryHandoff(handoff: ChannelHandoffDTO) {
  return handoff.status === 'uncertain' || handoff.status === 'failed';
}

const accessRefusals = new Set(['invalid_signature', 'receiver_http_401', 'receiver_http_403']);

// handoffRetryHint says what to check before a retry, by the stored code.
export function handoffRetryHint(handoff: ChannelHandoffDTO) {
  if (handoff.status === 'failed' && handoff.lastError && accessRefusals.has(handoff.lastError)) {
    return 'Backline refused the signature or access. Check that both deployments hold the same BACKLINE_HANDOFF_SECRET and that their clocks agree, then retry.';
  }
  return 'Check the Backline channel before retrying. A retry reuses the same occurrence key, so Backline posts it at most once.';
}

// latestRequest hands out tokens so a view can ignore a response that an
// earlier, slower request returned after a newer one started.
export function latestRequest() {
  let current = 0;
  return {
    begin() { current += 1; return current; },
    isCurrent(token: number) { return token === current; },
  };
}

export const mentionPolicyLabel = 'No mentions. Backline posts the text without notifying users, roles or everyone.';
