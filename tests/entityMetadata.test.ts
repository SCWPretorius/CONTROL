import { describe, it, expect } from 'vitest';
import {
  enrichEntityMetadata,
  calculateMatchScore,
  rankEntities,
  EntityMetadata,
} from '../src/memory/entityMetadata.js';

describe('Entity Metadata', () => {
  const mockRawEntities = [
    {
      entity_id: 'binary_sensor.driveway_beams',
      state: 'off',
      attributes: {
        friendly_name: 'Driveway Beams Sensor',
        area_name: 'Driveway',
        device_class: 'motion',
      },
    },
    {
      entity_id: 'switch.driveway_partition',
      state: 'armed',
      attributes: {
        friendly_name: 'Driveway Partition Armed',
        area_name: 'Driveway',
      },
    },
    {
      entity_id: 'sensor.kitchen_temperature',
      state: '22.5',
      attributes: {
        friendly_name: 'Kitchen Temperature',
        area_name: 'Kitchen',
        device_class: 'temperature',
      },
    },
  ];

  describe('enrichEntityMetadata', () => {
    it('should enrich entity metadata with device classes and capabilities', async () => {
      const enriched = await enrichEntityMetadata(mockRawEntities);

      expect(enriched.total).toBe(3);
      expect(enriched.domainSummary).toEqual({
        binary_sensor: 1,
        switch: 1,
        sensor: 1,
      });

      // Check binary_sensor enrichment
      const binarySensor = enriched.entities.find(
        e => e.entityId === 'binary_sensor.driveway_beams'
      );
      expect(binarySensor).toBeDefined();
      expect(binarySensor?.deviceClass).toBe('motion');
      expect(binarySensor?.capabilities).toContain('query');

      // Check switch enrichment
      const switchEntity = enriched.entities.find(
        e => e.entityId === 'switch.driveway_partition'
      );
      expect(switchEntity).toBeDefined();
      expect(switchEntity?.capabilities).toContain('toggle');
      expect(switchEntity?.capabilities).toContain('turn_on');
      expect(switchEntity?.capabilities).toContain('turn_off');
    });

    it('should infer device class from friendly name when not provided', async () => {
      const entities = [
        {
          entity_id: 'binary_sensor.test_motion',
          state: 'off',
          attributes: {
            friendly_name: 'Test Motion Sensor',
          },
        },
      ];

      const enriched = await enrichEntityMetadata(entities);
      const entity = enriched.entities[0];
      expect(entity.deviceClass).toBe('motion');
    });

    it('should handle entity relationships', async () => {
      const relationships = {
        'switch.driveway_partition': ['binary_sensor.driveway_beams'],
      };

      const enriched = await enrichEntityMetadata(mockRawEntities, relationships);
      const switchEntity = enriched.entities.find(
        e => e.entityId === 'switch.driveway_partition'
      );
      expect(switchEntity?.relatedEntities).toEqual(['binary_sensor.driveway_beams']);
    });
  });

  describe('calculateMatchScore', () => {
    const testEntities: EntityMetadata[] = [
      {
        entityId: 'binary_sensor.driveway_beams',
        friendlyName: 'Driveway Beams Sensor',
        domain: 'binary_sensor',
        deviceClass: 'motion',
        capabilities: ['query'],
        state: 'off',
      },
      {
        entityId: 'switch.driveway_partition',
        friendlyName: 'Driveway Partition Armed',
        domain: 'switch',
        deviceClass: 'armed_disarmed',
        capabilities: ['toggle', 'turn_on', 'turn_off'],
        state: 'armed',
      },
    ];

    it('should score exact entity ID match higher', () => {
      const result = calculateMatchScore('driveway_beams', testEntities[0]);
      expect(result.score).toBeGreaterThan(0);
      expect(result.reasons).toContain('exact match in entity ID');
    });

    it('should score exact friendly name match', () => {
      const result = calculateMatchScore('Driveway Beams Sensor', testEntities[0]);
      expect(result.score).toBeGreaterThan(0);
      expect(result.reasons).toContain('exact match in friendly name');
    });

    it('should give bonus for semantic capability match', () => {
      const armedQuery = calculateMatchScore('driveway armed', testEntities[1]);
      expect(armedQuery.score).toBeGreaterThan(0);
      expect(armedQuery.reasons.some(r => r.includes('armed'))).toBe(true);
    });

    it('should score partitions higher than sensors for armed queries', () => {
      const sensorMatch = calculateMatchScore('driveway beams armed', testEntities[0]);
      const partitionMatch = calculateMatchScore('driveway beams armed', testEntities[1]);
      
      // Partition should score higher because of armed_disarmed device class
      expect(partitionMatch.score).toBeGreaterThan(sensorMatch.score);
    });
  });

  describe('rankEntities', () => {
    const testEntities: EntityMetadata[] = [
      {
        entityId: 'sensor.kitchen_light',
        friendlyName: 'Kitchen Light',
        domain: 'sensor',
        state: 'on',
      },
      {
        entityId: 'sensor.kitchen_temperature',
        friendlyName: 'Kitchen Temperature',
        domain: 'sensor',
        deviceClass: 'temperature',
        capabilities: ['query'],
        state: '22.5',
      },
      {
        entityId: 'light.kitchen',
        friendlyName: 'Kitchen',
        domain: 'light',
        capabilities: ['turn_on', 'turn_off'],
        state: 'off',
      },
    ];

    it('should rank entities by match quality', () => {
      const ranked = rankEntities('kitchen temperature', testEntities, 3);
      expect(ranked).toHaveLength(3);
      expect(ranked[0].entityId).toBe('sensor.kitchen_temperature');
      expect(ranked[0].confidence).toBe(100);
    });

    it('should return top N results', () => {
      const ranked = rankEntities('kitchen', testEntities, 2);
      expect(ranked).toHaveLength(2);
    });

    it('should calculate confidence percentages', () => {
      const ranked = rankEntities('kitchen', testEntities, 3);
      expect(ranked[0].confidence).toBe(100);
      // Note: If all entities have similar scores, confidence might be similar
      // Just verify they're all within reasonable range
      expect(ranked[1].confidence).toBeGreaterThan(0);
      expect(ranked[1].confidence).toBeLessThanOrEqual(100);
      expect(ranked[2].confidence).toBeGreaterThan(0);
      expect(ranked[2].confidence).toBeLessThanOrEqual(ranked[1].confidence);
    });
  });
});
