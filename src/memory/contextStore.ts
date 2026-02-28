import {
  readFileSync,
  writeFileSync,
  existsSync,
  mkdirSync,
} from 'fs';
import { join, extname, dirname } from 'path';
import { config } from '../config/config.js';
import { ContextFile, ContextIndex } from '../types/index.js';
import { logger } from '../logging/logger.js';

const CONTEXTS_DIR = join(config.memoryDir, 'contexts');
const INDEX_FILE = join(config.memoryDir, 'index.json');

function ensureDirs(): void {
  mkdirSync(CONTEXTS_DIR, { recursive: true });
  mkdirSync(join(config.memoryDir, 'embeddings'), { recursive: true });
}

export function loadIndex(): ContextIndex {
  ensureDirs();
  if (!existsSync(INDEX_FILE)) {
    return { files: [] };
  }
  try {
    return JSON.parse(readFileSync(INDEX_FILE, 'utf-8')) as ContextIndex;
  } catch {
    return { files: [] };
  }
}

export function saveIndex(index: ContextIndex): void {
  ensureDirs();
  writeFileSync(INDEX_FILE, JSON.stringify(index, null, 2), 'utf-8');
}

export function loadContext(relativePath: string): ContextFile | null {
  const fullPath = join(CONTEXTS_DIR, relativePath);
  if (!existsSync(fullPath)) return null;
  try {
    const raw = readFileSync(fullPath, 'utf-8');
    if (extname(relativePath) === '.json') {
      return JSON.parse(raw) as ContextFile;
    }
    return {
      label: relativePath.replace(/\.[^.]+$/, ''),
      category: 'personal',
      priority: 'normal',
      lastUpdated: new Date().toISOString(),
      tags: [],
      content: raw,
    };
  } catch {
    return null;
  }
}

export function saveContext(relativePath: string, ctx: ContextFile): void {
  ensureDirs();
  const fullPath = join(CONTEXTS_DIR, relativePath);
  const dir = dirname(fullPath);
  mkdirSync(dir, { recursive: true });
  writeFileSync(fullPath, JSON.stringify(ctx, null, 2), 'utf-8');

  const index = loadIndex();
  const existing = index.files.findIndex(f => f.path === relativePath);
  const entry = {
    path: relativePath,
    label: ctx.label,
    category: ctx.category,
    priority: ctx.priority,
    tags: ctx.tags,
    lastUpdated: ctx.lastUpdated,
  };
  if (existing >= 0) {
    index.files[existing] = entry;
  } else {
    index.files.push(entry);
  }
  saveIndex(index);
}

export function getAllContexts(): ContextFile[] {
  const index = loadIndex();
  const contexts: ContextFile[] = [];
  for (const entry of index.files) {
    const ctx = loadContext(entry.path);
    if (ctx) contexts.push(ctx);
  }
  return contexts;
}

export function getContextsByTags(tags: string[]): ContextFile[] {
  const index = loadIndex();
  const matchingPaths = index.files
    .filter(f => tags.some(t => f.tags.includes(t)))
    .map(f => f.path);
  return matchingPaths.map(p => loadContext(p)).filter(Boolean) as ContextFile[];
}

export function getHighPriorityContexts(): ContextFile[] {
  const index = loadIndex();
  const highPaths = index.files.filter(f => f.priority === 'high').map(f => f.path);
  return highPaths.map(p => loadContext(p)).filter(Boolean) as ContextFile[];
}

export async function updateHomeAssistantContext(entities: Array<{entity_id: string; state: string; attributes: Record<string, unknown>}>): Promise<void> {
  const domains = new Map<string, number>();
  const recentEntities: Array<{entityId: string; state: string; friendlyName?: string}> = [];
  
  for (const entity of entities) {
    const domain = entity.entity_id.split('.')[0] ?? 'unknown';
    domains.set(domain, (domains.get(domain) ?? 0) + 1);
    
    // Keep only the most recently updated entities (limit to 15 total)
    if (recentEntities.length < 15) {
      const friendlyName = (entity.attributes?.['friendly_name'] as string) || undefined;
      recentEntities.push({
        entityId: entity.entity_id,
        state: entity.state,
        friendlyName,
      });
    }
  }

  // Create a concise summary
  const domainSummary = Array.from(domains.entries())
    .sort((a, b) => b[1] - a[1])
    .map(([domain, count]) => `${domain}: ${count}`)
    .join(', ');

  const recentList = recentEntities
    .map(e => `    ${e.entityId}: ${e.state}${e.friendlyName ? ` (${e.friendlyName})` : ''}`)
    .join('\n');

  const ctx: ContextFile = {
    label: 'home-assistant-entities',
    category: 'personal',
    priority: 'high',
    lastUpdated: new Date().toISOString(),
    version: 1,
    tags: ['home-assistant', 'entities', 'devices'],
    content: `Home Assistant Entities: ${domains.size} domains, ${entities.length} total entities
Available domains: ${domainSummary}

Recent/Sample entities:
${recentList}

To query a specific device state, use queryHomeAssistant skill.
To list all entities in a domain, use listHomeAssistantEntities skill.`,
  };
  
  saveContext('home-assistant-entities.json', ctx);
}

export function initDefaultContexts(): void {
  ensureDirs();
  const defaultCtx: ContextFile = {
    label: 'user-preferences-core',
    category: 'personal',
    priority: 'high',
    lastUpdated: new Date().toISOString(),
    version: 1,
    tags: ['always', 'behavior', 'constraints'],
    content:
      'You are CONTROL, a personal assistant. Use concise language. If unsure, ask rather than assume. Time zone: SAST (UTC+2). No meetings before 08:00 or after 18:00 unless explicitly allowed.',
  };
  const contextPath = 'user-preferences.json';
  if (!existsSync(join(CONTEXTS_DIR, contextPath))) {
    saveContext(contextPath, defaultCtx);
    logger.info('[MEMORY] Default context initialized');
  }
}
