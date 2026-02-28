import { z } from 'zod';

export type Role = 'guest' | 'user' | 'admin';

export class SkillExecutionError extends Error {
  constructor(
    message: string,
    public readonly retryable: boolean = true
  ) {
    super(message);
    this.name = 'SkillExecutionError';
  }
}

export interface NormalizedEvent {
  id: string;
  traceId: string;
  source: string;
  type: string;
  payload: Record<string, unknown>;
  timestamp: string;
  userId?: string;
  role?: Role;
  tenantId?: string;
}

export interface ContextFile {
  label: string;
  category: 'personal' | 'conversation' | 'temporary' | 'event-log';
  priority: 'high' | 'normal' | 'low';
  lastUpdated: string;
  tags: string[];
  content: string;
  version?: number;
}

export interface ContextIndex {
  files: Array<{
    path: string;
    label: string;
    category: string;
    priority: string;
    tags: string[];
    lastUpdated: string;
  }>;
}

export interface LLMDecision {
  skill: string;
  params: Record<string, unknown>;
  reasoning?: string;
}

export interface SkillDefinition {
  name: string;
  description: string;
  paramsSchema: z.ZodObject<z.ZodRawShape>;
  tags: string[];
  minRole: Role;
  requiresApproval: boolean;
  dailyLimit: number;
  maxConcurrent: number;
  priority: 'high' | 'normal' | 'low';
  timeoutMs: number;
}

export interface SkillModule {
  definition: SkillDefinition;
  execute: (params: Record<string, unknown>, event: NormalizedEvent) => Promise<unknown>;
}

export interface IntegrationModule {
  name: string;
  poll: () => Promise<NormalizedEvent[] | null>;
  send: (payload: Record<string, unknown>) => Promise<void>;
  onEvent: (callback: (event: NormalizedEvent) => void) => void;
}

export interface DecisionTrace {
  traceId: string;
  timestamp: string;
  event: NormalizedEvent;
  retrievedContexts: Array<{ label: string; score: number }>;
  llmPrompt: string;
  llmResponse: string;
  decision: LLMDecision | null;
  llmModel: string;
  executionTimeMs: number;
  status: 'success' | 'fallback' | 'abstained' | 'error';
}

export interface ExecutionRecord {
  id: string;
  skillName: string;
  params: Record<string, unknown>;
  userId: string;
  idempotencyKey: string;
  status: 'pending' | 'in-progress' | 'completed' | 'failed' | 'timeout';
  retries: number;
  createdAt: string;
  updatedAt: string;
  result?: unknown;
  error?: string;
}

export interface ApprovalRequest {
  type: 'approval-request';
  id: string;
  requestedBy: string;
  integration: {
    name: string;
    description: string;
    estimatedApis: string[];
    requiredSecrets: string[];
  };
  generatedCodePreview: string;
  fullCodeUrl: string;
  expiresAt: string;
  actions: Array<{ label: string; action: string; style: string }>;
  validationResult: { valid: boolean; violations: string[]; severity: string };
  status: 'pending' | 'approved' | 'rejected' | 'timeout';
  decidedAt?: string;
  decidedBy?: string;
}

export type QueueMessageType = 'incoming' | 'outgoing';
export type QueueMessageStatus = 'pending' | 'processing' | 'completed' | 'failed';

export interface QueueMessage {
  id: string;
  type: QueueMessageType;
  status: QueueMessageStatus;
  source: string;
  targetChannel?: string;
  payload: Record<string, unknown>;
  event?: NormalizedEvent;
  error?: string;
  retryCount: number;
  maxRetries: number;
  createdAt: string;
  updatedAt: string;
  expiresAt: string;
}
