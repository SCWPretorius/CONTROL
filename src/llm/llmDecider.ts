import { NormalizedEvent, LLMDecision, SkillDefinition } from '../types/index.js';
import { RetrievedContext } from '../memory/contextRetriever.js';
import { buildPrompt } from './promptBuilder.js';
import { validateLLMOutput } from './sanitizer.js';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';

type LLMStatus =
  | 'primary-ok'
  | 'fallback-1'
  | 'fallback-2'
  | 'deterministic'
  | 'unavailable';

let currentStatus: LLMStatus = 'primary-ok';

export function getLLMStatus(): LLMStatus {
  return currentStatus;
}

async function callLMStudio(
  model: string,
  prompt: string,
  timeoutMs: number
): Promise<string> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const response = await fetch(`${config.lmstudio.url}/chat/completions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        model,
        messages: [
          { role: 'system', content: 'You are CONTROL, a focused personal assistant. Output ONLY valid JSON, no other text.' },
          { role: 'user', content: prompt },
        ],
        temperature: 0.1,
        max_tokens: 1024,
      }),
      signal: controller.signal,
    });

    if (!response.ok) {
      const text = await response.text().catch(() => 'Unable to read response');
      logger.error({ status: response.status, text }, `[LLM] API returned error`);
      throw new Error(`LM Studio returned ${response.status}`);
    }

    const data = await response.json() as {
      choices?: Array<{ message?: { content?: string } }>;
    };

    const content = data.choices?.[0]?.message?.content ?? '';
    
    if (!content) {
      logger.warn({ data }, '[LLM] API returned empty content');
    }

    return content;
  } finally {
    clearTimeout(timer);
  }
}

function deterministicMatch(
  event: NormalizedEvent,
  skills: SkillDefinition[]
): LLMDecision | null {
  const text = JSON.stringify(event).toLowerCase();

  for (const skill of skills) {
    // Only match skills that don't have required parameters (to avoid execution failures)
    const shape = (skill.paramsSchema as any).shape || {};
    const hasRequiredParams = Object.entries(shape).some(
      ([key, schema]: any) => !schema._def?.optional && schema._def?.typeName !== 'ZodOptional'
    );
    
    if (hasRequiredParams) {
      // Skip skills with required params in deterministic mode
      continue;
    }

    const nameMatch = text.includes(skill.name.toLowerCase());
    const tagMatch = skill.tags.some(t => text.includes(t.toLowerCase()));
    if (nameMatch || tagMatch) {
      return {
        skill: skill.name,
        params: {},
        reasoning: `[DETERMINISTIC] Matched skill "${skill.name}" by keyword/tag`,
      };
    }
  }

  return null;
}

export async function decide(
  event: NormalizedEvent,
  contexts: RetrievedContext[],
  skills: SkillDefinition[]
): Promise<{ decision: LLMDecision | null; model: string; status: string }> {
  const prompt = buildPrompt(event, contexts, skills);
  const availableSkills = skills.map(s => s.name);

  try {
    const raw = await callLMStudio(
      config.lmstudio.primaryModel,
      prompt,
      config.lmstudio.primaryTimeoutMs
    );
    logger.debug({ raw: raw.slice(0, 500) }, '[LLM] Raw response from primary model');
    const decision = validateLLMOutput(raw, availableSkills);
    if (decision) {
      currentStatus = 'primary-ok';
      return { decision, model: config.lmstudio.primaryModel, status: 'success' };
    }
  } catch (err) {
    logger.warn({ err }, `[LLM-PRIMARY-DOWN] ${config.lmstudio.primaryModel} unreachable → falling back`);
  }

  for (let i = 0; i < config.lmstudio.fallbackModels.length; i++) {
    const fb = config.lmstudio.fallbackModels[i];
    if (!fb) continue;;
    try {
      const raw = await callLMStudio(fb.model, prompt, fb.timeoutMs);
      const decision = validateLLMOutput(raw, availableSkills);
      if (decision) {
        currentStatus = i === 0 ? 'fallback-1' : 'fallback-2';
        logger.info(`[LLM-FALLBACK-ACTIVE] Using ${fb.model}`);
        return { decision, model: fb.model, status: 'fallback' };
      }
    } catch (err) {
      logger.warn({ err, model: fb.model }, '[LLM] Fallback failed');
    }
  }

  logger.warn('[LLM-DETERMINISTIC-MODE] All LLM unavailable → keyword matching only');
  currentStatus = 'deterministic';
  const decision = deterministicMatch(event, skills);
  if (decision) {
    return { decision, model: 'deterministic', status: 'deterministic' };
  }

  currentStatus = 'unavailable';
  return { decision: null, model: 'none', status: 'abstained' };
}
