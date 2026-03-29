import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchWarehouseReceipts, fetchWarehouseDeliveries, fetchWarehouseFacilities } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { DataGrid, Column } from '../components/DataGrid';
import { WarehouseReceipt, PendingDelivery } from '../types';
import styles from './WarehouseOverview.module.css';

export function WarehouseOverviewPage() {
  const [receiptSearch, setReceiptSearch] = useState('');

  const receipts = usePolling(
    (signal) => fetchWarehouseReceipts({}, signal),
    60000,
  );

  const deliveries = usePolling(
    (signal) => fetchWarehouseDeliveries({}, signal),
    60000,
  );

  const facilities = usePolling(
    (signal) => fetchWarehouseFacilities(signal),
    60000,
  );

  const allReceipts = receipts.data?.data ?? [];
  const filteredReceipts = receiptSearch
    ? allReceipts.filter(r => r.id.toLowerCase().includes(receiptSearch.toLowerCase()))
    : allReceipts;

  const allDeliveries = deliveries.data?.data ?? [];
  const allFacilities = facilities.data?.data ?? [];

  // Count by commodity
  const commodityCounts: Record<string, number> = {};
  allReceipts.forEach(r => {
    commodityCounts[r.commodity] = (commodityCounts[r.commodity] || 0) + 1;
  });

  const receiptColumns: Column<WarehouseReceipt>[] = [
    { key: 'id', header: 'Receipt ID', mono: true, sortable: true },
    { key: 'commodity', header: 'Commodity', sortable: true },
    { key: 'grade', header: 'Grade' },
    { key: 'quantity', header: 'Quantity', render: (row) => `${row.quantity} ${row.unit}` },
    { key: 'warehouse_name', header: 'Warehouse', sortable: true },
    { key: 'status', header: 'Status', render: (row) => <StatusBadge status={row.status} /> },
    { key: 'holder_name', header: 'Holder', sortable: true },
    { key: 'issued_at', header: 'Issued', sortable: true, render: (row) => new Date(row.issued_at).toLocaleDateString() },
  ];

  const deliveryColumns: Column<PendingDelivery>[] = [
    { key: 'id', header: 'Delivery ID', mono: true, sortable: true },
    { key: 'receipt_id', header: 'Receipt', mono: true },
    { key: 'from_warehouse', header: 'From', sortable: true },
    { key: 'to_destination', header: 'To', sortable: true },
    { key: 'commodity', header: 'Commodity', sortable: true },
    { key: 'quantity', header: 'Quantity', align: 'right', mono: true },
    { key: 'status', header: 'Status', render: (row) => <StatusBadge status={row.status} /> },
    { key: 'requested_at', header: 'Requested', sortable: true, render: (row) => new Date(row.requested_at).toLocaleDateString() },
  ];

  return (
    <div>
      <h1>Warehouse Overview</h1>

      {/* Commodity summary */}
      {Object.keys(commodityCounts).length > 0 && (
        <div className={styles.commodityGrid}>
          {Object.entries(commodityCounts).map(([commodity, count]) => (
            <div key={commodity} className={styles.commodityCard}>
              <div className={styles.commodityName}>{commodity}</div>
              <div className={styles.commodityCount}>{count} receipts</div>
            </div>
          ))}
        </div>
      )}

      {/* Facility capacity */}
      {allFacilities.length > 0 && (
        <div className={styles.section}>
          <h2>Facility Capacity</h2>
          {allFacilities.map(f => {
            const pct = f.total_capacity > 0 ? (f.used_capacity / f.total_capacity) * 100 : 0;
            return (
              <div key={f.id} className={styles.capacityRow}>
                <span className={styles.facilityName}>{f.name}</span>
                <div className={styles.gaugeTrack}>
                  <div
                    className={styles.gaugeFill}
                    style={{ width: `${pct}%`, background: pct > 90 ? 'var(--accent-red)' : pct > 70 ? 'var(--accent-yellow)' : 'var(--accent-green)' }}
                  />
                </div>
                <span className={styles.gaugePct}>{pct.toFixed(0)}%</span>
              </div>
            );
          })}
        </div>
      )}

      {/* Receipt search */}
      <div className={styles.section}>
        <h2>Receipts</h2>
        <input
          type="text"
          placeholder="Search by receipt ID..."
          value={receiptSearch}
          onChange={e => setReceiptSearch(e.target.value)}
          className={styles.searchInput}
        />
        <DataGrid
          columns={receiptColumns}
          data={filteredReceipts}
          keyField="id"
          emptyMessage="No receipts found"
          exportFilename="warehouse-receipts"
        />
      </div>

      {/* Pending deliveries */}
      <div className={styles.section}>
        <h2>Pending Deliveries</h2>
        <DataGrid
          columns={deliveryColumns}
          data={allDeliveries}
          keyField="id"
          emptyMessage="No pending deliveries"
          exportFilename="warehouse-deliveries"
        />
      </div>
    </div>
  );
}
