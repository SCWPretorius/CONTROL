import { v4 as uuidv4 } from 'uuid';
import { decide } from '../llm/llmDecider.js';
import { executeDecision } from '../executor/skillExecutor.js';
import { retrieveContexts } from '../memory/contextRetriever.js';
import { skillRegistry } from '../skills/skillRegistry.js';
import { evaluateSourcePolicy, evaluateToolPolicy } from './policy-evaluator.js';
import { recordUsage } from '../permissions/rbac.js';
import { logger } from '../logging/logger.js';
import type { AgentRunOptions, AgentBlock } from './types.js';
import type { NormalizedEvent } from '../types/index.js';

// Skills that signal the end of the reasoning loop
const TERMINAL_SKILLS = new Set([
  'sendMessage',
  'queryCalendar',
  'queryHomeAssistant',
  'listHomeAssistantEntities',
  'selectEntityFromOptions',
  'createCalendarEvent',
  'deleteCalendarEvent',
  'updateContext',
  'controlHomeAssistant',
]);

function formatSkillResult(skillName: string, result: unknown): string | null {
  if (!result) return null;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const data = result as any;

  if (skillName === 'sendMessage') return data.text ?? null;
  if (skillName === 'queryHomeAssistant') {
    return data.results
      ?.map((r: any) => `📊 ${r.entityId}\nState: ${r.state}\nUpdated: ${r.lastUpdated}`)
      .join('\n\n') ?? null;
  }
  if (skillName === 'findHomeAssistantEntity') {
    if (data.count === 0) return `🔍 No entities found.\n${data.suggestion ?? ''}`;
    return (
      `🔍 Found ${data.count} matching entities:\n\n` +
      data.matches
        .map(
          (m: any, i: number) =>
            `${i + 1}. ${m.friendlyName}\n   ID: \`${m.entityId}\`\n   State: ${m.state}`,
        )
        .join('\n\n')
    );
  }
  if (skillName === 'queryCalendar') {
    if (data.count === 0) return '📅 No events found.';
    return (
      `📅 Found ${data.count} event(s):\n\n` +
      data.events
        .slice(0, 5)
        .map((e: any, i: number) => `${i + 1}. ${e.summary}\n   ${e.start} – ${e.end}`)
        .join('\n\n')
    );
  }
  if (skillName === 'selectEntityFromOptions') return data.message ?? null;
  if (skillName === 'createCalendarEvent') return `✅ Event created: ${data.summary ?? data.eventId ?? 'done'}`;
  if (skillName === 'deleteCalendarEvent') return '✅ Event deleted';
  if (skillName === 'updateContext') return '✅ Context updated';
  if (skillName === 'controlHomeAssistant') return `✅ ${data.message ?? 'Done'}`;
  if (skillName === 'listHomeAssistantEntities') {
    if (!Array.isArray(data.entities) || data.entities.length === 0) return '📋 No entities found.';
    return (
      `📋 ${data.entities.length} entities:\n\n` +
      data.entities
        .slice(0, 10)
        .map((e: any) => `• ${e.friendlyName ?? e.entityId} (${e.state})`)
        .join('\n')
    );
  }
  return `✅ ${skillName} completed:\n${JSON.stringify(result, null, 2).slice(0, 400)}`;
}

export async function runAgent(
  options: AgentRunOptions,
  onBlock: (block: AgentBlock) => void,
): Promise<void> {
  const sourcePolicy = evaluateSourcePolicy(options.source);
  if (!sourcePolicy.allowed) {
    onBlock({ type: 'error', message: sourcePolicy.reason ?? 'Source not allowed' });
    onBlock({ type: 'done', stepsExecuted: 0, model: 'none' });
    return;
  }

  const event: NormalizedEvent = {
    id: uuidv4(),
    traceId: uuidv4(),
    source: options.source,
    type: 'message',
    payload: { text: options.message, chatId: options.wsSessionId },
    timestamp: new Date().toISOString(),
    userId: options.userId,
    role: options.role,
  };

  const maxSteps = options.maxSteps ?? 5;
  let stepsExecuted = 0;
  let lastModel = 'none';

  try {
    for (let step = 0; step < maxSteps; step++) {
      const keywords = options.message.split(/\s+/).slice(0, 10);
      const contexts = await retrieveContexts(event.source, event.type, keywords);
      const skills = skillRegistry.getActiveDefinitions();

      const { decision, model, status } = await decide(event, contexts, skills);
      lastModel = model;
      stepsExecuted = step + 1;

      if (!decision) {
        if (status === 'unavailable') {
          onBlock({ type: 'error', message: 'LLM is currently unavailable. Please try again later.' });
        } else {
          onBlock({ type: 'text', content: "I wasn't sure how to help with that. Could you rephrase?" });
        }
        break;
      }

      // LLM requested clarification
      if (decision.skill.startsWith('Clarify')) {
        const question =
          (decision.params?.question as string) ??
          decision.skill.replace(/^Clarify:?\s*/i, '');
        onBlock({ type: 'text', content: question || 'Could you please clarify?' });
        break;
      }

      const policyResult = evaluateToolPolicy(decision.skill, event);
      if (!policyResult.allowed) {
        onBlock({ type: 'error', message: policyResult.reason ?? `Tool ${decision.skill} not allowed` });
        break;
      }

      const callId = uuidv4();

      // For sendMessage on gateway source: extract text and emit directly (no integration to call)
      if (decision.skill === 'sendMessage' && options.source === 'gateway') {
        const text = String(decision.params.text ?? '');
        onBlock({ type: 'tool-call', callId, name: decision.skill, params: decision.params });
        onBlock({ type: 'tool-result', callId, name: decision.skill, result: { sent: true, text }, success: true });
        recordUsage(decision.skill, event);
        if (text) onBlock({ type: 'text', content: text });
        break;
      }

      onBlock({ type: 'tool-call', callId, name: decision.skill, params: decision.params });

      const result = await executeDecision(decision, event);

      onBlock({
        type: 'tool-result',
        callId,
        name: decision.skill,
        result: result.result ?? null,
        success: result.success,
        error: result.error,
      });

      if (!result.success) {
        onBlock({ type: 'error', message: result.error ?? `${decision.skill} failed` });
        break;
      }

      recordUsage(decision.skill, event);

      if (TERMINAL_SKILLS.has(decision.skill)) {
        const text = formatSkillResult(decision.skill, result.result);
        if (text) onBlock({ type: 'text', content: text });
        break;
      }

      logger.debug({ step, skill: decision.skill }, '[AGENT] Intermediate step, continuing loop');
    }
  } catch (err) {
    logger.error({ err }, '[AGENT] Unexpected error in agent run');
    onBlock({ type: 'error', message: err instanceof Error ? err.message : 'Unexpected error' });
  }

  onBlock({ type: 'done', stepsExecuted, model: lastModel });
}
