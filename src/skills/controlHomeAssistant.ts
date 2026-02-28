import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import homeAssistantIntegration from '../integrations/homeAssistant.js';

const paramsSchema = z.object({
  domain: z
    .string()
    .describe('Service domain (e.g., "light", "climate", "switch", "scene")'),
  service: z
    .string()
    .describe('Service name (e.g., "turn_on", "turn_off", "set_temperature", "activate")'),
  entityId: z
    .union([z.string(), z.array(z.string())])
    .optional()
    .describe('Entity ID or array of IDs to control (e.g., "light.living_room")'),
  data: z
    .unknown()
    .optional()
    .describe('Additional service data (e.g., {"brightness": 255, "color_temp": 370})'),
});

export const definition = {
  name: 'controlHomeAssistant',
  description:
    'Control Home Assistant devices and services (turn lights on/off, set temperature, activate scenes, etc.)',
  paramsSchema,
  tags: ['home-automation', 'home-assistant', 'control', 'action'],
  minRole: 'admin' as const,
  requiresApproval: true,
  dailyLimit: 50,
  maxConcurrent: 3,
  priority: 'high' as const,
  timeoutMs: 15000,
};

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{
  success: boolean;
  domain: string;
  service: string;
  affectedEntities: number;
  result: unknown;
}> {
  const parsed = paramsSchema.parse(params);

  // Build service data
  const serviceData: Record<string, unknown> = (parsed.data as Record<string, unknown> | undefined) ?? {};

  // Add entity_id(s) if provided
  if (parsed.entityId) {
    const entityIds = Array.isArray(parsed.entityId) ? parsed.entityId : [parsed.entityId];
    serviceData.entity_id = entityIds.length === 1 ? entityIds[0] : entityIds;
  }

  try {
    logger.info(
      {
        domain: parsed.domain,
        service: parsed.service,
        entityId: parsed.entityId,
      },
      '[SKILL:controlHomeAssistant] Executing service'
    );

    const result = await homeAssistantIntegration.callService(
      parsed.domain,
      parsed.service,
      serviceData
    );

    const affectedEntities = Array.isArray(result) ? result.length : 1;
    logger.info(
      {
        domain: parsed.domain,
        service: parsed.service,
        affectedEntities,
      },
      '[SKILL:controlHomeAssistant] Service executed successfully'
    );

    return {
      success: true,
      domain: parsed.domain,
      service: parsed.service,
      affectedEntities,
      result,
    };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    logger.error(
      { err, domain: parsed.domain, service: parsed.service },
      '[SKILL:controlHomeAssistant] Service execution failed'
    );
    throw new SkillExecutionError(
      `Failed to control Home Assistant: ${message}`,
      true
    );
  }
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
