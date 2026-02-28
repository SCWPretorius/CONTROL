import { getMessageQueue } from './messageQueue.js';
import { logger } from '../logging/logger.js';
import { NormalizedEvent } from '../types/index.js';

// These will be injected at startup
let handleEventForRetry: ((event: NormalizedEvent) => Promise<void>) | null = null;
let sendMessageDirectly: ((chatId: string, text: string, source: string) => Promise<void>) | null = null;

let workerInterval: NodeJS.Timeout | null = null;
let cleanupInterval: NodeJS.Timeout | null = null;
let isRunning = false;

export function setEventHandler(handler: (event: NormalizedEvent) => Promise<void>): void {
  handleEventForRetry = handler;
}

export function setMessageSender(
  sender: (chatId: string, text: string, source: string) => Promise<void>
): void {
  sendMessageDirectly = sender;
}

export function startQueueWorker(
  retryIntervalMs: number = 10000,
  cleanupIntervalMs: number = 3600000
): void {
  if (isRunning) {
    logger.warn('[QUEUE:WORKER] Worker already running');
    return;
  }

  isRunning = true;
  logger.info(
    { retryIntervalMs, cleanupIntervalMs },
    '[QUEUE:WORKER] Queue worker started'
  );

  // Process pending messages at regular intervals
  workerInterval = setInterval(async () => {
    await processPendingMessages();
  }, retryIntervalMs);

  // Clean up expired messages periodically
  cleanupInterval = setInterval(() => {
    cleanupExpiredMessages();
  }, cleanupIntervalMs);

  // Process any pending messages immediately on startup
  processPendingMessages().catch(err => {
    logger.error({ err }, '[QUEUE:WORKER] Error processing initial messages');
  });
}

export function stopQueueWorker(): void {
  if (workerInterval) {
    clearInterval(workerInterval);
    workerInterval = null;
  }
  if (cleanupInterval) {
    clearInterval(cleanupInterval);
    cleanupInterval = null;
  }
  isRunning = false;
  logger.info('[QUEUE:WORKER] Queue worker stopped');
}

async function processPendingMessages(): Promise<void> {
  const queue = getMessageQueue();

  try {
    const pending = await queue.getPendingMessages(10);

    if (pending.length === 0) {
      return; // Nothing to do
    }

    logger.debug({ count: pending.length }, '[QUEUE:WORKER] Processing pending messages');

    for (const message of pending) {
      try {
        await queue.markProcessing(message.id);

        if (message.type === 'incoming') {
          await processIncomingMessage(message);
        } else if (message.type === 'outgoing') {
          await processOutgoingMessage(message);
        }

        await queue.markCompleted(message.id);
        logger.info({ messageId: message.id }, '[QUEUE:WORKER] Message processed successfully');
      } catch (err) {
        const error = err instanceof Error ? err.message : String(err);
        const retryCount = await queue.incrementRetry(message.id);

        if (retryCount >= message.maxRetries) {
          await queue.markFailed(message.id, `Max retries exceeded: ${error}`);
          logger.error(
            { messageId: message.id, error, retryCount },
            '[QUEUE:WORKER] Message failed after max retries'
          );
        } else {
          logger.warn(
            { messageId: message.id, error, retryCount, maxRetries: message.maxRetries },
            '[QUEUE:WORKER] Message processing failed, will retry'
          );
        }
      }
    }
  } catch (err) {
    logger.error({ err }, '[QUEUE:WORKER] Error in processPendingMessages');
  }
}

async function processIncomingMessage(message: any): Promise<void> {
  if (!handleEventForRetry) {
    throw new Error('Event handler not set for queue worker');
  }

  if (!message.event) {
    throw new Error('Incoming message missing event data');
  }

  logger.debug(
    { messageId: message.id, source: message.source },
    '[QUEUE:WORKER] Reprocessing incoming message'
  );

  // Reconstruct the event and reprocess it
  const event = message.event as any;
  await handleEventForRetry(event);
}

async function processOutgoingMessage(message: any): Promise<void> {
  if (!sendMessageDirectly) {
    throw new Error('Message sender not set for queue worker');
  }

  if (!message.targetChannel) {
    throw new Error('Outgoing message missing target channel');
  }

  const text = (message.payload.text as string) || '(no message text)';

  logger.debug(
    { messageId: message.id, target: message.targetChannel, source: message.source },
    '[QUEUE:WORKER] Reprocessing outgoing message'
  );

  // Resend the message via the integration
  await sendMessageDirectly(message.targetChannel, text, message.source);
}

function cleanupExpiredMessages(): void {
  const queue = getMessageQueue();

  try {
    queue.deleteExpiredMessages().then(deletedCount => {
      if (deletedCount > 0) {
        logger.info({ deletedCount }, '[QUEUE:WORKER] Cleaned up expired messages');
      }
    }).catch(err => {
      logger.error({ err }, '[QUEUE:WORKER] Error cleaning up expired messages');
    });
  } catch (err) {
    logger.error({ err }, '[QUEUE:WORKER] Error cleaning up expired messages');
  }
}

export async function getQueueWorkerStats(): Promise<{
  isRunning: boolean;
  stats: { total: number; pending: number; processing: number; completed: number; failed: number };
}> {
  const queue = getMessageQueue();
  return {
    isRunning,
    stats: await queue.getStats(),
  };
}
