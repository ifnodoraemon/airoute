import { assistantContent, textContent } from './assistantContent';
import { consoleFetch } from '../api/console';
import { thinkingResult } from './thinking';
import { readSSELines } from './sseLines';

export async function runPlaygroundModel(options, call, targetModel, setters) {
  const state = {
    startTime: Date.now(), firstByteTime: null, content: '', reasoning: '',
    rawContent: '', explicitReasoning: '', observation: {}, extraContent: undefined, tokens: 0,
  };
  try {
    const request = options.buildChatRequest(targetModel, options.withSystemPrompt(call.messages, targetModel));
    const response = await consoleFetch(request.path, call.credentials.keyId, {
      method: request.method, headers: options.buildHeaders(),
      body: JSON.stringify(request.body), signal: call.controller.signal,
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      setters.setOutput(`[Error ${response.status}] ${error.error?.message || response.statusText}`);
      options.showToast?.(error.error?.message || response.statusText, 'error');
      return { failed: true };
    }
    if (options.playStream && response.body) await readStreamingResult(response, state, setters);
    else await readUnaryResult(response, state, setters);
    publishPerformance(state, setters);
    return { content: state.content, reasoning: state.reasoning, thinking: state.observation, extra_content: state.extraContent };
  } catch (error) {
    if (options.isAbortError(error)) return { content: state.content, reasoning: state.reasoning, aborted: true };
    options.showToast?.(error.message, 'error');
    if (!state.content) setters.setOutput(`[Error] ${error.message}`);
    return { content: state.content, reasoning: state.reasoning, failed: true };
  }
}

function publishContent(state, setters, complete = false) {
  const parsed = assistantContent(state.rawContent, state.explicitReasoning, complete);
  state.content = parsed.content;
  state.reasoning = parsed.reasoning;
  setters.setOutput(state.content);
  setters.setReasoning(state.reasoning);
}

function applyStreamingFrame(frame, state, setters) {
  if (frame.error) throw new Error(frame.error.message || String(frame.error));
  if (frame.usage?.completion_tokens) state.tokens = frame.usage.completion_tokens;
  state.observation = thinkingResult(frame, state.observation);
  setters.setThinking(state.observation);
  const delta = frame.choices?.[0]?.delta;
  if (delta?.extra_content) state.extraContent = delta.extra_content;
  state.explicitReasoning += textContent(delta?.reasoning_content);
  state.rawContent += textContent(delta?.content);
  publishContent(state, setters);
}

async function readStreamingResult(response, state, setters) {
  await readSSELines(response, line => {
    const trimmed = line.trim();
    if (!trimmed.startsWith('data:')) return;
    const data = trimmed.slice(5).trim();
    if (!data || data === '[DONE]') return;
    let frame;
    try { frame = JSON.parse(data); } catch { return; }
    applyStreamingFrame(frame, state, setters);
  }, {
    onBytes: () => {
      if (!state.firstByteTime) {
        state.firstByteTime = Date.now();
        setters.setTTFT(state.firstByteTime - state.startTime);
      }
    },
    onComplete: () => publishContent(state, setters, true),
  });
}

async function readUnaryResult(response, state, setters) {
  const data = await response.json();
  state.firstByteTime = Date.now();
  setters.setTTFT(state.firstByteTime - state.startTime);
  if (data.usage?.completion_tokens) state.tokens = data.usage.completion_tokens;
  state.observation = thinkingResult(data, state.observation);
  setters.setThinking(state.observation);
  const message = data.choices?.[0]?.message;
  state.extraContent = message?.extra_content;
  state.rawContent = textContent(message?.content);
  state.explicitReasoning = textContent(message?.reasoning_content);
  publishContent(state, setters, true);
}

function publishPerformance(state, setters) {
  const totalMs = Date.now() - state.startTime;
  setters.setDuration(totalMs);
  const tokens = state.tokens > 0 ? state.tokens : state.content.length * 0.75;
  const activeSec = (totalMs - (state.firstByteTime ? state.firstByteTime - state.startTime : 0)) / 1000;
  if (activeSec > 0.1) setters.setTPS((tokens / activeSec).toFixed(1));
}
