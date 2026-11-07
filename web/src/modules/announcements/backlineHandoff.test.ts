import { describe, expect, it } from 'vitest';
import type { ChannelHandoffDTO } from '../../domain';
import { blockerMessage, canRetryHandoff, handoffRetryHint, handoffStateLabel, handoffTone, latestRequest } from './backlineHandoff';

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
  it('keeps an access refusal retryable and says what to fix', () => {
    const refused = { ...base, status: 'failed' as const, attemptCount: 1, lastError: 'invalid_signature' };
    expect(canRetryHandoff(refused)).toBe(true);
    expect(handoffStateLabel(refused)).toBe('Not delivered; held for review');
    expect(handoffRetryHint(refused)).toContain('BACKLINE_HANDOFF_SECRET');
    expect(handoffRetryHint({ ...base, status: 'uncertain' })).toContain('Check the Backline channel');
  });
  it('ignores a selection response that a newer request superseded', async () => {
    const selection = latestRequest();
    const shown: string[] = [];
    let resolveA: (value: string) => void = () => {};
    const slowA = new Promise<string>((resolve) => { resolveA = resolve; });
    async function load(request: Promise<string>) {
      const token = selection.begin();
      const value = await request;
      if (selection.isCurrent(token)) shown.push(value);
    }
    const first = load(slowA);
    await load(Promise.resolve('announcement-b'));
    resolveA('announcement-a');
    await first;
    expect(shown).toEqual(['announcement-b']);
    const cleared = selection.begin();
    expect(selection.isCurrent(cleared)).toBe(true);
  });
});
