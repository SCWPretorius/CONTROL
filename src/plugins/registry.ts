import type { LoadedPlugin } from './types.js';
import { logger } from '../logging/logger.js';

class PluginRegistry {
  private plugins: LoadedPlugin[] = [];

  register(plugin: LoadedPlugin): void {
    this.plugins.push(plugin);
    logger.info(
      { name: plugin.manifest.name, version: plugin.manifest.version },
      '[PLUGINS] Plugin registered',
    );
  }

  getAll(): LoadedPlugin[] {
    return [...this.plugins];
  }

  get(name: string): LoadedPlugin | undefined {
    return this.plugins.find(p => p.manifest.name === name);
  }

  getToolCount(): number {
    return this.plugins.reduce((sum, p) => sum + (p.module.tools?.length ?? 0), 0);
  }
}

export const pluginRegistry = new PluginRegistry();
