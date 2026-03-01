import { logger } from '../logging/logger.js';

interface DeprecatedVar {
  deprecated: string;
  replacement: string;
  since: string;
}

const DEPRECATED_VARS: DeprecatedVar[] = [
  { deprecated: 'OLLAMA_URL', replacement: 'LMSTUDIO_URL', since: '0.1.0' },
];

/**
 * Warn about deprecated environment variables and auto-migrate where safe.
 * Must be called before config.ts is imported (e.g., at the very top of entry.ts).
 * config.ts already handles OLLAMA_URL as a fallback, so this is primarily for user visibility.
 */
export function migrateConfig(): void {
  for (const { deprecated, replacement, since } of DEPRECATED_VARS) {
    if (process.env[deprecated]) {
      if (!process.env[replacement]) {
        process.env[replacement] = process.env[deprecated];
        logger.warn(
          `[CONFIG] ${deprecated} is deprecated since v${since} — use ${replacement} instead (auto-migrated)`,
        );
      } else {
        logger.debug(
          `[CONFIG] ${deprecated} is deprecated since v${since} — ignoring in favour of ${replacement}`,
        );
      }
    }
  }
}
