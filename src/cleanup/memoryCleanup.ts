import {
  existsSync,
  mkdirSync,
  renameSync,
  readFileSync,
  writeFileSync,
  statSync,
} from 'fs';
import { join } from 'path';
import { config } from '../config/config.js';
import { loadIndex, saveIndex } from '../memory/contextStore.js';
import { logger } from '../logging/logger.js';

const RETENTION_DAYS: Record<string, number> = {
  personal: 365,
  conversation: 30,
  temporary: 7,
  'event-log': 90,
};

const ARCHIVE_INDEX_FILE = join(config.memoryDir, 'archive', 'index.json');

interface ArchiveEntry {
  originalPath: string;
  archivePath: string;
  movedAt: string;
  category: string;
  size: number;
}

function loadArchiveIndex(): ArchiveEntry[] {
  if (!existsSync(ARCHIVE_INDEX_FILE)) return [];
  try {
    return JSON.parse(readFileSync(ARCHIVE_INDEX_FILE, 'utf-8')) as ArchiveEntry[];
  } catch {
    return [];
  }
}

function saveArchiveIndex(entries: ArchiveEntry[]): void {
  mkdirSync(join(config.memoryDir, 'archive'), { recursive: true });
  writeFileSync(ARCHIVE_INDEX_FILE, JSON.stringify(entries, null, 2), 'utf-8');
}

export function runMemoryCleanup(): void {
  logger.info('[CLEANUP] Starting memory cleanup');
  const index = loadIndex();
  const now = new Date();
  const toRemove: string[] = [];
  const archiveEntries = loadArchiveIndex();

  for (const entry of index.files) {
    const retentionDays = RETENTION_DAYS[entry.category] ?? 365;
    const lastUpdated = new Date(entry.lastUpdated);
    const ageMs = now.getTime() - lastUpdated.getTime();
    const ageDays = ageMs / (1000 * 60 * 60 * 24);

    if (ageDays > retentionDays) {
      const contextDir = join(config.memoryDir, 'contexts');
      const fullPath = join(contextDir, entry.path);

      const yearMonth = lastUpdated.toISOString().slice(0, 7);
      const archiveDir = join(config.memoryDir, 'archive', entry.category, yearMonth);
      mkdirSync(archiveDir, { recursive: true });

      const archivePath = join(archiveDir, entry.path.replace(/\//g, '_'));
      try {
        if (existsSync(fullPath)) {
          renameSync(fullPath, archivePath);
          const stat = statSync(archivePath);
          archiveEntries.push({
            originalPath: entry.path,
            archivePath: archivePath,
            movedAt: now.toISOString(),
            category: entry.category,
            size: stat.size,
          });
          logger.info({ path: entry.path, archivePath }, '[CLEANUP] Archived context');
        }
        toRemove.push(entry.path);
      } catch (err) {
        logger.error({ err, path: entry.path }, '[CLEANUP] Failed to archive');
      }
    }
  }

  if (toRemove.length > 0) {
    index.files = index.files.filter(f => !toRemove.includes(f.path));
    saveIndex(index);
    saveArchiveIndex(archiveEntries);
    logger.info({ archived: toRemove.length }, '[CLEANUP] Memory cleanup completed');
  } else {
    logger.info('[CLEANUP] No contexts eligible for archival');
  }
}

export function scheduleMemoryCleanup(): void {
  const now = new Date();
  const nextSunday = new Date(now);
  nextSunday.setUTCHours(0, 0, 0, 0);
  const dayOfWeek = nextSunday.getUTCDay();
  const daysUntilSunday = (7 - dayOfWeek) % 7 || 7;
  nextSunday.setUTCDate(nextSunday.getUTCDate() + daysUntilSunday);

  const msUntilCleanup = nextSunday.getTime() - now.getTime();
  logger.info(
    { nextRun: nextSunday.toISOString() },
    '[CLEANUP] Scheduled next memory cleanup'
  );

  setTimeout(() => {
    runMemoryCleanup();
    setInterval(runMemoryCleanup, 7 * 24 * 60 * 60 * 1000);
  }, msUntilCleanup);
}
