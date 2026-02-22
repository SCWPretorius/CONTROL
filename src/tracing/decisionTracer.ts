import { appendFileSync, mkdirSync } from 'fs';
import { join } from 'path';
import { config } from '../config/config.js';
import { DecisionTrace } from '../types/index.js';
import { logger } from '../logging/logger.js';

const TRACES_DIR = join(config.logsDir, 'decision-traces');

function ensureDir(): void {
  mkdirSync(TRACES_DIR, { recursive: true });
}

export function writeTrace(trace: DecisionTrace): void {
  ensureDir();
  const date = new Date().toISOString().slice(0, 10);
  const filePath = join(TRACES_DIR, `${date}.jsonl`);
  try {
    appendFileSync(filePath, JSON.stringify(trace) + '\n', 'utf-8');
  } catch (err) {
    logger.error({ err }, '[TRACER] Failed to write trace');
  }
}

export function buildTrace(
  partial: Omit<DecisionTrace, 'timestamp'>
): DecisionTrace {
  return {
    ...partial,
    timestamp: new Date().toISOString(),
  };
}
