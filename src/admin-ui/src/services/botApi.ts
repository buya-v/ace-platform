import { getConfig } from '../config';
import { getAccessToken } from './api';

export interface Action {
  id: string;
  label: string;
  type: 'link' | 'action' | 'api_call';
  payload: string;
}

export interface Suggestion {
  id: string;
  label: string;
  prompt: string;
  icon?: string;
}

export interface BotResponse {
  reply: string;
  actions?: Action[];
  suggestions?: Suggestion[];
}

export interface TicketInput {
  title: string;
  description: string;
  category: string;
  priority: string;
}

export interface TicketResult {
  id: string;
}

export function buildBotUrl(path: string): string {
  const config = getConfig();
  return `${config.API_BASE_URL}/bot${path}`;
}

export function buildAuthHeaders(): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  const token = getAccessToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return headers;
}

export function buildSendMessageBody(
  message: string,
  context?: { page?: string },
): string {
  return JSON.stringify({ message, context: context ?? {} });
}

export function buildCreateTicketBody(ticket: TicketInput): string {
  return JSON.stringify(ticket);
}

export async function sendBotMessage(
  message: string,
  context?: { page?: string },
): Promise<BotResponse> {
  const url = buildBotUrl('/chat');
  const response = await fetch(url, {
    method: 'POST',
    headers: buildAuthHeaders(),
    body: buildSendMessageBody(message, context),
  });

  if (!response.ok) {
    throw new Error(`Bot request failed: ${response.status}`);
  }

  const json = await response.json();
  const data = json.data ?? json;
  // Normalize suggestions from gateway format {text,category} to UI format {id,label,prompt}
  if (data.suggestions) {
    data.suggestions = data.suggestions.map(normalizeSuggestion);
  }
  return data;
}

export function normalizeSuggestion(raw: Record<string, string>, idx: number): Suggestion {
  return {
    id: raw.id ?? `sug-${idx}`,
    label: raw.label ?? raw.text ?? '',
    prompt: raw.prompt ?? raw.text ?? '',
    icon: raw.icon ?? raw.category,
  };
}

export async function getBotSuggestions(page: string): Promise<Suggestion[]> {
  const url = buildBotUrl(`/suggestions?page=${encodeURIComponent(page)}`);
  const response = await fetch(url, {
    method: 'GET',
    headers: buildAuthHeaders(),
  });

  if (!response.ok) {
    throw new Error(`Failed to get suggestions: ${response.status}`);
  }

  const json = await response.json();
  const items: Record<string, string>[] = json.data ?? json;
  return items.map(normalizeSuggestion);
}

export async function createTicket(ticket: TicketInput): Promise<TicketResult> {
  const url = buildBotUrl('/tickets');
  const response = await fetch(url, {
    method: 'POST',
    headers: buildAuthHeaders(),
    body: buildCreateTicketBody(ticket),
  });

  if (!response.ok) {
    throw new Error(`Failed to create ticket: ${response.status}`);
  }

  return response.json();
}
