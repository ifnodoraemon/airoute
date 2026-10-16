import React, { useState } from 'react';
import { X, Copy, Check, Activity, Key, AlertCircle, Trash2, Sparkles, ArrowRight } from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';

export default function LogDetailModal({ log, onClose, onCopy, onFilterBySession, onDeleteLog, onNavigate, lang = 'zh', t }) {
  useModalA11y(!!log, onClose);

  if (!log) return null;

  const isZh = lang === 'zh';
  const [copied, setCopied] = useState(false);

  const handleCopyRaw = () => {
    onCopy(JSON.stringify(log, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const isSuccess = log.status_code >= 200 && log.status_code < 300;

  const rca = (() => {
    const code = log.status_code || 200;
    const isErr = code >= 400 || !!log.error_message;
    const errStr = (log.error_message || '').toLowerCase();

    if (!isErr) {
      const isFast = log.ttft_ms && log.ttft_ms < 400;
      const isCached = log.cached_tokens > 0;
      return {
        type: 'success',
        badge: isZh ? '链路能效: 极佳' : 'Optimal Performance',
        title: isZh ? 'AI 链路与能效分析' : 'AI Latency & FinOps Audit',
        summary: isZh
          ? `该请求执行顺畅，${isFast ? `首字延迟 (TTFT) 仅 ${log.ttft_ms}ms (超越 P90 表现)` : `总耗时 ${log.duration_ms}ms`}。${isCached ? `成功命中 Prompt Cache，节约 ${log.cached_tokens} Tokens 传输开销。` : '若此类请求具备固定系统前缀，建议启用 Prompt Caching 进一步降低输入成本与推理延迟。'}`
          : `Execution optimal. ${isFast ? `TTFT is ${log.ttft_ms}ms (better than P90)` : `Duration: ${log.duration_ms}ms`}. ${isCached ? `Prompt cache hit saved ${log.cached_tokens} tokens.` : 'Consider prompt caching for static system prompts to reduce cost and latency.'}`,
        actionLabel: isZh ? '在 Playground 演练' : 'Test in Playground',
        actionTab: 'playground'
      };
    }

    if (code === 429 || errStr.includes('rate limit') || errStr.includes('tpm') || errStr.includes('rpm') || errStr.includes('quota')) {
      return {
        type: 'warning',
        badge: '429 Rate Limit',
        title: isZh ? '根因剖析: 上游服务商速率限制 (TPM/RPM Throttling)' : 'RCA: Upstream Rate Limit (TPM/RPM)',
        summary: isZh
          ? `上游渠道 [${log.channel || '默认'}] 的请求频率或单分钟 Token 消耗达到限额。建议在【模型路由】中为模型 [${log.model}] 挂载第二家服务商作为降级容灾渠道，或在【通道管理】中增加备用 Key 轮询。`
          : `Upstream channel [${log.channel || 'Default'}] hit rate limits. Recommended action: configure a secondary fallback provider for [${log.model}] in Model Routes, or add backup keys in Channels.`,
        actionLabel: isZh ? '前往【模型路由】配置备用降级' : 'Configure Failover Route',
        actionTab: 'models'
      };
    }

    if (code === 400 || errStr.includes('context') || errStr.includes('maximum context length') || errStr.includes('tokens')) {
      return {
        type: 'error',
        badge: 'Context / Parameter',
        title: isZh ? '根因剖析: 上下文超限或模型参数不兼容' : 'RCA: Context Overflow or Parameter Mismatch',
        summary: isZh
          ? `传入 Prompt 超过模型 [${log.model}] 的最大 Token 窗口，或携带了模型不支持的参数。建议开启 Prompt 压缩截断，或将路由升级为 128k/1M 上下文长文本模型。`
          : `Prompt tokens exceed the maximum context window of [${log.model}], or unsupported parameters were sent. Truncate conversation history or route to a long-context model.`,
        actionLabel: isZh ? '前往【模型路由】核查映射' : 'Review Model Routes',
        actionTab: 'models'
      };
    }

    if (code === 401 || code === 403 || errStr.includes('auth') || errStr.includes('key') || errStr.includes('permission')) {
      return {
        type: 'error',
        badge: 'Auth Failure',
        title: isZh ? '根因剖析: 上游凭证失效或权限拒绝' : 'RCA: Upstream Credential Revoked / Unauthorized',
        summary: isZh
          ? `上游通道 [${log.channel || '渠道'}] 绑定的 API 密钥已失效、服务商余额欠费或账号被封禁。请前往【通道管理】更新密钥并执行连通性探测。`
          : `The API key for upstream channel [${log.channel || 'Channel'}] is invalid, expired, or out of balance. Update upstream API key in Channels and re-probe.`,
        actionLabel: isZh ? '前往【通道管理】更新密钥' : 'Update Channel Key',
        actionTab: 'channels'
      };
    }

    if (code === 502 || code === 504 || code === 503 || errStr.includes('timeout') || errStr.includes('deadline')) {
      return {
        type: 'error',
        badge: 'Upstream Timeout',
        title: isZh ? '根因剖析: 上游网关握手超时' : 'RCA: Gateway Timeout or Network Unreachable',
        summary: isZh
          ? `网关向上游发起推理请求时超时。若该模型为复杂思维链推理（如 DeepSeek-R1 / o1），建议在【通道管理】中延长超时时间至 90s~120s，并开启断路器保护。`
          : `Upstream request timed out. For deep reasoning models, increase channel timeout to 90s-120s in Channel settings and keep circuit breaker enabled.`,
        actionLabel: isZh ? '前往【通道管理】调整超时' : 'Adjust Timeout in Channels',
        actionTab: 'channels'
      };
    }

    return {
      type: 'error',
      badge: `HTTP ${code}`,
      title: isZh ? '根因剖析: 上游内部异常' : 'RCA: Upstream Internal Failure',
      summary: isZh
        ? `上游服务商返回非预期状态码。已触发链路追踪 Trace ID，建议检查上游日志或在模型路由中将权重调度转移至其他稳定渠道。`
        : `Unexpected response from upstream provider. Shift traffic weight to other active channels in Model Routes.`,
      actionLabel: isZh ? '检查通道服务状态' : 'Inspect Channels',
      actionTab: 'channels'
    };
  })();

  return (
    <div
      className="fixed inset-0 z-50 overflow-hidden bg-slate-900/40 backdrop-blur-sm animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="fixed inset-y-0 right-0 w-full max-w-xl bg-white dark:bg-[#0F1420] border-l border-slate-200/70 dark:border-slate-800/80 shadow-2xl flex flex-col h-full animate-in slide-in-from-right duration-200">
        {/* Header */}
        <div className="p-6 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className={`w-9 h-9 rounded-lg flex items-center justify-center border ${
              isSuccess
                ? 'bg-emerald-50 dark:bg-emerald-950/40 border-emerald-200/80 dark:border-emerald-800/60 text-emerald-600 dark:text-emerald-400'
                : 'bg-rose-50 dark:bg-rose-950/40 border-rose-200/80 dark:border-rose-800/60 text-rose-600 dark:text-rose-400'
            }`}>
              <Activity className="w-4 h-4" />
            </div>
            <div>
              <h3 className="font-semibold text-base text-slate-900 dark:text-white tracking-tight flex items-center space-x-2">
                <span>{isZh ? '请求审计详情' : 'Audit Log Inspector'}</span>
                <span className={`text-xs px-2 py-0.5 rounded-full font-mono font-medium ${
                  isSuccess
                    ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                    : 'bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800'
                }`}>
                  HTTP {log.status_code || 200}
                </span>
              </h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 font-mono tabular-nums">
                {isZh ? '记录 #' : 'Log #'}{log.id} · {log.created_at ? new Date(log.created_at).toLocaleString() : (isZh ? '刚刚' : 'Just now')}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body grid */}
        <div className="p-6 space-y-5 overflow-y-auto flex-1 text-sm">
          {/* Top key metric cards */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div className="p-3.5 rounded-xl bg-slate-50/70 dark:bg-slate-900/50 border border-slate-200/70 dark:border-slate-800/70">
              <span className="text-[11px] text-slate-500 dark:text-slate-400 block font-medium">{isZh ? '请求模型' : 'Model'}</span>
              <span className="text-xs font-mono font-semibold text-slate-900 dark:text-white truncate block mt-1">
                {log.model || '-'}
              </span>
            </div>
            <div className="p-3.5 rounded-xl bg-slate-50/70 dark:bg-slate-900/50 border border-slate-200/70 dark:border-slate-800/70">
              <span className="text-[11px] text-slate-500 dark:text-slate-400 block font-medium">{isZh ? '路由渠道' : 'Channel'}</span>
              <span className="text-xs font-medium text-slate-900 dark:text-white truncate block mt-1 font-mono">
                {log.channel || (isZh ? '直通/多源' : 'Direct / Multi')}
              </span>
            </div>
            <div className="p-3.5 rounded-xl bg-slate-50/70 dark:bg-slate-900/50 border border-slate-200/70 dark:border-slate-800/70">
              <span className="text-[11px] text-slate-500 dark:text-slate-400 block font-medium">{isZh ? '首字延迟 (TTFT)' : 'TTFT Latency'}</span>
              <span className="text-xs font-mono tabular-nums font-semibold text-amber-600 dark:text-amber-400 block mt-1">
                {log.ttft_ms ? `${log.ttft_ms} ms` : '-'}
              </span>
            </div>
            <div className="p-3.5 rounded-xl bg-slate-50/70 dark:bg-slate-900/50 border border-slate-200/70 dark:border-slate-800/70">
              <span className="text-[11px] text-slate-500 dark:text-slate-400 block font-medium">{isZh ? '总执行耗时' : 'Total Duration'}</span>
              <span className="text-xs font-mono tabular-nums font-semibold text-slate-900 dark:text-white block mt-1">
                {log.duration_ms} ms
              </span>
            </div>
          </div>

          {/* Details list */}
          <div className="rounded-xl border border-slate-200/70 dark:border-slate-800/80 divide-y divide-slate-100 dark:divide-slate-800/70 overflow-hidden text-xs">
            <div className="p-3.5 flex items-center justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '链路追踪 ID (Trace ID)' : 'Trace ID'}</span>
              <div className="flex items-center space-x-2">
                <span className="font-mono text-xs font-semibold text-slate-900 dark:text-slate-100">
                  {log.trace_id || (isZh ? '未生成' : 'N/A')}
                </span>
                {log.trace_id && (
                  <>
                    <button
                      onClick={() => {
                        onCopy(log.trace_id);
                        setCopied(true);
                        setTimeout(() => setCopied(false), 2000);
                      }}
                      className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition cursor-pointer"
                      title={isZh ? '复制 Trace ID' : 'Copy Trace ID'}
                    >
                      <Copy className="w-3 h-3" />
                    </button>
                    {onFilterBySession && (
                      <button
                        onClick={() => {
                          onFilterBySession(log.trace_id);
                          onClose();
                        }}
                        className="px-2 py-0.5 rounded-md bg-indigo-50 dark:bg-indigo-950/50 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 text-[11px] font-medium transition cursor-pointer"
                      >
                        {isZh ? '追踪全链路' : 'Trace Flow'}
                      </button>
                    )}
                  </>
                )}
              </div>
            </div>
            {log.chat_id && (
              <div className="p-3.5 flex items-center justify-between">
                <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '对话 ID (Chat ID)' : 'Chat ID'}</span>
                <div className="flex items-center space-x-2">
                  <span className="font-mono text-xs font-medium text-slate-800 dark:text-slate-200">
                    {log.chat_id}
                  </span>
                  <button
                    onClick={() => {
                      onCopy(log.chat_id);
                      setCopied(true);
                      setTimeout(() => setCopied(false), 2000);
                    }}
                    className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition cursor-pointer"
                    title={isZh ? '复制对话 ID' : 'Copy Chat ID'}
                  >
                    <Copy className="w-3 h-3" />
                  </button>
                </div>
              </div>
            )}
            {log.session_id && (
              <div className="p-3.5 flex items-center justify-between">
                <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '会话 ID (Session ID)' : 'Session ID'}</span>
                <div className="flex items-center space-x-2">
                  <span className="font-mono text-xs font-medium text-slate-800 dark:text-slate-200">
                    {log.session_id}
                  </span>
                  <button
                    onClick={() => {
                      onCopy(log.session_id);
                      setCopied(true);
                      setTimeout(() => setCopied(false), 2000);
                    }}
                    className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition cursor-pointer"
                    title={isZh ? '复制会话 ID' : 'Copy Session ID'}
                  >
                    <Copy className="w-3 h-3" />
                  </button>
                  {onFilterBySession && (
                    <button
                      onClick={() => {
                        onFilterBySession(log.session_id);
                        onClose();
                      }}
                      className="px-2 py-0.5 rounded-md bg-indigo-50 dark:bg-indigo-950/50 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 text-[11px] font-medium transition cursor-pointer"
                    >
                      {isZh ? '筛选此会话' : 'Filter Session'}
                    </button>
                  )}
                </div>
              </div>
            )}
            <div className="p-3.5 flex justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '应用 / 团队' : 'App / Tenant'}</span>
              <span className="font-medium text-slate-800 dark:text-slate-200">{log.tenant_id || 'anonymous'}</span>
            </div>
            <div className="p-3.5 flex justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? 'API 密钥' : 'API Key'}</span>
              <span className="font-mono text-slate-700 dark:text-slate-300">{log.api_key || (isZh ? '无' : 'None')}</span>
            </div>
            <div className="p-3.5 flex justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? 'Token 明细' : 'Token Breakdown'}</span>
              <span className="text-slate-800 dark:text-slate-200 font-mono tabular-nums text-xs">
                {isZh ? '未缓存输入: ' : 'Uncached: '}<strong className="text-slate-900 dark:text-white font-semibold">{Math.max(0, (log.prompt_tokens || 0) - (log.cached_tokens || 0)).toLocaleString()}</strong> ·
                {isZh ? ' 输出: ' : ' Output: '}<strong className="text-slate-900 dark:text-white font-semibold">{(log.completion_tokens || 0).toLocaleString()}</strong> ·
                {isZh ? ' 缓存: ' : ' Cached: '}<strong className="text-amber-600 dark:text-amber-400 font-semibold">{(log.cached_tokens || 0).toLocaleString()}</strong>
              </span>
            </div>
            <div className="p-3.5 flex justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '计费扣减' : 'Billed Cost'}</span>
              <div className="flex flex-wrap items-center gap-2 font-mono tabular-nums">
                <span className="font-semibold text-slate-900 dark:text-white">
                  ¥{(log.cost || 0).toFixed(4)}
                </span>
                {log.is_off_peak && (
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300">
                    {isZh ? `闲时 (${Math.round((log.off_peak_discount || 0.5) * 10)}折)` : `Off-peak (${Math.round((1 - (log.off_peak_discount || 0.5)) * 100)}% off)`}
                  </span>
                )}
                {log.cached_tokens > 0 ? (
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800">
                    {isZh ? `缓存: ${log.cached_tokens} Tokens` : `Cache: ${log.cached_tokens} Tokens`}
                  </span>
                ) : (
                  <span className="text-slate-400 text-[11px] font-sans">{isZh ? '未命中缓存' : 'Cache miss'}</span>
                )}
              </div>
            </div>
            <div className="p-3.5 flex justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '计费核算公式' : 'Billing Formula'}</span>
              <span className="font-mono text-xs text-slate-700 dark:text-slate-300">
                {log.cost > 0 ? (
                  <>
                    <span>({isZh ? '输入' : 'Prompt'} {log.prompt_tokens} + {isZh ? '输出' : 'Completion'} {log.completion_tokens}) × {isZh ? '单价' : 'Price'}</span>
                    {log.is_off_peak && <span className="text-indigo-600 dark:text-indigo-400 font-medium"> × {isZh ? `闲时 ${Math.round((log.off_peak_discount || 0.5) * 10)}折` : 'Off-peak'}</span>}
                    <span> = ¥{(log.cost || 0).toFixed(4)}</span>
                  </>
                ) : (
                  <span className="text-slate-500 font-sans">{isZh ? '零费率模型或管理员豁免计费' : 'Zero-cost tier or admin billing exemption'}</span>
                )}
              </span>
            </div>
            <div className="p-3.5 flex justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '客户端 IP' : 'Client IP'}</span>
              <span className="font-mono text-slate-600 dark:text-slate-300">{log.ip || '-'}</span>
            </div>
          </div>

          {/* Error notice if failed */}
          {log.error_message && (
            <div className="p-4 rounded-xl bg-rose-50/70 dark:bg-rose-950/30 border border-rose-200/80 dark:border-rose-900/50 text-rose-800 dark:text-rose-300 text-xs">
              <div className="font-semibold mb-1 flex items-center space-x-1.5">
                <AlertCircle className="w-3.5 h-3.5" />
                <span>{isZh ? '上游异常原始回包' : 'Raw Upstream Error Message'}</span>
              </div>
              <div className="font-mono mt-1 text-[11px]">{log.error_message}</div>
            </div>
          )}

          {/* AI Root Cause Analysis & Auto-Remediation (AI RCA) */}
          {rca && (
            <div className={`p-4 rounded-xl border text-xs ${
              rca.type === 'success'
                ? 'bg-slate-50 dark:bg-slate-900/40 border-slate-200/80 dark:border-slate-800 text-slate-800 dark:text-slate-200'
                : rca.type === 'warning'
                ? 'bg-amber-50/60 dark:bg-amber-950/20 border-amber-200/80 dark:border-amber-900/40 text-amber-900 dark:text-amber-200'
                : 'bg-rose-50/60 dark:bg-rose-950/20 border-rose-200/80 dark:border-rose-900/40 text-rose-900 dark:text-rose-200'
            }`}>
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center space-x-1.5 font-semibold">
                  <Sparkles className="w-4 h-4 text-slate-500 dark:text-slate-400" />
                  <span>{rca.title}</span>
                </div>
                <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${
                  rca.type === 'success' ? 'bg-slate-200/70 dark:bg-slate-800 text-slate-700 dark:text-slate-300' : rca.type === 'warning' ? 'bg-amber-100 dark:bg-amber-900/60 text-amber-800 dark:text-amber-300' : 'bg-rose-100 dark:bg-rose-900/60 text-rose-800 dark:text-rose-300'
                }`}>
                  {rca.badge}
                </span>
              </div>
              <p className="leading-relaxed opacity-90">{rca.summary}</p>
              {rca.actionLabel && onNavigate && (
                <div className="mt-3 pt-2.5 border-t border-slate-200/60 dark:border-slate-800/60 flex justify-end">
                  <button
                    onClick={() => {
                      onClose();
                      onNavigate(rca.actionTab);
                    }}
                    className="px-3 py-1.5 rounded-lg bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white font-medium flex items-center space-x-1.5 transition cursor-pointer text-xs shadow-sm"
                  >
                    <span>{rca.actionLabel}</span>
                    <ArrowRight className="w-3.5 h-3.5" />
                  </button>
                </div>
              )}
            </div>
          )}

          {/* Raw JSON toggle */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{isZh ? '原始日志 JSON' : 'Raw Log JSON'}</span>
              <button
                onClick={handleCopyRaw}
                className="text-xs text-indigo-600 dark:text-indigo-400 hover:underline flex items-center space-x-1 font-medium cursor-pointer"
              >
                {copied ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                <span>{copied ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制 JSON' : 'Copy JSON')}</span>
              </button>
            </div>
            <pre className="p-3.5 rounded-xl bg-slate-950 text-slate-300 font-mono text-[11px] overflow-x-auto max-h-40 leading-normal border border-slate-800">
              {JSON.stringify(log, null, 2)}
            </pre>
          </div>
        </div>

        {/* Footer */}
        <div className="p-4 bg-slate-50/70 dark:bg-slate-900/50 border-t border-slate-100 dark:border-slate-800/80 flex items-center justify-between mt-auto">
          <div>
            {onDeleteLog && (
              <button
                onClick={() => {
                  if (window.confirm(isZh ? '确认删除该条审计调用日志？' : 'Are you sure you want to delete this audit log?')) {
                    onDeleteLog(log.id);
                    onClose();
                  }
                }}
                className="px-3 py-1.5 rounded-lg text-xs font-medium text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-950/50 transition flex items-center space-x-1.5 cursor-pointer border border-rose-200 dark:border-rose-900/60 shadow-sm"
              >
                <Trash2 className="w-3.5 h-3.5" />
                <span>{isZh ? '删除此日志' : 'Delete Log'}</span>
              </button>
            )}
          </div>
          <button
            onClick={onClose}
            className="px-4 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-lg text-xs font-medium transition cursor-pointer"
          >
            {isZh ? '关闭' : 'Close'}
          </button>
        </div>
      </div>
    </div>
  );
}
