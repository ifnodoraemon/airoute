import { describe, it, expect, vi } from 'vitest';
import { readSSELines } from './sseLines';

function streamReader(chunks, { readError, releaseError } = {}) {
  const reader = {
    read: vi.fn(async () => {
      if (chunks.length) return { value: chunks.shift(), done: false };
      if (readError) throw readError;
      return { done: true };
    }),
    cancel: vi.fn(async () => {}),
    releaseLock: vi.fn(() => { if (releaseError) throw releaseError; }),
  };
  return { response: { body: { getReader: () => reader } }, reader };
}

describe('readSSELines', () => {
  it('preserves split UTF-8 and the final unterminated line, then completes before release', async () => {
    const bytes = new TextEncoder().encode('data: {"text":"中文"}\n\ndata: [DONE]');
    const { response, reader } = streamReader([bytes.slice(0, 16), bytes.slice(16, 19), bytes.slice(19)]);
    const calls = [];
    await readSSELines(response, line => calls.push(line), {
      onBytes: chunk => calls.push(chunk.byteLength),
      onComplete: () => {
        expect(reader.cancel).not.toHaveBeenCalled();
        calls.push('complete');
      },
    });
    expect(calls).toEqual([16, 3, bytes.length - 19, 'data: {"text":"中文"}', '', 'data: [DONE]', 'complete']);
    expect(reader.cancel).toHaveBeenCalledOnce();
    expect(reader.releaseLock).toHaveBeenCalledOnce();
  });

  it.each(['read', 'parse'])('releases the reader and propagates a %s failure without completing', async (kind) => {
    const error = new Error(kind);
    const complete = vi.fn();
    const { response, reader } = streamReader([new TextEncoder().encode('data: invalid\n')], {
      readError: kind === 'read' ? error : null,
    });
    await expect(readSSELines(response, () => { if (kind === 'parse') throw error; }, { onComplete: complete })).rejects.toBe(error);
    expect(complete).not.toHaveBeenCalled();
    expect(reader.cancel).toHaveBeenCalledOnce();
    expect(reader.releaseLock).toHaveBeenCalledOnce();
  });

  it('retains each caller’s release-error policy', async () => {
    const error = new Error('release failed');
    await expect(readSSELines(streamReader([], { releaseError: error }).response, vi.fn())).rejects.toBe(error);
    await expect(readSSELines(streamReader([], { releaseError: error }).response, vi.fn(), { ignoreReleaseErrors: true })).resolves.toBeUndefined();
  });
});
