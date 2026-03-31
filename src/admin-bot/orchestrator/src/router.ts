import OpenAI from 'openai';
import type { RequestCategory } from './types.js';

const CATEGORY_VALUES: ReadonlySet<string> = new Set<string>([
  'status_query',
  'admin_action',
  'ticket_create',
  'ticket_update',
  'report_generate',
  'complex_analysis',
]);

function isValidCategory(value: string): value is RequestCategory {
  return CATEGORY_VALUES.has(value);
}

/**
 * Keyword-based fallback classification when no OpenAI API key is configured.
 */
export function classifyByKeyword(message: string): RequestCategory {
  const lower = message.toLowerCase();

  if (/\b(health|status|up|down|service|ping|alive|running)\b/.test(lower)) {
    return 'status_query';
  }
  if (/\b(halt|resume|cancel|disable|enable|suspend|activate|deactivate|block|unblock)\b/.test(lower)) {
    return 'admin_action';
  }
  if (/\b(update ticket|close ticket|resolve ticket|assign ticket|ticket\s+#?\d+)\b/.test(lower)) {
    return 'ticket_update';
  }
  if (/\b(bug|issue|ticket|request|create ticket|new ticket|file|submit)\b/.test(lower)) {
    return 'ticket_create';
  }
  if (/\b(report|summary|large trader|daily|eod|aggregate|statistics|analytics)\b/.test(lower)) {
    return 'report_generate';
  }

  return 'complex_analysis';
}

/**
 * Classify a user message into a RequestCategory.
 *
 * Uses GPT-nano (gpt-5.4-nano with gpt-4o-mini fallback) when OPENAI_API_KEY
 * is available. Falls back to keyword-based classification otherwise.
 */
export async function classifyRequest(message: string): Promise<RequestCategory> {
  const apiKey = process.env.OPENAI_API_KEY;
  if (!apiKey) {
    return classifyByKeyword(message);
  }

  const client = new OpenAI({ apiKey });

  const models = ['gpt-5.4-nano', 'gpt-4o-mini'];

  for (const model of models) {
    try {
      const response = await client.chat.completions.create({
        model,
        temperature: 0,
        max_tokens: 20,
        messages: [
          {
            role: 'system',
            content:
              'Classify this admin request into one of: status_query, admin_action, ticket_create, ticket_update, report_generate, complex_analysis. Respond with just the category.',
          },
          { role: 'user', content: message },
        ],
      });

      const raw = response.choices[0]?.message?.content?.trim().toLowerCase() ?? '';
      if (isValidCategory(raw)) {
        return raw;
      }
      // Model returned something unexpected — fall through to keyword
      return classifyByKeyword(message);
    } catch {
      // Model not available — try next, then keyword fallback
      continue;
    }
  }

  return classifyByKeyword(message);
}
