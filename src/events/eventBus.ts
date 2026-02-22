import { EventEmitter } from 'events';
import { NormalizedEvent } from '../types/index.js';
import { logger } from '../logging/logger.js';
import { config } from '../config/config.js';

class EventBus extends EventEmitter {
  private queue: NormalizedEvent[] = [];
  private processing = false;
  private queueMaxSize = config.concurrency.maxQueueSize;

  constructor() {
    super();
    this.setMaxListeners(100);
  }

  enqueue(event: NormalizedEvent): void {
    if (this.queue.length >= this.queueMaxSize) {
      logger.warn({ eventId: event.id }, '[EVENT-BUS] Queue overflow - dropping event');
      this.emit('queue-overflow', event);
      return;
    }
    this.queue.push(event);
    logger.debug({ eventId: event.id, source: event.source }, '[EVENT-BUS] Event enqueued');
    this.processQueue();
  }

  private processQueue(): void {
    if (this.processing || this.queue.length === 0) return;
    this.processing = true;
    const event = this.queue.shift()!;
    this.emit('event', event);
    this.processing = false;
    if (this.queue.length > 0) {
      setImmediate(() => this.processQueue());
    }
  }

  getQueueSize(): number {
    return this.queue.length;
  }
}

export const eventBus = new EventBus();
