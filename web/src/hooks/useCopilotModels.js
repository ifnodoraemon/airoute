import { getGatewayOrigin } from '../config';
import { useConsoleKeySelection } from './useConsoleKeySelection';
import { useEffect, useMemo } from 'react';
import { inferClientModality } from '../lib/modelModality';
import { availableKeyModels } from '../lib/keyModels';

export default function useCopilotModels({
  inputRef, isOpen, isZh, keys, messages, modelRoutes, models, onToggle, selectedModel, setMessages,
  setSelectedModel
}) {
  const origin = getGatewayOrigin();
  const baseUrl = origin ? `${origin}/v1` : '/v1';

  const { availableKeys: contextKeys, selectedKey: selectedKeyObj, keyId: selectedKeyId, setKeyId: setSelectedKeyId } = useConsoleKeySelection(keys);

  const candidateModels = useMemo(() => {
    let list = [];
    if (modelRoutes && modelRoutes.length > 0) {
      const chatRoutes = modelRoutes.filter(r => r.modality === 'chat' || !r.modality).map(r => r.model);
      if (chatRoutes.length > 0) list = chatRoutes;
    }
    if (list.length === 0 && models && models.length > 0) {
      const chatModels = models.filter(m => inferClientModality(m) === 'chat');
      list = chatModels.length > 0 ? chatModels : models;
    }
    // A selected key can restrict the models it may call; offering the rest would
    // produce 403s the operator cannot explain from this panel.
    const allowed = selectedKeyObj?.allowed_models || [];
    if (allowed.length > 0 && !allowed.includes('*')) {
      list = availableKeyModels(selectedKeyObj, list);
    }
    return Array.from(new Set(list));
  }, [models, modelRoutes, selectedKeyObj]);

  // Keep the driving model inside the (possibly narrowed) candidate list.
  useEffect(() => {
    if (candidateModels.length === 0) { setSelectedModel(''); return; }
    if (selectedModel && candidateModels.includes(selectedModel)) return;
    const preferred = candidateModels.find(m => m.includes('flash') || m.includes('deepseek') || m.includes('glm')) || candidateModels[0];
    setSelectedModel(preferred);
  }, [candidateModels, selectedModel, setSelectedModel]);

  // Seed a concise welcome for a new conversation.
  useEffect(() => {
    if (messages.length === 0) {
      setMessages([
        {
          id: 'welcome',
          role: 'assistant',
          content: isZh
            ? '选择一个任务，或直接提问。'
            : 'Choose a task or ask a question.',
          harnessTrace: []
        }
      ]);
    }
    // messages.length is read above, so it belongs in the list: seeding raises the
    // length, which stops the effect from re-seeding.
  }, [isZh, messages.length, setMessages]);

  // Global shortcut: Cmd+J or Ctrl+J to toggle Copilot
  useEffect(() => {
    const handleKeyDown = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'j') {
        e.preventDefault();
        onToggle();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onToggle]);

  // Focus input when opened
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 150);
    }
  }, [inputRef, isOpen]);
  return {
    baseUrl, candidateModels, contextKeys, origin, selectedKeyId, setSelectedKeyId
  };
}
