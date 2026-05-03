export interface ProactiveAlert {
  type: 'service_down' | 'service_recovered' | 'high_margin_utilization' | 'aging_tickets';
  service: string;
  message: string;
  timestamp: number;
  severity: 'critical' | 'warning' | 'info';
}

export type GatewayFn = (path: string, options?: RequestInit) => Promise<Response>;

interface ServiceState {
  status: 'healthy' | 'unhealthy' | 'unknown';
}

export class HealthMonitor {
  private intervalId: ReturnType<typeof setInterval> | null = null;
  private alertQueue: ProactiveAlert[] = [];
  private previousServiceStates: Map<string, ServiceState> = new Map();
  private gateway: GatewayFn;
  private intervalMs: number;

  constructor(gateway: GatewayFn, intervalMs?: number) {
    this.gateway = gateway;
    this.intervalMs = intervalMs ?? parseInt(process.env.HEALTH_CHECK_INTERVAL_MS ?? '60000', 10);
  }

  start(): void {
    if (this.intervalId) return;
    console.log(`[health-monitor] Starting with ${this.intervalMs}ms interval`);
    this.intervalId = setInterval(() => {
      this.runChecks().catch((err) => {
        console.error('[health-monitor] Check failed:', err);
      });
    }, this.intervalMs);
    // Run immediately on start
    this.runChecks().catch((err) => {
      console.error('[health-monitor] Initial check failed:', err);
    });
  }

  stop(): void {
    if (this.intervalId) {
      clearInterval(this.intervalId);
      this.intervalId = null;
      console.log('[health-monitor] Stopped');
    }
  }

  drainAlerts(): ProactiveAlert[] {
    const alerts = [...this.alertQueue];
    this.alertQueue = [];
    return alerts;
  }

  getAlertCount(): number {
    return this.alertQueue.length;
  }

  private pushAlert(alert: ProactiveAlert): void {
    this.alertQueue.push(alert);
  }

  async runChecks(): Promise<void> {
    await Promise.allSettled([
      this.checkServiceHealth(),
      this.checkMarginUtilization(),
      this.checkAgingTickets(),
    ]);
  }

  private async checkServiceHealth(): Promise<void> {
    try {
      const res = await this.gateway('/api/v1/admin/health');
      if (!res.ok) return;
      const data = await res.json() as { services?: Array<{ service?: string; name?: string; status: string }> };
      const services = Array.isArray(data) ? data : (data.services ?? []);

      for (const svc of services) {
        const name = svc.service ?? (svc as unknown as { name: string }).name ?? 'unknown';
        const currentStatus = svc.status === 'healthy' ? 'healthy' : 'unhealthy';
        const prev = this.previousServiceStates.get(name);

        if (prev && prev.status === 'healthy' && currentStatus === 'unhealthy') {
          this.pushAlert({
            type: 'service_down',
            service: name,
            message: `Service ${name} is down.`,
            timestamp: Date.now(),
            severity: 'critical',
          });
        } else if (prev && prev.status === 'unhealthy' && currentStatus === 'healthy') {
          this.pushAlert({
            type: 'service_recovered',
            service: name,
            message: `Service ${name} has recovered.`,
            timestamp: Date.now(),
            severity: 'info',
          });
        }

        this.previousServiceStates.set(name, { status: currentStatus });
      }
    } catch {
      // Gateway unreachable — don't flood alerts
    }
  }

  private async checkMarginUtilization(): Promise<void> {
    try {
      const res = await this.gateway('/api/v1/margin/calls/stats');
      if (!res.ok) return;
      const stats = await res.json() as { participants_in_call?: number; total_shortfall?: number };
      if (stats.participants_in_call && stats.participants_in_call > 0) {
        this.pushAlert({
          type: 'high_margin_utilization',
          service: 'margin-engine',
          message: `${stats.participants_in_call} participant(s) in margin call. Total shortfall: ${stats.total_shortfall ?? 0}`,
          timestamp: Date.now(),
          severity: 'warning',
        });
      }
    } catch {
      // Skip if margin service unavailable
    }
  }

  private async checkAgingTickets(): Promise<void> {
    try {
      const res = await this.gateway('/api/v1/tickets?status=open');
      if (!res.ok) return;
      const data = await res.json() as { data?: Array<{ id: string; created_at: string }> };
      const tickets = data.data ?? (Array.isArray(data) ? data : []);
      const now = Date.now();
      const DAY_MS = 24 * 60 * 60 * 1000;

      const aging = (tickets as Array<{ id: string; created_at: string }>).filter((t) => {
        const created = new Date(t.created_at).getTime();
        return now - created > DAY_MS;
      });

      if (aging.length > 0) {
        this.pushAlert({
          type: 'aging_tickets',
          service: 'tickets',
          message: `${aging.length} ticket(s) open longer than 24 hours.`,
          timestamp: Date.now(),
          severity: 'warning',
        });
      }
    } catch {
      // Skip if ticket service unavailable
    }
  }
}
