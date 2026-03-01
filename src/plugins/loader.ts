import { readdirSync, readFileSync, existsSync } from 'fs';
import { resolve, join, sep } from 'path';
import { pathToFileURL } from 'url';
import { ManifestSchema } from './manifest.js';
import type { PluginModule, LoadedPlugin } from './types.js';
import { pluginRegistry } from './registry.js';
import { hookRegistry } from './hooks.js';
import { toolRegistry } from '../agents/tool-registry.js';
import { logger } from '../logging/logger.js';

const WORKSPACE_ROOT = resolve(process.cwd());
const DEFAULT_PLUGINS_DIR = join(WORKSPACE_ROOT, 'extensions');

function isWithinWorkspace(filePath: string): boolean {
  const resolved = resolve(filePath);
  return resolved.startsWith(WORKSPACE_ROOT + sep) || resolved === WORKSPACE_ROOT;
}

export async function loadPlugins(pluginsDir: string = DEFAULT_PLUGINS_DIR): Promise<void> {
  if (!existsSync(pluginsDir)) {
    logger.debug({ pluginsDir }, '[PLUGINS] No extensions directory — skipping plugin load');
    return;
  }

  const entries = readdirSync(pluginsDir, { withFileTypes: true });

  for (const entry of entries) {
    if (!entry.isDirectory()) continue;

    const pluginDir = join(pluginsDir, entry.name);
    const manifestPath = join(pluginDir, 'manifest.json');

    if (!existsSync(manifestPath)) {
      logger.warn({ pluginDir }, '[PLUGINS] Directory missing manifest.json — skipping');
      continue;
    }

    try {
      const rawManifest: unknown = JSON.parse(readFileSync(manifestPath, 'utf-8'));
      const parseResult = ManifestSchema.safeParse(rawManifest);

      if (!parseResult.success) {
        logger.error(
          { pluginDir, issues: parseResult.error.issues },
          '[PLUGINS] Invalid manifest — skipping',
        );
        continue;
      }

      const manifest = parseResult.data;
      const entryPath = resolve(pluginDir, manifest.entry);

      // Workspace boundary enforcement
      if (!isWithinWorkspace(entryPath)) {
        logger.error(
          { entryPath },
          '[PLUGINS] Entry path escapes workspace root — skipping (security)',
        );
        continue;
      }

      if (!existsSync(entryPath)) {
        logger.error({ entryPath }, '[PLUGINS] Entry file not found — skipping');
        continue;
      }

      // Dynamic import with cache-bust to support hot-reload in dev.
      // pathToFileURL() is required on Windows — bare C:\... paths are rejected by the ESM loader.
      const entryUrl = `${pathToFileURL(entryPath).href}?v=${Date.now()}`;
      const mod = await import(entryUrl);
      const pluginModule: PluginModule = (mod.plugin ?? mod.default ?? mod) as PluginModule;

      // Register tools with the agent tool registry
      if (Array.isArray(pluginModule.tools)) {
        for (const tool of pluginModule.tools) {
          toolRegistry.register(tool);
          logger.info({ toolName: tool.name, plugin: manifest.name }, '[PLUGINS] Tool registered');
        }
      }

      // Register lifecycle hooks
      if (typeof pluginModule.onStartup === 'function') {
        hookRegistry.register('startup', pluginModule.onStartup.bind(pluginModule));
      }
      if (typeof pluginModule.onEvent === 'function') {
        hookRegistry.register('event', (pluginModule.onEvent as (arg?: unknown) => Promise<void>).bind(pluginModule));
      }
      if (typeof pluginModule.onAgentRun === 'function') {
        hookRegistry.register('agent-run', (pluginModule.onAgentRun as (arg?: unknown) => Promise<void>).bind(pluginModule));
      }
      if (typeof pluginModule.onToolCall === 'function') {
        hookRegistry.register('tool-call', (pluginModule.onToolCall as (arg?: unknown) => Promise<void>).bind(pluginModule));
      }

      const loaded: LoadedPlugin = { manifest, dir: pluginDir, module: pluginModule };
      pluginRegistry.register(loaded);
    } catch (err) {
      logger.error({ err, pluginDir }, '[PLUGINS] Failed to load plugin');
    }
  }

  // Fire startup hooks for all loaded plugins
  await hookRegistry.emit('startup');

  logger.info({ count: pluginRegistry.getAll().length }, '[PLUGINS] Plugin loading complete');
}
