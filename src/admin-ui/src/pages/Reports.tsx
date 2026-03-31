import React, { useState } from 'react';
import { fetchMarketSummaryReport, fetchLargeTraderReport } from '../services/api';
import { buildCSVString, exportToCSV, CsvColumn } from '../utils/export';
import styles from './Reports.module.css';

export interface MarketSummaryRow {
  instrument_id: string;
  open: string;
  high: string;
  low: string;
  close: string;
  volume: number;
  vwap?: string;
}

export interface LargeTraderRow {
  participant_id: string;
  participant_name: string;
  instrument_id: string;
  net_position: number;
  notional_value: string;
  pct_of_open_interest: number;
}

/** Format a numeric string to 2 decimal places */
export function formatPrice(value: string): string {
  if (!value) return '-';
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return num.toFixed(2);
}

/** Format volume with thousands separators */
export function formatVolume(volume: number): string {
  if (volume == null || isNaN(volume)) return '-';
  return volume.toLocaleString();
}

/** Format percentage */
export function formatPct(pct: number): string {
  if (pct == null || isNaN(pct)) return '-';
  return `${pct.toFixed(2)}%`;
}

/** Get today's date as YYYY-MM-DD */
export function todayDateString(): string {
  const d = new Date();
  return d.toISOString().split('T')[0];
}

/** Market summary columns for CSV export */
export const MARKET_SUMMARY_COLUMNS: CsvColumn[] = [
  { key: 'instrument_id', header: 'Instrument' },
  { key: 'open', header: 'Open' },
  { key: 'high', header: 'High' },
  { key: 'low', header: 'Low' },
  { key: 'close', header: 'Close' },
  { key: 'volume', header: 'Volume' },
  { key: 'vwap', header: 'VWAP' },
];

/** Large trader columns for CSV export */
export const LARGE_TRADER_COLUMNS: CsvColumn[] = [
  { key: 'participant_id', header: 'Participant ID' },
  { key: 'participant_name', header: 'Participant' },
  { key: 'instrument_id', header: 'Instrument' },
  { key: 'net_position', header: 'Net Position' },
  { key: 'notional_value', header: 'Notional Value' },
  { key: 'pct_of_open_interest', header: '% of Open Interest' },
];

/** Build market summary CSV string (pure function for testing) */
export function buildMarketSummaryCSV(rows: MarketSummaryRow[]): string {
  return buildCSVString(MARKET_SUMMARY_COLUMNS, rows as unknown as Record<string, unknown>[]);
}

/** Build large trader CSV string (pure function for testing) */
export function buildLargeTraderCSV(rows: LargeTraderRow[]): string {
  return buildCSVString(LARGE_TRADER_COLUMNS, rows as unknown as Record<string, unknown>[]);
}

export function ReportsPage() {
  const [reportDate, setReportDate] = useState(todayDateString());
  const [marketData, setMarketData] = useState<MarketSummaryRow[] | null>(null);
  const [traderData, setTraderData] = useState<LargeTraderRow[] | null>(null);
  const [loading, setLoading] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleMarketSummary = async () => {
    setLoading('market');
    setError(null);
    try {
      const result = await fetchMarketSummaryReport(reportDate);
      setMarketData(result.data ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load market summary');
    } finally {
      setLoading(null);
    }
  };

  const handleLargeTrader = async () => {
    setLoading('trader');
    setError(null);
    try {
      const result = await fetchLargeTraderReport(reportDate);
      setTraderData(result.data ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load large trader report');
    } finally {
      setLoading(null);
    }
  };

  return (
    <div>
      <h1>Reports</h1>

      <div className={styles.topRow}>
        <label htmlFor="report-date">Report Date:</label>
        <input
          id="report-date"
          type="date"
          className={styles.dateInput}
          value={reportDate}
          onChange={e => setReportDate(e.target.value)}
        />
        <button
          className={`${styles.btn} ${styles.btnPrimary}`}
          onClick={handleMarketSummary}
          disabled={loading === 'market'}
        >
          {loading === 'market' ? 'Loading...' : 'Generate Market Summary'}
        </button>
        <button
          className={`${styles.btn} ${styles.btnSecondary}`}
          onClick={handleLargeTrader}
          disabled={loading === 'trader'}
        >
          {loading === 'trader' ? 'Loading...' : 'Large Trader Report'}
        </button>
      </div>

      {error && <div className={styles.error}>{error}</div>}

      {marketData !== null && (
        <>
          <div className={styles.actionRow}>
            <h2 className={styles.sectionTitle}>Market Summary - {reportDate}</h2>
            <button
              className={`${styles.btn} ${styles.btnDownload}`}
              onClick={() => exportToCSV(MARKET_SUMMARY_COLUMNS, marketData as unknown as Record<string, unknown>[], `market-summary-${reportDate}.csv`)}
            >
              Download CSV
            </button>
          </div>
          {marketData.length > 0 ? (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Instrument</th>
                  <th>Open</th>
                  <th>High</th>
                  <th>Low</th>
                  <th>Close</th>
                  <th>Volume</th>
                  <th>VWAP</th>
                </tr>
              </thead>
              <tbody>
                {marketData.map(row => (
                  <tr key={row.instrument_id}>
                    <td>{row.instrument_id}</td>
                    <td className={styles.numericCell}>{formatPrice(row.open)}</td>
                    <td className={styles.numericCell}>{formatPrice(row.high)}</td>
                    <td className={styles.numericCell}>{formatPrice(row.low)}</td>
                    <td className={styles.numericCell}>{formatPrice(row.close)}</td>
                    <td className={styles.numericCell}>{formatVolume(row.volume)}</td>
                    <td className={styles.numericCell}>{formatPrice(row.vwap ?? '')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <div className={styles.empty}>No market data for {reportDate}</div>
          )}
        </>
      )}

      {traderData !== null && (
        <>
          <div className={styles.actionRow}>
            <h2 className={styles.sectionTitle}>Large Trader Report - {reportDate}</h2>
            <button
              className={`${styles.btn} ${styles.btnDownload}`}
              onClick={() => exportToCSV(LARGE_TRADER_COLUMNS, traderData as unknown as Record<string, unknown>[], `large-traders-${reportDate}.csv`)}
            >
              Download CSV
            </button>
          </div>
          {traderData.length > 0 ? (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Participant</th>
                  <th>Instrument</th>
                  <th>Net Position</th>
                  <th>Notional Value</th>
                  <th>% of Open Interest</th>
                </tr>
              </thead>
              <tbody>
                {traderData.map(row => (
                  <tr key={`${row.participant_id}-${row.instrument_id}`}>
                    <td>{row.participant_name}</td>
                    <td>{row.instrument_id}</td>
                    <td className={`${styles.numericCell} ${row.net_position >= 0 ? styles.positivePnl : styles.negativePnl}`}>
                      {row.net_position.toLocaleString()}
                    </td>
                    <td className={styles.numericCell}>{formatPrice(row.notional_value)}</td>
                    <td className={styles.numericCell}>{formatPct(row.pct_of_open_interest)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <div className={styles.empty}>No large trader positions for {reportDate}</div>
          )}
        </>
      )}
    </div>
  );
}
