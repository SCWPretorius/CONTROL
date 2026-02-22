import { watch } from 'chokidar';
import { mkdirSync } from 'fs';
import { config } from '../config/config.js';
import { skillRegistry } from './skillRegistry.js';
import { SkillModule } from '../types/index.js';
import { logger } from '../logging/logger.js';

function validateSkillModule(mod: unknown): mod is SkillModule {
  if (!mod || typeof mod !== 'object') return false;
  const m = mod as Record<string, unknown>;
  if (!m['definition'] || !m['execute']) return false;
  if (typeof m['execute'] !== 'function') return false;
  return true;
}

async function loadSkillFile(filePath: string): Promise<void> {
  try {
    const mod = await import(`${filePath}?v=${Date.now()}`);
    if (!validateSkillModule(mod)) {
      logger.warn({ filePath }, '[SKILL-LOADER] Invalid skill module structure');
      return;
    }
    skillRegistry.register(mod as SkillModule);
    logger.info({ filePath }, '[SKILL-LOADER] Skill loaded');
  } catch (err) {
    logger.error({ err, filePath }, '[SKILL-LOADER] Failed to load skill');
  }
}

export function startSkillLoader(): void {
  mkdirSync(config.skillsDir, { recursive: true });

  const watcher = watch(config.skillsDir, {
    persistent: true,
    ignoreInitial: false,
    awaitWriteFinish: { stabilityThreshold: 300 },
  });

  watcher.on('add', loadSkillFile);
  watcher.on('change', loadSkillFile);
  watcher.on('error', err => logger.error({ err }, '[SKILL-LOADER] Watcher error'));

  logger.info({ dir: config.skillsDir }, '[SKILL-LOADER] Watching for skills');
}
