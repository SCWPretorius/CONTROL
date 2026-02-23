import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import { createCalendarEvent } from '../integrations/googleCalendar.js';
import { integrationRegistry } from '../integrations/integrationLoader.js';
import { config } from '../config/config.js';

// Dynamically extract all calendar keys from config
const calendarKeys = Object.keys(config.google.calendars) as [string, ...string[]];

const paramsSchema = z.object({
  summary: z.string().describe('Event title/summary'),
  start: z.string().describe('Start datetime (ISO 8601)'),
  end: z.string().describe('End datetime (ISO 8601)'),
  description: z.string().optional().describe('Event description/details'),
  calendar: z
    .enum(calendarKeys)
    .optional()
    .describe(`Which calendar to use (options: ${calendarKeys.join(', ')}, default: primary)`),
});

export const definition = {
  name: 'createCalendarEvent',
  description: 'Create a new calendar event on the specified calendar (primary, family, wife, or holidays)',
  paramsSchema,
  tags: ['calendar', 'scheduling', 'create'],
  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 30,
  maxConcurrent: 2,
  priority: 'normal' as const,
  timeoutMs: 15000,
};

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{ created: boolean; eventId: string | null; summary: string }> {
  const parsed = paramsSchema.parse(params);

  const startDate = new Date(parsed.start);
  const endDate = new Date(parsed.end);

  if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) {
    throw new SkillExecutionError('Invalid start/end datetime. Use ISO 8601 format.', false);
  }

  if (endDate <= startDate) {
    throw new SkillExecutionError('End datetime must be after start datetime.', false);
  }

  // Ensure we're using local timezone format, not UTC
  let startIso = parsed.start;
  let endIso = parsed.end;
  
  // If LLM provided UTC (Z suffix), convert to local time
  if (startIso.endsWith('Z')) {
    logger.warn('[SKILL:createCalendarEvent] Received UTC timestamp, converting to SAST');
    startIso = new Date(startDate.getTime()).toISOString().replace('Z', '+02:00');
  }
  if (endIso.endsWith('Z')) {
    endIso = new Date(endDate.getTime()).toISOString().replace('Z', '+02:00');
  }

  const calendarKey = parsed.calendar ?? 'primary';
  const calendarId = config.google.calendars[calendarKey as keyof typeof config.google.calendars];

  logger.info(
    { summary: parsed.summary, start: startIso, end: endIso, calendar: calendarKey },
    '[SKILL:createCalendarEvent] Creating event'
  );

  const createdEvent = await createCalendarEvent({
    summary: parsed.summary,
    start: startIso,
    end: endIso,
    description: parsed.description,
    calendarId,
  });

  const eventId = createdEvent.id ?? null;

  if (event.source === 'telegram') {
    const chatId = (event.payload as { chatId?: string }).chatId;
    const integration = integrationRegistry.get(event.source);
    if (integration?.send && chatId) {
      const formatWhen = (value: string): string => {
        const parsed = new Date(value);
        if (Number.isNaN(parsed.getTime())) return value;
        return parsed.toLocaleString('en-ZA', {
          weekday: 'short',
          year: 'numeric',
          month: 'short',
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
        });
      };

      const start = formatWhen(startIso);
      const end = formatWhen(endIso);
      const calendarName = calendarKey.charAt(0).toUpperCase() + calendarKey.slice(1);
      let message = `✅ Event created on ${calendarName} calendar:\n\n${parsed.summary}\n${start} → ${end}`;
      
      if (parsed.description) {
        message += `\n\nDescription: ${parsed.description}`;
      }

      if (createdEvent.htmlLink) {
        message += `\n\nView: ${createdEvent.htmlLink}`;
      }

      await integration.send({ chatId, text: message });
    }
  }

  logger.info({ eventId, summary: parsed.summary }, '[SKILL:createCalendarEvent] Event created');
  return { created: true, eventId, summary: parsed.summary };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
