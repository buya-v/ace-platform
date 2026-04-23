import { getConfig } from '../config';

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

let accessToken: string | null = null;
let onUnauthorized: (() => void) | null = null;

export function setAccessToken(token: string | null): void {
  accessToken = token;
}

export function getAccessToken(): string | null {
  return accessToken;
}

export function setOnUnauthorized(cb: () => void): void {
  onUnauthorized = cb;
}

export async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
  signal?: AbortSignal,
): Promise<T> {
  const config = getConfig();
  const url = `${config.API_BASE_URL}${path}`;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  };

  if (accessToken) {
    headers['Authorization'] = `Bearer ${accessToken}`;
  }

  const response = await fetch(url, {
    ...options,
    headers,
    signal,
  });

  if (response.status === 401) {
    onUnauthorized?.();
    throw new ApiError(401, 'UNAUTHORIZED', 'Unauthorized');
  }

  if (!response.ok) {
    let code = 'UNKNOWN';
    let message = response.statusText;
    try {
      const body = await response.json();
      code = body.error?.code ?? code;
      message = body.error?.message ?? message;
    } catch {
      // ignore parse errors
    }
    throw new ApiError(response.status, code, message);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json();
}

// Auth
export function login(email: string, password: string) {
  return apiFetch<{ access_token: string; refresh_token: string; expires_in: number }>(
    '/auth/login',
    { method: 'POST', body: JSON.stringify({ email, password }) },
  );
}

export function refreshToken() {
  return apiFetch<{ access_token: string; expires_in: number }>(
    '/auth/refresh',
    { method: 'POST' },
  );
}

export function logout() {
  return apiFetch<void>('/auth/logout', { method: 'POST' });
}

// Health
export function fetchHealth(signal?: AbortSignal) {
  return apiFetch<{ services: import('../types').ServiceHealth[]; overall_status: string }>(
    '/admin/health',
    {},
    signal,
  );
}

// Participants
export async function fetchParticipants(params?: { status?: string; page?: number; limit?: number }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.status) qs.set('status', params.status);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.limit) qs.set('limit', String(params.limit));
  const query = qs.toString();
  const raw = await apiFetch<Record<string, unknown>>(
    `/participants${query ? `?${query}` : ''}`,
    {},
    signal,
  );
  // Normalize: compliance service returns { applications: [...] } with different field names
  const rawItems = (raw as any).data ?? (raw as any).applications ?? [];
  const items = rawItems.map((a: Record<string, unknown>) => ({
    id: a.ApplicationID ?? a.id ?? '',
    name: a.LegalName ?? a.entity_name ?? a.name ?? '',
    email: (a.Contact as Record<string, unknown>)?.Email ?? a.email ?? '',
    organization: a.TradingName ?? a.organization ?? '',
    kyc_status: a.Status ?? a.status ?? a.kyc_status ?? 'PENDING',
    risk_score: a.risk_score ?? 0,
    submitted_at: a.CreatedAt ?? a.submitted_at ?? a.created_at ?? '',
    updated_at: a.UpdatedAt ?? a.updated_at ?? '',
  }));
  return { data: items, pagination: (raw as any).pagination } as import('../types').ApiResponse<import('../types').Participant[]>;
}

export function fetchParticipant(id: string, signal?: AbortSignal) {
  return apiFetch<import('../types').Participant>(`/participants/${id}`, {}, signal);
}

export function approveParticipant(id: string) {
  return apiFetch<void>(`/participants/${id}/approve`, { method: 'POST' });
}

export function rejectParticipant(id: string, reason: string) {
  return apiFetch<void>(`/participants/${id}/reject`, { method: 'POST', body: JSON.stringify({ reason }) });
}

export function fetchParticipantDocuments(id: string, signal?: AbortSignal) {
  return apiFetch<import('../types').ParticipantDocument[]>(`/participants/${id}/documents`, {}, signal);
}

// Margin
export function fetchMarginCalls(signal?: AbortSignal) {
  return apiFetch<import('../types').ApiResponse<import('../types').MarginCall[]>>('/margin/calls', {}, signal);
}

export function fetchMarginCallStats(signal?: AbortSignal) {
  return apiFetch<import('../types').MarginCallStats>('/margin/calls/stats', {}, signal);
}

export function triggerMarginCalculation(participantId: string, instrumentId: string) {
  return apiFetch<void>('/margin/calculate', {
    method: 'POST',
    body: JSON.stringify({ participant_id: participantId, instrument_id: instrumentId }),
  });
}

// Settlement
export function fetchSettlementCycles(params?: { status?: string }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.status) qs.set('status', params.status);
  const query = qs.toString();
  return apiFetch<import('../types').ApiResponse<import('../types').SettlementCycle[]>>(
    `/settlement/cycles${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

// Circuit Breakers
export function fetchInstruments(signal?: AbortSignal) {
  return apiFetch<import('../types').ApiResponse<import('../types').InstrumentControl[]>>('/instruments', {}, signal);
}

export function setCircuitBreaker(instrumentId: string, config: { upper_limit_pct: number; lower_limit_pct: number; cooldown_minutes: number; reference_price: string }) {
  return apiFetch<void>(`/admin/instruments/${instrumentId}/circuit-breaker`, {
    method: 'PUT',
    body: JSON.stringify(config),
  });
}

export function haltTrading(instrumentId: string) {
  return apiFetch<void>(`/admin/instruments/${instrumentId}/halt`, { method: 'POST' });
}

export function resumeTrading(instrumentId: string) {
  return apiFetch<void>(`/admin/instruments/${instrumentId}/resume`, { method: 'POST' });
}

export function haltInstrument(id: string, signal?: AbortSignal) {
  return apiFetch<void>(`/admin/instruments/${id}/halt`, { method: 'POST' }, signal);
}

export function resumeInstrument(id: string, signal?: AbortSignal) {
  return apiFetch<void>(`/admin/instruments/${id}/resume`, { method: 'POST' }, signal);
}

// Warehouse
export function fetchWarehouseReceipts(params?: { status?: string; page?: number }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.status) qs.set('status', params.status);
  if (params?.page) qs.set('page', String(params.page));
  const query = qs.toString();
  return apiFetch<import('../types').ApiResponse<import('../types').WarehouseReceipt[]>>(
    `/warehouse/receipts${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

export function fetchWarehouseDeliveries(params?: { status?: string }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.status) qs.set('status', params.status);
  const query = qs.toString();
  return apiFetch<import('../types').ApiResponse<import('../types').PendingDelivery[]>>(
    `/warehouse/deliveries${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

export function fetchWarehouseFacilities(signal?: AbortSignal) {
  return apiFetch<import('../types').ApiResponse<import('../types').WarehouseFacility[]>>('/warehouse/facilities', {}, signal);
}

// Compliance
export function fetchComplianceAlerts(params?: { status?: string }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.status) qs.set('status', params.status);
  const query = qs.toString();
  return apiFetch<import('../types').ApiResponse<import('../types').ComplianceAlert[]>>(
    `/compliance/alerts${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

export function resolveAlert(alertId: string) {
  return apiFetch<void>(`/compliance/alerts/${alertId}/resolve`, { method: 'POST' });
}

export function fileSAR(data: { participant_id: string; alert_type: string; description: string }) {
  return apiFetch<void>('/compliance/sar', { method: 'POST', body: JSON.stringify(data) });
}

// Instruments / Order Book / Market Data
export function fetchInstrumentList(signal?: AbortSignal) {
  return apiFetch<{ instruments?: any[]; [key: string]: any }>('/instruments/list', {}, signal);
}

export function fetchOrderBook(instrumentId: string, signal?: AbortSignal) {
  return apiFetch<any>(`/instruments/${instrumentId}/book`, {}, signal);
}

export function fetchLastTrade(instrumentId: string, signal?: AbortSignal) {
  return apiFetch<any>(`/instruments/${instrumentId}/trades/latest`, {}, signal);
}

export function fetchMarketTrades(instrumentId: string, signal?: AbortSignal) {
  return apiFetch<any>(`/market-data/trades/${instrumentId}`, {}, signal);
}

export function fetchTicker(instrumentId: string, signal?: AbortSignal) {
  return apiFetch<any>(`/market-data/ticker/${instrumentId}`, {}, signal);
}

// Positions / Netting / Portfolio Margin
export function fetchPositions(signal?: AbortSignal) {
  return apiFetch<any[]>('/clearing/positions', {}, signal);
}

export function fetchNetting(signal?: AbortSignal) {
  return apiFetch<any>('/clearing/netting', {}, signal);
}

export function fetchPortfolioMargin(signal?: AbortSignal) {
  return apiFetch<any>('/margin', {}, signal);
}

// Admin actions
export function triggerSettlementCycle() {
  return apiFetch<{ cycle_id: string }>('/settlement/cycle', { method: 'POST' });
}

export function massCancel() {
  return apiFetch<void>('/admin/mass-cancel', { method: 'POST' });
}

// Surveillance
export function fetchSurveillanceAlerts(params?: { severity?: string; status?: string }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.severity) qs.set('severity', params.severity);
  if (params?.status) qs.set('status', params.status);
  const query = qs.toString();
  return apiFetch<import('../types').ApiResponse<import('../pages/Surveillance').SurveillanceAlert[]>>(
    `/compliance/surveillance/alerts${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

export function resolveSurveillanceAlert(alertId: string) {
  return apiFetch<void>(`/compliance/surveillance/alerts/${alertId}/resolve`, { method: 'POST' });
}

// Fee Management
export function fetchFeeSchedule(signal?: AbortSignal) {
  return apiFetch<import('../types').ApiResponse<import('../pages/FeeManagement').FeeRule[]>>(
    '/admin/fees',
    {},
    signal,
  );
}

// Reports
export function fetchMarketSummaryReport(date: string, signal?: AbortSignal) {
  return apiFetch<import('../types').ApiResponse<import('../pages/Reports').MarketSummaryRow[]>>(
    `/admin/reports/market-summary?date=${encodeURIComponent(date)}`,
    {},
    signal,
  );
}

export function fetchLargeTraderReport(date: string, signal?: AbortSignal) {
  return apiFetch<import('../types').ApiResponse<import('../pages/Reports').LargeTraderRow[]>>(
    `/admin/reports/large-traders?date=${encodeURIComponent(date)}`,
    {},
    signal,
  );
}

// Tickets
export function fetchTickets(params?: { category?: string; priority?: string; status?: string }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.category) qs.set('category', params.category);
  if (params?.priority) qs.set('priority', params.priority);
  if (params?.status) qs.set('status', params.status);
  const query = qs.toString();
  return apiFetch<{ data: import('../pages/Tickets').Ticket[]; total: number }>(
    `/tickets${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

export function fetchTicket(id: string, signal?: AbortSignal) {
  return apiFetch<{ ticket: import('../pages/Tickets').Ticket; comments: import('../pages/Tickets').TicketComment[] }>(
    `/tickets/${id}`,
    {},
    signal,
  );
}

export function updateTicket(id: string, updates: Record<string, unknown>) {
  return apiFetch<import('../pages/Tickets').Ticket>(`/tickets/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(updates),
  });
}

export function addTicketComment(ticketId: string, body: string) {
  return apiFetch<import('../pages/Tickets').TicketComment>(`/tickets/${ticketId}/comments`, {
    method: 'POST',
    body: JSON.stringify({ body }),
  });
}

export function fetchTicketStats(signal?: AbortSignal) {
  return apiFetch<{ total: number; open: number; in_progress: number; resolved: number; closed: number }>(
    '/tickets/stats',
    {},
    signal,
  );
}

// Audit
export function fetchAuditTrail(params?: { actor?: string; action?: string; from?: string; to?: string; page?: number }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.actor) qs.set('actor', params.actor);
  if (params?.action) qs.set('action', params.action);
  if (params?.from) qs.set('from', params.from);
  if (params?.to) qs.set('to', params.to);
  if (params?.page) qs.set('page', String(params.page));
  const query = qs.toString();
  return apiFetch<import('../types').ApiResponse<import('../types').AuditEvent[]>>(
    `/compliance/audit-trail${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

// Securities Instruments
export function fetchSecuritiesInstruments(filters?: { asset_class?: string; trading_status?: string }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (filters?.asset_class) qs.set('asset_class', filters.asset_class);
  if (filters?.trading_status) qs.set('trading_status', filters.trading_status);
  const query = qs.toString();
  return apiFetch<any>(
    `/securities/instruments${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

export function createSecuritiesInstrument(data: any) {
  return apiFetch<any>('/securities/instruments', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export function updateInstrumentStatus(id: string, status: string, reason?: string) {
  return apiFetch<void>(`/securities/instruments/${id}/status`, {
    method: 'PUT',
    body: JSON.stringify({ status, ...(reason ? { reason } : {}) }),
  });
}

// Securities Orders
export function fetchSecuritiesOrders(filters?: { instrument_id?: string; status?: string }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (filters?.instrument_id) qs.set('instrument_id', filters.instrument_id);
  if (filters?.status) qs.set('status', filters.status);
  const query = qs.toString();
  return apiFetch<any>(
    `/securities/orders${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}

export function submitSecuritiesOrder(data: any) {
  return apiFetch<any>('/securities/orders', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export function cancelSecuritiesOrder(id: string) {
  return apiFetch<void>(`/securities/orders/${id}/cancel`, { method: 'POST' });
}

// Securities Positions
export function fetchSecuritiesPositions(filters?: { participant_id?: string }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (filters?.participant_id) qs.set('participant_id', filters.participant_id);
  const query = qs.toString();
  return apiFetch<any>(
    `/securities/positions${query ? `?${query}` : ''}`,
    {},
    signal,
  );
}
