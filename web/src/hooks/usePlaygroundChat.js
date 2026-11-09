import { runPlaygroundModel } from '../lib/playgroundModelRun';

export default function usePlaygroundChat(options) {
  return { handleRunChat: () => runChat(options) };
}

async function runChat(options) {
  const { playModel, arenaModelB, playMode, resolveCallCredentials, abortRef } = options;
  const credentials = resolveCallCredentials('chat', playMode === 'arena' ? [playModel, arenaModelB] : [playModel]);
  if (!credentials) return;
  const messages = nextChatMessages(options);
  if (!messages?.length) return;
  beginChat(options, messages);
  const controller = new AbortController();
  abortRef.current = controller;
  resetChatOutputs(options);
  const call = { credentials, messages, controller };
  const promises = [runPlaygroundModel(options, call, playModel, {
    setOutput: options.setPlayOutput, setReasoning: options.setPlayReasoningOutput,
    setTTFT: options.setPlayTTFTMs, setDuration: options.setPlayDurationMs,
    setTPS: options.setPlayTokensPerSec, setThinking: options.setPlayThinking,
  })];
  if (playMode === 'arena' && arenaModelB) {
    promises.push(runPlaygroundModel(options, call, arenaModelB, {
      setOutput: options.setArenaOutputB, setReasoning: options.setArenaReasoningB,
      setTTFT: options.setArenaTTFTB, setDuration: options.setArenaDurationB,
      setTPS: options.setArenaTokensPerSecB, setThinking: options.setArenaThinkingB,
    }));
  }
  try {
    const results = await Promise.all(promises);
    commitChatResult(options, results);
  } finally {
    options.setPlayLoading(false);
    abortRef.current = null;
  }
}

function nextChatMessages({ chatInput, messages }) {
  const typed = chatInput.trim();
  if (typed) return [...messages, { role: 'user', content: typed }];
  const last = messages[messages.length - 1];
  return last?.role === 'user' ? messages : null;
}

function beginChat(options, messages) {
  options.setMessages(messages);
  options.setChatInput('');
  options.setPlayLoading(true);
  options.setPlayStopped(false);
}

function resetChatOutputs(options) {
  options.setPlayOutput('');
  options.setPlayReasoningOutput('');
  options.setPlayTTFTMs(0);
  options.setPlayDurationMs(0);
  options.setPlayTokensPerSec(0);
  if (options.playMode === 'arena') {
    options.setArenaOutputB('');
    options.setArenaReasoningB('');
    options.setArenaThinkingB(null);
    options.setArenaTTFTB(0);
    options.setArenaDurationB(0);
    options.setArenaTokensPerSecB(0);
  }
}

function commitChatResult(options, results) {
  const aborted = results.some(result => result && result.aborted);
  if (aborted || results.some(result => result?.failed) || options.playMode !== 'single' || !results[0]?.content) return;
  const result = results[0];
  options.setMessages(previous => [...previous, {
    role: 'assistant', content: result.content, reasoning: result.reasoning,
    thinking: result.thinking, model: options.playModel, extra_content: result.extra_content,
  }]);
  options.setPlayOutput('');
  options.setPlayReasoningOutput('');
}
