import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fireEvent } from '@testing-library/react';
import { pushEscapeLayer, hasEscapeLayers, resetEscapeLayers } from './escapeLayer';

function pressEscape() {
  fireEvent.keyDown(window, { key: 'Escape' });
}

describe('escape layer stack', () => {
  beforeEach(() => {
    resetEscapeLayers();
  });

  afterEach(() => {
    resetEscapeLayers();
  });

  it('hands Escape to the most recently opened layer only', () => {
    const lower = vi.fn();
    const upper = vi.fn();
    const removeLower = pushEscapeLayer(lower);
    pushEscapeLayer(upper);

    pressEscape();

    expect(upper).toHaveBeenCalledTimes(1);
    expect(lower).not.toHaveBeenCalled();
    removeLower();
  });

  it('falls back to the layer below once the top one closes', () => {
    const lower = vi.fn();
    const upper = vi.fn();
    pushEscapeLayer(lower);
    const removeUpper = pushEscapeLayer(upper);

    pressEscape();
    removeUpper();
    pressEscape();

    expect(upper).toHaveBeenCalledTimes(1);
    expect(lower).toHaveBeenCalledTimes(1);
  });

  it('reports whether any layer is open', () => {
    expect(hasEscapeLayers()).toBe(false);
    const remove = pushEscapeLayer(() => {});
    expect(hasEscapeLayers()).toBe(true);
    remove();
    expect(hasEscapeLayers()).toBe(false);
  });

  it('does nothing when no layer is registered', () => {
    expect(() => pressEscape()).not.toThrow();
  });
});
