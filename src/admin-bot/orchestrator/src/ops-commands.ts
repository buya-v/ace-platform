import type { ChatResponse } from './types.js';

export interface OpsResult {
  success: boolean;
  message: string;
  data?: unknown;
  confirmationRequired?: boolean;
  confirmationToken?: string;
}

export interface OpsCommand {
  name: string;
  aliases: string[];
  description: string;
  params: string[];
  destructive: boolean;
  execute: (args: Record<string, string>, gateway: GatewayFn) => Promise<OpsResult>;
}

export type GatewayFn = (path: string, options?: RequestInit) => Promise<Response>;

export interface ParsedOpsCommand {
  command: OpsCommand;
  args: Record<string, string>;
}

// --- Pending confirmations for destructive ops ---

interface PendingOp {
  command: OpsCommand;
  args: Record<string, string>;
  createdAt: number;
}

const pendingConfirmations = new Map<string, PendingOp>();
const CONFIRMATION_TTL_MS = 5 * 60 * 1000; // 5 minutes

export function generateToken(): string {
  return `confirm-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

export function storePendingOp(token: string, op: PendingOp): void {
  pendingConfirmations.set(token, op);
}

export function getPendingOp(token: string): PendingOp | undefined {
  const op = pendingConfirmations.get(token);
  if (!op) return undefined;
  if (Date.now() - op.createdAt > CONFIRMATION_TTL_MS) {
    pendingConfirmations.delete(token);
    return undefined;
  }
  return op;
}

export function removePendingOp(token: string): boolean {
  return pendingConfirmations.delete(token);
}

export async function confirmOp(token: string, confirmed: boolean, gateway: GatewayFn): Promise<OpsResult> {
  const op = getPendingOp(token);
  if (!op) {
    return { success: false, message: 'Confirmation expired or not found. Please re-issue the command.' };
  }
  removePendingOp(token);
  if (!confirmed) {
    return { success: true, message: 'Operation cancelled.' };
  }
  return op.command.execute(op.args, gateway);
}

// --- Command Registry ---

const commands: OpsCommand[] = [
  {
    name: 'halt_instrument',
    aliases: ['halt', 'stop', 'pause'],
    description: 'Halt trading on a specific instrument',
    params: ['instrument_id'],
    destructive: false,
    execute: async (args, gw) => {
      const res = await gw(`/api/v1/instruments/${args.instrument_id}/halt`, { method: 'POST' });
      if (!res.ok) return { success: false, message: `Failed to halt ${args.instrument_id} (HTTP ${res.status})` };
      return { success: true, message: `Trading halted on ${args.instrument_id}.` };
    },
  },
  {
    name: 'resume_instrument',
    aliases: ['resume', 'unhalt', 'start'],
    description: 'Resume trading on a specific instrument',
    params: ['instrument_id'],
    destructive: false,
    execute: async (args, gw) => {
      const res = await gw(`/api/v1/instruments/${args.instrument_id}/resume`, { method: 'POST' });
      if (!res.ok) return { success: false, message: `Failed to resume ${args.instrument_id} (HTTP ${res.status})` };
      return { success: true, message: `Trading resumed on ${args.instrument_id}.` };
    },
  },
  {
    name: 'mass_cancel',
    aliases: ['mass cancel', 'cancel all', 'cancel orders'],
    description: 'Cancel all open orders (destructive)',
    params: [],
    destructive: true,
    execute: async (_args, gw) => {
      const res = await gw('/api/v1/orders/mass-cancel', { method: 'POST' });
      if (!res.ok) return { success: false, message: `Mass cancel failed (HTTP ${res.status})` };
      const data = await res.json().catch(() => ({}));
      return { success: true, message: `Mass cancel executed.`, data };
    },
  },
  {
    name: 'trigger_margin_calc',
    aliases: ['margin calc', 'calculate margin', 'run margin'],
    description: 'Trigger margin calculation',
    params: [],
    destructive: false,
    execute: async (_args, gw) => {
      const res = await gw('/api/v1/margin/calculate', { method: 'POST' });
      if (!res.ok) return { success: false, message: `Margin calculation failed (HTTP ${res.status})` };
      return { success: true, message: 'Margin calculation triggered.' };
    },
  },
  {
    name: 'trigger_settlement',
    aliases: ['settle', 'run settlement', 'trigger settlement'],
    description: 'Trigger settlement cycle',
    params: [],
    destructive: false,
    execute: async (_args, gw) => {
      const res = await gw('/api/v1/settlement/cycle', { method: 'POST' });
      if (!res.ok) return { success: false, message: `Settlement trigger failed (HTTP ${res.status})` };
      return { success: true, message: 'Settlement cycle triggered.' };
    },
  },
  {
    name: 'approve_kyc',
    aliases: ['approve', 'approve kyc'],
    description: 'Approve KYC for a participant',
    params: ['participant_id'],
    destructive: false,
    execute: async (args, gw) => {
      const res = await gw(`/api/v1/participants/${args.participant_id}/kyc/approve`, { method: 'POST' });
      if (!res.ok) return { success: false, message: `KYC approval failed for ${args.participant_id} (HTTP ${res.status})` };
      return { success: true, message: `KYC approved for participant ${args.participant_id}.` };
    },
  },
  {
    name: 'reject_kyc',
    aliases: ['reject', 'reject kyc'],
    description: 'Reject KYC for a participant',
    params: ['participant_id'],
    destructive: false,
    execute: async (args, gw) => {
      const res = await gw(`/api/v1/participants/${args.participant_id}/kyc/reject`, { method: 'POST' });
      if (!res.ok) return { success: false, message: `KYC rejection failed for ${args.participant_id} (HTTP ${res.status})` };
      return { success: true, message: `KYC rejected for participant ${args.participant_id}.` };
    },
  },
  {
    name: 'set_participant_tier',
    aliases: ['set tier', 'tier'],
    description: 'Set participant trading tier',
    params: ['participant_id', 'tier'],
    destructive: false,
    execute: async (args, gw) => {
      const res = await gw(`/api/v1/participants/${args.participant_id}/tier`, {
        method: 'PUT',
        body: JSON.stringify({ tier: args.tier }),
      });
      if (!res.ok) return { success: false, message: `Set tier failed for ${args.participant_id} (HTTP ${res.status})` };
      return { success: true, message: `Participant ${args.participant_id} set to tier ${args.tier}.` };
    },
  },
  {
    name: 'create_instrument',
    aliases: ['create instrument', 'add instrument', 'new instrument'],
    description: 'Create a new trading instrument',
    params: ['symbol'],
    destructive: false,
    execute: async (args, gw) => {
      const res = await gw('/api/v1/instruments', {
        method: 'POST',
        body: JSON.stringify({ symbol: args.symbol }),
      });
      if (!res.ok) return { success: false, message: `Create instrument failed (HTTP ${res.status})` };
      return { success: true, message: `Instrument ${args.symbol} created.` };
    },
  },
  {
    name: 'update_instrument',
    aliases: ['update instrument', 'modify instrument'],
    description: 'Update an existing instrument',
    params: ['instrument_id'],
    destructive: false,
    execute: async (args, gw) => {
      const res = await gw(`/api/v1/instruments/${args.instrument_id}`, {
        method: 'PATCH',
        body: JSON.stringify({ id: args.instrument_id }),
      });
      if (!res.ok) return { success: false, message: `Update instrument failed (HTTP ${res.status})` };
      return { success: true, message: `Instrument ${args.instrument_id} updated.` };
    },
  },
  {
    name: 'halt_all',
    aliases: ['halt all', 'stop all', 'halt everything'],
    description: 'Halt trading on all instruments (destructive)',
    params: [],
    destructive: true,
    execute: async (_args, gw) => {
      const res = await gw('/api/v1/instruments/halt-all', { method: 'POST' });
      if (!res.ok) return { success: false, message: `Halt all failed (HTTP ${res.status})` };
      return { success: true, message: 'All instruments halted.' };
    },
  },
  {
    name: 'resume_all',
    aliases: ['resume all', 'start all', 'unhalt all'],
    description: 'Resume trading on all instruments',
    params: [],
    destructive: false,
    execute: async (_args, gw) => {
      const res = await gw('/api/v1/instruments/resume-all', { method: 'POST' });
      if (!res.ok) return { success: false, message: `Resume all failed (HTTP ${res.status})` };
      return { success: true, message: 'All instruments resumed.' };
    },
  },
];

/**
 * Build a flattened list of (alias, command) pairs sorted by alias length descending
 * so longer aliases match before shorter ones (e.g., "halt all" before "halt").
 */
const aliasIndex: Array<{ alias: string; command: OpsCommand }> = commands
  .flatMap((cmd) => cmd.aliases.map((alias) => ({ alias, command: cmd })))
  .sort((a, b) => b.alias.length - a.alias.length);

/**
 * Parse a natural language message into a command + args.
 * Returns null if no command matches.
 */
export function parseOpsCommand(message: string): ParsedOpsCommand | null {
  const lower = message.toLowerCase().trim();

  for (const { alias, command } of aliasIndex) {
    if (lower.startsWith(alias)) {
      const rest = message.slice(alias.length).trim();
      const args = extractArgs(command.params, rest);
      return { command, args };
    }
  }
  return null;
}

function extractArgs(params: string[], rest: string): Record<string, string> {
  const args: Record<string, string> = {};
  const tokens = rest.split(/\s+/).filter(Boolean);
  for (let i = 0; i < params.length && i < tokens.length; i++) {
    args[params[i]] = tokens[i];
  }
  return args;
}

/**
 * Execute a parsed ops command. For destructive commands, returns a confirmation
 * prompt instead of executing directly.
 */
export async function executeOpsCommand(
  parsed: ParsedOpsCommand,
  gateway: GatewayFn,
): Promise<OpsResult> {
  if (parsed.command.destructive) {
    const token = generateToken();
    storePendingOp(token, {
      command: parsed.command,
      args: parsed.args,
      createdAt: Date.now(),
    });
    return {
      success: true,
      message: `This is a destructive operation: ${parsed.command.description}. Please confirm to proceed.`,
      confirmationRequired: true,
      confirmationToken: token,
    };
  }
  return parsed.command.execute(parsed.args, gateway);
}

/**
 * Convert an OpsResult to a ChatResponse for the bot.
 */
export function opsResultToChatResponse(result: OpsResult): ChatResponse {
  const prefix = result.success ? '\u2705' : '\u274c';
  const response: ChatResponse = {
    reply: `${prefix} ${result.message}`,
    category: 'admin_action',
  };
  if (result.confirmationRequired && result.confirmationToken) {
    response.actions = [
      { label: 'Confirm', type: 'api_call', target: `/bot/confirm?token=${result.confirmationToken}&confirmed=true` },
      { label: 'Cancel', type: 'api_call', target: `/bot/confirm?token=${result.confirmationToken}&confirmed=false` },
    ];
  }
  return response;
}

export { commands as registeredCommands };
