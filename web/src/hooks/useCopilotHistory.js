import { copyToClipboard as crossOriginCopy } from '../config';

export default function useCopilotHistory({
  abortControllerRef, isZh, setCopiedId, setIsLoading, setMessages, showToast
}) {
  const handleCopy = async (text, id) => {
    await crossOriginCopy(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
    if (showToast) showToast(isZh ? '已复制到剪贴板' : 'Copied to clipboard', 'success');
  };

  const handleClearHistory = () => {
    setMessages([
      {
        id: `welcome_${Date.now()}`,
        role: 'assistant',
        content: isZh
          ? `会话已重置。**Airoute Agent Harness** 就绪，请随时提问！`
          : `Conversation reset. **Airoute Agent Harness** ready for your questions!`,
        harnessTrace: []
      }
    ]);
  };

  const handleStopGeneration = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setIsLoading(false);
  };
  return {
    handleClearHistory, handleCopy, handleStopGeneration
  };
}
