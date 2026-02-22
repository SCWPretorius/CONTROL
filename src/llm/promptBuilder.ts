import { NormalizedEvent, SkillDefinition } from '../types/index.js';
import { RetrievedContext } from '../memory/contextRetriever.js';
import { sanitizeInput } from './sanitizer.js';
import { z } from 'zod';

function getZodTypeName(schema: z.ZodTypeAny): string {
  const typeName = schema._def.typeName;
  if (typeName === 'ZodString') return 'string';
  if (typeName === 'ZodNumber') return 'number';
  if (typeName === 'ZodBoolean') return 'boolean';
  if (typeName === 'ZodArray') return 'array';
  if (typeName === 'ZodObject') return 'object';
  if (typeName === 'ZodOptional') return getZodTypeName(schema._def.innerType) + '?';
  return 'any';
}

function formatSkillParams(paramsSchema: z.ZodObject<any>): string {
  const shape = paramsSchema.shape;
  const params = Object.entries(shape).map(([key, schema]) => {
    const zodSchema = schema as z.ZodTypeAny;
    const type = getZodTypeName(zodSchema);
    const description = zodSchema.description || '';
    return `    ${key}: ${type}${description ? ` - ${description}` : ''}`;
  });
  return params.join('\n');
}

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
    .map(s => `- ${s.name}: ${s.description}\n${formatSkillParams(s.paramsSchema)}`)
    .join('\n\n');

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

**CRITICAL**: Your response must be ONLY a single valid JSON object, nothing else. No explanations, no thinking out loud.
Use correct types: strings in "quotes", booleans as true/false, numbers without quotes.

JSON OUTPUT:
{ "skill": "<name>", "params": {...}, "reasoning": "..." }`;
}
