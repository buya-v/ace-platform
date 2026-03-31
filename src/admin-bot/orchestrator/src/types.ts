export type RequestCategory =
  | 'status_query'
  | 'admin_action'
  | 'ticket_create'
  | 'ticket_update'
  | 'report_generate'
  | 'complex_analysis';

export interface ChatRequest {
  message: string;
  context?: {
    page?: string;
    ticketId?: string;
    userId?: string;
  };
}

export interface ChatResponse {
  reply: string;
  actions?: Action[];
  ticket_id?: string;
  category?: RequestCategory;
}

export interface Action {
  label: string;
  type: 'link' | 'api_call';
  target: string;
}

export interface Suggestion {
  label: string;
  prompt: string;
  icon?: string;
}

export interface GatewayConfig {
  baseUrl: string;
  token?: string;
}

export interface HealthStatus {
  service: string;
  status: 'healthy' | 'unhealthy' | 'unknown';
  latency_ms?: number;
  details?: string;
}

export interface TicketPayload {
  title: string;
  description: string;
  priority?: 'low' | 'medium' | 'high' | 'critical';
}
