import { it, expect, vi } from 'vitest';
import { runCopilotIteration } from './copilotIteration';
import { consoleFetch } from '../api/console';
import { executeHarnessTool } from '../components/copilotHarness';

vi.mock('../api/console', () => ({ consoleFetch: vi.fn() }));
vi.mock('../components/copilotHarness', async importOriginal => ({
  ...await importOriginal(), executeHarnessTool: vi.fn(),
}));

function framesResponse(frames) {
  const encoded = new TextEncoder().encode(frames.map(frame => 'data: ' + JSON.stringify(frame)).join('\n') + '\ndata: [DONE]');
  return { ok: true, body: new ReadableStream({ start(controller) { controller.enqueue(encoded); controller.close(); } }) };
}

it('preserves signed fragmented tool history, execution order and reasoning totals across rounds', async () => {
  const toolSignature = { google: { thought_signature: 'signed-tool' } };
  const messageSignature = { google: { thought_signature: 'signed-message' } };
  consoleFetch.mockReset().mockResolvedValueOnce(framesResponse([
    { choices: [{ delta: { content: 'checking', reasoning_content: 'round one', extra_content: messageSignature, tool_calls: [
      { index: 1, id: 'call-second', function: { name: 'second', arguments: '{}' } },
      { index: 0, id: 'call-first', extra_content: toolSignature, function: { name: 'get_', arguments: '{"q":' } },
    ] } }] },
    { choices: [{ delta: { tool_calls: [{ index: 0, function: { name: 'models', arguments: '"x"}' } }] } }] },
    { choices: [], usage: { completion_tokens_details: { reasoning_tokens: 3 } } },
  ])).mockResolvedValueOnce(framesResponse([
    { choices: [{ delta: { content: 'answer', reasoning_content: 'round two' } }] },
    { choices: [], usage: { completion_tokens_details: { reasoning_tokens: 7 } } },
  ]));
  executeHarnessTool.mockReset().mockImplementation(async (name, args) => ({ result: { name, args } }));
  let visible = [{ id: 'reply', content: '' }];
  const options = { isZh: true, thinkingCapabilities: {}, thinkingByModel: {}, toolContext: {}, setMessages: update => { visible = update(visible); } };
  const run = { id: 'reply', model: 'model', keyId: '12', controller: new AbortController(), content: '', reasoning: '', reasoningTokens: 0, hasReasoningCount: false, trace: [] };
  const messages = [{ role: 'user', content: 'question' }];
  expect(await runCopilotIteration(options, run, messages)).toBe(true);
  expect(executeHarnessTool.mock.calls.map(([name, args]) => [name, args])).toEqual([['get_models', { q: 'x' }], ['second', {}]]);
  expect(messages[1]).toMatchObject({ role: 'assistant', content: 'checking', reasoning_content: 'round one', extra_content: messageSignature });
  expect(messages[1].tool_calls[0]).toEqual({ id: 'call-first', type: 'function', extra_content: toolSignature, function: { name: 'get_models', arguments: '{"q":"x"}' } });
  expect(messages.slice(2).map(message => message.tool_call_id)).toEqual(['call-first', 'call-second']);
  expect(await runCopilotIteration(options, run, messages)).toBe(false);
  const request = JSON.parse(consoleFetch.mock.calls[1][2].body);
  expect(request.messages).toEqual(messages);
  expect(run.content).toBe('checking\n\nanswer');
  expect(run.reasoning).toBe('round oneround two');
  expect(run.reasoningTokens).toBe(10);
  expect(visible[0]).toMatchObject({ content: 'checking\n\nanswer', thinking: { reasoning_tokens: 10 }, harnessTrace: run.trace });
});
