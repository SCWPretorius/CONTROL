import { initApp } from './app.js';
import { startServer } from './server/server.js';
import { logger } from './logging/logger.js';

async function main(): Promise<void> {
  await initApp();
  startServer();
}

main().catch(err => {
  logger.fatal({ err }, '[CONTROL] Fatal startup error');
  process.exit(1);
});
