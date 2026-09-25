import { describe, expect, it } from 'vitest';
import { checkoutDestination, checkoutTerminalState, nextPurchaseIntent } from './purchaseIntent';

describe('purchase intent retry identity', () => {
  it('keeps a key for the same form and replaces it when the form changes', () => {
    const first = nextPurchaseIntent(null, 'guest@example.test', 'Guest', () => 'key-1');
    expect(nextPurchaseIntent(first, 'guest@example.test', 'Guest', () => 'key-2')).toBe(first);
    expect(nextPurchaseIntent(first, 'other@example.test', 'Guest', () => 'key-2')).toEqual({ email: 'other@example.test', displayName: 'Guest', key: 'key-2' });
  });

  it('does not redirect a pending reconciliation response', () => {
    expect(checkoutDestination({ ticketId: 't', ticketCode: 'c', ticketUrl: '/tickets/c', checkoutSessionId: '', checkoutUrl: '', checkoutStatus: 'pending_reconciliation' })).toBeNull();
    expect(checkoutDestination({ ticketId: 't', ticketCode: 'c', ticketUrl: '/tickets/c', checkoutSessionId: 'cs', checkoutUrl: 'https://checkout.example/cs', checkoutStatus: 'ready' })).toBe('https://checkout.example/cs');
  });

  it('accepts only ticket-bearing terminal checkout states', () => {
    expect(checkoutTerminalState({ checkoutStatus: 'expired', ticketUrl: '/tickets/expired' })).toEqual({ status: 'expired', ticketUrl: '/tickets/expired' });
    expect(checkoutTerminalState({ checkoutStatus: 'reconciliation_required', ticketUrl: '/tickets/reconcile' })).toEqual({ status: 'reconciliation_required', ticketUrl: '/tickets/reconcile' });
    expect(checkoutTerminalState({ checkoutStatus: 'expired' })).toBeNull();
    expect(checkoutTerminalState({ checkoutStatus: 'ready', ticketUrl: '/tickets/ready' })).toBeNull();
  });
});
