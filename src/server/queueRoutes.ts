import { Router, Request, Response } from 'express';
import { getMessageQueue } from '../queue/messageQueue.js';
import { logger } from '../logging/logger.js';
import { QueueMessageStatus } from '../types/index.js';

const router = Router();

// Middleware to require admin role (can be enhanced with proper RBAC)
function requireAdmin(req: Request, res: Response, next: Function): void {
  const role = (req as any).user?.role;
  if (role !== 'admin') {
    res.status(403).json({ error: 'Admin access required' });
    return;
  }
  next();
}

/**
 * GET /queue/status
 * Get overall queue statistics
 */
router.get('/queue/status', requireAdmin, async (req: Request, res: Response) => {
  try {
    const queue = getMessageQueue();
    const stats = await queue.getStats();

    res.json({
      success: true,
      data: stats,
    });
  } catch (err) {
    logger.error({ err }, '[QUEUE:ROUTES] Error getting queue status');
    res.status(500).json({ error: 'Failed to get queue status' });
  }
});

/**
 * GET /queue/pending
 * List pending messages
 */
router.get('/queue/pending', requireAdmin, async (req: Request, res: Response) => {
  try {
    const queue = getMessageQueue();
    const limit = Math.min(parseInt(req.query.limit as string) || 50, 1000);
    const messages = await queue.getPendingMessages(limit);

    res.json({
      success: true,
      count: messages.length,
      data: messages,
    });
  } catch (err) {
    logger.error({ err }, '[QUEUE:ROUTES] Error getting pending messages');
    res.status(500).json({ error: 'Failed to get pending messages' });
  }
});

/**
 * GET /queue/failed
 * List failed messages
 */
router.get('/queue/failed', requireAdmin, async (req: Request, res: Response) => {
  try {
    const queue = getMessageQueue();
    const limit = Math.min(parseInt(req.query.limit as string) || 100, 1000);
    const messages = await queue.getFailedMessages(limit);

    res.json({
      success: true,
      count: messages.length,
      data: messages,
    });
  } catch (err) {
    logger.error({ err }, '[QUEUE:ROUTES] Error getting failed messages');
    res.status(500).json({ error: 'Failed to get failed messages' });
  }
});

/**
 * GET /queue/all
 * List all messages with optional status filter
 */
router.get('/queue/all', requireAdmin, async (req: Request, res: Response) => {
  try {
    const queue = getMessageQueue();
    const status = req.query.status as QueueMessageStatus | undefined;
    const limit = Math.min(parseInt(req.query.limit as string) || 100, 1000);
    const messages = await queue.getAllMessages(status, limit);

    res.json({
      success: true,
      count: messages.length,
      filter: { status: status || 'none' },
      data: messages,
    });
  } catch (err) {
    logger.error({ err }, '[QUEUE:ROUTES] Error getting all messages');
    res.status(500).json({ error: 'Failed to get messages' });
  }
});

/**
 * GET /queue/:messageId
 * Get a specific message by ID
 */
router.get('/queue/:messageId', requireAdmin, async (req: Request, res: Response) => {
  const messageId = req.params.messageId as string;
  try {
    const queue = getMessageQueue();
    const message = await queue.getMessageById(messageId);

    if (!message) {
      res.status(404).json({ error: 'Message not found' });
      return;
    }

    res.json({
      success: true,
      data: message,
    });
  } catch (err) {
    logger.error({ err, messageId }, '[QUEUE:ROUTES] Error getting message');
    res.status(500).json({ error: 'Failed to get message' });
  }
});

/**
 * POST /queue/retry/:messageId
 * Retry processing a specific message (move from failed to pending)
 */
router.post('/queue/retry/:messageId', requireAdmin, async (req: Request, res: Response) => {
  const messageId = req.params.messageId as string;
  try {
    const queue = getMessageQueue();
    const message = await queue.getMessageById(messageId);

    if (!message) {
      res.status(404).json({ error: 'Message not found' });
      return;
    }

    if (message.status === 'pending') {
      res.status(400).json({ error: 'Message is already pending' });
      return;
    }

    // Reset retry count and mark as pending
    const { dbRun } = await import('../queue/queueSchema.js');
    await dbRun(
      `
      UPDATE queue_messages
      SET status = 'pending', retryCount = 0, error = NULL, updatedAt = ?
      WHERE id = ?
    `,
      [new Date().toISOString(), messageId]
    );

    logger.info(
      { messageId },
      '[QUEUE:ROUTES] Message marked for retry'
    );

    res.json({
      success: true,
      message: 'Message queued for retry',
      data: { messageId },
    });
  } catch (err) {
    logger.error({ err, messageId }, '[QUEUE:ROUTES] Error retrying message');
    res.status(500).json({ error: 'Failed to retry message' });
  }
});

/**
 * POST /queue/retry-all
 * Retry all failed messages
 */
router.post('/queue/retry-all', requireAdmin, async (req: Request, res: Response) => {
  try {
    const queue = getMessageQueue();
    const failed = await queue.getFailedMessages(10000); // Get all failed messages

    const { dbRun } = await import('../queue/queueSchema.js');
    await dbRun(
      `
      UPDATE queue_messages
      SET status = 'pending', retryCount = 0, error = NULL, updatedAt = ?
      WHERE status = 'failed'
    `,
      [new Date().toISOString()]
    );

    logger.info(
      { count: failed.length },
      '[QUEUE:ROUTES] All failed messages marked for retry'
    );

    res.json({
      success: true,
      message: `${failed.length} messages queued for retry`,
      data: { retryCount: failed.length },
    });
  } catch (err) {
    logger.error({ err }, '[QUEUE:ROUTES] Error retrying all messages');
    res.status(500).json({ error: 'Failed to retry messages' });
  }
});

/**
 * DELETE /queue/:messageId
 * Delete a specific message from queue
 */
router.delete('/queue/:messageId', requireAdmin, async (req: Request, res: Response) => {
  try {
    const queue = getMessageQueue();
    const messageId = req.params.messageId as string;
    const deleted = await queue.deleteMessage(messageId);

    if (!deleted) {
      res.status(404).json({ error: 'Message not found' });
      return;
    }

    logger.info({ messageId }, '[QUEUE:ROUTES] Message deleted');

    res.json({
      success: true,
      message: 'Message deleted',
    });
  } catch (err) {
    logger.error({ err, messageId: req.params.messageId as string }, '[QUEUE:ROUTES] Error deleting message');
    res.status(500).json({ error: 'Failed to delete message' });
  }
});

/**
 * DELETE /queue
 * Delete all expired messages
 */
router.delete('/queue', requireAdmin, async (req: Request, res: Response) => {
  try {
    const queue = getMessageQueue();
    const count = await queue.deleteExpiredMessages();

    logger.info({ deletedCount: count }, '[QUEUE:ROUTES] Expired messages deleted');

    res.json({
      success: true,
      message: `${count} expired messages deleted`,
      data: { deletedCount: count },
    });
  } catch (err) {
    logger.error({ err }, '[QUEUE:ROUTES] Error deleting expired messages');
    res.status(500).json({ error: 'Failed to delete expired messages' });
  }
});

export default router;
