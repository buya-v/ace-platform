import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  buildBotUrl,
  buildAuthHeaders,
  buildSendMessageBody,
  buildCreateTicketBody,
  sendBotMessage,
  getBotSuggestions,
  createTicket,
} from '../services/botApi';

// Mock config module
vi.mock('../config', () => ({
  getConfig: () => ({
    API_BASE_URL: 'http://localhost:8080/api/v1',
    WS_BASE_URL: 'ws://localhost:8080/api/v1/ws',
    HEALTH_POLL_INTERVAL: 15000,
    AUTH_TOKEN_REFRESH_BUFFER: 60,
  }),
}));

// Mock api module for getAccessToken
vi.mock('../services/api', () => ({
  getAccessToken: vi.fn(() => 'test-token-abc'),
  setAccessToken: vi.fn(),
}));

describe('botApi URL construction', () => {
  it('builds chat endpoint URL from config base', () => {
    const url = buildBotUrl('/chat');
    expect(url).toBe('http://localhost:8080/api/v1/bot/chat');
  });

  it('builds suggestions endpoint URL with query params', () => {
    const url = buildBotUrl('/suggestions?page=dashboard');
    expect(url).toBe('http://localhost:8080/api/v1/bot/suggestions?page=dashboard');
  });

  it('builds tickets endpoint URL', () => {
    const url = buildBotUrl('/tickets');
    expect(url).toBe('http://localhost:8080/api/v1/bot/tickets');
  });

  it('builds health endpoint URL', () => {
    const url = buildBotUrl('/health');
    expect(url).toBe('http://localhost:8080/api/v1/bot/health');
  });

  it('handles paths without leading slash', () => {
    // Should still concatenate, even if missing slash
    const url = buildBotUrl('chat');
    expect(url).toContain('botchat');
  });
});

describe('botApi auth headers', () => {
  it('includes Content-Type application/json', () => {
    const headers = buildAuthHeaders();
    expect(headers['Content-Type']).toBe('application/json');
  });

  it('includes Authorization header with bearer token', () => {
    const headers = buildAuthHeaders();
    expect(headers['Authorization']).toBe('Bearer test-token-abc');
  });
});

describe('botApi request body builders', () => {
  it('serializes message with empty context when none given', () => {
    const body = buildSendMessageBody('hello bot');
    const parsed = JSON.parse(body);
    expect(parsed.message).toBe('hello bot');
    expect(parsed.context).toEqual({});
  });

  it('serializes message with page context', () => {
    const body = buildSendMessageBody('help me', { page: 'trading' });
    const parsed = JSON.parse(body);
    expect(parsed.message).toBe('help me');
    expect(parsed.context.page).toBe('trading');
  });

  it('serializes ticket creation body', () => {
    const body = buildCreateTicketBody({
      title: 'Bug in matching',
      description: 'Order not matched correctly',
      category: 'bug_report',
      priority: 'high',
    });
    const parsed = JSON.parse(body);
    expect(parsed.title).toBe('Bug in matching');
    expect(parsed.description).toBe('Order not matched correctly');
    expect(parsed.category).toBe('bug_report');
    expect(parsed.priority).toBe('high');
  });
});

describe('sendBotMessage', () => {
  let mockFetch: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    mockFetch = vi.fn();
    vi.stubGlobal('fetch', mockFetch);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends POST to /bot/chat with correct body', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ reply: 'Hello!', actions: [] }),
    });

    const result = await sendBotMessage('hi there');

    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/bot/chat'),
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Content-Type': 'application/json',
        }),
      }),
    );
    expect(result.reply).toBe('Hello!');
  });

  it('passes page context when provided', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ reply: 'Dashboard help...' }),
    });

    await sendBotMessage('help', { page: 'dashboard' });

    const callBody = mockFetch.mock.calls[0][1].body;
    const parsed = JSON.parse(callBody);
    expect(parsed.context.page).toBe('dashboard');
  });

  it('throws on non-ok response', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
    });

    await expect(sendBotMessage('test')).rejects.toThrow('Bot request failed: 500');
  });

  it('propagates network errors', async () => {
    mockFetch.mockRejectedValueOnce(new Error('Network failure'));

    await expect(sendBotMessage('test')).rejects.toThrow('Network failure');
  });
});

describe('getBotSuggestions', () => {
  let mockFetch: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    mockFetch = vi.fn();
    vi.stubGlobal('fetch', mockFetch);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends GET to /bot/suggestions with page param', async () => {
    const suggestions = [
      { id: '1', label: 'Check health', prompt: 'system health', icon: 'heart' },
    ];
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => suggestions,
    });

    const result = await getBotSuggestions('dashboard');

    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/bot/suggestions?page=dashboard'),
      expect.objectContaining({ method: 'GET' }),
    );
    expect(result).toHaveLength(1);
    expect(result[0].label).toBe('Check health');
  });

  it('encodes page parameter', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => [],
    });

    await getBotSuggestions('page with spaces');

    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('page%20with%20spaces'),
      expect.any(Object),
    );
  });

  it('throws on non-ok response', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
    });

    await expect(getBotSuggestions('unknown')).rejects.toThrow('Failed to get suggestions: 404');
  });
});

describe('createTicket', () => {
  let mockFetch: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    mockFetch = vi.fn();
    vi.stubGlobal('fetch', mockFetch);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends POST to /bot/tickets with ticket data', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 'TKT-100' }),
    });

    const result = await createTicket({
      title: 'Login fails',
      description: 'Cannot login with valid credentials',
      category: 'bug_report',
      priority: 'high',
    });

    expect(result.id).toBe('TKT-100');
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/bot/tickets'),
      expect.objectContaining({ method: 'POST' }),
    );

    const callBody = mockFetch.mock.calls[0][1].body;
    const parsed = JSON.parse(callBody);
    expect(parsed.title).toBe('Login fails');
    expect(parsed.priority).toBe('high');
  });

  it('throws on non-ok response', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 422,
    });

    await expect(
      createTicket({
        title: 'Test',
        description: 'Desc',
        category: 'support',
        priority: 'low',
      }),
    ).rejects.toThrow('Failed to create ticket: 422');
  });

  it('includes auth headers in ticket creation request', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 'TKT-101' }),
    });

    await createTicket({
      title: 'Feature',
      description: 'Need X',
      category: 'feature_request',
      priority: 'medium',
    });

    const callHeaders = mockFetch.mock.calls[0][1].headers;
    expect(callHeaders['Authorization']).toBe('Bearer test-token-abc');
  });
});
