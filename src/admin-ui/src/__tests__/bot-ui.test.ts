import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  botReducer,
  initialBotState,
  formatMessageTime,
  generateMessageId,
  filterSuggestionsByQuery,
  BotState,
} from '../contexts/BotContext';
import {
  buildBotUrl,
  buildAuthHeaders,
  buildSendMessageBody,
  buildCreateTicketBody,
} from '../services/botApi';
import {
  getCategoryFromTrigger,
  captureTicketMetadata,
  validateTicketForm,
  TICKET_CATEGORIES,
  TICKET_PRIORITIES,
} from '../components/BotTicketForm';

// --- botApi pure functions ---

describe('buildBotUrl', () => {
  it('constructs chat endpoint URL', () => {
    const url = buildBotUrl('/chat');
    expect(url).toContain('/bot/chat');
  });

  it('constructs suggestions endpoint URL', () => {
    const url = buildBotUrl('/suggestions?page=dashboard');
    expect(url).toContain('/bot/suggestions?page=dashboard');
  });

  it('constructs tickets endpoint URL', () => {
    const url = buildBotUrl('/tickets');
    expect(url).toContain('/bot/tickets');
  });
});

describe('buildAuthHeaders', () => {
  it('includes Content-Type header', () => {
    const headers = buildAuthHeaders();
    expect(headers['Content-Type']).toBe('application/json');
  });

  it('returns headers object', () => {
    const headers = buildAuthHeaders();
    expect(typeof headers).toBe('object');
    expect(headers['Content-Type']).toBeDefined();
  });
});

describe('buildSendMessageBody', () => {
  it('serializes message without context', () => {
    const body = buildSendMessageBody('hello');
    const parsed = JSON.parse(body);
    expect(parsed.message).toBe('hello');
    expect(parsed.context).toEqual({});
  });

  it('serializes message with page context', () => {
    const body = buildSendMessageBody('help', { page: '/dashboard/monitoring' });
    const parsed = JSON.parse(body);
    expect(parsed.message).toBe('help');
    expect(parsed.context.page).toBe('/dashboard/monitoring');
  });

  it('handles empty message', () => {
    const body = buildSendMessageBody('');
    const parsed = JSON.parse(body);
    expect(parsed.message).toBe('');
  });
});

describe('buildCreateTicketBody', () => {
  it('serializes ticket input', () => {
    const ticket = {
      title: 'Bug in dashboard',
      description: 'Charts not loading',
      category: 'bug_report',
      priority: 'high',
    };
    const body = buildCreateTicketBody(ticket);
    const parsed = JSON.parse(body);
    expect(parsed.title).toBe('Bug in dashboard');
    expect(parsed.description).toBe('Charts not loading');
    expect(parsed.category).toBe('bug_report');
    expect(parsed.priority).toBe('high');
  });
});

// --- BotContext reducer ---

describe('botReducer', () => {
  it('TOGGLE_PANEL opens when closed', () => {
    const state = botReducer(initialBotState, { type: 'TOGGLE_PANEL' });
    expect(state.isOpen).toBe(true);
  });

  it('TOGGLE_PANEL closes when open', () => {
    const openState: BotState = { ...initialBotState, isOpen: true };
    const state = botReducer(openState, { type: 'TOGGLE_PANEL' });
    expect(state.isOpen).toBe(false);
  });

  it('TOGGLE_PANEL clears unread when closing', () => {
    const openState: BotState = { ...initialBotState, isOpen: true, unreadCount: 3 };
    const state = botReducer(openState, { type: 'TOGGLE_PANEL' });
    expect(state.unreadCount).toBe(0);
  });

  it('CLOSE_PANEL closes the panel', () => {
    const openState: BotState = { ...initialBotState, isOpen: true };
    const state = botReducer(openState, { type: 'CLOSE_PANEL' });
    expect(state.isOpen).toBe(false);
  });

  it('ADD_USER_MESSAGE appends user message', () => {
    const state = botReducer(initialBotState, {
      type: 'ADD_USER_MESSAGE',
      payload: { id: 'msg-1', content: 'hello', timestamp: 1000 },
    });
    expect(state.messages).toHaveLength(1);
    expect(state.messages[0].role).toBe('user');
    expect(state.messages[0].content).toBe('hello');
    expect(state.messages[0].id).toBe('msg-1');
  });

  it('ADD_BOT_MESSAGE appends bot message', () => {
    const state = botReducer(initialBotState, {
      type: 'ADD_BOT_MESSAGE',
      payload: { id: 'msg-2', content: 'Hi there!', timestamp: 2000 },
    });
    expect(state.messages).toHaveLength(1);
    expect(state.messages[0].role).toBe('bot');
    expect(state.messages[0].content).toBe('Hi there!');
  });

  it('ADD_BOT_MESSAGE increments unread when panel is closed', () => {
    const state = botReducer(initialBotState, {
      type: 'ADD_BOT_MESSAGE',
      payload: { id: 'msg-2', content: 'notification', timestamp: 2000 },
    });
    expect(state.unreadCount).toBe(1);
  });

  it('ADD_BOT_MESSAGE does not increment unread when panel is open', () => {
    const openState: BotState = { ...initialBotState, isOpen: true };
    const state = botReducer(openState, {
      type: 'ADD_BOT_MESSAGE',
      payload: { id: 'msg-2', content: 'notification', timestamp: 2000 },
    });
    expect(state.unreadCount).toBe(0);
  });

  it('ADD_BOT_MESSAGE includes actions', () => {
    const actions = [{ id: 'a1', label: 'View', type: 'link' as const, payload: '/dashboard' }];
    const state = botReducer(initialBotState, {
      type: 'ADD_BOT_MESSAGE',
      payload: { id: 'msg-3', content: 'Check this', timestamp: 3000, actions },
    });
    expect(state.messages[0].actions).toHaveLength(1);
    expect(state.messages[0].actions![0].label).toBe('View');
  });

  it('SET_TYPING sets isTyping', () => {
    const state = botReducer(initialBotState, { type: 'SET_TYPING', payload: true });
    expect(state.isTyping).toBe(true);
  });

  it('CLEAR_UNREAD resets unread count', () => {
    const withUnread: BotState = { ...initialBotState, unreadCount: 5 };
    const state = botReducer(withUnread, { type: 'CLEAR_UNREAD' });
    expect(state.unreadCount).toBe(0);
  });

  it('SET_SUGGESTIONS updates suggestions list', () => {
    const suggestions = [{ id: 's1', label: 'Help', prompt: 'How can I help?' }];
    const state = botReducer(initialBotState, {
      type: 'SET_SUGGESTIONS',
      payload: suggestions,
    });
    expect(state.suggestions).toHaveLength(1);
    expect(state.suggestions[0].label).toBe('Help');
  });

  it('SHOW_TICKET_FORM opens ticket form with category', () => {
    const state = botReducer(initialBotState, {
      type: 'SHOW_TICKET_FORM',
      payload: { category: 'bug_report' },
    });
    expect(state.showTicketForm).toBe(true);
    expect(state.ticketCategory).toBe('bug_report');
  });

  it('HIDE_TICKET_FORM closes ticket form', () => {
    const formOpen: BotState = { ...initialBotState, showTicketForm: true };
    const state = botReducer(formOpen, { type: 'HIDE_TICKET_FORM' });
    expect(state.showTicketForm).toBe(false);
  });

  it('returns current state for unknown action', () => {
    const state = botReducer(initialBotState, { type: 'UNKNOWN' } as never);
    expect(state).toBe(initialBotState);
  });
});

// --- formatMessageTime ---

describe('formatMessageTime', () => {
  it('formats timestamp as HH:MM', () => {
    // 2026-01-15 14:30:00 UTC
    const ts = new Date(2026, 0, 15, 14, 30, 0).getTime();
    const result = formatMessageTime(ts);
    expect(result).toBe('14:30');
  });

  it('pads single-digit hours', () => {
    const ts = new Date(2026, 0, 15, 9, 5, 0).getTime();
    const result = formatMessageTime(ts);
    expect(result).toBe('09:05');
  });

  it('handles midnight', () => {
    const ts = new Date(2026, 0, 15, 0, 0, 0).getTime();
    const result = formatMessageTime(ts);
    expect(result).toBe('00:00');
  });
});

// --- generateMessageId ---

describe('generateMessageId', () => {
  it('starts with msg- prefix', () => {
    const id = generateMessageId();
    expect(id).toMatch(/^msg-/);
  });

  it('generates unique IDs', () => {
    const ids = new Set(Array.from({ length: 10 }, () => generateMessageId()));
    expect(ids.size).toBe(10);
  });
});

// --- filterSuggestionsByQuery ---

describe('filterSuggestionsByQuery', () => {
  const suggestions = [
    { id: '1', label: 'System Health', prompt: 'Show system health' },
    { id: '2', label: 'Order Book', prompt: 'Show order book status' },
    { id: '3', label: 'Recent Trades', prompt: 'List recent trades' },
  ];

  it('returns all suggestions for empty query', () => {
    const result = filterSuggestionsByQuery(suggestions, '');
    expect(result).toHaveLength(3);
  });

  it('filters by label match', () => {
    const result = filterSuggestionsByQuery(suggestions, 'order');
    expect(result).toHaveLength(1);
    expect(result[0].label).toBe('Order Book');
  });

  it('filters by prompt match', () => {
    const result = filterSuggestionsByQuery(suggestions, 'trades');
    expect(result).toHaveLength(1);
    expect(result[0].label).toBe('Recent Trades');
  });

  it('is case insensitive', () => {
    const result = filterSuggestionsByQuery(suggestions, 'SYSTEM');
    expect(result).toHaveLength(1);
  });

  it('returns empty array when nothing matches', () => {
    const result = filterSuggestionsByQuery(suggestions, 'zzzzz');
    expect(result).toHaveLength(0);
  });
});

// --- Ticket form pure functions ---

describe('getCategoryFromTrigger', () => {
  it('maps bug_report trigger', () => {
    expect(getCategoryFromTrigger('bug_report')).toBe('bug_report');
  });

  it('maps feature_request trigger', () => {
    expect(getCategoryFromTrigger('feature_request')).toBe('feature_request');
  });

  it('defaults to support for unknown trigger', () => {
    expect(getCategoryFromTrigger('other')).toBe('support');
    expect(getCategoryFromTrigger('')).toBe('support');
  });
});

describe('captureTicketMetadata', () => {
  it('returns object with page, userAgent, and timestamp', () => {
    const metadata = captureTicketMetadata();
    expect(metadata).toHaveProperty('page');
    expect(metadata).toHaveProperty('userAgent');
    expect(metadata).toHaveProperty('timestamp');
  });

  it('timestamp is valid ISO string', () => {
    const metadata = captureTicketMetadata();
    const parsed = new Date(metadata.timestamp);
    expect(parsed.toISOString()).toBe(metadata.timestamp);
  });
});

describe('validateTicketForm', () => {
  it('valid when title and description provided', () => {
    const result = validateTicketForm('Bug title', 'Description here');
    expect(result.valid).toBe(true);
    expect(result.errors).toHaveLength(0);
  });

  it('invalid when title is empty', () => {
    const result = validateTicketForm('', 'Description here');
    expect(result.valid).toBe(false);
    expect(result.errors).toContain('Title is required');
  });

  it('invalid when description is empty', () => {
    const result = validateTicketForm('Title', '');
    expect(result.valid).toBe(false);
    expect(result.errors).toContain('Description is required');
  });

  it('invalid when title is too short', () => {
    const result = validateTicketForm('ab', 'Description');
    expect(result.valid).toBe(false);
    expect(result.errors).toContain('Title must be at least 3 characters');
  });

  it('valid when title is exactly 3 characters', () => {
    const result = validateTicketForm('abc', 'Description');
    expect(result.valid).toBe(true);
  });

  it('reports multiple errors', () => {
    const result = validateTicketForm('', '');
    expect(result.valid).toBe(false);
    expect(result.errors.length).toBeGreaterThanOrEqual(2);
  });
});

// --- Constants ---

describe('TICKET_CATEGORIES', () => {
  it('has 4 categories', () => {
    expect(TICKET_CATEGORIES).toHaveLength(4);
  });

  it('includes bug_report and feature_request', () => {
    const values = TICKET_CATEGORIES.map((c) => c.value);
    expect(values).toContain('bug_report');
    expect(values).toContain('feature_request');
    expect(values).toContain('customization');
    expect(values).toContain('support');
  });
});

describe('TICKET_PRIORITIES', () => {
  it('has 4 priorities', () => {
    expect(TICKET_PRIORITIES).toHaveLength(4);
  });

  it('includes all priority levels', () => {
    const values = TICKET_PRIORITIES.map((p) => p.value);
    expect(values).toEqual(['low', 'medium', 'high', 'critical']);
  });
});
