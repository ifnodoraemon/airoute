import React, { useState } from 'react';
import { X, Copy, Check, Activity, Clock, Zap, Server, Key, AlertCircle, Shield, Trash2 } from 'lucide-react';

export default function LogDetailModal({ log, onClose, onCopy, onFilterBySession, onDeleteLog }) {
  if (!log) return null;

  const [copied, setCopied] = useState(false);

  const handleCopyRaw = () => {
    onCopy(JSON.stringify(log, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const isSuccess = log.status_code >= 200 && log.status_code < 300;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm animate-in fade-in">
      <div className="bg-white dark:bg-[#111726] border border-slate-200 dark:border-slate-800 rounded-3xl w-full max-w-2xl shadow-2xl overflow-hidden flex flex-col max-h-[85vh]">
        {/* Header */}
        <div className="p-6 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between bg-slate-50/50 dark:bg-slate-900/50">
          <div className="flex items-center space-x-3">
            <div className={`w-10 h-10 rounded-2xl flex items-center justify-center border ${
              isSuccess
                ? 'bg-emerald-50 dark:bg-emerald-900/30 border-emerald-200 dark:border-emerald-700/50 text-emerald-600 dark:text-emerald-400'
                : 'bg-rose-50 dark:bg-rose-900/30 border-rose-200 dark:border-rose-700/50 text-rose-600 dark:text-rose-400'
            }`}>
              <Activity className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-base text-slate-900 dark:text-slate-100 tracking-tight flex items-center space-x-2">
                <span>请求审计详情</span>
                <span className={`text-xs px-2 py-0.5 rounded-full font-semibold ${
                  isSuccess
                    ? 'bg-emerald-100 dark:bg-emerald-900/60 text-emerald-800 dark:text-emerald-300'
                    : 'bg-rose-100 dark:bg-rose-900/60 text-rose-800 dark:text-rose-300'
                }`}>
                  HTTP {log.status_code || 200}
                </span>
              </h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                记录 ID: #{log.id} · 时间: {log.created_at ? new Date(log.created_at).toLocaleString() : '刚刚'}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-xl text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body grid */}
        <div className="p-6 space-y-4 overflow-y-auto flex-1 text-sm">
          {/* Top key metric cards */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/80 dark:border-slate-800/80">
              <span className="text-[11px] text-slate-400 block font-medium">请求模型</span>
              <span className="text-xs font-mono font-bold text-indigo-600 dark:text-indigo-400 truncate block mt-1">
                {log.model || '-'}
              </span>
            </div>
            <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/80 dark:border-slate-800/80">
              <span className="text-[11px] text-slate-400 block font-medium">路由渠道</span>
              <span className="text-xs font-bold text-slate-800 dark:text-slate-200 truncate block mt-1">
                {log.channel || '直通/多源'}
              </span>
            </div>
            <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/80 dark:border-slate-800/80">
              <span className="text-[11px] text-slate-400 block font-medium">首字延迟 (TTFT)</span>
              <span className="text-xs font-bold text-amber-600 dark:text-amber-400 block mt-1">
                {log.ttft_ms ? `${log.ttft_ms} ms` : '-'}
              </span>
            </div>
            <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/80 dark:border-slate-800/80">
              <span className="text-[11px] text-slate-400 block font-medium">总执行耗时</span>
              <span className="text-xs font-bold text-slate-900 dark:text-slate-100 block mt-1">
                {log.duration_ms} ms
              </span>
            </div>
          </div>

          {/* Details list */}
          <div className="rounded-2xl border border-slate-200/80 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800/60 overflow-hidden text-xs">
            <div className="p-3 flex items-center justify-between bg-purple-50/50 dark:bg-purple-950/30">
              <span className="text-slate-500 font-medium">链路追踪 ID (Trace ID)</span>
              <div className="flex items-center space-x-2">
                <span className="font-mono text-xs font-bold text-purple-700 dark:text-purple-300">
                  {log.trace_id || log.chat_id || '未生成'}
                </span>
                {(log.trace_id || log.chat_id) && (
                  <>
                    <button
                      onClick={() => {
                        onCopy(log.trace_id || log.chat_id);
                        setCopied(true);
                        setTimeout(() => setCopied(false), 2000);
                      }}
                      className="p-1 hover:bg-white dark:hover:bg-slate-800 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition cursor-pointer"
                      title="复制 Trace ID"
                    >
                      <Copy className="w-3 h-3" />
                    </button>
                    {onFilterBySession && (
                      <button
                        onClick={() => {
                          onFilterBySession(log.trace_id || log.chat_id);
                          onClose();
                        }}
                        className="px-2 py-0.5 rounded-lg bg-purple-600 hover:bg-purple-700 text-white text-[11px] font-medium transition cursor-pointer"
                      >
                        追踪全链路
                      </button>
                    )}
                  </>
                )}
              </div>
            </div>
            <div className="p-3 flex items-center justify-between bg-indigo-50/40 dark:bg-indigo-950/30">
              <span className="text-slate-500 font-medium">对话 ID (Chat ID)</span>
              <div className="flex items-center space-x-2">
                <span className="font-mono text-xs font-bold text-indigo-700 dark:text-indigo-300">
                  {log.chat_id || log.session_id || '未指定 (自动生成)'}
                </span>
                {(log.chat_id || log.session_id) && (
                  <>
                    <button
                      onClick={() => {
                        onCopy(log.chat_id || log.session_id);
                        setCopied(true);
                        setTimeout(() => setCopied(false), 2000);
                      }}
                      className="p-1 hover:bg-white dark:hover:bg-slate-800 rounded text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition cursor-pointer"
                      title="复制对话 ID"
                    >
                      <Copy className="w-3 h-3" />
                    </button>
                    {onFilterBySession && (
                      <button
                        onClick={() => {
                          onFilterBySession(log.chat_id || log.session_id);
                          onClose();
                        }}
                        className="px-2 py-0.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-[11px] font-medium transition cursor-pointer"
                      >
                        筛选此对话
                      </button>
                    )}
                  </>
                )}
              </div>
            </div>
            {log.session_id && log.session_id !== log.chat_id && (
              <div className="p-3 flex items-center justify-between">
                <span className="text-slate-500 font-medium">关联会话 (Session)</span>
                <span className="font-mono text-xs text-slate-600 dark:text-slate-400">{log.session_id}</span>
              </div>
            )}
            <div className="p-3 flex justify-between bg-slate-50/50 dark:bg-slate-900/30">
              <span className="text-slate-500 font-medium">应用 / 团队</span>
              <span className="font-semibold text-slate-800 dark:text-slate-200">{log.tenant_id || 'anonymous'}</span>
            </div>
            <div className="p-3 flex justify-between">
              <span className="text-slate-500 font-medium">API 密钥</span>
              <span className="font-mono text-indigo-600 dark:text-indigo-400">{log.virtual_key || '无'}</span>
            </div>
            <div className="p-3 flex justify-between bg-slate-50/50 dark:bg-slate-900/30">
              <span className="text-slate-500 font-medium">Token 明细</span>
              <span className="text-slate-800 dark:text-slate-200">
                输入: <strong className="text-indigo-600 dark:text-indigo-400">{log.prompt_tokens}</strong> · 
                输出: <strong className="text-emerald-600 dark:text-emerald-400">{log.completion_tokens}</strong> · 
                合计: <strong className="text-slate-900 dark:text-slate-100">{log.total_tokens}</strong>
              </span>
            </div>
            <div className="p-3 flex justify-between">
              <span className="text-slate-500 font-medium">计费扣减</span>
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono font-bold text-slate-900 dark:text-slate-100">
                  ¥{(log.cost || 0).toFixed(4)}
                </span>
                {log.is_off_peak && (
                  <span className="px-2 py-0.5 rounded-full text-[11px] font-bold bg-indigo-50 text-indigo-700 border border-indigo-200">
                    闲时 ({Math.round((log.off_peak_discount || 0.5) * 10)}折)
                  </span>
                )}
                {log.cached_tokens > 0 ? (
                  <span className="px-2 py-0.5 rounded-full text-[11px] font-bold bg-emerald-50 text-emerald-700 border border-emerald-200">
                    缓存: {log.cached_tokens} Tokens
                  </span>
                ) : (
                  <span className="text-slate-400 text-[11px]">未命中缓存</span>
                )}
              </div>
            </div>
            <div className="p-3 flex justify-between bg-slate-50/50 dark:bg-slate-900/30">
              <span className="text-slate-500 font-medium">客户端 IP</span>
              <span className="font-mono text-slate-600 dark:text-slate-300">{log.ip || '127.0.0.1'}</span>
            </div>
          </div>

          {/* Error notice if failed */}
          {log.error_message && (
            <div className="p-3.5 rounded-2xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-900/50 text-rose-800 dark:text-rose-300 text-xs">
              <div className="font-semibold mb-1 flex items-center space-x-1.5">
                <AlertCircle className="w-3.5 h-3.5" />
                <span>上游异常信息</span>
              </div>
              <div className="font-mono">{log.error_message}</div>
            </div>
          )}

          {/* Raw JSON toggle */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-xs font-semibold text-slate-600 dark:text-slate-400">原始日志 JSON</span>
              <button
                onClick={handleCopyRaw}
                className="text-xs text-indigo-600 dark:text-indigo-400 hover:underline flex items-center space-x-1 font-medium"
              >
                {copied ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                <span>{copied ? '已复制' : '复制 JSON'}</span>
              </button>
            </div>
            <pre className="p-3 rounded-2xl bg-slate-950 text-slate-300 font-mono text-[11px] overflow-x-auto max-h-40 leading-normal">
              {JSON.stringify(log, null, 2)}
            </pre>
          </div>
        </div>

        {/* Footer */}
        <div className="p-4 bg-slate-50 dark:bg-slate-900/50 border-t border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
          <div>
            {onDeleteLog && (
              <button
                onClick={() => {
                  if (window.confirm('确认删除该条审计调用日志？')) {
                    onDeleteLog(log.id);
                    onClose();
                  }
                }}
                className="px-3 py-1.5 rounded-xl text-xs font-medium text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-950/50 transition flex items-center space-x-1.5 cursor-pointer border border-rose-200 dark:border-rose-900/60"
              >
                <Trash2 className="w-3.5 h-3.5" />
                <span>删除此日志</span>
              </button>
            )}
          </div>
          <button
            onClick={onClose}
            className="px-4 py-2 bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-xl text-xs font-medium transition cursor-pointer"
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  );
}
