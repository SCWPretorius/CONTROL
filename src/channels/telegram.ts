import telegramIntegration from '../integrations/telegram.js';
import { chunkMessage, type Channel } from './channel-interface.js';

export const TELEGRAM_MAX_LENGTH = 4096;

export const telegramChannel: Channel = {
  name: 'telegram',
  capabilities: { maxMessageLength: TELEGRAM_MAX_LENGTH, supportsMarkdown: true },

  async send(chatId: string, text: string): Promise<void> {
    for (const part of chunkMessage(text, TELEGRAM_MAX_LENGTH)) {
      await telegramIntegration.send({ chatId, text: part });
    }
  },
};
