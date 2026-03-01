import { logger } from '../logging/logger.js';

interface ConnectionWindow {
  count: number;
  windowStart: number;
}

const WINDOW_MS = 60_000;
const MAX_PER_WINDOW = 10;

const wsWindows = new Map<string, ConnectionWindow>();

/** Rate-limit WS handshake attempts to MAX_PER_WINDOW per origin per minute. */
export function checkWsHandshakeRate(origin: string): { allowed: boolean; reason?: string } {
  const now = Date.now();
  let entry = wsWindows.get(origin);

  if (!entry || now - entry.windowStart >= WINDOW_MS) {
    entry = { count: 0, windowStart: now };
    wsWindows.set(origin, entry);
  }

  if (entry.count >= MAX_PER_WINDOW) {
    logger.warn({ origin }, '[RATE] WS handshake rate limit exceeded');
    return { allowed: false, reason: `Too many WS connections from ${origin} — try again in 1 minute` };
  }

  entry.count++;
  return { allowed: true };
}

/** Reset all WS rate-limit windows (for testing). */
export function resetWsRateLimiter(): void {
  wsWindows.clear();
}
