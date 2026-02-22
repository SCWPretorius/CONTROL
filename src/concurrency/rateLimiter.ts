interface RateWindow {
  count: number;
  windowStart: number;
  burst: number;
  cooldownUntil: number;
}

interface SkillConcurrencyState {
  active: number;
  queue: Array<() => void>;
}

const INTEGRATION_LIMITS: Record<string, { perMinute: number; burst: number; cooldownMs: number }> = {
  telegram: { perMinute: 30, burst: 5, cooldownMs: 60000 },
  calendar: { perMinute: 10, burst: 2, cooldownMs: 120000 },
  discord: { perMinute: 50, burst: 5, cooldownMs: 60000 },
};

const SKILL_LIMITS: Record<string, { maxConcurrent: number }> = {
  sendMessage: { maxConcurrent: 1 },
  queryCalendar: { maxConcurrent: 3 },
  deleteCalendarEvent: { maxConcurrent: 1 },
};

const integrationWindows = new Map<string, RateWindow>();
const skillConcurrency = new Map<string, SkillConcurrencyState>();
let globalActiveSkills = 0;
const MAX_GLOBAL_SKILLS = 5;

export function checkRateLimit(source: string): { allowed: boolean; reason?: string } {
  const limit = INTEGRATION_LIMITS[source];
  if (!limit) return { allowed: true };

  const now = Date.now();
  const windowMs = 60000;

  let w = integrationWindows.get(source);
  if (!w) {
    w = { count: 0, windowStart: now, burst: 0, cooldownUntil: 0 };
    integrationWindows.set(source, w);
  }

  if (now < w.cooldownUntil) {
    return { allowed: false, reason: `${source} in cooldown until ${new Date(w.cooldownUntil).toISOString()}` };
  }

  if (now - w.windowStart >= windowMs) {
    w.count = 0;
    w.windowStart = now;
    w.burst = 0;
  }

  if (w.count >= limit.perMinute) {
    w.cooldownUntil = now + limit.cooldownMs;
    return { allowed: false, reason: `${source} rate limit exceeded (${limit.perMinute}/min)` };
  }

  if (w.burst >= limit.burst) {
    return { allowed: false, reason: `${source} burst limit exceeded` };
  }

  w.count++;
  w.burst++;
  const capturedWindow = w;
  setTimeout(() => { capturedWindow.burst = Math.max(0, capturedWindow.burst - 1); }, 1000);

  return { allowed: true };
}

export async function acquireSkillSlot(skillName: string): Promise<() => void> {
  const limit = SKILL_LIMITS[skillName] ?? { maxConcurrent: 1 };

  if (!skillConcurrency.has(skillName)) {
    skillConcurrency.set(skillName, { active: 0, queue: [] });
  }

  const state = skillConcurrency.get(skillName)!;

  return new Promise(resolve => {
    const tryAcquire = () => {
      if (state.active < limit.maxConcurrent && globalActiveSkills < MAX_GLOBAL_SKILLS) {
        state.active++;
        globalActiveSkills++;
        resolve(() => {
          state.active--;
          globalActiveSkills--;
          const next = state.queue.shift();
          if (next) next();
        });
      } else {
        state.queue.push(tryAcquire);
      }
    };
    tryAcquire();
  });
}
