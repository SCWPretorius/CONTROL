import { createHmac } from 'crypto';
import { NormalizedEvent } from '../types/index.js';
import { v4 as uuidv4 } from 'uuid';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

let eventCallback: ((event: NormalizedEvent) => void) | null = null;

export function validateTelegramSignature(body: string, signature: string): boolean {
  if (!config.telegram.webhookSecret) return true;
  const expected = createHmac('sha256', config.telegram.webhookSecret)
    .update(body)
    .digest('hex');
  return signature === expected;
}

function normalizeUpdate(update: Record<string, unknown>): NormalizedEvent | null {
  const message = update['message'] as Record<string, unknown> | undefined;
  if (!message) return null;

  const from = message['from'] as Record<string, unknown> | undefined;

  return {
    id: uuidv4(),
    traceId: uuidv4(),
    source: 'telegram',
    type: 'message',
    payload: { update },
    timestamp: new Date().toISOString(),
    userId: from?.['id']?.toString(),
    role: 'user',
  };
}

const telegramIntegration = {
  name: 'telegram',

  async poll(): Promise<NormalizedEvent[] | null> {
    if (!config.telegram.botToken) return null;
    try {
      const response = await fetch(
        `https://api.telegram.org/bot${config.telegram.botToken}/getUpdates?timeout=5`
      );
      if (!response.ok) return null;
      const data = await response.json() as { ok: boolean; result: Record<string, unknown>[] };
      if (!data.ok) return null;
      return data.result
        .map(u => normalizeUpdate(u))
        .filter(Boolean) as NormalizedEvent[];
    } catch {
      return null;
    }
  },

  async send(payload: Record<string, unknown>): Promise<void> {
    if (!config.telegram.botToken) return;
    const { chatId, text } = payload as { chatId: string; text: string };
    await fetch(`https://api.telegram.org/bot${config.telegram.botToken}/sendMessage`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ chat_id: chatId, text }),
    });
  },

  onEvent(callback: (event: NormalizedEvent) => void): void {
    eventCallback = callback;
  },

  handleWebhook(body: string, signature: string): NormalizedEvent | null {
    if (!validateTelegramSignature(body, signature)) {
      logger.warn('[TELEGRAM] Invalid webhook signature');
      return null;
    }
    try {
      const update = JSON.parse(body) as Record<string, unknown>;
      const event = normalizeUpdate(update);
      if (event && eventCallback) eventCallback(event);
      return event;
    } catch (err) {
      logger.error({ err }, '[TELEGRAM] Failed to parse webhook');
      return null;
    }
  },
};

export default telegramIntegration;
