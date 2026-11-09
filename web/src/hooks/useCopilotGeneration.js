import { assistantHistory } from '../lib/thinking';
import { runCopilotIteration } from '../lib/copilotIteration';

export default function useCopilotGeneration(options) {
  const handleSend = async (queryText = options.input) => {
    const text = (queryText || '').trim();
    if (!canSend(options, text)) return;
    const run = startGeneration(options, text);
    try {
      const history = [
        { role: 'system', content: options.buildHarnessSystemPrompt() },
        ...options.messages
          .filter(message => message.id !== 'welcome' && !message.id.startsWith('err_') && !message.isError && !message.isFallback)
          .slice(-6).map(message => assistantHistory(message, true, run.model)),
        { role: 'user', content: text },
      ];
      let iteration = 0;
      let shouldContinue = true;
      while (shouldContinue && iteration < 4) {
        iteration++;
        shouldContinue = await runCopilotIteration(options, run, history);
      }
    } catch (error) {
      if (error.name === 'AbortError') return;
      await failGeneration(options, run, error);
    } finally {
      finishGeneration(options, run);
    }
  };
  return { handleSend };
}

function canSend(options, text) {
  if (!text || options.isLoading) return false;
  if (!options.selectedKeyId) {
    options.showToast?.(options.isZh ? '请先创建并选择一把可用的 API 访问密钥。' : 'Create and select an active API key first.', 'warning');
    return false;
  }
  if (!options.candidateModels.includes(options.selectedModel)) {
    options.showToast?.(options.isZh ? '所选密钥暂无可用的对话模型，请切换密钥或配置模型权限。' : 'No chat model is available for this key. Select another key or update model access.', 'warning');
    return false;
  }
  return true;
}

function startGeneration(options, text) {
  const userId = `u_${Date.now()}`;
  const id = `a_${Date.now()}`;
  options.setMessages(previous => [...previous, { id: userId, role: 'user', content: text }]);
  options.setInput('');
  options.setIsLoading(true);
  const controller = new AbortController();
  options.abortControllerRef.current = controller;
  options.setMessages(previous => [...previous, {
    id, role: 'assistant', content: '', reasoning: '', harnessTrace: [], actions: [],
  }]);
  return {
    id, text, controller, keyId: options.selectedKeyId, model: options.selectedModel,
    content: '', reasoning: '', reasoningTokens: 0, hasReasoningCount: false, trace: [],
  };
}

async function failGeneration(options, run, error) {
  let message = error?.message || (options.isZh ? '请求失败' : 'Request failed');
  if (error?.gatewayCode === 'api_key_revoked' || error?.gatewayCode === 'key_access_denied') {
    message = options.isZh
      ? '所选密钥已被删除、禁用或不可访问。请在「API 密钥」页确认状态，然后选择一把可用密钥。'
      : 'The selected key was deleted or disabled, or is no longer accessible. Check the API Keys page and select an active key.';
  }
  options.showToast?.(message, 'error');
  const fallback = run.content ? null : await options.executeLocalHarnessDiagnostic(run.text);
  if (fallback) {
    run.content = fallback.text;
    run.trace.push(...fallback.trace);
  }
  options.setMessages(previous => previous.map(item => item.id === run.id ? {
    ...item, content: run.content, reasoning: run.reasoning, harnessTrace: [...run.trace],
    isError: true, errorMessage: message, isFallback: Boolean(fallback),
  } : item));
}

function finishGeneration(options, run) {
  options.setIsLoading(false);
  options.setActiveBenchmark(null);
  options.abortControllerRef.current = null;
  options.setMessages(previous => previous.map(message => message.id === run.id && (!message.actions || !message.actions.length)
    ? { ...message, actions: options.deriveActionsFromContent(message.content) } : message));
}
