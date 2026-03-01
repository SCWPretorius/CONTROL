import type { Role } from '../types/index.js';

export interface TextBlock {
  type: 'text';
  content: string;
}

export interface ToolCallBlock {
  type: 'tool-call';
  callId: string;
  name: string;
  params: Record<string, unknown>;
}

export interface ToolResultBlock {
  type: 'tool-result';
  callId: string;
  name: string;
  result: unknown;
  success: boolean;
  error?: string;
}

export interface ErrorBlock {
  type: 'error';
  message: string;
}

export interface DoneBlock {
  type: 'done';
  stepsExecuted: number;
  model: string;
}

export type AgentBlock = TextBlock | ToolCallBlock | ToolResultBlock | ErrorBlock | DoneBlock;

export interface AgentRunOptions {
  wsSessionId: string;
  userId: string;
  role: Role;
  source: string;
  message: string;
  maxSteps?: number;
}
