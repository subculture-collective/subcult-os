import { apiBlob } from '../../api';
import type { WorkspaceRole } from '../../domain';

const settlementExportFilename = 'event-settlement.csv';
const settlementReportExports = {
  markdown: { suffix: 'settlement.md', filename: 'event-settlement.md', accept: 'text/markdown' },
  print: { suffix: 'settlement-print.html', filename: 'event-settlement-print.html', accept: 'text/html' },
} as const;

export type SettlementReportExportKind = keyof typeof settlementReportExports;

function downloadBlob(blob: Blob, filename: string) {
  const objectURL = URL.createObjectURL(blob);
  try {
    const link = document.createElement('a');
    link.href = objectURL;
    link.download = filename;
    link.hidden = true;
    document.body.appendChild(link);
    link.click();
    link.remove();
  } finally {
    URL.revokeObjectURL(objectURL);
  }
}

export async function downloadSettlementExport(eventID: string) {
	const csv = await apiBlob(`/api/events/${encodeURIComponent(eventID)}/exports/settlement.csv`, {
		headers: { Accept: 'text/csv' },
	});
  downloadBlob(csv, settlementExportFilename);
}

export function settlementReportExportDetails(kind: SettlementReportExportKind) {
  return settlementReportExports[kind];
}

export async function downloadSettlementReport(eventID: string, kind: SettlementReportExportKind) {
  const report = settlementReportExportDetails(kind);
  const blob = await apiBlob(`/api/events/${encodeURIComponent(eventID)}/exports/${report.suffix}`, {
    headers: { Accept: report.accept },
  });
  downloadBlob(blob, report.filename);
}

export function canDownloadSettlementExport(workspaceID: string | undefined, eventWorkspaceID: string | undefined, role: WorkspaceRole | undefined) {
  return workspaceID === eventWorkspaceID && (role === 'owner' || role === 'finance');
}
