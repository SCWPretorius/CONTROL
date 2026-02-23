import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import { listCalendarEvents } from '../integrations/googleCalendar.js';
import { integrationRegistry } from '../integrations/integrationLoader.js';
import { config } from '../config/config.js';
import { saveContext } from '../memory/contextStore.js';

// Dynamically extract calendar keys that have values
const calendarKeysWithValues = Object.entries(config.google.calendars)
  .filter(([, value]) => value) // Only include calendars with a value
  .map(([key]) => key) as [string, ...string[]];

const paramsSchema = z.object({
  start: z.string().describe('Start datetime (ISO 8601)'),
  end: z.string().describe('End datetime (ISO 8601)'),
  calendars: z
    .array(z.enum(calendarKeysWithValues))
    .optional()
    .describe(`Which calendars to query (default: ${calendarKeysWithValues.join(', ')})`),
});

export const definition = {
  name: 'queryCalendar',
  description: 'Check calendar availability across primary, family, and wife calendars between two times',
  paramsSchema,
  tags: ['calendar', 'scheduling'],
  minRole: 'user' as const,
  requiresApproval: false,
  dailyLimit: 50,
  maxConcurrent: 3,
  priority: 'normal' as const,
  timeoutMs: 15000,
};

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{ events: unknown[] }> {
  const parsed = paramsSchema.parse(params);
  const startDate = new Date(parsed.start);
  const endDate = new Date(parsed.end);

  if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) {
    throw new SkillExecutionError('Invalid start/end datetime. Use ISO 8601 format.', false);
  }

  if (endDate <= startDate) {
    throw new SkillExecutionError('End datetime must be after start datetime.', false);
  }

  const timeMin = startDate.toISOString();
  const timeMax = endDate.toISOString();

  // Determine which calendars to query (default to all configured calendars)
  const calendarsToQuery = parsed.calendars ?? calendarKeysWithValues;
  
  logger.info(
    { start: timeMin, end: timeMax, calendars: calendarsToQuery },
    '[SKILL:queryCalendar] Querying'
  );

  // Query all specified calendars in parallel
  const calendarResults = await Promise.all(
    calendarsToQuery.map(async (calKey) => {
      const calendarId = config.google.calendars[calKey as keyof typeof config.google.calendars];
      try {
        const items = await listCalendarEvents(timeMin, timeMax, calendarId);
        return items.map(item => ({
          calendar: calKey,
          id: item.id ?? null,
          summary: item.summary ?? '(No title)',
          start: item.start?.dateTime ?? item.start?.date ?? null,
          end: item.end?.dateTime ?? item.end?.date ?? null,
          location: item.location ?? null,
          status: item.status ?? null,
          htmlLink: item.htmlLink ?? null,
          organizer: item.organizer?.email ?? null,
        }));
      } catch (err) {
        logger.warn({ calendar: calKey, err }, '[SKILL:queryCalendar] Failed to query calendar');
        return [];
      }
    })
  );

  // Flatten and sort by start time
  const events = calendarResults.flat().sort((a, b) => {
    const aStart = a.start ? new Date(a.start).getTime() : 0;
    const bStart = b.start ? new Date(b.start).getTime() : 0;
    return aStart - bStart;
  });

  logger.info({ count: events.length }, '[SKILL:queryCalendar] Results');

  if (event.source === 'telegram') {
    const chatId = (event.payload as { chatId?: string }).chatId;
    const integration = integrationRegistry.get(event.source);
    if (integration?.send && chatId) {
      const formatWhen = (value: string | null): string => {
        if (!value) return 'unknown';
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

      const header = `Calendar results (${events.length})`;
      const body = events.length === 0
        ? 'No events found for that range.'
        : events
          .slice(0, 20)
          .map((e, idx) => {
            const start = formatWhen(e.start ?? null);
            const end = formatWhen(e.end ?? null);
            const title = e.summary ?? '(No title)';
            const cal = e.calendar ? ` [${e.calendar.toUpperCase()}]` : '';
            const id = e.id ? `\n   ID: ${e.id}` : '';
            return `${idx + 1}. ${title}${cal}\n   ${start} → ${end}${id}`;
          })
          .join('\n');
      const tail = events.length > 20 ? `\n…and ${events.length - 20} more.` : '';
      await integration.send({ chatId, text: `${header}\n${body}${tail}` });
    } else {
      logger.warn('[SKILL:queryCalendar] Telegram response skipped (missing chatId or integration)');
    }
  }

  // Store results in persistent context for follow-up actions (e.g., "delete the first one")
  const contextContent = `Last calendar query results:
Start: ${parsed.start}
End: ${parsed.end}
Calendars: ${calendarsToQuery.join(', ')}
Total events: ${events.length}

${events.length === 0 ? 'No events found.' : events.map((e, idx) => `${idx + 1}. [${e.calendar}] ${e.summary} (ID: ${e.id}) - ${e.start ?? 'unknown'} to ${e.end ?? 'unknown'}`).join('\n')}`;

  saveContext('conversation/last-query.json', {
    label: 'last-calendar-query',
    category: 'conversation',
    priority: 'high',
    tags: ['calendar', 'query', 'conversation'],
    lastUpdated: new Date().toISOString(),
    content: contextContent,
  });

  return { events };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
