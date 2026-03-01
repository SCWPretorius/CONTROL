import { NormalizedEvent, SkillDefinition } from '../types/index.js';
import { RetrievedContext } from '../memory/contextRetriever.js';
import { sanitizeInput } from './sanitizer.js';
import { config } from '../config/config.js';
import { z } from 'zod';
import { cacheEntitySelection } from '../memory/entitySelectionCache.js';

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
  let other = contexts.filter(c => c.method !== 'always');

  // Truncate Home Assistant entities context if it's too large (limit to ~1500 chars)
  const updatedHighPriority = highPriority.map(c => {
    if (c.context.label === 'home-assistant-entities' && c.context.content.length > 1500) {
      const truncated = c.context.content.slice(0, 1500) + '\n\n(... see listHomeAssistantEntities skill for more)';
      return { ...c, context: { ...c.context, content: truncated } };
    }
    return c;
  });

  const coreRules = updatedHighPriority
    .map(c => `[Label: ${c.context.label}]\n${c.context.content}`)
    .join('\n\n');

  let retrievedMemory = other
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
  
  // Special handling for selectEntityFromOptions result (user just picked an option number)
  if (last.skill === 'selectEntityFromOptions' && last.result?.candidates) {
    // Check if current user message is just a number (1, 2, or 3)
    const userMsg = (event.payload?.text as string ?? '').trim();
    const selectedNum = parseInt(userMsg, 10);
    
    if (!isNaN(selectedNum) && selectedNum >= 1 && selectedNum <= (last.result.candidates?.length || 0)) {
      const candidates = last.result.candidates as any[];
      const selected = candidates[selectedNum - 1];
      if (selected) {
        // Cache the entity selection for future use
        try {
          // Extract query from original selectEntityFromOptions params
          const originalQuery = ((event.payload?.text as string) || '').trim();
          cacheEntitySelection(
            originalQuery || 'entity selection',
            selected.entityId,
            selected.friendlyName,
            selected.domain,
            selected.confidence || 100
          );
        } catch (err) {
          // Ignore caching errors
        }
        
        return `ENTITY SELECTION DETECTED: User picked option ${selectedNum}
Selected Entity: **${selected.friendlyName}** 
Entity ID: ${selected.entityId}
Domain: ${selected.domain}
Confidence: ${selected.confidence || 'N/A'}%

ACTION: Use queryHomeAssistant with entityIds: "${selected.entityId}" to get the current state.
The entity has been cached for future reference to this query.`;
      }
    }
  }
  
  // Special handling for ambiguous entity search results
  if (last.skill === 'findHomeAssistantEntity' && (last.result?.isAmbiguous === true || (last.result?.matches?.length ?? 0) > 1)) {
    return `Last skill executed: ${last.skill} — IMPORTANT: AMBIGUOUS RESULTS DETECTED
Result: Multiple entities matched the user's query

Matches found:
${(last.result.matches || []).map((m: any, i: number) => 
  `  (${i + 1}) ${m.friendlyName} [${m.confidence || '?'}%] - domain: ${m.domain} - ID: ${m.entityId}`
).join('\n')}

ACTION REQUIRED: Call selectEntityFromOptions with these candidates so the user can pick which entity they meant.
Extract matches into the candidates parameter and include the user's original query.

Otherwise, if there is only ONE match with high confidence (>75%), use queryHomeAssistant with that entity ID.`;
  }
  
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

## Home Assistant Entity Selection Reasoning
IMPORTANT: When selecting Home Assistant entities, reason through these steps:

STEP 1: UNDERSTAND USER INTENT
- What state is the user asking about? (armed/disarmed, on/off, motion detected, temperature, etc.)
- What area or device did they mention? (driveway, kitchen, bedroom, etc.)
- Are they asking about a control (partition, switch) or a sensor (motion, door)?

STEP 2: SEMANTIC ANALYSIS
- Armed/Disarmed states → Look for: partitions, switches with armed capability, binary_sensors with armed_disarmed device class
- Motion/Movement → Look for: binary_sensors with motion device class
- Temperature → Look for: sensors with temperature device class
- On/Off states → Look for: switches, lights, input_boolean entities
- Position/Opening → Look for: covers (doors, shutters)

STEP 3: ENTITY RELATIONSHIP REASONING
- If user asks "Are driveway beams armed?" the entity might be:
  - Direct entity: "binary_sensor.driveway_beams_armed"
  - Control entity: "switch.driveway_partition" (the partition that controls those beams)
  - A partition controls multiple sensors in an area
  - Prefer the CONTROL entity (partition/switch) over sensors if asking about "armed" state

STEP 4: SEARCH AND CONFIDENCE
When calling findHomeAssistantEntity:
  - Return results indicate: entityId, friendlyName, domain, deviceClass, confidence %
  - Confidence > 75% means high confidence - can use directly
  - Confidence 50-75% means ambiguous - consider selectEntityFromOptions
  - Confidence < 50% means poor match - try different search terms or ask user for clarification
  - Multiple results with similar confidence (within 15%) = ambiguous situation

STEP 5: DISAMBIGUATE IF NEEDED
If findHomeAssistantEntity returns multiple candidates with similar confidence:
- Use selectEntityFromOptions to present top 2-3 options to the user
- Include confidence scores and entity types to help user decide
- Explain the reasoning: "Option 1 is a partition (controls the beams), Option 2 is a motion sensor"
- Wait for user to confirm which entity they meant

STEP 6: EXECUTE THE QUERY
Once you have an entity ID with high confidence, use queryHomeAssistant to get its state.

## Workflow Examples
Example 1: "Is the driveway armed?"
1. Intent: User asking about armed/disarmed state
2. Semantic: "armed" keywords + "driveway" area → look for armed_disarmed entities
3. Search: findHomeAssistantEntity(query: "driveway armed") [no domain filter!]
4. Results: partition.driveway (95%), binary_sensor.driveway_beams (60%)
5. Action: Confidence is high (95%) → Use partition directly → queryHomeAssistant(entityIds: "partition.driveway")

Example 2: "Are the driveway beams armed?"
1. Intent: User asking about a specific sensor's state, but context is "armed" (not typical for sensors)
2. Semantic: "beams" is a sensor component, but "armed" is a partition state → user probably means the partition
3. Search: findHomeAssistantEntity(query: "driveway beams armed") → find both
4. Results: partition.driveway (88%), sensor.driveway_beams (72%)
5. Action: Results are ambiguous (12% difference) → selectEntityFromOptions to confirm → wait for user

Example 3: "What's the kitchen temperature?"
1. Intent: User asking for temperature reading
2. Semantic: "temperature" is a sensor measurement value
3. Search: findHomeAssistantEntity(query: "kitchen temperature") → find sensors
4. Result: sensor.kitchen_temperature (95%) - single high-confidence result
5. Action: Use sensor directly → queryHomeAssistant(entityIds: "sensor.kitchen_temperature")

## Core Rule for Ambiguity
- NEVER guess between multiple equally-likely entities
- ALWAYS use selectEntityFromOptions when confidence < 75% OR multiple results within 15% confidence
- This ensures user experience is clear and mapping is cached for future requests

## Available Workflow
- queryHomeAssistant: Query exact entity state when you have the entity ID
- findHomeAssistantEntity: Search for entity by name when you don't know the exact ID
- selectEntityFromOptions: Ask user to pick from multiple candidates (when ambiguous)
- listHomeAssistantEntities: Browse all entities in a specific domain

Think step-by-step:
1. What does the user want?
2. Is this about Home Assistant?
3. If YES: Do I know the exact entity ID?
   - If YES → queryHomeAssistant directly (use full ID: "domain.entity_name")
   - If NO → findHomeAssistantEntity to search (let ranking + confidence guide you)
4. If findHomeAssistantEntity returns ambiguous results (< 75% confidence OR multiple similar matches)
   → Use selectEntityFromOptions to ask user
5. If unsure about anything → Clarify: [question]

**CRITICAL**: Your response must be ONLY a single valid JSON object, nothing else. No explanations, no thinking out loud.
Use correct types: strings in "quotes", booleans as true/false, numbers without quotes.

JSON OUTPUT:
{ "skill": "<name>", "params": {...}, "reasoning": "..." }`;
}
