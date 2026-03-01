import { runAgent } from '../agents/agent-runner.js';
import { channelRegistry } from './registry.js';
import { getOrCreateSession } from '../session/store.js';
import { logger } from '../logging/logger.js';
import type { NormalizedEvent } from '../types/index.js';

export async function routeEvent(event: NormalizedEvent): Promise<void> {
  if (event.type !== 'message') {
    logger.debug({ eventId: event.id, type: event.type }, '[ROUTER] Skipping non-message event');
    return;
  }

  const text = event.payload['text'] as string | undefined;
  if (!text?.trim()) {
    logger.debug({ eventId: event.id }, '[ROUTER] Skipping empty text event');
    return;
  }

  const channel = channelRegistry.get(event.source);
  if (!channel) {
    logger.warn({ source: event.source }, '[ROUTER] No channel registered for source — event dropped');
    return;
  }

  const chatId = event.payload['chatId'] as string | undefined;
  if (!chatId) {
    logger.warn({ eventId: event.id }, '[ROUTER] No chatId in event payload — event dropped');
    return;
  }

  const userId = event.userId ?? 'unknown';
  const session = getOrCreateSession(event.source, userId);

  logger.info(
    { eventId: event.id, sessionId: session.id, source: event.source },
    '[ROUTER] Routing event to agent',
  );

  try {
    let sendChain = Promise.resolve();

    await runAgent(
      {
        wsSessionId: session.id,
        userId,
        role: event.role ?? 'user',
        source: event.source,
        message: text,
      },
      (block) => {
        if (block.type === 'text' && block.content) {
          // Chain sends to preserve message order
          sendChain = sendChain.then(() =>
            channel.send(chatId, block.content).catch((err) =>
              logger.error({ err, chatId, source: event.source }, '[ROUTER] Failed to send text block'),
            ),
          );
        }
      },
    );

    // Await any remaining sends before returning
    await sendChain;
  } catch (err) {
    logger.error({ err, eventId: event.id }, '[ROUTER] Agent run failed');
    channel.send(chatId, '❌ An error occurred. Please try again.').catch(() => undefined);
  }
}
