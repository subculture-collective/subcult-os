import { describe, expect, it } from 'vitest';

import { canDownloadSettlementExport, settlementReportExportDetails } from './settlementExport';

describe('canDownloadSettlementExport', () => {
  it.each(['owner', 'finance'] as const)('allows the %s role for the event workspace', (role) => {
    expect(canDownloadSettlementExport('workspace-1', 'workspace-1', role)).toBe(true);
  });

  it.each(['organizer', 'door', 'crew', 'member'] as const)('does not render the action for %s', (role) => {
    expect(canDownloadSettlementExport('workspace-1', 'workspace-1', role)).toBe(false);
  });

  it('does not render the action for another workspace', () => {
    expect(canDownloadSettlementExport('workspace-1', 'workspace-2', 'finance')).toBe(false);
  });
});

describe('settlementReportExportDetails', () => {
  it('maps each report kind to its private attachment endpoint and filename', () => {
    expect(settlementReportExportDetails('markdown')).toEqual({
      suffix: 'settlement.md', filename: 'event-settlement.md', accept: 'text/markdown',
    });
    expect(settlementReportExportDetails('print')).toEqual({
      suffix: 'settlement-print.html', filename: 'event-settlement-print.html', accept: 'text/html',
    });
  });
});
