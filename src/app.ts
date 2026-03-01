import './config/config.js';
import { config } from './config/config.js';
import { eventBus } from './events/eventBus.js';
import { startHeartbeat } from './heartbeat/heartbeat.js';
import { startSkillLoader } from './skills/skillLoader.js';
import { startIntegrationLoader, integrationRegistry } from './integrations/integrationLoader.js';
import { skillRegistry } from './skills/skillRegistry.js';
import { initDefaultContexts, updateHomeAssistantContext } from './memory/contextStore.js';
import { resumePendingExecutions } from './executor/skillExecutor.js';
import { scheduleMemoryCleanup } from './cleanup/memoryCleanup.js';
import { scheduleBackups } from './backup/backupManager.js';
import { retrieveContexts } from './memory/contextRetriever.js';
import { decide } from './llm/llmDecider.js';
import { executeDecision } from './executor/skillExecutor.js';
import { writeTrace, buildTrace } from './tracing/decisionTracer.js';
import { NormalizedEvent } from './types/index.js';
import { logger } from './logging/logger.js';
import { initializeQueueDatabase } from './queue/queueSchema.js';
import { initializeMessageQueue, getMessageQueue } from './queue/messageQueue.js';
import { startQueueWorker, setEventHandler, setMessageSender } from './queue/queueWorker.js';
import { channelRegistry } from './channels/registry.js';
import { telegramChannel } from './channels/telegram.js';
import { discordChannel } from './channels/discord.js';

import sendMessageSkill from './skills/sendMessage.js';
import queryCalendarSkill from './skills/queryCalendar.js';
import createCalendarEventSkill from './skills/createCalendarEvent.js';
import deleteCalendarEventSkill from './skills/deleteCalendarEvent.js';
import updateContextSkill from './skills/updateContext.js';
import queryHomeAssistantSkill from './skills/queryHomeAssistant.js';
import listHomeAssistantEntitiesSkill from './skills/listHomeAssistantEntities.js';
import findHomeAssistantEntitySkill from './skills/findHomeAssistantEntity.js';

import telegramIntegration from './integrations/telegram.js';
import discordIntegration from './integrations/discord.js';
import googleCalendarIntegration from './integrations/googleCalendar.js';
import homeAssistantIntegration from './integrations/homeAssistant.js';

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

      if (status === 'unavailable') {
        try {
          const queue = getMessageQueue();
          const messageId = await queue.enqueue('incoming', event.source, event.payload, event);
          logger.info({ eventId: event.id, messageId }, '[CONTROL] Message queued due to LLM unavailability');
        } catch (queueErr) {
          logger.warn({ queueErr, eventId: event.id }, '[CONTROL] Queue not available, unable to queue message');
        }
      }
      return;
    }

    if (decision.skill.startsWith('Clarify')) {
      logger.info({ eventId: event.id, clarification: decision.skill }, '[CONTROL] LLM requesting clarification');
      return;
    }

    const result = await executeDecision(decision, event);
    if (!result.success) {
      logger.error({ error: result.error, skill: decision.skill }, '[CONTROL] Skill execution failed');

      if (event.type === 'message' && result.error) {
        try {
          const integration = integrationRegistry.get(event.source);
          if (integration && integration.send) {
            await integration.send({
              chatId: event.payload['chatId'] as string,
              text: `❌ Error: ${result.error}`,
            });
          }
        } catch (sendErr) {
          logger.warn({ err: sendErr }, '[CONTROL] Failed to send error message');
        }
      }
    } else if (result.success && result.result) {
      logger.info({ skill: decision.skill, result: result.result }, '[CONTROL] Skill executed successfully');

      const autoResponseSkills = [
        'queryHomeAssistant',
        'findHomeAssistantEntity',
        'listHomeAssistantEntities',
        'queryCalendar',
      ];

      if (event.type === 'message' && autoResponseSkills.includes(decision.skill)) {
        try {
          const integration = integrationRegistry.get(event.source);
          if (integration && integration.send) {
            let responseText = '';

            if (decision.skill === 'findHomeAssistantEntity') {
              const data = result.result as any;
              if (data.count === 0) {
                responseText = `🔍 No entities found matching "${decision.params.query}".\n${data.suggestion || 'Try a different search term.'}`;
              } else {
                responseText = `🔍 Found ${data.count} matching entities:\n\n`;
                responseText += data.matches.map((m: any, i: number) =>
                  `${i + 1}. ${m.friendlyName}\n   ID: \`${m.entityId}\`\n   State: ${m.state}`
                ).join('\n\n');
              }
            } else if (decision.skill === 'queryHomeAssistant') {
              const data = result.result as any;
              responseText = data.results.map((r: any) =>
                `📊 ${r.entityId}\nState: ${r.state}\nUpdated: ${r.lastUpdated}`
              ).join('\n\n');
            } else if (decision.skill === 'queryCalendar') {
              const data = result.result as any;
              if (data.count === 0) {
                responseText = '📅 No events found.';
              } else {
                responseText = `📅 Found ${data.count} event(s):\n\n`;
                responseText += data.events.slice(0, 5).map((e: any, i: number) =>
                  `${i + 1}. ${e.summary}\n   ${e.start} - ${e.end}`
                ).join('\n\n');
              }
            } else {
              responseText = `✅ ${decision.skill} completed:\n${JSON.stringify(result.result, null, 2).slice(0, 500)}`;
            }

            await integration.send({
              chatId: event.payload['chatId'] as string,
              text: responseText,
            });
            logger.info({ skill: decision.skill }, '[CONTROL] Auto-response sent');
          }
        } catch (sendErr) {
          logger.warn({ err: sendErr }, '[CONTROL] Failed to send auto-response');
        }
      }
    }
  } catch (err) {
    logger.error({ err, eventId: event.id }, '[CONTROL] Event processing failed');

    try {
      const queue = getMessageQueue();
      const messageId = await queue.enqueue('incoming', event.source, event.payload, event);
      logger.info({ eventId: event.id, messageId }, '[CONTROL] Message queued due to processing error');
    } catch (queueErr) {
      logger.error({ err: queueErr, eventId: event.id }, '[CONTROL] Failed to queue message after error');
    }
  }
}

export async function initApp(): Promise<void> {
  logger.info('[CONTROL] Starting up...');
  logger.info({ adminUserIds: config.adminUserIds }, '[CONTROL] Admin users configured');

  initDefaultContexts();

  skillRegistry.registerBuiltin(sendMessageSkill);
  skillRegistry.registerBuiltin(queryCalendarSkill);
  skillRegistry.registerBuiltin(createCalendarEventSkill);
  skillRegistry.registerBuiltin(deleteCalendarEventSkill);
  skillRegistry.registerBuiltin(updateContextSkill);
  skillRegistry.registerBuiltin(queryHomeAssistantSkill);
  skillRegistry.registerBuiltin(listHomeAssistantEntitiesSkill);
  skillRegistry.registerBuiltin(findHomeAssistantEntitySkill);

  integrationRegistry.register(telegramIntegration);
  integrationRegistry.register(discordIntegration);
  integrationRegistry.register(googleCalendarIntegration);
  integrationRegistry.register(homeAssistantIntegration);

  channelRegistry.register(telegramChannel);
  channelRegistry.register(discordChannel);

  await resumePendingExecutions();

  const queueDbPath = config.queueDbPath || 'data/queue.db';
  const queueEnabled = config.queueEnabled !== false;

  if (queueEnabled) {
    try {
      await initializeQueueDatabase(queueDbPath);
      const maxRetries = config.queueMaxRetries || 5;
      const ttlMs = config.queueMessageTtlMs || 86400000;
      initializeMessageQueue(maxRetries, ttlMs);

      setEventHandler(handleEvent);

      setMessageSender(async (chatId: string, text: string, source: string) => {
        const integration = integrationRegistry.get(source);
        if (!integration || !integration.send) {
          throw new Error(`Integration ${source} does not support sending messages`);
        }
        await integration.send({ chatId, text });
      });

      const retryInterval = config.queueRetryIntervalMs || 10000;
      const cleanupInterval = config.queueCleanupIntervalMs || 3600000;
      startQueueWorker(retryInterval, cleanupInterval);

      logger.info('[CONTROL] Queue initialized and worker started');
    } catch (err) {
      logger.error({ err }, '[CONTROL] Failed to initialize queue, continuing without queue');
    }
  } else {
    logger.info('[CONTROL] Queue disabled');
  }

  startSkillLoader();
  startIntegrationLoader();

  async function loadHomeAssistantContext(): Promise<void> {
    try {
      const haIntegration = integrationRegistry.get('home-assistant');
      if (haIntegration && typeof (haIntegration as any).getAvailableEntities === 'function') {
        const entities = await (haIntegration as any).getAvailableEntities();
        if (entities && Array.isArray(entities)) {
          await updateHomeAssistantContext(entities);
          logger.info({ count: entities.length }, '[CONTROL] Home Assistant entities loaded into context');
        }
      }
    } catch (err) {
      logger.warn({ err }, '[CONTROL] Failed to load Home Assistant entities');
    }
  }

  setTimeout(() => loadHomeAssistantContext(), 2000);
  setInterval(() => loadHomeAssistantContext(), 3600000);

  startHeartbeat();
  scheduleMemoryCleanup();
  scheduleBackups();

  eventBus.on('event', handleEvent);

  logger.info('[CONTROL] Ready');
}
