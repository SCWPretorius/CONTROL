import { z } from 'zod';
import { SkillModule, NormalizedEvent, ContextFile } from '../types/index.js';
import { saveContext } from '../memory/contextStore.js';
import { logger } from '../logging/logger.js';

const paramsSchema = z.object({
  path: z.string().describe('Relative path within contexts/ directory'),
  label: z.string().describe('Context label'),
  content: z.string().describe('Context content'),
  tags: z.array(z.string()).optional().describe('Context tags'),
  category: z.enum(['personal', 'conversation', 'temporary', 'event-log']).optional(),
});

export const definition = {
  name: 'updateContext',
  description: 'Update or create a context file in memory (admin only)',
  paramsSchema,
  tags: ['memory', 'context', 'admin'],
  minRole: 'admin' as const,
  requiresApproval: true,
  dailyLimit: 50,
  maxConcurrent: 1,
  priority: 'normal' as const,
  timeoutMs: 10000,
};

export async function execute(
  params: Record<string, unknown>,
  _event: NormalizedEvent
): Promise<{ updated: boolean; path: string }> {
  const parsed = paramsSchema.parse(params);
  const ctx: ContextFile = {
    label: parsed.label,
    category: parsed.category ?? 'personal',
    priority: 'normal',
    lastUpdated: new Date().toISOString(),
    tags: parsed.tags ?? [],
    content: parsed.content,
    version: 1,
  };
  saveContext(parsed.path, ctx);
  logger.info({ path: parsed.path }, '[SKILL:updateContext] Context updated');
  return { updated: true, path: parsed.path };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
