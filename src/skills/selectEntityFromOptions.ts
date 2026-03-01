import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import { cacheEntitySelection } from '../memory/entitySelectionCache.js';
import { integrationRegistry } from '../integrations/integrationLoader.js';

const paramsSchema = z.object({
  candidates: z.array(
    z.object({
      entityId: z.string(),
      friendlyName: z.string(),
      domain: z.string(),
      deviceClass: z.string().optional(),
      capabilities: z.array(z.string()).optional(),
      confidence: z.number().optional(),
    })
  ).describe('List of 2-3 candidate entities the user should choose from'),
  userQuery: z.string().describe('The original query the user made'),
  reasoning: z.string().optional().describe('Explanation of why these candidates were selected'),
});

export const definition = {
  name: 'selectEntityFromOptions',
  description: 'Present multiple Home Assistant entity options to the user and ask them to pick one. User responds with the option number.',
  paramsSchema,
  tags: ['home-automation', 'home-assistant', 'disambiguation', 'user-interaction'],  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 50,
  maxConcurrent: 5,
  priority: 'high' as const,
  timeoutMs: 30000,
};

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{
  message: string;
  sent: boolean;
  candidates: Array<{
    option: number;
    entityId: string;
    friendlyName: string;
    domain: string;
    confidence?: number;
  }>;
}> {
  const parsed = paramsSchema.parse(params);
  const { candidates, userQuery, reasoning } = parsed;

  if (candidates.length === 0) {
    throw new SkillExecutionError('No candidate entities provided', false);
  }

  if (candidates.length === 1) {
    // Single candidate - cache it automatically
    const entity = candidates[0];
    cacheEntitySelection(
      userQuery,
      entity.entityId,
      entity.friendlyName,
      entity.domain,
      entity.confidence || 100
    );

    logger.info(
      { query: userQuery, entityId: entity.entityId },
      '[SKILL:selectEntityFromOptions] Single match auto-cached'
    );

    const singleOptionMsg = `Auto-selected entity: **${entity.friendlyName}** (${entity.entityId})`;
    
    // Send message via integration
    const target = (event.payload as any).chatId || event.userId;
    const integration = integrationRegistry.get(event.source);
    if (integration && integration.send && target) {
      await integration.send({ chatId: target, text: singleOptionMsg });
    }

    return {
      message: singleOptionMsg,
      sent: true,
      candidates: [
        {
          option: 1,
          entityId: entity.entityId,
          friendlyName: entity.friendlyName,
          domain: entity.domain,
          confidence: entity.confidence,
        },
      ],
    };
  }

  // Multiple candidates - ask user to pick
  const optionsList = candidates.map((candidate, idx) => ({
    option: idx + 1,
    entityId: candidate.entityId,
    friendlyName: candidate.friendlyName,
    domain: candidate.domain,
    confidence: candidate.confidence,
    details: candidate.deviceClass
      ? `[${candidate.domain}/${candidate.deviceClass}] ${(candidate.capabilities || []).join(', ')}`
      : `[${candidate.domain}]`,
  }));

  const optionsText = optionsList
    .map(
      opt =>
        `**(${opt.option})** ${opt.friendlyName} - ${opt.details}${
          opt.confidence ? ` [confidence: ${opt.confidence}%]` : ''
        }`
    )
    .join('\n');

  const reasoningText = reasoning ? `\n_${reasoning}_\n` : '';

  const message = `I found multiple entities matching **"${userQuery}"**. Which one did you mean?\n${reasoningText}\n${optionsText}\n\nReply with just the number (1, 2, or 3).`;

  // Send message via integration
  const target = (event.payload as any).chatId || event.userId;
  const integration = integrationRegistry.get(event.source);
  let sent = false;
  
  if (integration && integration.send && target) {
    try {
      await integration.send({ chatId: target, text: message });
      sent = true;
      logger.info(
        { query: userQuery, optionCount: optionsList.length, target },
        '[SKILL:selectEntityFromOptions] Options presented to user'
      );
    } catch (err) {
      logger.error({ err, target }, '[SKILL:selectEntityFromOptions] Failed to send message');
      throw new SkillExecutionError(`Failed to send entity selection message: ${err}`, true);
    }
  }

  return {
    message,
    sent,
    candidates: optionsList.map(opt => ({
      option: opt.option,
      entityId: opt.entityId,
      friendlyName: opt.friendlyName,
      domain: opt.domain,
      confidence: opt.confidence,
    })),
  };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
