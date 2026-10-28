import { useState, useMemo } from 'react';
import {
  X,
  Copy,
  Check,
  Activity,
  AlertCircle,
  Trash2,
  Sparkles,
  ArrowRight,
  Terminal,
  Play,
  Clock
} from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';

export default function LogDetailModal({ log, onClose, onCopy, onFilterBySession, onDeleteLog, onNavigate, onOpenTrace, lang = 'zh' }) {
  useModalA11y(!!log, onClose);

  const isZh = lang === 'zh';
  const [copied, setCopied] = useState(false);
  const [copiedCurl, setCopiedCurl] = useState(false);
  const matchedPrice = log?.pricing_snapshot;

  const handleCopyRaw = () => {
    if (!log) return;
    onCopy(JSON.stringify(log, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const waterfall = useMemo(() => {
    if (!log) return null;
    const dur = log.duration_ms || 0;
    const ttft = log.ttft_ms || 0;
    const gen = dur > ttft && ttft > 0 ? dur - ttft : (dur > 0 ? dur : 0);
    const ttftPct = dur > 0 ? Math.min(100, Math.round((ttft / dur) * 100)) : 0;
    const genPct = dur > 0 ? Math.max(0, 100 - ttftPct) : 0;

    let tps = null;
    if (log.completion_tokens && log.completion_tokens > 0 && gen > 0) {
      tps = ((log.completion_tokens * 1000) / gen).toFixed(1);
    }
    return { dur, ttft, gen, ttftPct, genPct, tps };
  }, [log]);

  const handleCopyCurl = () => {
    if (!log) return;
    const origin = window.location.origin;
    const model = log.model || 'gpt-4o';
    const key = 'YOUR_API_KEY';
    const curl = `curl -X POST "${origin}/v1/chat/completions" \\
  -H "Authorization: Bearer ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "messages": [
      {"role": "user", "content": "Ping test diagnostic"}
    ],
    "stream": true
  }'`;
    onCopy(curl);
    setCopiedCurl(true);
    setTimeout(() => setCopiedCurl(false), 2000);
  };

  const isSuccess = log ? log.status_code >= 200 && log.status_code < 300 : false;
  // Evidence flags used by the remediation panel below. They must live at
  // component scope: the classification block is an IIFE, so anything declared
  // inside it is invisible to the JSX.
  const errorMessage = (log && log.error_message ? String(log.error_message) : '').trim();
  const hasUpstreamDetail = errorMessage.length > 0;
  // A channel value such as "罗总→gpustack-cluster" is a failover chain, not a
  // single name; render it as an ordered sequence so the path is legible.
  const channelChain = (log && log.channel ? String(log.channel) : '')
    .split('→')
    .map((name) => name.trim())
    .filter(Boolean);
  const isErrorLog = !!log && ((log.status_code || 200) >= 400 || hasUpstreamDetail);
  // 499 ("client closed request") is synthesized by the gateway itself when it
  // notices the downstream caller hung up; no upstream provider ever returns
  // it. Every surface that talks about the failure cause must branch on this,
  // otherwise a client-side abort gets misattributed to the provider.
  const isClientClosed = !!log && log.status_code === 499;

  // Heuristic remediation hint.
  //
  // This is derived ONLY from the HTTP status code and the captured error text,
  // so it must be presented as a *possible* cause with a suggested next step —
  // not as a verified root cause. The upstream error message (shown separately)
  // is the authoritative evidence; the classification below cannot see the
  // upstream's actual reason.
  const rca = (() => {
    if (!log) return null;
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

    // 499 MUST be classified before the generic upstream-failure heuristics
    // below (and before the final "upstream returned an error status"
    // fallback): it is >= 400, so without this explicit branch it would fall
    // through and be blamed on the provider — including the harmful advice to
    // shift channel weights. 499 is in fact a gateway-synthesized audit status
    // meaning the *client* disconnected before the response completed, so the
    // remediation direction is the caller side, not the upstream side.
    if (isClientClosed) {
      const hasBilledOutput = (log.completion_tokens || 0) > 0;
      // `estimated` means the provider never reported usage before the client
      // left and the count is the local estimator's approximation. Describing it
      // as "actually consumed" would overstate the evidence — and inventing
      // certainty is exactly what this branch exists to stop.
      const estimatedUsage = log.usage_source === 'estimated';
      const usageCopy = !hasBilledOutput
        ? ''
        : estimatedUsage
          ? isZh
            ? `本次记录有 ${log.completion_tokens} 个输出 Token（上游未回报用量，此处为本地估算），已按估算用量计费，并非上游故障。`
            : ` This record has ${log.completion_tokens} output tokens, estimated locally because the provider did not report usage before the client left; the estimated usage was billed and this is not an upstream failure.`
          : isZh
            ? `本次记录仍有 ${log.completion_tokens} 个输出 Token，说明上游已完成生成，用量已按实际消耗计费，并非上游故障。`
            : ` This record still has ${log.completion_tokens} output tokens, so the provider had already finished generating and the usage was billed — it is not an upstream failure.`;
      return {
        type: 'warning',
        badge: isZh ? '499 客户端断开' : '499 Client Closed',
        title: isZh
          ? '本次请求由客户端提前断开（网关合成状态码，非上游返回）'
          : 'Request aborted by the client (gateway-synthesized status, not an upstream response)',
        summary: isZh
          ? `499 是网关检测到下游连接已关闭后写入的合成状态码，上游服务商不会返回该状态码。${usageCopy}排查方向应在调用方一侧：HTTP 客户端超时（例如 120s 请求超时）、反向代理 / LB 空闲超时、网络中断或客户端主动取消。无需切换渠道权重，也无需按上游故障处理。`
          : `499 is synthesized by the gateway when it detects the downstream connection was closed; an upstream provider never returns this status.${usageCopy} Investigate the caller side: HTTP client timeout (e.g. a 120s request timeout), reverse proxy / LB idle timeout, network drop, or a client-side cancel. No channel-weight failover is needed.`
        // Intentionally no actionLabel/actionTab: sending the operator to the
        // channel/models screens would imply an upstream fault that does not
        // exist. The panel only renders the action button when actionLabel is set.
      };
    }

    if (code === 429 || errStr.includes('rate limit') || errStr.includes('tpm') || errStr.includes('rpm') || errStr.includes('quota')) {
      return {
        type: 'warning',
        badge: '429 Rate Limit',
        title: isZh ? '可能原因: 上游限流 (TPM/RPM)' : 'Likely Cause: Upstream Rate Limit (TPM/RPM)',
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
        title: isZh ? '可能原因: 上下文超限或参数不兼容' : 'Likely Cause: Context Overflow or Parameter Mismatch',
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
        title: isZh ? '可能原因: 上游凭证失效或权限拒绝' : 'Likely Cause: Upstream Credential Invalid / Unauthorized',
        summary: isZh
          ? `上游通道 [${log.channel || '渠道'}] 绑定的 API 密钥已失效、服务商余额欠费或账号被封禁。请前往【通道管理】更新密钥并执行连通性探测。`
          : `The API key for upstream channel [${log.channel || 'Channel'}] is invalid, expired, or out of balance. Update upstream API key in Channels and re-probe.`,
        actionLabel: isZh ? '前往【通道管理】更新密钥' : 'Update Channel Key',
        actionTab: 'channels'
      };
    }

    if (code === 404 || errStr.includes('not found') || errStr.includes('no route') || errStr.includes('model not found')) {
      return {
        type: 'error',
        badge: '404 Route / Model Missing',
        title: isZh ? '可能原因: 模型路由未配置或上游模型不存在' : 'Likely Cause: Model Route Missing or Upstream Model 404',
        summary: isZh
          ? `客户端请求的模型 [${log.model}] 在网关中没有可用的健康通道映射，或上游渠道返回 404。请在【模型路由】中核查该模型的分流配置与健康通道。`
          : `Requested model [${log.model}] has no active route or upstream returned 404. Check Model Routes for channel bindings.`,
        actionLabel: isZh ? '前往【模型路由】核查映射' : 'Review Model Routes',
        actionTab: 'models'
      };
    }

    if (code === 502 || code === 504 || code === 503 || errStr.includes('timeout') || errStr.includes('deadline')) {
      return {
        type: 'error',
        badge: 'Upstream Timeout',
        title: isZh ? '可能原因: 上游超时或网络不可达' : 'Likely Cause: Upstream Timeout or Network Unreachable',
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
      title: isZh ? '可能原因: 上游返回异常状态' : 'Likely Cause: Upstream Returned an Error Status',
      summary: isZh
        ? `上游服务商返回非预期状态码。已触发链路追踪 Trace ID，建议检查上游日志或在模型路由中将权重调度转移至其他稳定渠道。`
        : `Unexpected response from upstream provider. Shift traffic weight to other active channels in Model Routes.`,
      actionLabel: isZh ? '检查通道服务状态' : 'Inspect Channels',
      actionTab: 'channels'
    };
  })();

  if (!log) return null;

  return (
    <div
      className="fixed inset-0 z-50 overflow-hidden bg-slate-900/40 backdrop-blur-sm animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="fixed inset-y-0 right-0 w-full max-w-xl bg-white dark:bg-[#191B1F] border-l border-[#E4E6E9] dark:border-[#2C3038] shadow-2xl flex flex-col h-full animate-in slide-in-from-right duration-200">
        {/* Header */}
        <div className="p-6 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className={`w-9 h-9 rounded-lg flex items-center justify-center border ${
              isSuccess
                ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-600 dark:text-emerald-400'
                : 'bg-rose-500/10 border-rose-500/30 text-rose-600 dark:text-rose-400'
            }`}>
              <Activity className="w-4 h-4" />
            </div>
            <div>
              <h3 className="font-semibold text-base text-[#202124] dark:text-[#F1F2F4] tracking-tight flex items-center space-x-2">
                <span>{isZh ? '请求审计详情' : 'Audit Log Inspector'}</span>
                <span className={`text-xs px-2 py-0.5 rounded-full font-mono font-medium ${
                  isSuccess
                    ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30'
                    : 'bg-rose-500/10 text-rose-700 dark:text-rose-300 border border-rose-500/30'
                }`}>
                  HTTP {log.status_code || 200}
                </span>
              </h3>
              <p className="text-xs text-[#656B75] dark:text-[#A1A7B0] mt-0.5 font-mono tabular-nums">
                {isZh ? '记录 #' : 'Log #'}{log.id} · {log.created_at ? new Date(log.created_at).toLocaleString() : (isZh ? '刚刚' : 'Just now')}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-lg text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body grid */}
        <div className="p-6 space-y-5 overflow-y-auto flex-1 text-sm">
          {/* Top key metric cards */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div className="p-3.5 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
              <span className="ui-meta block font-medium">{isZh ? '请求模型' : 'Model'}</span>
              <span className="text-xs font-mono font-semibold text-[#202124] dark:text-[#F1F2F4] truncate block mt-1">
                {log.model || '-'}
              </span>
            </div>
            <div className="p-3.5 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
              <span className="ui-meta block font-medium">{isZh ? '路由渠道' : 'Channel'}</span>
              {channelChain.length > 1 ? (
                <span
                  className="flex flex-wrap items-center gap-1 mt-1 font-mono text-[11px] text-[#202124] dark:text-[#F1F2F4]"
                  title={isZh ? '降级链：从左到右依次尝试' : 'Failover chain: tried left to right'}
                >
                  {channelChain.map((name, index) => (
                    <span key={`${name}-${index}`} className="inline-flex items-center gap-1">
                      {index > 0 && <ArrowRight className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0]" />}
                      <span className="px-1.5 py-0.5 rounded-md bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038]">
                        {name}
                      </span>
                    </span>
                  ))}
                </span>
              ) : (
                <span className="text-xs font-medium text-[#202124] dark:text-[#F1F2F4] truncate block mt-1 font-mono">
                  {log.channel || (isZh ? '直通/多源' : 'Direct / Multi')}
                </span>
              )}
            </div>
            <div className="p-3.5 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
              <span className="ui-meta block font-medium">{isZh ? '首字延迟 (TTFT)' : 'TTFT Latency'}</span>
              <span className="text-xs font-mono tabular-nums font-semibold text-amber-600 dark:text-amber-400 block mt-1">
                {log.ttft_ms ? `${log.ttft_ms} ms` : '-'}
              </span>
            </div>
            <div className="p-3.5 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
              <span className="ui-meta block font-medium">{isZh ? '总执行耗时' : 'Total Duration'}</span>
              <span className="text-xs font-mono tabular-nums font-semibold text-[#202124] dark:text-[#F1F2F4] block mt-1">
                {log.duration_ms} ms
              </span>
            </div>
          </div>

          {/* Quick Troubleshooting Actions Toolbar */}
          <div className="flex flex-wrap items-center gap-2 p-2.5 rounded-xl bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038]">
            <span className="text-[11px] font-semibold text-[#656B75] dark:text-[#A1A7B0] px-1">
              {isZh ? '快捷排障:' : 'Diagnostics:'}
            </span>
            <button
              onClick={handleCopyCurl}
              className="px-2.5 py-1 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] text-xs font-medium border border-[#E4E6E9] dark:border-[#2C3038] transition flex items-center space-x-1.5 shadow-2xs cursor-pointer"
              title={isZh ? '复制聊天接口示例，请填写有效密钥；不包含原始请求内容' : 'Copy a chat API example; replace YOUR_API_KEY. Original request body is not included.'}
            >
              {copiedCurl ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Terminal className="w-3.5 h-3.5 text-[#355CFF]" />}
              <span>{copiedCurl ? (isZh ? '已复制 cURL 指令' : 'cURL Copied') : (isZh ? '复制聊天 cURL 示例' : 'Copy chat example')}</span>
            </button>

            {onNavigate && (
              <button
                onClick={() => {
                  onClose();
                  onNavigate('playground');
                }}
                className="px-2.5 py-1 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] text-xs font-medium border border-[#E4E6E9] dark:border-[#2C3038] transition flex items-center space-x-1.5 shadow-2xs cursor-pointer"
                title={isZh ? '在 Playground 演练测试此模型' : 'Test this model in Playground'}
              >
                <Play className="w-3.5 h-3.5 text-emerald-500" />
                <span>{isZh ? 'Playground 演练' : 'Open Playground'}</span>
              </button>
            )}

            {log.trace_id && onFilterBySession && (
              <button
                onClick={() => {
                  onFilterBySession(log.trace_id);
                  onClose();
                }}
                className="px-2.5 py-1 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] text-xs font-medium border border-[#E4E6E9] dark:border-[#2C3038] transition flex items-center space-x-1.5 shadow-2xs cursor-pointer"
                title={isZh ? '在调用日志中锁定该 Trace ID' : 'Filter by this Trace ID'}
              >
                <Activity className="w-3.5 h-3.5 text-amber-500" />
                <span>{isZh ? '追踪全链路' : 'Trace Flow'}</span>
              </button>
            )}
          </div>

          {/* Latency Waterfall & TPS Speed Breakdown */}
          {waterfall && waterfall.dur > 0 && (
            <div className="p-3.5 rounded-xl border border-slate-200/80 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-900/30 space-y-2">
              <div className="flex items-center justify-between text-xs">
                <span className="font-semibold text-slate-800 dark:text-slate-200 flex items-center space-x-1.5">
                  <Clock className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
                  <span>{isZh ? '时延瀑布与解码速率' : 'Latency Waterfall & TPS'}</span>
                </span>
                {waterfall.tps && (
                  <span className={`px-2 py-0.5 rounded-full text-[11px] font-mono font-semibold ${
                    Number(waterfall.tps) >= 50
                      ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30'
                      : Number(waterfall.tps) >= 15
                      ? 'bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20 dark:border-[#91A7FF]/30'
                      : 'bg-rose-500/10 text-rose-700 dark:text-rose-300 border border-rose-500/30'
                  }`}>
                    {Number(waterfall.tps) >= 50 ? '⚡ ' : Number(waterfall.tps) >= 15 ? '🟢 ' : '🐢 '}
                    {waterfall.tps} tok/s {isZh ? '解码速率' : 'TPS'}
                  </span>
                )}
              </div>

              {/* Progress bar */}
              <div className="w-full h-2.5 rounded-full bg-[#E4E6E9] dark:bg-[#2C3038] overflow-hidden flex">
                {waterfall.ttftPct > 0 && (
                  <div
                    style={{ width: `${waterfall.ttftPct}%` }}
                    className="h-full bg-amber-500 transition-all duration-300"
                    title={`TTFT: ${waterfall.ttft}ms (${waterfall.ttftPct}%)`}
                  />
                )}
                {waterfall.genPct > 0 && (
                  <div
                    style={{ width: `${waterfall.genPct}%` }}
                    className="h-full bg-[#355CFF] transition-all duration-300"
                    title={`Generation: ${waterfall.gen}ms (${waterfall.genPct}%)`}
                  />
                )}
              </div>

              {/* Legend & Summary */}
              <div className="flex items-center justify-between text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
                <div className="flex items-center space-x-3">
                  <span className="flex items-center space-x-1">
                    <span className="w-2 h-2 rounded-xs bg-amber-500" />
                    <span>{isZh ? '首字延迟(TTFT): ' : 'TTFT: '}</span>
                    <strong className="text-[#202124] dark:text-[#F1F2F4] font-mono">{waterfall.ttft}ms</strong>
                    <span>({waterfall.ttftPct}%)</span>
                  </span>
                  <span className="flex items-center space-x-1">
                    <span className="w-2 h-2 rounded-xs bg-[#355CFF]" />
                    <span>{isZh ? '流式解码: ' : 'Generation: '}</span>
                    <strong className="text-[#202124] dark:text-[#F1F2F4] font-mono">{waterfall.gen}ms</strong>
                    <span>({waterfall.genPct}%)</span>
                  </span>
                </div>
                <span className="font-mono">{waterfall.dur}ms</span>
              </div>
            </div>
          )}

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
                        className="px-2 py-0.5 rounded-md bg-[#EEF2FF] dark:bg-[#242D48] hover:bg-[#E0E7FF] dark:hover:bg-[#2e3a5e] text-[#355CFF] dark:text-[#91A7FF] text-[11px] font-medium transition cursor-pointer"
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
                    className="p-1 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition cursor-pointer"
                    title={isZh ? '复制对话 ID' : 'Copy Chat ID'}
                  >
                    <Copy className="w-3 h-3" />
                  </button>
                  {onOpenTrace && (
                    <button
                      onClick={() => {
                        onOpenTrace(log.chat_id, 'chat_id');
                        onClose();
                      }}
                      className="px-2 py-0.5 rounded-md bg-[#EEF2FF] dark:bg-[#242D48] hover:bg-[#E0E7FF] dark:hover:bg-[#2e3a5e] text-[#355CFF] dark:text-[#91A7FF] text-[11px] font-medium transition cursor-pointer"
                    >
                      {isZh ? '查看快照' : 'Snapshot'}
                    </button>
                  )}
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
                    className="p-1 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition cursor-pointer"
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
                      className="px-2 py-0.5 rounded-md bg-[#EEF2FF] dark:bg-[#242D48] hover:bg-[#E0E7FF] dark:hover:bg-[#2e3a5e] text-[#355CFF] dark:text-[#91A7FF] text-[11px] font-medium transition cursor-pointer"
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
                {log.usage_source === 'estimated' && (
                  <span className="ml-1.5 px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-100 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 align-middle">
                    {isZh ? '估算' : 'estimated'}
                  </span>
                )}
              </span>
            </div>
            <div className="p-3.5 flex justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '计费扣减' : 'Billed Cost'}</span>
              <div className="flex flex-wrap items-center gap-2 font-mono tabular-nums">
                <span className="font-semibold text-slate-900 dark:text-white">
                  ¥{(log.cost || 0).toFixed(4)}
                </span>
                {log.is_off_peak && (
                  <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300">
                    {isZh ? `闲时 (${Math.round((log.off_peak_discount || 0.5) * 10)}折)` : `Off-peak (${Math.round((1 - (log.off_peak_discount || 0.5)) * 100)}% off)`}
                  </span>
                )}
                {log.cached_tokens > 0 ? (
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800">
                    {isZh ? `缓存: ${log.cached_tokens} Tokens` : `Cache: ${log.cached_tokens} Tokens`}
                  </span>
                ) : (
                  <span className="ui-meta font-sans">{isZh ? '未命中缓存' : 'Cache miss'}</span>
                )}
              </div>
            </div>
            <div className="p-3.5 flex justify-between items-start gap-4">
              <span className="text-slate-500 dark:text-slate-400 font-medium shrink-0">{isZh ? '计费核算公式' : 'Billing Formula'}</span>
              <div className="text-right font-mono text-xs text-slate-700 dark:text-slate-300">
                {log.cost > 0 ? (() => {
                  const pTokens = log.prompt_tokens || 0;
                  const cTokens = log.completion_tokens || 0;
                  const cachedTokens = Math.min(pTokens, log.cached_tokens || 0);
                  const uncachedPrompt = Math.max(0, pTokens - cachedTokens);

                  if (matchedPrice) {
                    const pPrice = matchedPrice.prompt_price ?? 0;
                    const cPrice = matchedPrice.completion_price ?? 0;
                    const cachePrice = matchedPrice.cache_read_price ?? 0;
                    const fixedPrice = matchedPrice.fixed_price ?? 0;

                    const parts = [];
                    if (cachedTokens > 0) {
                      parts.push(`${isZh ? '未缓存' : 'Uncached'} ${uncachedPrompt.toLocaleString()} × ¥${pPrice}/1M`);
                      parts.push(`${isZh ? '缓存' : 'Cache'} ${cachedTokens.toLocaleString()} × ¥${cachePrice}/1M`);
                    } else {
                      parts.push(`${isZh ? '输入' : 'Prompt'} ${pTokens.toLocaleString()} × ¥${pPrice}/1M`);
                    }
                    parts.push(`${isZh ? '输出' : 'Completion'} ${cTokens.toLocaleString()} × ¥${cPrice}/1M`);
                    if (fixedPrice > 0) {
                      parts.push(`${isZh ? '固定' : 'Fixed'} ¥${fixedPrice} × ${matchedPrice.billable_units || 1}`);
                    }

                    const formulaBase = parts.join(' + ');

                    return (
                      <div className="flex flex-wrap items-center justify-end gap-1">
                        <span>({formulaBase}) × {matchedPrice.group_multiplier ?? 1}</span>
                        {log.is_off_peak && (
                          <span className="text-[#355CFF] dark:text-[#91A7FF] font-medium">
                            {' '}× {isZh ? `闲时 ${Math.round((log.off_peak_discount || 0.5) * 10)}折` : `Off-peak ${Math.round((1 - (log.off_peak_discount || 0.5)) * 100)}% off`}
                          </span>
                        )}
                        <span className="font-semibold text-slate-900 dark:text-white"> = ¥{(log.cost || 0).toFixed(4)}</span>
                      </div>
                    );
                  }

                  return (
                    <span className="font-sans text-slate-500">
                      {isZh ? '该记录没有历史费率快照，以已入账金额为准。' : 'No historical rate snapshot is available. The recorded charge is authoritative.'}
                    </span>
                  );
                })() : (
                  <span className="text-slate-500 font-sans">{isZh ? '本次请求未产生扣费' : 'No charge recorded for this request'}</span>
                )}
              </div>
            </div>
            <div className="p-3.5 flex justify-between">
              <span className="text-slate-500 dark:text-slate-400 font-medium">{isZh ? '客户端 IP' : 'Client IP'}</span>
              <span className="font-mono text-slate-600 dark:text-slate-300">{log.ip || '-'}</span>
            </div>
          </div>

          {/* Error notice if failed */}
          {log.error_message && (
            <div className="p-4 rounded-xl bg-rose-50/70 dark:bg-rose-950/30 border border-rose-200/80 dark:border-rose-900/50 text-rose-800 dark:text-rose-300 text-xs">
              <div className="font-semibold mb-1 flex items-center justify-between gap-2">
                <span className="flex items-center space-x-1.5">
                  <AlertCircle className="w-3.5 h-3.5" />
                  <span>
                    {isClientClosed
                      ? (isZh ? '网关记录的断开原因（非上游回包）' : 'Gateway-recorded disconnect reason (not a raw upstream response)')
                      : (isZh ? '上游异常原始回包' : 'Raw Upstream Error Message')}
                  </span>
                </span>
                <button
                  onClick={() => onCopy(errorMessage)}
                  className="text-[11px] font-medium hover:underline flex items-center space-x-1 cursor-pointer"
                  title={isZh ? '复制完整错误文本' : 'Copy the full error text'}
                >
                  <Copy className="w-3 h-3" />
                  <span>{isZh ? '复制错误' : 'Copy error'}</span>
                </button>
              </div>
              <div className="font-mono mt-1 text-[11px] whitespace-pre-wrap break-all">{log.error_message}</div>
            </div>
          )}

          {/* Heuristic remediation hint derived from the status code. The
              upstream's own error message is the authoritative evidence and is
              rendered below it when present. */}
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
              {hasUpstreamDetail && (
                <div className="mt-2.5 pt-2.5 border-t border-current/10">
                  <span className="block text-xs font-semibold uppercase tracking-wide opacity-75 mb-1">
                    {isClientClosed
                      ? (isZh ? '网关侧记录（非上游回包）' : 'Gateway-side record (not an upstream response)')
                      : (isZh ? '上游原始错误（权威依据）' : 'Upstream error (authoritative)')}
                  </span>
                  <code className="block font-mono text-[11px] break-all opacity-90">{errorMessage}</code>
                </div>
              )}
              {!hasUpstreamDetail && isErrorLog && (
                <p className="mt-2 text-xs opacity-75">
                  {isClientClosed
                    ? (isZh
                      ? '说明：该状态码由网关在下游连接断开时生成，不是上游服务商返回的状态码，因此不作为上游故障的依据。'
                      : 'Note: this status code is synthesized by the gateway on downstream disconnect; it is not an upstream provider status and is not evidence of an upstream failure.')
                    : (isZh
                      ? '本次请求未记录上游错误详情，以上仅为基于状态码的推测，不能作为根因结论。'
                      : 'No upstream error detail was recorded for this request; the above is inferred from the status code only.')}
                </p>
              )}
              {rca.actionLabel && onNavigate && (
                <div className="mt-3 pt-2.5 border-t border-slate-200/60 dark:border-slate-800/60 flex justify-end">
                  <button
                    onClick={() => {
                      onClose();
                      onNavigate(rca.actionTab);
                    }}
                    className="ui-btn ui-btn-primary ui-btn-sm"
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
                className="text-xs text-[#355CFF] dark:text-[#91A7FF] hover:underline flex items-center space-x-1 font-medium cursor-pointer"
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
