import useCopilotGeneration from '../hooks/useCopilotGeneration';
import useCopilotHarness from '../hooks/useCopilotHarness';
import useCopilotHistory from '../hooks/useCopilotHistory';
import useCopilotLifecycle from '../hooks/useCopilotLifecycle';
import useCopilotModels from '../hooks/useCopilotModels';
import useCopilotDrag from '../hooks/useCopilotDrag';
import useCopilotState from '../hooks/useCopilotState';
import CopilotWindow from './CopilotWindow';

import { useEffect } from 'react';
import { Sparkles, GripVertical } from 'lucide-react';

export default function AiCopilot({
  isOpen,
  onClose,
  onToggle,
  setCurrentTab,
  setViewMode,
  models = [],
  thinkingCapabilities = {},
  channels = [],
  keys = [],
  logs = [],
  modelRoutes = [],
  prices = [],
  stats = {},
  lang = 'zh',

  showToast,
  adminFetch,
  initialQuery = '',
  onClearInitialQuery,
  onCreateKey
}) {
  const isZh = lang === 'zh';
  const {
    abortControllerRef, activeBenchmark, copiedId, expandedReasoningIds, expandedStepObsIds, expandedTraceIds,
    input, inputRef, isExpanded, isLoading, messages, messagesEndRef, selectedModel, setActiveBenchmark,
    setCopiedId, setExpandedReasoningIds, setExpandedStepObsIds, setExpandedTraceIds, setInput, setIsExpanded,
    setIsLoading, setMessages, setSelectedModel, setShowToolsModal, setThinkingByModel, showToolsModal,
    thinkingByModel
  } = useCopilotState({

  });

  // Floating trigger button draggable state
  const {
    btnPos, btnRef, handleBtnClick, handleBtnPointerDown, handleModalHeaderPointerDown, isBtnDragging, modalPos,
    modalRef
  } = useCopilotDrag({
    isExpanded, onToggle
  });

  const {
    baseUrl, candidateModels, contextKeys, origin, selectedKeyId, setSelectedKeyId
  } = useCopilotModels({
    inputRef, isOpen, isZh, keys, messages, modelRoutes, models, onToggle, selectedModel, setMessages,
    setSelectedModel
  });

  // Closing the panel (or leaving the console) cancels the generation that is in
  // flight. Hiding the panel is not "keep working invisibly": the ReAct loop would
  // otherwise keep issuing upstream requests and executing tools (up to the
  // iteration cap) for an answer nobody is waiting for — and that costs quota.
  const {
    handleSendRef
  } = useCopilotLifecycle({
    abortControllerRef, initialQuery, isOpen, messages, messagesEndRef, onClearInitialQuery, setIsLoading
  });

  const {
    handleClearHistory, handleCopy, handleStopGeneration
  } = useCopilotHistory({
    abortControllerRef, isZh, setCopiedId, setIsLoading, setMessages, showToast
  });

  // Build System Prompt for the Harness Agent
  const {
    buildHarnessSystemPrompt, deriveActionsFromContent, executeLocalHarnessDiagnostic, toolContext
  } = useCopilotHarness({
    adminFetch, baseUrl, channels, isZh, keys, logs, modelRoutes, models, origin, prices, setCurrentTab,
    setViewMode, showToast, stats
  });

  // Main ReAct Harness Loop
  // Not memoised on purpose: it closes over the current input, so a stable identity
  // would need input as a dependency, which would rebuild it on every keystroke
  // anyway. The one effect that needs it reads it through a ref instead.
  const {
    handleSend
  } = useCopilotGeneration({
    abortControllerRef, buildHarnessSystemPrompt, candidateModels, deriveActionsFromContent,
    executeLocalHarnessDiagnostic, input, isLoading, isZh, messages, selectedKeyId, selectedModel,
    setActiveBenchmark, setInput, setIsLoading, setMessages, showToast, thinkingByModel, thinkingCapabilities,
    toolContext
  });

  // Keeps the ref above pointing at the newest closure. Runs after every render on
  // purpose, which is why it is not a dependency of anything.
  useEffect(() => {
    handleSendRef.current = handleSend;
  }, [handleSend, handleSendRef]);

  const toggleReasoning = (id) => {
    setExpandedReasoningIds(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const toggleTrace = (id) => {
    setExpandedTraceIds(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const toggleStepObs = (stepKey) => {
    setExpandedStepObsIds(prev => ({ ...prev, [stepKey]: !prev[stepKey] }));
  };

  if (!isOpen) {
    const style = btnPos
      ? { left: `${btnPos.x}px`, top: `${btnPos.y}px` }
      : { right: '24px', bottom: '96px' };

    return (
      <button
        ref={btnRef}
        type="button"
        onPointerDown={handleBtnPointerDown}
        onClick={handleBtnClick}
        style={style}
        className={`fixed z-40 flex items-center space-x-2 px-3.5 py-2.5 rounded-full bg-[#355CFF] hover:bg-[#2849E6] text-white shadow-xl shadow-[#355CFF]/25 hover:shadow-[#355CFF]/35 border border-white/20 select-none touch-none ${
          isBtnDragging ? 'cursor-grabbing scale-105 opacity-90 shadow-2xl' : 'cursor-grab hover:scale-[1.02] transition-transform transition-shadow duration-200'
        }`}
        aria-label={isZh ? 'AI 智能副驾驶' : 'AI Copilot'}
        title={isZh ? '副驾驶 · Cmd+J · 可拖动' : 'Copilot · Cmd+J · Drag to move'}
      >
        <GripVertical className="w-3.5 h-3.5 text-white/50 -ml-1 mr-0.5 shrink-0" />
        <div className="relative">
          <Sparkles className="w-4 h-4 text-amber-300 animate-pulse" />
        </div>
        <span className="text-xs font-bold tracking-tight">
          {isZh ? '副驾驶' : 'Copilot'}
        </span>
        <kbd className="hidden sm:inline-flex items-center px-1.5 py-0.5 text-[10px] font-mono font-semibold bg-white/20 text-white/90 rounded-sm border border-white/20">
          ⌘J
        </kbd>
      </button>
    );
  }

  return (
    <CopilotWindow {...{
      activeBenchmark, candidateModels, contextKeys, copiedId, expandedReasoningIds, expandedStepObsIds,
      expandedTraceIds, handleClearHistory, handleCopy, handleModalHeaderPointerDown, handleSend,
      handleStopGeneration, input, inputRef, isExpanded, isLoading, isZh, lang, messages, messagesEndRef,
      modalPos, modalRef, onClose, onCreateKey, selectedKeyId, selectedModel, setActiveBenchmark, setCurrentTab,
      setInput, setIsExpanded, setSelectedKeyId, setSelectedModel, setShowToolsModal, setThinkingByModel,
      setViewMode, showToolsModal, thinkingByModel, thinkingCapabilities, toggleReasoning, toggleStepObs,
      toggleTrace
    }} />
  );
}
