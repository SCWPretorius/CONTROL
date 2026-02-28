import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import homeAssistantIntegration from '../integrations/homeAssistant.js';

const paramsSchema = z.object({
  query: z.string().describe('Search query - device name, friendly name, or partial entity ID (e.g., "driveway motion", "beams", "sensor.temperature")'),
  domain: z.string().optional().describe('Optional: filter by domain (e.g., "sensor", "binary_sensor", "light", "switch")'),
});

export const definition = {
  name: 'findHomeAssistantEntity',
  description: 'Find Home Assistant entities by friendly name or query. Use this to find the correct entity ID when you don\'t know it exactly.',
  paramsSchema,
  tags: ['home-automation', 'home-assistant', 'search', 'discovery'],
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
  matches: Array<{ entityId: string; friendlyName: string; domain: string; state: string }>;
  count: number;
  totalEntities?: number;
  suggestion?: string;
}> {
  const parsed = paramsSchema.parse(params);
  const searchQuery = parsed.query.toLowerCase();
  const filterDomain = parsed.domain?.toLowerCase();

  logger.info(
    { query: searchQuery, domain: filterDomain },
    '[SKILL:findHomeAssistantEntity] Searching entities'
  );

  const entities = await homeAssistantIntegration.getAvailableEntities();
  if (!entities) {
    throw new SkillExecutionError('Failed to retrieve entities from Home Assistant', true);
  }

  logger.debug(
    { totalEntities: entities.length },
    '[SKILL:findHomeAssistantEntity] Total entities available'
  );

  const searchWords = searchQuery.split(/\s+/).filter(w => w.length > 0);

  const matches = entities
    .filter(entity => {
      // Domain filter if specified
      if (filterDomain) {
        const domain = entity.entity_id.split('.')[0];
        if (domain !== filterDomain) return false;
      }

      // Search in entity ID and friendly name
      const friendlyName = (entity.attributes?.['friendly_name'] as string || '').toLowerCase();
      const entityId = entity.entity_id.toLowerCase();
      const areaSuggestion = (entity.attributes?.['area_name'] as string || '').toLowerCase();
      const combinedText = `${entityId} ${friendlyName} ${areaSuggestion}`;

      return (
        // Exact phrase match
        combinedText.includes(searchQuery) ||
        // All words match (fuzzy)
        searchWords.every(word => combinedText.includes(word)) ||
        // Any word matches (partial)
        searchWords.some(word => word.length >= 3 && combinedText.includes(word))
      );
    })
    .slice(0, 10) // Limit to top 10 results
    .map(entity => ({
      entityId: entity.entity_id,
      friendlyName: (entity.attributes?.['friendly_name'] as string) || entity.entity_id,
      domain: entity.entity_id.split('.')[0] || 'unknown',
      state: entity.state,
    }));

  logger.info(
    { query: searchQuery, matches: matches.length, totalEntities: entities.length },
    '[SKILL:findHomeAssistantEntity] Search completed'
  );

  let suggestion: string | undefined;
  if (matches.length === 0) {
    if (filterDomain) {
      suggestion = `No matches found. Try searching without the domain filter "${filterDomain}" to see all entities.`;
    } else {
      suggestion = `No matches found. Try simpler search terms, or use listHomeAssistantEntities to browse all entities.`;
    }
  }

  return {
    matches,
    count: matches.length,
    totalEntities: entities.length,
    suggestion,
  };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
