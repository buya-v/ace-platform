import React, { useEffect, useRef, useState, useCallback } from 'react';
import type { Candle } from '../types/trade';
import { apiRequest } from '../services/api';
import styles from './Chart.module.css';

interface ChartProps {
  instrumentId: string | null;
}

type Timeframe = '1m' | '5m' | '15m' | '1h' | '4h' | '1d';

const TIMEFRAMES: Timeframe[] = ['1m', '5m', '15m', '1h', '4h', '1d'];

function drawCandlestick(
  ctx: CanvasRenderingContext2D,
  candles: Candle[],
  width: number,
  height: number,
) {
  if (candles.length === 0) return;

  ctx.clearRect(0, 0, width, height);

  const padding = { top: 20, bottom: 30, left: 60, right: 20 };
  const chartW = width - padding.left - padding.right;
  const chartH = height - padding.top - padding.bottom;

  const allPrices = candles.flatMap((c) => [c.high, c.low]);
  const minPrice = Math.min(...allPrices);
  const maxPrice = Math.max(...allPrices);
  const priceRange = maxPrice - minPrice || 1;

  const candleWidth = Math.max(2, chartW / candles.length - 2);

  const priceToY = (p: number) =>
    padding.top + chartH - ((p - minPrice) / priceRange) * chartH;

  // Grid lines
  ctx.strokeStyle = '#333';
  ctx.lineWidth = 0.5;
  const gridLines = 5;
  for (let i = 0; i <= gridLines; i++) {
    const y = padding.top + (chartH / gridLines) * i;
    ctx.beginPath();
    ctx.moveTo(padding.left, y);
    ctx.lineTo(width - padding.right, y);
    ctx.stroke();

    const price = maxPrice - (priceRange / gridLines) * i;
    ctx.fillStyle = '#888';
    ctx.font = '10px monospace';
    ctx.textAlign = 'right';
    ctx.fillText(price.toFixed(2), padding.left - 4, y + 4);
  }

  // Candles
  candles.forEach((candle, i) => {
    const x = padding.left + (chartW / candles.length) * i + candleWidth / 2;
    const isGreen = candle.close >= candle.open;

    // Wick
    ctx.strokeStyle = isGreen ? '#2ecc71' : '#e74c3c';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x, priceToY(candle.high));
    ctx.lineTo(x, priceToY(candle.low));
    ctx.stroke();

    // Body
    const bodyTop = priceToY(Math.max(candle.open, candle.close));
    const bodyBottom = priceToY(Math.min(candle.open, candle.close));
    const bodyHeight = Math.max(1, bodyBottom - bodyTop);

    ctx.fillStyle = isGreen ? '#2ecc71' : '#e74c3c';
    ctx.fillRect(x - candleWidth / 2, bodyTop, candleWidth, bodyHeight);
  });
}

export const Chart: React.FC<ChartProps> = ({ instrumentId }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [timeframe, setTimeframe] = useState<Timeframe>('15m');
  const [candles, setCandles] = useState<Candle[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchCandles = useCallback(async () => {
    if (!instrumentId) return;
    setLoading(true);
    try {
      const now = Date.now();
      const from = new Date(now - 24 * 60 * 60 * 1000).toISOString();
      const to = new Date(now).toISOString();
      const data = await apiRequest<{ candles: Candle[] }>(
        `/market-data/candles?instrument_id=${instrumentId}&interval=${timeframe}&from=${from}&to=${to}`,
      );
      setCandles(data.candles || []);
    } catch {
      // Use empty candles on error
      setCandles([]);
    } finally {
      setLoading(false);
    }
  }, [instrumentId, timeframe]);

  useEffect(() => {
    fetchCandles();
  }, [fetchCandles]);

  useEffect(() => {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const resizeObserver = new ResizeObserver(() => {
      const rect = container.getBoundingClientRect();
      canvas.width = rect.width;
      canvas.height = rect.height;
      const ctx = canvas.getContext('2d');
      if (ctx) drawCandlestick(ctx, candles, canvas.width, canvas.height);
    });

    resizeObserver.observe(container);
    return () => resizeObserver.disconnect();
  }, [candles]);

  return (
    <div className={styles.chart}>
      <div className={styles.toolbar}>
        {TIMEFRAMES.map((tf) => (
          <button
            key={tf}
            className={`${styles.tfBtn} ${timeframe === tf ? styles.tfActive : ''}`}
            onClick={() => setTimeframe(tf)}
          >
            {tf}
          </button>
        ))}
      </div>
      <div className={styles.canvasContainer} ref={containerRef}>
        {loading && <div className={styles.loading}>Loading chart...</div>}
        <canvas ref={canvasRef} />
      </div>
    </div>
  );
};
