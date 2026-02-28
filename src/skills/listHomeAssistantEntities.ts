import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import homeAssistantIntegration from '../integrations/homeAssistant.js';

const paramsSchema = z.object({
  domain: z
    .string()
    .optional()
    .describe('Filter by domain (e.g., "light", "sensor", "switch", "climate")'),
  limit: z
    .number()
    .optional()
    .describe('Maximum number of entities to return (default: 100)'),
});

interface EntityGroup {
  domain: string;
  count: number;
  entities: Array<{
    entityId: string;
    state: string;
    friendlyName?: string;
  }>;
}

export const definition = {
  name: 'listHomeAssistantEntities',
  description:
    'List available Home Assistant entities (devices, sensors, lights, etc.). Can filter by domain.',
  paramsSchema,
  tags: ['home-automation', 'home-assistant', 'discovery', 'list'],
  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 50,
  maxConcurrent: 3,
  priority: 'normal' as const,
  timeoutMs: 10000,
};

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{
  total: number;
  groups: EntityGroup[];
  domains: string[];
}> {
  const parsed = paramsSchema.parse(params);
  const limit = parsed.limit ?? 100;

  try {
    logger.info(
      { domain: parsed.domain, limit },
      '[SKILL:listHomeAssistantEntities] Fetching entities'
    );

    const entities = await homeAssistantIntegration.getAvailableEntities();
    if (!entities) {
      throw new SkillExecutionError(
        'Failed to retrieve entities from Home Assistant',
        true
      );
    }

    // Filter by domain if specified
    let filtered = entities;
    if (parsed.domain) {
      const domainPrefix = `${parsed.domain}.`;
      filtered = entities.filter(e => e.entity_id.startsWith(domainPrefix));
    }

    // Group by domain
    const groupMap = new Map<string, Array<(typeof entities)[0]>>();
    for (const entity of filtered) {
      const domain = entity.entity_id.split('.')[0] ?? 'unknown';
      if (!groupMap.has(domain)) {
        groupMap.set(domain, []);
      }
      groupMap.get(domain)!.push(entity);
    }

    // Build groups with limit
    const groups: EntityGroup[] = [];
    let totalIncluded = 0;

    for (const [domain, domainEntities] of Array.from(groupMap.entries()).sort()) {
      const entitiesToInclude = domainEntities.slice(0, Math.max(0, limit - totalIncluded));
      totalIncluded += entitiesToInclude.length;

      groups.push({
        domain,
        count: domainEntities.length,
        entities: entitiesToInclude.map(e => ({
          entityId: e.entity_id,
          state: e.state,
          friendlyName: (e.attributes['friendly_name'] as string | undefined) ?? e.entity_id,
        })),
      });

      if (totalIncluded >= limit) break;
    }

    const domains = groups.map(g => g.domain);

    logger.info(
      {
        totalEntities: entities.length,
        filteredCount: filtered.length,
        groupCount: groups.length,
        domains,
      },
      '[SKILL:listHomeAssistantEntities] Entities retrieved'
    );

    return {
      total: filtered.length,
      groups,
      domains,
    };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    logger.error(
      { err },
      '[SKILL:listHomeAssistantEntities] Failed to list entities'
    );
    throw new SkillExecutionError(
      `Failed to list Home Assistant entities: ${message}`,
      true
    );
  }
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
