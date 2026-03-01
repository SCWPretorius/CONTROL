import { resolve } from 'path';
import { checkPermission } from '../permissions/rbac.js';
import type { NormalizedEvent } from '../types/index.js';

export interface PolicyResult {
  allowed: boolean;
  reason?: string;
}

const ALLOWED_SOURCES = new Set([
  'gateway', 'telegram', 'discord', 'home-assistant', 'calendar', 'test',
]);

export function evaluateToolPolicy(toolName: string, event: NormalizedEvent): PolicyResult {
  return checkPermission(toolName, event);
}

export function evaluateWorkspaceBoundary(filePath: string): PolicyResult {
  const workspaceRoot = resolve(process.cwd());
  const resolvedPath = resolve(filePath);
  if (!resolvedPath.startsWith(workspaceRoot + '/') && resolvedPath !== workspaceRoot) {
    return { allowed: false, reason: `Path escapes workspace root: ${resolvedPath}` };
  }
  return { allowed: true };
}

export function evaluateSourcePolicy(source: string): PolicyResult {
  if (!ALLOWED_SOURCES.has(source)) {
    return { allowed: false, reason: `Source not in allowlist: ${source}` };
  }
  return { allowed: true };
}

export function evaluateModelPolicy(provider: string, allowedProviders?: string[]): PolicyResult {
  if (allowedProviders && !allowedProviders.includes(provider)) {
    return { allowed: false, reason: `Provider ${provider} not in allowlist` };
  }
  return { allowed: true };
}
