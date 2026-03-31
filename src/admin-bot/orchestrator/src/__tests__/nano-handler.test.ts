import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { handleSimple } from '../nano-handler.js';
import type { ChatRequest } from '../types.js';

// Mock fetch globally
const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

describe('handleSimple', () => {
  const originalEnv = process.env;

  beforeEach(() => {
    process.env = { ...originalEnv };
    delete process.env.OPENAI_API_KEY;
    delete process.env.GATEWAY_URL;
    delete process.env.GATEWAY_TOKEN;
    mockFetch.mockReset();
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  // --- status_query ---
  describe('status_query', () => {
    it('formats health response when gateway returns services', async () => {
      const services = [
        { service: 'matching-engine', status: 'healthy', latency_ms: 12 },
        { service: 'clearing-engine', status: 'unhealthy', details: 'timeout' },
      ];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ services }),
      });

      const result = await handleSimple({ message: 'health' }, 'status_query');

      expect(result.reply).toContain('Platform Health: 1/2');
      expect(result.reply).toContain('[OK] matching-engine');
      expect(result.reply).toContain('[DOWN] clearing-engine');
      expect(result.reply).toContain('12ms');
      expect(result.reply).toContain('timeout');
      expect(result.category).toBe('status_query');
    });

    it('handles health response as an array', async () => {
      const services = [
        { service: 'gateway', status: 'healthy' },
      ];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => services,
      });

      const result = await handleSimple({ message: 'status' }, 'status_query');

      expect(result.reply).toContain('1/1 services healthy');
      expect(result.reply).toContain('[OK] gateway');
    });

    it('returns error message when gateway returns non-ok', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 503,
      });

      const result = await handleSimple({ message: 'health' }, 'status_query');

      expect(result.reply).toContain('503');
      expect(result.reply).toContain('experiencing issues');
      expect(result.actions).toBeDefined();
    });

    it('returns error when gateway is unreachable', async () => {
      mockFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'));

      const result = await handleSimple({ message: 'health' }, 'status_query');

      expect(result.reply).toContain('Unable to reach the gateway');
      expect(result.reply).toContain('ECONNREFUSED');
    });

    it('handles empty services array', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ services: [] }),
      });

      const result = await handleSimple({ message: 'health' }, 'status_query');

      expect(result.reply).toContain('No service health data available');
    });
  });

  // --- ticket_create ---
  describe('ticket_create', () => {
    it('creates ticket via POST /api/v1/tickets', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ id: 'TKT-001' }),
      });

      const result = await handleSimple(
        { message: 'Found a critical bug in the matching engine' },
        'ticket_create',
      );

      expect(result.reply).toContain('Ticket created successfully');
      expect(result.reply).toContain('TKT-001');
      expect(result.ticket_id).toBe('TKT-001');
      expect(result.category).toBe('ticket_create');

      // Verify fetch was called with POST
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/v1/tickets'),
        expect.objectContaining({ method: 'POST' }),
      );
    });

    it('extracts priority from message keywords', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ id: 'TKT-002' }),
      });

      const result = await handleSimple(
        { message: 'Critical: system crash on order submission' },
        'ticket_create',
      );

      expect(result.reply).toContain('critical');
    });

    it('handles ticket creation failure', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      });

      const result = await handleSimple(
        { message: 'bug report' },
        'ticket_create',
      );

      expect(result.reply).toContain('Failed to create ticket');
      expect(result.reply).toContain('500');
    });

    it('handles network error during ticket creation', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'));

      const result = await handleSimple(
        { message: 'bug report' },
        'ticket_create',
      );

      expect(result.reply).toContain('Unable to create ticket');
      expect(result.reply).toContain('Network error');
    });
  });

  // --- admin_action ---
  describe('admin_action', () => {
    it('returns confirmation message with action links', async () => {
      const result = await handleSimple(
        { message: 'halt trading on wheat' },
        'admin_action',
      );

      expect(result.reply).toContain('Admin action requested');
      expect(result.reply).toContain('halt trading on wheat');
      expect(result.reply).toContain('confirmation');
      expect(result.category).toBe('admin_action');
      expect(result.actions).toHaveLength(2);
      expect(result.actions![0].label).toBe('Open Admin Panel');
    });
  });

  // --- report_generate ---
  describe('report_generate', () => {
    it('generates platform report when gateway is available', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          services: [
            { service: 'matching-engine', status: 'healthy' },
            { service: 'gateway', status: 'healthy' },
          ],
        }),
      });

      const result = await handleSimple(
        { message: 'generate report' },
        'report_generate',
      );

      expect(result.reply).toContain('Platform Report');
      expect(result.reply).toContain('2/2 healthy');
      expect(result.category).toBe('report_generate');
      expect(result.actions).toBeDefined();
    });

    it('handles gateway error during report generation', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Connection refused'));

      const result = await handleSimple(
        { message: 'generate report' },
        'report_generate',
      );

      expect(result.reply).toContain('Unable to generate report');
    });
  });

  // --- ticket_update ---
  describe('ticket_update', () => {
    it('updates ticket when ticket ID is in context', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({}),
      });

      const request: ChatRequest = {
        message: 'mark this as resolved',
        context: { ticketId: '42' },
      };

      const result = await handleSimple(request, 'ticket_update');

      expect(result.reply).toContain('42');
      expect(result.reply).toContain('updated successfully');
    });

    it('prompts for ticket ID when none provided', async () => {
      const request: ChatRequest = {
        message: 'update it please',
      };

      const result = await handleSimple(request, 'ticket_update');

      expect(result.reply).toContain('specify a ticket ID');
    });

    it('extracts ticket ID from message text', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({}),
      });

      const request: ChatRequest = {
        message: 'update ticket #99 to resolved',
      };

      const result = await handleSimple(request, 'ticket_update');

      expect(result.reply).toContain('99');
      expect(result.reply).toContain('updated successfully');
    });
  });

  // --- gateway URL config ---
  describe('gateway configuration', () => {
    it('uses GATEWAY_URL env var when set', async () => {
      process.env.GATEWAY_URL = 'http://custom-gateway:9090';
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ services: [] }),
      });

      await handleSimple({ message: 'health' }, 'status_query');

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('http://custom-gateway:9090'),
        expect.any(Object),
      );
    });

    it('includes authorization header when GATEWAY_TOKEN is set', async () => {
      process.env.GATEWAY_TOKEN = 'test-token-123';
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ services: [] }),
      });

      await handleSimple({ message: 'health' }, 'status_query');

      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token-123',
          }),
        }),
      );
    });
  });
});
