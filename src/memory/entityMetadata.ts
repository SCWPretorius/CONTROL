import { logger } from '../logging/logger.js';

export interface EntityMetadata {
  entityId: string;
  friendlyName: string;
  domain: string;
  deviceClass?: string; // e.g., "armed_disarmed", "motion", "temperature"
  capabilities?: string[]; // e.g., ["arm", "disarm", "query"]
  areaName?: string;
  relatedEntities?: string[]; // IDs of related entities (sensors linked to partition, etc.)
  state: string;
  lastUpdated?: string;
}

export interface EnrichedEntityContext {
  total: number;
  domainSummary: Record<string, number>;
  entities: EntityMetadata[];
}

/**
 * Enriches entity data with semantic metadata for better LLM reasoning
 * Infers device class and capabilities from:
 * - Entity domain (binary_sensor, switch, select, etc.)
 * - Entity ID patterns
 * - Friendly name patterns
 * - Loaded relationships from entity-relationships.json
 */
export async function enrichEntityMetadata(
  rawEntities: Array<{
    entity_id: string;
    state: string;
    attributes?: Record<string, unknown>;
  }>,
  entityRelationships?: Record<string, string[]>
): Promise<EnrichedEntityContext> {
  const enriched: EntityMetadata[] = [];
  const domainCounts: Record<string, number> = {};

  for (const entity of rawEntities) {
    const [domain, ...rest] = entity.entity_id.split('.');
    const friendlyName = (entity.attributes?.['friendly_name'] as string) || entity.entity_id;
    const areaName = entity.attributes?.['area_name'] as string | undefined;

    let deviceClass = (entity.attributes?.['device_class'] as string) || undefined;
    let capabilities: string[] = [];

    // Infer capabilities from domain and device class
    if (domain === 'binary_sensor') {
      capabilities = ['query'];
      if (!deviceClass) {
        // Guess device class from friendly name
        const nameLower = friendlyName.toLowerCase();
        if (nameLower.includes('motion')) deviceClass = 'motion';
        else if (nameLower.includes('armed') || nameLower.includes('disarm')) deviceClass = 'armed_disarmed';
        else if (nameLower.includes('window') || nameLower.includes('door')) deviceClass = 'door';
        else if (nameLower.includes('presence')) deviceClass = 'presence';
      }
    } else if (domain === 'switch' || domain === 'input_boolean') {
      capabilities = ['toggle', 'turn_on', 'turn_off'];
      // Switches with "armed" connotation might be partition control
      if ((friendlyName.toLowerCase().includes('armed') || friendlyName.toLowerCase().includes('disarm')) &&
          !deviceClass) {
        deviceClass = 'armed_disarmed';
      }
    } else if (domain === 'cover') {
      capabilities = ['open', 'close', 'set_position'];
      deviceClass = deviceClass || 'shutter';
    } else if (domain === 'light') {
      capabilities = ['turn_on', 'turn_off', 'set_brightness'];
      deviceClass = deviceClass || 'light';
    } else if (domain === 'select') {
      capabilities = ['query', 'select_option'];
    } else if (domain === 'number') {
      capabilities = ['query', 'set_value'];
    } else if (domain === 'sensor') {
      capabilities = ['query'];
      // Common sensor device classes already handled by HA
    } else {
      capabilities = ['query'];
    }

    // Get related entities from the relationships map
    const relatedEntities = entityRelationships?.[entity.entity_id] || [];

    enriched.push({
      entityId: entity.entity_id,
      friendlyName,
      domain,
      deviceClass,
      capabilities,
      areaName,
      relatedEntities,
      state: entity.state,
      lastUpdated: entity.attributes?.['last_updated'] as string | undefined,
    });

    domainCounts[domain] = (domainCounts[domain] || 0) + 1;
  }

  logger.debug(
    { totalEntities: enriched.length, domains: Object.keys(domainCounts).length },
    '[ENTITY_METADATA] Enrichment complete'
  );

  return {
    total: enriched.length,
    domainSummary: domainCounts,
    entities: enriched,
  };
}

/**
 * Calculates semantic match score for entity selection
 * Higher score = better match for user's query
 */
export function calculateMatchScore(
  query: string,
  entity: EntityMetadata
): { score: number; reasons: string[] } {
  const queryLower = query.toLowerCase();
  const reasons: string[] = [];
  let score = 0;

  // Exact phrase match in entity ID (highest priority)
  if (entity.entityId.toLowerCase().includes(queryLower)) {
    score += 30;
    reasons.push('exact match in entity ID');
  }

  // Exact phrase match in friendly name
  if (entity.friendlyName.toLowerCase().includes(queryLower)) {
    score += 25;
    reasons.push('exact match in friendly name');
  }

  // Word-by-word match in friendly name
  const queryWords = queryLower.split(/\s+/).filter(w => w.length > 0);
  const friendlyWords = entity.friendlyName.toLowerCase().split(/\s+/);
  const matchedWords = queryWords.filter(qw =>
    friendlyWords.some(fw => fw.includes(qw) || qw.includes(fw))
  );
  if (matchedWords.length > 0) {
    const wordScore = (matchedWords.length / queryWords.length) * 20;
    score += wordScore;
    reasons.push(`${matchedWords.length}/${queryWords.length} words matched`);
  }

  // Match in area name
  if (entity.areaName && entity.areaName.toLowerCase().includes(queryLower)) {
    score += 15;
    reasons.push('matched in area name');
  }

  // Bonus for semantic capability match
  // If query contains "armed", favor entities with armed_disarmed capability
  if (queryLower.includes('armed') && entity.deviceClass === 'armed_disarmed') {
    score += 20;
    reasons.push('semantic match: user asking about armed state');
  }
  if (queryLower.includes('motion') && entity.deviceClass === 'motion') {
    score += 20;
    reasons.push('semantic match: user asking about motion');
  }
  if (queryLower.includes('temperature') && entity.domain === 'sensor' && 
      entity.deviceClass?.includes('temperature')) {
    score += 20;
    reasons.push('semantic match: user asking about temperature');
  }

  // Penalty for low-confidence automation matches
  if (entity.domain === 'automation' && !queryLower.includes('automation')) {
    score -= 10;
    reasons.push('penalty: automation entity when not explicitly requested');
  }

  return { score: Math.max(0, score), reasons };
}

/**
 * Ranks entities by match quality and returns top N with confidence scores
 */
export function rankEntities(
  query: string,
  entities: EntityMetadata[],
  limit = 3
): Array<EntityMetadata & { score: number; confidence: number; reasons: string[] }> {
  const scored = entities.map(entity => {
    const { score, reasons } = calculateMatchScore(query, entity);
    return {
      ...entity,
      score,
      reasons,
    };
  });

  scored.sort((a, b) => b.score - a.score);
  const top = scored.slice(0, limit);

  // Calculate confidence percentage (0-100)
  const maxScore = top[0]?.score || 1;
  return top.map(e => ({
    ...e,
    confidence: Math.round((e.score / maxScore) * 100),
  }));
}
