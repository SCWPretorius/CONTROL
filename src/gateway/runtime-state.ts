import { EventEmitter } from 'events';
import { WebSocket } from 'ws';
import { logger } from '../logging/logger.js';

export interface ClientSession {
  id: string;
  deviceId?: string;
  ws: WebSocket;
  connectedAt: Date;
  lastSeen: Date;
}

export interface ActiveRun {
  id: string;
  sessionId: string;
  startedAt: Date;
  method: string;
}

class RuntimeState extends EventEmitter {
  private clients = new Map<string, ClientSession>();
  private runs = new Map<string, ActiveRun>();

  addClient(session: ClientSession): void {
    this.clients.set(session.id, session);
    logger.debug({ sessionId: session.id }, '[GATEWAY] Client connected');
    this.emit('client:connect', session);
  }

  removeClient(id: string): void {
    this.clients.delete(id);
    logger.debug({ sessionId: id }, '[GATEWAY] Client disconnected');
    this.emit('client:disconnect', id);
  }

  getClient(id: string): ClientSession | undefined {
    return this.clients.get(id);
  }

  getAllClients(): ClientSession[] {
    return Array.from(this.clients.values());
  }

  touchClient(id: string): void {
    const c = this.clients.get(id);
    if (c) c.lastSeen = new Date();
  }

  addRun(run: ActiveRun): void {
    this.runs.set(run.id, run);
  }

  removeRun(id: string): void {
    this.runs.delete(id);
  }

  getActiveRuns(): ActiveRun[] {
    return Array.from(this.runs.values());
  }

  async drainRuns(timeoutMs = 5000): Promise<void> {
    const deadline = Date.now() + timeoutMs;
    while (this.runs.size > 0 && Date.now() < deadline) {
      await new Promise(res => setTimeout(res, 100));
    }
  }

  push(sessionId: string, message: unknown): void {
    const client = this.clients.get(sessionId);
    if (client && client.ws.readyState === WebSocket.OPEN) {
      client.ws.send(JSON.stringify(message));
    }
  }

  broadcast(message: unknown): void {
    const payload = JSON.stringify(message);
    for (const client of this.clients.values()) {
      if (client.ws.readyState === WebSocket.OPEN) {
        client.ws.send(payload);
      }
    }
  }
}

export const runtimeState = new RuntimeState();
