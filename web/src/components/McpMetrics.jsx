import { Cpu, Server, Globe, Check, Copy, Layers } from 'lucide-react';

export default function McpMetrics({
  copiedKey, handleCopyText, handleToggleGlobalMcp, isZh, mcpSettings, onlineServersCount, servers, sseUrl,
  togglingGlobal, totalToolsCount
}) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-5 p-6 rounded-2xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] shadow-xs">
      {/* 指标 1：协议总闸 */}
      <div className="flex items-center gap-4">
        <div className="w-12 h-12 rounded-2xl bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border border-[#355CFF]/20 dark:border-[#91A7FF]/30 flex items-center justify-center shrink-0 shadow-2xs">
          <Cpu className="w-6 h-6" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="ui-meta font-medium">{isZh ? '协议总闸状态' : 'Master Switch'}</div>
          <div className="flex items-center gap-2.5 mt-1">
            <button
              onClick={handleToggleGlobalMcp}
              disabled={togglingGlobal}
              className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${mcpSettings.mcp_enabled ? 'bg-[#355CFF]' : 'bg-[#E4E6E9] dark:bg-[#2C3038]'}`}
              title={mcpSettings.mcp_enabled ? (isZh ? '点击关闭 MCP' : 'Click to disable MCP') : (isZh ? '点击开启 MCP' : 'Click to enable MCP')}
            >
              <span
                className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ease-in-out ${mcpSettings.mcp_enabled ? 'translate-x-4' : 'translate-x-0'}`}
              />
            </button>
            <span className={`text-xs font-semibold ${mcpSettings.mcp_enabled ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}>
              {mcpSettings.mcp_enabled ? (isZh ? '已激活运行' : 'Active') : (isZh ? '已全局停用' : 'Disabled')}
            </span>
          </div>
        </div>
      </div>

      {/* 指标 2：活跃服务 */}
      <div className="flex items-center gap-4">
        <div className="w-12 h-12 rounded-2xl bg-emerald-50 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border border-emerald-200/60 dark:border-emerald-800/60 flex items-center justify-center shrink-0 shadow-2xs">
          <Server className="w-6 h-6" />
        </div>
        <div>
          <div className="ui-meta font-medium">{isZh ? '已启用 MCP 服务' : 'Enabled Servers'}</div>
          <div className="text-lg font-bold text-slate-900 dark:text-white mt-0.5">
            {onlineServersCount} <span className="ui-meta font-normal">/ {servers.length} {isZh ? '已配置' : 'configured'}</span>
          </div>
        </div>
      </div>

      {/* 指标 3：挂载工具 */}
      <div className="flex items-center gap-4">
        <div className="w-12 h-12 rounded-2xl bg-blue-50 dark:bg-blue-950/40 text-[#355CFF] dark:text-[#91A7FF] border border-blue-200/60 dark:border-blue-800/60 flex items-center justify-center shrink-0 shadow-2xs">
          <Layers className="w-6 h-6" />
        </div>
        <div>
          <div className="ui-meta font-medium">{isZh ? '聚合工具总数' : 'Mounted Tools'}</div>
          <div className="text-lg font-bold text-slate-900 dark:text-white mt-0.5">
            {totalToolsCount} <span className="ui-meta font-normal">{isZh ? '个原子能力' : 'tools registered'}</span>
          </div>
        </div>
      </div>

      {/* 指标 4：统一 SSE 端点 */}
      <div className="flex items-center gap-4">
        <div className="w-12 h-12 rounded-2xl bg-blue-50 dark:bg-blue-950/40 text-[#355CFF] dark:text-[#91A7FF] border border-blue-200/60 dark:border-blue-800/60 flex items-center justify-center shrink-0 shadow-2xs">
          <Globe className="w-6 h-6" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="ui-meta font-medium">{isZh ? '网关统一 SSE 端点' : 'SSE Gateway Endpoint'}</div>
          <div className="flex items-center gap-2 mt-1">
            <code className="px-2 py-0.5 rounded-md font-mono text-xs bg-slate-100 dark:bg-slate-900 text-slate-700 dark:text-slate-300 border border-slate-200/80 dark:border-slate-800 truncate">
              /mcp/sse
            </code>
            <button
              onClick={() => handleCopyText(sseUrl, 'sse-endpoint')}
              className="p-1 text-slate-400 hover:text-[#355CFF] dark:hover:text-[#91A7FF] transition-colors"
              title={isZh ? '复制完整端点 URL' : 'Copy Full Endpoint URL'}
            >
              {copiedKey === 'sse-endpoint' ? <Check className="w-4 h-4 text-emerald-500" /> : <Copy className="w-4 h-4" />}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
