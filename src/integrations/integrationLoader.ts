import { watch } from 'chokidar';
import { mkdirSync } from 'fs';
import { config } from '../config/config.js';
import { IntegrationModule } from '../types/index.js';
import { logger } from '../logging/logger.js';

class IntegrationRegistry {
  private integrations = new Map<string, IntegrationModule>();

  register(integration: IntegrationModule): void {
    this.integrations.set(integration.name, integration);
    logger.info({ name: integration.name }, '[INTEGRATION] Registered');
  }

  get(name: string): IntegrationModule | undefined {
    return this.integrations.get(name);
  }

  getAll(): IntegrationModule[] {
    return Array.from(this.integrations.values());
  }

  has(name: string): boolean {
    return this.integrations.has(name);
  }
}

export const integrationRegistry = new IntegrationRegistry();

async function loadIntegrationFile(filePath: string): Promise<void> {
  try {
    const mod = await import(`${filePath}?v=${Date.now()}`);
    if (mod.default && mod.default.name && typeof mod.default.poll === 'function') {
      integrationRegistry.register(mod.default as IntegrationModule);
    } else {
      logger.warn({ filePath }, '[INTEGRATION-LOADER] Invalid integration module');
    }
  } catch (err) {
    logger.error({ err, filePath }, '[INTEGRATION-LOADER] Failed to load');
  }
}

export function startIntegrationLoader(): void {
  mkdirSync(config.integrationsDir, { recursive: true });

  const watcher = watch(config.integrationsDir, {
    persistent: true,
    ignoreInitial: false,
    awaitWriteFinish: { stabilityThreshold: 300 },
  });

  watcher.on('add', loadIntegrationFile);
  watcher.on('change', loadIntegrationFile);
  watcher.on('error', err => logger.error({ err }, '[INTEGRATION-LOADER] Watcher error'));

  logger.info({ dir: config.integrationsDir }, '[INTEGRATION-LOADER] Watching for integrations');
}
