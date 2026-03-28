export type StepStatus = 'PENDING' | 'RUNNING' | 'PASS' | 'FAIL' | 'SKIP';

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE';

export interface StepDefinition {
  id: string;
  title: string;
  description: string;
  method: HttpMethod;
  url: string | ((state: Record<string, unknown>) => string);
  headers?: (state: Record<string, unknown>) => Record<string, string>;
  body?: (state: Record<string, unknown>) => unknown;
  validateResponse: (status: number, body: unknown) => StepStatus;
  extractState?: (body: unknown, state: Record<string, unknown>) => Record<string, unknown>;
}

export interface ChecklistItem {
  id: string;
  category: string;
  description: string;
  status: 'Ready' | 'Not Ready' | 'Partial';
}

export interface StepResult {
  stepId: string;
  status: StepStatus;
  requestMethod: string;
  requestUrl: string;
  requestHeaders: Record<string, string>;
  requestBody: unknown;
  responseStatus: number | null;
  responseBody: unknown;
  responseTime: number;
  error?: string;
}
