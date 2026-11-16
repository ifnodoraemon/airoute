import { describe, it, expect } from 'vitest';
import { formatDiscountLabel, formatSlotPreview, formatDaysLabel, parseMinutes } from './pricingSlots';

describe('formatDiscountLabel', () => {
  it('renders the zh 折 convention', () => {
    expect(formatDiscountLabel(0.5, true)).toBe('5 折 (50%)');
    expect(formatDiscountLabel(0.85, true)).toBe('8.5 折 (85%)');
    expect(formatDiscountLabel(0.1, true)).toBe('1 折 (10%)');
  });
  it('renders the en percent-off convention', () => {
    expect(formatDiscountLabel(0.5, false)).toBe('50% Off (50%)');
    expect(formatDiscountLabel(0.1, false)).toBe('90% Off (10%)');
  });
  it('supports custom fractions outside the old presets', () => {
    expect(formatDiscountLabel(0.25, true)).toBe('2.5 折 (25%)');
    expect(formatDiscountLabel(0.95, false)).toBe('5% Off (95%)');
  });
  it('flags invalid fractions', () => {
    expect(formatDiscountLabel(0, true)).toBe('无效折扣');
    expect(formatDiscountLabel(1.5, true)).toBe('无效折扣');
    expect(formatDiscountLabel(NaN, false)).toBe('Invalid');
  });
});

describe('formatSlotPreview', () => {
  it('combines days, window and discount', () => {
    const slot = { start: '00:00', end: '08:30', discount: 0.5, days: [] };
    expect(formatSlotPreview(slot, true)).toBe('每天 00:00–08:30 · 5 折 (50%)');
  });
  it('uses the weekday label when set', () => {
    const slot = { start: '22:00', end: '06:00', discount: 0.3, days: [1, 2, 3, 4, 5] };
    expect(formatSlotPreview(slot, true)).toBe('工作日 22:00–06:00 · 3 折 (30%)');
  });
  it('handles an empty slot gracefully', () => {
    expect(formatSlotPreview(null)).toBe('');
  });
});

describe('existing helpers stay intact', () => {
  it('formatDaysLabel collapses weekends', () => {
    expect(formatDaysLabel([6, 7], true)).toBe('周末');
    expect(formatDaysLabel([], true)).toBe('');
  });
  it('parseMinutes validates HH:MM', () => {
    expect(parseMinutes('08:30')).toBe(510);
    expect(parseMinutes('25:00')).toBeNull();
    expect(parseMinutes('bad')).toBeNull();
  });
});