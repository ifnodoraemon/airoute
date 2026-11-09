import { TOOL_TEMPLATES, getToolInfo, getGroupLabel, getDefaultArgsForTool } from '../lib/mcpToolMetadata';
import useMcpPresentation from '../hooks/useMcpPresentation';
import useMcpProbe from '../hooks/useMcpProbe';
import useMcpServers from '../hooks/useMcpServers';
import useMcpState from '../hooks/useMcpState';
import McpProbeModal from './McpProbeModal';
import McpHeader from './McpHeader';
import McpMetrics from './McpMetrics';
import McpServerCards from './McpServerCards';
import McpConfigModal from './McpConfigModal';
import McpRegistrationModal from './McpRegistrationModal';

import { Cpu, Search } from 'lucide-react';

export default function McpIntegrationView({ adminFetch, onCopy, showToast, lang = 'zh', }) {
  const isZh = lang === 'zh';
  const {
    activeConfigClient, addServerModalOpen, configModalOpen, configToken, copiedKey, editingServerId,
    getInitialToken, lastSentPayload, loading, mcpSettings, messagesUrl, newServer, probeFailed,
    probeHttpStatus, probeLatency, probeMethod, probeModalOpen, probeResult, probeRunning, probeServer,
    probeToken, probeToolArgs, probeToolName, requestViewTab, savingServer, searchQuery, selectedCategory,
    selectedServerForConfig, servers, setActiveConfigClient, setAddServerModalOpen, setConfigModalOpen,
    setConfigToken, setCopiedKey, setEditingServerId, setLastSentPayload, setLoading, setMcpSettings,
    setNewServer, setProbeHttpStatus, setProbeLatency, setProbeMethod, setProbeModalOpen, setProbeResult,
    setProbeRunning, setProbeServer, setProbeToken, setProbeToolArgs, setProbeToolName, setRequestViewTab,
    setSavingServer, setSearchQuery, setSelectedCategory, setSelectedServerForConfig, setServers,
    setTogglingGlobal, setTogglingServer, sseUrl, togglingGlobal, togglingServer
  } = useMcpState({
    getDefaultArgsForTool
  });

  const {
    fetchServersAndSettings, handleCreateServer, handleDeleteServer, handleToggleGlobalMcp, handleToggleServer
  } = useMcpServers({
    adminFetch, isZh, mcpSettings, newServer, savingServer, setAddServerModalOpen, setLoading, setMcpSettings,
    setNewServer, setSavingServer, setServers, setTogglingGlobal, setTogglingServer, showToast, togglingGlobal
  });

  const {
    getCurlRequestText, getCurrentRpcPayload, getWireRequestText, handleOpenProbe, handleRunProbe,
    handleSelectTool
  } = useMcpProbe({
    getDefaultArgsForTool, getInitialToken, isZh, lastSentPayload, messagesUrl, probeMethod, probeServer,
    probeToken, probeToolArgs, probeToolName, setLastSentPayload, setProbeHttpStatus, setProbeLatency,
    setProbeMethod, setProbeModalOpen, setProbeResult, setProbeRunning, setProbeServer, setProbeToken,
    setProbeToolArgs, setProbeToolName, showToast
  });

  const {
    categories, filteredServers, getClaudeConfig, getClineConfig, getCurlSnippet, getCursorConfig,
    getPythonSnippet, handleCopyText, onlineServersCount, totalToolsCount
  } = useMcpPresentation({
    configToken, isZh, messagesUrl, onCopy, searchQuery, selectedCategory, selectedServerForConfig, servers,
    setCopiedKey, showToast, sseUrl
  });

  return (
    <div className="space-y-8 animate-in fade-in duration-200">
      {/* 顶部标题与轻量操作栏 */}
      <McpHeader {...{
        fetchServersAndSettings, handleOpenProbe, isZh, loading, setAddServerModalOpen, setConfigModalOpen,
        setEditingServerId, setNewServer, setSelectedServerForConfig
      }} />

      {/* 核心指标与规格看板 */}
      <McpMetrics {...{
        copiedKey, handleCopyText, handleToggleGlobalMcp, isZh, mcpSettings, onlineServersCount, servers, sseUrl,
        togglingGlobal, totalToolsCount
      }} />

      {/* 搜索与分类导航 */}
      <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-4 bg-white dark:bg-[#191B1F] p-2.5 rounded-2xl border border-slate-200/80 dark:border-[#2C3038] shadow-2xs">
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 md:pb-0 scrollbar-none px-1">
          {categories.map(cat => {
            const Icon = cat.icon;
            const active = selectedCategory === cat.id;
            return (
              <button
                key={cat.id}
                onClick={() => setSelectedCategory(cat.id)}
                className={`flex items-center gap-2 px-3.5 py-2 text-xs font-medium rounded-xl whitespace-nowrap transition-all ${active ? 'bg-[#355CFF] text-white shadow-xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'}`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{cat.name}</span>
              </button>
            );
          })}
        </div>

        <div className="relative min-w-[240px] px-1">
          <Search className="w-4 h-4 text-[#656B75] dark:text-[#A1A7B0] absolute left-3.5 top-2.5" />
          <input
            type="text"
            placeholder={isZh ? '搜索 MCP 服务或能力...' : 'Search MCP servers or tools...'}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-9 pr-3.5 py-2 text-xs rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-[#F7F7F5] dark:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] placeholder-[#656B75] dark:placeholder-[#A1A7B0] focus:outline-none focus:ring-1 focus:ring-[#355CFF] shadow-2xs"
          />
        </div>
      </div>

      {/* MCP 服务卡片列表 */}
      {filteredServers.length === 0 ? (
        <div className="p-16 text-center bg-white dark:bg-[#191B1F] rounded-2xl border border-dashed border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs">
          <Cpu className="w-10 h-10 text-slate-300 dark:text-slate-600 mx-auto mb-3" />
          <p className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">
            {isZh ? '未找到匹配的 MCP 服务' : 'No matching MCP servers found'}
          </p>
          <p className="ui-meta mt-1.5">
            {isZh ? '可在上方切换分类筛选，或点击右上角「接入新服务」进行自定义配置' : 'Switch category filters above or click "Add Server" to configure'}
          </p>
        </div>
      ) : (
        <McpServerCards {...{
          filteredServers, handleDeleteServer, handleOpenProbe, handleToggleServer, isZh, setAddServerModalOpen,
          setConfigModalOpen, setEditingServerId, setNewServer, setSelectedServerForConfig, togglingServer
        }} />
      )}

      {/* 客户端集成配置弹窗 */}
      {configModalOpen && (
        <McpConfigModal {...{
          activeConfigClient, configToken, copiedKey, getClaudeConfig, getClineConfig, getCurlSnippet,
          getCursorConfig, getPythonSnippet, handleCopyText, isZh, selectedServerForConfig, setActiveConfigClient,
          setConfigModalOpen, setConfigToken
        }} />
      )}

      {/* 注册自定义 MCP 服务模态框 */}
      {addServerModalOpen && (
        <McpRegistrationModal {...{
          editingServerId, handleCreateServer, isZh, newServer, savingServer, setAddServerModalOpen, setNewServer
        }} />
      )}

      {/* 在线 JSON-RPC 2.0 协议探针控制台 */}
      {probeModalOpen && (
        <McpProbeModal {...{
          TOOL_TEMPLATES, copiedKey, getCurlRequestText, getCurrentRpcPayload, getGroupLabel, getToolInfo,
          getWireRequestText, handleCopyText, handleRunProbe, handleSelectTool, isZh, lastSentPayload, messagesUrl,
          probeFailed, probeHttpStatus, probeLatency, probeMethod, probeResult, probeRunning, probeServer, probeToken,
          probeToolArgs, probeToolName, requestViewTab, servers, setProbeMethod, setProbeModalOpen, setProbeServer,
          setProbeToken, setProbeToolArgs, setRequestViewTab
        }} />
      )}
    </div>
  );
}
