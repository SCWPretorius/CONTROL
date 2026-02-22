import { z } from 'zod';
import { SkillModule, NormalizedEvent } from '../types/index.js';
import { logger } from '../logging/logger.js';

const paramsSchema = z.object({
  target: z.string().describe('Target recipient or channel'),
  text: z.string().describe('Message text to send'),
  mention: z.boolean().optional().describe('Whether to mention the user'),
});

export const definition = {
  name: 'sendMessage',
  description: 'Send a message via the source channel',
  paramsSchema,
  tags: ['communication', 'outbound'],
  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 100,
  maxConcurrent: 1,
  priority: 'high' as const,
  timeoutMs: 10000,
};

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{ sent: boolean; target: string; text: string }> {
  const parsed = paramsSchema.parse(params);
  logger.info(
    { target: parsed.target, source: event.source },
    '[SKILL:sendMessage] Sending message'
  );
  return { sent: true, target: parsed.target, text: parsed.text };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
