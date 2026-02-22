import { describe, it, expect } from 'vitest';
import { checkRateLimit } from '../src/concurrency/rateLimiter.js';

describe('checkRateLimit', () => {
  it('allows requests within limits', () => {
    const result = checkRateLimit('telegram');
    expect(result.allowed).toBe(true);
  });

  it('allows unknown integration', () => {
    const result = checkRateLimit('unknown-source');
    expect(result.allowed).toBe(true);
  });
});
