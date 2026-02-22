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
  encryptionKey: process.env['ENCRYPTION_KEY'] ?? '',
  ollama: {
    url: process.env['OLLAMA_URL'] ?? 'http://localhost:11434',
    primaryModel: process.env['LLM_PRIMARY_MODEL'] ?? 'qwen3:4b',
    fallbackModels: [
      { model: process.env['LLM_FALLBACK_1'] ?? 'mistral:7b', timeoutMs: 8000 },
      { model: process.env['LLM_FALLBACK_2'] ?? 'phi:2.7b', timeoutMs: 10000 },
    ],
    primaryTimeoutMs: 5000,
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
  },
  dataDir: resolve(process.cwd(), 'data'),
  logsDir: resolve(process.cwd(), 'logs'),
  memoryDir: resolve(process.cwd(), 'data', 'memory'),
  executionsDir: resolve(process.cwd(), 'data', 'executions'),
  approvalsDir: resolve(process.cwd(), 'data', 'approvals'),
  integrationsDir: resolve(process.cwd(), 'integrations'),
  skillsDir: resolve(process.cwd(), 'skills'),
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
} as const;
