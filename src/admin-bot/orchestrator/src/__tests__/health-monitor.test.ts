import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { HealthMonitor, type GatewayFn } from '../health-monitor.js';

describe('HealthMonitor', () => {
  let mockGateway: ReturnType<typeof vi.fn>;
  let monitor: HealthMonitor;

  beforeEach(() => {
    mockGateway = vi.fn();
    monitor = new HealthMonitor(mockGateway as GatewayFn, 60000);
  });

  afterEach(() => {
    monitor.stop();
  });

  function mockHealthResponse(services: Array<{ service: string; status: string }>) {
    return { ok: true, json: async () => ({ services }) };
  }

  function mockMarginResponse(stats: { participants_in_call: number; total_shortfall?: number }) {
    return { ok: true, json: async () => stats };
  }

  function mockTicketResponse(tickets: Array<{ id: string; created_at: string }>) {
    return { ok: true, json: async () => ({ data: tickets }) };
  }

  function mockNotOk() {
    return { ok: false, status: 500 };
  }

  it('detects service_down transition', async () => {
    // First check — all healthy (establishes baseline)
    mockGateway
      .mockResolvedValueOnce(mockHealthResponse([{ service: 'gateway', status: 'healthy' }]))
      .mockResolvedValueOnce(mockNotOk())
      .mockResolvedValueOnce(mockNotOk());
    await monitor.runChecks();
    expect(monitor.drainAlerts()).toHaveLength(0);

    // Second check — gateway goes down
    mockGateway
      .mockResolvedValueOnce(mockHealthResponse([{ service: 'gateway', status: 'unhealthy' }]))
      .mockResolvedValueOnce(mockNotOk())
      .mockResolvedValueOnce(mockNotOk());
    await monitor.runChecks();

    const alerts = monitor.drainAlerts();
    expect(alerts).toHaveLength(1);
    expect(alerts[0].type).toBe('service_down');
    expect(alerts[0].service).toBe('gateway');
    expect(alerts[0].severity).toBe('critical');
  });

  it('detects service_recovered transition', async () => {
    // First check — unhealthy
    mockGateway
      .mockResolvedValueOnce(mockHealthResponse([{ service: 'matching', status: 'unhealthy' }]))
      .mockResolvedValueOnce(mockNotOk())
      .mockResolvedValueOnce(mockNotOk());
    await monitor.runChecks();
    monitor.drainAlerts(); // clear

    // Second check — recovered
    mockGateway
      .mockResolvedValueOnce(mockHealthResponse([{ service: 'matching', status: 'healthy' }]))
      .mockResolvedValueOnce(mockNotOk())
      .mockResolvedValueOnce(mockNotOk());
    await monitor.runChecks();

    const alerts = monitor.drainAlerts();
    expect(alerts).toHaveLength(1);
    expect(alerts[0].type).toBe('service_recovered');
    expect(alerts[0].severity).toBe('info');
  });

  it('detects high margin utilization', async () => {
    mockGateway
      .mockResolvedValueOnce(mockNotOk()) // health
      .mockResolvedValueOnce(mockMarginResponse({ participants_in_call: 3, total_shortfall: 50000 }))
      .mockResolvedValueOnce(mockNotOk()); // tickets
    await monitor.runChecks();

    const alerts = monitor.drainAlerts();
    expect(alerts).toHaveLength(1);
    expect(alerts[0].type).toBe('high_margin_utilization');
    expect(alerts[0].message).toContain('3 participant');
    expect(alerts[0].severity).toBe('warning');
  });

  it('detects aging tickets', async () => {
    const oldDate = new Date(Date.now() - 48 * 60 * 60 * 1000).toISOString(); // 48h ago
    mockGateway
      .mockResolvedValueOnce(mockNotOk()) // health
      .mockResolvedValueOnce(mockNotOk()) // margin
      .mockResolvedValueOnce(mockTicketResponse([
        { id: 'TK-1', created_at: oldDate },
        { id: 'TK-2', created_at: new Date().toISOString() }, // recent — not aging
      ]));
    await monitor.runChecks();

    const alerts = monitor.drainAlerts();
    expect(alerts).toHaveLength(1);
    expect(alerts[0].type).toBe('aging_tickets');
    expect(alerts[0].message).toContain('1 ticket');
  });

  it('drains alerts and clears queue', async () => {
    mockGateway
      .mockResolvedValueOnce(mockNotOk())
      .mockResolvedValueOnce(mockMarginResponse({ participants_in_call: 1 }))
      .mockResolvedValueOnce(mockNotOk());
    await monitor.runChecks();

    const first = monitor.drainAlerts();
    expect(first).toHaveLength(1);

    const second = monitor.drainAlerts();
    expect(second).toHaveLength(0);
  });

  it('does not alert when margin has zero participants in call', async () => {
    mockGateway
      .mockResolvedValueOnce(mockNotOk())
      .mockResolvedValueOnce(mockMarginResponse({ participants_in_call: 0 }))
      .mockResolvedValueOnce(mockNotOk());
    await monitor.runChecks();
    expect(monitor.drainAlerts()).toHaveLength(0);
  });

  it('no alerts when gateway is unreachable', async () => {
    mockGateway.mockRejectedValue(new Error('ECONNREFUSED'));
    await monitor.runChecks();
    expect(monitor.drainAlerts()).toHaveLength(0);
  });

  it('start and stop controls interval', () => {
    monitor.start();
    monitor.start(); // idempotent
    monitor.stop();
    monitor.stop(); // idempotent
  });

  it('getAlertCount returns current count', async () => {
    mockGateway
      .mockResolvedValueOnce(mockNotOk())
      .mockResolvedValueOnce(mockMarginResponse({ participants_in_call: 2 }))
      .mockResolvedValueOnce(mockNotOk());
    await monitor.runChecks();
    expect(monitor.getAlertCount()).toBe(1);
  });
});
