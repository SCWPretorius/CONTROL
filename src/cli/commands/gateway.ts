import { initApp } from '../../app.js';
import { startGateway, stopGateway } from '../../gateway/server.impl.js';
import { logger } from '../../logging/logger.js';

export async function runGateway(): Promise<void> {
  await initApp();
  startGateway();

  for (const sig of ['SIGINT', 'SIGTERM'] as NodeJS.Signals[]) {
    process.once(sig, async () => {
      logger.info({ signal: sig }, '[CLI] Received signal, shutting down...');
      await stopGateway();
      process.exit(0);
    });
  }
}
