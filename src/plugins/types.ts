import type { Tool } from '../agents/tool-registry.js';
import type { NormalizedEvent } from '../types/index.js';
import type { AgentRunOptions } from '../agents/types.js';
import type { Manifest } from './manifest.js';

export interface PluginModule {
  tools?: Tool[];
  onStartup?: () => Promise<void>;
  onEvent?: (event: NormalizedEvent) => Promise<void>;
  onAgentRun?: (options: AgentRunOptions) => Promise<void>;
  onToolCall?: (toolName: string, params: Record<string, unknown>) => Promise<void>;
}

export interface LoadedPlugin {
  manifest: Manifest;
  dir: string;
  module: PluginModule;
}
