import { createServer, type Server as HttpServer } from 'http';
import { WebSocketServer, WebSocket } from 'ws';
import { v4 as uuidv4 } from 'uuid';
import { app } from '../server/server.js';
import { config } from '../config/config.js';
import { runtimeState } from './runtime-state.js';
import { dispatch } from './server-methods.js';
import { checkWsHandshakeRate } from './rate-limiter.js';
import { logger } from '../logging/logger.js';
import type { RpcRequest } from '../types/gateway.js';
import type { IncomingMessage } from 'http';

let httpServer: HttpServer | null = null;
let wss: WebSocketServer | null = null;
let heartbeatTimer: ReturnType<typeof setInterval> | null = null;

function handleConnection(ws: WebSocket, req: IncomingMessage): void {
  const origin =
    (req.headers['x-forwarded-for'] as string | undefined)?.split(',')[0]?.trim()
    ?? req.socket.remoteAddress
    ?? 'unknown';

  const rateCheck = checkWsHandshakeRate(origin);
  if (!rateCheck.allowed) {
    ws.close(1008, rateCheck.reason ?? 'Rate limit exceeded');
    return;
  }

  const sessionId = uuidv4();
  runtimeState.addClient({ id: sessionId, ws, connectedAt: new Date(), lastSeen: new Date() });

  ws.send(JSON.stringify({
    event: 'connected',
    data: { sessionId, version: '1.0' },
    ts: new Date().toISOString(),
  }));

  ws.on('message', async (raw) => {
    runtimeState.touchClient(sessionId);
    let request: RpcRequest;
    try {
      request = JSON.parse(raw.toString()) as RpcRequest;
    } catch {
      ws.send(JSON.stringify({ id: null, error: { code: -32700, message: 'Parse error' } }));
      return;
    }

    if (request.id === undefined || request.id === null || !request.method) {
      ws.send(JSON.stringify({ id: request.id ?? null, error: { code: -32600, message: 'Invalid request' } }));
      return;
    }

    const runId = uuidv4();
    runtimeState.addRun({ id: runId, sessionId, startedAt: new Date(), method: request.method });
    try {
      const response = await dispatch(request, sessionId);
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(response));
      }
    } finally {
      runtimeState.removeRun(runId);
    }
  });

  ws.on('close', () => runtimeState.removeClient(sessionId));
  ws.on('error', (err) => {
    logger.error({ err, sessionId }, '[GATEWAY] Socket error');
    runtimeState.removeClient(sessionId);
  });
}

export function startGateway(): void {
  httpServer = createServer(app);
  wss = new WebSocketServer({ server: httpServer });
  wss.on('connection', handleConnection);

  heartbeatTimer = setInterval(() => {
    const ts = new Date().toISOString();
    runtimeState.broadcast({ event: 'ping', data: { ts }, ts });
  }, 30_000);

  httpServer.listen(config.port, config.host, () => {
    logger.info({ port: config.port, host: config.host }, '[GATEWAY] Started (HTTP + WS)');
  });
}

export async function stopGateway(): Promise<void> {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }

  await runtimeState.drainRuns(5000);

  for (const client of runtimeState.getAllClients()) {
    client.ws.close(1001, 'Server shutting down');
  }

  return new Promise((resolve) => {
    if (wss) {
      wss!.close(() => {
        if (httpServer) {
          httpServer!.close(() => {
            logger.info('[GATEWAY] Stopped');
            resolve();
          });
        } else {
          resolve();
        }
      });
    } else {
      resolve();
    }
  });
}
