import { Sparkles, Bot, User, Copy, Check, ArrowRight, RefreshCw, Terminal, ChevronDown, ChevronRight, AlertTriangle, CheckCircle2, Clock } from 'lucide-react';
import ThinkingStatus from './ThinkingStatus';
import MarkdownRenderer from './MarkdownRenderer';

export default function CopilotMessages({
  copiedId, expandedReasoningIds, expandedStepObsIds, expandedTraceIds, handleCopy, isExpanded, isLoading,
  isZh, lang, messages, messagesEndRef, onClose, setCurrentTab, setViewMode, toggleReasoning, toggleStepObs,
  toggleTrace
}) {
  return (
    <div className="flex-1 overflow-y-auto p-4 space-y-4 text-xs bg-[#F7F7F5] dark:bg-[#111214]">
      {messages.map((m) => {
        const isUser = m.role === 'user';
        const isReasoningOpen = !!expandedReasoningIds[m.id];
        const isTraceOpen = !!expandedTraceIds[m.id];
        const traceSteps = m.harnessTrace || [];

        return (
          <div
            key={m.id}
            className={`flex items-start space-x-2.5 ${isUser ? 'flex-row-reverse space-x-reverse' : ''}`}
          >
            <div
              className={`w-7 h-7 rounded-xl flex items-center justify-center shrink-0 shadow-2xs ${
                isUser
                  ? 'bg-[#202124] dark:bg-[#2C3038] text-white'
                  : 'bg-[#355CFF] text-white'
              }`}
            >
              {isUser ? <User className="w-3.5 h-3.5" /> : <Bot className="w-3.5 h-3.5" />}
            </div>

            <div
              className={`max-w-[88%] rounded-2xl p-4 leading-relaxed relative group ${
                isUser
                  ? 'bg-[#355CFF] text-white rounded-tr-xs shadow-xs'
                  : 'bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] text-[#202124] dark:text-[#F1F2F4] rounded-tl-xs shadow-xs'
              }`}
            >
              {!isUser && (
                <button
                  onClick={() => handleCopy(m.content, m.id)}
                  className="absolute top-2.5 right-2.5 opacity-0 group-hover:opacity-100 p-1.5 rounded-lg bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] shadow-2xs transition border border-[#E4E6E9] dark:border-[#2C3038] cursor-pointer"
                  title={isZh ? '复制内容' : 'Copy'}
                >
                  {copiedId === m.id ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                </button>
              )}

              {/* 调用失败原因：错误必须可见，不能被静默吞掉 */}
              {!isUser && m.isError && m.errorMessage && (
                <div
                  role="alert"
                  className="mb-3 rounded-xl border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-xs leading-relaxed text-rose-900 dark:text-rose-200"
                >
                  ⚠️ {isZh ? '模型调用失败' : 'Model call failed'}：{m.errorMessage}
                </div>
              )}

              {/* 离线降级提示：本地模板不是实时遥测 */}
              {!isUser && m.isFallback && (
                <div className="mb-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs leading-relaxed text-amber-900 dark:text-amber-200">
                  {isZh
                    ? '⚠️ 离线本地模板（非实时）：模型不可用，以下内容由静态诊断生成，未读取实时集群指标，请勿据此直接做运维决策。'
                    : '⚠️ Offline template (not live): the model was unavailable, so this reply comes from a static diagnostic and does not reflect live cluster metrics.'}
                </div>
              )}

              {/* 深度思考过程 (Thinking Process) 折叠卡片 */}
              {!isUser && m.reasoning && (
                <div className="mb-3 rounded-xl border border-amber-500/30 bg-amber-500/10 overflow-hidden">
                  <button
                    type="button"
                    onClick={() => toggleReasoning(m.id)}
                    className="w-full flex items-center justify-between px-3 py-1.5 text-xs font-semibold text-amber-900 dark:text-amber-200 hover:bg-amber-500/20 transition cursor-pointer"
                  >
                    <div className="flex items-center space-x-1.5">
                      <Sparkles className="w-3 h-3 text-amber-500 animate-pulse" />
                      <span>{isZh ? '深度思考链 (Thinking Process)' : 'Thinking Process'}</span>
                    </div>
                    {isReasoningOpen ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                  </button>
                  {isReasoningOpen && (
                    <div className="px-3 py-2 border-t border-amber-500/20 text-xs font-mono leading-relaxed text-amber-900/90 dark:text-amber-200/90 whitespace-pre-wrap max-h-48 overflow-y-auto">
                      {m.reasoning}
                    </div>
                  )}
                </div>
              )}

              {/* 标准 Agent Harness 执行轨迹 (Tool Execution Steps) */}
              {!isUser && traceSteps.length > 0 && (
                <div className="mb-3 rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-[#F7F7F5] dark:bg-[#22252B] overflow-hidden">
                  <button
                    type="button"
                    onClick={() => toggleTrace(m.id)}
                    className="w-full flex items-center justify-between px-3 py-1.5 text-xs font-semibold text-[#202124] dark:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#2c3038] transition cursor-pointer"
                  >
                    <div className="flex items-center space-x-1.5">
                      <Terminal className="w-3 h-3 text-[#355CFF] dark:text-[#91A7FF]" />
                      <span>{isZh ? `工具调用 (${traceSteps.length})` : `Tool calls (${traceSteps.length})`}</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <span className="text-xs font-mono text-[#355CFF] dark:text-[#91A7FF]">
                        {traceSteps.reduce((acc, s) => acc + (s.duration_ms || 0), 0)}ms
                      </span>
                      {isTraceOpen ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                    </div>
                  </button>

                  {isTraceOpen && (
                    <div className="px-3 py-2 border-t border-[#E4E6E9] dark:border-[#2C3038] space-y-2 max-h-60 overflow-y-auto">
                      {traceSteps.map((step, sIdx) => {
                        const stepKey = `${m.id}_step_${sIdx}`;
                        const isObsOpen = !!expandedStepObsIds[stepKey];
                        const isSuccess = step.status === 'success';

                        return (
                          <div key={sIdx} className="rounded-lg bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] p-2 text-xs">
                            <div className="flex items-center justify-between">
                              <div className="flex items-center space-x-1.5 font-mono font-medium">
                                {isSuccess ? (
                                  <CheckCircle2 className="w-3 h-3 text-emerald-500" />
                                ) : (
                                  <AlertTriangle className="w-3 h-3 text-rose-500" />
                                )}
                                <span className="text-[#355CFF] dark:text-[#91A7FF] font-semibold">{step.tool}</span>
                              </div>
                              <div className="flex items-center gap-1 font-mono text-[#656B75] dark:text-[#A1A7B0] text-xs">
                                <Clock className="w-2.5 h-2.5" />
                                <span>{step.duration_ms || 0}ms</span>
                              </div>
                            </div>

                            {/* 参数概览 */}
                            {step.args && Object.keys(step.args).length > 0 && (
                              <div className="mt-1 text-[#656B75] dark:text-[#A1A7B0] font-mono text-xs truncate">
                                args: {JSON.stringify(step.args)}
                              </div>
                            )}

                            {/* 观察结果 (Observation) 折叠 */}
                            <div className="mt-1.5 pt-1 border-t border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between">
                              <button
                                type="button"
                                onClick={() => toggleStepObs(stepKey)}
                                className="text-xs text-[#355CFF] dark:text-[#91A7FF] hover:underline flex items-center gap-0.5 cursor-pointer"
                              >
                                <span>{isObsOpen ? (isZh ? '收起观测数据' : 'Hide Observation') : (isZh ? '查看观测数据 (Observation)' : 'View Observation')}</span>
                                {isObsOpen ? <ChevronDown className="w-2.5 h-2.5" /> : <ChevronRight className="w-2.5 h-2.5" />}
                              </button>
                            </div>

                            {isObsOpen && (
                              <pre className="mt-1 p-2 rounded bg-[#111214] text-[#F1F2F4] border border-[#2C3038] font-mono text-xs overflow-x-auto max-h-36">
                                {JSON.stringify(step.result || step.error, null, 2)}
                              </pre>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              )}

              {!isUser && <ThinkingStatus observation={m.thinking} reasoning={m.reasoning} complete={!isLoading} lang={lang} />}
              {/* 消息正文 */}
              {isUser ? (
                <div className="whitespace-pre-wrap font-sans text-xs leading-relaxed">{m.content}</div>
              ) : (
                <MarkdownRenderer content={m.content} lang={lang} />
              )}

              {/* 交互式操作引导按钮 */}
              {m.actions && m.actions.length > 0 && (
                <div className="mt-3.5 pt-2.5 border-t border-[#E4E6E9] dark:border-[#2C3038] flex flex-wrap gap-2">
                  {m.actions.map((act, i) => (
                    <button
                      key={i}
                      onClick={() => {
                        if (act.tab) {
                          setCurrentTab(act.tab);
                          if (setViewMode) setViewMode('console');
                          if (!isExpanded) onClose();
                        }
                      }}
                      className="px-3 py-1.5 rounded-xl bg-[#EEF2FF] hover:bg-[#E0E7FF] dark:bg-[#242D48] dark:hover:bg-[#2e3a5e] text-[#355CFF] dark:text-[#91A7FF] font-semibold text-xs flex items-center space-x-1.5 transition border border-[#355CFF]/20 dark:border-[#91A7FF]/30 cursor-pointer shadow-2xs"
                    >
                      <span>{act.label}</span>
                      <ArrowRight className="w-3 h-3" />
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        );
      })}

      {isLoading && (
        <div className="flex items-center space-x-2 text-xs text-[#656B75] dark:text-[#A1A7B0] py-2 px-1">
          <RefreshCw className="w-3.5 h-3.5 animate-spin text-[#355CFF] dark:text-[#91A7FF]" />
          <span>{isZh ? '处理中…' : 'Working…'}</span>
        </div>
      )}
      <div ref={messagesEndRef} />
    </div>
  );
}
