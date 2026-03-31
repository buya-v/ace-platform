import cron from 'node-cron';
import type { HealthStatus } from './types.js';

interface DailySchedulerOptions {
  gatewayUrl?: string;
  gatewayToken?: string;
  morningHour?: number;
  eodHour?: number;
  agingTicketsHour?: number;
}

function log(tag: string, message: string): void {
  const ts = new Date().toISOString();
  console.log(`[${ts}] [daily-admin:${tag}] ${message}`);
}

function getBaseUrl(options?: DailySchedulerOptions): string {
  return options?.gatewayUrl ?? process.env.GATEWAY_URL ?? 'http://localhost:8080';
}

function getHeaders(options?: DailySchedulerOptions): Record<string, string> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const token = options?.gatewayToken ?? process.env.GATEWAY_TOKEN;
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return headers;
}

async function fetchJSON<T>(baseUrl: string, path: string, headers: Record<string, string>): Promise<T | null> {
  try {
    const res = await fetch(`${baseUrl}${path}`, { headers });
    if (!res.ok) {
      log('fetch', `${path} returned ${res.status}`);
      return null;
    }
    return (await res.json()) as T;
  } catch (err) {
    log('fetch', `${path} failed: ${err instanceof Error ? err.message : String(err)}`);
    return null;
  }
}

async function morningHealthCheck(options?: DailySchedulerOptions): Promise<void> {
  log('morning', 'Starting morning health check');

  const baseUrl = getBaseUrl(options);
  const headers = getHeaders(options);

  const data = await fetchJSON<{ services?: HealthStatus[] } | HealthStatus[]>(
    baseUrl,
    '/api/v1/admin/health',
    headers,
  );

  if (!data) {
    log('morning', 'WARN: Unable to fetch health data from gateway');
    return;
  }

  const services: HealthStatus[] = Array.isArray(data) ? data : (data.services ?? []);
  const healthy = services.filter((s) => s.status === 'healthy').length;
  const unhealthy = services.filter((s) => s.status === 'unhealthy');

  log('morning', `Health: ${healthy}/${services.length} services healthy`);

  if (unhealthy.length > 0) {
    log('morning', `ALERT: Unhealthy services: ${unhealthy.map((s) => s.service).join(', ')}`);
  }
}

async function eodReport(options?: DailySchedulerOptions): Promise<void> {
  log('eod', 'Starting end-of-day report');

  const baseUrl = getBaseUrl(options);
  const headers = getHeaders(options);

  // Fetch settlement status
  const settlement = await fetchJSON<Record<string, unknown>>(
    baseUrl,
    '/api/v1/admin/settlement/status',
    headers,
  );

  if (settlement) {
    log('eod', `Settlement status: ${JSON.stringify(settlement)}`);
  } else {
    log('eod', 'Settlement status: unavailable');
  }

  // Fetch margin stats
  const margin = await fetchJSON<Record<string, unknown>>(
    baseUrl,
    '/api/v1/admin/margin/stats',
    headers,
  );

  if (margin) {
    log('eod', `Margin stats: ${JSON.stringify(margin)}`);
  } else {
    log('eod', 'Margin stats: unavailable');
  }

  // Compile summary
  const summary = [
    '=== End-of-Day Report ===',
    `Date: ${new Date().toISOString().split('T')[0]}`,
    `Settlement: ${settlement ? 'data available' : 'unavailable'}`,
    `Margin: ${margin ? 'data available' : 'unavailable'}`,
  ];

  log('eod', summary.join(' | '));
}

async function agingTicketsCheck(options?: DailySchedulerOptions): Promise<void> {
  log('aging', 'Checking for aging tickets');

  const baseUrl = getBaseUrl(options);
  const headers = getHeaders(options);

  const tickets = await fetchJSON<Array<{ id: string; title?: string; created_at?: string; status?: string }>>(
    baseUrl,
    '/api/v1/tickets?status=open',
    headers,
  );

  if (!tickets || !Array.isArray(tickets)) {
    log('aging', 'Unable to fetch ticket list');
    return;
  }

  const now = Date.now();
  const ONE_DAY_MS = 24 * 60 * 60 * 1000;

  const aging = tickets.filter((t) => {
    if (!t.created_at) return false;
    const created = new Date(t.created_at).getTime();
    return now - created > ONE_DAY_MS;
  });

  if (aging.length === 0) {
    log('aging', 'No aging tickets found');
    return;
  }

  log('aging', `WARNING: ${aging.length} ticket(s) open >24h:`);
  for (const t of aging) {
    const age = t.created_at
      ? Math.floor((now - new Date(t.created_at).getTime()) / ONE_DAY_MS)
      : '?';
    log('aging', `  #${t.id} "${t.title ?? 'untitled'}" - ${age} day(s) old`);
  }
}

export class DailyScheduler {
  private tasks: cron.ScheduledTask[] = [];
  private options: DailySchedulerOptions;

  constructor(options?: DailySchedulerOptions) {
    this.options = options ?? {};
  }

  start(): void {
    const morningHour = this.options.morningHour ?? 9;
    const eodHour = this.options.eodHour ?? 17;
    const agingHour = this.options.agingTicketsHour ?? 10;

    // Morning health check
    this.tasks.push(
      cron.schedule(`0 ${morningHour} * * *`, () => {
        morningHealthCheck(this.options).catch((err) =>
          log('morning', `Error: ${err instanceof Error ? err.message : String(err)}`),
        );
      }),
    );

    // EOD report
    this.tasks.push(
      cron.schedule(`0 ${eodHour} * * 1-5`, () => {
        eodReport(this.options).catch((err) =>
          log('eod', `Error: ${err instanceof Error ? err.message : String(err)}`),
        );
      }),
    );

    // Aging tickets check
    this.tasks.push(
      cron.schedule(`0 ${agingHour} * * *`, () => {
        agingTicketsCheck(this.options).catch((err) =>
          log('aging', `Error: ${err instanceof Error ? err.message : String(err)}`),
        );
      }),
    );

    log('scheduler', `Started: morning@${morningHour}:00, eod@${eodHour}:00 (M-F), aging@${agingHour}:00`);
  }

  stop(): void {
    for (const task of this.tasks) {
      task.stop();
    }
    this.tasks = [];
    log('scheduler', 'Stopped all scheduled tasks');
  }
}
