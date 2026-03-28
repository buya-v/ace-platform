import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchWarehouseReceipts, fetchWarehouseDeliveries, fetchWarehouseFacilities } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
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
                    style={{ width: `${pct}%`, background: pct > 90 ? '#dc3545' : pct > 70 ? '#ffc107' : '#28a745' }}
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
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Receipt ID</th>
              <th>Commodity</th>
              <th>Grade</th>
              <th>Quantity</th>
              <th>Warehouse</th>
              <th>Status</th>
              <th>Holder</th>
              <th>Issued</th>
            </tr>
          </thead>
          <tbody>
            {filteredReceipts.map(r => (
              <tr key={r.id}>
                <td className={styles.mono}>{r.id}</td>
                <td>{r.commodity}</td>
                <td>{r.grade}</td>
                <td>{r.quantity} {r.unit}</td>
                <td>{r.warehouse_name}</td>
                <td><StatusBadge status={r.status} /></td>
                <td>{r.holder_name}</td>
                <td>{new Date(r.issued_at).toLocaleDateString()}</td>
              </tr>
            ))}
            {filteredReceipts.length === 0 && (
              <tr><td colSpan={8} style={{ textAlign: 'center', color: '#888', padding: 32 }}>No receipts found</td></tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Pending deliveries */}
      <div className={styles.section}>
        <h2>Pending Deliveries</h2>
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Delivery ID</th>
              <th>Receipt</th>
              <th>From</th>
              <th>To</th>
              <th>Commodity</th>
              <th>Quantity</th>
              <th>Status</th>
              <th>Requested</th>
            </tr>
          </thead>
          <tbody>
            {allDeliveries.map(d => (
              <tr key={d.id}>
                <td className={styles.mono}>{d.id}</td>
                <td className={styles.mono}>{d.receipt_id}</td>
                <td>{d.from_warehouse}</td>
                <td>{d.to_destination}</td>
                <td>{d.commodity}</td>
                <td>{d.quantity}</td>
                <td><StatusBadge status={d.status} /></td>
                <td>{new Date(d.requested_at).toLocaleDateString()}</td>
              </tr>
            ))}
            {allDeliveries.length === 0 && (
              <tr><td colSpan={8} style={{ textAlign: 'center', color: '#888', padding: 32 }}>No pending deliveries</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
