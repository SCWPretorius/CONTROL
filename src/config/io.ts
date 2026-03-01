import { config } from './config.js';
import { logger } from '../logging/logger.js';

/** Log warnings for missing or suspicious configuration values at startup. */
export function validateConfig(): void {
  if (!config.lmstudio.primaryModel || config.lmstudio.primaryModel === 'local-model') {
    logger.warn('[CONFIG] LLM_PRIMARY_MODEL is not set — using default "local-model"');
  }

  const hasIntegration =
    config.telegram.botToken || config.discord.botToken;
  if (!hasIntegration) {
    logger.warn('[CONFIG] No integrations configured (missing TELEGRAM_BOT_TOKEN or DISCORD_BOT_TOKEN)');
  }

  if (!config.encryptionKey) {
    logger.warn('[CONFIG] ENCRYPTION_KEY not set — encrypted secrets will not work');
  }

  // Freeze the top-level config object so mutations are caught at runtime.
  // TypeScript already enforces readonly types; this catches JS-layer mutations.
  Object.freeze(config);

  logger.info('[CONFIG] Startup validation complete');
}
