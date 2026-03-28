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
export function fetchParticipants(params?: { status?: string; page?: number; limit?: number }, signal?: AbortSignal) {
  const qs = new URLSearchParams();
  if (params?.status) qs.set('status', params.status);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.limit) qs.set('limit', String(params.limit));
  const query = qs.toString();
  return apiFetch<import('../types').ApiResponse<import('../types').Participant[]>>(
    `/participants${query ? `?${query}` : ''}`,
    {},
    signal,
  );
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
