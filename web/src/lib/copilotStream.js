import { readSSELines } from './sseLines';
import { thinkingResult } from './thinking';
import { extractErrorMessage } from '../api/client';

export async function readCopilotStream(response, options, run) {
  const round = { content: '', reasoning: '', observation: {}, extra: undefined, toolCalls: {} };
  await readSSELines(response, line => {
    const trimmed = line.trim();
    if (!trimmed.startsWith('data:')) return;
    const data = trimmed.slice(5).trim();
    if (!data || data === '[DONE]') return;
    let frame;
    try { frame = JSON.parse(data); } catch { return; }
    applyCopilotFrame(frame, options, run, round);
  }, { ignoreReleaseErrors: true });
  if (Number.isFinite(round.observation.reasoning_tokens)) {
    run.reasoningTokens += round.observation.reasoning_tokens;
    run.hasReasoningCount = true;
  }
  return round;
}

function applyCopilotFrame(frame, options, run, round) {
  if (!frame.choices?.length && (frame.error || frame.type === 'error')) {
    const failure = new Error(extractErrorMessage(frame, options.isZh ? '模型流式响应出错' : 'Model stream error'));
    failure.streamError = true;
    throw failure;
  }
  round.observation = thinkingResult(frame, round.observation);
  const observation = { ...round.observation };
  if (Number.isFinite(round.observation.reasoning_tokens)) observation.reasoning_tokens = run.reasoningTokens + round.observation.reasoning_tokens;
  else if (run.hasReasoningCount) observation.reasoning_tokens = run.reasoningTokens;
  options.setMessages(previous => previous.map(message => message.id === run.id ? { ...message, thinking: observation } : message));
  const delta = frame.choices?.[0]?.delta;
  if (!delta) return;
  if (delta.extra_content) round.extra = delta.extra_content;
  if (delta.reasoning_content) {
    round.reasoning += delta.reasoning_content;
    run.reasoning += delta.reasoning_content;
  }
  if (delta.content) {
    round.content += delta.content;
    run.content += delta.content;
  }
  accumulateToolCalls(round.toolCalls, delta.tool_calls);
  options.setMessages(previous => previous.map(message => message.id === run.id ? {
    ...message, content: run.content, reasoning: run.reasoning, harnessTrace: [...run.trace],
    extra_content: round.extra, model: run.model, thinking: observation,
    reasoning_content: round.reasoning,
  } : message));
}

function accumulateToolCalls(calls, deltas = []) {
  for (const delta of deltas) {
    const index = delta.index ?? 0;
    if (!calls[index]) calls[index] = { id: delta.id || `call_${Date.now()}_${index}`, name: '', arguments: '' };
    const call = calls[index];
    if (delta.id) call.id = delta.id;
    if (delta.extra_content) call.extra_content = delta.extra_content;
    if (delta.function?.name) call.name += delta.function.name;
    if (delta.function?.arguments) call.arguments += delta.function.arguments;
  }
}
