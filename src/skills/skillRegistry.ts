import { SkillModule, SkillDefinition } from '../types/index.js';
import { logger } from '../logging/logger.js';

class SkillRegistry {
  private skills = new Map<string, SkillModule>();
  private previousVersions = new Map<string, SkillModule>();
  private quarantine = new Map<string, { loadedAt: Date; errors: number }>();
  private active = new Set<string>();

  register(module: SkillModule): void {
    const name = module.definition.name;
    if (this.skills.has(name)) {
      this.previousVersions.set(name, this.skills.get(name)!);
    }
    this.skills.set(name, module);
    this.quarantine.set(name, { loadedAt: new Date(), errors: 0 });
    logger.info({ skill: name }, '[SKILL] Loaded into quarantine (5 min monitoring)');

    setTimeout(() => {
      const q = this.quarantine.get(name);
      if (q && q.errors < 3) {
        this.quarantine.delete(name);
        this.active.add(name);
        logger.info({ skill: name }, '[SKILL] Quarantine passed → active');
      } else if (q) {
        this.rollback(name);
      }
    }, 5 * 60 * 1000);
  }

  recordError(name: string): void {
    const q = this.quarantine.get(name);
    if (q) {
      q.errors++;
      if (q.errors >= 3) {
        logger.error({ skill: name }, '[SKILL] Too many quarantine errors → rolling back');
        this.rollback(name);
      }
    }
  }

  rollback(name: string): void {
    const prev = this.previousVersions.get(name);
    if (prev) {
      this.skills.set(name, prev);
      this.active.add(name);
      this.quarantine.delete(name);
      logger.warn({ skill: name }, '[SKILL] Rolled back to previous version');
    } else {
      this.skills.delete(name);
      this.active.delete(name);
      this.quarantine.delete(name);
      logger.warn({ skill: name }, '[SKILL] No previous version; skill removed');
    }
  }

  get(name: string): SkillModule | undefined {
    if (!this.active.has(name)) return undefined;
    return this.skills.get(name);
  }

  getActive(): SkillModule[] {
    return Array.from(this.active)
      .map(n => this.skills.get(n))
      .filter(Boolean) as SkillModule[];
  }

  getActiveDefinitions(): SkillDefinition[] {
    return this.getActive().map(m => m.definition);
  }

  has(name: string): boolean {
    return this.active.has(name) && this.skills.has(name);
  }

  registerBuiltin(module: SkillModule): void {
    this.skills.set(module.definition.name, module);
    this.active.add(module.definition.name);
    logger.info({ skill: module.definition.name }, '[SKILL] Built-in skill registered');
  }
}

export const skillRegistry = new SkillRegistry();
