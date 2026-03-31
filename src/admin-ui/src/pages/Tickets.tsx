import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchTickets, fetchTicket, updateTicket, addTicketComment } from '../services/api';
import styles from './Tickets.module.css';

// --- Types ---

export interface Ticket {
  id: string;
  title: string;
  category: string;
  priority: string;
  status: string;
  reporter_name: string;
  reporter_email: string;
  description: string;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface TicketComment {
  id: string;
  ticket_id: string;
  author_name: string;
  body: string;
  is_bot: boolean;
  created_at: string;
}

// --- Pure functions (exported for testing) ---

export function priorityClassName(priority: string): string {
  switch (priority) {
    case 'critical': return 'priorityCritical';
    case 'high': return 'priorityHigh';
    case 'medium': return 'priorityMedium';
    case 'low': return 'priorityLow';
    default: return '';
  }
}

export function ticketStatusClassName(status: string): string {
  switch (status) {
    case 'open': return 'statusOpen';
    case 'in_progress': return 'statusInProgress';
    case 'resolved': return 'statusResolved';
    case 'closed': return 'statusClosed';
    default: return '';
  }
}

export function categoryLabel(category: string): string {
  switch (category) {
    case 'bug_report': return 'Bug Report';
    case 'customization': return 'Customization';
    case 'support': return 'Support';
    case 'feature_request': return 'Feature Request';
    default: return category.replace(/_/g, ' ');
  }
}

export function priorityLabel(priority: string): string {
  switch (priority) {
    case 'critical': return 'Critical';
    case 'high': return 'High';
    case 'medium': return 'Medium';
    case 'low': return 'Low';
    default: return priority;
  }
}

export function statusLabel(status: string): string {
  switch (status) {
    case 'open': return 'Open';
    case 'in_progress': return 'In Progress';
    case 'resolved': return 'Resolved';
    case 'closed': return 'Closed';
    default: return status.replace(/_/g, ' ');
  }
}

export function formatTicketTime(isoString: string): string {
  if (!isoString) return '';
  try {
    return new Date(isoString).toLocaleString();
  } catch {
    return isoString;
  }
}

export function truncateId(id: string, maxLen = 8): string {
  if (!id) return '';
  if (id.length <= maxLen) return id;
  return id.slice(0, maxLen) + '...';
}

export function filterTickets(
  tickets: Ticket[],
  categoryFilter: string,
  priorityFilter: string,
  statusFilter: string,
  searchQuery: string,
): Ticket[] {
  const query = searchQuery.toLowerCase().trim();
  return tickets.filter(t => {
    if (categoryFilter && t.category !== categoryFilter) return false;
    if (priorityFilter && t.priority !== priorityFilter) return false;
    if (statusFilter && t.status !== statusFilter) return false;
    if (query) {
      const searchable = `${t.id} ${t.title} ${t.reporter_name} ${t.reporter_email} ${t.description}`.toLowerCase();
      if (!searchable.includes(query)) return false;
    }
    return true;
  });
}

export function sortTickets(tickets: Ticket[]): Ticket[] {
  const priorityOrder: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 };
  return [...tickets].sort((a, b) => {
    const pDiff = (priorityOrder[a.priority] ?? 4) - (priorityOrder[b.priority] ?? 4);
    if (pDiff !== 0) return pDiff;
    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
  });
}

export function computeStatusCounts(tickets: Ticket[]): Record<string, number> {
  const counts: Record<string, number> = { open: 0, in_progress: 0, resolved: 0, closed: 0 };
  tickets.forEach(t => {
    if (counts[t.status] !== undefined) {
      counts[t.status]++;
    }
  });
  return counts;
}

// --- Component ---

export function TicketsPage() {
  const [categoryFilter, setCategoryFilter] = useState('');
  const [priorityFilter, setPriorityFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [ticketDetail, setTicketDetail] = useState<{ ticket: Ticket; comments: TicketComment[] } | null>(null);
  const [commentText, setCommentText] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const { data, refresh } = usePolling(
    (signal) => fetchTickets({
      category: categoryFilter || undefined,
      priority: priorityFilter || undefined,
      status: statusFilter || undefined,
    }, signal),
    30000,
  );

  const rawTickets: Ticket[] = data?.data ?? [];
  const filtered = filterTickets(rawTickets, categoryFilter, priorityFilter, statusFilter, searchQuery);
  const tickets = sortTickets(filtered);
  const counts = computeStatusCounts(rawTickets);

  const handleRowClick = async (ticket: Ticket) => {
    if (expandedId === ticket.id) {
      setExpandedId(null);
      setTicketDetail(null);
      setCommentText('');
      return;
    }
    setExpandedId(ticket.id);
    try {
      const detail = await fetchTicket(ticket.id);
      setTicketDetail(detail);
    } catch {
      setTicketDetail({ ticket, comments: [] });
    }
  };

  const handleAssign = async () => {
    if (!expandedId) return;
    setSubmitting(true);
    try {
      await updateTicket(expandedId, { status: 'in_progress' });
      refresh();
    } finally {
      setSubmitting(false);
    }
  };

  const handleStatusChange = async (newStatus: string) => {
    if (!expandedId) return;
    setSubmitting(true);
    try {
      await updateTicket(expandedId, { status: newStatus });
      refresh();
    } finally {
      setSubmitting(false);
    }
  };

  const handlePriorityChange = async (newPriority: string) => {
    if (!expandedId) return;
    setSubmitting(true);
    try {
      await updateTicket(expandedId, { priority: newPriority });
      refresh();
    } finally {
      setSubmitting(false);
    }
  };

  const handleAddComment = async () => {
    if (!expandedId || !commentText.trim()) return;
    setSubmitting(true);
    try {
      const comment = await addTicketComment(expandedId, commentText.trim());
      setTicketDetail(prev => prev ? { ...prev, comments: [...prev.comments, comment] } : prev);
      setCommentText('');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div>
      <h1>Tickets</h1>

      <div className={styles.statsRow}>
        {(['open', 'in_progress', 'resolved', 'closed'] as const).map(status => (
          <div key={status} className={styles.statCard}>
            <div className={styles.statValue}>{counts[status]}</div>
            <div className={styles.statLabel}>{statusLabel(status)}</div>
          </div>
        ))}
      </div>

      <div className={styles.topRow}>
        <div className={styles.filters}>
          <select value={categoryFilter} onChange={e => setCategoryFilter(e.target.value)} className={styles.select}>
            <option value="">All Categories</option>
            <option value="bug_report">Bug Report</option>
            <option value="customization">Customization</option>
            <option value="support">Support</option>
            <option value="feature_request">Feature Request</option>
          </select>
          <select value={priorityFilter} onChange={e => setPriorityFilter(e.target.value)} className={styles.select}>
            <option value="">All Priorities</option>
            <option value="critical">Critical</option>
            <option value="high">High</option>
            <option value="medium">Medium</option>
            <option value="low">Low</option>
          </select>
          <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)} className={styles.select}>
            <option value="">All Status</option>
            <option value="open">Open</option>
            <option value="in_progress">In Progress</option>
            <option value="resolved">Resolved</option>
            <option value="closed">Closed</option>
          </select>
          <input
            type="text"
            placeholder="Search tickets..."
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            className={styles.searchInput}
          />
        </div>
      </div>

      <table className={styles.table}>
        <thead>
          <tr>
            <th>ID</th>
            <th>Title</th>
            <th>Category</th>
            <th>Priority</th>
            <th>Status</th>
            <th>Reporter</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          {tickets.map(ticket => (
            <React.Fragment key={ticket.id}>
              <tr
                onClick={() => handleRowClick(ticket)}
                className={expandedId === ticket.id ? styles.expandedRow : undefined}
              >
                <td className={styles.monoCell}>{truncateId(ticket.id)}</td>
                <td>{ticket.title}</td>
                <td>{categoryLabel(ticket.category)}</td>
                <td>
                  <span className={`${styles.priorityBadge} ${styles[priorityClassName(ticket.priority)]}`}>
                    {priorityLabel(ticket.priority)}
                  </span>
                </td>
                <td>
                  <span className={`${styles.statusBadge} ${styles[ticketStatusClassName(ticket.status)]}`}>
                    {statusLabel(ticket.status)}
                  </span>
                </td>
                <td>{ticket.reporter_name}</td>
                <td>{formatTicketTime(ticket.created_at)}</td>
              </tr>
              {expandedId === ticket.id && (
                <tr className={styles.detailRow}>
                  <td colSpan={7}>
                    <div className={styles.detailPanel}>
                      <div className={styles.detailGrid}>
                        <div className={styles.detailField}>
                          <span className={styles.detailLabel}>Description</span>
                          <span className={styles.detailValue}>
                            {ticketDetail?.ticket.description ?? ticket.description}
                          </span>
                        </div>
                        {ticketDetail?.ticket.metadata && Object.keys(ticketDetail.ticket.metadata).length > 0 && (
                          <div className={styles.detailField}>
                            <span className={styles.detailLabel}>Metadata</span>
                            <pre className={styles.metadata}>
                              {JSON.stringify(ticketDetail.ticket.metadata, null, 2)}
                            </pre>
                          </div>
                        )}
                      </div>

                      <div className={styles.actions}>
                        <button
                          className={styles.assignBtn}
                          onClick={handleAssign}
                          disabled={submitting}
                        >
                          Assign to me
                        </button>
                        <select
                          value={ticketDetail?.ticket.status ?? ticket.status}
                          onChange={e => handleStatusChange(e.target.value)}
                          className={styles.select}
                          disabled={submitting}
                        >
                          <option value="open">Open</option>
                          <option value="in_progress">In Progress</option>
                          <option value="resolved">Resolved</option>
                          <option value="closed">Closed</option>
                        </select>
                        <select
                          value={ticketDetail?.ticket.priority ?? ticket.priority}
                          onChange={e => handlePriorityChange(e.target.value)}
                          className={styles.select}
                          disabled={submitting}
                        >
                          <option value="critical">Critical</option>
                          <option value="high">High</option>
                          <option value="medium">Medium</option>
                          <option value="low">Low</option>
                        </select>
                      </div>

                      <div className={styles.commentsSection}>
                        <span className={styles.detailLabel}>Comments ({ticketDetail?.comments.length ?? 0})</span>
                        <div className={styles.commentsList}>
                          {(ticketDetail?.comments ?? []).map(comment => (
                            <div key={comment.id} className={styles.comment}>
                              <div className={styles.commentHeader}>
                                <span className={styles.commentAuthor}>
                                  {comment.is_bot ? '\u{1F916} ' : ''}{comment.author_name}
                                </span>
                                <span className={styles.commentTime}>
                                  {formatTicketTime(comment.created_at)}
                                </span>
                              </div>
                              <div className={styles.commentBody}>{comment.body}</div>
                            </div>
                          ))}
                        </div>
                        <div className={styles.commentForm}>
                          <textarea
                            className={styles.commentInput}
                            placeholder="Add a comment..."
                            value={commentText}
                            onChange={e => setCommentText(e.target.value)}
                            rows={3}
                          />
                          <button
                            className={styles.commentBtn}
                            onClick={handleAddComment}
                            disabled={submitting || !commentText.trim()}
                          >
                            Add Comment
                          </button>
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              )}
            </React.Fragment>
          ))}
        </tbody>
      </table>

      {tickets.length === 0 && (
        <div className={styles.empty}>No tickets found</div>
      )}
    </div>
  );
}
