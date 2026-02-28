import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import homeAssistantIntegration, { HomeAssistantEntity } from '../integrations/homeAssistant.js';

const paramsSchema = z.object({
  entityIds: z
    .union([z.string(), z.array(z.string())])
    .describe('Entity ID or array of entity IDs to query (e.g., "light.living_room" or ["sensor.temperature", "light.bedroom"])'),
});

export const definition = {
  name: 'queryHomeAssistant',
  description: 'Query the current state of one or more Home Assistant entities (devices, lights, sensors, etc.)',
  paramsSchema,
  tags: ['home-automation', 'home-assistant', 'query', 'status'],
  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 100,
  maxConcurrent: 5,
  priority: 'normal' as const,
  timeoutMs: 10000,
};

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{
  results: Array<{ entityId: string; state: string; attributes: Record<string, unknown>; lastUpdated: string }>;
  count: number;
}> {
  const parsed = paramsSchema.parse(params);
  const entityIds = Array.isArray(parsed.entityIds) ? parsed.entityIds : [parsed.entityIds];

  if (entityIds.length === 0) {
    throw new SkillExecutionError('No entity IDs specified', false);
  }

  logger.info(
    { entityCount: entityIds.length, entities: entityIds.slice(0, 5) },
    '[SKILL:queryHomeAssistant] Querying entities'
  );

  const results = [];

  for (const entityId of entityIds) {
    try {
      const entity = await homeAssistantIntegration.queryEntity(entityId);
      if (entity) {
        results.push({
          entityId: entity.entity_id,
          state: entity.state,
          attributes: entity.attributes,
          lastUpdated: entity.last_updated,
        });
      } else {
        logger.warn({ entityId }, '[SKILL:queryHomeAssistant] Entity not found');
        results.push({
          entityId,
          state: 'unknown',
          attributes: {},
          lastUpdated: 'unknown',
        });
      }
    } catch (err) {
      logger.error({ err, entityId }, '[SKILL:queryHomeAssistant] Failed to query entity');
      results.push({
        entityId,
        state: 'error',
        attributes: { error: String(err) },
        lastUpdated: 'error',
      });
    }
  }

  return {
    results,
    count: results.length,
  };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
