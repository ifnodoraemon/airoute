import { Plus, RefreshCw, Radio, FileCode } from 'lucide-react';

export default function McpHeader({
  fetchServersAndSettings, handleOpenProbe, isZh, loading, setAddServerModalOpen, setConfigModalOpen,
  setEditingServerId, setNewServer, setSelectedServerForConfig
}) {
  return (
    <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-slate-200/70 dark:border-slate-800/80">
      <div>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-bold tracking-tight text-slate-900 dark:text-white">
            {isZh ? 'Model Context Protocol (MCP) 广场' : 'Model Context Protocol (MCP) Hub'}
          </h1>
          <span className="px-2.5 py-0.5 text-xs font-semibold rounded-full bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20 dark:border-[#91A7FF]/30">
            JSON-RPC 2.0
          </span>
        </div>
        <p className="text-xs text-slate-500 dark:text-slate-400 mt-1.5">
          {isZh ? '连接外部系统与协议扩展，支持 SSE 与 Stdio 双向传输，提供统一鉴权、安全脱敏与客户端无缝对接。' : 'Connect external tools and protocol servers via SSE & Stdio with unified auth, data redaction, and client integration.'}
        </p>
      </div>

      <div className="flex items-center gap-2.5 flex-wrap">
        <button
          onClick={() => handleOpenProbe(null)}
          className="inline-flex items-center gap-2 px-3.5 py-2 text-xs font-medium text-slate-700 dark:text-slate-200 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-xl hover:bg-slate-50 dark:hover:bg-slate-700 transition-colors shadow-2xs"
        >
          <Radio className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
          {isZh ? '协议探针' : 'Protocol Probe'}
        </button>
        <button
          onClick={() => { setSelectedServerForConfig(null); setConfigModalOpen(true); }}
          className="inline-flex items-center gap-2 px-3.5 py-2 text-xs font-medium text-slate-700 dark:text-slate-200 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-xl hover:bg-slate-50 dark:hover:bg-slate-700 transition-colors shadow-2xs"
        >
          <FileCode className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
          {isZh ? '客户端配置' : 'Client Config'}
        </button>
        <button
          onClick={() => { setEditingServerId(''); setNewServer({ id: '', name: '', description: '', category: 'dev', transport: 'stdio', endpoint: '', author: 'Community', version: '1.0.0', tools: '', prompts: '', resources: '', env_vars: '', enabled: true }); setAddServerModalOpen(true); }}
          className="ui-btn ui-btn-primary ui-btn-sm"
        >
          <Plus className="w-3.5 h-3.5" />
          {isZh ? '接入新服务' : 'Add Server'}
        </button>
        <button
          onClick={fetchServersAndSettings}
          disabled={loading}
          className="p-2 text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 transition-colors shadow-2xs"
          title={isZh ? '刷新服务状态' : 'Refresh Server Status'}
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>
    </div>
  );
}
