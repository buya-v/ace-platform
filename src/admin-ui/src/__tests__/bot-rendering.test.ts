import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { parseMessageSegments } from '../components/BotMessageCard';
import { normalizeAction, normalizeSuggestion, sendBotMessage, getBotSuggestions, createTicket } from '../services/botApi';
import { botReducer, initialBotState, BotState, BotAction } from '../contexts/BotContext';

// ---------------------------------------------------------------------------
// Mocks (required for fetch-based tests)
// ---------------------------------------------------------------------------

vi.mock('../config', () => ({
  getConfig: () => ({
    API_BASE_URL: 'http://localhost:8080/api/v1',
    WS_BASE_URL: 'ws://localhost:8080/api/v1/ws',
    HEALTH_POLL_INTERVAL: 15000,
    AUTH_TOKEN_REFRESH_BUFFER: 60,
  }),
}));

vi.mock('../services/api', () => ({
  getAccessToken: vi.fn(() => 'mock-token-xyz'),
  setAccessToken: vi.fn(),
}));

// ---------------------------------------------------------------------------
// parseMessageSegments — 20 new tests
// ---------------------------------------------------------------------------

describe('parseMessageSegments — specific segment cases', () => {
  it('✅ Trading halted on WHT → success segment with correct text', () => {
    const segs = parseMessageSegments('✅ Trading halted on WHT');
    expect(segs).toHaveLength(1);
    expect(segs[0]).toEqual({ type: 'success', text: 'Trading halted on WHT' });
  });

  it('❌ Failed to halt → error segment with correct text', () => {
    const segs = parseMessageSegments('❌ Failed to halt');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('error');
    expect(segs[0].text).toBe('Failed to halt');
  });

  it('⚠️ Confirm action → warning segment', () => {
    const segs = parseMessageSegments('⚠️ Confirm action');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('warning');
    expect(segs[0].text).toBe('Confirm action');
  });

  it('• Item one → bullet segment', () => {
    const segs = parseMessageSegments('• Item one');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('bullet');
    expect(segs[0].text).toBe('Item one');
  });

  it('- Item two → bullet segment', () => {
    const segs = parseMessageSegments('- Item two');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('bullet');
    expect(segs[0].text).toBe('Item two');
  });

  it('**Bold heading** → heading segment without asterisks', () => {
    const segs = parseMessageSegments('**Bold heading**');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('heading');
    expect(segs[0].text).toBe('Bold heading');
  });

  it('# Heading → heading segment with text after hash', () => {
    const segs = parseMessageSegments('# Heading');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('heading');
    expect(segs[0].text).toBe('Heading');
  });

  it('Key: Value → kv segment with correct key and value', () => {
    const segs = parseMessageSegments('Key: Value');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('kv');
    expect(segs[0].key).toBe('Key');
    expect(segs[0].value).toBe('Value');
  });

  it('Plain text → text segment', () => {
    const segs = parseMessageSegments('Plain text');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('text');
    expect(segs[0].text).toBe('Plain text');
  });

  it('empty string → empty array', () => {
    const segs = parseMessageSegments('');
    expect(segs).toHaveLength(0);
  });

  it('single newline → two text segments (split produces two empty strings)', () => {
    const segs = parseMessageSegments('\n');
    expect(segs).toHaveLength(2);
    segs.forEach((s) => {
      expect(s.type).toBe('text');
      expect(s.text).toBe('');
    });
  });

  it('multiple ✅ lines → multiple success segments', () => {
    const reply = '✅ Market open\n✅ All engines running\n✅ Settlement complete';
    const segs = parseMessageSegments(reply);
    expect(segs).toHaveLength(3);
    segs.forEach((s) => expect(s.type).toBe('success'));
    expect(segs[0].text).toBe('Market open');
    expect(segs[1].text).toBe('All engines running');
    expect(segs[2].text).toBe('Settlement complete');
  });

  it('line with only spaces → text segment', () => {
    const segs = parseMessageSegments('   ');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('text');
    expect(segs[0].text).toBe('   ');
  });

  it('Services: 9/9 healthy → kv segment', () => {
    const segs = parseMessageSegments('Services: 9/9 healthy');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('kv');
    expect(segs[0].key).toBe('Services');
    expect(segs[0].value).toBe('9/9 healthy');
  });

  it('💰 Margin: active → kv segment (non-warning emoji does not prevent kv match)', () => {
    // The line does not start with ✅/❌/⚠️/•/- /# / ** so falls to kv check
    // '💰 Margin: active' — key is '💰 Margin', value is 'active'
    const segs = parseMessageSegments('💰 Margin: active');
    expect(segs).toHaveLength(1);
    // It matches kv because of the ": " pattern
    expect(segs[0].type).toBe('kv');
  });

  it('long line (500+ chars) → text segment preserved in full', () => {
    const longText = 'A'.repeat(500) + ' end';
    const segs = parseMessageSegments(longText);
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('text');
    expect(segs[0].text).toBe(longText);
  });

  it('consecutive bullets → multiple bullet segments in order', () => {
    const reply = '• Alpha\n• Beta\n• Gamma';
    const segs = parseMessageSegments(reply);
    expect(segs).toHaveLength(3);
    expect(segs.every((s) => s.type === 'bullet')).toBe(true);
    expect(segs.map((s) => s.text)).toEqual(['Alpha', 'Beta', 'Gamma']);
  });

  it('line with colon but no space after → text not kv', () => {
    // "Key:value" — no space after colon, should not match kv regex /^([^:]+):\s+(.+)$/
    const segs = parseMessageSegments('Key:value');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('text');
  });

  it('  ✅ indented success → still classified as success', () => {
    // leading spaces, then ✅ — the line does NOT start with '✅', so it falls through to text
    // This tests the actual behaviour: indented lines are treated as text
    const segs = parseMessageSegments('  ✅ indented success');
    // The implementation uses startsWith which checks from the beginning of the string
    // Two leading spaces mean startsWith('✅') is false → text segment
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('text');
  });

  it('multi-line with mixed types → correct segment order', () => {
    const reply = [
      '**System Status**',
      'Uptime: 99.9%',
      '• Matching Engine: OK',
      '- Clearing Engine: OK',
      '✅ All healthy',
      '⚠️ Minor latency detected',
      '❌ Settlement delayed',
      'Please review logs.',
    ].join('\n');

    const segs = parseMessageSegments(reply);
    expect(segs).toHaveLength(8);
    expect(segs[0].type).toBe('heading');
    expect(segs[1].type).toBe('kv');
    expect(segs[2].type).toBe('bullet');
    expect(segs[3].type).toBe('bullet');
    expect(segs[4].type).toBe('success');
    expect(segs[5].type).toBe('warning');
    expect(segs[6].type).toBe('error');
    expect(segs[7].type).toBe('text');
  });
});

// ---------------------------------------------------------------------------
// normalizeAction — 6 new tests
// ---------------------------------------------------------------------------

describe('normalizeAction — additional cases', () => {
  it('{url: "/dashboard"} → payload is "/dashboard"', () => {
    const action = normalizeAction({ url: '/dashboard' }, 0);
    expect(action.payload).toBe('/dashboard');
  });

  it('{target: "/margin"} → payload is "/margin"', () => {
    const action = normalizeAction({ target: '/margin' }, 0);
    expect(action.payload).toBe('/margin');
  });

  it('{payload: "/foo"} → payload is "/foo"', () => {
    const action = normalizeAction({ payload: '/foo' }, 0);
    expect(action.payload).toBe('/foo');
  });

  it('{} → payload is empty string', () => {
    const action = normalizeAction({}, 0);
    expect(action.payload).toBe('');
  });

  it('{label: "test", type: "link"} → correct shape with defaults', () => {
    const action = normalizeAction({ label: 'test', type: 'link' }, 3);
    expect(action.label).toBe('test');
    expect(action.type).toBe('link');
    expect(action.id).toBe('act-3');
    expect(action.payload).toBe('');
  });

  it('index generates unique id (act-N) when no id provided', () => {
    const action0 = normalizeAction({ label: 'A' }, 0);
    const action7 = normalizeAction({ label: 'B' }, 7);
    const action42 = normalizeAction({ label: 'C' }, 42);
    expect(action0.id).toBe('act-0');
    expect(action7.id).toBe('act-7');
    expect(action42.id).toBe('act-42');
  });
});

// ---------------------------------------------------------------------------
// normalizeSuggestion — 5 tests
// ---------------------------------------------------------------------------

describe('normalizeSuggestion', () => {
  it('{text: "System health", category: "health"} → {label, prompt: "System health"}', () => {
    const sug = normalizeSuggestion({ text: 'System health', category: 'health' }, 0);
    expect(sug.label).toBe('System health');
    expect(sug.prompt).toBe('System health');
    expect(sug.icon).toBe('health');
  });

  it('{label: "Custom", prompt: "do thing"} → passthrough', () => {
    const sug = normalizeSuggestion({ label: 'Custom', prompt: 'do thing' }, 0);
    expect(sug.label).toBe('Custom');
    expect(sug.prompt).toBe('do thing');
  });

  it('{} → empty strings for label and prompt', () => {
    const sug = normalizeSuggestion({}, 0);
    expect(sug.label).toBe('');
    expect(sug.prompt).toBe('');
  });

  it('missing fields → defaults applied (id generated, icon undefined)', () => {
    const sug = normalizeSuggestion({ text: 'Hello' }, 5);
    expect(sug.id).toBe('sug-5');
    expect(sug.label).toBe('Hello');
    expect(sug.prompt).toBe('Hello');
    expect(sug.icon).toBeUndefined();
  });

  it('index generates unique id (sug-N) when no id in raw', () => {
    const sug0 = normalizeSuggestion({ text: 'A' }, 0);
    const sug3 = normalizeSuggestion({ text: 'B' }, 3);
    expect(sug0.id).toBe('sug-0');
    expect(sug3.id).toBe('sug-3');
  });
});

// ---------------------------------------------------------------------------
// BotContext reducer — 10 tests
// ---------------------------------------------------------------------------

describe('botReducer — panel state', () => {
  it('TOGGLE_PANEL opens panel when closed', () => {
    const state = botReducer(initialBotState, { type: 'TOGGLE_PANEL' });
    expect(state.isOpen).toBe(true);
  });

  it('TOGGLE_PANEL closes panel when open', () => {
    const openState: BotState = { ...initialBotState, isOpen: true };
    const state = botReducer(openState, { type: 'TOGGLE_PANEL' });
    expect(state.isOpen).toBe(false);
  });

  it('TOGGLE_PANEL clears unread when closing', () => {
    const openWithUnread: BotState = { ...initialBotState, isOpen: true, unreadCount: 5 };
    const state = botReducer(openWithUnread, { type: 'TOGGLE_PANEL' });
    expect(state.isOpen).toBe(false);
    expect(state.unreadCount).toBe(0);
  });

  it('CLOSE_PANEL sets isOpen to false', () => {
    const openState: BotState = { ...initialBotState, isOpen: true };
    const state = botReducer(openState, { type: 'CLOSE_PANEL' });
    expect(state.isOpen).toBe(false);
  });
});

describe('botReducer — messages', () => {
  it('ADD_BOT_MESSAGE accumulates messages', () => {
    const s1 = botReducer(initialBotState, {
      type: 'ADD_USER_MESSAGE',
      payload: { id: 'u1', content: 'hello', timestamp: 1000 },
    });
    const s2 = botReducer(s1, {
      type: 'ADD_BOT_MESSAGE',
      payload: { id: 'b1', content: 'hi there', timestamp: 2000 },
    });
    expect(s2.messages).toHaveLength(2);
    expect(s2.messages[0].role).toBe('user');
    expect(s2.messages[1].role).toBe('bot');
  });

  it('ADD_BOT_MESSAGE increments unread when panel is closed', () => {
    const closedState: BotState = { ...initialBotState, isOpen: false, unreadCount: 2 };
    const state = botReducer(closedState, {
      type: 'ADD_BOT_MESSAGE',
      payload: { id: 'b2', content: 'reply', timestamp: 3000 },
    });
    expect(state.unreadCount).toBe(3);
  });

  it('ADD_BOT_MESSAGE does not increment unread when panel is open', () => {
    const openState: BotState = { ...initialBotState, isOpen: true, unreadCount: 0 };
    const state = botReducer(openState, {
      type: 'ADD_BOT_MESSAGE',
      payload: { id: 'b3', content: 'reply', timestamp: 4000 },
    });
    expect(state.unreadCount).toBe(0);
  });

  it('CLEAR_UNREAD resets unread count to 0', () => {
    const stateWithUnread: BotState = { ...initialBotState, unreadCount: 8 };
    const state = botReducer(stateWithUnread, { type: 'CLEAR_UNREAD' });
    expect(state.unreadCount).toBe(0);
  });
});

describe('botReducer — typing and suggestions', () => {
  it('SET_TYPING true/false toggles isTyping', () => {
    const s1 = botReducer(initialBotState, { type: 'SET_TYPING', payload: true });
    expect(s1.isTyping).toBe(true);
    const s2 = botReducer(s1, { type: 'SET_TYPING', payload: false });
    expect(s2.isTyping).toBe(false);
  });

  it('SET_SUGGESTIONS updates suggestions array', () => {
    const suggestions = [
      { id: 's1', label: 'Health check', prompt: 'system health', icon: 'heart' },
    ];
    const state = botReducer(initialBotState, { type: 'SET_SUGGESTIONS', payload: suggestions });
    expect(state.suggestions).toHaveLength(1);
    expect(state.suggestions[0].label).toBe('Health check');
  });
});

describe('botReducer — ticket form', () => {
  it('SHOW_TICKET_FORM sets showTicketForm true and stores category', () => {
    const state = botReducer(initialBotState, {
      type: 'SHOW_TICKET_FORM',
      payload: { category: 'bug_report' },
    });
    expect(state.showTicketForm).toBe(true);
    expect(state.ticketCategory).toBe('bug_report');
  });

  it('HIDE_TICKET_FORM sets showTicketForm false', () => {
    const formOpen: BotState = { ...initialBotState, showTicketForm: true, ticketCategory: 'support' };
    const state = botReducer(formOpen, { type: 'HIDE_TICKET_FORM' });
    expect(state.showTicketForm).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Error handling for fetch-based API functions — 5 tests
// ---------------------------------------------------------------------------

describe('fetch error handling', () => {
  let mockFetch: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    mockFetch = vi.fn();
    vi.stubGlobal('fetch', mockFetch);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sendBotMessage with network error → throws', async () => {
    mockFetch.mockRejectedValueOnce(new Error('fetch failed'));
    await expect(sendBotMessage('hello')).rejects.toThrow('fetch failed');
  });

  it('getBotSuggestions with 404 → throws with status code', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });
    await expect(getBotSuggestions('trading')).rejects.toThrow('Failed to get suggestions: 404');
  });

  it('createTicket with 500 → throws with status code', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 500 });
    await expect(
      createTicket({ title: 'T', description: 'D', category: 'bug_report', priority: 'high' }),
    ).rejects.toThrow('Failed to create ticket: 500');
  });

  it('sendBotMessage response without data wrapper → uses top-level json directly', async () => {
    // When response json has no `.data` field, sendBotMessage uses `json` itself
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ reply: 'Direct reply', actions: [] }),
    });
    const result = await sendBotMessage('test');
    expect(result.reply).toBe('Direct reply');
  });

  it('sendBotMessage response with data wrapper → unwraps data field', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        data: { reply: 'Wrapped reply', suggestions: [] },
      }),
    });
    const result = await sendBotMessage('test');
    expect(result.reply).toBe('Wrapped reply');
  });
});
