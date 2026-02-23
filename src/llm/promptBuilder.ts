import { NormalizedEvent, SkillDefinition } from '../types/index.js';
import { RetrievedContext } from '../memory/contextRetriever.js';
import { sanitizeInput } from './sanitizer.js';
import { config } from '../config/config.js';
import { z } from 'zod';

function getZodTypeName(schema: z.ZodTypeAny): string {
  const typeName = (schema._def as any).typeName;
  if (typeName === 'ZodString') return 'string';
  if (typeName === 'ZodNumber') return 'number';
  if (typeName === 'ZodBoolean') return 'boolean';
  if (typeName === 'ZodArray') return 'array';
  if (typeName === 'ZodObject') return 'object';
  if (typeName === 'ZodOptional') return getZodTypeName((schema._def as any).innerType) + '?';
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
  const nowIso = new Date().toISOString();
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

  const timezone = config.timezone;
  const tzOffset = new Date().toLocaleString('en-US', { timeZone: timezone, timeZoneName: 'short' }).split(' ').pop() || 'UTC+2';
  
  // Calculate week boundaries (Monday to Sunday) with explicit day-to-date mapping
  const now = new Date();
  const dayOfWeek = now.getDay(); // 0 = Sunday, 1 = Monday, ..., 6 = Saturday
  const daysFromMonday = dayOfWeek === 0 ? 6 : dayOfWeek - 1; // Days since last Monday
  const thisWeekMonday = new Date(now);
  thisWeekMonday.setDate(now.getDate() - daysFromMonday);
  thisWeekMonday.setHours(0, 0, 0, 0);
  const thisWeekSunday = new Date(thisWeekMonday);
  thisWeekSunday.setDate(thisWeekMonday.getDate() + 6);
  thisWeekSunday.setHours(23, 59, 59, 999);
  
  // Build explicit day-to-date mapping for this week
  const dayNames = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
  const weekDayMapping = dayNames.map((name, idx) => {
    const d = new Date(thisWeekMonday);
    d.setDate(thisWeekMonday.getDate() + idx);
    const dateStr = d.toISOString().split('T')[0];
    return `${name}: ${dateStr}`;
  }).join(' | ');
  
  return `You are CONTROL — a focused, concise personal assistant like Jarvis.
ONLY use provided context and skills. NEVER invent facts.
If uncertain → reply: "Clarify: [question]"

Current system time (ISO 8601): ${nowIso}
User timezone: ${timezone} (${tzOffset})

IMPORTANT DATE/TIME RULES:
1. Week definition: Monday (start) to Sunday (end)
2. THIS WEEK'S DATES: ${weekDayMapping}
3. When user says a day name (e.g., "Thursday"), use the date from the mapping above
4. "Next week" = add 7 days to the dates shown above
5. "Last week" = subtract 7 days from the dates shown above
6. Always use ISO 8601 format with timezone offset: YYYY-MM-DDTHH:MM:SS+02:00
7. Example: "5pm Thursday" = "2026-02-26T17:00:00+02:00" (NOT UTC, and match the date from the mapping)
8. Never use UTC (Z) suffix unless explicitly requested
9. For date ranges, use start of day (00:00:00) for start time and end of day (23:59:59) for end time

## Core Identity & Rules (always apply)
${coreRules || '(No core rules loaded)'}

## Relevant Retrieved Memory
${retrievedMemory || '(No additional memory retrieved)'}

## Previous Skill Execution (context for follow-up actions)
${(() => {
  const last = event.payload?.lastSkillResult as any;
  if (!last) return '(None - this is the first message)';
  return `Last skill executed: ${last.skill}
Result: ${JSON.stringify(last.result, null, 2).slice(0, 1000)}

IMPORTANT: Use this data to refer to items from the previous result. For example:
- If user says "delete the first one", use deleteCalendarEvent with the event's ID from the result above
- If user says "delete Drinks with Daniel event on Thursday", use deleteCalendarEvent with eventTitle + start/end dates (search method)
- If user says "show me more", you already have the data from the previous query
- If user asks about "that event", refer to the most recent result`;
})()}

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
