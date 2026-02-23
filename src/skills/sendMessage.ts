import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import { integrationRegistry } from '../integrations/integrationLoader.js';

const paramsSchema = z.object({
  target: z.string().optional().describe('Target recipient or channel (defaults to reply to sender)'),
  text: z.string().describe('Message text to send'),
  mention: z.boolean().optional().describe('Whether to mention the user'),
});

export const definition = {
  name: 'sendMessage',
  description: 'Send a message via the source channel (replies to the sender by default)',
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
  
  // Default to replying in the same chat
  const target = parsed.target || (event.payload as any).chatId || event.userId;
  
  if (!target) {
    throw new SkillExecutionError('No target specified and unable to determine chat ID from event', false);
  }
  
  // Get the integration for this event source
  const integration = integrationRegistry.get(event.source);
  if (!integration || !integration.send) {
    throw new SkillExecutionError(`Integration ${event.source} does not support sending messages`, false);
  }
  
  // Send the message via the integration
  await integration.send({
    chatId: target,
    text: parsed.text,
  });
  
  logger.info(
    { target, source: event.source, textLength: parsed.text.length },
    '[SKILL:sendMessage] Message sent'
  );
  
  return { sent: true, target, text: parsed.text };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
