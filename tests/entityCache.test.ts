import { describe, it, expect, beforeEach } from 'vitest';
import {
  getCachedEntitySelection,
  cacheEntitySelection,
  clearEntityCache,
  getEntityCacheStats,
} from '../src/memory/entitySelectionCache.js';

describe('Entity Selection Cache', () => {
  beforeEach(() => {
    clearEntityCache();
  });

  it('should cache and retrieve entity selection', () => {
    cacheEntitySelection(
      'driveway beams armed',
      'binary_sensor.driveway_partition',
      'Driveway Partition Armed',
      'binary_sensor',
      95
    );

    const cached = getCachedEntitySelection('driveway beams armed');
    expect(cached).toBeDefined();
    expect(cached?.selectedEntityId).toBe('binary_sensor.driveway_partition');
    expect(cached?.selectedFriendlyName).toBe('Driveway Partition Armed');
    expect(cached?.confidence).toBe(95);
  });

  it('should return null for cache miss', () => {
    const cached = getCachedEntitySelection('nonexistent query');
    expect(cached).toBeNull();
  });

  it('should match similar queries', () => {
    cacheEntitySelection(
      'driveway beams armed',
      'binary_sensor.driveway_partition',
      'Driveway Partition Armed',
      'binary_sensor',
      95
    );

    // Test with slightly different query
    const cached1 = getCachedEntitySelection('driveway beams');
    expect(cached1).toBeDefined();
    expect(cached1?.selectedEntityId).toBe('binary_sensor.driveway_partition');

    // Test with reordered words
    const cached2 = getCachedEntitySelection('beams driveway armed');
    expect(cached2).toBeDefined();
    expect(cached2?.selectedEntityId).toBe('binary_sensor.driveway_partition');
  });

  it('should not match dissimilar queries', () => {
    cacheEntitySelection(
      'driveway beams armed',
      'binary_sensor.driveway_partition',
      'Driveway Partition Armed',
      'binary_sensor',
      95
    );

    const cached = getCachedEntitySelection('kitchen light');
    expect(cached).toBeNull();
  });

  it('should provide cache statistics', () => {
    cacheEntitySelection(
      'query1',
      'entity.id1',
      'Entity 1',
      'sensor',
      80
    );
    cacheEntitySelection(
      'query2',
      'entity.id2',
      'Entity 2',
      'switch',
      90
    );

    const stats = getEntityCacheStats();
    expect(stats.total).toBe(2);
    expect(stats.entries).toHaveLength(2);
    expect(stats.entries[0].query).toBe('query1');
    expect(stats.entries[1].query).toBe('query2');
  });

  it('should clear cache', () => {
    cacheEntitySelection(
      'driveway beams',
      'binary_sensor.driveway',
      'Driveway Sensor',
      'binary_sensor',
      90
    );

    let stats = getEntityCacheStats();
    expect(stats.total).toBe(1);

    clearEntityCache();

    stats = getEntityCacheStats();
    expect(stats.total).toBe(0);
  });
});
