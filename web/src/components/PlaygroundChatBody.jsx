import { MessageSquare, Sparkles, RefreshCw, Send, Clock, Zap } from 'lucide-react';
import ThinkingStatus from './ThinkingStatus';
import MarkdownRenderer from './MarkdownRenderer';

export default function PlaygroundChatBody({
  activeChatModel, arenaDurationB, arenaModelB, arenaOutputB, arenaReasoningB, arenaTTFTB, arenaThinkingB,
  arenaTokensPerSecB, chatInput, chatInputRef, handleOptimizeChatInput, handleRunChat, isZh, lang, messages,
  offPeak, playLoading, playMode, playModel, playOutput, playReasoningOutput, playTTFTMs, playThinking,
  playTokensPerSec, setChatInput, showSystemPrompt, systemPrompt
}) {
  return (
    <div className="flex-1 flex flex-col min-h-0">
      {playMode === 'single' ? (
        /* Single Model Chat View */
        <div className="flex-1 overflow-y-auto space-y-4 p-4 rounded-xl border border-slate-200/80 dark:border-slate-800/80 bg-slate-50/60 dark:bg-slate-900/40">
          {messages.length === 0 && !playLoading && !playOutput && !playReasoningOutput && (
            <div className="h-full min-h-[240px] flex flex-col items-center justify-center text-center py-16 text-slate-400 dark:text-slate-500">
              <MessageSquare className="w-10 h-10 mb-3 opacity-30 stroke-1" />
              <p className="text-xs font-medium">{isZh ? '输入提示词开始调试演练' : 'Enter a prompt to start testing'}</p>
              <p className="text-[11px] opacity-75 mt-1">{isZh ? '支持流式输出、思考内容与摘要显示与多模型竞技场对比' : 'Supports streaming responses, reasoning traces, and model arena'}</p>
            </div>
          )}

          {/* Messages Bubble Stream */}
          {messages.map((msg, idx) => (
            <div
              key={idx}
              className={`flex flex-col ${msg.role === 'user' ? 'items-end' : 'items-start'}`}
            >
              <div className="ui-meta font-mono mb-1">
                {msg.role === 'user' ? (isZh ? '调用方 (User)' : 'User') : `${playModel} (Assistant)`}
              </div>
              <div
                className={`max-w-[85%] rounded-2xl p-3.5 text-xs leading-relaxed ${
                  msg.role === 'user'
                    ? 'bg-indigo-600 text-white'
                    : 'bg-white dark:bg-slate-800 text-slate-800 dark:text-slate-100 border border-slate-200/80 dark:border-slate-700 shadow-sm'
                }`}
              >
                <ThinkingStatus observation={msg.thinking} reasoning={msg.reasoning} lang={lang} />
                {msg.reasoning && (
                  <div className="mb-2.5 p-2.5 bg-amber-50/70 dark:bg-amber-950/30 border border-amber-200/80 dark:border-amber-800/60 rounded-xl text-xs text-amber-950 dark:text-amber-200">
                    <div className="font-bold flex items-center space-x-1.5 text-amber-800 dark:text-amber-400 mb-1">
                      <Sparkles className="w-3 h-3" />
                      <span>{isZh ? '思考内容 / 摘要' : 'Thinking content / summary'}</span>
                    </div>
                    <div className="whitespace-pre-wrap text-[11px] font-mono leading-relaxed opacity-85">
                      {msg.reasoning}
                    </div>
                  </div>
                )}
                {msg.role === 'user' ? (
                  <div className="whitespace-pre-wrap">{msg.content}</div>
                ) : (
                  <MarkdownRenderer content={msg.content} lang={lang} />
                )}
              </div>
            </div>
          ))}

          {/* Live Streaming Assistant Message (Unified Bubble) */}
          {(playLoading || playOutput || playReasoningOutput) && (
            <div className="flex flex-col items-start w-full">
              <div className="ui-meta font-mono mb-1 flex items-center space-x-1.5">
                <span>{playModel} (Assistant)</span>
                {playLoading && <span className="inline-block w-1.5 h-1.5 rounded-full bg-indigo-500 animate-pulse" />}
              </div>
              <div className="max-w-[85%] w-full rounded-2xl p-3.5 text-xs bg-white dark:bg-slate-800 text-slate-800 dark:text-slate-100 border border-slate-200/80 dark:border-slate-700 shadow-sm leading-relaxed">
                <ThinkingStatus observation={playThinking} reasoning={playReasoningOutput} complete={!playLoading} lang={lang} />
                {playReasoningOutput && (
                  <div className="mb-2.5 p-2.5 bg-amber-50/70 dark:bg-amber-950/30 border border-amber-200/80 dark:border-amber-800/60 rounded-xl text-xs text-amber-950 dark:text-amber-200">
                    <div className="font-bold flex items-center space-x-1.5 text-amber-800 dark:text-amber-400 mb-1">
                      <Sparkles className="w-3 h-3 text-amber-500 animate-pulse" />
                      <span>{isZh ? '思考内容 / 摘要' : 'Thinking content / summary'}</span>
                    </div>
                    <div className="whitespace-pre-wrap text-[11px] font-mono leading-relaxed opacity-85">
                      {playReasoningOutput}
                    </div>
                  </div>
                )}
                {playOutput ? (
                  <MarkdownRenderer content={playOutput} lang={lang} />
                ) : (
                  playLoading && !playReasoningOutput && (
                    <div className="flex items-center space-x-2 ui-meta py-1 font-mono">
                      <span className="w-2 h-2 rounded-full bg-indigo-500 animate-ping" />
                      <span>{isZh ? '正在思考与组织响应...' : 'Thinking and generating response...'}</span>
                    </div>
                  )
                )}
              </div>
            </div>
          )}
        </div>
      ) : (
        /* Side-by-Side Arena Split View */
        <div className="flex-1 grid grid-cols-2 gap-3 min-h-0">
          {/* Left Contender: Model A */}
          <div className="flex flex-col rounded-xl border border-[#355CFF]/30 dark:border-[#355CFF]/40 bg-[#EEF2FF]/30 dark:bg-[#242D48]/20 p-3 overflow-y-auto space-y-3">
            <div className="flex items-center justify-between pb-2 border-b border-[#355CFF]/20 dark:border-[#355CFF]/30 text-xs">
              <span className="font-bold text-[#355CFF] dark:text-[#91A7FF] font-mono">
                Model A: {playModel}
              </span>
              <div className="ui-meta font-mono space-x-2">
                <span>TTFT: {playTTFTMs ? `${playTTFTMs}ms` : '-'}</span>
                <span>{isZh ? '速度' : 'Speed'}: {playTokensPerSec ? `${playTokensPerSec}tps` : '-'}</span>
              </div>
            </div>
            {playReasoningOutput && (
              <div className="p-2.5 rounded-xl bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-900 dark:text-amber-200 font-mono whitespace-pre-wrap">
                {playReasoningOutput}
              </div>
            )}
            <div className="text-xs text-[#202124] dark:text-[#F1F2F4] leading-relaxed">
              {playOutput ? (
                <MarkdownRenderer content={playOutput} lang={lang} />
              ) : (
                <div className="text-center py-24 ui-meta">
                  {isZh ? '等待发送评测指令...' : 'Awaiting prompt evaluation...'}
                </div>
              )}
            </div>
          </div>

          {/* Right Contender: Model B */}
          <div className="flex flex-col rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-[#F0F1F2]/60 dark:bg-[#22252B]/40 p-3 overflow-y-auto space-y-3">
            <div className="flex items-center justify-between pb-2 border-b border-[#E4E6E9] dark:border-[#2C3038] text-xs">
              <span className="font-bold text-[#202124] dark:text-[#F1F2F4] font-mono">
                Model B: {arenaModelB}
              </span>
              <div className="ui-meta font-mono space-x-2">
                <span>TTFT: {arenaTTFTB ? `${arenaTTFTB}ms` : '-'}</span>
                <span>{isZh ? '速度' : 'Speed'}: {arenaTokensPerSecB ? `${arenaTokensPerSecB}tps` : '-'}</span>
                {/* The single-model view reports the total duration; the
                    arena collected it for B and never showed it. */}
                <span>{isZh ? '总耗时' : 'Duration'}: {arenaDurationB ? `${arenaDurationB}ms` : '-'}</span>
              </div>
            </div>
            <ThinkingStatus observation={arenaThinkingB} reasoning={arenaReasoningB} complete={!playLoading} lang={lang} />
            {arenaReasoningB && (
              <div className="p-2.5 rounded-xl bg-amber-50 dark:bg-amber-950/40 border border-amber-200 text-[11px] text-amber-900 dark:text-amber-200 font-mono whitespace-pre-wrap">
                {arenaReasoningB}
              </div>
            )}
            <div className="text-xs text-slate-800 dark:text-slate-100 leading-relaxed">
              {arenaOutputB ? (
                <MarkdownRenderer content={arenaOutputB} lang={lang} />
              ) : (
                <div className="text-center py-24 ui-meta">
                  {isZh ? '等待发送评测指令...' : 'Awaiting prompt evaluation...'}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Live FinOps Telemetry Bar */}
      <div className="ui-meta flex flex-wrap items-center justify-between gap-x-2 gap-y-1 px-1 pt-2">
        <div className="flex items-center space-x-2">
          <span className="font-mono">
            {isZh ? '预估输入 Tokens: ~' : 'Est. Prompt Tokens: ~'}
            <span className="font-bold text-slate-700 dark:text-slate-300">
              {Math.max(0, Math.round((chatInput.length + (showSystemPrompt ? systemPrompt.length : 0)) / 3.2))}
            </span>
          </span>
          <span>·</span>
          <span className="text-emerald-600 dark:text-emerald-400 font-medium flex items-center space-x-1">
            <Zap className="w-3 h-3" />
            <span>{isZh ? 'Prompt 缓存识别就绪' : 'Prompt Caching Ready'}</span>
          </span>
        </div>
        {offPeak ? (
          <div
            className="flex items-center space-x-1 text-indigo-600 dark:text-indigo-400 font-medium"
            title={isZh
              ? `模型 [${activeChatModel}] 配置了闲时折扣；是否生效取决于当前时刻与优惠时段设置。`
              : `Model [${activeChatModel}] has an off-peak discount; whether it applies now depends on the configured time windows.`}
          >
            <Clock className="w-3 h-3" />
            <span>
              {isZh
                ? `闲时折扣已配置 (${offPeak.label} 折, 按时段生效)`
                : `Off-peak discount configured (${offPeak.label}/10, time-windowed)`}
            </span>
          </div>
        ) : (
          <div className="flex items-center space-x-1 text-slate-400 dark:text-slate-500 font-medium">
            <Clock className="w-3 h-3" />
            <span>{isZh ? '该模型未配置闲时折扣' : 'No off-peak discount configured'}</span>
          </div>
        )}
      </div>

      {/* Chat Input Bar: auto-growing composer (Enter sends, Shift+Enter newline) */}
      <div className="mt-2 flex items-end space-x-2">
        <textarea
          ref={chatInputRef}
          rows={1}
          value={chatInput}
          onChange={(e) => setChatInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== 'Enter' || e.shiftKey) return;
            // An IME candidate confirmation also reports Enter: never send
            // while composing (Chinese / Japanese input).
            if (e.nativeEvent.isComposing || e.keyCode === 229) return;
            e.preventDefault();
            if (!playLoading) handleRunChat();
          }}
          placeholder={playMode === 'arena'
            ? (isZh ? '输入消息，对比两个模型' : 'Prompt to benchmark both models at once (Enter to send / Shift+Enter for newline)...')
            : (isZh ? '输入消息，Enter 发送' : 'Enter a message')}
          className="ui-field flex-1 min-w-0 resize-none overflow-y-auto"
        />
        <button
          type="button"
          onClick={handleOptimizeChatInput}
          disabled={!chatInput.trim()}
          className="ui-btn px-2.5 py-2.5 shrink-0 border border-purple-200 dark:border-purple-800/80 bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 hover:bg-purple-100 dark:hover:bg-purple-900/60"
          title={isZh ? '使用 AI 将输入 Prompt 升格为专业技术规格' : 'AI Prompt Engineering Enhancement'}
        >
          <Sparkles className="w-4 h-4 text-amber-500" />
        </button>
        <button
          type="button"
          onClick={handleRunChat}
          disabled={playLoading}
          className="ui-btn-primary px-5 shrink-0"
        >
          {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
          <span>{playLoading ? (isZh ? '流式推导中...' : 'Generating...') : (playMode === 'arena' ? (isZh ? '并发对决测试' : 'Run Arena Test') : (isZh ? '发送请求' : 'Send'))}</span>
        </button>
      </div>
    </div>
  );
}
