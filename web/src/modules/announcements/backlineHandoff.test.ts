import { describe, expect, it } from 'vitest';
import type { ChannelHandoffDTO } from '../../domain';
import { blockerMessage, canRetryHandoff, handoffStateLabel, handoffTone } from './backlineHandoff';

const base: ChannelHandoffDTO = {
  id: 'h1', bindingId: 'b1', bindingLabel: 'Synthetic', occurrenceKey: 'k', contentRevision: 'sha256:x', operation: 'publish', status: 'pending',
  attemptCount: 0, deliverAt: '2026-11-08T18:00:00Z', expiresAt: '2026-11-09T18:00:00Z', createdAt: '', updatedAt: '',
};

describe('Backline handoff states', () => {
  it('describes every state in words and only offers retry for held outcomes', () => {
    expect(handoffStateLabel({ ...base, status: 'uncertain' })).toContain('uncertain');
    expect(handoffTone({ ...base, status: 'uncertain' })).toBe('warning');
    expect(canRetryHandoff({ ...base, status: 'uncertain' })).toBe(true);
    expect(canRetryHandoff({ ...base, status: 'failed' })).toBe(true);
    expect(canRetryHandoff({ ...base, status: 'accepted' })).toBe(false);
    expect(handoffStateLabel({ ...base, operation: 'cancel', status: 'accepted', receiverStatus: 'already_delivered' })).toContain('could not withdraw');
    expect(handoffStateLabel({ ...base, operation: 'cancel', status: 'accepted', receiverStatus: 'cancelled' })).toBe('Cancelled at Backline');
  });
  it('explains known blockers and names unknown ones', () => {
    expect(blockerMessage('announcement_not_approved')).toContain('Schedule');
    expect(blockerMessage('future_blocker')).toContain('future_blocker');
  });
});
