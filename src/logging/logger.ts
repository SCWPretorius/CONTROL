import pino from 'pino';

const NEVER_LOG = [
  'ENCRYPTION_KEY',
  'TOKEN_TELEGRAM',
  'GOOGLE_CLIENT_SECRET',
  'GMAIL_PASSWORD',
  'BACKUP_ENCRYPTION_KEY',
];

function redact(obj: Record<string, unknown>): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(obj)) {
    if (NEVER_LOG.some(k => key.toUpperCase().includes(k))) {
      result[key] = '[REDACTED]';
    } else if (value && typeof value === 'object') {
      result[key] = redact(value as Record<string, unknown>);
    } else {
      result[key] = value;
    }
  }
  return result;
}

export const logger = pino({
  level: process.env['LOG_LEVEL'] ?? 'info',
  transport:
    process.env['NODE_ENV'] !== 'production'
      ? { target: 'pino-pretty', options: { colorize: true } }
      : undefined,
  serializers: {
    err: pino.stdSerializers.err,
  },
});

export { redact };
