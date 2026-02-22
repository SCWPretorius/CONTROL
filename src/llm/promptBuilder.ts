import { NormalizedEvent, SkillDefinition } from '../types/index.js';
import { RetrievedContext } from '../memory/contextRetriever.js';
import { sanitizeInput } from './sanitizer.js';

export function buildPrompt(
  event: NormalizedEvent,
  contexts: RetrievedContext[],
  skills: SkillDefinition[]
): string {
  const highPriority = contexts.filter(c => c.method === 'always');
  const other = contexts.filter(c => c.method !== 'always');

  const coreRules = highPriority
    .map(c => `[Label: ${c.context.label}]\n${c.context.content}`)
    .join('\n\n');

  const retrievedMemory = other
    .map(c => `[Label: ${c.context.label}] (score: ${c.score.toFixed(2)}, method: ${c.method})\n${c.context.content}`)
    .join('\n\n');

  const skillList = skills
    .map(s => `- ${s.name}(${Object.keys(s.paramsSchema.shape).join(', ')}): ${s.description}`)
    .join('\n');

  const eventSummary = sanitizeInput(JSON.stringify(event, null, 2).slice(0, 500));

  return `You are CONTROL — a focused, concise personal assistant like Jarvis.
ONLY use provided context and skills. NEVER invent facts.
If uncertain → reply: "Clarify: [question]"

## Core Identity & Rules (always apply)
${coreRules || '(No core rules loaded)'}

## Relevant Retrieved Memory
${retrievedMemory || '(No additional memory retrieved)'}

## Current Event
${eventSummary}

## Available Skills
${skillList || '(No skills loaded)'}

Think step-by-step:
1. What does the user/event want?
2. Which contexts apply?
3. Which skill (or none) should be used? Why?
4. Parameters?
5. If unsure → Clarify: ...

Output JSON only:
{ "skill": "<name>", "params": {...}, "reasoning": "..." }`;
}
