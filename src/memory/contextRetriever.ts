import { ContextFile } from '../types/index.js';
import {
  getAllContexts,
  getContextsByTags,
  getHighPriorityContexts,
} from './contextStore.js';
import { searchVectors } from './vectorStore.js';
import { logger } from '../logging/logger.js';

export interface RetrievedContext {
  context: ContextFile;
  score: number;
  method: 'always' | 'tag' | 'keyword' | 'semantic';
}

export async function retrieveContexts(
  eventSource: string,
  eventType: string,
  keywords: string[],
  queryVector?: number[]
): Promise<RetrievedContext[]> {
  const results: RetrievedContext[] = [];
  const seen = new Set<string>();

  const high = getHighPriorityContexts();
  for (const ctx of high) {
    if (!seen.has(ctx.label)) {
      results.push({ context: ctx, score: 1.0, method: 'always' });
      seen.add(ctx.label);
    }
  }

  const tagMatches = getContextsByTags([eventSource, eventType]);
  for (const ctx of tagMatches) {
    if (!seen.has(ctx.label)) {
      results.push({ context: ctx, score: 0.8, method: 'tag' });
      seen.add(ctx.label);
    }
  }

  if (keywords.length > 0) {
    const all = getAllContexts();
    for (const ctx of all) {
      if (seen.has(ctx.label)) continue;
      const contentLower = ctx.content.toLowerCase();
      const tagMatch = ctx.tags.some(t => keywords.some(k => t.toLowerCase().includes(k.toLowerCase())));
      const contentMatch = keywords.some(k => contentLower.includes(k.toLowerCase()));
      if (tagMatch || contentMatch) {
        results.push({ context: ctx, score: 0.6, method: 'keyword' });
        seen.add(ctx.label);
      }
    }
  }

  if (queryVector && queryVector.length > 0) {
    const semanticResults = searchVectors(queryVector, 5);
    for (const sr of semanticResults) {
      if (!seen.has(sr.label)) {
        const all = getAllContexts();
        const ctx = all.find(c => c.label === sr.label);
        if (ctx) {
          results.push({ context: ctx, score: sr.score, method: 'semantic' });
          seen.add(ctx.label);
        }
      }
    }
  }

  logger.debug({ count: results.length }, '[CONTEXT] Retrieved contexts');
  return results;
}
