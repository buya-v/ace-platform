import React from 'react';
import styles from './Sparkline.module.css';

export interface SparklineProps {
  data: number[];
  width?: number;
  height?: number;
  color?: string;
  fillColor?: string;
  className?: string;
}

export function Sparkline({
  data,
  width = 120,
  height = 32,
  color = 'var(--accent-blue)',
  fillColor,
  className,
}: SparklineProps) {
  if (data.length === 0) {
    return <svg width={width} height={height} className={className} />;
  }

  if (data.length === 1) {
    const cx = width / 2;
    const cy = height / 2;
    return (
      <svg width={width} height={height} className={`${styles.sparkline} ${className ?? ''}`}>
        <circle cx={cx} cy={cy} r={2} fill={color} />
      </svg>
    );
  }

  const padding = 2;
  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min || 1;

  const points = data.map((value, i) => {
    const x = (i / (data.length - 1)) * (width - padding * 2) + padding;
    const y = height - padding - ((value - min) / range) * (height - padding * 2);
    return { x, y };
  });

  const linePoints = points.map(p => `${p.x},${p.y}`).join(' ');

  const fillPoints = fillColor
    ? `${padding},${height - padding} ${linePoints} ${width - padding},${height - padding}`
    : undefined;

  return (
    <svg
      width={width}
      height={height}
      className={`${styles.sparkline} ${className ?? ''}`}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
    >
      {fillPoints && (
        <polygon points={fillPoints} fill={fillColor} opacity={0.2} />
      )}
      <polyline
        points={linePoints}
        fill="none"
        stroke={color}
        strokeWidth={1.5}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
