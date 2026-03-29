/**
 * CSV export utility for admin dashboard data tables.
 */

export interface CsvColumn {
  key: string;
  header: string;
}

function escapeCSVValue(value: unknown): string {
  const str = value == null ? '' : String(value);
  if (str.includes(',') || str.includes('"') || str.includes('\n') || str.includes('\r')) {
    return '"' + str.replace(/"/g, '""') + '"';
  }
  return str;
}

export function buildCSVString(columns: CsvColumn[], data: Record<string, unknown>[]): string {
  const header = columns.map((c) => escapeCSVValue(c.header)).join(',');
  const rows = data.map((row) =>
    columns.map((c) => escapeCSVValue(row[c.key])).join(',')
  );
  return [header, ...rows].join('\r\n');
}

export function exportToCSV(
  columns: CsvColumn[],
  data: Record<string, unknown>[],
  filename: string
): void {
  const csv = buildCSVString(columns, data);
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.style.display = 'none';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
