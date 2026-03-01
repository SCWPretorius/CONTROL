import { z } from 'zod';

export const HOOK_TYPES = ['startup', 'event', 'agent-run', 'tool-call'] as const;
export type HookType = (typeof HOOK_TYPES)[number];

export const ManifestSchema = z.object({
  name: z.string().min(1),
  version: z.string(),
  description: z.string().default(''),
  entry: z.string().default('index.js'),
  tools: z.array(z.string()).default([]),
  hooks: z.array(z.string()).default([]),
  configSchema: z.record(z.string(), z.unknown()).optional(),
});

export type Manifest = z.infer<typeof ManifestSchema>;
