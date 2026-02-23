import { Role, NormalizedEvent } from '../types/index.js';

const ROLE_HIERARCHY: Record<Role, number> = {
  guest: 0,
  user: 1,
  admin: 2,
};

interface SkillPermission {
  minRole: Role;
  requiresApproval: boolean;
  dailyLimit: number;
}

const SKILL_PERMISSIONS: Record<string, SkillPermission> = {
  sendMessage: { minRole: 'user', requiresApproval: false, dailyLimit: 100 },
  queryCalendar: { minRole: 'user', requiresApproval: false, dailyLimit: 50 },
  createCalendarEvent: { minRole: 'user', requiresApproval: false, dailyLimit: 30 },
  deleteCalendarEvent: { minRole: 'admin', requiresApproval: true, dailyLimit: 10 },
  updateContext: { minRole: 'admin', requiresApproval: true, dailyLimit: 50 },
};

const INTEGRATION_ACCESS: Record<string, Role> = {
  telegram: 'user',
  calendar: 'user',
  discord: 'user',
  gmail: 'admin',
  aws: 'admin',
  azure: 'admin',
};

const dailyUsage: Map<string, number> = new Map();
let usageDate = new Date().toDateString();

function resetDailyUsageIfNeeded(): void {
  const today = new Date().toDateString();
  if (today !== usageDate) {
    dailyUsage.clear();
    usageDate = today;
  }
}

export function checkPermission(
  skillName: string,
  event: NormalizedEvent
): { allowed: boolean; reason?: string } {
  resetDailyUsageIfNeeded();

  const userRole: Role = event.role ?? 'guest';
  const permission = SKILL_PERMISSIONS[skillName];

  if (!permission) {
    return { allowed: false, reason: `Unknown skill: ${skillName}` };
  }

  if (ROLE_HIERARCHY[userRole] < ROLE_HIERARCHY[permission.minRole]) {
    return {
      allowed: false,
      reason: `Insufficient role. Required: ${permission.minRole}, got: ${userRole}`,
    };
  }

  const integrationRole = INTEGRATION_ACCESS[event.source];
  if (integrationRole && ROLE_HIERARCHY[userRole] < ROLE_HIERARCHY[integrationRole]) {
    return {
      allowed: false,
      reason: `Integration ${event.source} requires role ${integrationRole}`,
    };
  }

  const usageKey = `${event.userId ?? 'anonymous'}:${skillName}`;
  const used = dailyUsage.get(usageKey) ?? 0;
  if (used >= permission.dailyLimit) {
    return {
      allowed: false,
      reason: `Daily limit exceeded for ${skillName} (${permission.dailyLimit}/day)`,
    };
  }

  return { allowed: true };
}

export function recordUsage(skillName: string, event: NormalizedEvent): void {
  resetDailyUsageIfNeeded();
  const usageKey = `${event.userId ?? 'anonymous'}:${skillName}`;
  dailyUsage.set(usageKey, (dailyUsage.get(usageKey) ?? 0) + 1);
}

export function requiresApproval(skillName: string): boolean {
  return SKILL_PERMISSIONS[skillName]?.requiresApproval ?? false;
}
