import { v4 as uuidv4 } from 'uuid';
import { NormalizedEvent, LLMDecision, ExecutionRecord, SkillExecutionError } from '../types/index.js';
import { skillRegistry } from '../skills/skillRegistry.js';
import { checkPermission, recordUsage } from '../permissions/rbac.js';
import { acquireSkillSlot } from '../concurrency/rateLimiter.js';
import { saveExecution } from './executionStore.js';
import { hashIdempotencyKey } from '../config/secrets.js';
import { logger } from '../logging/logger.js';

const MAX_RETRIES = 3;
const DEFAULT_TIMEOUT_MS = 30000;

async function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return Promise.race([
    promise,
    new Promise<never>((_, reject) =>
      setTimeout(() => reject(new Error(`Skill execution timed out after ${ms}ms`)), ms)
    ),
  ]);
}

export async function executeDecision(
  decision: LLMDecision,
  event: NormalizedEvent,
  retries = 0
): Promise<{ success: boolean; result?: unknown; error?: string }> {
  const { skill: skillName, params } = decision;

  const perm = checkPermission(skillName, event);
  if (!perm.allowed) {
    logger.warn({ skillName, reason: perm.reason }, '[EXECUTOR] Permission denied');
    return { success: false, error: perm.reason };
  }

  const skill = skillRegistry.get(skillName);
  if (!skill) {
    logger.warn({ skillName }, '[EXECUTOR] Skill not found');
    return { success: false, error: `Skill not found: ${skillName}` };
  }

  const idempotencyKey = hashIdempotencyKey(skillName, params, event.userId ?? 'anonymous');
  const execId = uuidv4();

  const record: ExecutionRecord = {
    id: execId,
    skillName,
    params,
    userId: event.userId ?? 'anonymous',
    idempotencyKey,
    status: 'pending',
    retries,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  };
  saveExecution(record);

  const release = await acquireSkillSlot(skillName);

  try {
    record.status = 'in-progress';
    record.updatedAt = new Date().toISOString();
    saveExecution(record);

    const timeoutMs = skill.definition.timeoutMs ?? DEFAULT_TIMEOUT_MS;
    const result = await withTimeout(skill.execute(params, event), timeoutMs);

    record.status = 'completed';
    record.result = result;
    record.updatedAt = new Date().toISOString();
    saveExecution(record);

    recordUsage(skillName, event);
    
    // Store result in event payload for next LLM decision to reference
    event.payload.lastSkillResult = {
      skill: skillName,
      result,
      timestamp: new Date().toISOString(),
    };
    
    logger.info({ skillName, execId }, '[EXECUTOR] Skill executed successfully');
    return { success: true, result };
  } catch (err) {
    const error = err instanceof Error ? err.message : String(err);
    const isNonRetryable = err instanceof SkillExecutionError && !err.retryable;
    
    logger.error({ err, skillName, execId, retries, isNonRetryable }, '[EXECUTOR] Skill execution failed');

    if (!isNonRetryable && retries < MAX_RETRIES - 1) {
      record.status = 'pending';
      record.retries++;
      record.updatedAt = new Date().toISOString();
      saveExecution(record);
      const backoff = Math.min(1000 * 2 ** retries, 30000);
      await new Promise(r => setTimeout(r, backoff));
      return executeDecision(decision, event, retries + 1);
    }

    record.status = error.includes('timed out') ? 'timeout' : 'failed';
    record.error = error;
    record.updatedAt = new Date().toISOString();
    saveExecution(record);

    return { success: false, error };
  } finally {
    release();
  }
}

export async function resumePendingExecutions(): Promise<void> {
  const { getPendingExecutions } = await import('./executionStore.js');
  const pending = getPendingExecutions();
  logger.info({ count: pending.length }, '[EXECUTOR] Resuming pending executions on startup');
  for (const record of pending) {
    if (record.retries >= MAX_RETRIES) {
      logger.warn({ id: record.id }, '[EXECUTOR] Skipping exhausted execution');
    }
  }
}
