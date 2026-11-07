import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { BacklinePreviewDTO, ChannelHandoffDTO } from '../domain';
import { BacklineHandoffPanel } from './BacklineHandoffPanel';

// Synthetic fixture matching the backend preview contract; no real channel.
const preview: BacklinePreviewDTO = {
  announcementId: 'announcement-a', announcementStatus: 'scheduled', approved: true,
  subject: 'Doors move to 8pm', body: 'Synthetic channel announcement.',
  contentRevision: 'sha256:abc', contentLength: 50, contentLimit: 1800, mentionPolicy: 'none',
  deliverAt: '2026-11-08T18:00:00Z', expiresAt: '2026-11-09T18:00:00Z', adapterConfigured: true,
  binding: { id: 'binding-a', channel: 'backline', label: 'Synthetic #announcements', serverId: '112233445566778899', channelId: '998877665544332211', mentionPolicy: 'none', active: true, createdAt: '' },
  handoff: null, blockers: [],
};
const handoff: ChannelHandoffDTO = {
  id: 'handoff-a', bindingId: 'binding-a', bindingLabel: 'Synthetic #announcements', occurrenceKey: 'k', contentRevision: 'sha256:abc',
  operation: 'publish', status: 'uncertain', attemptCount: 1, lastError: 'receiver_http_500',
  deliverAt: preview.deliverAt, expiresAt: preview.expiresAt, createdAt: '', updatedAt: '',
};
function render(overrides: Partial<BacklinePreviewDTO> = {}, handoffs: ChannelHandoffDTO[] = []) {
  return renderToStaticMarkup(<BacklineHandoffPanel preview={{ ...preview, ...overrides }} handoffs={handoffs} busy={false} onHandoff={vi.fn()} onRetry={vi.fn()} />);
}

describe('Backline handoff preview', () => {
  it('shows approved content, destination and the no-mention policy before handoff', () => {
    const html = render();
    expect(html).toContain('Doors move to 8pm');
    expect(html).toContain('Synthetic channel announcement.');
    expect(html).toContain('Synthetic #announcements');
    expect(html).toContain('998877665544332211');
    expect(html).toContain('No mentions');
    expect(html).toContain('Hand off to Synthetic #announcements');
    expect(html).toContain('Nothing handed off yet.');
  });
  it('offers no handoff while a blocker remains', () => {
    const html = render({ approved: false, announcementStatus: 'draft', blockers: ['announcement_not_approved'] });
    expect(html).toContain('Schedule the announcement first');
    expect(html).not.toContain('Hand off to');
  });
  it('offers no handoff without a connected channel', () => {
    const html = render({ binding: null, blockers: ['no_active_binding'] });
    expect(html).toContain('No connected channel');
    expect(html).not.toContain('Hand off to');
  });
  it('keeps an uncertain outcome visible with an explicit same-key retry', () => {
    const html = render({ blockers: ['already_handed_off'], handoff }, [handoff]);
    expect(html).not.toContain('Hand off to');
    expect(html).toContain('Outcome uncertain; held for review');
    expect(html).toContain('receiver_http_500');
    expect(html).toContain('Retry with the same key');
  });
  it('shows an acknowledged handoff and its delivery receipt without a retry', () => {
    const accepted: ChannelHandoffDTO = { ...handoff, status: 'accepted', lastError: undefined, receiverStatus: 'accepted', deliveryOutcome: 'delivered' };
    const html = render({ blockers: ['already_handed_off'], handoff: accepted }, [accepted]);
    expect(html).toContain('Accepted by Backline');
    expect(html).toContain('Channel delivery receipt: delivered');
    expect(html).not.toContain('Retry with the same key');
  });
});
