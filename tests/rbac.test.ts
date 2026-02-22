import { describe, it, expect } from 'vitest';
import { checkPermission } from '../src/permissions/rbac.js';
import { NormalizedEvent } from '../src/types/index.js';

function makeEvent(role: 'guest' | 'user' | 'admin', source = 'telegram'): NormalizedEvent {
  return {
    id: 'test-id',
    traceId: 'trace-id',
    source,
    type: 'message',
    payload: {},
    timestamp: new Date().toISOString(),
    userId: 'user-1',
    role,
  };
}

describe('checkPermission', () => {
  it('allows user role for sendMessage', () => {
    const result = checkPermission('sendMessage', makeEvent('user'));
    expect(result.allowed).toBe(true);
  });

  it('denies guest role for sendMessage', () => {
    const result = checkPermission('sendMessage', makeEvent('guest'));
    expect(result.allowed).toBe(false);
  });

  it('allows admin role for deleteCalendarEvent', () => {
    const result = checkPermission('deleteCalendarEvent', makeEvent('admin'));
    expect(result.allowed).toBe(true);
  });

  it('denies user role for deleteCalendarEvent', () => {
    const result = checkPermission('deleteCalendarEvent', makeEvent('user'));
    expect(result.allowed).toBe(false);
  });

  it('denies unknown skill', () => {
    const result = checkPermission('unknownSkill', makeEvent('admin'));
    expect(result.allowed).toBe(false);
  });

  it('denies user for admin-only integrations', () => {
    const result = checkPermission('sendMessage', makeEvent('user', 'gmail'));
    expect(result.allowed).toBe(false);
  });
});
