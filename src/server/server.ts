import express, { Request, Response, NextFunction } from 'express';
import { config } from '../config/config.js';
import { eventBus } from '../events/eventBus.js';
import telegramIntegration from '../integrations/telegram.js';
import discordIntegration from '../integrations/discord.js';
import googleCalendarIntegration from '../integrations/googleCalendar.js';
import { getGoogleAuthStatus, getGoogleAuthUrl, exchangeCodeForTokens } from '../integrations/googleCalendar.js';
import { skillRegistry } from '../skills/skillRegistry.js';
import { integrationRegistry } from '../integrations/integrationLoader.js';
import { getIntegrationHealth } from '../integrations/integrationRunner.js';
import { getLLMStatus } from '../llm/llmDecider.js';
import { getVectorCount } from '../memory/vectorStore.js';
import { logger } from '../logging/logger.js';
import { approvalRouter } from './approvalRoutes.js';

const app = express();

app.use(express.json({ limit: '1mb' }));
app.use(express.text({ type: 'text/plain' }));

app.get('/health', (_req: Request, res: Response) => {
  const integrations = integrationRegistry.getAll().map(i => i.name);
  const health = getIntegrationHealth();
  const llmStatus = getLLMStatus();
  const vectorCount = getVectorCount();

  res.json({
    status: 'ok',
    timestamp: new Date().toISOString(),
    checks: {
      llmAccessible: llmStatus !== 'unavailable',
      llmStatus,
      integrationsLoaded: integrations,
      vectorStoreResponsive: true,
      vectorCount,
    },
    metrics: {
      uptime: process.uptime(),
      eventsQueueSize: eventBus.getQueueSize(),
      activeSkills: skillRegistry.getActive().length,
    },
    integrationHealth: health.map(h => ({
      name: h.name,
      quarantined: h.quarantined,
      consecutiveErrors: h.consecutiveErrors,
      lastPollAt: h.lastPollAt?.toISOString(),
    })),
  });
});

app.post('/webhooks/telegram', (req: Request, res: Response) => {
  const signature = req.headers['x-telegram-bot-api-secret-token'] as string ?? '';
  const body = JSON.stringify(req.body);
  const event = telegramIntegration.handleWebhook(body, signature);
  if (!event) {
    res.status(401).json({ error: 'Invalid signature' });
    return;
  }
  eventBus.enqueue(event);
  res.json({ ok: true });
});

app.post('/webhooks/discord', (req: Request, res: Response) => {
  const signature = req.headers['x-signature-ed25519'] as string ?? '';
  const timestamp = req.headers['x-signature-timestamp'] as string ?? '';
  const body = JSON.stringify(req.body);

  if (req.body?.type === 1) {
    res.json({ type: 1 });
    return;
  }

  const event = discordIntegration.handleWebhook(body, signature, timestamp);
  if (!event) {
    res.status(401).json({ error: 'Invalid signature' });
    return;
  }
  eventBus.enqueue(event);
  res.json({ type: 5 });
});

app.post('/webhooks/calendar', (req: Request, res: Response) => {
  const token = req.headers['x-goog-channel-token'] as string ?? '';
  const body = JSON.stringify(req.body ?? {});
  googleCalendarIntegration.handlePushWebhook(body, token);
  res.status(200).end();
});

app.get('/auth/google/start', (_req: Request, res: Response) => {
  try {
    const url = getGoogleAuthUrl();
    res.redirect(url);
  } catch (err) {
    logger.error({ err }, '[SERVER] Failed to start Google OAuth');
    res.status(500).json({ error: 'Google OAuth is not configured' });
  }
});

app.get('/auth/google/callback', async (req: Request, res: Response) => {
  const code = req.query['code'];
  if (!code || typeof code !== 'string') {
    res.status(400).json({ error: 'Missing OAuth code' });
    return;
  }
  try {
    await exchangeCodeForTokens(code);
    res.status(200).send('Google Calendar connected. You can close this window.');
  } catch (err) {
    logger.error({ err }, '[SERVER] Google OAuth callback failed');
    res.status(500).json({ error: 'Google OAuth failed' });
  }
});

app.get('/auth/google/status', (_req: Request, res: Response) => {
  res.json(getGoogleAuthStatus());
});

app.use('/api/approval', approvalRouter);

app.use((err: Error, _req: Request, res: Response, _next: NextFunction) => {
  logger.error({ err }, '[SERVER] Unhandled error');
  res.status(500).json({ error: 'Internal server error' });
});

export function startServer(): void {
  app.listen(config.port, config.host, () => {
    logger.info({ port: config.port, host: config.host }, '[SERVER] Started');
  });
}

export { app };
