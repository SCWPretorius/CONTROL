import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import homeAssistantIntegration from '../integrations/homeAssistant.js';
import { enrichEntityMetadata, rankEntities } from '../memory/entityMetadata.js';
import { getCachedEntitySelection, cacheEntitySelection } from '../memory/entitySelectionCache.js';
import { readFileSync, existsSync } from 'fs';
import { resolve } from 'path';

const paramsSchema = z.object({
  query: z.string().describe('Search query - device name, friendly name, or partial entity ID (e.g., "driveway motion", "beams", "sensor.temperature")'),
  domain: z.string().optional().describe('Optional: filter by domain (e.g., "sensor", "binary_sensor", "light", "switch")'),
});

export const definition = {
  name: 'findHomeAssistantEntity',
  description: 'Find Home Assistant entities by friendly name or query. Returns top 3 matches with confidence scores. Use this to find the correct entity ID when you don\'t know it exactly.',
  paramsSchema,
  tags: ['home-automation', 'home-assistant', 'search', 'discovery'],
  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 100,
  maxConcurrent: 5,
  priority: 'normal' as const,
  timeoutMs: 10000,
};

function loadEntityRelationships(): Record<string, string[]> {
  try {
    const path = resolve(process.cwd(), 'data/memory/contexts/entity-relationships.json');
    if (existsSync(path)) {
      const data = JSON.parse(readFileSync(path, 'utf-8'));
      // Extract the relationships object from the content if it's in JSON format
      if (data.content && typeof data.content === 'string') {
        const match = data.content.match(/CURRENT RELATIONSHIPS:\s*(\{[\s\S]*?\})/);
        if (match) {
          try {
            return JSON.parse(match[1]);
          } catch {
            return {};
          }
        }
      }
      return {};
    }
  } catch (err) {
    logger.debug({ err }, '[ENTITY_SEARCH] Failed to load relationships');
  }
  return {};
}

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{
  matches: Array<{ 
    entityId: string; 
    friendlyName: string; 
    domain: string; 
    state: string;
    deviceClass?: string;
    capabilities?: string[];
    confidence?: number;
  }>;
  count: number;
  totalEntities?: number;
  cachedResult?: boolean;
  suggestion?: string;
  isAmbiguous?: boolean;
}> {
  const parsed = paramsSchema.parse(params);
  const searchQuery = parsed.query.toLowerCase();
  const filterDomain = parsed.domain?.toLowerCase();

  logger.info(
    { query: searchQuery, domain: filterDomain },
    '[SKILL:findHomeAssistantEntity] Searching entities'
  );

  // Check cache first
  const cached = getCachedEntitySelection(searchQuery);
  if (cached && !filterDomain) {
    logger.info(
      { query: searchQuery, cachedEntityId: cached.selectedEntityId },
      '[SKILL:findHomeAssistantEntity] Returning cached result'
    );
    return {
      matches: [
        {
          entityId: cached.selectedEntityId,
          friendlyName: cached.selectedFriendlyName,
          domain: cached.selectedDomain,
          state: 'unknown',
          confidence: cached.confidence,
        },
      ],
      count: 1,
      cachedResult: true,
    };
  }

  const rawEntities = await homeAssistantIntegration.getAvailableEntities();
  if (!rawEntities) {
    throw new SkillExecutionError('Failed to retrieve entities from Home Assistant', true);
  }

  logger.debug(
    { totalEntities: rawEntities.length },
    '[SKILL:findHomeAssistantEntity] Total entities available'
  );

  // Load relationships and enrich metadata
  const relationships = loadEntityRelationships();
  const enriched = await enrichEntityMetadata(rawEntities, relationships);
  
  // Filter entities based on domain if specified
  let candidateEntities = enriched.entities;
  if (filterDomain) {
    candidateEntities = candidateEntities.filter(e => e.domain === filterDomain);
  }

  // Rank entities by semantic match
  const ranked = rankEntities(searchQuery, candidateEntities, 10); // Get top 10 first, then return top 3
  const topMatches = ranked.slice(0, 3); // Return top 3 with confidence

  const matches = topMatches.map(match => ({
    entityId: match.entityId,
    friendlyName: match.friendlyName,
    domain: match.domain,
    state: match.state,
    deviceClass: match.deviceClass,
    capabilities: match.capabilities,
    confidence: match.confidence,
  }));

  logger.info(
    { query: searchQuery, matches: matches.length, totalEntities: enriched.total },
    '[SKILL:findHomeAssistantEntity] Search completed'
  );

  // Determine if results are ambiguous
  const isAmbiguous = matches.length > 1 && 
    (matches[0].confidence - (matches[1].confidence || 0) < 15);

  let suggestion: string | undefined;
  if (matches.length === 0) {
    if (filterDomain) {
      suggestion = `No matches found in domain "${filterDomain}". Try searching without the domain filter.`;
    } else {
      suggestion = `No matches found. Try simpler search terms, or use listHomeAssistantEntities to browse all entities.`;
    }
  } else if (matches.length === 1) {
    suggestion = `Found 1 entity. Confidence: ${matches[0].confidence}%`;
  } else if (isAmbiguous) {
    suggestion = topMatches.slice(0, 2).map((m, i) => 
      `(${i + 1}) ${m.friendlyName} [${m.confidence}%]`
    ).join(', ');
  }

  return {
    matches,
    count: matches.length,
    totalEntities: enriched.total,
    cachedResult: false,
    suggestion,
    isAmbiguous,
  };
}


const skillModule: SkillModule = { definition, execute };
export default skillModule;
