import { z } from 'zod';

/** Zod schema for the application configuration object. Used for validation at startup. */
export const ConfigSchema = z.object({
  port: z.number().int().min(1).max(65535),
  host: z.string().min(1),
  webhookBaseUrl: z.string(),
  adminUserIds: z.array(z.string()),
  lmstudio: z.object({
    url: z.string(),
    primaryModel: z.string().min(1),
    primaryTimeoutMs: z.number().positive(),
  }),
  telegram: z.object({
    botToken: z.string(),
    webhookSecret: z.string(),
  }),
  discord: z.object({
    botToken: z.string(),
    publicKey: z.string(),
  }),
  google: z.object({
    clientId: z.string(),
    clientSecret: z.string(),
    redirectUri: z.string(),
  }),
  homeAssistant: z.object({
    baseUrl: z.string(),
    monitoredEntities: z.array(z.string()),
  }),
  dataDir: z.string().min(1),
  logsDir: z.string().min(1),
  memoryDir: z.string().min(1),
  executionsDir: z.string().min(1),
}).partial();

export type AppConfigInput = z.input<typeof ConfigSchema>;
export type AppConfig = z.infer<typeof ConfigSchema>;
