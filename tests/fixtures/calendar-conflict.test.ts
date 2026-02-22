import { describe, it, expect } from 'vitest';

describe('Calendar Conflict Detection', () => {
  it('detects time overlap', () => {
    function hasOverlap(
      a: { start: string; end: string },
      b: { start: string; end: string }
    ): boolean {
      const aStart = new Date(a.start).getTime();
      const aEnd = new Date(a.end).getTime();
      const bStart = new Date(b.start).getTime();
      const bEnd = new Date(b.end).getTime();
      return aStart < bEnd && bStart < aEnd;
    }

    expect(
      hasOverlap(
        { start: '2026-02-22T09:00:00Z', end: '2026-02-22T10:00:00Z' },
        { start: '2026-02-22T09:30:00Z', end: '2026-02-22T10:30:00Z' }
      )
    ).toBe(true);

    expect(
      hasOverlap(
        { start: '2026-02-22T09:00:00Z', end: '2026-02-22T10:00:00Z' },
        { start: '2026-02-22T10:00:00Z', end: '2026-02-22T11:00:00Z' }
      )
    ).toBe(false);
  });

  it('respects time constraints (no meetings before 08:00 SAST)', () => {
    function isAllowedTime(datetime: string): boolean {
      const d = new Date(datetime);
      const hours = d.getUTCHours() + 2; // SAST = UTC+2
      return hours >= 8 && hours < 18;
    }

    // 04:00 UTC = 06:00 SAST — before business hours
    expect(isAllowedTime('2026-02-22T04:00:00Z')).toBe(false);
    // 05:00 UTC = 07:00 SAST — still before business hours
    expect(isAllowedTime('2026-02-22T05:00:00Z')).toBe(false);
    // 06:00 UTC = 08:00 SAST — exactly at boundary, allowed
    expect(isAllowedTime('2026-02-22T06:00:00Z')).toBe(true);
    // 08:00 UTC = 10:00 SAST — during business hours
    expect(isAllowedTime('2026-02-22T08:00:00Z')).toBe(true);
  });
});
