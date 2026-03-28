export interface ApiResponse<T> {
  data: T;
  pagination?: {
    page: number;
    limit: number;
    total: number;
    total_pages: number;
  };
}

export interface ServiceHealth {
  name: string;
  status: 'healthy' | 'degraded' | 'unhealthy';
  latency_ms: number;
  last_check: string;
  uptime_seconds: number;
  version: string;
}

export interface HealthResponse {
  services: ServiceHealth[];
  overall_status: 'healthy' | 'degraded' | 'unhealthy';
}

export interface User {
  id: string;
  email: string;
  name: string;
  roles: string[];
  participant_id: string | null;
}

export interface Participant {
  id: string;
  name: string;
  email: string;
  organization: string;
  kyc_status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'UNDER_REVIEW';
  risk_score: number;
  submitted_at: string;
  updated_at: string;
}

export interface ParticipantDocument {
  id: string;
  name: string;
  type: string;
  uploaded_at: string;
  url: string;
}

export interface MarginCall {
  id: string;
  participant_id: string;
  participant_name: string;
  instrument_id: string;
  required_margin: string;
  current_margin: string;
  shortfall: string;
  status: 'PENDING' | 'MET' | 'BREACHED';
  issued_at: string;
  deadline: string;
}

export interface MarginCallStats {
  total_active: number;
  total_shortfall: string;
  participants_in_call: number;
  average_utilization: number;
}

export interface SettlementCycle {
  id: string;
  phase: 'OPEN' | 'NETTING' | 'SETTLING' | 'COMPLETED' | 'FAILED';
  started_at: string;
  expected_completion: string;
  total_settlements: number;
  total_value: string;
  completed_at?: string;
}

export interface InstrumentControl {
  instrument_id: string;
  ticker: string;
  last_price: string;
  upper_limit: string;
  lower_limit: string;
  status: 'TRADING' | 'HALTED' | 'PRE_OPEN';
  daily_volume: number;
}

export interface WarehouseReceipt {
  id: string;
  commodity: string;
  grade: string;
  quantity: string;
  unit: string;
  warehouse_name: string;
  status: 'ACTIVE' | 'PLEDGED' | 'IN_TRANSIT' | 'DELIVERED' | 'CANCELLED';
  issued_at: string;
  holder_name: string;
}

export interface WarehouseFacility {
  id: string;
  name: string;
  location: string;
  total_capacity: number;
  used_capacity: number;
  unit: string;
}

export interface PendingDelivery {
  id: string;
  receipt_id: string;
  from_warehouse: string;
  to_destination: string;
  commodity: string;
  quantity: string;
  status: 'PENDING' | 'IN_TRANSIT' | 'DELIVERED';
  requested_at: string;
}

export interface ComplianceAlert {
  id: string;
  participant_id: string;
  participant_name: string;
  alert_type: 'UNUSUAL_VOLUME' | 'WATCHLIST_MATCH' | 'PATTERN_DETECTED';
  severity: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
  description: string;
  status: 'OPEN' | 'UNDER_REVIEW' | 'RESOLVED' | 'DISMISSED';
  created_at: string;
}

export interface AuditEvent {
  id: string;
  timestamp: string;
  actor: string;
  action: string;
  target_type: string;
  target_id: string;
  details: Record<string, unknown>;
  ip_address: string;
}

export type AdminRole = 'admin' | 'exchange_admin' | 'compliance_officer';

export function isAdminRole(role: string): boolean {
  return role === 'admin' || role === 'exchange_admin';
}

export function hasAdminAccess(roles: string[]): boolean {
  return roles.some(isAdminRole);
}

export function hasComplianceAccess(roles: string[]): boolean {
  return roles.some(r => isAdminRole(r) || r === 'compliance_officer');
}
