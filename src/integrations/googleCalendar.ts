import { NormalizedEvent } from '../types/index.js';
import { v4 as uuidv4 } from 'uuid';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

let eventCallback: ((event: NormalizedEvent) => void) | null = null;

const googleCalendarIntegration = {
  name: 'calendar',

  async poll(): Promise<NormalizedEvent[] | null> {
    if (!config.google.clientId) return null;
    logger.debug('[CALENDAR] Poll called (requires OAuth token setup)');
    return null;
  },

  async send(payload: Record<string, unknown>): Promise<void> {
    logger.info({ payload }, '[CALENDAR] Send called');
  },

  onEvent(callback: (event: NormalizedEvent) => void): void {
    eventCallback = callback;
  },

  handlePushWebhook(body: string, _token: string): NormalizedEvent | null {
    try {
      const notification = JSON.parse(body) as Record<string, unknown>;
      const event: NormalizedEvent = {
        id: uuidv4(),
        traceId: uuidv4(),
        source: 'calendar',
        type: 'push-notification',
        payload: { notification },
        timestamp: new Date().toISOString(),
        role: 'user',
      };
      if (eventCallback) eventCallback(event);
      return event;
    } catch (err) {
      logger.error({ err }, '[CALENDAR] Failed to parse push webhook');
      return null;
    }
  },
};

export default googleCalendarIntegration;
