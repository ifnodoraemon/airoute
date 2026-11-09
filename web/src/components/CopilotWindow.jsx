import CopilotHeader from './CopilotHeader';
import { X, Send, StopCircle, Key, Layers } from 'lucide-react';
import { COPILOT_HARNESS_TOOLS, HARNESS_BENCHMARKS } from './copilotHarness';
import CopilotMessages from './CopilotMessages';
import { keyDisplayRef } from '../lib/apiKey';
import ThinkingControls from './ThinkingControls';

export default function CopilotWindow({
  activeBenchmark, candidateModels, contextKeys, copiedId, expandedReasoningIds, expandedStepObsIds,
  expandedTraceIds, handleClearHistory, handleCopy, handleModalHeaderPointerDown, handleSend,
  handleStopGeneration, input, inputRef, isExpanded, isLoading, isZh, lang, messages, messagesEndRef,
  modalPos, modalRef, onClose, onCreateKey, selectedKeyId, selectedModel, setActiveBenchmark, setCurrentTab,
  setInput, setIsExpanded, setSelectedKeyId, setSelectedModel, setShowToolsModal, setThinkingByModel,
  setViewMode, showToolsModal, thinkingByModel, thinkingCapabilities, toggleReasoning, toggleStepObs,
  toggleTrace
}) {
  return (
    <div
      ref={modalRef}
      style={!isExpanded && modalPos ? { left: `${modalPos.x}px`, top: `${modalPos.y}px`, bottom: 'auto', right: 'auto' } : undefined}
      className={`fixed z-50 transition-all duration-300 ease-out flex flex-col ${
        isExpanded
          ? 'inset-4 sm:inset-10 bg-white dark:bg-[#191B1F] rounded-2xl shadow-2xl border border-[#E4E6E9] dark:border-[#2C3038]'
          : `${modalPos ? '' : 'bottom-6 right-6'} w-full max-w-[420px] h-[640px] max-h-[88vh] bg-white dark:bg-[#191B1F] rounded-2xl shadow-2xl border border-[#E4E6E9] dark:border-[#2C3038]`
      } overflow-hidden animate-in zoom-in-95`}
    >
      {/* 头部控制栏 (可按住拖动) */}
      <CopilotHeader {...{
        candidateModels, handleClearHistory, handleModalHeaderPointerDown, isExpanded, isZh, onClose, selectedModel,
        setIsExpanded, setSelectedModel, setShowToolsModal, showToolsModal
      }} />

      {/* 标准工具契约抽屉 (Tools Schema Modal) */}
      {showToolsModal && (
        <div className="px-4 py-2.5 bg-[#F7F7F5] dark:bg-[#22252B] border-b border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#202124] dark:text-[#F1F2F4] animate-in slide-in-from-top-2">
          <div className="flex items-center justify-between mb-1.5">
            <div className="flex items-center gap-1.5 font-semibold text-[#202124] dark:text-[#F1F2F4] text-xs">
              <Layers className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
              <span>{isZh ? '可用工具' : 'Available tools'}</span>
            </div>
            <button
              onClick={() => setShowToolsModal(false)}
              className="text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
          <div className="space-y-1.5 max-h-36 overflow-y-auto pr-1 text-xs">
            {COPILOT_HARNESS_TOOLS.map((t, idx) => (
              <div key={idx} className="p-1.5 rounded-lg bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038]">
                <div className="font-mono font-semibold text-[#355CFF] dark:text-[#91A7FF] text-xs">
                  {t.function.name}
                </div>
                <div className="text-xs text-[#656B75] dark:text-[#A1A7B0] line-clamp-1">
                  {t.function.description}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 消息对话容器 */}
      <CopilotMessages {...{
        copiedId, expandedReasoningIds, expandedStepObsIds, expandedTraceIds, handleCopy, isExpanded, isLoading,
        isZh, lang, messages, messagesEndRef, onClose, setCurrentTab, setViewMode, toggleReasoning, toggleStepObs,
        toggleTrace
      }} />

      {/* 标准 Harness Benchmark 基准任务快捷栏 */}
      <div className="px-3 py-2 bg-[#F7F7F5] dark:bg-[#191B1F] border-t border-[#E4E6E9] dark:border-[#2C3038] flex items-center gap-2 overflow-x-auto text-xs no-scrollbar">
        {HARNESS_BENCHMARKS.map((bm) => (
          <button
            key={bm.id}
            onClick={() => {
              setActiveBenchmark(bm.id);
              handleSend(bm.prompt);
            }}
            disabled={isLoading}
            aria-pressed={activeBenchmark === bm.id}
            className={`px-2.5 py-1.5 rounded-xl border whitespace-nowrap transition cursor-pointer font-medium shadow-2xs disabled:opacity-50 flex items-center gap-1.5 ${
              activeBenchmark === bm.id
                ? 'bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border-[#355CFF] dark:border-[#91A7FF]'
                : 'bg-white dark:bg-[#22252B] hover:bg-[#F0F1F2] dark:hover:bg-[#2c3038] text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] border-[#E4E6E9] dark:border-[#2C3038]'
            }`}
            title={bm.description}
          >
            <span className="px-1 py-0.2 rounded text-xs font-mono bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20 dark:border-[#91A7FF]/30 font-semibold">
              {bm.code}
            </span>
            <span>{isZh ? bm.label : bm.enLabel}</span>
          </button>
        ))}
      </div>

      <div className="px-3 py-1.5 border-t border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] space-y-1.5">
        <div className="flex items-center gap-2">
          <Key className={`w-3 h-3 shrink-0 ${selectedKeyId ? 'text-emerald-500' : 'text-[#9AA0A6]'}`} />
          <select value={selectedKeyId} onChange={e => setSelectedKeyId(e.target.value)}
            disabled={!contextKeys.length || isLoading}
            aria-label={isZh ? '选择 API 访问密钥' : 'Select API key'}
            className="min-w-0 max-w-[240px] bg-[#F7F7F5] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] rounded-lg px-2 py-1 text-xs font-mono text-[#202124] dark:text-[#F1F2F4] focus:outline-none focus:ring-1 focus:ring-[#355CFF] cursor-pointer truncate">
            {!contextKeys.length && <option value="">{isZh ? '暂无可用密钥' : 'No active keys'}</option>}
            {contextKeys.map(k => <option key={k.id} value={String(k.id)}>{k.tenant_id ? `${k.tenant_id} - ` : ''}{keyDisplayRef(k)}</option>)}
          </select>
          {selectedKeyId && <span className="text-[11px] text-emerald-600 dark:text-emerald-400">{isZh ? '已就绪' : 'Ready'}</span>}
        </div>
        {!selectedKeyId && <div className="flex items-center justify-between gap-2 text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
          <span>{isZh ? '创建访问密钥后即可使用副驾驶。' : 'Create an API key to use Copilot.'}</span>
          <button type="button" className="text-indigo-600 hover:underline" onClick={() => {
            setViewMode?.('console'); setCurrentTab?.('keys'); onCreateKey?.();
          }}>{isZh ? '创建访问密钥' : 'Create API key'}</button>
        </div>}
      </div>

      <div className="px-3 py-2 border-t border-slate-200 dark:border-slate-700"><ThinkingControls capability={thinkingCapabilities[selectedModel]} value={thinkingByModel[selectedModel]} onChange={value => setThinkingByModel(prev => ({...prev,[selectedModel]:value}))} tools disabled={isLoading} lang={lang} /></div>
      {/* 底部输入栏 */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          handleSend();
        }}
        className="p-3 border-t border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] flex items-center space-x-2"
      >
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={isZh ? '输入问题或操作指令' : 'Ask a question or describe a task'}
          className="flex-1 bg-[#F7F7F5] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl px-3.5 py-2 text-xs text-[#202124] dark:text-[#F1F2F4] placeholder-[#656B75] dark:placeholder-[#A1A7B0] focus:outline-none focus:ring-1 focus:ring-[#355CFF] shadow-2xs"
        />

        {isLoading ? (
          <button
            type="button"
            onClick={handleStopGeneration}
            className="p-2 rounded-xl bg-rose-600 hover:bg-rose-500 text-white transition cursor-pointer shadow-sm"
            title={isZh ? '中断任务' : 'Abort task'}
          >
            <StopCircle className="w-4 h-4" />
          </button>
        ) : (
          <button
            type="submit"
            disabled={!input.trim()}
            className="p-2 rounded-xl bg-[#355CFF] hover:bg-[#2849E6] disabled:opacity-40 text-white transition cursor-pointer shadow-md shadow-[#355CFF]/20"
            title={isZh ? '发送' : 'Send'}
          >
            <Send className="w-4 h-4" />
          </button>
        )}
      </form>
    </div>
  );
}
