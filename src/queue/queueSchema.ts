import sqlite3 from 'sqlite3';
import path from 'path';
import { fileURLToPath } from 'url';
import { logger } from '../logging/logger.js';
import { promisify } from 'util';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

let db: sqlite3.Database | null = null;

export function initializeQueueDatabase(dbPath: string): Promise<sqlite3.Database> {
  return new Promise((resolve, reject) => {
    db = new sqlite3.Database(dbPath, async (err) => {
      if (err) {
        logger.error({ err, dbPath }, '[QUEUE] Failed to initialize database');
        reject(err);
        return;
      }

      try {
        // Enable foreign keys and optimize for our use case
        await runAsync('PRAGMA foreign_keys = ON');
        await runAsync('PRAGMA journal_mode = WAL');
        
        await createSchema();
        logger.info({ dbPath }, '[QUEUE] Database initialized');
        resolve(db!);
      } catch (schemaErr) {
        logger.error({ schemaErr }, '[QUEUE] Failed to create schema');
        reject(schemaErr);
      }
    });
  });
}

async function createSchema(): Promise<void> {
  // Create messages table
  await runAsync(`
    CREATE TABLE IF NOT EXISTS queue_messages (
      id TEXT PRIMARY KEY,
      type TEXT NOT NULL CHECK(type IN ('incoming', 'outgoing')),
      status TEXT NOT NULL CHECK(status IN ('pending', 'processing', 'completed', 'failed')) DEFAULT 'pending',
      source TEXT NOT NULL,
      targetChannel TEXT,
      payload TEXT NOT NULL,
      event TEXT,
      error TEXT,
      retryCount INTEGER NOT NULL DEFAULT 0,
      maxRetries INTEGER NOT NULL DEFAULT 5,
      createdAt TEXT NOT NULL,
      updatedAt TEXT NOT NULL,
      expiresAt TEXT NOT NULL
    );
  `);

  // Create indices for common queries
  await runAsync(`
    CREATE INDEX IF NOT EXISTS idx_queue_status ON queue_messages(status);
  `);
  await runAsync(`
    CREATE INDEX IF NOT EXISTS idx_queue_expires ON queue_messages(expiresAt);
  `);
  await runAsync(`
    CREATE INDEX IF NOT EXISTS idx_queue_created ON queue_messages(createdAt);
  `);
  await runAsync(`
    CREATE INDEX IF NOT EXISTS idx_queue_source ON queue_messages(source);
  `);

  logger.debug('[QUEUE] Schema created/verified');
}

function runAsync(sql: string, params: any[] = []): Promise<void> {
  return new Promise((resolve, reject) => {
    if (!db) {
      reject(new Error('Database not initialized'));
      return;
    }
    db.run(sql, params, function(err) {
      if (err) {
        reject(err);
      } else {
        resolve();
      }
    });
  });
}

export function getQueueDatabase(): sqlite3.Database {
  if (!db) {
    throw new Error('Queue database not initialized. Call initializeQueueDatabase first.');
  }
  return db;
}

export function closeQueueDatabase(): Promise<void> {
  return new Promise((resolve, reject) => {
    if (db) {
      db.close((err) => {
        db = null;
        if (err) {
          logger.error({ err }, '[QUEUE] Error closing database');
          reject(err);
        } else {
          logger.info('[QUEUE] Database closed');
          resolve();
        }
      });
    } else {
      resolve();
    }
  });
}

export function dbRun(sql: string, params: any[] = []): Promise<{ lastID?: number; changes?: number }> {
  return new Promise((resolve, reject) => {
    if (!db) {
      reject(new Error('Database not initialized'));
      return;
    }
    db.run(sql, params, function(err) {
      if (err) {
        reject(err);
      } else {
        resolve({ lastID: this.lastID, changes: this.changes });
      }
    });
  });
}

export function dbGet(sql: string, params: any[] = []): Promise<any> {
  return new Promise((resolve, reject) => {
    if (!db) {
      reject(new Error('Database not initialized'));
      return;
    }
    db.get(sql, params, (err, row) => {
      if (err) {
        reject(err);
      } else {
        resolve(row);
      }
    });
  });
}

export function dbAll(sql: string, params: any[] = []): Promise<any[]> {
  return new Promise((resolve, reject) => {
    if (!db) {
      reject(new Error('Database not initialized'));
      return;
    }
    db.all(sql, params, (err, rows) => {
      if (err) {
        reject(err);
      } else {
        resolve(rows || []);
      }
    });
  });
}
