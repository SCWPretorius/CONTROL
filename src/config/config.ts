import { readFileSync, existsSync } from 'fs';
import { resolve } from 'path';

// Load .env if present
if (existsSync(resolve(process.cwd(), '.env'))) {
  const envContent = readFileSync(resolve(process.cwd(), '.env'), 'utf-8');
  for (const line of envContent.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const eqIdx = trimmed.indexOf('=');
    if (eqIdx === -1) continue;
    const key = trimmed.slice(0, eqIdx).trim();
    const value = trimmed.slice(eqIdx + 1).trim().replace(/^["']|["']$/g, '');
    if (key && !(key in process.env)) {
      process.env[key] = value;
    }
  }
}

export const config = {
  port: parseInt(process.env['PORT'] ?? '3000', 10),
  host: process.env['HOST'] ?? '0.0.0.0',
  webhookBaseUrl: process.env['WEBHOOK_BASE_URL'] ?? 'http://localhost:3000',
  heartbeatIntervalMs: parseInt(process.env['HEARTBEAT_INTERVAL_MS'] ?? '30000', 10),
  timezone: process.env['TIMEZONE'] ?? '',
  adminUserIds: (process.env['ADMIN_USER_IDS'] ?? '').split(',').map(s => s.trim()).filter(Boolean),
  encryptionKey: process.env['ENCRYPTION_KEY'] ?? '',
  lmstudio: {
    url: process.env['LMSTUDIO_URL'] ?? process.env['OLLAMA_URL'] ?? 'http://localhost:1234/v1',
    primaryModel: process.env['LLM_PRIMARY_MODEL'] ?? 'local-model',
    fallbackModels: [
      ...(process.env['LLM_FALLBACK_1']
        ? [{ model: process.env['LLM_FALLBACK_1'], timeoutMs: 10000 }]
        : []),
      ...(process.env['LLM_FALLBACK_2']
        ? [{ model: process.env['LLM_FALLBACK_2'], timeoutMs: 12000 }]
        : []),
    ],
    primaryTimeoutMs: parseInt(process.env['LLM_PRIMARY_TIMEOUT_MS'] ?? '15000', 10),
  },
  telegram: {
    botToken: process.env['TELEGRAM_BOT_TOKEN'] ?? '',
    webhookSecret: process.env['TELEGRAM_WEBHOOK_SECRET'] ?? '',
  },
  discord: {
    botToken: process.env['DISCORD_BOT_TOKEN'] ?? '',
    publicKey: process.env['DISCORD_PUBLIC_KEY'] ?? '',
  },
  google: {
    clientId: process.env['GOOGLE_CLIENT_ID'] ?? '',
    clientSecret: process.env['GOOGLE_CLIENT_SECRET'] ?? '',
    redirectUri: process.env['GOOGLE_REDIRECT_URI'] ?? '',
    calendars: {
      primary: process.env['GOOGLE_CALENDAR_PRIMARY'] ?? 'primary',
      family: process.env['GOOGLE_CALENDAR_FAMILY'] ?? '',
      wife: process.env['GOOGLE_CALENDAR_WIFE'] ?? '',
      holidays: process.env['GOOGLE_CALENDAR_HOLIDAYS'] ?? '',
    },
  },
  homeAssistant: {
    baseUrl: process.env['HOME_ASSISTANT_URL'] ?? '',
    monitoredEntities: (process.env['HOME_ASSISTANT_MONITORED_ENTITIES'] ?? '')
      .split(',')
      .map(s => s.trim())
      .filter(Boolean),
  },
  dataDir: resolve(process.cwd(), 'data'),
  logsDir: resolve(process.cwd(), 'logs'),
  memoryDir: resolve(process.cwd(), 'data', 'memory'),
  executionsDir: resolve(process.cwd(), 'data', 'executions'),
  approvalsDir: resolve(process.cwd(), 'data', 'approvals'),
  integrationsDir: resolve(process.cwd(), 'integrations'),
  skillsDir: resolve(process.cwd(), 'skills'),
  queueEnabled: process.env['QUEUE_ENABLED'] !== 'false',
  queueDbPath: process.env['QUEUE_DB_PATH'] ?? 'data/queue.db',
  queueAutoRetryEnabled: process.env['QUEUE_AUTO_RETRY_ENABLED'] !== 'false',
  queueRetryIntervalMs: parseInt(process.env['QUEUE_RETRY_INTERVAL_MS'] ?? '10000', 10),
  queueMaxRetries: parseInt(process.env['QUEUE_MAX_RETRIES'] ?? '5', 10),
  queueMessageTtlMs: parseInt(process.env['QUEUE_MESSAGE_TTL_MS'] ?? '86400000', 10),
  queueCleanupIntervalMs: parseInt(process.env['QUEUE_CLEANUP_INTERVAL_MS'] ?? '3600000', 10),
  backup: {
    encryptionKey: process.env['BACKUP_ENCRYPTION_KEY'] ?? '',
    s3Bucket: process.env['BACKUP_S3_BUCKET'] ?? '',
    enabled: process.env['BACKUP_ENABLED'] === 'true',
  },
  approval: {
    enabled: process.env['APPROVAL_ENABLED'] !== 'false',
    expiryMinutes: parseInt(process.env['APPROVAL_EXPIRY_MINUTES'] ?? '30', 10),
    preferredChannel: process.env['APPROVAL_PREFERRED_CHANNEL'] ?? 'telegram',
    requireSecretValidation: process.env['APPROVAL_REQUIRE_SECRET_VALIDATION'] !== 'false',
    autoRejectTimeout: process.env['APPROVAL_AUTO_REJECT_TIMEOUT'] === 'true',
    auditLogPath: resolve(process.cwd(), 'data', 'approvals'),
    preApproved: ['telegram', 'discord', 'google-calendar', 'slack'],
  },
  slo: {
    eventIngestP95Ms: 500,
    llmDecisionP95Ms: 5000,
    skillExecutionP95Ms: 2000,
  },
  concurrency: {
    maxSkills: 5,
    maxIntegrations: 10,
    maxQueueSize: 1000,
  },
  integrationFailure: {
    maxConsecutiveErrors: 5,
    pollTimeoutMs: 30000,
    healthCheckIntervalMs: 5 * 60 * 1000,
    maxBackoffMs: 5 * 60 * 1000,
  },
  queue: {
    enabled: process.env['QUEUE_ENABLED'] !== 'false',
    dbPath: process.env['QUEUE_DB_PATH'] ?? 'data/queue.db',
    autoRetryEnabled: process.env['QUEUE_AUTO_RETRY_ENABLED'] !== 'false',
    retryIntervalMs: parseInt(process.env['QUEUE_RETRY_INTERVAL_MS'] ?? '10000', 10),
    maxRetries: parseInt(process.env['QUEUE_MAX_RETRIES'] ?? '5', 10),
    messageTtlMs: parseInt(process.env['QUEUE_MESSAGE_TTL_MS'] ?? '86400000', 10),
    cleanupIntervalMs: parseInt(process.env['QUEUE_CLEANUP_INTERVAL_MS'] ?? '3600000', 10),
  },
} as const;
