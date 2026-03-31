import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { classifyRequest } from './router.js';
import { handleSimple } from './nano-handler.js';
import { handleComplex } from './claude-handler.js';
import type { ChatRequest, ChatResponse, Suggestion } from './types.js';

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization',
  'Access-Control-Max-Age': '86400',
};

/** Page-aware quick action suggestions. */
const PAGE_SUGGESTIONS: Record<string, Suggestion[]> = {
  dashboard: [
    { label: 'Check platform health', prompt: 'What is the current platform health status?', icon: 'heart' },
    { label: 'View daily report', prompt: 'Generate today\'s platform report', icon: 'chart' },
    { label: 'Active alerts', prompt: 'Are there any active alerts?', icon: 'bell' },
  ],
  tickets: [
    { label: 'Create ticket', prompt: 'Create a new support ticket', icon: 'plus' },
    { label: 'Aging tickets', prompt: 'Show tickets open longer than 24 hours', icon: 'clock' },
    { label: 'My tickets', prompt: 'Show my open tickets', icon: 'user' },
  ],
  trading: [
    { label: 'Market status', prompt: 'What is the current market status?', icon: 'trending' },
    { label: 'Halt trading', prompt: 'Halt trading on all instruments', icon: 'stop' },
    { label: 'Large trader report', prompt: 'Generate large trader position report', icon: 'chart' },
  ],
  settlement: [
    { label: 'Settlement status', prompt: 'What is the current settlement cycle status?', icon: 'check' },
    { label: 'Run settlement', prompt: 'Trigger end-of-day settlement cycle', icon: 'play' },
    { label: 'Failed settlements', prompt: 'Show failed settlement transactions', icon: 'alert' },
  ],
  participants: [
    { label: 'Participant list', prompt: 'Show all registered participants', icon: 'users' },
    { label: 'KYC pending', prompt: 'Show participants with pending KYC', icon: 'clock' },
    { label: 'Compliance check', prompt: 'Run compliance check on all participants', icon: 'shield' },
  ],
  default: [
    { label: 'Platform health', prompt: 'What is the current platform health status?', icon: 'heart' },
    { label: 'Create ticket', prompt: 'Create a new support ticket', icon: 'plus' },
    { label: 'Help', prompt: 'What can you help me with?', icon: 'help' },
  ],
};

function sendJSON(res: ServerResponse, statusCode: number, body: unknown): void {
  const json = JSON.stringify(body);
  res.writeHead(statusCode, {
    ...CORS_HEADERS,
    'Content-Type': 'application/json',
    'Content-Length': Buffer.byteLength(json),
  });
  res.end(json);
}

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    let size = 0;
    const MAX_BODY = 64 * 1024; // 64KB limit

    req.on('data', (chunk: Buffer) => {
      size += chunk.length;
      if (size > MAX_BODY) {
        reject(new Error('Request body too large'));
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf-8')));
    req.on('error', reject);
  });
}

function parseURL(req: IncomingMessage): URL {
  return new URL(req.url ?? '/', `http://${req.headers.host ?? 'localhost'}`);
}

async function handleChat(req: IncomingMessage, res: ServerResponse): Promise<void> {
  if (req.method !== 'POST') {
    sendJSON(res, 405, { error: 'Method not allowed' });
    return;
  }

  let body: ChatRequest;
  try {
    const raw = await readBody(req);
    body = JSON.parse(raw) as ChatRequest;
  } catch {
    sendJSON(res, 400, { error: 'Invalid JSON body' });
    return;
  }

  if (!body.message || typeof body.message !== 'string') {
    sendJSON(res, 400, { error: 'Missing required field: message' });
    return;
  }

  const category = await classifyRequest(body.message);

  let response: ChatResponse;

  if (category === 'complex_analysis') {
    response = await handleComplex(body);
  } else {
    response = await handleSimple(body, category);
  }

  // Ensure category is set on the response
  if (!response.category) {
    response.category = category;
  }

  sendJSON(res, 200, response);
}

function handleSuggestions(req: IncomingMessage, res: ServerResponse): void {
  const url = parseURL(req);
  const page = url.searchParams.get('page')?.toLowerCase() ?? 'default';
  const suggestions = PAGE_SUGGESTIONS[page] ?? PAGE_SUGGESTIONS['default']!;
  sendJSON(res, 200, { suggestions });
}

function handleHealth(_req: IncomingMessage, res: ServerResponse): void {
  sendJSON(res, 200, {
    status: 'ok',
    service: 'bot-orchestrator',
    version: '1.0.0',
    uptime_seconds: Math.floor(process.uptime()),
    timestamp: new Date().toISOString(),
  });
}

export function createAPIServer() {
  const server = createServer(async (req, res) => {
    // Handle CORS preflight
    if (req.method === 'OPTIONS') {
      res.writeHead(204, CORS_HEADERS);
      res.end();
      return;
    }

    const url = parseURL(req);
    const path = url.pathname;

    try {
      switch (path) {
        case '/chat':
          await handleChat(req, res);
          break;
        case '/suggestions':
          handleSuggestions(req, res);
          break;
        case '/health':
          handleHealth(req, res);
          break;
        default:
          sendJSON(res, 404, { error: 'Not found' });
      }
    } catch (err) {
      console.error(`[api] Error handling ${req.method} ${path}:`, err);
      sendJSON(res, 500, { error: 'Internal server error' });
    }
  });

  return server;
}
