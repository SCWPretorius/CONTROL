import { v4 as uuidv4 } from 'uuid';
import { QueueMessage, QueueMessageType, QueueMessageStatus, NormalizedEvent } from '../types/index.js';
import { dbRun, dbGet, dbAll } from './queueSchema.js';
import { logger } from '../logging/logger.js';

export class MessageQueue {
  private maxRetries: number;
  private messageTtlMs: number;

  constructor(maxRetries: number = 5, messageTtlMs: number = 86400000) {
    // Default 24 hours
    this.maxRetries = maxRetries;
    this.messageTtlMs = messageTtlMs;
  }

  async enqueue(
    type: QueueMessageType,
    source: string,
    payload: Record<string, unknown>,
    event?: NormalizedEvent,
    targetChannel?: string
  ): Promise<string> {
    const id = uuidv4();
    const now = new Date().toISOString();
    const expiresAt = new Date(Date.now() + this.messageTtlMs).toISOString();

    try {
      await dbRun(
        `
        INSERT INTO queue_messages (
          id, type, status, source, targetChannel, payload, event, 
          retryCount, maxRetries, createdAt, updatedAt, expiresAt
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `,
        [
          id,
          type,
          'pending',
          source,
          targetChannel || null,
          JSON.stringify(payload),
          event ? JSON.stringify(event) : null,
          0,
          this.maxRetries,
          now,
          now,
          expiresAt,
        ]
      );

      logger.info(
        { messageId: id, type, source, status: 'pending' },
        '[QUEUE] Message enqueued'
      );

      return id;
    } catch (err) {
      logger.error({ err, type, source }, '[QUEUE] Failed to enqueue message');
      throw err;
    }
  }

  async markProcessing(messageId: string): Promise<void> {
    const now = new Date().toISOString();

    try {
      const result = await dbRun(
        `
        UPDATE queue_messages
        SET status = ?, updatedAt = ?
        WHERE id = ?
      `,
        ['processing', now, messageId]
      );

      if (!result.changes || result.changes === 0) {
        logger.warn({ messageId }, '[QUEUE] Message not found for marking processing');
      } else {
        logger.debug({ messageId }, '[QUEUE] Message marked as processing');
      }
    } catch (err) {
      logger.error({ err, messageId }, '[QUEUE] Failed to mark message as processing');
    }
  }

  async markCompleted(messageId: string): Promise<void> {
    const now = new Date().toISOString();

    try {
      const result = await dbRun(
        `
        UPDATE queue_messages
        SET status = ?, updatedAt = ?
        WHERE id = ?
      `,
        ['completed', now, messageId]
      );

      if (!result.changes || result.changes === 0) {
        logger.warn({ messageId }, '[QUEUE] Message not found for marking completed');
      } else {
        logger.info({ messageId }, '[QUEUE] Message marked as completed');
      }
    } catch (err) {
      logger.error({ err, messageId }, '[QUEUE] Failed to mark message as completed');
    }
  }

  async markFailed(messageId: string, error: string): Promise<void> {
    const now = new Date().toISOString();

    try {
      const result = await dbRun(
        `
        UPDATE queue_messages
        SET status = ?, error = ?, updatedAt = ?
        WHERE id = ?
      `,
        ['failed', error, now, messageId]
      );

      if (!result.changes || result.changes === 0) {
        logger.warn({ messageId }, '[QUEUE] Message not found for marking failed');
      } else {
        logger.warn({ messageId, error }, '[QUEUE] Message marked as failed');
      }
    } catch (err) {
      logger.error({ err, messageId }, '[QUEUE] Failed to mark message as failed');
    }
  }

  async incrementRetry(messageId: string): Promise<number> {
    const now = new Date().toISOString();

    try {
      await dbRun(
        `
        UPDATE queue_messages
        SET retryCount = retryCount + 1, status = ?, updatedAt = ?
        WHERE id = ?
      `,
        ['pending', now, messageId]
      );

      const result = await dbGet(`SELECT retryCount FROM queue_messages WHERE id = ?`, [messageId]);

      if (!result) {
        logger.warn({ messageId }, '[QUEUE] Message not found for retry increment');
        return -1;
      }

      logger.debug({ messageId, retryCount: result.retryCount }, '[QUEUE] Retry count incremented');
      return result.retryCount as number;
    } catch (err) {
      logger.error({ err, messageId }, '[QUEUE] Failed to increment retry count');
      return -1;
    }
  }

  async getPendingMessages(limit: number = 10): Promise<QueueMessage[]> {
    try {
      const rows = await dbAll(
        `
        SELECT * FROM queue_messages
        WHERE status = 'pending' AND retryCount < maxRetries
        ORDER BY createdAt ASC
        LIMIT ?
      `,
        [limit]
      );

      return rows.map(row => this.rowToQueueMessage(row));
    } catch (err) {
      logger.error({ err }, '[QUEUE] Failed to get pending messages');
      return [];
    }
  }

  async getFailedMessages(limit: number = 100): Promise<QueueMessage[]> {
    try {
      const rows = await dbAll(
        `
        SELECT * FROM queue_messages
        WHERE status = 'failed'
        ORDER BY updatedAt DESC
        LIMIT ?
      `,
        [limit]
      );

      return rows.map(row => this.rowToQueueMessage(row));
    } catch (err) {
      logger.error({ err }, '[QUEUE] Failed to get failed messages');
      return [];
    }
  }

  async getMessageById(messageId: string): Promise<QueueMessage | null> {
    try {
      const row = await dbGet(`SELECT * FROM queue_messages WHERE id = ?`, [messageId]);

      return row ? this.rowToQueueMessage(row) : null;
    } catch (err) {
      logger.error({ err, messageId }, '[QUEUE] Failed to get message by ID');
      return null;
    }
  }

  async getAllMessages(status?: QueueMessageStatus, limit: number = 1000): Promise<QueueMessage[]> {
    try {
      let query = `SELECT * FROM queue_messages`;
      const params: any[] = [];

      if (status) {
        query += ` WHERE status = ?`;
        params.push(status);
      }

      query += ` ORDER BY createdAt DESC LIMIT ?`;
      params.push(limit);

      const rows = await dbAll(query, params);
      return rows.map(row => this.rowToQueueMessage(row));
    } catch (err) {
      logger.error({ err, status }, '[QUEUE] Failed to get all messages');
      return [];
    }
  }

  async deleteMessage(messageId: string): Promise<boolean> {
    try {
      const result = await dbRun(`DELETE FROM queue_messages WHERE id = ?`, [messageId]);

      if (result.changes && result.changes > 0) {
        logger.info({ messageId }, '[QUEUE] Message deleted');
        return true;
      }
      logger.warn({ messageId }, '[QUEUE] Message not found for deletion');
      return false;
    } catch (err) {
      logger.error({ err, messageId }, '[QUEUE] Failed to delete message');
      return false;
    }
  }

  async deleteExpiredMessages(): Promise<number> {
    try {
      const result = await dbRun(`
        DELETE FROM queue_messages
        WHERE expiresAt < datetime('now')
      `);

      const changes = result.changes || 0;
      if (changes > 0) {
        logger.info({ deletedCount: changes }, '[QUEUE] Expired messages deleted');
      }
      return changes;
    } catch (err) {
      logger.error({ err }, '[QUEUE] Failed to delete expired messages');
      return 0;
    }
  }

  async getStats(): Promise<{ total: number; pending: number; processing: number; completed: number; failed: number }> {
    try {
      const result = await dbGet(`
        SELECT
          COUNT(*) as total,
          SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
          SUM(CASE WHEN status = 'processing' THEN 1 ELSE 0 END) as processing,
          SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed,
          SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
        FROM queue_messages
      `);

      return {
        total: result?.total || 0,
        pending: result?.pending || 0,
        processing: result?.processing || 0,
        completed: result?.completed || 0,
        failed: result?.failed || 0,
      };
    } catch (err) {
      logger.error({ err }, '[QUEUE] Failed to get queue stats');
      return { total: 0, pending: 0, processing: 0, completed: 0, failed: 0 };
    }
  }

  private rowToQueueMessage(row: any): QueueMessage {
    return {
      id: row.id,
      type: row.type as QueueMessageType,
      status: row.status as QueueMessageStatus,
      source: row.source,
      targetChannel: row.targetChannel,
      payload: JSON.parse(row.payload),
      event: row.event ? JSON.parse(row.event) : undefined,
      error: row.error,
      retryCount: row.retryCount,
      maxRetries: row.maxRetries,
      createdAt: row.createdAt,
      updatedAt: row.updatedAt,
      expiresAt: row.expiresAt,
    };
  }
}

// Singleton instance
let queueInstance: MessageQueue | null = null;

export function initializeMessageQueue(
  maxRetries?: number,
  messageTtlMs?: number
): MessageQueue {
  if (!queueInstance) {
    queueInstance = new MessageQueue(maxRetries, messageTtlMs);
  }
  return queueInstance;
}

export function getMessageQueue(): MessageQueue {
  if (!queueInstance) {
    throw new Error('Message queue not initialized. Call initializeMessageQueue first.');
  }
  return queueInstance;
}
