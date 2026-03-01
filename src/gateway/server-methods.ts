import type { RpcRequest, RpcResponse } from '../types/gateway.js';
import { runtimeState } from './runtime-state.js';
import { skillRegistry } from '../skills/skillRegistry.js';
import { getLLMStatus } from '../llm/llmDecider.js';
import { getIntegrationHealth } from '../integrations/integrationRunner.js';
import { logger } from '../logging/logger.js';
import { runAgent } from '../agents/agent-runner.js';
import type { AgentBlock } from '../agents/types.js';

type MethodHandler = (params: Record<string, unknown>, sessionId: string) => Promise<unknown>;

const methods: Record<string, MethodHandler> = {
  status: async () => ({
    clients: runtimeState.getAllClients().map(c => ({
      id: c.id,
      deviceId: c.deviceId,
      connectedAt: c.connectedAt.toISOString(),
    })),
    activeRuns: runtimeState.getActiveRuns().length,
    skills: skillRegistry.getActive().length,
    llm: getLLMStatus(),
    integrations: getIntegrationHealth().map(h => ({
      name: h.name,
      quarantined: h.quarantined,
      errors: h.consecutiveErrors,
    })),
  }),

  'models.list': async () => ({
    models: [
      { id: 'primary', provider: 'lmstudio', status: getLLMStatus() },
    ],
  }),

  'skills.list': async () => ({
    skills: skillRegistry.getActiveDefinitions().map(d => ({
      name: d.name,
      description: d.description,
      tags: d.tags,
      minRole: d.minRole,
    })),
  }),

  chat: async (params, sessionId) => {
    const message = params.message;
    if (typeof message !== 'string' || !message.trim()) {
      throw new Error('params.message must be a non-empty string');
    }

    const client = runtimeState.getClient(sessionId);
    const userId = client?.deviceId ?? sessionId;
    let blocksEmitted = 0;

    await runAgent(
      { wsSessionId: sessionId, userId, role: 'admin', source: 'gateway', message },
      (block: AgentBlock) => {
        blocksEmitted++;
        runtimeState.push(sessionId, {
          event: 'agent:block',
          data: block,
          ts: new Date().toISOString(),
        });
      },
    );

    return { ok: true, blocksEmitted };
  },

  agent: async (params, sessionId) => {
    const message = params.message;
    if (typeof message !== 'string' || !message.trim()) {
      throw new Error('params.message must be a non-empty string');
    }

    const client = runtimeState.getClient(sessionId);
    const userId = client?.deviceId ?? sessionId;
    let blocksEmitted = 0;

    await runAgent(
      { wsSessionId: sessionId, userId, role: 'admin', source: 'gateway', message },
      (block: AgentBlock) => {
        blocksEmitted++;
        runtimeState.push(sessionId, {
          event: 'agent:block',
          data: block,
          ts: new Date().toISOString(),
        });
      },
    );

    return { ok: true, blocksEmitted };
  },
};

export function registerMethod(name: string, handler: MethodHandler): void {
  methods[name] = handler;
}

export async function dispatch(
  request: RpcRequest,
  sessionId: string,
): Promise<RpcResponse> {
  const handler = methods[request.method];
  if (!handler) {
    return {
      id: request.id,
      error: { code: -32601, message: `Method not found: ${request.method}` },
    };
  }
  try {
    const result = await handler(request.params ?? {}, sessionId);
    return { id: request.id, result };
  } catch (err) {
    logger.error({ err, method: request.method }, '[GATEWAY] Method handler error');
    return {
      id: request.id,
      error: { code: -32603, message: err instanceof Error ? err.message : 'Internal error' },
    };
  }
}
