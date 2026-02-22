import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockFetch = vi.fn();
global.fetch = mockFetch;

import { decide } from '../../src/llm/llmDecider.js';
import { NormalizedEvent, SkillDefinition } from '../../src/types/index.js';
import { z } from 'zod';

const mockEvent: NormalizedEvent = {
  id: 'test-event-1',
  traceId: 'trace-1',
  source: 'telegram',
  type: 'message',
  payload: { text: 'Send John a hello message' },
  timestamp: new Date().toISOString(),
  userId: 'user-1',
  role: 'user',
};

const mockSkills: SkillDefinition[] = [
  {
    name: 'sendMessage',
    description: 'Send a message',
    paramsSchema: z.object({ target: z.string(), text: z.string() }),
    tags: ['communication', 'send', 'message'],
    minRole: 'user',
    requiresApproval: false,
    dailyLimit: 100,
    maxConcurrent: 1,
    priority: 'high',
    timeoutMs: 10000,
  },
];

describe('LLM Fallback Scenarios', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('scenario: primary LLM unavailable → deterministic fallback', async () => {
    mockFetch.mockRejectedValue(new Error('Connection refused'));

    const { decision, model, status } = await decide(mockEvent, [], mockSkills);

    expect(model).toBe('deterministic');
    expect(status).toBe('deterministic');
    expect(decision?.skill).toBe('sendMessage');
  });

  it('scenario: primary times out → fallback LLM succeeds', async () => {
    const validDecision = {
      skill: 'sendMessage',
      params: { target: 'John', text: 'hello' },
      reasoning: 'User wants to send hello to John',
    };

    mockFetch
      .mockRejectedValueOnce(new Error('timeout'))
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ response: JSON.stringify(validDecision) }),
      });

    const { decision, status } = await decide(mockEvent, [], mockSkills);
    expect(decision?.skill).toBe('sendMessage');
    expect(status).toBe('fallback');
  });

  it('scenario: all LLMs unavailable → abstain', async () => {
    mockFetch.mockRejectedValue(new Error('All down'));

    const noMatchEvent: NormalizedEvent = {
      ...mockEvent,
      payload: { text: 'xyzabcdef unrecognized action' },
      type: 'unknown-type-xyz',
    };

    const { decision, status } = await decide(noMatchEvent, [], []);
    expect(decision).toBeNull();
    expect(status).toBe('abstained');
  });
});
