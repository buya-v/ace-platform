import React, { useState, useMemo, useCallback } from 'react';
import { SortAscIcon, SortDescIcon, DownloadIcon, InboxIcon } from './icons';
import styles from './DataGrid.module.css';

export interface Column<T> {
  key: string;
  header: string;
  render?: (row: T, index: number) => React.ReactNode;
  sortable?: boolean;
  filterable?: boolean;
  align?: 'left' | 'right' | 'center';
  mono?: boolean;
  width?: string;
}

export interface DataGridProps<T> {
  columns: Column<T>[];
  data: T[];
  keyField: string;
  loading?: boolean;
  emptyMessage?: string;
  onRowClick?: (row: T) => void;
  exportFilename?: string;
  stickyHeader?: boolean;
  compact?: boolean;
}

type SortDir = 'asc' | 'desc' | null;

function getCellValue<T>(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key];
}

function exportCSV<T>(columns: Column<T>[], data: T[], filename: string) {
  const headers = columns.map(c => c.header);
  const rows = data.map(row =>
    columns.map(col => {
      const val = getCellValue(row, col.key);
      const str = val == null ? '' : String(val);
      // Escape double quotes and wrap if contains comma/newline/quote
      if (str.includes(',') || str.includes('\n') || str.includes('"')) {
        return `"${str.replace(/"/g, '""')}"`;
      }
      return str;
    })
  );

  const csv = [headers.join(','), ...rows.map(r => r.join(','))].join('\n');
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${filename}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

export function DataGrid<T>({
  columns,
  data,
  keyField,
  loading = false,
  emptyMessage = 'No data available',
  onRowClick,
  exportFilename,
  stickyHeader = false,
  compact = false,
}: DataGridProps<T>) {
  const [sortKey, setSortKey] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>(null);
  const [filters, setFilters] = useState<Record<string, string>>({});

  const hasFilters = columns.some(c => c.filterable);

  const onHeaderClick = useCallback((key: string) => {
    if (sortKey !== key) {
      setSortKey(key);
      setSortDir('asc');
    } else if (sortDir === 'asc') {
      setSortDir('desc');
    } else if (sortDir === 'desc') {
      setSortKey(null);
      setSortDir(null);
    }
  }, [sortKey, sortDir]);

  const handleFilterChange = useCallback((key: string, value: string) => {
    setFilters(prev => ({ ...prev, [key]: value }));
  }, []);

  const processedData = useMemo(() => {
    let result = [...data];

    // Apply filters
    const activeFilters = Object.entries(filters).filter(([, v]) => v.trim() !== '');
    if (activeFilters.length > 0) {
      result = result.filter(row =>
        activeFilters.every(([key, filterVal]) => {
          const val = getCellValue(row, key);
          if (val == null) return false;
          return String(val).toLowerCase().includes(filterVal.toLowerCase());
        })
      );
    }

    // Apply sort
    if (sortKey && sortDir) {
      result.sort((a, b) => {
        const aVal = getCellValue(a, sortKey);
        const bVal = getCellValue(b, sortKey);
        if (aVal == null && bVal == null) return 0;
        if (aVal == null) return 1;
        if (bVal == null) return -1;

        let cmp: number;
        if (typeof aVal === 'number' && typeof bVal === 'number') {
          cmp = aVal - bVal;
        } else {
          cmp = String(aVal).localeCompare(String(bVal));
        }
        return sortDir === 'desc' ? -cmp : cmp;
      });
    }

    return result;
  }, [data, filters, sortKey, sortDir]);

  const wrapperClasses = [
    styles.wrapper,
    stickyHeader ? styles.sticky : '',
    compact ? styles.compact : '',
  ].filter(Boolean).join(' ');

  const skeletonRows = 5;

  return (
    <div className={wrapperClasses}>
      {exportFilename && (
        <div className={styles.toolbar}>
          <button
            className={styles.exportBtn}
            onClick={() => exportCSV(columns, processedData, exportFilename)}
            title="Export CSV"
          >
            <DownloadIcon size={14} /> Export
          </button>
        </div>
      )}
      <table className={styles.table}>
        <thead className={styles.thead}>
          <tr>
            {columns.map(col => {
              const alignClass = col.align === 'right' ? styles.thRight : col.align === 'center' ? styles.thCenter : '';
              const isSorted = sortKey === col.key;
              return (
                <th
                  key={col.key}
                  className={`${col.sortable ? styles.thSortable : styles.th} ${alignClass}`}
                  style={col.width ? { width: col.width } : undefined}
                  onClick={col.sortable ? () => onHeaderClick(col.key) : undefined}
                >
                  {col.header}
                  {col.sortable && isSorted && sortDir && (
                    <span className={styles.sortIcon}>
                      {sortDir === 'asc' ? <SortAscIcon size={12} /> : <SortDescIcon size={12} />}
                    </span>
                  )}
                </th>
              );
            })}
          </tr>
          {hasFilters && (
            <tr className={styles.filterRow}>
              {columns.map(col => (
                <td key={col.key}>
                  {col.filterable ? (
                    <input
                      className={styles.filterInput}
                      placeholder={`Filter ${col.header.toLowerCase()}...`}
                      value={filters[col.key] ?? ''}
                      onChange={e => handleFilterChange(col.key, e.target.value)}
                    />
                  ) : null}
                </td>
              ))}
            </tr>
          )}
        </thead>
        <tbody>
          {loading ? (
            Array.from({ length: skeletonRows }).map((_, i) => (
              <tr key={`skeleton-${i}`} className={styles.tr}>
                {columns.map(col => (
                  <td key={col.key} className={styles.td}>
                    <div className={styles.skeletonCell} style={{ width: `${60 + Math.random() * 30}%` }} />
                  </td>
                ))}
              </tr>
            ))
          ) : processedData.length === 0 ? (
            <tr>
              <td colSpan={columns.length} className={styles.empty}>
                <div className={styles.emptyContainer}>
                  <span className={styles.emptyIcon}>
                    <InboxIcon size={32} />
                  </span>
                  <span className={styles.emptyText}>{emptyMessage}</span>
                </div>
              </td>
            </tr>
          ) : (
            processedData.map((row, idx) => {
              const key = String(getCellValue(row, keyField));
              return (
                <tr
                  key={key}
                  className={onRowClick ? styles.trClickable : styles.tr}
                  onClick={onRowClick ? () => onRowClick(row) : undefined}
                >
                  {columns.map(col => {
                    const alignClass = col.align === 'right' ? styles.tdRight : col.align === 'center' ? styles.tdCenter : '';
                    const monoClass = col.mono ? styles.mono : '';
                    return (
                      <td key={col.key} className={`${styles.td} ${alignClass} ${monoClass}`}>
                        {col.render ? col.render(row, idx) : String(getCellValue(row, col.key) ?? '')}
                      </td>
                    );
                  })}
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
}
