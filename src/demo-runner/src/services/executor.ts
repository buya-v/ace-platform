import { StepDefinition, StepResult, StepStatus } from '../types/step';

export async function executeStep(
  step: StepDefinition,
  gatewayUrl: string,
  state: Record<string, unknown>,
): Promise<{ result: StepResult; newState: Record<string, unknown> }> {
  const url = typeof step.url === 'function' ? step.url(state) : step.url;
  const resolvedUrl = resolveUrl(url, gatewayUrl);
  const headers = step.headers ? step.headers(state) : {};
  const body = step.body ? step.body(state) : undefined;

  const requestBody = body;
  const requestHeaders = { ...headers };

  const start = performance.now();
  let responseStatus: number | null = null;
  let responseBody: unknown = null;
  let status: StepStatus = 'FAIL';
  let error: string | undefined;

  try {
    const fetchOptions: RequestInit = {
      method: step.method,
      headers: {
        'Content-Type': 'application/json',
        ...requestHeaders,
      },
    };
    if (body !== undefined && step.method !== 'GET') {
      fetchOptions.body = JSON.stringify(body);
    }

    const response = await fetch(resolvedUrl, fetchOptions);
    responseStatus = response.status;

    const text = await response.text();
    try {
      responseBody = JSON.parse(text);
    } catch {
      responseBody = text;
    }

    status = step.validateResponse(responseStatus, responseBody);
  } catch (err) {
    error = err instanceof Error ? err.message : String(err);
    status = 'FAIL';
  }

  const responseTime = Math.round(performance.now() - start);

  let newState = { ...state };
  if (status === 'PASS' && step.extractState && responseBody) {
    newState = { ...newState, ...step.extractState(responseBody, state) };
  }

  return {
    result: {
      stepId: step.id,
      status,
      requestMethod: step.method,
      requestUrl: resolvedUrl,
      requestHeaders,
      requestBody,
      responseStatus,
      responseBody,
      responseTime,
      error,
    },
    newState,
  };
}

export function resolveUrl(url: string, gatewayUrl: string): string {
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url;
  }
  const base = gatewayUrl.replace(/\/$/, '');
  const path = url.startsWith('/') ? url : `/${url}`;
  return `${base}${path}`;
}
