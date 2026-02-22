import { NormalizedEvent, IntegrationModule } from '../types/index.js';
import { eventBus } from '../events/eventBus.js';
import { checkRateLimit } from '../concurrency/rateLimiter.js';
import { logger } from '../logging/logger.js';
import { config } from '../config/config.js';

interface IntegrationHealth {
  name: string;
  consecutiveErrors: number;
  quarantined: boolean;
  lastPollAt: Date | null;
  lastError: string | null;
  backoffMs: number;
  nextPollAt: Date | null;
}

const healthMap = new Map<string, IntegrationHealth>();

function getHealth(name: string): IntegrationHealth {
  if (!healthMap.has(name)) {
    healthMap.set(name, {
      name,
      consecutiveErrors: 0,
      quarantined: false,
      lastPollAt: null,
      lastError: null,
      backoffMs: 1000,
      nextPollAt: null,
    });
  }
  return healthMap.get(name)!;
}

export function getIntegrationHealth(): IntegrationHealth[] {
  return Array.from(healthMap.values());
}

export async function pollIntegration(
  integration: IntegrationModule
): Promise<NormalizedEvent[] | null> {
  const health = getHealth(integration.name);

  if (health.quarantined) {
    logger.debug({ name: integration.name }, '[INTEGRATION] Quarantined, skipping');
    return null;
  }

  const rateCheck = checkRateLimit(integration.name);
  if (!rateCheck.allowed) {
    logger.debug({ name: integration.name, reason: rateCheck.reason }, '[INTEGRATION] Rate limited');
    return null;
  }

  const pollTimeout = config.integrationFailure.pollTimeoutMs;

  try {
    const events = await Promise.race([
      integration.poll(),
      new Promise<null>((_, reject) =>
        setTimeout(() => reject(new Error('Poll timeout')), pollTimeout)
      ),
    ]);

    health.consecutiveErrors = 0;
    health.backoffMs = 1000;
    health.lastPollAt = new Date();
    health.nextPollAt = null;

    if (events) {
      for (const event of events) {
        eventBus.enqueue(event);
      }
    }

    return events;
  } catch (err) {
    const error = err instanceof Error ? err.message : String(err);
    health.consecutiveErrors++;
    health.lastError = error;
    logger.error({ err, name: integration.name, consecutiveErrors: health.consecutiveErrors }, '[INTEGRATION] Poll failed');

    if (health.consecutiveErrors >= config.integrationFailure.maxConsecutiveErrors) {
      health.quarantined = true;
      logger.error({ name: integration.name }, '[INTEGRATION] Quarantined due to repeated failures');
      scheduleRecovery(integration, health);
    } else {
      health.backoffMs = Math.min(health.backoffMs * 2, config.integrationFailure.maxBackoffMs);
      health.nextPollAt = new Date(Date.now() + health.backoffMs);
    }

    return null;
  }
}

function scheduleRecovery(integration: IntegrationModule, health: IntegrationHealth): void {
  const delay = Math.min(health.backoffMs * 2, config.integrationFailure.maxBackoffMs);
  setTimeout(async () => {
    try {
      await integration.poll();
      health.quarantined = false;
      health.consecutiveErrors = 0;
      health.backoffMs = 1000;
      logger.info({ name: integration.name }, '[INTEGRATION] Recovered from quarantine');
    } catch {
      scheduleRecovery(integration, health);
    }
  }, delay);
}
