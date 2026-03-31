import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { classifyByKeyword, classifyRequest } from '../router.js';

describe('classifyByKeyword', () => {
  // --- status_query ---
  it('classifies "What is the system health?" as status_query', () => {
    expect(classifyByKeyword('What is the system health?')).toBe('status_query');
  });

  it('classifies "Is the gateway running?" as status_query', () => {
    expect(classifyByKeyword('Is the gateway running?')).toBe('status_query');
  });

  it('classifies "check service status" as status_query', () => {
    expect(classifyByKeyword('check service status')).toBe('status_query');
  });

  it('classifies "ping the system" as status_query', () => {
    expect(classifyByKeyword('ping the system')).toBe('status_query');
  });

  // --- admin_action ---
  it('classifies "Halt trading on wheat" as admin_action', () => {
    expect(classifyByKeyword('Halt trading on wheat')).toBe('admin_action');
  });

  it('classifies "disable user account" as admin_action', () => {
    expect(classifyByKeyword('disable user account')).toBe('admin_action');
  });

  it('classifies "suspend participant X" as admin_action', () => {
    expect(classifyByKeyword('suspend participant X')).toBe('admin_action');
  });

  // --- ticket_create ---
  it('classifies "I found a bug" as ticket_create', () => {
    expect(classifyByKeyword('I found a bug')).toBe('ticket_create');
  });

  it('classifies "file a new issue" as ticket_create', () => {
    expect(classifyByKeyword('file a new issue')).toBe('ticket_create');
  });

  it('classifies "submit a request" as ticket_create', () => {
    expect(classifyByKeyword('submit a request')).toBe('ticket_create');
  });

  // --- ticket_update ---
  it('classifies "update ticket #42" as ticket_update', () => {
    expect(classifyByKeyword('update ticket #42')).toBe('ticket_update');
  });

  it('classifies "close ticket 100" as ticket_update', () => {
    expect(classifyByKeyword('close ticket 100')).toBe('ticket_update');
  });

  // --- report_generate ---
  it('classifies "Generate market summary" as report_generate', () => {
    expect(classifyByKeyword('Generate market summary')).toBe('report_generate');
  });

  it('classifies "show me the daily report" as report_generate', () => {
    expect(classifyByKeyword('show me the daily report')).toBe('report_generate');
  });

  it('classifies "get analytics" as report_generate', () => {
    expect(classifyByKeyword('get analytics')).toBe('report_generate');
  });

  // --- complex_analysis (fallback) ---
  it('classifies "Analyze the trading patterns" as complex_analysis', () => {
    expect(classifyByKeyword('Analyze the trading patterns')).toBe('complex_analysis');
  });

  it('classifies "what do you think about recent market trends?" as complex_analysis', () => {
    expect(classifyByKeyword('what do you think about recent market trends?')).toBe('complex_analysis');
  });

  // --- edge cases ---
  it('classifies empty string as complex_analysis', () => {
    expect(classifyByKeyword('')).toBe('complex_analysis');
  });

  it('classifies very long input by scanning keywords', () => {
    const longInput = 'a'.repeat(5000) + ' check the service status ' + 'b'.repeat(5000);
    expect(classifyByKeyword(longInput)).toBe('status_query');
  });

  it('handles special characters without crashing', () => {
    expect(classifyByKeyword('!@#$%^&*()_+')).toBe('complex_analysis');
  });

  it('handles unicode characters without crashing', () => {
    expect(classifyByKeyword('\u{1F600} hello world')).toBe('complex_analysis');
  });

  it('is case-insensitive', () => {
    expect(classifyByKeyword('HALT TRADING NOW')).toBe('admin_action');
    expect(classifyByKeyword('CHECK HEALTH')).toBe('status_query');
  });
});

describe('classifyRequest', () => {
  const originalEnv = process.env;

  beforeEach(() => {
    process.env = { ...originalEnv };
    delete process.env.OPENAI_API_KEY;
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it('falls back to keyword classification when no API key', async () => {
    const result = await classifyRequest('check the system health');
    expect(result).toBe('status_query');
  });

  it('falls back to keyword classification for admin action without API key', async () => {
    const result = await classifyRequest('halt trading');
    expect(result).toBe('admin_action');
  });

  it('returns complex_analysis for unrecognized input without API key', async () => {
    const result = await classifyRequest('tell me something interesting');
    expect(result).toBe('complex_analysis');
  });
});
