import { z } from 'zod';
import { SkillModule, NormalizedEvent } from '../types/index.js';
import { logger } from '../logging/logger.js';

const paramsSchema = z.object({
  eventId: z.string().describe('Calendar event ID to delete'),
});

export const definition = {
  name: 'deleteCalendarEvent',
  description: 'Delete a calendar event (admin only)',
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
  _event: NormalizedEvent
): Promise<{ deleted: boolean; eventId: string }> {
  const parsed = paramsSchema.parse(params);
  logger.info({ eventId: parsed.eventId }, '[SKILL:deleteCalendarEvent] Deleting');
  return { deleted: true, eventId: parsed.eventId };
}

const skillModule: SkillModule = { definition, execute };
export default skillModule;
