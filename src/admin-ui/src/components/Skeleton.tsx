import React from 'react';
import styles from './Skeleton.module.css';

interface SkeletonProps {
  width?: string;
  height?: string;
  variant?: 'text' | 'card' | 'circle';
  count?: number;
}

export function Skeleton({ width, height, variant = 'text', count = 1 }: SkeletonProps) {
  const variantClass = styles[variant];
  const elements = [];

  for (let i = 0; i < count; i++) {
    elements.push(
      <div
        key={i}
        className={variantClass}
        style={{
          width: width ?? (variant === 'circle' ? '32px' : '100%'),
          height: height ?? undefined,
        }}
      />
    );
  }

  return <>{elements}</>;
}
