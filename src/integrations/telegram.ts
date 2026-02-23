import { createHmac } from 'crypto';
import { NormalizedEvent } from '../types/index.js';
import { v4 as uuidv4 } from 'uuid';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

let eventCallback: ((event: NormalizedEvent) => void) | null = null;
let lastUpdateId = 0;

export function validateTelegramSignature(body: string, signature: string): boolean {
  if (!config.telegram.webhookSecret) return true;
  const expected = createHmac('sha256', config.telegram.webhookSecret)
    .update(body)
    .digest('hex');
  return signature === expected;
}

function normalizeUpdate(update: Record<string, unknown>): NormalizedEvent | null {
  const message = update['message'] as Record<string, unknown> | undefined;
  if (!message) {
    logger.debug({ update }, '[TELEGRAM] Skipping non-message update');
    return null;
  }

  const from = message['from'] as Record<string, unknown> | undefined;
  const text = message['text'] as string | undefined;
  const chat = message['chat'] as Record<string, unknown> | undefined;

  // Skip messages without text (photos, stickers, etc.)
  if (!text || text.trim().length === 0) {
    logger.debug({ messageId: message['message_id'] }, '[TELEGRAM] Skipping message without text');
    return null;
  }

  const role = config.adminUserIds.includes(from?.['id']?.toString() ?? '') ? 'admin' : 'user';
  const userId = from?.['id']?.toString();

  const event: NormalizedEvent = {
    id: uuidv4(),
    traceId: uuidv4(),
    source: 'telegram',
    type: 'message',
    payload: {
      text,
      chatId: chat?.['id']?.toString() ?? from?.['id']?.toString() ?? '',
      messageId: message['message_id'],
      from: {
        id: from?.['id'],
        firstName: from?.['first_name'],
        username: from?.['username'],
      },
      update,
    },
    timestamp: new Date().toISOString(),
    userId,
    role,
  };
  
  logger.debug(
    { 
      userId, 
      adminUserIds: config.adminUserIds, 
      role 
    }, 
    '[TELEGRAM] User role assigned'
  );
  
  return event;
}

const telegramIntegration = {
  name: 'telegram',

  async poll(): Promise<NormalizedEvent[] | null> {
    if (!config.telegram.botToken) return null;
    try {
      const offset = lastUpdateId > 0 ? lastUpdateId + 1 : undefined;
      const url = new URL(`https://api.telegram.org/bot${config.telegram.botToken}/getUpdates`);
      url.searchParams.set('timeout', '5');
      if (offset) {
        url.searchParams.set('offset', offset.toString());
      }

      const response = await fetch(url.toString());
      if (!response.ok) return null;

      const data = await response.json() as { ok: boolean; result: Record<string, unknown>[] };
      if (!data.ok || !data.result || data.result.length === 0) return null;

      // Update lastUpdateId to the highest update_id received
      for (const update of data.result) {
        const updateId = update['update_id'] as number;
        if (updateId > lastUpdateId) {
          lastUpdateId = updateId;
        }
      }

      const events = data.result
        .map(u => normalizeUpdate(u))
        .filter(Boolean) as NormalizedEvent[];

      if (events.length > 0) {
        logger.debug({ count: events.length, lastUpdateId }, '[TELEGRAM] Processed updates');
      }

      return events;
    } catch (err) {
      logger.error({ err }, '[TELEGRAM] Poll failed');
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
