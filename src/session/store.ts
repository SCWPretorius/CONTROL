import { mkdirSync, readFileSync, writeFileSync, existsSync } from 'fs';
import { join } from 'path';
import { createHash } from 'crypto';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

export interface Session {
  id: string;
  source: string;
  userId: string;
  createdAt: string;
  lastActivityAt: string;
}

const sessionsDir = join(config.dataDir, 'sessions');
mkdirSync(sessionsDir, { recursive: true });

function sessionPath(id: string): string {
  const hash = createHash('sha256').update(id).digest('hex');
  return join(sessionsDir, `${hash}.json`);
}

export function getSession(id: string): Session | undefined {
  const p = sessionPath(id);
  if (!existsSync(p)) return undefined;
  try {
    return JSON.parse(readFileSync(p, 'utf-8')) as Session;
  } catch {
    return undefined;
  }
}

export function saveSession(session: Session): void {
  try {
    writeFileSync(sessionPath(session.id), JSON.stringify(session, null, 2));
  } catch (err) {
    logger.warn({ err, sessionId: session.id }, '[SESSION] Failed to save session');
  }
}

export function getOrCreateSession(source: string, userId: string): Session {
  const id = `${source}:${userId}`;
  const existing = getSession(id);
  if (existing) {
    existing.lastActivityAt = new Date().toISOString();
    saveSession(existing);
    return existing;
  }
  const session: Session = {
    id,
    source,
    userId,
    createdAt: new Date().toISOString(),
    lastActivityAt: new Date().toISOString(),
  };
  saveSession(session);
  return session;
}
