import { describe, it, expect } from 'vitest';

import { saveContext, loadContext, loadIndex } from '../src/memory/contextStore.js';
import { ContextFile } from '../src/types/index.js';

describe('contextStore', () => {
  const testCtx: ContextFile = {
    label: 'test-context',
    category: 'personal',
    priority: 'high',
    lastUpdated: new Date().toISOString(),
    tags: ['test', 'unit'],
    content: 'Test content for unit testing',
    version: 1,
  };

  it('saves and loads a context', () => {
    saveContext('test-context.json', testCtx);
    const loaded = loadContext('test-context.json');
    expect(loaded).not.toBeNull();
    expect(loaded?.label).toBe('test-context');
    expect(loaded?.content).toBe('Test content for unit testing');
  });

  it('returns null for non-existent context', () => {
    const loaded = loadContext('nonexistent.json');
    expect(loaded).toBeNull();
  });

  it('updates index when saving', () => {
    saveContext('indexed-context.json', { ...testCtx, label: 'indexed-context' });
    const index = loadIndex();
    const found = index.files.find(f => f.path === 'indexed-context.json');
    expect(found).toBeDefined();
    expect(found?.label).toBe('indexed-context');
  });
});
