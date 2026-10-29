// AI Copilot: closing the panel stops the generation that is running.
//
// Regression for T-5 (was: hiding the panel kept the ReAct loop alive, so it kept
// calling the upstream model and executing tools for an answer nobody was waiting
// for). Fails on the pre-fix code because the request signal is never aborted.
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/AiCopilot.closeAborts.test.jsx

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import AiCopilot from './AiCopilot';

// A response whose body never produces a chunk: the request stays in flight.
function hangingStreamResponse() {
  return {
    ok: true,
    status: 200,
    body: {
      getReader: () => ({ read: () => new Promise(() => {}), releaseLock: () => {} }),
    },
  };
}

function renderCopilot(isOpen) {
  return render(
    <AiCopilot
      isOpen={isOpen}
      onClose={() => {}}
      onToggle={() => {}}
      setCurrentTab={() => {}}
      setViewMode={() => {}}
      models={['gpt-x']}
      channels={[]}
      keys={[]}
      logs={[]}
      modelRoutes={[]}
      prices={[]}
      stats={{}}
      lang="zh"
      showToast={() => {}}
      adminFetch={vi.fn()}
      initialQuery=""
      onClearInitialQuery={() => {}}
    />,
  );
}

describe('AiCopilot close cancels the generation', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('aborts the in-flight model request when the panel is closed', async () => {
    const signals = [];
    vi.stubGlobal('fetch', vi.fn((url, options) => {
      if (String(url).includes('/v1/chat/completions')) {
        signals.push(options?.signal);
        return Promise.resolve(hangingStreamResponse());
      }
      return Promise.resolve({ ok: true, status: 200, json: async () => ({ code: 0, data: {} }) });
    }));

    const { rerender } = renderCopilot(true);

    const input = screen.getByPlaceholderText('向 Harness 发出运维巡检、排错或架构指令...');
    fireEvent.change(input, { target: { value: '帮我看一下最近的运行情况' } });
    await act(async () => {
      fireEvent.submit(input.closest('form'));
    });

    await waitFor(() => expect(signals.length).toBe(1));
    expect(signals[0].aborted).toBe(false);

    // Closing the panel must cancel the request instead of leaving it running.
    rerender(
      <AiCopilot
        isOpen={false}
        onClose={() => {}}
        onToggle={() => {}}
        setCurrentTab={() => {}}
        setViewMode={() => {}}
        models={['gpt-x']}
        channels={[]}
        keys={[]}
        logs={[]}
        modelRoutes={[]}
        prices={[]}
        stats={{}}
        lang="zh"
        showToast={() => {}}
        adminFetch={vi.fn()}
        initialQuery=""
        onClearInitialQuery={() => {}}
      />,
    );

    await waitFor(() => expect(signals[0].aborted).toBe(true));
  });
});
