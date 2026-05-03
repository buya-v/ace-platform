import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  parseOpsCommand,
  executeOpsCommand,
  opsResultToChatResponse,
  confirmOp,
  storePendingOp,
  getPendingOp,
  removePendingOp,
  generateToken,
  registeredCommands,
  type GatewayFn,
} from '../ops-commands.js';

describe('parseOpsCommand', () => {
  it('parses halt with instrument ID', () => {
    const result = parseOpsCommand('halt WHT-HRW-2026M07-UB');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('halt_instrument');
    expect(result!.args.instrument_id).toBe('WHT-HRW-2026M07-UB');
  });

  it('parses resume with instrument ID', () => {
    const result = parseOpsCommand('resume WHT-HRW-2026M07-UB');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('resume_instrument');
    expect(result!.args.instrument_id).toBe('WHT-HRW-2026M07-UB');
  });

  it('parses mass cancel', () => {
    const result = parseOpsCommand('mass cancel');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('mass_cancel');
  });

  it('parses cancel all', () => {
    const result = parseOpsCommand('cancel all orders');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('mass_cancel');
  });

  it('parses margin calc', () => {
    const result = parseOpsCommand('calculate margin');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('trigger_margin_calc');
  });

  it('parses trigger settlement', () => {
    const result = parseOpsCommand('trigger settlement');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('trigger_settlement');
  });

  it('parses approve kyc with participant ID', () => {
    const result = parseOpsCommand('approve P-001');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('approve_kyc');
    expect(result!.args.participant_id).toBe('P-001');
  });

  it('parses reject kyc', () => {
    const result = parseOpsCommand('reject P-002');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('reject_kyc');
    expect(result!.args.participant_id).toBe('P-002');
  });

  it('parses set tier with participant and tier', () => {
    const result = parseOpsCommand('set tier P-001 gold');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('set_participant_tier');
    expect(result!.args.participant_id).toBe('P-001');
    expect(result!.args.tier).toBe('gold');
  });

  it('parses halt all', () => {
    const result = parseOpsCommand('halt all');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('halt_all');
  });

  it('parses resume all', () => {
    const result = parseOpsCommand('resume all');
    expect(result).not.toBeNull();
    expect(result!.command.name).toBe('resume_all');
  });

  it('returns null for unknown commands', () => {
    expect(parseOpsCommand('what is the weather?')).toBeNull();
    expect(parseOpsCommand('show me dashboard')).toBeNull();
  });
});

describe('executeOpsCommand', () => {
  let mockGateway: GatewayFn;

  beforeEach(() => {
    mockGateway = vi.fn();
  });

  it('executes non-destructive command directly', async () => {
    (mockGateway as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    const parsed = parseOpsCommand('halt WHT-001')!;
    const result = await executeOpsCommand(parsed, mockGateway);
    expect(result.success).toBe(true);
    expect(result.message).toContain('halted');
    expect(result.confirmationRequired).toBeUndefined();
    expect(mockGateway).toHaveBeenCalledWith('/api/v1/instruments/WHT-001/halt', { method: 'POST' });
  });

  it('returns confirmation for destructive command (mass_cancel)', async () => {
    const parsed = parseOpsCommand('mass cancel')!;
    const result = await executeOpsCommand(parsed, mockGateway);
    expect(result.confirmationRequired).toBe(true);
    expect(result.confirmationToken).toBeTruthy();
    expect(mockGateway).not.toHaveBeenCalled(); // Should NOT execute yet
  });

  it('returns confirmation for halt_all', async () => {
    const parsed = parseOpsCommand('halt all')!;
    const result = await executeOpsCommand(parsed, mockGateway);
    expect(result.confirmationRequired).toBe(true);
    expect(result.confirmationToken).toBeTruthy();
  });

  it('handles gateway failure', async () => {
    (mockGateway as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: false, status: 500 });
    const parsed = parseOpsCommand('resume INST-1')!;
    const result = await executeOpsCommand(parsed, mockGateway);
    expect(result.success).toBe(false);
    expect(result.message).toContain('500');
  });
});

describe('opsResultToChatResponse', () => {
  it('prefixes success with checkmark', () => {
    const response = opsResultToChatResponse({ success: true, message: 'Done' });
    expect(response.reply).toContain('\u2705');
    expect(response.reply).toContain('Done');
  });

  it('prefixes failure with X', () => {
    const response = opsResultToChatResponse({ success: false, message: 'Failed' });
    expect(response.reply).toContain('\u274c');
  });

  it('adds confirm/cancel actions for confirmation', () => {
    const response = opsResultToChatResponse({
      success: true,
      message: 'Confirm?',
      confirmationRequired: true,
      confirmationToken: 'tok-123',
    });
    expect(response.actions).toHaveLength(2);
    expect(response.actions![0].label).toBe('Confirm');
    expect(response.actions![1].label).toBe('Cancel');
  });
});

describe('confirmation flow', () => {
  let mockGateway: GatewayFn;

  beforeEach(() => {
    mockGateway = vi.fn();
  });

  it('confirms and executes pending op', async () => {
    (mockGateway as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: async () => ({}),
    });
    const token = generateToken();
    const cmd = registeredCommands.find((c) => c.name === 'mass_cancel')!;
    storePendingOp(token, { command: cmd, args: {}, createdAt: Date.now() });

    const result = await confirmOp(token, true, mockGateway);
    expect(result.success).toBe(true);
    expect(mockGateway).toHaveBeenCalled();
  });

  it('denies pending op without executing', async () => {
    const token = generateToken();
    const cmd = registeredCommands.find((c) => c.name === 'halt_all')!;
    storePendingOp(token, { command: cmd, args: {}, createdAt: Date.now() });

    const result = await confirmOp(token, false, mockGateway);
    expect(result.success).toBe(true);
    expect(result.message).toBe('Operation cancelled.');
    expect(mockGateway).not.toHaveBeenCalled();
  });

  it('returns error for expired/missing token', async () => {
    const result = await confirmOp('nonexistent-token', true, mockGateway);
    expect(result.success).toBe(false);
    expect(result.message).toContain('expired');
  });

  it('expires tokens after TTL', async () => {
    const token = generateToken();
    const cmd = registeredCommands.find((c) => c.name === 'mass_cancel')!;
    storePendingOp(token, { command: cmd, args: {}, createdAt: Date.now() - 6 * 60 * 1000 }); // 6 min ago

    const op = getPendingOp(token);
    expect(op).toBeUndefined();
  });
});

describe('registeredCommands', () => {
  it('has 12 commands', () => {
    expect(registeredCommands).toHaveLength(12);
  });

  it('destructive commands are mass_cancel and halt_all', () => {
    const destructive = registeredCommands.filter((c) => c.destructive);
    expect(destructive.map((c) => c.name).sort()).toEqual(['halt_all', 'mass_cancel']);
  });
});
