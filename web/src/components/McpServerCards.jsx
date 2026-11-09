import { Server, Globe, Terminal, Trash2, Pencil, Layers, Database, Code2, Sparkles, Radio } from 'lucide-react';

export default function McpServerCards({
  filteredServers, handleDeleteServer, handleOpenProbe, handleToggleServer, isZh, setAddServerModalOpen,
  setConfigModalOpen, setEditingServerId, setNewServer, setSelectedServerForConfig, togglingServer
}) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {filteredServers.map(server => {
        const isToggling = !!togglingServer[server.id];
        const isCustom = server.id !== 'airoute-gateway';

        return (
          <div
            key={server.id}
            className={`flex flex-col justify-between p-6 rounded-2xl border transition-all duration-200 ${server.enabled ? 'border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] shadow-xs hover:shadow-md hover:border-slate-300 dark:hover:border-slate-700' : 'border-[#E4E6E9]/50 dark:border-[#2C3038]/50 bg-slate-50/50 dark:bg-slate-900/30 opacity-70'}`}
          >
            <div>
              {/* 卡片头部 */}
              <div className="flex items-start justify-between gap-3 mb-3">
                <div className="flex items-center gap-3">
                  <div className={`w-11 h-11 rounded-2xl flex items-center justify-center shrink-0 shadow-2xs ${server.id === 'airoute-gateway' ? 'bg-blue-50 dark:bg-blue-950/70 text-[#355CFF] dark:text-[#91A7FF] border border-blue-200/50 dark:border-blue-800/50' : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] border border-[#E4E6E9] dark:border-[#2C3038]'}`}>
                    {server.category === 'ops' ? <Server className="w-5 h-5" /> :
                     server.category === 'dev' ? <Code2 className="w-5 h-5" /> :
                     server.category === 'search' ? <Globe className="w-5 h-5" /> :
                     server.category === 'db' ? <Database className="w-5 h-5" /> :
                     server.category === 'ai' ? <Sparkles className="w-5 h-5" /> :
                     <Layers className="w-5 h-5" />}
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <h3 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4] truncate">
                        {server.name}
                      </h3>
                      <span className={`px-2 py-0.5 text-xs font-mono font-semibold rounded-md ${server.transport === 'sse' ? 'bg-blue-50 dark:bg-blue-950/60 text-[#355CFF] dark:text-[#91A7FF]' : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]'}`}>
                        {server.transport.toUpperCase()}
                      </span>
                    </div>
                    <div className="ui-meta font-mono truncate mt-0.5">
                      {server.id}
                    </div>
                  </div>
                </div>

                <button
                  onClick={() => handleToggleServer(server.id, server.enabled)}
                  disabled={isToggling}
                  className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${server.enabled ? 'bg-[#355CFF]' : 'bg-slate-300 dark:bg-slate-600'}`}
                  title={server.enabled ? (isZh ? '点击停用' : 'Click to disable') : (isZh ? '点击启用' : 'Click to enable')}
                >
                  <span
                    className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ease-in-out ${server.enabled ? 'translate-x-4' : 'translate-x-0'}`}
                  />
                </button>
              </div>

              {/* 描述文案 */}
              <p className="text-xs text-[#656B75] dark:text-[#A1A7B0] leading-relaxed line-clamp-2 mb-3.5 min-h-[38px]">
                {server.description}
              </p>

              {/* 暴露的 Tools 标签组 */}
              {server.tools && server.tools.length > 0 && (
                <div className="flex items-center gap-1.5 flex-wrap mb-4 text-xs">
                  <span className="ui-meta">{isZh ? '能力:' : 'Tools:'}</span>
                  {server.tools.slice(0, 3).map(t => (
                    <span
                      key={t}
                      className="px-2 py-0.5 font-mono text-xs bg-[#F0F1F2] dark:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] rounded-lg border border-[#E4E6E9] dark:border-[#2C3038]"
                    >
                      {t}
                    </span>
                  ))}
                  {server.tools.length > 3 && (
                    <span className="ui-meta">+{server.tools.length - 3}</span>
                  )}
                </div>
              )}
            </div>

            {/* 卡片底部操作栏 */}
            <div className="pt-3.5 border-t border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <button
                  onClick={() => handleOpenProbe(server)}
                  className="px-2.5 py-1.5 text-xs font-medium text-[#355CFF] dark:text-[#91A7FF] hover:bg-blue-50 dark:hover:bg-blue-950/60 rounded-xl transition-colors inline-flex items-center gap-1.5"
                >
                  <Radio className="w-3.5 h-3.5" />
                  {isZh ? '调试探针' : 'Probe'}
                </button>
                {server.id !== 'airoute-gateway' && <button className="ui-btn-secondary" title={isZh ? '编辑服务' : 'Edit server'} onClick={() => {
                  setEditingServerId(server.id);
                  setNewServer({ ...server, tools: (server.tools || []).join(', '), prompts: (server.prompts || []).join(', '), resources: (server.resources || []).join(', '), env_vars: server.env_vars || '' });
                  setAddServerModalOpen(true);
                }}><Pencil className="w-3.5 h-3.5" />{isZh ? '编辑' : 'Edit'}</button>}
                <button
                  onClick={() => { setSelectedServerForConfig(server); setConfigModalOpen(true); }}
                  className="px-2.5 py-1.5 text-xs font-medium text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-xl transition-colors inline-flex items-center gap-1.5 border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs"
                >
                  <Terminal className="w-3.5 h-3.5" />
                  {isZh ? '配置' : 'Config'}
                </button>
              </div>

              {isCustom && (
                <button
                  onClick={() => handleDeleteServer(server.id)}
                  className="p-1.5 text-slate-400 hover:text-rose-500 rounded-xl transition-colors"
                  title={isZh ? '移除自定义 MCP 服务' : 'Remove custom MCP server'}
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
