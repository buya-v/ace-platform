import React from 'react';
import { Action } from '../services/botApi';
import styles from './BotMessageCard.module.css';

export type SegmentType = 'success' | 'error' | 'warning' | 'bullet' | 'heading' | 'kv' | 'text';

export interface Segment {
  type: SegmentType;
  text: string;
  key?: string;
  value?: string;
}

/**
 * Classify a single line of bot reply text into a Segment.
 */
function classifyLine(line: string): Segment {
  // ✅ success
  if (line.startsWith('✅')) {
    return { type: 'success', text: line.slice(1).trimStart() };
  }
  // ❌ error
  if (line.startsWith('❌')) {
    return { type: 'error', text: line.slice(1).trimStart() };
  }
  // ⚠️ warning (multi-codepoint emoji — match both U+26A0 and compound forms)
  if (line.startsWith('⚠️') || line.startsWith('⚠')) {
    // Strip the emoji (up to 2 codepoints) and optional trailing variation selector
    const stripped = line.replace(/^⚠️?\s*/, '');
    return { type: 'warning', text: stripped };
  }
  // Bullet: • prefix or leading dash (-) with a space
  if (line.startsWith('•') || /^- /.test(line)) {
    const text = line.startsWith('•')
      ? line.slice(1).trimStart()
      : line.slice(2);
    return { type: 'bullet', text };
  }
  // Heading: **text** or # text
  if (/^\*\*.+\*\*$/.test(line.trim()) || line.startsWith('# ')) {
    const text = line.startsWith('# ')
      ? line.slice(2)
      : line.trim().replace(/^\*\*|\*\*$/g, '');
    return { type: 'heading', text };
  }
  // Key-Value: "Key: Value" — key is non-empty, separated by ": "
  const kvMatch = line.match(/^([^:]+):\s+(.+)$/);
  if (kvMatch) {
    return { type: 'kv', text: line, key: kvMatch[1], value: kvMatch[2] };
  }
  // Default
  return { type: 'text', text: line };
}

/**
 * Parse a multi-line bot reply into typed segments.
 * Empty lines are preserved as text segments so layout spacing is retained.
 */
export function parseMessageSegments(reply: string): Segment[] {
  if (!reply) return [];
  return reply.split('\n').map(classifyLine);
}

interface BotMessageCardProps {
  reply: string;
  actions?: Action[];
}

/**
 * Renders a bot reply as rich formatted segments, with optional pill action buttons.
 */
export function BotMessageCard({ reply, actions }: BotMessageCardProps) {
  const segments = parseMessageSegments(reply);

  const handleActionClick = (action: Action) => {
    if (action.type === 'link') {
      window.location.pathname = action.payload;
    }
  };

  return (
    <div className={styles.card}>
      {segments.map((seg, i) => {
        if (seg.type === 'kv') {
          return (
            <div key={i} className={`${styles.segment} ${styles.kv}`}>
              <span className={styles.kvKey}>{seg.key}:</span>
              <span className={styles.kvValue}>{seg.value}</span>
            </div>
          );
        }
        return (
          <div key={i} className={`${styles.segment} ${styles[seg.type]}`}>
            {seg.text}
          </div>
        );
      })}
      {actions && actions.length > 0 && (
        <div className={styles.actions}>
          {actions.map((action) => (
            <button
              key={action.id}
              className={styles.actionPill}
              onClick={() => handleActionClick(action)}
              type="button"
            >
              {action.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
