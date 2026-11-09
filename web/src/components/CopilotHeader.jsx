import { Sparkles, X, Cpu, Minimize2, Maximize2, Trash2, Wrench } from 'lucide-react';
import { COPILOT_HARNESS_TOOLS } from './copilotHarness';

export default function CopilotHeader({
  candidateModels, handleClearHistory, handleModalHeaderPointerDown, isExpanded, isZh, onClose, selectedModel,
  setIsExpanded, setSelectedModel, setShowToolsModal, showToolsModal
}) {
  return (
    <div
      onPointerDown={handleModalHeaderPointerDown}
      className={`px-4 py-3 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between bg-white dark:bg-[#191B1F] ${
        isExpanded ? '' : 'cursor-grab active:cursor-grabbing select-none'
      }`}
      title={isExpanded ? '' : (isZh ? '按住此处可拖动窗口位置' : 'Drag header to move window')}
    >
      <div className="flex items-center space-x-2.5 min-w-0">
        <div className="w-8 h-8 rounded-xl bg-[#355CFF] flex items-center justify-center text-white shadow-sm shrink-0">
          <Sparkles className="w-4 h-4 text-amber-300" />
        </div>
        <div className="min-w-0">
          <div className="flex items-center space-x-2">
            <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4] truncate">
              {isZh ? '副驾驶' : 'Copilot'}
            </h3>
          </div>

          {/* 模型与工具契约展示 */}
          <div className="flex items-center gap-2 mt-0.5 text-xs text-[#656B75] dark:text-[#A1A7B0]">
            <div className="flex items-center gap-1">
              <Cpu className="w-3 h-3 text-[#355CFF] dark:text-[#91A7FF] shrink-0" />
              {candidateModels.length > 0 ? (
                <select
                  value={selectedModel}
                  onChange={(e) => setSelectedModel(e.target.value)}
                  aria-label={isZh ? '副驾驶驱动模型' : 'Copilot model'}
                  className="bg-transparent text-[#202124] dark:text-[#F1F2F4] font-mono text-xs border-none focus:outline-none cursor-pointer truncate max-w-[130px]"
                  title={isZh ? '切换副驾驶驱动模型' : 'Switch Copilot Model'}
                >
                  {candidateModels.map(m => (
                    <option key={m} value={m} className="bg-white dark:bg-[#191B1F] text-[#202124] dark:text-[#F1F2F4]">
                      {m}
                    </option>
                  ))}
                </select>
              ) : (
                <span className="font-mono text-xs text-[#656B75] dark:text-[#A1A7B0]">auto-telemetry</span>
              )}
            </div>

            <span className="text-[#E4E6E9] dark:text-[#2C3038]">•</span>

            <button
              type="button"
              onClick={() => setShowToolsModal(!showToolsModal)}
              className="inline-flex items-center gap-1 text-xs text-[#355CFF] dark:text-[#91A7FF] hover:underline cursor-pointer"
              title={isZh ? '查看工具' : 'View tools'}
            >
              <Wrench className="w-2.5 h-2.5" />
              <span>{COPILOT_HARNESS_TOOLS.length} 工具就绪</span>
            </button>
          </div>
        </div>
      </div>

      <div className="flex items-center space-x-1 shrink-0">
        <button
          onClick={handleClearHistory}
          className="p-1.5 text-[#656B75] hover:text-rose-500 dark:hover:text-rose-400 rounded-lg hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition"
          title={isZh ? '清空会话' : 'Clear Chat'}
        >
          <Trash2 className="w-4 h-4" />
        </button>
        <button
          onClick={() => setIsExpanded(!isExpanded)}
          className="p-1.5 text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] rounded-lg hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition"
          title={isExpanded ? 'Restore' : 'Expand'}
        >
          {isExpanded ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
        </button>
        <button
          onClick={onClose}
          className="p-1.5 text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] rounded-lg hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition cursor-pointer"
          title="Close (ESC)"
        >
          <X className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
}
