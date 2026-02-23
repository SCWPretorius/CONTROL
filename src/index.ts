import './config/config.js';
import { config } from './config/config.js';
import { eventBus } from './events/eventBus.js';
import { startServer } from './server/server.js';
import { startHeartbeat } from './heartbeat/heartbeat.js';
import { startSkillLoader } from './skills/skillLoader.js';
import { startIntegrationLoader, integrationRegistry } from './integrations/integrationLoader.js';
import { skillRegistry } from './skills/skillRegistry.js';
import { initDefaultContexts } from './memory/contextStore.js';
import { resumePendingExecutions } from './executor/skillExecutor.js';
import { scheduleMemoryCleanup } from './cleanup/memoryCleanup.js';
import { scheduleBackups } from './backup/backupManager.js';
import { retrieveContexts } from './memory/contextRetriever.js';
import { decide } from './llm/llmDecider.js';
import { executeDecision } from './executor/skillExecutor.js';
import { writeTrace, buildTrace } from './tracing/decisionTracer.js';
import { NormalizedEvent } from './types/index.js';
import { logger } from './logging/logger.js';

import sendMessageSkill from './skills/sendMessage.js';
import queryCalendarSkill from './skills/queryCalendar.js';
import createCalendarEventSkill from './skills/createCalendarEvent.js';
import deleteCalendarEventSkill from './skills/deleteCalendarEvent.js';
import updateContextSkill from './skills/updateContext.js';

import telegramIntegration from './integrations/telegram.js';
import discordIntegration from './integrations/discord.js';
import googleCalendarIntegration from './integrations/googleCalendar.js';

async function handleEvent(event: NormalizedEvent): Promise<void> {
  const startTime = Date.now();
  logger.info({ eventId: event.id, source: event.source, type: event.type }, '[CONTROL] Processing event');

  try {
    const keywords = event.type === 'message'
      ? ((event.payload['text'] as string) ?? '').split(/\s+/).slice(0, 10)
      : [event.type];

    const contexts = await retrieveContexts(event.source, event.type, keywords);
    const skills = skillRegistry.getActiveDefinitions();
    const { decision, model, status } = await decide(event, contexts, skills);

    const executionTimeMs = Date.now() - startTime;

    const trace = buildTrace({
      traceId: event.traceId,
      event,
      retrievedContexts: contexts.map(c => ({ label: c.context.label, score: c.score })),
      llmPrompt: '(see promptBuilder)',
      llmResponse: decision ? JSON.stringify(decision) : 'null',
      decision,
      llmModel: model,
      executionTimeMs,
      status: status === 'success' ? 'success' : status === 'abstained' ? 'abstained' : 'fallback',
    });
    writeTrace(trace);

    if (!decision) {
      logger.info({ eventId: event.id }, '[CONTROL] No decision made (abstained)');
      return;
    }

    const result = await executeDecision(decision, event);
    if (!result.success) {
      logger.error({ error: result.error, skill: decision.skill }, '[CONTROL] Skill execution failed');
    }
  } catch (err) {
    logger.error({ err, eventId: event.id }, '[CONTROL] Event processing failed');
  }
}

async function main(): Promise<void> {
  logger.info('[CONTROL] Starting up...');
  logger.info({ adminUserIds: config.adminUserIds }, '[CONTROL] Admin users configured');

  initDefaultContexts();

  skillRegistry.registerBuiltin(sendMessageSkill);
  skillRegistry.registerBuiltin(queryCalendarSkill);
  skillRegistry.registerBuiltin(createCalendarEventSkill);
  skillRegistry.registerBuiltin(deleteCalendarEventSkill);
  skillRegistry.registerBuiltin(updateContextSkill);

  integrationRegistry.register(telegramIntegration);
  integrationRegistry.register(discordIntegration);
  integrationRegistry.register(googleCalendarIntegration);

  await resumePendingExecutions();

  startSkillLoader();
  startIntegrationLoader();

  startServer();
  startHeartbeat();

  scheduleMemoryCleanup();
  scheduleBackups();

  eventBus.on('event', handleEvent);

  logger.info('[CONTROL] Ready');
}

main().catch(err => {
  logger.fatal({ err }, '[CONTROL] Fatal startup error');
  process.exit(1);
});
