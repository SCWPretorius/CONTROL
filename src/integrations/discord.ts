import { createVerify } from 'crypto';
import { NormalizedEvent } from '../types/index.js';
import { v4 as uuidv4 } from 'uuid';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

let eventCallback: ((event: NormalizedEvent) => void) | null = null;

export function validateDiscordSignature(
  body: string,
  signature: string,
  timestamp: string
): boolean {
  if (!config.discord.publicKey) return true;
  try {
    const verify = createVerify('ed25519');
    verify.update(timestamp + body);
    return verify.verify(
      Buffer.from(config.discord.publicKey, 'hex'),
      Buffer.from(signature, 'hex')
    );
  } catch {
    return false;
  }
}

const discordIntegration = {
  name: 'discord',

  async poll(): Promise<NormalizedEvent[] | null> {
    return null;
  },

  async send(payload: Record<string, unknown>): Promise<void> {
    if (!config.discord.botToken) return;
    const { channelId, content } = payload as { channelId: string; content: string };
    await fetch(`https://discord.com/api/v10/channels/${channelId}/messages`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bot ${config.discord.botToken}`,
      },
      body: JSON.stringify({ content }),
    });
  },

  onEvent(callback: (event: NormalizedEvent) => void): void {
    eventCallback = callback;
  },

  handleWebhook(body: string, signature: string, timestamp: string): NormalizedEvent | null {
    if (!validateDiscordSignature(body, signature, timestamp)) {
      logger.warn('[DISCORD] Invalid webhook signature');
      return null;
    }
    try {
      const interaction = JSON.parse(body) as Record<string, unknown>;
      const event: NormalizedEvent = {
        id: uuidv4(),
        traceId: uuidv4(),
        source: 'discord',
        type: (interaction['type'] as string) ?? 'interaction',
        payload: { interaction },
        timestamp: new Date().toISOString(),
        userId: (interaction['member'] as Record<string, unknown>)?.['user']?.toString(),
        role: 'user',
      };
      if (eventCallback) eventCallback(event);
      return event;
    } catch (err) {
      logger.error({ err }, '[DISCORD] Failed to parse webhook');
      return null;
    }
  },
};

export default discordIntegration;
