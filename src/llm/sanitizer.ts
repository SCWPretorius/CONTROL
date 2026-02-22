import { z } from 'zod';
import { LLMDecision } from '../types/index.js';
import { logger } from '../logging/logger.js';

const MAX_INPUT_LENGTH = 2048;
const MAX_PARAM_SIZE_BYTES = 10 * 1024;

const INJECTION_PATTERNS = [
  /ignore\s+previous/i,
  /instead\s+do/i,
  /forget\s+about/i,
  /new\s+instructions/i,
  /disregard\s+all/i,
  /you\s+are\s+now/i,
  /act\s+as\s+if/i,
];

const CONTROL_CHAR_REGEX = /[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g;

export function sanitizeInput(text: string): string {
  let sanitized = text.replace(CONTROL_CHAR_REGEX, '');
  sanitized = sanitized.slice(0, MAX_INPUT_LENGTH);
  for (const pattern of INJECTION_PATTERNS) {
    if (pattern.test(sanitized)) {
      logger.warn({ pattern: pattern.source }, '[SANITIZER] Injection pattern detected');
      sanitized = sanitized.replace(pattern, '[BLOCKED]');
    }
  }
  return sanitized;
}

const decisionSchema = z.object({
  skill: z.string().min(1),
  params: z.record(z.string(), z.unknown()),
  reasoning: z.string(),
});

export function validateLLMOutput(
  raw: string,
  availableSkills: string[]
): LLMDecision | null {
  let parsed: unknown;
  try {
    const jsonMatch = raw.match(/\{[\s\S]*\}/);
    if (!jsonMatch) {
      logger.warn('[LLM] No JSON found in response');
      return null;
    }
    parsed = JSON.parse(jsonMatch[0]);
  } catch {
    logger.warn({ raw: raw.slice(0, 200) }, '[LLM] Failed to parse JSON response');
    return null;
  }

  const result = decisionSchema.safeParse(parsed);
  if (!result.success) {
    logger.warn({ error: result.error.message }, '[LLM] Invalid decision schema');
    return null;
  }

  const decision = result.data;

  if (!availableSkills.includes(decision.skill)) {
    logger.warn({ skill: decision.skill }, '[LLM] Unknown skill in decision');
    return null;
  }

  const paramSize = JSON.stringify(decision.params).length;
  if (paramSize > MAX_PARAM_SIZE_BYTES) {
    logger.warn({ paramSize }, '[LLM] Params exceed max size');
    return null;
  }

  return {
    skill: decision.skill,
    params: decision.params,
    reasoning: decision.reasoning,
  };
}
