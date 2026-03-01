import { readFileSync, writeFileSync } from 'fs';
import { existsSync } from 'fs';
import { logger } from '../logging/logger.js';
import { resolve } from 'path';

export interface CachedEntitySelection {
  selectedEntityId: string;
  selectedFriendlyName: string;
  selectedDomain: string;
  timestamp: string;
  confidence: number;
  userQuery: string; // The original user query that led to this selection
  relatedQueries: string[]; // Similar queries that could use this cached result
}

interface EntityCacheFile {
  version: 1;
  lastUpdated: string;
  cache: Record<string, CachedEntitySelection>;
}

const CACHE_FILE = resolve(process.cwd(), 'data/memory/contexts/entity-cache.json');
let cacheData: EntityCacheFile | null = null;

function loadCache(): EntityCacheFile {
  if (cacheData) return cacheData;

  if (existsSync(CACHE_FILE)) {
    try {
      const raw = readFileSync(CACHE_FILE, 'utf-8');
      const loaded = JSON.parse(raw) as EntityCacheFile;
      cacheData = loaded;
      logger.debug({ entries: Object.keys(cacheData.cache).length }, '[ENTITY_CACHE] Loaded from disk');
      return cacheData;
    } catch (err) {
      logger.warn({ err }, '[ENTITY_CACHE] Failed to load, starting fresh');
    }
  }

  cacheData = {
    version: 1,
    lastUpdated: new Date().toISOString(),
    cache: {},
  };
  return cacheData;
}

function saveCache(): void {
  if (!cacheData) return;
  try {
    cacheData.lastUpdated = new Date().toISOString();
    writeFileSync(CACHE_FILE, JSON.stringify(cacheData, null, 2));
    logger.debug({ entries: Object.keys(cacheData.cache).length }, '[ENTITY_CACHE] Saved to disk');
  } catch (err) {
    logger.error({ err }, '[ENTITY_CACHE] Failed to save');
  }
}

/**
 * Normalizes a query string for cache lookup
 * Removes common words, converts to lowercase, trims whitespace
 */
function normalizeQuery(query: string): string {
  return query
    .toLowerCase()
    .trim()
    .replace(/\s+/g, ' '); // Normalize whitespace
}

/**
 * Check if a query is semantically similar to a cached query
 * Returns true if overlap is > 60%
 */
function isSimilarQuery(query: string, cachedQuery: string): boolean {
  const q1 = normalizeQuery(query).split(/\s+/);
  const q2 = normalizeQuery(cachedQuery).split(/\s+/);
  
  const commonWords = q1.filter(w => q2.includes(w) && w.length > 2);
  const totalUnique = new Set([...q1, ...q2]).size;
  
  if (totalUnique === 0) return false;
  const similarity = commonWords.length / totalUnique;
  return similarity > 0.6;
}

/**
 * Get a cached entity selection, or return null if not found
 * Also checks for similar queries
 */
export function getCachedEntitySelection(userQuery: string): CachedEntitySelection | null {
  const cache = loadCache();
  const normalized = normalizeQuery(userQuery);

  // Exact match first
  if (cache.cache[normalized]) {
    const entry = cache.cache[normalized];
    logger.debug(
      { query: userQuery, entityId: entry.selectedEntityId },
      '[ENTITY_CACHE] Cache hit (exact)'
    );
    return entry;
  }

  // Check for similar queries
  for (const [_, entry] of Object.entries(cache.cache)) {
    if (isSimilarQuery(userQuery, entry.userQuery)) {
      logger.debug(
        { query: userQuery, cachedQuery: entry.userQuery, entityId: entry.selectedEntityId },
        '[ENTITY_CACHE] Cache hit (similar)'
      );
      return entry;
    }
  }

  logger.debug({ query: userQuery }, '[ENTITY_CACHE] Cache miss');
  return null;
}

/**
 * Store a successful entity selection in the cache
 */
export function cacheEntitySelection(
  userQuery: string,
  selectedEntityId: string,
  selectedFriendlyName: string,
  selectedDomain: string,
  confidence: number
): void {
  const cache = loadCache();
  const normalized = normalizeQuery(userQuery);

  cache.cache[normalized] = {
    userQuery,
    selectedEntityId,
    selectedFriendlyName,
    selectedDomain,
    confidence,
    timestamp: new Date().toISOString(),
    relatedQueries: [], // Could build this from similar queries
  };

  saveCache();
  logger.info(
    { query: userQuery, entityId: selectedEntityId },
    '[ENTITY_CACHE] Cached entity selection'
  );
}

/**
 * Clear the cache (useful for testing)
 */
export function clearEntityCache(): void {
  cacheData = {
    version: 1,
    lastUpdated: new Date().toISOString(),
    cache: {},
  };
  saveCache();
  logger.info('[ENTITY_CACHE] Cache cleared');
}

/**
 * Get cache statistics
 */
export function getEntityCacheStats(): { total: number; entries: Array<{ query: string; entityId: string }> } {
  const cache = loadCache();
  const entries = Object.values(cache.cache).map(e => ({
    query: e.userQuery,
    entityId: e.selectedEntityId,
  }));
  return {
    total: entries.length,
    entries,
  };
}
