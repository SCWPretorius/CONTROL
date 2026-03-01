import { logger } from '../logging/logger.js';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type HookHandler = (arg?: any) => Promise<void>;

class HookRegistry {
  private handlers = new Map<string, HookHandler[]>();

  register(type: string, handler: HookHandler): void {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, []);
    }
    this.handlers.get(type)!.push(handler);
    logger.debug({ type }, '[HOOKS] Handler registered');
  }

  async emit(type: string, arg?: unknown): Promise<void> {
    const handlers = this.handlers.get(type) ?? [];
    for (const handler of handlers) {
      try {
        await handler(arg);
      } catch (err) {
        logger.error({ err, type }, '[HOOKS] Handler error (non-fatal)');
      }
    }
  }

  getHandlerCount(type: string): number {
    return this.handlers.get(type)?.length ?? 0;
  }
}

export const hookRegistry = new HookRegistry();
