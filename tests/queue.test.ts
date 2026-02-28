import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { MessageQueue } from '../src/queue/messageQueue.js';
import { initializeQueueDatabase, closeQueueDatabase } from '../src/queue/queueSchema.js';
import path from 'path';
import { fileURLToPath } from 'url';
import { rmSync, existsSync } from 'fs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

let testDbPath: string;
let queue: MessageQueue;

describe('MessageQueue', () => {
  beforeEach(async () => {
    // Use an in-memory database for tests
    testDbPath = ':memory:';
    await initializeQueueDatabase(testDbPath);
    queue = new MessageQueue(5, 86400000);
  });

  afterEach(async () => {
    try {
      await closeQueueDatabase();
    } catch (e) {
      // Ignore errors on close
    }
  });

  it('should enqueue an incoming message', async () => {
    const messageId = await queue.enqueue('incoming', 'telegram', { text: 'Hello' });
    expect(messageId).toBeTruthy();

    const message = await queue.getMessageById(messageId);
    expect(message).toBeTruthy();
    expect(message?.type).toBe('incoming');
    expect(message?.status).toBe('pending');
    expect(message?.source).toBe('telegram');
  });

  it('should enqueue an outgoing message with target', async () => {
    const messageId = await queue.enqueue(
      'outgoing',
      'discord',
      { text: 'Response' },
      undefined,
      '12345'
    );
    expect(messageId).toBeTruthy();

    const message = await queue.getMessageById(messageId);
    expect(message?.type).toBe('outgoing');
    expect(message?.targetChannel).toBe('12345');
  });

  it('should mark a message as processing', async () => {
    const messageId = await queue.enqueue('incoming', 'telegram', { text: 'Test' });
    await queue.markProcessing(messageId);

    const message = await queue.getMessageById(messageId);
    expect(message?.status).toBe('processing');
  });

  it('should mark a message as completed', async () => {
    const messageId = await queue.enqueue('incoming', 'telegram', { text: 'Test' });
    await queue.markProcessing(messageId);
    await queue.markCompleted(messageId);

    const message = await queue.getMessageById(messageId);
    expect(message?.status).toBe('completed');
  });

  it('should mark a message as failed with error', async () => {
    const messageId = await queue.enqueue('incoming', 'telegram', { text: 'Test' });
    await queue.markProcessing(messageId);
    await queue.markFailed(messageId, 'Network error');

    const message = await queue.getMessageById(messageId);
    expect(message?.status).toBe('failed');
    expect(message?.error).toBe('Network error');
  });

  it('should increment retry count', async () => {
    const messageId = await queue.enqueue('incoming', 'telegram', { text: 'Test' });

    const count1 = await queue.incrementRetry(messageId);
    expect(count1).toBe(1);

    const count2 = await queue.incrementRetry(messageId);
    expect(count2).toBe(2);

    const message = await queue.getMessageById(messageId);
    expect(message?.retryCount).toBe(2);
  });

  it('should get pending messages', async () => {
    await queue.enqueue('incoming', 'telegram', { text: 'Message 1' });
    await queue.enqueue('incoming', 'telegram', { text: 'Message 2' });
    await queue.enqueue('incoming', 'discord', { text: 'Message 3' });

    const pending = await queue.getPendingMessages(10);
    expect(pending.length).toBe(3);
    expect(pending.every(m => m.status === 'pending')).toBe(true);
  });

  it('should not return messages that exceed max retries', async () => {
    const messageId = await queue.enqueue('incoming', 'telegram', { text: 'Test' });

    // Increment retry count to max
    for (let i = 0; i < 5; i++) {
      await queue.incrementRetry(messageId);
    }

    const pending = await queue.getPendingMessages(10);
    expect(pending.length).toBe(0);
  });

  it('should get failed messages', async () => {
    const id1 = await queue.enqueue('incoming', 'telegram', { text: 'Msg 1' });
    const id2 = await queue.enqueue('incoming', 'telegram', { text: 'Msg 2' });

    await queue.markFailed(id1, 'Error 1');
    await queue.markFailed(id2, 'Error 2');

    const failed = await queue.getFailedMessages(10);
    expect(failed.length).toBe(2);
    expect(failed.every(m => m.status === 'failed')).toBe(true);
  });

  it('should delete a message', async () => {
    const messageId = await queue.enqueue('incoming', 'telegram', { text: 'Test' });
    expect(await queue.getMessageById(messageId)).toBeTruthy();

    const deleted = await queue.deleteMessage(messageId);
    expect(deleted).toBe(true);
    expect(await queue.getMessageById(messageId)).toBeNull();
  });

  it('should return queue statistics', async () => {
    await queue.enqueue('incoming', 'telegram', { text: 'Msg 1' });
    await queue.enqueue('incoming', 'telegram', { text: 'Msg 2' });
    await queue.enqueue('incoming', 'telegram', { text: 'Msg 3' });

    const stats = await queue.getStats();
    expect(stats.total).toBe(3);
    expect(stats.pending).toBe(3);
    expect(stats.completed).toBe(0);
    expect(stats.failed).toBe(0);
  });

  it('should include event data in incoming messages', async () => {
    const event = {
      id: 'evt-123',
      traceId: 'trace-123',
      source: 'telegram',
      type: 'message',
      payload: { text: 'Hello' },
      timestamp: new Date().toISOString(),
      userId: 'user-1',
    };

    const messageId = await queue.enqueue('incoming', 'telegram', event.payload, event);
    const message = await queue.getMessageById(messageId);

    expect(message?.event).toEqual(event);
  });

  it('should limit pending messages query by limit parameter', async () => {
    for (let i = 0; i < 15; i++) {
      await queue.enqueue('incoming', 'telegram', { text: `Message ${i}` });
    }

    const pending = await queue.getPendingMessages(5);
    expect(pending.length).toBe(5);
  });

  it('should get all messages with optional status filter', async () => {
    const id1 = await queue.enqueue('incoming', 'telegram', { text: 'Msg 1' });
    const id2 = await queue.enqueue('incoming', 'telegram', { text: 'Msg 2' });
    const id3 = await queue.enqueue('incoming', 'telegram', { text: 'Msg 3' });

    await queue.markCompleted(id1);
    await queue.markFailed(id2, 'Error');

    const all = await queue.getAllMessages();
    expect(all.length).toBe(3);

    const pending = await queue.getAllMessages('pending');
    expect(pending.length).toBe(1);
    expect(pending[0].id).toBe(id3);

    const completed = await queue.getAllMessages('completed');
    expect(completed.length).toBe(1);
    expect(completed[0].id).toBe(id1);
  });
});
