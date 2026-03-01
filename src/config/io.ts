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

  logger.info('[CONFIG] Startup validation complete');
}
