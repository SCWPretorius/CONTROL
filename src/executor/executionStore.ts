import {
  readFileSync,
  writeFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
} from 'fs';
import { join } from 'path';
import { config } from '../config/config.js';
import { ExecutionRecord } from '../types/index.js';

function ensureDir(): void {
  mkdirSync(config.executionsDir, { recursive: true });
}

export function saveExecution(record: ExecutionRecord): void {
  ensureDir();
  const filePath = join(config.executionsDir, `${record.id}.json`);
  writeFileSync(filePath, JSON.stringify(record, null, 2), 'utf-8');
}

export function loadExecution(id: string): ExecutionRecord | null {
  const filePath = join(config.executionsDir, `${id}.json`);
  if (!existsSync(filePath)) return null;
  try {
    return JSON.parse(readFileSync(filePath, 'utf-8')) as ExecutionRecord;
  } catch {
    return null;
  }
}

export function getPendingExecutions(): ExecutionRecord[] {
  ensureDir();
  const files = readdirSync(config.executionsDir).filter(f => f.endsWith('.json'));
  return files
    .map(f => {
      try {
        return JSON.parse(readFileSync(join(config.executionsDir, f), 'utf-8')) as ExecutionRecord;
      } catch {
        return null;
      }
    })
    .filter((r): r is ExecutionRecord => r !== null && (r.status === 'pending' || r.status === 'in-progress'));
}
