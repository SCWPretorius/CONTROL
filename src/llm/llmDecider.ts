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

async function callOllama(
  model: string,
  prompt: string,
  timeoutMs: number
): Promise<string> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const response = await fetch(`${config.ollama.url}/api/generate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        model,
        prompt,
        stream: false,
        options: { temperature: 0.1, num_predict: 512 },
      }),
      signal: controller.signal,
    });

    if (!response.ok) {
      throw new Error(`Ollama returned ${response.status}`);
    }

    const data = await response.json() as { response: string };
    return data.response;
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
    const raw = await callOllama(
      config.ollama.primaryModel,
      prompt,
      config.ollama.primaryTimeoutMs
    );
    const decision = validateLLMOutput(raw, availableSkills);
    if (decision) {
      currentStatus = 'primary-ok';
      return { decision, model: config.ollama.primaryModel, status: 'success' };
    }
  } catch (err) {
    logger.warn({ err }, `[LLM-PRIMARY-DOWN] ${config.ollama.primaryModel} unreachable → falling back`);
  }

  for (let i = 0; i < config.ollama.fallbackModels.length; i++) {
    const fb = config.ollama.fallbackModels[i];
    if (!fb) continue;;
    try {
      const raw = await callOllama(fb.model, prompt, fb.timeoutMs);
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
