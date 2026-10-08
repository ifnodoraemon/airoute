import React, { useState, useEffect } from 'react';
import {
  Cpu,
  Server,
  Globe,
  Terminal,
  Play,
  Check,
  Copy,
  Plus,
  Trash2,
  Search,
  X,
  Activity,
  RefreshCw,
  Layers,
  ExternalLink,
  ShieldCheck,
  Database,
  Code2,
  Sparkles,
  Send,
  Radio,
  Wrench,
  ArrowRight,
  BookOpen,
  AlertCircle,
  FileCode,
  Zap,
  CheckCircle2,
  Lock,
  Boxes
} from 'lucide-react';

export default function McpIntegrationView({ adminFetch, onCopy, showToast }) {
  const [servers, setServers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [togglingServer, setTogglingServer] = useState({});
  const [copiedKey, setCopiedKey] = useState('');

  // MCP Master Switch & Stats
  const [mcpSettings, setMcpSettings] = useState({
    mcp_enabled: true,
    enabled_skills_count: 6,
    active_tools_count: 9,
    servers_total: 8,
    servers_enabled: 7
  });
  const [togglingGlobal, setTogglingGlobal] = useState(false);

  // Modals
  const [configModalOpen, setConfigModalOpen] = useState(false);
  const [activeConfigClient, setActiveConfigClient] = useState('cursor');
  const [selectedServerForConfig, setSelectedServerForConfig] = useState(null);

  const [addServerModalOpen, setAddServerModalOpen] = useState(false);
  const [newServer, setNewServer] = useState({
    id: '',
    name: '',
    description: '',
    category: 'dev',
    transport: 'stdio',
    endpoint: '',
    author: 'Community',
    version: '1.0.0',
    tools: '',
    prompts: '',
    resources: '',
    env_vars: '',
    enabled: true
  });

  // Probe Modal
  const [probeModalOpen, setProbeModalOpen] = useState(false);
  const [probeServer, setProbeServer] = useState(null);
  const [probeMethod, setProbeMethod] = useState('server/discover');
  const [probeToolName, setProbeToolName] = useState('airoute_data_redact');
  const [probeToolArgs, setProbeToolArgs] = useState('{\n  "text": "客户张三 手机13812345678 身份证110101199003072345 密钥sk-abc123xyz789"\n}');
  const [probeRunning, setProbeRunning] = useState(false);
  const [probeResult, setProbeResult] = useState(null);
  const [probeLatency, setProbeLatency] = useState(null);

  const origin = typeof window !== 'undefined' && window.location ? window.location.origin : 'http://localhost:8080';
  const sseUrl = `${origin}/mcp/sse`;
  const messagesUrl = `${origin}/mcp/messages`;

  const fetchServersAndSettings = async () => {
    if (!adminFetch) return;
    setLoading(true);
    try {
      // 1. MCP Settings
      const setRes = await adminFetch('/api/v1/admin/mcp/settings');
      if (setRes.ok) {
        const setData = await setRes.json();
        if (setData.code === 0 && setData.data) {
          setMcpSettings(setData.data);
        }
      }

      // 2. MCP Servers list
      const srvRes = await adminFetch('/api/v1/admin/mcp/servers');
      if (srvRes.ok) {
        const srvData = await srvRes.json();
        if (srvData.code === 0 && srvData.data) {
          setServers(srvData.data);
        }
      }
    } catch (e) {
      console.warn('Failed to load MCP servers:', e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchServersAndSettings();
  }, []);

  const handleToggleGlobalMcp = async () => {
    if (!adminFetch || togglingGlobal) return;
    setTogglingGlobal(true);
    const nextState = !mcpSettings.mcp_enabled;
    try {
      const res = await adminFetch('/api/v1/admin/mcp/toggle', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mcp_enabled: nextState })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setMcpSettings(prev => ({ ...prev, mcp_enabled: nextState }));
        if (showToast) showToast(`MCP 协议网关服务已${nextState ? '启用' : '关闭'}`, 'success');
      } else {
        if (showToast) showToast(data.error || '切换失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    } finally {
      setTogglingGlobal(false);
    }
  };

  const handleToggleServer = async (serverId, currentEnabled) => {
    if (!adminFetch) return;
    setTogglingServer(prev => ({ ...prev, [serverId]: true }));
    try {
      const res = await adminFetch(`/api/v1/admin/mcp/servers/${serverId}/toggle`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !currentEnabled })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setServers(prev => prev.map(s => s.id === serverId ? { ...s, enabled: !currentEnabled } : s));
        if (showToast) showToast(`MCP 服务 [${serverId}] 已${!currentEnabled ? '开启' : '关闭'}`, 'success');
        fetchServersAndSettings();
      } else {
        if (showToast) showToast(data.error || '切换服务状态失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    } finally {
      setTogglingServer(prev => ({ ...prev, [serverId]: false }));
    }
  };

  const handleDeleteServer = async (serverId) => {
    if (!adminFetch) return;
    if (!window.confirm(`确定要移除自定义 MCP 服务 [${serverId}] 吗？`)) return;
    try {
      const res = await adminFetch(`/api/v1/admin/mcp/servers/${serverId}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setServers(prev => prev.filter(s => s.id !== serverId));
        if (showToast) showToast(`MCP 服务 [${serverId}] 已成功移除`, 'success');
        fetchServersAndSettings();
      } else {
        if (showToast) showToast(data.error || '删除失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    }
  };

  const handleCreateServer = async (e) => {
    e.preventDefault();
    if (!adminFetch) return;
    if (!newServer.name || !newServer.endpoint) {
      if (showToast) showToast('请填写服务名称与 Endpoint', 'error');
      return;
    }

    const payload = {
      ...newServer,
      tools: newServer.tools ? newServer.tools.split(/[,，\n]/).map(t => t.trim()).filter(Boolean) : [],
      prompts: newServer.prompts ? newServer.prompts.split(/[,，\n]/).map(p => p.trim()).filter(Boolean) : [],
      resources: newServer.resources ? newServer.resources.split(/[,，\n]/).map(r => r.trim()).filter(Boolean) : []
    };

    try {
      const res = await adminFetch('/api/v1/admin/mcp/servers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast('新 MCP 服务已成功注册到广场', 'success');
        setAddServerModalOpen(false);
        setNewServer({
          id: '',
          name: '',
          description: '',
          category: 'dev',
          transport: 'stdio',
          endpoint: '',
          author: 'Community',
          version: '1.0.0',
          tools: '',
          prompts: '',
          resources: '',
          env_vars: '',
          enabled: true
        });
        fetchServersAndSettings();
      } else {
        if (showToast) showToast(data.error || '创建 MCP 服务失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    }
  };

  const handleOpenProbe = (server) => {
    setProbeServer(server);
    setProbeResult(null);
    setProbeLatency(null);
    if (server && server.tools && server.tools.length > 0) {
      setProbeToolName(server.tools[0]);
    } else {
      setProbeToolName('airoute_cluster_status');
    }
    setProbeModalOpen(true);
  };

  const handleRunProbe = async () => {
    setProbeRunning(true);
    setProbeResult(null);
    const start = performance.now();

    try {
      let rpcReq = {};
      if (probeMethod === 'server/discover') {
        rpcReq = {
          jsonrpc: "2.0",
          id: "probe-discover-" + Date.now(),
          method: "server/discover",
          params: {}
        };
      } else if (probeMethod === 'tools/list') {
        rpcReq = {
          jsonrpc: "2.0",
          id: "probe-list-" + Date.now(),
          method: "tools/list",
          params: {}
        };
      } else {
        let parsedArgs = {};
        try {
          parsedArgs = JSON.parse(probeToolArgs);
        } catch (e) {
          if (showToast) showToast('工具参数 JSON 语法错误: ' + e.message, 'error');
          setProbeRunning(false);
          return;
        }
        rpcReq = {
          jsonrpc: "2.0",
          id: "probe-call-" + Date.now(),
          method: "tools/call",
          params: {
            name: probeToolName,
            arguments: parsedArgs
          }
        };
      }

      const res = await fetch(messagesUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(rpcReq)
      });
      const data = await res.json();
      const duration = Math.round(performance.now() - start);
      setProbeLatency(duration);
      setProbeResult(data);
    } catch (err) {
      const duration = Math.round(performance.now() - start);
      setProbeLatency(duration);
      setProbeResult({
        jsonrpc: "2.0",
        id: "error",
        error: { code: -32603, message: "探针请求失败: " + err.message }
      });
    } finally {
      setProbeRunning(false);
    }
  };

  const handleCopyText = (text, key) => {
    if (onCopy) {
      onCopy(text);
    } else {
      navigator.clipboard.writeText(text);
    }
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(''), 2000);
    if (showToast) showToast('配置已复制到剪贴板', 'success');
  };

  const categories = [
    { id: 'all', name: '全部服务', icon: Boxes },
    { id: 'ops', name: '网关原生', icon: Server },
    { id: 'dev', name: '研发协作', icon: Code2 },
    { id: 'search', name: '搜索抓取', icon: Globe },
    { id: 'db', name: '数据存储', icon: Database },
    { id: 'ai', name: '深度推理', icon: Sparkles },
    { id: 'productivity', name: '企业协同', icon: Layers }
  ];

  const filteredServers = servers.filter(srv => {
    const matchCategory = selectedCategory === 'all' || srv.category === selectedCategory;
    const matchSearch = !searchQuery ||
      srv.name?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      srv.id?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      srv.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (srv.tools && srv.tools.some(t => t.toLowerCase().includes(searchQuery.toLowerCase())));
    return matchCategory && matchSearch;
  });

  const totalToolsCount = servers.reduce((acc, s) => acc + (s.tools ? s.tools.length : 0), 0);
  const onlineServersCount = servers.filter(s => s.enabled).length;

  // Code Generation for Clients
  const getCursorConfig = () => {
    return JSON.stringify({
      mcpServers: {
        "airoute-gateway": {
          url: sseUrl,
          headers: {
            "Authorization": "Bearer YOUR_AIRUTE_KEY"
          }
        },
        ...(selectedServerForConfig && selectedServerForConfig.id !== 'airoute-gateway' ? {
          [selectedServerForConfig.id]: {
            command: selectedServerForConfig.transport === 'stdio' ? "npx" : undefined,
            args: selectedServerForConfig.transport === 'stdio' ? selectedServerForConfig.endpoint.replace('npx -y ', '').split(' ') : undefined,
            url: selectedServerForConfig.transport === 'sse' ? selectedServerForConfig.endpoint : undefined
          }
        } : {})
      }
    }, null, 2);
  };

  const getClaudeConfig = () => {
    return JSON.stringify({
      mcpServers: {
        "airoute-gateway": {
          command: "curl",
          args: ["-N", sseUrl]
        },
        ...(selectedServerForConfig && selectedServerForConfig.id !== 'airoute-gateway' ? {
          [selectedServerForConfig.id]: {
            command: selectedServerForConfig.transport === 'stdio' ? "npx" : "curl",
            args: selectedServerForConfig.transport === 'stdio' ? ["-y", selectedServerForConfig.endpoint.replace('npx -y ', '')] : ["-N", selectedServerForConfig.endpoint]
          }
        } : {})
      }
    }, null, 2);
  };

  const getClineConfig = () => {
    return JSON.stringify({
      mcpServers: {
        "airoute-gateway": {
          type: "sse",
          url: sseUrl,
          autoApprove: [
            "airoute_cluster_status",
            "airoute_data_redact",
            "airoute_sql_security_check"
          ]
        }
      }
    }, null, 2);
  };

  const getPythonSnippet = () => {
    return `from mcp import ClientSession, StdioServerParameters
from mcp.client.sse import sse_client

async def run_mcp_client():
    # 连接 Airoute 统一 MCP 代理网关
    async with sse_client("${sseUrl}") as (read_stream, write_stream):
        async with ClientSession(read_stream, write_stream) as session:
            await session.initialize()
            
            # 列出网关聚合的所有 MCP 工具
            tools = await session.list_tools()
            print(f"发现 {len(tools.tools)} 个可用安全工具")
            
            # 调用数据安全脱敏工具示例
            result = await session.call_tool(
                "airoute_data_redact",
                arguments={"text": "机密手机号: 13800138000"}
            )
            print("脱敏结果:", result.content[0].text)

if __name__ == "__main__":
    import asyncio
    asyncio.run(run_mcp_client())`;
  };

  return (
    <div className="space-y-6">
      {/* 顶部标题与说明 */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-2 border-b border-gray-100 dark:border-gray-800">
        <div>
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-xl text-white shadow-md shadow-indigo-500/20">
              <Cpu className="w-6 h-6" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-xl font-bold text-gray-900 dark:text-white">
                  Model Context Protocol (MCP) 广场
                </h1>
                <span className="px-2.5 py-0.5 text-xs font-semibold rounded-full bg-indigo-50 dark:bg-indigo-900/40 text-indigo-600 dark:text-indigo-400 border border-indigo-200 dark:border-indigo-800/50">
                  Anthropic Spec 2024-11-05
                </span>
                <span className="px-2 py-0.5 text-[11px] font-medium rounded-full bg-emerald-50 dark:bg-emerald-900/30 text-emerald-600 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800/40">
                  ModelScope 风格
                </span>
              </div>
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                外部协议连接服务中心：基于标准 JSON-RPC 2.0 (SSE / Stdio) 连接外部系统，提供统一鉴权、安全脱敏与客户端无缝对接。
              </p>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2.5 flex-wrap">
          <button
            onClick={() => handleOpenProbe(null)}
            className="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-indigo-700 dark:text-indigo-300 bg-indigo-50 dark:bg-indigo-950/60 border border-indigo-200 dark:border-indigo-800 rounded-lg hover:bg-indigo-100 dark:hover:bg-indigo-900/50 transition-colors shadow-sm"
          >
            <Radio className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
            在线协议探针
          </button>
          <button
            onClick={() => { setSelectedServerForConfig(null); setConfigModalOpen(true); }}
            className="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-750 transition-colors shadow-sm"
          >
            <FileCode className="w-4 h-4 text-purple-600 dark:text-purple-400" />
            客户端连接配置
          </button>
          <button
            onClick={() => setAddServerModalOpen(true)}
            className="inline-flex items-center gap-1.5 px-3.5 py-2 text-xs font-medium text-white bg-gradient-to-r from-indigo-600 to-purple-600 rounded-lg hover:from-indigo-700 hover:to-purple-700 transition-all shadow-md shadow-indigo-500/20"
          >
            <Plus className="w-4 h-4" />
            接入新服务
          </button>
          <button
            onClick={fetchServersAndSettings}
            disabled={loading}
            className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 hover:bg-gray-50 transition-colors"
            title="刷新服务状态"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* 核心指标 & 全局管控卡片 */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* 全局主控开关 */}
        <div className="p-4 rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-sm flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className={`p-2.5 rounded-lg ${mcpSettings.mcp_enabled ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400' : 'bg-rose-50 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400'}`}>
              <ShieldCheck className="w-5 h-5" />
            </div>
            <div>
              <div className="text-xs text-gray-500 dark:text-gray-400 font-medium">网关 MCP 协议总闸</div>
              <div className="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2 mt-0.5">
                {mcpSettings.mcp_enabled ? '运行就绪 (Active)' : '已停用 (Inactive)'}
              </div>
            </div>
          </div>
          <button
            onClick={handleToggleGlobalMcp}
            disabled={togglingGlobal}
            className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${mcpSettings.mcp_enabled ? 'bg-emerald-600' : 'bg-gray-300 dark:bg-gray-600'}`}
          >
            <span
              className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${mcpSettings.mcp_enabled ? 'translate-x-5' : 'translate-x-0'}`}
            />
          </button>
        </div>

        {/* 活跃 MCP 服务数 */}
        <div className="p-4 rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-sm flex items-center gap-3">
          <div className="p-2.5 bg-indigo-50 dark:bg-indigo-950/40 text-indigo-600 dark:text-indigo-400 rounded-lg">
            <Server className="w-5 h-5" />
          </div>
          <div>
            <div className="text-xs text-gray-500 dark:text-gray-400 font-medium">活跃 MCP 服务</div>
            <div className="text-lg font-bold text-gray-900 dark:text-white flex items-baseline gap-1 mt-0.5">
              <span>{onlineServersCount}</span>
              <span className="text-xs font-normal text-gray-500 dark:text-gray-400">/ {servers.length} 已启用</span>
            </div>
          </div>
        </div>

        {/* 聚合暴露 Tools 总数 */}
        <div className="p-4 rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-sm flex items-center gap-3">
          <div className="p-2.5 bg-purple-50 dark:bg-purple-950/40 text-purple-600 dark:text-purple-400 rounded-lg">
            <Wrench className="w-5 h-5" />
          </div>
          <div>
            <div className="text-xs text-gray-500 dark:text-gray-400 font-medium">聚合协议工具 (Tools)</div>
            <div className="text-lg font-bold text-gray-900 dark:text-white flex items-baseline gap-1 mt-0.5">
              <span>{totalToolsCount}</span>
              <span className="text-xs font-normal text-gray-500 dark:text-gray-400">个生产级工具</span>
            </div>
          </div>
        </div>

        {/* 协议传输端点 */}
        <div className="p-4 rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-sm flex items-center gap-3">
          <div className="p-2.5 bg-blue-50 dark:bg-blue-950/40 text-blue-600 dark:text-blue-400 rounded-lg">
            <Zap className="w-5 h-5" />
          </div>
          <div className="overflow-hidden">
            <div className="text-xs text-gray-500 dark:text-gray-400 font-medium">网关 SSE 端点</div>
            <div className="text-xs font-mono font-semibold text-gray-800 dark:text-gray-200 truncate mt-0.5" title={sseUrl}>
              /mcp/sse
            </div>
          </div>
        </div>
      </div>

      {/* 搜索与分类导航 */}
      <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3 bg-white dark:bg-gray-800/80 p-3 rounded-xl border border-gray-200 dark:border-gray-800 shadow-sm">
        {/* 分类 Tabs */}
        <div className="flex items-center gap-1 overflow-x-auto pb-1 md:pb-0 scrollbar-none">
          {categories.map(cat => {
            const Icon = cat.icon;
            const active = selectedCategory === cat.id;
            return (
              <button
                key={cat.id}
                onClick={() => setSelectedCategory(cat.id)}
                className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-all ${active ? 'bg-indigo-600 text-white shadow-sm' : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700/60'}`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{cat.name}</span>
              </button>
            );
          })}
        </div>

        {/* 搜索输入框 */}
        <div className="relative min-w-[240px]">
          <Search className="w-4 h-4 text-gray-400 absolute left-3 top-2.5" />
          <input
            type="text"
            placeholder="搜索 MCP 服务、工具名或描述..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-9 pr-3 py-1.5 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/60 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-indigo-500"
          />
        </div>
      </div>

      {/* MCP 服务卡片列表 */}
      {filteredServers.length === 0 ? (
        <div className="p-12 text-center bg-white dark:bg-gray-800/50 rounded-xl border border-dashed border-gray-200 dark:border-gray-800">
          <Cpu className="w-10 h-10 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300">未找到匹配的 MCP 服务</p>
          <p className="text-xs text-gray-400 mt-1">可更换分类筛选或点击右上角「接入新服务」进行自定义添加</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
          {filteredServers.map(server => {
            const isToggling = !!togglingServer[server.id];
            const isCustom = server.id.startsWith('custom-');

            return (
              <div
                key={server.id}
                className={`flex flex-col justify-between p-5 rounded-xl border transition-all duration-200 ${server.enabled ? 'border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-sm hover:shadow-md' : 'border-gray-200/60 dark:border-gray-800/60 bg-gray-50/70 dark:bg-gray-850/50 opacity-80'}`}
              >
                <div>
                  {/* 卡片头部 */}
                  <div className="flex items-start justify-between gap-3 mb-3">
                    <div className="flex items-center gap-3">
                      <div className={`p-2 rounded-lg ${server.id === 'airoute-gateway' ? 'bg-indigo-100 dark:bg-indigo-950/70 text-indigo-600 dark:text-indigo-400' : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300'}`}>
                        {server.category === 'ops' ? <Server className="w-5 h-5" /> :
                         server.category === 'dev' ? <Code2 className="w-5 h-5" /> :
                         server.category === 'search' ? <Globe className="w-5 h-5" /> :
                         server.category === 'db' ? <Database className="w-5 h-5" /> :
                         server.category === 'ai' ? <Sparkles className="w-5 h-5" /> :
                         <Layers className="w-5 h-5" />}
                      </div>
                      <div>
                        <h3 className="text-sm font-bold text-gray-900 dark:text-white flex items-center gap-1.5">
                          {server.name}
                        </h3>
                        <div className="text-[11px] font-mono text-gray-400 dark:text-gray-500">
                          {server.id}
                        </div>
                      </div>
                    </div>

                    {/* 开关 */}
                    <button
                      onClick={() => handleToggleServer(server.id, server.enabled)}
                      disabled={isToggling}
                      className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${server.enabled ? 'bg-indigo-600' : 'bg-gray-300 dark:bg-gray-600'}`}
                      title={server.enabled ? '点击停用' : '点击启用'}
                    >
                      <span
                        className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${server.enabled ? 'translate-x-4' : 'translate-x-0'}`}
                      />
                    </button>
                  </div>

                  {/* 标签栏 */}
                  <div className="flex items-center gap-1.5 flex-wrap mb-3">
                    <span className={`px-2 py-0.5 text-[10px] font-semibold rounded ${server.transport === 'sse' ? 'bg-blue-50 dark:bg-blue-900/40 text-blue-600 dark:text-blue-300 border border-blue-200/60 dark:border-blue-800/40' : 'bg-purple-50 dark:bg-purple-900/40 text-purple-600 dark:text-purple-300 border border-purple-200/60 dark:border-purple-800/40'}`}>
                      {server.transport.toUpperCase()}
                    </span>
                    <span className="px-2 py-0.5 text-[10px] rounded bg-gray-100 dark:bg-gray-700/60 text-gray-600 dark:text-gray-300">
                      v{server.version || '1.0.0'}
                    </span>
                    <span className="px-2 py-0.5 text-[10px] rounded bg-gray-100 dark:bg-gray-700/60 text-gray-500 dark:text-gray-400">
                      {server.author || 'Anthropic'}
                    </span>
                    {server.status === 'online' && (
                      <span className="flex items-center gap-1 px-1.5 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
                        <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                        Online
                      </span>
                    )}
                  </div>

                  {/* 描述文案 */}
                  <p className="text-xs text-gray-600 dark:text-gray-400 leading-relaxed line-clamp-2 mb-4">
                    {server.description}
                  </p>

                  {/* 暴露的 Tools 标签组 */}
                  <div className="mb-4">
                    <div className="flex items-center justify-between text-[11px] font-medium text-gray-500 dark:text-gray-400 mb-1.5">
                      <span>包含工具 ({server.tools ? server.tools.length : 0})</span>
                      <span className="text-[10px] text-gray-400 font-mono">JSON-RPC 2.0</span>
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {(server.tools || []).slice(0, 4).map(t => (
                        <span
                          key={t}
                          className="px-2 py-0.5 text-[10px] font-mono bg-gray-100 dark:bg-gray-700/50 text-gray-700 dark:text-gray-300 rounded border border-gray-200/60 dark:border-gray-700/50"
                        >
                          {t}
                        </span>
                      ))}
                      {(server.tools || []).length > 4 && (
                        <span className="px-1.5 py-0.5 text-[10px] font-medium text-gray-400 dark:text-gray-500">
                          +{server.tools.length - 4} 更多
                        </span>
                      )}
                    </div>
                  </div>
                </div>

                {/* 卡片底部操作栏 */}
                <div className="pt-3 border-t border-gray-100 dark:border-gray-800/80 flex items-center justify-between gap-2">
                  <div className="flex items-center gap-1.5">
                    <button
                      onClick={() => handleOpenProbe(server)}
                      className="px-2.5 py-1 text-xs font-medium text-indigo-600 dark:text-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-950/60 rounded-md transition-colors flex items-center gap-1"
                    >
                      <Radio className="w-3.5 h-3.5" />
                      探针测试
                    </button>
                    <button
                      onClick={() => { setSelectedServerForConfig(server); setConfigModalOpen(true); }}
                      className="px-2.5 py-1 text-xs font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-md transition-colors flex items-center gap-1"
                    >
                      <Terminal className="w-3.5 h-3.5" />
                      连接配置
                    </button>
                  </div>

                  {isCustom && (
                    <button
                      onClick={() => handleDeleteServer(server.id)}
                      className="p-1 text-gray-400 hover:text-rose-500 rounded transition-colors"
                      title="移除自定义 MCP 服务"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* 客户端集成配置弹窗 */}
      {configModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-2xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-indigo-100 dark:bg-indigo-900/50 text-indigo-600 dark:text-indigo-400 rounded-lg">
                  <FileCode className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    客户端连接配置指南
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {selectedServerForConfig ? `针对 [${selectedServerForConfig.name}] 的专属配置` : '将 Airoute 统一 MCP 网关导入您的 AI 客户端'}
                  </p>
                </div>
              </div>
              <button
                onClick={() => setConfigModalOpen(false)}
                className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* 客户端选择 Tabs */}
            <div className="flex items-center gap-2 border-b border-gray-200 dark:border-gray-800 pb-2">
              {[
                { id: 'cursor', label: 'Cursor (.cursor/mcp.json)' },
                { id: 'claude', label: 'Claude Desktop' },
                { id: 'cline', label: 'Cline / Roo-Code' },
                { id: 'python', label: 'Python SDK' }
              ].map(c => (
                <button
                  key={c.id}
                  onClick={() => setActiveConfigClient(c.id)}
                  className={`px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${activeConfigClient === c.id ? 'bg-indigo-600 text-white' : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'}`}
                >
                  {c.label}
                </button>
              ))}
            </div>

            {/* 配置代码预览与一键复制 */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-gray-700 dark:text-gray-300">
                  {activeConfigClient === 'cursor' && '配置文件位置: 项目根目录 .cursor/mcp.json'}
                  {activeConfigClient === 'claude' && '配置文件位置: ~/Library/Application Support/Claude/claude_desktop_config.json'}
                  {activeConfigClient === 'cline' && '配置文件位置: Cline Settings -> MCP Servers'}
                  {activeConfigClient === 'python' && '安装依赖: pip install mcp httpx'}
                </span>
                <button
                  onClick={() => {
                    const code = activeConfigClient === 'cursor' ? getCursorConfig() :
                                 activeConfigClient === 'claude' ? getClaudeConfig() :
                                 activeConfigClient === 'cline' ? getClineConfig() : getPythonSnippet();
                    handleCopyText(code, 'client-config');
                  }}
                  className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors"
                >
                  {copiedKey === 'client-config' ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                  <span>{copiedKey === 'client-config' ? '已复制' : '复制配置'}</span>
                </button>
              </div>

              <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto border border-gray-800">
                <pre>
                  {activeConfigClient === 'cursor' && getCursorConfig()}
                  {activeConfigClient === 'claude' && getClaudeConfig()}
                  {activeConfigClient === 'cline' && getClineConfig()}
                  {activeConfigClient === 'python' && getPythonSnippet()}
                </pre>
              </div>
            </div>

            <div className="p-3 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/50 rounded-lg text-xs text-amber-800 dark:text-amber-300 flex items-start gap-2">
              <AlertCircle className="w-4 h-4 mt-0.5 flex-shrink-0" />
              <div>
                <strong>安全最佳实践：</strong>所有客户端均通过 Airoute 统一协议代理连接，无需直接暴露内部数据库或私有服务凭据。网关会自动执行参数脱敏与 SQL 注入审计。
              </div>
            </div>

            <div className="flex justify-end pt-2">
              <button
                onClick={() => setConfigModalOpen(false)}
                className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
              >
                关闭
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 注册自定义 MCP 服务模态框 */}
      {addServerModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-gradient-to-br from-indigo-500 to-purple-600 text-white rounded-lg">
                  <Plus className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    接入新 MCP 服务
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    遵循 ModelScope / Anthropic 规范标准接入自定义协议服务
                  </p>
                </div>
              </div>
              <button
                onClick={() => setAddServerModalOpen(false)}
                className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleCreateServer} className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    服务唯一标识 (ID) *
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="如: docker-mcp"
                    value={newServer.id}
                    onChange={(e) => setNewServer({ ...newServer, id: e.target.value })}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    服务显示名称 *
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="如: Docker 容器管理服务"
                    value={newServer.name}
                    onChange={(e) => setNewServer({ ...newServer, name: e.target.value })}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    分类领域
                  </label>
                  <select
                    value={newServer.category}
                    onChange={(e) => setNewServer({ ...newServer, category: e.target.value })}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  >
                    <option value="ops">运维网关 (ops)</option>
                    <option value="dev">研发协作 (dev)</option>
                    <option value="search">搜索抓取 (search)</option>
                    <option value="db">数据存储 (db)</option>
                    <option value="ai">深度推理 (ai)</option>
                    <option value="productivity">企业协同 (productivity)</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    传输通道 (Transport)
                  </label>
                  <select
                    value={newServer.transport}
                    onChange={(e) => setNewServer({ ...newServer, transport: e.target.value })}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  >
                    <option value="stdio">stdio (本地命令/子进程)</option>
                    <option value="sse">sse (远程 HTTP Server-Sent Events)</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  服务 Endpoint / 执行命令 *
                </label>
                <input
                  type="text"
                  required
                  placeholder="如: npx -y @modelcontextprotocol/server-docker 或 https://mcp.example.com/sse"
                  value={newServer.endpoint}
                  onChange={(e) => setNewServer({ ...newServer, endpoint: e.target.value })}
                  className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  服务描述
                </label>
                <textarea
                  rows="2"
                  placeholder="简述该 MCP 服务的功能、适用场景及接入说明..."
                  value={newServer.description}
                  onChange={(e) => setNewServer({ ...newServer, description: e.target.value })}
                  className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                  暴露的 Tools 工具集 (逗号分隔)
                </label>
                <input
                  type="text"
                  placeholder="如: docker_ps, docker_logs, docker_restart"
                  value={newServer.tools}
                  onChange={(e) => setNewServer({ ...newServer, tools: e.target.value })}
                  className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white font-mono"
                />
              </div>

              <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-gray-200 dark:border-gray-800">
                <button
                  type="button"
                  onClick={() => setAddServerModalOpen(false)}
                  className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200"
                >
                  取消
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 text-xs font-medium text-white bg-indigo-600 rounded-lg hover:bg-indigo-700 shadow-sm"
                >
                  确认接入
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 在线 JSON-RPC 2.0 协议探针控制台 */}
      {probeModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-3xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-gradient-to-br from-indigo-500 to-purple-600 text-white rounded-lg">
                  <Radio className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
                    在线协议探针控制台
                    <span className="text-xs font-mono font-normal text-indigo-500">JSON-RPC 2.0</span>
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    向网关端点 {messagesUrl} 发送标准协议报文并实时验证安全拦截与工具输出
                  </p>
                </div>
              </div>
              <button
                onClick={() => setProbeModalOpen(false)}
                className="p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded-lg"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* 方法与参数选择 */}
            <div className="space-y-3">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    探针方法 (JSON-RPC Method)
                  </label>
                  <select
                    value={probeMethod}
                    onChange={(e) => setProbeMethod(e.target.value)}
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  >
                    <option value="server/discover">server/discover (服务与能力探测)</option>
                    <option value="tools/list">tools/list (枚举工具列表与 Schema)</option>
                    <option value="tools/call">tools/call (在线执行具体工具)</option>
                  </select>
                </div>

                {probeMethod === 'tools/call' && (
                  <div>
                    <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                      选择目标工具 (Tool Name)
                    </label>
                    <select
                      value={probeToolName}
                      onChange={(e) => {
                        const name = e.target.value;
                        setProbeToolName(name);
                        if (name === 'airoute_data_redact') {
                          setProbeToolArgs('{\n  "text": "用户张三 身份证110101199003072345 手机13812345678 密钥sk-abcdef123456"\n}');
                        } else if (name === 'airoute_sql_security_check') {
                          setProbeToolArgs('{\n  "query": "DROP TABLE users; -- 注入攻击"\n}');
                        } else if (name === 'airoute_deep_search') {
                          setProbeToolArgs('{\n  "query": "DeepSeek R1 模型推理架构与性能"\n}');
                        } else if (name === 'airoute_recommend_model') {
                          setProbeToolArgs('{\n  "task_type": "coding",\n  "max_budget_per_m": 1.0\n}');
                        } else {
                          setProbeToolArgs('{}');
                        }
                      }}
                      className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white font-mono"
                    >
                      <option value="airoute_data_redact">airoute_data_redact (敏感数据脱敏)</option>
                      <option value="airoute_sql_security_check">airoute_sql_security_check (SQL安全审计拦截)</option>
                      <option value="airoute_cluster_status">airoute_cluster_status (集群高可用状态)</option>
                      <option value="airoute_model_topology">airoute_model_topology (模型拓扑与路由矩阵)</option>
                      <option value="airoute_deep_search">airoute_deep_search (高质量结构化联网深度检索)</option>
                      <option value="airoute_recommend_model">airoute_recommend_model (智能模型选型仲裁)</option>
                    </select>
                  </div>
                )}
              </div>

              {probeMethod === 'tools/call' && (
                <div>
                  <div className="flex items-center justify-between mb-1">
                    <label className="text-xs font-medium text-gray-700 dark:text-gray-300">
                      工具输入参数 (JSON Arguments)
                    </label>
                    <div className="flex items-center gap-1.5">
                      <button
                        type="button"
                        onClick={() => {
                          setProbeToolName('airoute_data_redact');
                          setProbeToolArgs('{\n  "text": "用户张三 身份证110101199003072345 手机13812345678 密钥sk-abcdef123456"\n}');
                        }}
                        className="text-[10px] text-indigo-600 hover:underline"
                      >
                        敏感脱敏示例
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => {
                          setProbeToolName('airoute_sql_security_check');
                          setProbeToolArgs('{\n  "query": "DROP TABLE users; -- 注入攻击"\n}');
                        }}
                        className="text-[10px] text-rose-600 hover:underline"
                      >
                        危险 SQL 拦截示例
                      </button>
                    </div>
                  </div>
                  <textarea
                    rows="3"
                    value={probeToolArgs}
                    onChange={(e) => setProbeToolArgs(e.target.value)}
                    className="w-full px-3 py-2 font-mono text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-900 text-gray-100"
                  />
                </div>
              )}

              <div className="flex items-center justify-between pt-1">
                <span className="text-xs text-gray-400">
                  {probeLatency !== null && (
                    <span className="text-emerald-500 font-mono">
                      ✓ 往返耗时: {probeLatency} ms
                    </span>
                  )}
                </span>
                <button
                  onClick={handleRunProbe}
                  disabled={probeRunning}
                  className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-white bg-indigo-600 rounded-lg hover:bg-indigo-700 disabled:opacity-50 transition-colors shadow-sm"
                >
                  <Send className={`w-3.5 h-3.5 ${probeRunning ? 'animate-pulse' : ''}`} />
                  <span>{probeRunning ? '探针执行中...' : '发送 JSON-RPC 2.0 报文'}</span>
                </button>
              </div>

              {/* 探针响应输出 */}
              {probeResult && (
                <div>
                  <div className="flex items-center justify-between text-xs font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                    <span className="flex items-center gap-1.5">
                      <CheckCircle2 className="w-4 h-4 text-emerald-500" />
                      服务器响应 (Response Payload)
                    </span>
                    <button
                      onClick={() => handleCopyText(JSON.stringify(probeResult, null, 2), 'probe-res')}
                      className="inline-flex items-center gap-1 text-[11px] text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
                    >
                      {copiedKey === 'probe-res' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                      复制响应
                    </button>
                  </div>
                  <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto max-h-60 border border-gray-800">
                    <pre>{JSON.stringify(probeResult, null, 2)}</pre>
                  </div>
                </div>
              )}
            </div>

            <div className="flex justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
              <button
                onClick={() => setProbeModalOpen(false)}
                className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
              >
                关闭
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
