import discordIntegration from '../integrations/discord.js';
import { chunkMessage, type Channel } from './channel-interface.js';

export const DISCORD_MAX_LENGTH = 2000;

export const discordChannel: Channel = {
  name: 'discord',
  capabilities: { maxMessageLength: DISCORD_MAX_LENGTH, supportsMarkdown: true },

  async send(chatId: string, text: string): Promise<void> {
    for (const part of chunkMessage(text, DISCORD_MAX_LENGTH)) {
      await discordIntegration.send({ channelId: chatId, content: part });
    }
  },
};
