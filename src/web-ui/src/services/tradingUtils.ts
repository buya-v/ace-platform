import type { Position } from '../types/trade';
import type { TradeRecord, TradeHistoryFilter } from '../types/trade';

/**
 * Calculate unrealized P&L for a position.
 * For long: (currentPrice - avgEntryPrice) * quantity
 * For short: (avgEntryPrice - currentPrice) * quantity
 */
export function calculateUnrealizedPnl(
  side: 'long' | 'short' | 'flat',
  avgEntryPrice: string,
  currentPrice: string,
  quantity: string,
): string {
  if (side === 'flat') return '0.0000';
  const entry = Number(avgEntryPrice);
  const current = Number(currentPrice);
  const qty = Math.abs(Number(quantity));
  if (isNaN(entry) || isNaN(current) || isNaN(qty)) return '0.0000';

  const pnl = side === 'long'
    ? (current - entry) * qty
    : (entry - current) * qty;

  return pnl.toFixed(4);
}

/**
 * Calculate total P&L (realized + unrealized) across all positions.
 */
export function calculateTotalPnl(positions: Position[]): {
  totalRealized: string;
  totalUnrealized: string;
  totalPnl: string;
} {
  let realized = 0;
  let unrealized = 0;

  for (const pos of positions) {
    realized += Number(pos.realizedPnl) || 0;
    unrealized += Number(pos.unrealizedPnl) || 0;
  }

  return {
    totalRealized: realized.toFixed(4),
    totalUnrealized: unrealized.toFixed(4),
    totalPnl: (realized + unrealized).toFixed(4),
  };
}

/**
 * Format P&L value with sign prefix.
 */
export function formatPnl(value: string): string {
  const num = Number(value);
  if (isNaN(num)) return value;
  if (num > 0) return `+${num.toFixed(4)}`;
  return num.toFixed(4);
}

/**
 * Determine P&L color class: positive, negative, or neutral.
 */
export function pnlColorClass(value: string): 'positive' | 'negative' | 'neutral' {
  const num = Number(value);
  if (isNaN(num) || num === 0) return 'neutral';
  return num > 0 ? 'positive' : 'negative';
}

/**
 * Filter trade records by date range, instrument, and side.
 */
export function filterTrades(
  trades: TradeRecord[],
  filter: TradeHistoryFilter,
): TradeRecord[] {
  return trades.filter((trade) => {
    if (filter.instrumentId && trade.instrumentId !== filter.instrumentId) {
      return false;
    }
    if (filter.side && trade.side !== filter.side) {
      return false;
    }
    if (filter.startDate) {
      const start = new Date(filter.startDate);
      const tradeDate = new Date(trade.timestamp);
      if (!isNaN(start.getTime()) && tradeDate < start) return false;
    }
    if (filter.endDate) {
      const end = new Date(filter.endDate);
      // Include the full end day
      end.setHours(23, 59, 59, 999);
      const tradeDate = new Date(trade.timestamp);
      if (!isNaN(end.getTime()) && tradeDate > end) return false;
    }
    return true;
  });
}

/**
 * Build a CSV string from trade records.
 */
export function buildTradesCsv(trades: TradeRecord[]): string {
  const header = 'Time,Instrument,Side,Quantity,Price,Total Value';
  const rows = trades.map((t) =>
    `${t.timestamp},${t.instrumentSymbol},${t.side},${t.quantity},${t.price},${t.totalValue}`
  );
  return [header, ...rows].join('\n');
}

/**
 * Trigger CSV download in the browser.
 * Separated from buildTradesCsv for testability: buildTradesCsv is pure,
 * exportTradesCsv depends on DOM.
 */
export function exportTradesCsv(trades: TradeRecord[], filename: string): void {
  const csv = buildTradesCsv(trades);
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

/**
 * Format a timestamp for display in trade history table.
 */
export function formatTradeTime(timestamp: string): string {
  try {
    const d = new Date(timestamp);
    if (isNaN(d.getTime())) return timestamp;
    return d.toLocaleString('en-US', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
  } catch {
    return timestamp;
  }
}

/**
 * Calculate total value for a trade: price * quantity.
 */
export function calculateTradeValue(price: string, quantity: string): string {
  const p = Number(price);
  const q = Number(quantity);
  if (isNaN(p) || isNaN(q)) return '0.0000';
  return (p * q).toFixed(4);
}
