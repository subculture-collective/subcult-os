import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { AnnouncementPreviewDTO, ListmonkBindingDTO } from '../domain';
import { ListmonkRelayPanel, listmonkStatusLabel } from './ListmonkRelayPanel';

// Synthetic fixtures matching the backend contract (#220); no real listmonk.
const binding: ListmonkBindingDTO = {
  id: 'binding-a', label: 'Synthetic newsletter', listId: 7, templateId: 3, senderAddress: 'news@example.test',
  disclosureVersion: 'subcult-os-listmonk-v1', active: true, createdAt: '',
  korkPolicy: { listUrl: 'https://lists.example.test', sender: 'Signal Collective', purpose: 'Show announcements', disclosure: 'Once you confirm…' },
};
const preview: AnnouncementPreviewDTO = {
  subject: 'Doors move to 8pm', body: 'Synthetic.', recipientCount: 12, withheldCount: 3, estimatedCostCents: 0,
  deliveryPath: 'listmonk', listmonkConfigured: true,
};
function render(props: Partial<Parameters<typeof ListmonkRelayPanel>[0]> = {}) {
  return renderToStaticMarkup(<ListmonkRelayPanel binding={binding} preview={preview} counts={{}} busy={false}
    onConnect={vi.fn()} onRevoke={vi.fn()} onRetry={vi.fn()} {...props} />);
}

describe('listmonk relay panel', () => {
  it('shows the one bound list, sender, disclosure and Kork policy', () => {
    const html = render();
    expect(html).toContain('Synthetic newsletter (list 7, template 3)');
    expect(html).toContain('news@example.test');
    expect(html).toContain('subcult-os-listmonk-v1');
    expect(html).toContain('Accepted for &quot;Show announcements&quot; from Signal Collective');
    expect(html).toContain('Disconnect list');
    expect(html).not.toContain('Connect list<');
  });
  it('previews eligible and withheld counts without recipient addresses', () => {
    const html = render();
    expect(html).toContain('Eligible now</dt><dd>12');
    expect(html).toContain('Withheld now</dt><dd>3');
    expect(html).toContain('Delivery path</dt><dd>listmonk');
    // The only address on the page is the bound sender.
    expect(html.match(/@/g)?.length).toBe(1);
  });
  it('offers the binding form with an optional Kork receiver policy when nothing is bound', () => {
    const html = render({ binding: null, preview: { ...preview, deliveryPath: 'native', withheldCount: 0 } });
    expect(html).toContain('listmonk list id');
    expect(html).toContain('Transactional template id');
    expect(html).toContain('Required disclosure version');
    expect(html).toContain('Accept Kork sign-up receipts for this list');
    expect(html).toContain('Delivery path</dt><dd>Subcult OS email');
  });
  it('warns when the path is listmonk but the server is not configured', () => {
    expect(render({ preview: { ...preview, listmonkConfigured: false } })).toContain('listmonk is not configured on this server');
  });
  it('keeps uncertain outcomes visible with an explicit retry warning', () => {
    const html = render({ counts: { accepted: 9, uncertain: 1, withheld: 2 }, deliveryPath: 'listmonk' });
    expect(html).toContain('Accepted by listmonk: 9');
    expect(html).toContain('Outcome uncertain; held for review: 1');
    expect(html).toContain('can send it twice');
    expect(html).toContain('Retry 1 held message');
    expect(html).toContain('(recorded at dispatch)');
  });
  it('labels every relay status in words', () => {
    for (const status of ['held', 'pending', 'sending', 'accepted', 'rejected', 'uncertain', 'failed', 'withheld', 'suppressed', 'expired']) {
      expect(listmonkStatusLabel(status)).not.toBe(status);
    }
  });
});
