import { NormalizedEvent } from '../types/index.js';
import { v4 as uuidv4 } from 'uuid';
import { google, calendar_v3 } from 'googleapis';
import type { OAuth2Client, Credentials } from 'google-auth-library';
import { config } from '../config/config.js';
import { logger } from '../logging/logger.js';
import { getSecret, setSecret } from '../config/secrets.js';

let eventCallback: ((event: NormalizedEvent) => void) | null = null;

const GOOGLE_TOKEN_SECRET = 'GOOGLE_OAUTH_TOKENS';
const GOOGLE_SCOPES = ['https://www.googleapis.com/auth/calendar'];

function ensureGoogleConfig(): void {
  if (!config.google.clientId || !config.google.clientSecret || !config.google.redirectUri) {
    throw new Error('Google OAuth config missing: set GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REDIRECT_URI');
  }
}

function loadTokens(): Credentials | null {
  const raw = getSecret(GOOGLE_TOKEN_SECRET);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as Credentials;
  } catch (err) {
    logger.warn({ err }, '[CALENDAR] Failed to parse stored OAuth tokens');
    return null;
  }
}

function mergeTokens(
  existing: Credentials | null,
  incoming: Credentials
): Credentials {
  return {
    ...existing,
    ...incoming,
    refresh_token: incoming.refresh_token ?? existing?.refresh_token,
  };
}

function saveTokens(tokens: Credentials): void {
  setSecret(GOOGLE_TOKEN_SECRET, JSON.stringify(tokens));
}

function createOAuthClient(): OAuth2Client {
  ensureGoogleConfig();
  const client = new google.auth.OAuth2(
    config.google.clientId,
    config.google.clientSecret,
    config.google.redirectUri
  );
  client.on('tokens', tokens => {
    if (!tokens) return;
    const current = loadTokens();
    const merged = mergeTokens(current, tokens);
    saveTokens(merged);
  });
  return client;
}

export function getGoogleAuthUrl(): string {
  const client = createOAuthClient();
  return client.generateAuthUrl({
    access_type: 'offline',
    prompt: 'consent',
    scope: GOOGLE_SCOPES,
    include_granted_scopes: true,
  });
}

export async function exchangeCodeForTokens(code: string): Promise<void> {
  const client = createOAuthClient();
  const { tokens } = await client.getToken(code);
  if (!tokens) {
    throw new Error('No OAuth tokens returned from Google');
  }
  const merged = mergeTokens(loadTokens(), tokens);
  saveTokens(merged);
  logger.info('[CALENDAR] OAuth tokens stored');
}

export function getGoogleAuthStatus(): {
  configured: boolean;
  connected: boolean;
  hasRefreshToken: boolean;
} {
  const configured = Boolean(
    config.google.clientId && config.google.clientSecret && config.google.redirectUri
  );
  const tokens = loadTokens();
  const connected = Boolean(tokens?.access_token || tokens?.refresh_token);
  const hasRefreshToken = Boolean(tokens?.refresh_token);
  return { configured, connected, hasRefreshToken };
}

export function getAuthorizedClient(): OAuth2Client {
  const client = createOAuthClient();
  const tokens = loadTokens();
  if (!tokens) {
    throw new Error('Google Calendar not connected. Visit /auth/google/start to authorize.');
  }
  client.setCredentials(tokens);
  return client;
}

export async function listCalendarEvents(
  timeMin: string,
  timeMax: string,
  calendarId = 'primary'
): Promise<calendar_v3.Schema$Event[]> {
  const auth = getAuthorizedClient();
  const calendar = google.calendar({ version: 'v3', auth });
  const response = await calendar.events.list({
    calendarId,
    timeMin,
    timeMax,
    singleEvents: true,
    orderBy: 'startTime',
    maxResults: 100,
  });
  return response.data.items ?? [];
}

export async function getCalendarEvent(
  eventId: string,
  calendarId = 'primary'
): Promise<calendar_v3.Schema$Event | null> {
  const auth = getAuthorizedClient();
  const calendar = google.calendar({ version: 'v3', auth });
  
  try {
    const response = await calendar.events.get({
      calendarId,
      eventId,
    });
    return response.data;
  } catch (err: any) {
    if (err.code === 404) {
      return null;
    }
    throw err;
  }
}

export async function createCalendarEvent(params: {
  summary: string;
  start: string;
  end: string;
  description?: string;
  calendarId?: string;
}): Promise<calendar_v3.Schema$Event> {
  const auth = getAuthorizedClient();
  const calendar = google.calendar({ version: 'v3', auth });
  const calendarId = params.calendarId ?? 'primary';
  
  const response = await calendar.events.insert({
    calendarId,
    requestBody: {
      summary: params.summary,
      description: params.description,
      start: {
        dateTime: params.start,
        timeZone: config.timezone,
      },
      end: {
        dateTime: params.end,
        timeZone: config.timezone,
      },
    },
  });
  
  if (!response.data) {
    throw new Error('Failed to create calendar event');
  }
  
  logger.info({ eventId: response.data.id, summary: params.summary }, '[CALENDAR] Event created');
  return response.data;
}

export async function deleteCalendarEvent(
  eventId: string,
  calendarId = 'primary'
): Promise<void> {
  const auth = getAuthorizedClient();
  const calendar = google.calendar({ version: 'v3', auth });
  
  try {
    await calendar.events.delete({
      calendarId,
      eventId,
    });
    
    logger.info({ eventId, calendarId }, '[CALENDAR] Event deleted');
  } catch (err: any) {
    if (err.code === 404) {
      throw new Error(`Event not found: ${eventId}. Use queryCalendar to get valid event IDs.`);
    }
    throw err;
  }
}

const googleCalendarIntegration = {
  name: 'calendar',

  async poll(): Promise<NormalizedEvent[] | null> {
    if (!config.google.clientId) return null;
    logger.debug('[CALENDAR] Poll called (requires OAuth token setup)');
    return null;
  },

  async send(payload: Record<string, unknown>): Promise<void> {
    logger.info({ payload }, '[CALENDAR] Send called');
  },

  onEvent(callback: (event: NormalizedEvent) => void): void {
    eventCallback = callback;
  },

  handlePushWebhook(body: string, _token: string): NormalizedEvent | null {
    try {
      const notification = JSON.parse(body) as Record<string, unknown>;
      const event: NormalizedEvent = {
        id: uuidv4(),
        traceId: uuidv4(),
        source: 'calendar',
        type: 'push-notification',
        payload: { notification },
        timestamp: new Date().toISOString(),
        role: 'user',
      };
      if (eventCallback) eventCallback(event);
      return event;
    } catch (err) {
      logger.error({ err }, '[CALENDAR] Failed to parse push webhook');
      return null;
    }
  },
};

export default googleCalendarIntegration;
