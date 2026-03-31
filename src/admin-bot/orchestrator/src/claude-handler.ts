import { spawn } from 'node:child_process';
import { resolve } from 'node:path';
import type { ChatRequest, ChatResponse } from './types.js';

const CLAUDE_TIMEOUT_MS = 60_000;
const MAX_RETRIES = 2;
const RETRY_BACKOFF_MS = 5_000;

/**
 * Check if the `claude` CLI binary is available on PATH.
 */
async function isClaudeAvailable(): Promise<boolean> {
  return new Promise((resolveP) => {
    const proc = spawn('which', ['claude']);
    proc.on('close', (code) => resolveP(code === 0));
    proc.on('error', () => resolveP(false));
  });
}

/**
 * Run a prompt through the Claude CLI.
 */
function runClaude(prompt: string, cwd: string): Promise<string> {
  return new Promise((resolveP, rejectP) => {
    const proc = spawn('claude', ['-p', prompt, '--max-budget-usd', '0.10'], {
      cwd,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env },
    });

    let stdout = '';
    let stderr = '';

    proc.stdout.on('data', (chunk: Buffer) => {
      stdout += chunk.toString();
    });

    proc.stderr.on('data', (chunk: Buffer) => {
      stderr += chunk.toString();
    });

    const timer = setTimeout(() => {
      proc.kill('SIGTERM');
      rejectP(new Error(`Claude CLI timed out after ${CLAUDE_TIMEOUT_MS}ms`));
    }, CLAUDE_TIMEOUT_MS);

    proc.on('close', (code) => {
      clearTimeout(timer);
      if (code === 0) {
        resolveP(stdout.trim());
      } else {
        rejectP(new Error(`Claude CLI exited with code ${code}: ${stderr.trim()}`));
      }
    });

    proc.on('error', (err) => {
      clearTimeout(timer);
      rejectP(err);
    });
  });
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

/**
 * Determine the project root directory (3 levels up from this file's location in dist/).
 */
function getProjectRoot(): string {
  // When running from dist/claude-handler.js, project root is ../../.. from the orchestrator dir
  // Use GARUDAX_PROJECT_ROOT env var if set, otherwise walk up from cwd
  return process.env.GARUDAX_PROJECT_ROOT ?? resolve(process.cwd(), '..', '..', '..');
}

/**
 * Handle complex analysis requests by delegating to the Claude CLI.
 *
 * Spawns `claude -p "<prompt>" --max-budget-usd 0.10` with the project root
 * as CWD so that Claude picks up `.mcp.json` and project context.
 *
 * Retries up to 2 times with 5-second backoff on failure.
 * Returns a fallback message if the Claude CLI is not installed.
 */
export async function handleComplex(request: ChatRequest): Promise<ChatResponse> {
  const available = await isClaudeAvailable();
  if (!available) {
    return {
      reply:
        'Claude CLI is not available in this environment. Please install it or handle this request through the admin dashboard.',
      category: 'complex_analysis',
      actions: [{ label: 'Open Admin Dashboard', type: 'link', target: '/admin' }],
    };
  }

  const cwd = getProjectRoot();

  const contextParts: string[] = [];
  if (request.context?.page) contextParts.push(`Current page: ${request.context.page}`);
  if (request.context?.userId) contextParts.push(`User ID: ${request.context.userId}`);
  if (request.context?.ticketId) contextParts.push(`Ticket ID: ${request.context.ticketId}`);

  const contextBlock = contextParts.length > 0 ? `\nContext: ${contextParts.join(', ')}\n` : '';

  const prompt = `You are an admin assistant for the GarudaX commodity exchange platform. Answer the following admin request concisely and actionably.${contextBlock}\nRequest: ${request.message}`;

  let lastError: Error | undefined;

  for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
    try {
      const reply = await runClaude(prompt, cwd);
      return {
        reply,
        category: 'complex_analysis',
      };
    } catch (err) {
      lastError = err instanceof Error ? err : new Error(String(err));
      if (attempt < MAX_RETRIES - 1) {
        await sleep(RETRY_BACKOFF_MS);
      }
    }
  }

  return {
    reply: `Unable to process complex analysis after ${MAX_RETRIES} attempts. Error: ${lastError?.message ?? 'unknown'}. Please try again or use the admin dashboard.`,
    category: 'complex_analysis',
    actions: [{ label: 'Open Admin Dashboard', type: 'link', target: '/admin' }],
  };
}
