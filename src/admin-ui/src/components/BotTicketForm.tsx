import React, { useState } from 'react';
import { useBot } from '../contexts/BotContext';
import { createTicket } from '../services/botApi';
import styles from './BotTicketForm.module.css';

export const TICKET_CATEGORIES = [
  { value: 'bug_report', label: 'Bug Report' },
  { value: 'customization', label: 'Customization' },
  { value: 'support', label: 'Support' },
  { value: 'feature_request', label: 'Feature Request' },
] as const;

export const TICKET_PRIORITIES = [
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'critical', label: 'Critical' },
] as const;

export function getCategoryFromTrigger(trigger: string): string {
  if (trigger === 'bug_report') return 'bug_report';
  if (trigger === 'feature_request') return 'feature_request';
  return 'support';
}

export function captureTicketMetadata(): { page: string; userAgent: string; timestamp: string } {
  return {
    page: typeof window !== 'undefined' ? window.location.pathname : '',
    userAgent: typeof navigator !== 'undefined' ? navigator.userAgent : '',
    timestamp: new Date().toISOString(),
  };
}

export function validateTicketForm(title: string, description: string): { valid: boolean; errors: string[] } {
  const errors: string[] = [];
  if (!title.trim()) errors.push('Title is required');
  if (!description.trim()) errors.push('Description is required');
  if (title.trim().length < 3) errors.push('Title must be at least 3 characters');
  return { valid: errors.length === 0, errors };
}

export function BotTicketForm() {
  const { state, hideTicketForm, sendMessage } = useBot();
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [category, setCategory] = useState(getCategoryFromTrigger(state.ticketCategory));
  const [priority, setPriority] = useState('medium');
  const [submitting, setSubmitting] = useState(false);

  const { valid } = validateTicketForm(title, description);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid) return;

    setSubmitting(true);
    try {
      const metadata = captureTicketMetadata();
      const result = await createTicket({
        title: title.trim(),
        description: `${description.trim()}\n\n---\nPage: ${metadata.page}\nUser Agent: ${metadata.userAgent}\nTimestamp: ${metadata.timestamp}`,
        category,
        priority,
      });
      hideTicketForm();
      sendMessage(`Ticket created successfully: #${result.id}`);
    } catch {
      hideTicketForm();
      sendMessage('Failed to create ticket. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form className={styles.form} onSubmit={handleSubmit}>
      <div className={styles.formTitle}>
        {category === 'bug_report' ? 'Report a Bug' : 'Request a Feature'}
      </div>

      <div>
        <label className={styles.fieldLabel}>Title</label>
        <input
          className={styles.fieldInput}
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Brief summary..."
          required
        />
      </div>

      <div>
        <label className={styles.fieldLabel}>Description</label>
        <textarea
          className={styles.fieldTextarea}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Detailed description..."
          required
        />
      </div>

      <div>
        <label className={styles.fieldLabel}>Category</label>
        <select
          className={styles.fieldSelect}
          value={category}
          onChange={(e) => setCategory(e.target.value)}
        >
          {TICKET_CATEGORIES.map((c) => (
            <option key={c.value} value={c.value}>
              {c.label}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className={styles.fieldLabel}>Priority</label>
        <select
          className={styles.fieldSelect}
          value={priority}
          onChange={(e) => setPriority(e.target.value)}
        >
          {TICKET_PRIORITIES.map((p) => (
            <option key={p.value} value={p.value}>
              {p.label}
            </option>
          ))}
        </select>
      </div>

      <div className={styles.formActions}>
        <button
          className={styles.submitBtn}
          type="submit"
          disabled={!valid || submitting}
        >
          {submitting ? 'Submitting...' : 'Submit'}
        </button>
        <button
          className={styles.cancelBtn}
          type="button"
          onClick={hideTicketForm}
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
