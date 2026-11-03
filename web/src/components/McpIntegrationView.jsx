import { useState, useEffect, useCallback } from 'react';
import {
  Cpu,
  Server,
  Globe,
  Terminal,
  Check,
  Copy,
  Plus,
  Trash2,
  Pencil,
  Search,
  X,
  RefreshCw,
  Layers,
  Database,
  Code2,
  Sparkles,
  Send,
  Radio,
  BookOpen,
  AlertCircle,
  FileCode,
  CheckCircle2,
  Lock,
  Boxes
} from 'lucide-react';
import { resolveGatewayUrl, copyToClipboard } from '../config';
import useModalA11y from '../hooks/useModalA11y';
import { buildMCPClientConfig } from '../lib/mcpClientConfig';

const TOOL_TEMPLATES = {
  // 网关原生安全与审计
  airoute_data_redact: {
    label: 'airoute_data_redact (敏感数据脱敏与隐私合规)',
    group: '网关原生安全审计',
    defaultArgs: '{\n  "text": "客户张三 手机13812345678 身份证110101199003072345 密钥sk-abcdef123456"\n}'
  },
  airoute_sql_security_check: {
    label: 'airoute_sql_security_check (SQL高危注入拦截与安全审计)',
    group: '网关原生安全审计',
    defaultArgs: '{\n  "query": "DROP TABLE users; -- 注入攻击"\n}'
  },
  airoute_cluster_status: {
    label: 'airoute_cluster_status (集群高可用健康探活与指标)',
    group: '网关原生运维',
    defaultArgs: '{\n}'
  },
  airoute_model_topology: {
    label: 'airoute_model_topology (模型路由矩阵与拓扑视图)',
    group: '网关原生运维',
    defaultArgs: '{\n  "modality": "chat"\n}'
  },
  airoute_deep_search: {
    label: 'airoute_deep_search (高质量结构化联网深度检索)',
    group: '网关原生增强',
    defaultArgs: '{\n  "query": "DeepSeek R1 模型推理架构与性能",\n  "max_results": 5\n}'
  },
  airoute_recommend_model: {
    label: 'airoute_recommend_model (智能模型选型与吞吐仲裁)',
    group: '网关原生增强',
    defaultArgs: '{\n  "task_description": "企业级高并发智能选型与路由仲裁",\n  "priority": "quality"\n}'
  },
  airoute_query_logs: {
    label: 'airoute_query_logs (统一审计日志快速检索)',
    group: '网关原生运维',
    defaultArgs: '{\n  "limit": 10\n}'
  },

  // Puppeteer 无头浏览器
  puppeteer_navigate: {
    label: 'puppeteer_navigate (无头浏览器页面导航与加载)',
    group: 'Puppeteer 浏览器渲染',
    defaultArgs: '{\n  "url": "https://news.ycombinator.com"\n}'
  },
  puppeteer_screenshot: {
    label: 'puppeteer_screenshot (页面视口全景截图捕获)',
    group: 'Puppeteer 浏览器渲染',
    defaultArgs: '{\n  "name": "homepage_preview",\n  "full_page": false\n}'
  },
  puppeteer_click: {
    label: 'puppeteer_click (DOM 节点模拟点击与交互)',
    group: 'Puppeteer 浏览器渲染',
    defaultArgs: '{\n  "selector": "button.submit"\n}'
  },
  puppeteer_evaluate: {
    label: 'puppeteer_evaluate (沙箱安全 JavaScript 表达式求值)',
    group: 'Puppeteer 浏览器渲染',
    defaultArgs: '{\n  "script": "document.title"\n}'
  },

  // 关系型与嵌入式数据库
  read_query: {
    label: 'read_query (Postgres 只读安全 SQL 查询)',
    group: '数据库与持久存储',
    defaultArgs: '{\n  "query": "SELECT id, name, status, latency_ms FROM public.channels LIMIT 5;"\n}'
  },
  list_tables: {
    label: 'list_tables (枚举数据表元数据清单)',
    group: '数据库与持久存储',
    defaultArgs: '{\n}'
  },
  describe_table: {
    label: 'describe_table (获取数据表字段与约束模式)',
    group: '数据库与持久存储',
    defaultArgs: '{\n  "table_name": "channels"\n}'
  },
  query: {
    label: 'query (通用数据库查询执行)',
    group: '数据库与持久存储',
    defaultArgs: '{\n  "query": "SELECT 1 as test_conn;"\n}'
  },

  // 深度推理
  sequentialthinking: {
    label: 'sequentialthinking (思维链深层分步逻辑推理)',
    group: '深度反思与推理',
    defaultArgs: '{\n  "thought": "分析分布式网关集群在流量洪峰下的限流与缓存降级拓扑",\n  "thoughtNumber": 1,\n  "totalThoughts": 3\n}'
  },

  // GitHub / Git 研发协作
  search_repositories: {
    label: 'search_repositories (GitHub 仓库检索)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n  "query": "airoute language:Go"\n}'
  },
  create_issue: {
    label: 'create_issue (创建规范化协同工单)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n  "title": "网关 MCP 协议探针联调测试",\n  "body": "企业级统一接入网关协议探测已顺利通过验证"\n}'
  },
  get_file_contents: {
    label: 'get_file_contents (获取仓库代码文件详情)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n  "owner": "airoute",\n  "repo": "gateway",\n  "path": "README.md"\n}'
  },
  create_pull_request: {
    label: 'create_pull_request (创建代码拉取审查请求)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n  "title": "feat: mcp probe auth token",\n  "base": "main",\n  "head": "feature/mcp"\n}'
  },
  git_status: {
    label: 'git_status (查询本地 Git 仓库工作树状态)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n}'
  },
  git_diff: {
    label: 'git_diff (查看变更差异对比)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n}'
  },

  // DevOps & 容器
  docker_ps: {
    label: 'docker_ps (枚举当前运行中容器列表)',
    group: 'DevOps 与容器运维',
    defaultArgs: '{\n}'
  },
  docker_logs: {
    label: 'docker_logs (检索容器最新标准输出日志)',
    group: 'DevOps 与容器运维',
    defaultArgs: '{\n  "container": "airoute-gateway-1",\n  "tail": 50\n}'
  },

  // 长期记忆与知识图谱
  create_entities: {
    label: 'create_entities (向长期记忆图谱写入实体节点)',
    group: '长期记忆与知识图谱',
    defaultArgs: '{\n  "entities": [\n    {"name": "airoute_gateway", "type": "infrastructure", "observations": ["HA dual node", "healthy"]}\n  ]\n}'
  },
  read_graph: {
    label: 'read_graph (读取跨会话完整实体拓扑图谱)',
    group: '长期记忆与知识图谱',
    defaultArgs: '{\n}'
  },

  // 网页检索
  brave_web_search: {
    label: 'brave_web_search (Brave 隐私实时网页检索)',
    group: '网络检索与抓取',
    defaultArgs: '{\n  "query": "Model Context Protocol enterprise architecture"\n}'
  },
  web_fetch_markdown: {
    label: 'web_fetch_markdown (网页解析提炼结构化Markdown)',
    group: '网络检索与抓取',
    defaultArgs: '{\n  "url": "https://example.com"\n}'
  }
};

const getToolInfo = (toolName) => {
  if (TOOL_TEMPLATES[toolName]) {
    return TOOL_TEMPLATES[toolName];
  }
  return {
    label: `${toolName} (MCP 工具)`,
    group: '扩展与自定义服务工具',
    defaultArgs: '{\n}'
  };
};

const getGroupLabel = (group, isZh) => {
  if (group === '网关原生安全审计') return isZh ? '网关原生安全审计' : 'Gateway Security & Compliance';
  if (group === '网关原生运维') return isZh ? '网关原生运维' : 'Gateway Native Ops';
  if (group === '网关原生增强') return isZh ? '网关原生增强' : 'Gateway Native Augmentation';
  if (group === 'Puppeteer 浏览器渲染') return isZh ? 'Puppeteer 浏览器渲染' : 'Puppeteer Browser Automation';
  if (group === '数据库与持久存储') return isZh ? '数据库与持久存储' : 'Databases & Storage';
  if (group === '认知与深度推理') return isZh ? '认知与深度推理' : 'Cognitive & Deep Reasoning';
  if (group === '扩展与自定义服务工具') return isZh ? '扩展与自定义服务工具' : 'Extended & Custom Tools';
  return group;
};

const getDefaultArgsForTool = (toolName) => {
  return getToolInfo(toolName).defaultArgs;
};

export default function McpIntegrationView({ adminFetch, onCopy, showToast, lang = 'zh', t }) {
  const isZh = lang === 'zh';
  const [servers, setServers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [togglingServer, setTogglingServer] = useState({});
  const [copiedKey, setCopiedKey] = useState('');

  // Token Management.
  //
  // The snippets used to be pre-filled with a stored API key, which rendered a saved
  // credential into the page (and into every copied cURL/config block). Plaintext is
  // shown once, at creation, so the fields start with the placeholder and the
  // operator either pastes a key they kept or explicitly selects one below.
  const getInitialToken = useCallback(() => '', []);

  const [probeToken, setProbeToken] = useState('');
  const [configToken, setConfigToken] = useState('');
  const [requestViewTab, setRequestViewTab] = useState('wire'); // 'wire' | 'json' | 'curl'

  useEffect(() => {
    const initTok = getInitialToken();
    if (!probeToken && initTok) setProbeToken(initTok);
    if (!configToken && initTok) setConfigToken(initTok);
    // The guards above make the extra dependencies harmless: once a token is set,
    // re-running cannot overwrite it.
  }, [getInitialToken, probeToken, configToken]);

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
  const [editingServerId, setEditingServerId] = useState('');
  const [savingServer, setSavingServer] = useState(false);
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
  const [probeToolArgs, setProbeToolArgs] = useState(getDefaultArgsForTool('airoute_data_redact'));
  const [probeRunning, setProbeRunning] = useState(false);
  const [probeResult, setProbeResult] = useState(null);
  const [probeLatency, setProbeLatency] = useState(null);
  const [probeHttpStatus, setProbeHttpStatus] = useState(null);
  const probeFailed = !!probeResult?.error || !!probeResult?.result?.isError || probeHttpStatus >= 400;
  const [lastSentPayload, setLastSentPayload] = useState(null);

  useModalA11y(configModalOpen, () => setConfigModalOpen(false));
  useModalA11y(addServerModalOpen, () => setAddServerModalOpen(false));
  useModalA11y(probeModalOpen, () => setProbeModalOpen(false));

  const sseUrl = resolveGatewayUrl(mcpSettings?.sse_endpoint || '/mcp/sse');
  const messagesUrl = resolveGatewayUrl(mcpSettings?.messages_endpoint || '/mcp/messages');

  const fetchServersAndSettings = useCallback(async () => {
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
  }, [adminFetch]);

  useEffect(() => {
    fetchServersAndSettings();
  }, [fetchServersAndSettings]);

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
        if (showToast) showToast(isZh ? `MCP 协议网关服务已${nextState ? '启用' : '关闭'}` : `MCP gateway service ${nextState ? 'enabled' : 'disabled'}`, 'success');
      } else {
        if (showToast) showToast(data.error || (isZh ? '切换失败' : 'Failed to toggle service'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
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
        if (showToast) showToast(isZh ? `MCP 服务 [${serverId}] 已${!currentEnabled ? '开启' : '关闭'}` : `MCP service [${serverId}] ${!currentEnabled ? 'enabled' : 'disabled'}`, 'success');
        fetchServersAndSettings();
      } else {
        if (showToast) showToast(data.error || (isZh ? '切换服务状态失败' : 'Failed to toggle server state'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setTogglingServer(prev => ({ ...prev, [serverId]: false }));
    }
  };

  const handleDeleteServer = async (serverId) => {
    if (!adminFetch) return;
    if (!window.confirm(isZh ? `确定要移除自定义 MCP 服务 [${serverId}] 吗？` : `Are you sure you want to remove MCP service [${serverId}]?`)) return;
    try {
      const res = await adminFetch(`/api/v1/admin/mcp/servers/${serverId}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setServers(prev => prev.filter(s => s.id !== serverId));
        if (showToast) showToast(isZh ? `MCP 服务 [${serverId}] 已成功移除` : `MCP service [${serverId}] removed successfully`, 'success');
        fetchServersAndSettings();
      } else {
        if (showToast) showToast(data.error || (isZh ? '删除失败' : 'Failed to delete service'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  const handleCreateServer = async (e) => {
    e.preventDefault();
    if (!adminFetch || savingServer) return;
    if (!newServer.name || !newServer.endpoint) {
      if (showToast) showToast(isZh ? '请填写服务名称与 Endpoint' : 'Please fill in service name and Endpoint', 'error');
      return;
    }

    const payload = {
      ...newServer,
      tools: newServer.tools ? newServer.tools.split(/[,，\n]/).map(t => t.trim()).filter(Boolean) : [],
      prompts: newServer.prompts ? newServer.prompts.split(/[,，\n]/).map(p => p.trim()).filter(Boolean) : [],
      resources: newServer.resources ? newServer.resources.split(/[,，\n]/).map(r => r.trim()).filter(Boolean) : []
    };

    setSavingServer(true);
    try {
      const res = await adminFetch('/api/v1/admin/mcp/servers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        if (showToast) showToast(isZh ? 'MCP 服务配置已保存' : 'MCP server configuration saved', 'success');
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
        if (showToast) showToast(data.error || (isZh ? '创建 MCP 服务失败' : 'Failed to create MCP service'), 'error');
      }
    } catch (err) {
      if (showToast) showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    } finally {
      setSavingServer(false);
    }
  };

  const getCurrentRpcPayload = () => {
    if (probeMethod === 'server/discover') {
      return {
        jsonrpc: "2.0",
        id: "probe-discover-" + (probeServer ? probeServer.id : 'core'),
        method: "server/discover",
        params: {}
      };
    } else if (probeMethod === 'tools/list') {
      return {
        jsonrpc: "2.0",
        id: "probe-list-" + (probeServer ? probeServer.id : 'core'),
        method: "tools/list",
        params: {}
      };
    } else {
      let parsedArgs = {};
      try {
        parsedArgs = JSON.parse(probeToolArgs);
      } catch (e) {
        parsedArgs = { _syntax_hint: "JSON 待补全" };
      }
      return {
        jsonrpc: "2.0",
        id: "probe-call-" + (probeServer ? probeServer.id : 'core'),
        method: "tools/call",
        params: {
          name: probeToolName,
          arguments: parsedArgs
        }
      };
    }
  };

  const handleSelectTool = (name) => {
    setProbeToolName(name);
    setProbeToolArgs(getDefaultArgsForTool(name));
  };

  const handleOpenProbe = (server) => {
    setProbeServer(server);
    setProbeMethod(server ? 'tools/call' : 'server/discover');
    setProbeResult(null);
    setProbeLatency(null);
    setProbeHttpStatus(null);
    setLastSentPayload(null);
    const initialTool = (server && server.tools && server.tools.length > 0)
      ? server.tools[0]
      : 'airoute_data_redact';
    setProbeToolName(initialTool);
    setProbeToolArgs(getDefaultArgsForTool(initialTool));
    if (!probeToken) {
      setProbeToken(getInitialToken());
    }
    setProbeModalOpen(true);
  };

  const handleRunProbe = async () => {
    setProbeRunning(true);
    setProbeResult(null);
    setProbeHttpStatus(null);
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
          if (showToast) showToast((isZh ? '工具参数 JSON 语法错误: ' : 'Tool argument JSON syntax error: ') + e.message, 'error');
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

      setLastSentPayload(rpcReq);

      const headers = { 'Content-Type': 'application/json' };
      if (probeToken && probeToken.trim()) {
        headers['Authorization'] = `Bearer ${probeToken.trim()}`;
      }

      const res = await fetch(messagesUrl, {
        method: 'POST',
        headers,
        body: JSON.stringify(rpcReq)
      });
      setProbeHttpStatus(res.status);
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
        error: { code: -32603, message: (isZh ? "探针请求失败: " : "Probe request failed: ") + err.message }
      });
    } finally {
      setProbeRunning(false);
    }
  };

  const getWireRequestText = () => {
    const payload = lastSentPayload || getCurrentRpcPayload();
    let path = '/mcp/messages';
    let host = typeof window !== 'undefined' && window.location?.host ? window.location.host : 'api.airoute.local';
    try {
      const u = new URL(messagesUrl, window.location.origin);
      path = u.pathname + (u.search || '');
      host = u.host;
    } catch (e) {
      // fallback
    }
    const token = probeToken?.trim() || 'YOUR_AIROUTE_KEY';
    return `POST ${path} HTTP/1.1\nHost: ${host}\nContent-Type: application/json\nAuthorization: Bearer ${token}\n\n${JSON.stringify(payload, null, 2)}`;
  };

  const getCurlRequestText = () => {
    const payload = lastSentPayload || getCurrentRpcPayload();
    const token = probeToken?.trim() || 'YOUR_AIROUTE_KEY';
    return `curl -X POST "${messagesUrl}" \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer ${token}" \\\n  -d '${JSON.stringify(payload)}'`;
  };

  const handleCopyText = async (text, key) => {
    if (onCopy) {
      onCopy(text);
    } else {
      await copyToClipboard(text);
    }
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(''), 2000);
    if (showToast) showToast(isZh ? '内容已复制到剪贴板' : 'Copied to clipboard', 'success');
  };

  const categories = [
    { id: 'all', name: isZh ? '全部服务' : 'All Services', icon: Boxes },
    { id: 'ops', name: isZh ? '网关原生' : 'Gateway Native', icon: Server },
    { id: 'dev', name: isZh ? '研发协作' : 'Dev Collab', icon: Code2 },
    { id: 'search', name: isZh ? '搜索抓取' : 'Search & Scrape', icon: Globe },
    { id: 'db', name: isZh ? '数据存储' : 'Data & Storage', icon: Database },
    { id: 'ai', name: isZh ? '深度推理' : 'Deep Reasoning', icon: Sparkles },
    { id: 'productivity', name: isZh ? '企业协同' : 'Productivity', icon: Layers }
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

  // Code Generation for Clients with Token Support
  const clientConfig = (client) => {
    try { return buildMCPClientConfig({ client, gatewayUrl: sseUrl, token: configToken, server: selectedServerForConfig }); }
    catch (err) { return JSON.stringify({ error: err.message }, null, 2); }
  };
  const getCursorConfig = () => clientConfig('cursor');
  const getClaudeConfig = () => clientConfig('claude');
  const getClineConfig = () => clientConfig('cline');

  const getPythonSnippet = () => {
    const token = configToken?.trim() || 'YOUR_AIROUTE_KEY';
    return `from mcp import ClientSession, StdioServerParameters
from mcp.client.sse import sse_client
import asyncio

async def run_mcp_client():
    # 配置统一鉴权 Token 请求头 (支持虚拟 API Key 与多租户配额隔离)
    headers = {
        "Authorization": "Bearer ${token}"
    }

    # 连接 Airoute 统一 MCP 代理网关 SSE 端点
    async with sse_client("${sseUrl}", headers=headers) as (read_stream, write_stream):
        async with ClientSession(read_stream, write_stream) as session:
            await session.initialize()
            
            # 列出网关聚合的所有 MCP 安全工具
            tools = await session.list_tools()
            print(f"网关已就绪，挂载 {len(tools.tools)} 个安全合规工具")
            
            # 调用数据安全脱敏工具示例
            result = await session.call_tool(
                "airoute_data_redact",
                arguments={"text": "客户张三 手机13812345678 密钥sk-abcdef123456"}
            )
            print("脱敏审计响应:", result.content[0].text)

if __name__ == "__main__":
    asyncio.run(run_mcp_client())`;
  };

  const getCurlSnippet = () => {
    const token = configToken?.trim() || 'YOUR_AIROUTE_KEY';
    return `# 1. 建立 MCP SSE 事件流长连接 (监听工具通知与端点握手)
curl -N -H "Authorization: Bearer ${token}" \\
  "${sseUrl}"

# 2. 发送 JSON-RPC 2.0 请求报文 (执行工具或发现服务)
curl -X POST "${messagesUrl}" \\
  -H "Authorization: Bearer ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "jsonrpc": "2.0",
    "id": "cli-test-01",
    "method": "tools/call",
    "params": {
      "name": "airoute_data_redact",
      "arguments": {
        "text": "客户张三 手机13812345678 密钥sk-abcdef123456"
      }
    }
  }'`;
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-200">
      {/* 顶部标题与轻量操作栏 */}
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

      {/* 核心指标与规格看板 */}
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
      )}

      {/* 客户端集成配置弹窗 */}
      {configModalOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={(e) => {
            if (e.target === e.currentTarget) setConfigModalOpen(false);
          }}
        >
          <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-[#E4E6E9] dark:border-[#2C3038] max-w-2xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-[#E4E6E9] dark:border-[#2C3038]">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-blue-50 dark:bg-blue-950/50 text-[#355CFF] dark:text-[#91A7FF] rounded-lg">
                  <FileCode className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    {isZh ? '客户端连接配置指南' : 'Client Connection Guide'}
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {selectedServerForConfig 
                      ? (isZh ? `针对 [${selectedServerForConfig.name}] 的专属配置` : `Configuration for [${selectedServerForConfig.name}]`)
                      : (isZh ? '将 Airoute 统一 MCP 网关导入您的 AI 客户端' : 'Integrate Airoute unified MCP gateway into your AI clients')}
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

            {/* 注入鉴权 Token 凭证栏 */}
            <div className="p-3 bg-[#F0F1F2] dark:bg-[#22252B] rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] flex flex-col sm:flex-row sm:flex-wrap sm:items-center justify-between gap-2.5">
              <div className="flex items-center gap-2">
                <Lock className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
                <div>
                  <span className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
                    {isZh ? '鉴权凭证注入 (Authorization Token / API Key)' : 'Auth Credential Injection (Token / API Key)'}
                  </span>
                  <span className="hidden sm:inline text-xs text-[#656B75] dark:text-[#A1A7B0] ml-2">
                    {isZh ? '(自动写入客户端配置文件与请求头)' : '(Injected into client configs & headers)'}
                  </span>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  value={configToken}
                  onChange={(e) => setConfigToken(e.target.value)}
                  placeholder="YOUR_AIROUTE_KEY"
                  className="px-2.5 py-1 text-xs font-mono rounded-sm border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-[#202124] dark:text-[#F1F2F4] min-w-[200px]"
                />
              </div>
              <p className="basis-full text-xs text-[#656B75] dark:text-[#A1A7B0]">
                {isZh
                  ? '列表里的 ak_… 是密钥 ID，只用于日志与工单识别，不能用于调用；明文仅在创建密钥时显示一次，遗失请新建一把。'
                  : 'An ak_… value in the list is a key ID: it identifies the key in logs and tickets but cannot be used to call. The plaintext is shown once at creation — create a new key if it was lost.'}
              </p>
            </div>

            {/* 客户端选择 Tabs */}
            <div className="flex items-center gap-2 border-b border-[#E4E6E9] dark:border-[#2C3038] pb-2 overflow-x-auto">
              {[
                { id: 'cursor', label: 'Cursor (.cursor/mcp.json)' },
                { id: 'claude', label: 'Claude Desktop' },
                { id: 'cline', label: 'Cline / Roo-Code' },
                { id: 'python', label: 'Python SDK' },
                { id: 'curl', label: isZh ? 'cURL / 终端 CLI' : 'cURL / Terminal CLI' }
              ].map(c => (
                <button
                  key={c.id}
                  onClick={() => setActiveConfigClient(c.id)}
                  className={`px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-colors ${activeConfigClient === c.id ? 'bg-[#355CFF] text-white shadow-2xs' : 'text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'}`}
                >
                  {c.label}
                </button>
              ))}
            </div>

            {/* 配置代码预览与一键复制 */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-[#202124] dark:text-[#F1F2F4]">
                  {activeConfigClient === 'cursor' && (isZh ? '配置文件位置: 项目根目录 .cursor/mcp.json' : 'Config path: project root .cursor/mcp.json')}
                  {activeConfigClient === 'claude' && (isZh ? '配置文件位置: ~/Library/Application Support/Claude/claude_desktop_config.json' : 'Config path: ~/Library/Application Support/Claude/claude_desktop_config.json')}
                  {activeConfigClient === 'cline' && (isZh ? '配置文件位置: Cline Settings -> MCP Servers' : 'Config path: Cline Settings -> MCP Servers')}
                  {activeConfigClient === 'python' && (isZh ? '安装依赖: pip install mcp httpx' : 'Dependencies: pip install mcp httpx')}
                  {activeConfigClient === 'curl' && (isZh ? '终端命令行测试 (自动携带 Authorization Bearer 鉴权头)' : 'Terminal CLI test (auto-includes Authorization Bearer header)')}
                </span>
                <button
                  onClick={() => {
                    const code = activeConfigClient === 'cursor' ? getCursorConfig() :
                                 activeConfigClient === 'claude' ? getClaudeConfig() :
                                 activeConfigClient === 'cline' ? getClineConfig() :
                                 activeConfigClient === 'python' ? getPythonSnippet() : getCurlSnippet();
                    handleCopyText(code, 'client-config');
                  }}
                  className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-[#F0F1F2] dark:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] rounded-sm hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors"
                >
                  {copiedKey === 'client-config' ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                  <span>{copiedKey === 'client-config' ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制配置' : 'Copy Config')}</span>
                </button>
              </div>

              <div className="bg-[#111214] text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto border border-[#2C3038]">
                <pre className="whitespace-pre-wrap leading-relaxed">
                  {activeConfigClient === 'cursor' && getCursorConfig()}
                  {activeConfigClient === 'claude' && getClaudeConfig()}
                  {activeConfigClient === 'cline' && getClineConfig()}
                  {activeConfigClient === 'python' && getPythonSnippet()}
                  {activeConfigClient === 'curl' && getCurlSnippet()}
                </pre>
              </div>
            </div>

            <div className="p-3 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/50 rounded-lg text-xs text-amber-800 dark:text-amber-300 flex items-start gap-2">
              <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
              <div>
                <strong>{isZh ? '安全最佳实践：' : 'Security Best Practice: '}</strong>
                {isZh ? '所有客户端均通过 Airoute 统一协议代理连接，无需直接暴露内部数据库或私有服务凭据。网关会自动执行参数脱敏与 SQL 注入审计。' : 'All clients connect via the unified Airoute protocol proxy without exposing database or private server credentials. Gateway auto-executes redaction and SQL injection auditing.'}
              </div>
            </div>

            <div className="flex justify-end pt-2">
              <button
                onClick={() => setConfigModalOpen(false)}
                className="px-4 py-2 text-xs font-medium text-[#656B75] dark:text-[#A1A7B0] bg-[#F0F1F2] dark:bg-[#22252B] rounded-lg hover:bg-slate-200 dark:hover:bg-slate-700"
              >
                {isZh ? '关闭' : 'Close'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 注册自定义 MCP 服务模态框 */}
      {addServerModalOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={(e) => {
            if (e.target === e.currentTarget) setAddServerModalOpen(false);
          }}
        >
          <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-[#E4E6E9] dark:border-[#2C3038] max-w-xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-[#E4E6E9] dark:border-[#2C3038]">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-[#355CFF] text-white rounded-lg shadow-2xs">
                  <Plus className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-gray-900 dark:text-white">
                    {isZh ? '接入新 MCP 服务' : 'Connect New MCP Server'}
                  </h3>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {isZh ? '遵循 ModelScope / Anthropic 规范标准接入自定义协议服务' : 'Connect custom protocol servers adhering to ModelScope / Anthropic standards'}
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
                  <label className="ui-label">
                    {isZh ? '服务唯一标识 (ID) *' : 'Server Unique ID *'}
                  </label>
                  <input
                    type="text"
                    required
                    placeholder={isZh ? "如: docker-mcp" : "e.g. docker-mcp"}
                    readOnly={!!editingServerId}
                    value={newServer.id}
                    onChange={(e) => setNewServer({ ...newServer, id: e.target.value })}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  />
                </div>
                <div>
                  <label className="ui-label">
                    {isZh ? '服务显示名称 *' : 'Server Display Name *'}
                  </label>
                  <input
                    type="text"
                    required
                    placeholder={isZh ? '如: Docker 容器管理服务' : 'e.g. Docker Container Service'}
                    value={newServer.name}
                    onChange={(e) => setNewServer({ ...newServer, name: e.target.value })}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="ui-label">
                    {isZh ? '分类领域' : 'Category'}
                  </label>
                  <select
                    value={newServer.category}
                    onChange={(e) => setNewServer({ ...newServer, category: e.target.value })}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  >
                    <option value="ops">{isZh ? '运维网关 (ops)' : 'Ops Gateway (ops)'}</option>
                    <option value="dev">{isZh ? '研发协作 (dev)' : 'Dev Collaboration (dev)'}</option>
                    <option value="search">{isZh ? '搜索抓取 (search)' : 'Search & Scrape (search)'}</option>
                    <option value="db">{isZh ? '数据存储 (db)' : 'Data & Storage (db)'}</option>
                    <option value="ai">{isZh ? '深度推理 (ai)' : 'Deep Reasoning (ai)'}</option>
                    <option value="productivity">{isZh ? '企业协同 (productivity)' : 'Enterprise Collab (productivity)'}</option>
                  </select>
                </div>
                <div>
                  <label className="ui-label">
                    {isZh ? '传输通道 (Transport)' : 'Transport Channel'}
                  </label>
                  <select
                    value={newServer.transport}
                    onChange={(e) => setNewServer({ ...newServer, transport: e.target.value })}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  >
                    <option value="stdio">{isZh ? 'stdio (本地命令/子进程)' : 'stdio (Local Process)'}</option>
                    <option value="sse">{isZh ? 'sse (远程 HTTP Server-Sent Events)' : 'sse (Remote HTTP SSE)'}</option>
                    <option value="http">http (Streamable HTTP)</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="ui-label">
                  {isZh ? '服务 Endpoint / 执行命令 *' : 'Service Endpoint / Command *'}
                </label>
                <input
                  type="text"
                  required
                  placeholder={isZh ? '如: npx -y @modelcontextprotocol/server-docker 或 https://mcp.example.com/sse' : 'e.g. npx -y @modelcontextprotocol/server-docker or https://mcp.example.com/sse'}
                  value={newServer.endpoint}
                  onChange={(e) => setNewServer({ ...newServer, endpoint: e.target.value })}
                  className="ui-field bg-gray-50 dark:bg-gray-900"
                />
              </div>

              <div>
                <label className="ui-label">
                  {isZh ? '服务描述' : 'Description'}
                </label>
                <textarea
                  rows="2"
                  placeholder={isZh ? '简述该 MCP 服务的功能、适用场景及接入说明...' : 'Describe MCP server capabilities, scenario, and instructions...'}
                  value={newServer.description}
                  onChange={(e) => setNewServer({ ...newServer, description: e.target.value })}
                  className="ui-field bg-gray-50 dark:bg-gray-900"
                />
              </div>

              <div>
                <label className="ui-label">
                  {isZh ? '暴露的 Tools 工具集 (逗号分隔)' : 'Exposed Tools (comma separated)'}
                </label>
                <input
                  type="text"
                  placeholder={isZh ? "如: docker_ps, docker_logs, docker_restart" : "e.g. docker_ps, docker_logs, docker_restart"}
                  value={newServer.tools}
                  onChange={(e) => setNewServer({ ...newServer, tools: e.target.value })}
                  className="ui-field bg-gray-50 dark:bg-gray-900 font-mono"
                />
              </div>

              <label className="block space-y-1 text-xs">
                <span>{isZh ? '进程环境变量 (JSON)' : 'Process environment (JSON)'}</span>
                <textarea rows={3} value={newServer.env_vars} onChange={e => setNewServer({ ...newServer, env_vars: e.target.value })} className="ui-field font-mono" placeholder='{"SERVICE_TOKEN":"..."}' />
              </label>
              <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-gray-200 dark:border-gray-800">
                <button
                  type="button"
                  onClick={() => setAddServerModalOpen(false)}
                  className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={savingServer}
                  className="px-4 py-2 text-xs font-medium text-white bg-[#355CFF] rounded-lg hover:bg-blue-600 shadow-sm"
                >
                  {savingServer ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存服务' : 'Save server')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 在线 JSON-RPC 2.0 协议探针控制台 */}
      {probeModalOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={(e) => {
            if (e.target === e.currentTarget) setProbeModalOpen(false);
          }}
        >
          <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-[#E4E6E9] dark:border-[#2C3038] max-w-5xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between pb-3 border-b border-[#E4E6E9] dark:border-[#2C3038]">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-[#355CFF] text-white rounded-lg shadow-2xs">
                  <Radio className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-[#202124] dark:text-[#F1F2F4] flex items-center gap-2">
                    {isZh ? '在线协议探针控制台' : 'Online Protocol Probe Console'}
                    <span className="text-xs font-mono font-normal text-[#355CFF] dark:text-[#91A7FF]">JSON-RPC 2.0</span>
                  </h3>
                  <p className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
                    {isZh ? `向网关端点 ${messagesUrl} 发送标准协议报文并实时验证安全拦截与工具输出` : `Send standard JSON-RPC 2.0 messages to ${messagesUrl} to verify security interception & output`}
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

            {/* 协议教程与通信原理解析 */}
            <div className="p-3 bg-blue-50/60 dark:bg-[#242D48]/50 border border-blue-200/50 dark:border-blue-900/40 rounded-xl text-xs text-blue-950 dark:text-[#91A7FF]">
              <div className="flex items-center gap-2 font-semibold mb-1 text-[#355CFF] dark:text-[#91A7FF]">
                <BookOpen className="w-4 h-4" />
                <span>{isZh ? 'MCP 双向通信原理解析与接入教程' : 'MCP Bi-directional Architecture & Tutorial'}</span>
              </div>
              <p className="text-xs leading-relaxed text-[#656B75] dark:text-[#A1A7B0]">
                {isZh ? (
                  <><b>标准交互流:</b> 客户端首先发起 <code className="font-mono bg-blue-100 dark:bg-blue-950/80 px-1 py-0.5 rounded-sm">GET /mcp/sse</code> 建立长连接握手；随后将左侧的 <b>JSON-RPC 2.0 请求报文</b> 通过 <code className="font-mono bg-blue-100 dark:bg-blue-950/80 px-1 py-0.5 rounded-sm">POST /mcp/messages</code> 投递至网关。网关安全审计并调度工具后，将右侧的 <b>响应结果</b> 实时返回给客户端。</>
                ) : (
                  <><b>Standard Flow:</b> Clients initiate <code className="font-mono bg-blue-100 dark:bg-blue-950/80 px-1 py-0.5 rounded-sm">GET /mcp/sse</code> handshake, then send <b>JSON-RPC 2.0 request</b> via <code className="font-mono bg-blue-100 dark:bg-blue-950/80 px-1 py-0.5 rounded-sm">POST /mcp/messages</code>. Gateway audits security and dispatches tools, returning <b>responses</b> in real-time.</>
                )}
              </p>
            </div>

            {/* 鉴权 Token 凭证与目标服务配置栏 */}
            <div className="p-3 bg-[#F0F1F2] dark:bg-[#22252B] rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] space-y-2.5">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <Lock className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
                  <span className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
                    {isZh ? '鉴权凭证 (Authorization Token / 虚拟 API Key)' : 'Authentication Token / Virtual API Key'}
                  </span>
                  <span className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
                    {isZh ? '(支持 Bearer Token 与 X-API-Key 多租户隔离)' : '(Supports Bearer Token & multi-tenant isolation)'}
                  </span>
                </div>
              </div>
              <input
                type="text"
                value={probeToken}
                onChange={(e) => setProbeToken(e.target.value)}
                placeholder={isZh ? '输入 API Key 明文 (如 sk-airoute-...)' : 'Enter plaintext API Key (e.g., sk-airoute-...)'}
                className="w-full px-3 py-1.5 font-mono text-xs rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] text-[#202124] dark:text-[#F1F2F4] placeholder-[#656B75] dark:placeholder-[#A1A7B0] focus:outline-none focus:ring-1 focus:ring-[#355CFF]"
              />
              <p className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
                {isZh
                  ? '列表里的 ak_… 是密钥 ID，只用于日志与工单识别，不能用于调用；明文仅在创建密钥时显示一次，遗失请新建一把。'
                  : 'An ak_… value in the list is a key ID: it identifies the key in logs and tickets but cannot be used to call. The plaintext is shown once at creation — create a new key if it was lost.'}
              </p>
            </div>

            {/* 方法、服务与参数选择 */}
            <div className="space-y-3">
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <div>
                  <label className="ui-label">
                    {isZh ? '工具来源（能力发现与列表始终查询聚合网关）' : 'Tool source (discovery and listing query the gateway)'}
                  </label>
                  <select
                    value={probeServer ? probeServer.id : 'all'}
                    onChange={(e) => {
                      const sid = e.target.value;
                      if (sid === 'all') {
                        setProbeServer(null);
                      } else {
                        const found = servers.find(s => s.id === sid);
                        setProbeServer(found || null);
                        if (found && found.tools && found.tools.length > 0) {
                          handleSelectTool(found.tools[0]);
                        }
                      }
                    }}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  >
                    <option value="all">{isZh ? '🌐 统一 MCP 聚合网关 (全部已启用服务)' : '🌐 Unified MCP Gateway (All Enabled Services)'}</option>
                    {servers.map(s => (
                      <option key={s.id} value={s.id}>
                        {s.name} ({s.id})
                      </option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="ui-label">
                    {isZh ? '探针方法 (JSON-RPC Method)' : 'Probe Method (JSON-RPC Method)'}
                  </label>
                  <select
                    value={probeMethod}
                    onChange={(e) => setProbeMethod(e.target.value)}
                    className="ui-field bg-gray-50 dark:bg-gray-900"
                  >
                    <option value="server/discover">{isZh ? 'server/discover (服务与能力探测)' : 'server/discover (Discover Capabilities)'}</option>
                    <option value="tools/list">{isZh ? 'tools/list (枚举工具列表与 Schema)' : 'tools/list (List Tools & Schema)'}</option>
                    <option value="tools/call">{isZh ? 'tools/call (在线执行具体工具)' : 'tools/call (Execute Specific Tool)'}</option>
                  </select>
                </div>

                {probeMethod === 'tools/call' && (
                  <div>
                    <label className="ui-label">
                      {isZh ? '选择目标工具 (Tool Name)' : 'Select Target Tool (Tool Name)'}
                    </label>
                    <select
                      value={probeToolName}
                      onChange={(e) => handleSelectTool(e.target.value)}
                      className="ui-field bg-gray-50 dark:bg-gray-900 font-mono"
                    >
                      {/* 如果当前指定了服务，将其工具置顶 */}
                      {probeServer && probeServer.tools && probeServer.tools.length > 0 && (
                        <optgroup label={isZh ? `★ 当前服务 [${probeServer.name || probeServer.id}] 工具集` : `★ Current Server [${probeServer.name || probeServer.id}] Tools`}>
                          {probeServer.tools.map(t => (
                            <option key={`cur-${t}`} value={t}>
                              {getToolInfo(t).label}
                            </option>
                          ))}
                        </optgroup>
                      )}

                      {/* 按分类分组呈现内置与生态工具 */}
                      {Object.entries(
                        Object.entries(TOOL_TEMPLATES).reduce((acc, [name, info]) => {
                          acc[info.group] = acc[info.group] || [];
                          acc[info.group].push({ name, ...info });
                          return acc;
                        }, {})
                      ).map(([grp, toolList]) => (
                        <optgroup key={grp} label={getGroupLabel(grp, isZh)}>
                          {toolList.map(t => (
                            <option key={t.name} value={t.name}>
                              {t.label}
                            </option>
                          ))}
                        </optgroup>
                      ))}

                      {/* 容底项：若选中工具不在模板库中 */}
                      {!TOOL_TEMPLATES[probeToolName] && (!probeServer?.tools?.includes(probeToolName)) && (
                        <optgroup label={isZh ? '自定义探针工具' : 'Custom Probe Tool'}>
                          <option value={probeToolName}>{probeToolName} ({isZh ? '当前选定工具' : 'Selected'})</option>
                        </optgroup>
                      )}
                    </select>
                  </div>
                )}
              </div>

              {probeMethod === 'tools/call' && (
                <div>
                  <div className="flex items-center justify-between mb-1">
                    <label className="text-xs font-medium text-gray-700 dark:text-gray-300">
                      {isZh ? '工具输入参数 (JSON Arguments)' : 'Tool Input Arguments (JSON Arguments)'}
                    </label>
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <button
                        type="button"
                        onClick={() => handleSelectTool('airoute_data_redact')}
                        className="text-xs text-[#355CFF] dark:text-[#91A7FF] hover:underline"
                      >
                        {isZh ? '敏感脱敏' : 'Redaction'}
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => handleSelectTool('airoute_sql_security_check')}
                        className="text-xs text-rose-600 dark:text-rose-400 hover:underline"
                      >
                        {isZh ? 'SQL拦截' : 'SQL Guard'}
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => handleSelectTool('puppeteer_navigate')}
                        className="text-xs text-emerald-600 dark:text-emerald-400 hover:underline"
                      >
                        {isZh ? 'Puppeteer导航' : 'Puppeteer'}
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => handleSelectTool('read_query')}
                        className="text-xs text-amber-600 dark:text-amber-400 hover:underline"
                      >
                        {isZh ? '只读SQL' : 'Read-only SQL'}
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => handleSelectTool('sequentialthinking')}
                        className="text-xs text-sky-600 dark:text-sky-400 hover:underline"
                      >
                        {isZh ? '深度推理' : 'Thinking'}
                      </button>
                    </div>
                  </div>
                  <textarea
                    rows="3"
                    value={probeToolArgs}
                    onChange={(e) => setProbeToolArgs(e.target.value)}
                    className="w-full px-3 py-2 font-mono text-xs rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-[#111214] text-gray-100"
                  />
                </div>
              )}

              <div className="flex items-center justify-between pt-1">
                <span className="text-xs text-[#656B75] dark:text-[#A1A7B0]">
                  {probeLatency !== null && (
                    <span className={probeFailed ? 'text-rose-500 font-mono' : 'text-emerald-500 font-mono'}>
                      {probeFailed ? '✗' : '✓'} {isZh ? `往返耗时: ${probeLatency} ms` : `Roundtrip: ${probeLatency} ms`}
                    </span>
                  )}
                </span>
                <button
                  onClick={handleRunProbe}
                  disabled={probeRunning}
                  className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-white bg-[#355CFF] rounded-lg hover:bg-blue-600 disabled:opacity-50 transition-colors shadow-sm"
                >
                  <Send className={`w-3.5 h-3.5 ${probeRunning ? 'animate-pulse' : ''}`} />
                  <span>{probeRunning ? (isZh ? '探针执行中...' : 'Probing...') : (isZh ? '发送 JSON-RPC 2.0 报文' : 'Send JSON-RPC 2.0 Request')}</span>
                </button>
              </div>

              {/* 双向报文链路对比: 发送报文 (左) 与 响应结果 (右) */}
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 pt-2">
                {/* 1. 发送请求报文 */}
                <div className="flex flex-col space-y-1.5">
                  <div className="flex items-center justify-between text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
                    <div className="flex items-center gap-1.5">
                      <span className="flex items-center gap-1 text-[#355CFF] dark:text-[#91A7FF]">
                        <Send className="w-3.5 h-3.5" />
                        {isZh ? '发送请求报文' : 'Request Payload'}
                      </span>
                      <div className="flex items-center bg-[#F0F1F2] dark:bg-[#22252B] rounded-lg p-0.5 ml-1 text-xs">
                        <button
                          type="button"
                          onClick={() => setRequestViewTab('wire')}
                          className={`px-1.5 py-0.5 rounded-sm ${requestViewTab === 'wire' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-medium' : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
                        >
                          {isZh ? 'HTTP报文' : 'Wire'}
                        </button>
                        <button
                          type="button"
                          onClick={() => setRequestViewTab('json')}
                          className={`px-1.5 py-0.5 rounded-sm ${requestViewTab === 'json' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-medium' : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
                        >
                          JSON-RPC
                        </button>
                        <button
                          type="button"
                          onClick={() => setRequestViewTab('curl')}
                          className={`px-1.5 py-0.5 rounded-sm ${requestViewTab === 'curl' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-medium' : 'text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
                        >
                          cURL
                        </button>
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={() => {
                        const txt = requestViewTab === 'wire' ? getWireRequestText() :
                                    requestViewTab === 'json' ? JSON.stringify(lastSentPayload || getCurrentRpcPayload(), null, 2) :
                                    getCurlRequestText();
                        handleCopyText(txt, 'probe-req');
                      }}
                      className="inline-flex items-center gap-1 text-xs font-normal text-[#656B75] hover:text-[#355CFF] dark:hover:text-[#91A7FF] transition-colors"
                    >
                      {copiedKey === 'probe-req' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                      {isZh ? '复制请求' : 'Copy Request'}
                    </button>
                  </div>
                  <div className="bg-[#111214] text-gray-100 p-3.5 rounded-xl font-mono text-xs overflow-x-auto min-h-[170px] max-h-64 border border-[#2C3038] inset-shadow-sm">
                    <pre className="whitespace-pre-wrap leading-relaxed">
                      {requestViewTab === 'wire' && getWireRequestText()}
                      {requestViewTab === 'json' && JSON.stringify(lastSentPayload || getCurrentRpcPayload(), null, 2)}
                      {requestViewTab === 'curl' && getCurlRequestText()}
                    </pre>
                  </div>
                  <div className="text-xs text-[#656B75] dark:text-[#A1A7B0] flex items-center justify-between">
                    <span>{isZh ? `传输端点: POST ${messagesUrl}` : `Endpoint: POST ${messagesUrl}`}</span>
                    <span>{isZh ? (probeToken ? '鉴权: Bearer Token (已配置)' : '鉴权: 未配置 Token') : (probeToken ? 'Auth: Bearer Token' : 'Auth: None')}</span>
                  </div>
                </div>

                {/* 2. 网关接收响应 */}
                <div className="flex flex-col space-y-1.5">
                  <div className="flex items-center justify-between text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
                    <span className="flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
                      <CheckCircle2 className="w-3.5 h-3.5" />
                      {isZh ? `接收响应 (${probeHttpStatus ? `HTTP ${probeHttpStatus}` : '等待请求'})` : `Response Payload (${probeHttpStatus ? `HTTP ${probeHttpStatus}` : 'Pending'})`}
                    </span>
                    {probeResult && (
                      <button
                        type="button"
                        onClick={() => handleCopyText(JSON.stringify(probeResult, null, 2), 'probe-res')}
                        className="inline-flex items-center gap-1 text-xs font-normal text-[#656B75] hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors"
                      >
                        {copiedKey === 'probe-res' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                        {isZh ? '复制响应' : 'Copy Response'}
                      </button>
                    )}
                  </div>
                  <div className="bg-[#111214] text-gray-100 p-3.5 rounded-xl font-mono text-xs overflow-x-auto min-h-[170px] max-h-64 border border-[#2C3038] inset-shadow-sm flex flex-col justify-start">
                    {probeResult ? (
                      <pre className="whitespace-pre-wrap leading-relaxed">{JSON.stringify(probeResult, null, 2)}</pre>
                    ) : (
                      <div className="h-full flex flex-col items-center justify-center text-gray-500 text-center py-8">
                        <Terminal className="w-8 h-8 mb-2 opacity-40 text-[#355CFF]" />
                        <p className="text-xs font-medium text-gray-400">{isZh ? '等待发送探针' : 'Awaiting Probe Execution'}</p>
                        <p className="text-xs text-gray-400 mt-1">{isZh ? '点击上方“发送 JSON-RPC 2.0 报文”发起实时调用测试' : 'Click "Send JSON-RPC 2.0 Request" above to test live execution'}</p>
                      </div>
                    )}
                  </div>
                  <div className="text-xs text-slate-500 dark:text-slate-400 flex items-center justify-between">
                    <span>{isZh ? `网关延迟: ${probeLatency !== null ? `${probeLatency} ms` : '--'}` : `Latency: ${probeLatency !== null ? `${probeLatency} ms` : '--'}`}</span>
                    <span>{isZh ? `状态: ${probeResult ? (probeFailed ? '调用失败' : '成功返回') : '待调用'}` : `Status: ${probeResult ? (probeFailed ? 'Failed' : 'Success') : 'Pending'}`}</span>
                  </div>
                </div>
              </div>
            </div>

            <div className="flex justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
              <button
                onClick={() => setProbeModalOpen(false)}
                className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
              >
                {isZh ? '关闭' : 'Close'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
