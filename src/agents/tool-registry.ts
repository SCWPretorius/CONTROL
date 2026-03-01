import { skillRegistry } from '../skills/skillRegistry.js';
import type { SkillModule, NormalizedEvent } from '../types/index.js';

export interface Tool {
  name: string;
  description: string;
  paramsSchema: unknown;
  execute: (params: Record<string, unknown>, event: NormalizedEvent) => Promise<unknown>;
}

class ToolRegistry {
  private extras = new Map<string, Tool>();

  getAll(): Tool[] {
    const skillTools = skillRegistry.getActive().map(m => this.skillToTool(m));
    return [...skillTools, ...Array.from(this.extras.values())];
  }

  get(name: string): Tool | undefined {
    const skill = skillRegistry.get(name);
    if (skill) return this.skillToTool(skill);
    return this.extras.get(name);
  }

  register(tool: Tool): void {
    this.extras.set(tool.name, tool);
  }

  private skillToTool(module: SkillModule): Tool {
    return {
      name: module.definition.name,
      description: module.definition.description,
      paramsSchema: module.definition.paramsSchema,
      execute: module.execute,
    };
  }
}

export const toolRegistry = new ToolRegistry();
