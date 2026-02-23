import { z } from 'zod';
import { SkillModule, NormalizedEvent, SkillExecutionError } from '../types/index.js';
import { logger } from '../logging/logger.js';
import { getCalendarEvent, deleteCalendarEvent as deleteEvent, listCalendarEvents } from '../integrations/googleCalendar.js';
import { integrationRegistry } from '../integrations/integrationLoader.js';
import { config } from '../config/config.js';

// Dynamically extract calendar keys from config
const calendarKeys = Object.keys(config.google.calendars) as [string, ...string[]];

const paramsSchema = z.object({
  eventId: z.string().optional().describe('Calendar event ID to delete (if you have it)'),
  eventTitle: z.string().optional().describe('Event title/summary to search for and delete'),
  start: z.string().optional().describe('Start date for searching events (ISO 8601)'),
  end: z.string().optional().describe('End date for searching events (ISO 8601)'),
  calendar: z
    .enum(calendarKeys)
    .optional()
    .describe(`Which calendar to delete from (options: ${calendarKeys.join(', ')})`),
}).refine(
  (data) => data.eventId || (data.eventTitle && data.start && data.end),
  'Either eventId or (eventTitle + start + end) must be provided'
);

export const definition = {
  name: 'deleteCalendarEvent',
  description: 'Delete a calendar event by ID or by searching for it in a time range by title',
  paramsSchema,
  tags: ['calendar', 'delete'],
  minRole: 'admin' as const,
  requiresApproval: true,
  dailyLimit: 10,
  maxConcurrent: 1,
  priority: 'high' as const,
  timeoutMs: 20000,
};

export async function execute(
  params: Record<string, unknown>,
  event: NormalizedEvent
): Promise<{ deleted: boolean; eventId: string }> {
  const parsed = paramsSchema.parse(params);
  
  const calendarKey = parsed.calendar ?? 'primary';
  const calendarId = config.google.calendars[calendarKey as keyof typeof config.google.calendars];
  
  let eventIdToDelete: string;
  let eventSummary: string;
  
  // Path 1: Delete by eventId (direct)
  if (parsed.eventId) {
    logger.info(
      { eventId: parsed.eventId, calendar: calendarKey },
      '[SKILL:deleteCalendarEvent] Looking up event by ID'
    );
    
    const existingEvent = await getCalendarEvent(parsed.eventId, calendarId);
    
    if (!existingEvent) {
      const errorMsg = `Event not found: ${parsed.eventId}. Use "show my calendar" to get valid event IDs.`;
      
      if (event.source === 'telegram') {
        const chatId = (event.payload as { chatId?: string }).chatId;
        const integration = integrationRegistry.get(event.source);
        if (integration?.send && chatId) {
          await integration.send({ chatId, text: `❌ ${errorMsg}` });
        }
      }
      
      throw new SkillExecutionError(errorMsg, false);
    }
    
    eventIdToDelete = parsed.eventId;
    eventSummary = existingEvent.summary ?? '(No title)';
  }
  // Path 2: Delete by searching for event by title in date range
  else if (parsed.eventTitle && parsed.start && parsed.end) {
    const startDate = new Date(parsed.start);
    const endDate = new Date(parsed.end);
    
    if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) {
      throw new SkillExecutionError('Invalid start/end datetime. Use ISO 8601 format.', false);
    }
    
    logger.info(
      { title: parsed.eventTitle, start: parsed.start, end: parsed.end, calendar: calendarKey },
      '[SKILL:deleteCalendarEvent] Searching for event by title'
    );
    
    // Query calendar for events in the range
    const events = await listCalendarEvents(startDate.toISOString(), endDate.toISOString(), calendarId);
    
    // Find event matching the title (case-insensitive, partial match)
    const searchTitle = parsed.eventTitle.toLowerCase();
    const matchingEvent = events.find(e => 
      (e.summary ?? '').toLowerCase().includes(searchTitle) ||
      searchTitle.includes((e.summary ?? '').toLowerCase())
    );
    
    if (!matchingEvent || !matchingEvent.id) {
      const errorMsg = `Event "${parsed.eventTitle}" not found between ${parsed.start} and ${parsed.end}. Use "show my calendar" to see available events.`;
      
      if (event.source === 'telegram') {
        const chatId = (event.payload as { chatId?: string }).chatId;
        const integration = integrationRegistry.get(event.source);
        if (integration?.send && chatId) {
          await integration.send({ chatId, text: `❌ ${errorMsg}` });
        }
      }
      
      throw new SkillExecutionError(errorMsg, false);
    }
    
    if (events.filter(e => (e.summary ?? '').toLowerCase().includes(searchTitle)).length > 1) {
      const errorMsg = `Multiple events matching "${parsed.eventTitle}" found. Please specify the exact event ID using "show my calendar".`;
      
      if (event.source === 'telegram') {
        const chatId = (event.payload as { chatId?: string }).chatId;
        const integration = integrationRegistry.get(event.source);
        if (integration?.send && chatId) {
          await integration.send({ chatId, text: `⚠️ ${errorMsg}` });
        }
      }
      
      throw new SkillExecutionError(errorMsg, false);
    }
    
    eventIdToDelete = matchingEvent.id;
    eventSummary = matchingEvent.summary ?? '(No title)';
  } else {
    throw new SkillExecutionError('Either eventId or (eventTitle + start + end) must be provided', false);
  }
  
  logger.info(
    { eventId: eventIdToDelete, summary: eventSummary, calendar: calendarKey },
    '[SKILL:deleteCalendarEvent] Event found, deleting'
  );
  
  try {
    await deleteEvent(eventIdToDelete, calendarId);
    
    if (event.source === 'telegram') {
      const chatId = (event.payload as { chatId?: string }).chatId;
      const integration = integrationRegistry.get(event.source);
      if (integration?.send && chatId) {
        const calendarName = calendarKey.charAt(0).toUpperCase() + calendarKey.slice(1);
        const message = `✅ Event deleted from ${calendarName} calendar\n\n"${eventSummary}"\n\nEvent ID: ${eventIdToDelete}`;
        await integration.send({ chatId, text: message });
      }
    }
    
    logger.info({ eventId: eventIdToDelete }, '[SKILL:deleteCalendarEvent] Event deleted');
    return { deleted: true, eventId: eventIdToDelete };
  } catch (err: any) {
    // Send error message to user
    if (event.source === 'telegram') {
      const chatId = (event.payload as { chatId?: string }).chatId;
      const integration = integrationRegistry.get(event.source);
      if (integration?.send && chatId) {
        const errorMsg = err.message?.includes('not found') 
          ? `❌ Event not found\n\nUse "show my calendar" to get valid event IDs.`
          : `❌ Failed to delete event: ${err.message}`;
        await integration.send({ chatId, text: errorMsg });
      }
    }
    throw err;
  }
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
