import { NormalizedEvent } from '../types/index.js';
import { v4 as uuidv4 } from 'uuid';
import { config } from '../config/config.js';
import { getSecret } from '../config/secrets.js';
import { logger } from '../logging/logger.js';

let eventCallback: ((event: NormalizedEvent) => void) | null = null;
let lastStateUpdateTime: Record<string, string> = {};

interface HomeAssistantEntity {
  entity_id: string;
  state: string;
  attributes: Record<string, unknown>;
  last_updated: string;
  last_changed: string;
  context: {
    id: string;
    parent_id: string | null;
    user_id: string | null;
  };
}

interface HomeAssistantStatesResponse extends Array<HomeAssistantEntity> {}

async function getAllStates(): Promise<HomeAssistantStatesResponse | null> {
  if (!config.homeAssistant?.baseUrl) {
    logger.warn('[HOME_ASSISTANT] Base URL not configured');
    return null;
  }

  const token = getSecret('home_assistant_token');
  if (!token) {
    logger.warn('[HOME_ASSISTANT] Token not configured');
    return null;
  }

  try {
    const url = `${config.homeAssistant.baseUrl}/api/states`;
    const response = await fetch(url, {
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
    });

    if (!response.ok) {
      logger.error(
        { status: response.status, statusText: response.statusText },
        '[HOME_ASSISTANT] Get states failed'
      );
      return null;
    }

    return await response.json() as HomeAssistantStatesResponse;
  } catch (err) {
    logger.error({ err }, '[HOME_ASSISTANT] Failed to get states');
    return null;
  }
}

function normalizeEntityChange(entity: HomeAssistantEntity): NormalizedEvent | null {
  // Only create events for entities in our monitored list
  if (
    config.homeAssistant?.monitoredEntities &&
    !config.homeAssistant.monitoredEntities.includes(entity.entity_id)
  ) {
    return null;
  }

  const lastUpdate = lastStateUpdateTime[entity.entity_id];
  if (lastUpdate && lastUpdate === entity.last_updated) {
    return null;
  }

  lastStateUpdateTime[entity.entity_id] = entity.last_updated;

  const event: NormalizedEvent = {
    id: uuidv4(),
    traceId: uuidv4(),
    source: 'home-assistant',
    type: 'state-change',
    payload: {
      entityId: entity.entity_id,
      state: entity.state,
      attributes: entity.attributes,
      lastUpdated: entity.last_updated,
      lastChanged: entity.last_changed,
      context: entity.context,
    },
    timestamp: new Date().toISOString(),
    role: 'user', // Home Assistant is system-level, treat as user
  };

  return event;
}

const homeAssistantIntegration = {
  name: 'home-assistant',

  async poll(): Promise<NormalizedEvent[] | null> {
    const states = await getAllStates();
    if (!states) return null;

    const events = states
      .map(entity => normalizeEntityChange(entity))
      .filter(Boolean) as NormalizedEvent[];

    if (events.length > 0) {
      logger.debug({ count: events.length }, '[HOME_ASSISTANT] Detected state changes');
    }

    return events.length > 0 ? events : null;
  },

  async send(payload: Record<string, unknown>): Promise<void> {
    if (!config.homeAssistant?.baseUrl) {
      logger.warn('[HOME_ASSISTANT] Cannot send: Base URL not configured');
      return;
    }

    const token = getSecret('home_assistant_token');
    if (!token) {
      logger.warn('[HOME_ASSISTANT] Cannot send: Token not configured');
      return;
    }

    // For now, send is used for logging/tracing purposes
    // In the future, could send notifications to Home Assistant
    logger.debug({ payload }, '[HOME_ASSISTANT] Message payload received');
  },

  onEvent(callback: (event: NormalizedEvent) => void): void {
    eventCallback = callback;
  },

  /**
   * Query a specific entity state
   */
  async queryEntity(entityId: string): Promise<HomeAssistantEntity | null> {
    if (!config.homeAssistant?.baseUrl) return null;

    const token = getSecret('home_assistant_token');
    if (!token) return null;

    try {
      const url = `${config.homeAssistant.baseUrl}/api/states/${entityId}`;
      const response = await fetch(url, {
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) return null;
      return await response.json() as HomeAssistantEntity;
    } catch (err) {
      logger.error({ err, entityId }, '[HOME_ASSISTANT] Query entity failed');
      return null;
    }
  },

  /**
   * Call a Home Assistant service
   */
  async callService(
    domain: string,
    service: string,
    serviceData: Record<string, unknown> = {}
  ): Promise<unknown> {
    if (!config.homeAssistant?.baseUrl) {
      throw new Error('Home Assistant base URL not configured');
    }

    const token = getSecret('home_assistant_token');
    if (!token) {
      throw new Error('Home Assistant token not configured');
    }

    try {
      const url = `${config.homeAssistant.baseUrl}/api/services/${domain}/${service}`;
      const response = await fetch(url, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(serviceData),
      });

      if (!response.ok) {
        const text = await response.text();
        throw new Error(`Service call failed: ${response.statusText} - ${text}`);
      }

      const result = await response.json();
      logger.info(
        { domain, service, entityCount: Array.isArray(result) ? result.length : 1 },
        '[HOME_ASSISTANT] Service called'
      );
      return result;
    } catch (err) {
      logger.error({ err, domain, service }, '[HOME_ASSISTANT] Service call failed');
      throw err;
    }
  },

  /**
   * Get all available entities
   */
  async getAvailableEntities(): Promise<HomeAssistantEntity[] | null> {
    return getAllStates();
  },
};

export default homeAssistantIntegration;
export type { HomeAssistantEntity };
