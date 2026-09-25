import type { PaidReservationDTO } from '../../domain';

export type PurchaseIntent = { email: string; displayName: string; key: string };

export function nextPurchaseIntent(current: PurchaseIntent | null, email: string, displayName: string, createKey: () => string): PurchaseIntent {
  if (current && current.email === email && current.displayName === displayName) return current;
  return { email, displayName, key: createKey() };
}

export function checkoutDestination(checkout: PaidReservationDTO): string | null {
  if (checkout.checkoutStatus === 'ready' && checkout.checkoutUrl) return checkout.checkoutUrl;
  return checkout.checkoutStatus === 'paid' ? checkout.ticketUrl : null;
}

export type PurchaseTerminalState = 'expired' | 'reconciliation_required';

export function checkoutTerminalState(value: unknown): { status: PurchaseTerminalState; ticketUrl: string } | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  if ((record.checkoutStatus !== 'expired' && record.checkoutStatus !== 'reconciliation_required') || typeof record.ticketUrl !== 'string' || record.ticketUrl === '') return null;
  return { status: record.checkoutStatus, ticketUrl: record.ticketUrl };
}
