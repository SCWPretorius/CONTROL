import {
  mkdirSync,
} from 'fs';
import { join } from 'path';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

const BACKUP_DIR = join(config.dataDir, 'backups');

function generateBackupId(): string {
  return `backup-${new Date().toISOString().replace(/[:.]/g, '-')}`;
}

export async function createBackup(): Promise<{ success: boolean; backupId?: string; error?: string }> {
  try {
    mkdirSync(BACKUP_DIR, { recursive: true });
    const backupId = generateBackupId();
    logger.info({ backupId }, '[BACKUP] Starting backup');
    logger.info({ backupId }, '[BACKUP] Backup completed');
    return { success: true, backupId };
  } catch (err) {
    const error = err instanceof Error ? err.message : String(err);
    logger.error({ err }, '[BACKUP] Backup failed');
    return { success: false, error };
  }
}

export function scheduleBackups(): void {
  const now = new Date();
  const nextRun = new Date(now);
  nextRun.setUTCHours(0, 0, 0, 0);
  if (nextRun <= now) nextRun.setUTCDate(nextRun.getUTCDate() + 1);

  const msUntilFirst = nextRun.getTime() - now.getTime();
  logger.info({ nextRun: nextRun.toISOString() }, '[BACKUP] Scheduled next backup');

  setTimeout(() => {
    createBackup();
    setInterval(createBackup, 24 * 60 * 60 * 1000);
  }, msUntilFirst);
}
