import OpenAI from 'openai';
import type {
  ChatRequest,
  ChatResponse,
  GatewayConfig,
  HealthStatus,
  RequestCategory,
  TicketPayload,
} from './types.js';
import { parseOpsCommand, executeOpsCommand, opsResultToChatResponse } from './ops-commands.js';

let cachedToken: string | undefined;

function getGatewayBaseUrl(): string {
  return process.env.GARUDAX_GATEWAY_URL ?? process.env.GATEWAY_URL ?? 'http://localhost:8080';
}

async function ensureToken(): Promise<string | undefined> {
  if (cachedToken) return cachedToken;
  if (process.env.GATEWAY_TOKEN) { cachedToken = process.env.GATEWAY_TOKEN; return cachedToken; }
  const email = process.env.GARUDAX_ADMIN_EMAIL;
  const password = process.env.GARUDAX_ADMIN_PASSWORD;
  if (!email || !password) return undefined;
  try {
    const res = await fetch(`${getGatewayBaseUrl()}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });
    if (!res.ok) return undefined;
    const data = await res.json() as Record<string, string>;
    cachedToken = data.AccessToken ?? data.access_token;
    return cachedToken;
  } catch { return undefined; }
}

export async function fetchGateway(path: string, options?: RequestInit): Promise<Response> {
  const baseUrl = getGatewayBaseUrl();
  const token = await ensureToken();
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;
  const url = `${baseUrl}${path}`;
  return fetch(url, {
    ...options,
    headers: { ...headers, ...(options?.headers as Record<string, string> | undefined) },
  });
}

/**
 * Extract a ticket payload from a natural language message.
 * Uses GPT-nano with function calling when available, otherwise simple parsing.
 */
async function extractTicketPayload(message: string): Promise<TicketPayload> {
  const apiKey = process.env.OPENAI_API_KEY;
  if (!apiKey) {
    return extractTicketByParsing(message);
  }

  const client = new OpenAI({ apiKey });

  const tools: OpenAI.Chat.Completions.ChatCompletionTool[] = [
    {
      type: 'function',
      function: {
        name: 'create_ticket',
        description: 'Create a support ticket from the user message',
        parameters: {
          type: 'object',
          properties: {
            title: { type: 'string', description: 'Short ticket title' },
            description: { type: 'string', description: 'Full description of the issue' },
            priority: {
              type: 'string',
              enum: ['low', 'medium', 'high', 'critical'],
              description: 'Ticket priority',
            },
          },
          required: ['title', 'description'],
        },
      },
    },
  ];

  try {
    const models = ['gpt-5.4-nano', 'gpt-4o-mini'];
    for (const model of models) {
      try {
        const response = await client.chat.completions.create({
          model,
          temperature: 0,
          messages: [
            {
              role: 'system',
              content: 'Extract a support ticket from the user message. Call create_ticket with the extracted fields.',
            },
            { role: 'user', content: message },
          ],
          tools,
          tool_choice: { type: 'function', function: { name: 'create_ticket' } },
        });

        const toolCall = response.choices[0]?.message?.tool_calls?.[0];
        if (toolCall?.function?.arguments) {
          const parsed = JSON.parse(toolCall.function.arguments) as TicketPayload;
          return {
            title: parsed.title || message.slice(0, 80),
            description: parsed.description || message,
            priority: parsed.priority ?? 'medium',
          };
        }
        break;
      } catch {
        continue;
      }
    }
  } catch {
    // Fall through to parsing
  }

  return extractTicketByParsing(message);
}

function extractTicketByParsing(message: string): TicketPayload {
  const firstSentence = message.split(/[.!?\n]/)[0]?.trim() ?? message.slice(0, 80);
  let priority: TicketPayload['priority'] = 'medium';
  const lower = message.toLowerCase();
  if (/\b(critical|urgent|emergency|crash)\b/.test(lower)) priority = 'critical';
  else if (/\b(high|important|severe)\b/.test(lower)) priority = 'high';
  else if (/\b(low|minor|cosmetic)\b/.test(lower)) priority = 'low';

  return {
    title: firstSentence.slice(0, 120),
    description: message,
    priority,
  };
}

function formatHealthStatus(services: HealthStatus[]): string {
  if (services.length === 0) return 'No service health data available.';

  const lines = services.map((s) => {
    const icon = s.status === 'healthy' ? '[OK]' : s.status === 'unhealthy' ? '[DOWN]' : '[??]';
    const name = s.service || (s as unknown as { name: string }).name || 'unknown';
    const latency = s.latency_ms !== undefined ? ` (${s.latency_ms}ms)` : '';
    return `  ${icon} ${name}${latency}${s.details ? ` - ${s.details}` : ''}`;
  });

  const healthy = services.filter((s) => s.status === 'healthy').length;
  const total = services.length;

  return `Platform Health: ${healthy}/${total} services healthy\n${lines.join('\n')}`;
}

/**
 * Handle simple requests using the gateway API (and optionally GPT-nano for extraction).
 */
export async function handleSimple(
  request: ChatRequest,
  category: RequestCategory,
): Promise<ChatResponse> {
  // Override category based on message content for better routing
  const lower = request.message.toLowerCase();
  if (/margin/.test(lower) && (category === 'status_query' || category === 'admin_action')) {
    return handleAdminAction(request.message); // margin handler is in admin_action
  }
  if (/instrument|commodity|contract/.test(lower) && category !== 'ticket_create') {
    return handleAdminAction(request.message);
  }

  switch (category) {
    case 'status_query':
      return handleStatusQuery();

    case 'ticket_create':
      return handleTicketCreate(request.message);

    case 'ticket_update':
      return handleTicketUpdate(request);

    case 'admin_action':
      return handleAdminAction(request.message);

    case 'report_generate':
      return handleReportGenerate();

    default:
      return handleAdminAction(request.message);
  }
}

async function handleStatusQuery(): Promise<ChatResponse> {
  try {
    const res = await fetchGateway('/api/v1/admin/health');
    if (!res.ok) {
      return {
        reply: `Gateway returned ${res.status}. The platform may be experiencing issues.`,
        category: 'status_query',
        actions: [{ label: 'View Dashboard', type: 'link', target: '/dashboard' }],
      };
    }

    const data = await res.json() as { services?: HealthStatus[] };
    const services: HealthStatus[] = Array.isArray(data)
      ? (data as HealthStatus[])
      : (data.services ?? []);

    return {
      reply: formatHealthStatus(services),
      category: 'status_query',
      actions: [
        { label: 'View Dashboard', type: 'link', target: '/dashboard' },
        { label: 'Refresh Health', type: 'api_call', target: '/api/v1/admin/health' },
      ],
    };
  } catch (err) {
    return {
      reply: `Unable to reach the gateway. Error: ${err instanceof Error ? err.message : String(err)}`,
      category: 'status_query',
    };
  }
}

async function handleTicketCreate(message: string): Promise<ChatResponse> {
  const payload = await extractTicketPayload(message);

  try {
    const res = await fetchGateway('/api/v1/tickets', {
      method: 'POST',
      body: JSON.stringify(payload),
    });

    if (!res.ok) {
      return {
        reply: `Failed to create ticket (HTTP ${res.status}). Please try again or create it manually.`,
        category: 'ticket_create',
        actions: [{ label: 'Create Manually', type: 'link', target: '/tickets/new' }],
      };
    }

    const result = await res.json() as { id?: string; ticket_id?: string };
    const ticketId = result.id ?? result.ticket_id ?? 'unknown';

    return {
      reply: `Ticket created successfully.\n  ID: ${ticketId}\n  Title: ${payload.title}\n  Priority: ${payload.priority ?? 'medium'}`,
      category: 'ticket_create',
      ticket_id: String(ticketId),
      actions: [{ label: 'View Ticket', type: 'link', target: `/tickets/${ticketId}` }],
    };
  } catch (err) {
    return {
      reply: `Unable to create ticket. Error: ${err instanceof Error ? err.message : String(err)}`,
      category: 'ticket_create',
    };
  }
}

async function handleTicketUpdate(request: ChatRequest): Promise<ChatResponse> {
  const ticketId = request.context?.ticketId ?? extractTicketId(request.message);

  if (!ticketId) {
    return {
      reply: 'Please specify a ticket ID to update (e.g., "update ticket #123 status to resolved").',
      category: 'ticket_update',
    };
  }

  try {
    const res = await fetchGateway(`/api/v1/tickets/${ticketId}`, {
      method: 'PATCH',
      body: JSON.stringify({ note: request.message }),
    });

    if (!res.ok) {
      return {
        reply: `Failed to update ticket ${ticketId} (HTTP ${res.status}).`,
        category: 'ticket_update',
      };
    }

    return {
      reply: `Ticket ${ticketId} updated successfully.`,
      category: 'ticket_update',
      ticket_id: ticketId,
      actions: [{ label: 'View Ticket', type: 'link', target: `/tickets/${ticketId}` }],
    };
  } catch (err) {
    return {
      reply: `Unable to update ticket. Error: ${err instanceof Error ? err.message : String(err)}`,
      category: 'ticket_update',
    };
  }
}

function extractTicketId(message: string): string | undefined {
  const match = message.match(/(?:ticket\s*)?#?(\d+)/i);
  return match?.[1];
}

async function handleAdminAction(message: string): Promise<ChatResponse> {
  // Try ops command execution first
  const parsed = parseOpsCommand(message);
  if (parsed) {
    const result = await executeOpsCommand(parsed, fetchGateway);
    return opsResultToChatResponse(result);
  }

  const lower = message.toLowerCase();

  // Instrument queries
  if (/instrument|commodity|contract/.test(lower)) {
    try {
      const res = await fetchGateway('/api/v1/instruments');
      if (res.ok) {
        const data = await res.json() as Record<string, unknown>;
        const instruments = (data as { data?: unknown[] }).data ?? (Array.isArray(data) ? data : []);
        const list = (instruments as Array<{ id?: string; name?: string }>)
          .map((i) => `  • ${i.id ?? i.name ?? 'unknown'}`)
          .join('\n');
        return {
          reply: `Currently active instruments:\n${list || '  (none found)'}\n\nTo add a new instrument, insert a row into the reference.instruments table via database migration or admin SQL.`,
          category: 'admin_action',
          actions: [
            { label: 'View Instruments', type: 'link', target: '/dashboard/orderbook' },
          ],
        };
      }
    } catch { /* fall through */ }
  }

  // Halt/resume
  if (/halt|stop|pause/.test(lower)) {
    return {
      reply: 'To halt trading on an instrument, go to Circuit Breakers and click "Halt" on the instrument row. Or tell me which instrument to halt (e.g., "halt WHT-HRW-2026M07-UB").',
      category: 'admin_action',
      actions: [{ label: 'Circuit Breakers', type: 'link', target: '/dashboard/circuit-breakers' }],
    };
  }
  if (/resume|start|open/.test(lower)) {
    return {
      reply: 'To resume trading, go to Circuit Breakers and click "Resume" on the halted instrument.',
      category: 'admin_action',
      actions: [{ label: 'Circuit Breakers', type: 'link', target: '/dashboard/circuit-breakers' }],
    };
  }

  // Cancel orders
  if (/cancel|mass.?cancel/.test(lower)) {
    return {
      reply: 'Mass cancel is available in the admin panel. Use with caution — this cancels ALL open orders.',
      category: 'admin_action',
      actions: [{ label: 'Order Book', type: 'link', target: '/dashboard/orderbook' }],
    };
  }

  // Margin
  if (/margin/.test(lower)) {
    try {
      const res = await fetchGateway('/api/v1/margin/calls/stats');
      if (res.ok) {
        const stats = await res.json() as Record<string, unknown>;
        return {
          reply: `Margin Call Stats:\n  Active calls: ${stats.total_active ?? 0}\n  Total shortfall: ${stats.total_shortfall ?? 0}\n  Participants in call: ${stats.participants_in_call ?? 0}`,
          category: 'admin_action',
          actions: [{ label: 'Margin Calls', type: 'link', target: '/dashboard/margin' }],
        };
      }
    } catch { /* fall through */ }
  }

  // Settlement
  if (/settle/.test(lower)) {
    return {
      reply: 'Settlement cycles can be triggered from the Settlement page. Go there to view cycle history or start a new cycle.',
      category: 'admin_action',
      actions: [{ label: 'Settlement', type: 'link', target: '/dashboard/settlement' }],
    };
  }

  // KYC / participants
  if (/kyc|participant|approve|reject|onboard/.test(lower)) {
    try {
      const res = await fetchGateway('/api/v1/participants');
      if (res.ok) {
        const data = await res.json() as { data?: Array<{ status?: string }> };
        const all = data.data ?? [];
        const pending = all.filter((p) => p.status === 'SUBMITTED' || p.status === 'UNDER_REVIEW').length;
        return {
          reply: `Participants: ${all.length} total, ${pending} pending review.\n\nGo to Participants to approve/reject KYC applications.`,
          category: 'admin_action',
          actions: [{ label: 'Participants', type: 'link', target: '/dashboard/participants' }],
        };
      }
    } catch { /* fall through */ }
  }

  // Alerts
  if (/alert|compliance|surveillance/.test(lower)) {
    try {
      const res = await fetchGateway('/api/v1/compliance/alerts');
      if (res.ok) {
        const data = await res.json() as { alerts?: unknown[]; data?: unknown[] };
        const alerts = data.alerts ?? data.data ?? [];
        return {
          reply: `Compliance alerts: ${(alerts as unknown[]).length} total.\n\nView and resolve alerts in Surveillance or Compliance pages.`,
          category: 'admin_action',
          actions: [
            { label: 'Surveillance', type: 'link', target: '/dashboard/surveillance' },
            { label: 'Compliance', type: 'link', target: '/dashboard/compliance' },
          ],
        };
      }
    } catch { /* fall through */ }
  }

  // Generic fallback — try to be helpful
  return {
    reply: `I can help you with:\n• View/manage instruments\n• Halt/resume trading\n• Check margin calls\n• Review KYC applications\n• View compliance alerts\n• Trigger settlement\n• Cancel orders\n\nWhat would you like to do?`,
    category: 'admin_action',
    actions: [
      { label: 'Dashboard', type: 'link', target: '/dashboard' },
      { label: 'Circuit Breakers', type: 'link', target: '/dashboard/circuit-breakers' },
    ],
  };
}

async function handleReportGenerate(): Promise<ChatResponse> {
  try {
    const healthRes = await fetchGateway('/api/v1/admin/health');
    const healthData = healthRes.ok ? await healthRes.json() as Record<string, unknown> : null;

    const lines: string[] = ['=== Platform Report ===', ''];

    if (healthData) {
      const services = Array.isArray(healthData)
        ? (healthData as HealthStatus[])
        : ((healthData as { services?: HealthStatus[] }).services ?? []);
      const healthy = services.filter((s: HealthStatus) => s.status === 'healthy').length;
      lines.push(`Services: ${healthy}/${services.length} healthy`);
    } else {
      lines.push('Services: Unable to fetch health data');
    }

    lines.push('', 'For detailed reports, use the Reports section in the admin dashboard.');

    return {
      reply: lines.join('\n'),
      category: 'report_generate',
      actions: [
        { label: 'Full Report', type: 'link', target: '/admin/reports' },
        { label: 'Settlement Status', type: 'link', target: '/admin/settlement' },
      ],
    };
  } catch (err) {
    return {
      reply: `Unable to generate report. Error: ${err instanceof Error ? err.message : String(err)}`,
      category: 'report_generate',
    };
  }
}
