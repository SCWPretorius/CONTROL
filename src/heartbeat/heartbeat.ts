import { config } from '../config/config.js';
import { integrationRegistry } from '../integrations/integrationLoader.js';
import { pollIntegration } from '../integrations/integrationRunner.js';
import { logger } from '../logging/logger.js';

let heartbeatTimer: ReturnType<typeof setInterval> | null = null;

export function startHeartbeat(): void {
  if (heartbeatTimer) return;

  logger.info(
    { intervalMs: config.heartbeatIntervalMs },
    '[HEARTBEAT] Starting'
  );

  heartbeatTimer = setInterval(async () => {
    const integrations = integrationRegistry.getAll();
    if (integrations.length === 0) return;

    logger.debug({ count: integrations.length }, '[HEARTBEAT] Polling integrations');

    await Promise.allSettled(
      integrations.map(integration => pollIntegration(integration))
    );
  }, config.heartbeatIntervalMs);
}

export function stopHeartbeat(): void {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer);
    heartbeatTimer = null;
    logger.info('[HEARTBEAT] Stopped');
  }
}
