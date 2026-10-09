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
import { getGatewayOrigin, resolveGatewayUrl } from '../config';

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
    defaultArgs: '{\n  "task_type": "coding",\n  "max_budget_per_m": 1.0\n}'
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
    defaultArgs: '{\n  "entities": [\n    {"name": "airoute_gateway", "type": "infrastructure", "observations": ["HA dual node", "port 8080"]}\n  ]\n}'
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

const getDefaultArgsForTool = (toolName) => {
  return getToolInfo(toolName).defaultArgs;
};

export default function McpIntegrationView({ adminFetch, adminToken, keys = [], onCopy, showToast }) {
  const [servers, setServers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [togglingServer, setTogglingServer] = useState({});
  const [copiedKey, setCopiedKey] = useState('');

  // Token Management
  const getInitialToken = () => {
    if (keys && keys.length > 0) {
      const active = keys.find(k => k.status === 'active' || k.status === 1 || !k.status);
      return active ? active.key : keys[0].key;
    }
    return adminToken || '';
  };

  const [probeToken, setProbeToken] = useState('');
  const [configToken, setConfigToken] = useState('');
  const [requestViewTab, setRequestViewTab] = useState('wire'); // 'wire' | 'json' | 'curl'

  useEffect(() => {
    const initTok = getInitialToken();
    if (!probeToken && initTok) setProbeToken(initTok);
    if (!configToken && initTok) setConfigToken(initTok);
  }, [keys, adminToken]);

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
  const [probeToolArgs, setProbeToolArgs] = useState(getDefaultArgsForTool('airoute_data_redact'));
  const [probeRunning, setProbeRunning] = useState(false);
  const [probeResult, setProbeResult] = useState(null);
  const [probeLatency, setProbeLatency] = useState(null);
  const [lastSentPayload, setLastSentPayload] = useState(null);

  const origin = getGatewayOrigin();
  const sseUrl = resolveGatewayUrl(mcpSettings?.sse_endpoint || '/mcp/sse');
  const messagesUrl = resolveGatewayUrl(mcpSettings?.messages_endpoint || '/mcp/messages');

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
    setProbeResult(null);
    setProbeLatency(null);
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

  const getWireRequestText = () => {
    const payload = lastSentPayload || getCurrentRpcPayload();
    let path = '/mcp/messages';
    let host = 'localhost:8080';
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

  const handleCopyText = (text, key) => {
    if (onCopy) {
      onCopy(text);
    } else {
      navigator.clipboard.writeText(text);
    }
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(''), 2000);
    if (showToast) showToast('内容已复制到剪贴板', 'success');
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

  // Code Generation for Clients with Token Support
  const getCursorConfig = () => {
    const token = configToken?.trim() || 'YOUR_AIROUTE_KEY';
    return JSON.stringify({
      mcpServers: {
        "airoute-gateway": {
          url: sseUrl,
          headers: {
            "Authorization": `Bearer ${token}`
          }
        },
        ...(selectedServerForConfig && selectedServerForConfig.id !== 'airoute-gateway' ? {
          [selectedServerForConfig.id]: {
            command: selectedServerForConfig.transport === 'stdio' ? "npx" : undefined,
            args: selectedServerForConfig.transport === 'stdio' ? selectedServerForConfig.endpoint.replace('npx -y ', '').split(' ') : undefined,
            url: selectedServerForConfig.transport === 'sse' ? selectedServerForConfig.endpoint : undefined,
            headers: selectedServerForConfig.transport === 'sse' ? { "Authorization": `Bearer ${token}` } : undefined
          }
        } : {})
      }
    }, null, 2);
  };

  const getClaudeConfig = () => {
    const token = configToken?.trim() || 'YOUR_AIROUTE_KEY';
    return JSON.stringify({
      mcpServers: {
        "airoute-gateway": {
          command: "npx",
          args: ["-y", "mcp-remote", sseUrl, "--header", `Authorization: Bearer ${token}`],
          env: {
            "AIRoute_API_KEY": token
          }
        },
        ...(selectedServerForConfig && selectedServerForConfig.id !== 'airoute-gateway' ? {
          [selectedServerForConfig.id]: {
            command: "npx",
            args: selectedServerForConfig.transport === 'stdio' 
              ? ["-y", ...selectedServerForConfig.endpoint.replace('npx -y ', '').split(' ')] 
              : ["-y", "mcp-remote", selectedServerForConfig.endpoint, "--header", `Authorization: Bearer ${token}`],
            env: {
              "AIRoute_API_KEY": token
            }
          }
        } : {})
      }
    }, null, 2);
  };

  const getClineConfig = () => {
    const token = configToken?.trim() || 'YOUR_AIROUTE_KEY';
    return JSON.stringify({
      mcpServers: {
        "airoute-gateway": {
          type: "sse",
          url: sseUrl,
          headers: {
            "Authorization": `Bearer ${token}`
          },
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
    <div className="space-y-5">
      {/* 顶部标题与轻量操作栏 */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 pb-3 border-b border-gray-100 dark:border-gray-800">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-lg font-bold text-gray-900 dark:text-white">
              Model Context Protocol (MCP) 广场
            </h1>
            <span className="px-2 py-0.5 text-[11px] font-medium rounded-full bg-indigo-50 dark:bg-indigo-950/60 text-indigo-600 dark:text-indigo-400 border border-indigo-200 dark:border-indigo-800">
              JSON-RPC 2.0
            </span>
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
            连接外部系统与协议扩展，支持 SSE 与 Stdio 传输，提供统一鉴权、安全脱敏与客户端对接。
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => handleOpenProbe(null)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-750 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors shadow-2xs"
          >
            <Radio className="w-3.5 h-3.5 text-indigo-600 dark:text-indigo-400" />
            协议探针
          </button>
          <button
            onClick={() => { setSelectedServerForConfig(null); setConfigModalOpen(true); }}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-750 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors shadow-2xs"
          >
            <FileCode className="w-3.5 h-3.5 text-purple-600 dark:text-purple-400" />
            客户端配置
          </button>
          <button
            onClick={() => setAddServerModalOpen(true)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-indigo-600 hover:bg-indigo-700 rounded-lg transition-colors shadow-2xs"
          >
            <Plus className="w-3.5 h-3.5" />
            接入新服务
          </button>
          <button
            onClick={fetchServersAndSettings}
            disabled={loading}
            className="p-1.5 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded-lg border border-gray-200 dark:border-gray-750 bg-white dark:bg-gray-800 hover:bg-gray-50 transition-colors"
            title="刷新服务状态"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* 核心指标与主控简报 */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-3 rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-2xs">
        <div className="flex items-center gap-4 flex-wrap text-xs">
          {/* 总控开关 */}
          <div className="flex items-center gap-2 pr-4 border-r border-gray-200 dark:border-gray-750">
            <span className="font-medium text-gray-700 dark:text-gray-200">协议总闸:</span>
            <button
              onClick={handleToggleGlobalMcp}
              disabled={togglingGlobal}
              className={`relative inline-flex h-4.5 w-8 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${mcpSettings.mcp_enabled ? 'bg-indigo-600' : 'bg-gray-300 dark:bg-gray-600'}`}
              title={mcpSettings.mcp_enabled ? '点击关闭 MCP' : '点击开启 MCP'}
            >
              <span
                className={`pointer-events-none inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${mcpSettings.mcp_enabled ? 'translate-x-3.5' : 'translate-x-0'}`}
              />
            </button>
            <span className={`text-[11px] font-medium ${mcpSettings.mcp_enabled ? 'text-indigo-600 dark:text-indigo-400' : 'text-gray-400'}`}>
              {mcpSettings.mcp_enabled ? '已激活' : '已停用'}
            </span>
          </div>

          {/* 活跃服务计数 */}
          <div className="flex items-center gap-1.5 text-gray-600 dark:text-gray-300">
            <span className="text-gray-400">活跃服务:</span>
            <span className="font-semibold text-gray-900 dark:text-white">{onlineServersCount}</span>
            <span className="text-gray-400">/ {servers.length}</span>
          </div>

          {/* 聚合工具计数 */}
          <div className="flex items-center gap-1.5 text-gray-600 dark:text-gray-300">
            <span className="text-gray-400">挂载工具:</span>
            <span className="font-semibold text-gray-900 dark:text-white">{totalToolsCount}</span>
            <span className="text-gray-400">个</span>
          </div>
        </div>

        {/* SSE 端点展示与复制 */}
        <div className="flex items-center gap-1.5 text-xs text-gray-500 shrink-0">
          <span className="text-gray-400 text-[11px]">SSE 端点:</span>
          <code className="px-1.5 py-0.5 rounded font-mono text-[11px] bg-gray-100 dark:bg-gray-900 text-gray-700 dark:text-gray-300 border border-gray-200/80 dark:border-gray-800">
            /mcp/sse
          </code>
          <button
            onClick={() => handleCopyText(sseUrl, 'sse-endpoint')}
            className="p-1 text-gray-400 hover:text-indigo-600 transition-colors"
            title="复制完整端点 URL"
          >
            {copiedKey === 'sse-endpoint' ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
          </button>
        </div>
      </div>

      {/* 搜索与分类导航 */}
      <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3 bg-white dark:bg-gray-800/80 p-2.5 rounded-xl border border-gray-200 dark:border-gray-800 shadow-2xs">
        <div className="flex items-center gap-1 overflow-x-auto pb-1 md:pb-0 scrollbar-none">
          {categories.map(cat => {
            const Icon = cat.icon;
            const active = selectedCategory === cat.id;
            return (
              <button
                key={cat.id}
                onClick={() => setSelectedCategory(cat.id)}
                className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg whitespace-nowrap transition-all ${active ? 'bg-indigo-600 text-white shadow-2xs' : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700/60'}`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{cat.name}</span>
              </button>
            );
          })}
        </div>

        <div className="relative min-w-[220px]">
          <Search className="w-3.5 h-3.5 text-gray-400 absolute left-2.5 top-2.5" />
          <input
            type="text"
            placeholder="搜索 MCP 服务或工具..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-8 pr-3 py-1 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900/60 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-indigo-500"
          />
        </div>
      </div>

      {/* MCP 服务卡片列表 */}
      {filteredServers.length === 0 ? (
        <div className="p-10 text-center bg-white dark:bg-gray-800/50 rounded-xl border border-dashed border-gray-200 dark:border-gray-800">
          <Cpu className="w-8 h-8 text-gray-300 dark:text-gray-600 mx-auto mb-2" />
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300">未找到匹配的 MCP 服务</p>
          <p className="text-xs text-gray-400 mt-1">可在上方切换分类筛选或点击「接入新服务」</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredServers.map(server => {
            const isToggling = !!togglingServer[server.id];
            const isCustom = server.id.startsWith('custom-');

            return (
              <div
                key={server.id}
                className={`flex flex-col justify-between p-4 rounded-xl border transition-all duration-150 ${server.enabled ? 'border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-800/90 shadow-2xs hover:shadow-xs' : 'border-gray-200/60 dark:border-gray-800/60 bg-gray-50/60 dark:bg-gray-850/40 opacity-75'}`}
              >
                <div>
                  {/* 卡片头部 */}
                  <div className="flex items-start justify-between gap-2.5 mb-2">
                    <div className="flex items-center gap-2.5">
                      <div className={`p-2 rounded-lg shrink-0 ${server.id === 'airoute-gateway' ? 'bg-indigo-50 dark:bg-indigo-950/70 text-indigo-600 dark:text-indigo-400 border border-indigo-200/50 dark:border-indigo-800/50' : 'bg-gray-50 dark:bg-gray-800 text-gray-600 dark:text-gray-300 border border-gray-200/50 dark:border-gray-750'}`}>
                        {server.category === 'ops' ? <Server className="w-4 h-4" /> :
                         server.category === 'dev' ? <Code2 className="w-4 h-4" /> :
                         server.category === 'search' ? <Globe className="w-4 h-4" /> :
                         server.category === 'db' ? <Database className="w-4 h-4" /> :
                         server.category === 'ai' ? <Sparkles className="w-4 h-4" /> :
                         <Layers className="w-4 h-4" />}
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <h3 className="text-sm font-semibold text-gray-900 dark:text-white truncate">
                            {server.name}
                          </h3>
                          <span className={`px-1.5 py-0.2 text-[10px] font-mono font-semibold rounded ${server.transport === 'sse' ? 'bg-blue-50 dark:bg-blue-950/60 text-blue-600 dark:text-blue-300' : 'bg-purple-50 dark:bg-purple-950/60 text-purple-600 dark:text-purple-300'}`}>
                            {server.transport.toUpperCase()}
                          </span>
                        </div>
                        <div className="text-[11px] font-mono text-gray-400 truncate">
                          {server.id}
                        </div>
                      </div>
                    </div>

                    <button
                      onClick={() => handleToggleServer(server.id, server.enabled)}
                      disabled={isToggling}
                      className={`relative inline-flex h-4.5 w-8 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${server.enabled ? 'bg-indigo-600' : 'bg-gray-300 dark:bg-gray-600'}`}
                      title={server.enabled ? '点击停用' : '点击启用'}
                    >
                      <span
                        className={`pointer-events-none inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${server.enabled ? 'translate-x-3.5' : 'translate-x-0'}`}
                      />
                    </button>
                  </div>

                  {/* 描述文案 */}
                  <p className="text-xs text-gray-600 dark:text-gray-400 leading-relaxed line-clamp-2 mb-2.5">
                    {server.description}
                  </p>

                  {/* 暴露的 Tools 标签组 */}
                  {server.tools && server.tools.length > 0 && (
                    <div className="flex items-center gap-1.5 flex-wrap mb-3 text-[11px]">
                      <span className="text-gray-400">工具:</span>
                      {server.tools.slice(0, 3).map(t => (
                        <span
                          key={t}
                          className="px-1.5 py-0.5 font-mono text-[10px] bg-gray-50 dark:bg-gray-900 text-gray-600 dark:text-gray-300 rounded border border-gray-200/80 dark:border-gray-750"
                        >
                          {t}
                        </span>
                      ))}
                      {server.tools.length > 3 && (
                        <span className="text-gray-400 text-[10px]">+{server.tools.length - 3}</span>
                      )}
                    </div>
                  )}
                </div>

                {/* 卡片底部操作栏 */}
                <div className="pt-2.5 border-t border-gray-100 dark:border-gray-800 flex items-center justify-between gap-1.5">
                  <div className="flex items-center gap-1.5">
                    <button
                      onClick={() => handleOpenProbe(server)}
                      className="px-2 py-1 text-xs font-medium text-indigo-600 dark:text-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-950/60 rounded transition-colors inline-flex items-center gap-1"
                    >
                      <Radio className="w-3.5 h-3.5" />
                      调试探针
                    </button>
                    <button
                      onClick={() => { setSelectedServerForConfig(server); setConfigModalOpen(true); }}
                      className="px-2 py-1 text-xs font-medium text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-750 rounded transition-colors inline-flex items-center gap-1 border border-gray-200 dark:border-gray-700"
                    >
                      <Terminal className="w-3.5 h-3.5" />
                      配置
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

            {/* 注入鉴权 Token 凭证栏 */}
            <div className="p-3 bg-gray-50 dark:bg-gray-900/60 rounded-xl border border-gray-200 dark:border-gray-800 flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
              <div className="flex items-center gap-2">
                <Lock className="w-4 h-4 text-indigo-500" />
                <div>
                  <span className="text-xs font-semibold text-gray-800 dark:text-gray-200">
                    鉴权凭证注入 (Authorization Token / API Key)
                  </span>
                  <span className="hidden sm:inline text-[10px] text-gray-400 ml-2">
                    (自动写入客户端配置文件与请求头)
                  </span>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {keys && keys.length > 0 && (
                  <select
                    onChange={(e) => {
                      if (e.target.value) setConfigToken(e.target.value);
                    }}
                    className="px-2 py-1 text-xs rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-850 text-gray-700 dark:text-gray-200"
                    defaultValue=""
                  >
                    <option value="" disabled>选择已发行 Key...</option>
                    {keys.map(k => (
                      <option key={k.key} value={k.key}>
                        {k.name ? `${k.name} (${k.key.slice(0, 10)}...)` : k.key}
                      </option>
                    ))}
                  </select>
                )}
                <input
                  type="text"
                  value={configToken}
                  onChange={(e) => setConfigToken(e.target.value)}
                  placeholder="YOUR_AIROUTE_KEY"
                  className="px-2.5 py-1 text-xs font-mono rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-850 text-gray-900 dark:text-white min-w-[200px]"
                />
              </div>
            </div>

            {/* 客户端选择 Tabs */}
            <div className="flex items-center gap-2 border-b border-gray-200 dark:border-gray-800 pb-2 overflow-x-auto">
              {[
                { id: 'cursor', label: 'Cursor (.cursor/mcp.json)' },
                { id: 'claude', label: 'Claude Desktop' },
                { id: 'cline', label: 'Cline / Roo-Code' },
                { id: 'python', label: 'Python SDK' },
                { id: 'curl', label: 'cURL / 终端 CLI' }
              ].map(c => (
                <button
                  key={c.id}
                  onClick={() => setActiveConfigClient(c.id)}
                  className={`px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-colors ${activeConfigClient === c.id ? 'bg-indigo-600 text-white' : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'}`}
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
                  {activeConfigClient === 'curl' && '终端命令行测试 (自动携带 Authorization Bearer 鉴权头)'}
                </span>
                <button
                  onClick={() => {
                    const code = activeConfigClient === 'cursor' ? getCursorConfig() :
                                 activeConfigClient === 'claude' ? getClaudeConfig() :
                                 activeConfigClient === 'cline' ? getClineConfig() :
                                 activeConfigClient === 'python' ? getPythonSnippet() : getCurlSnippet();
                    handleCopyText(code, 'client-config');
                  }}
                  className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors"
                >
                  {copiedKey === 'client-config' ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                  <span>{copiedKey === 'client-config' ? '已复制' : '复制配置'}</span>
                </button>
              </div>

              <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto border border-gray-800">
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
          <div className="bg-white dark:bg-gray-850 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-750 max-w-5xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
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

            {/* 协议教程与通信原理解析 */}
            <div className="p-3 bg-indigo-50/70 dark:bg-indigo-950/40 border border-indigo-100 dark:border-indigo-900/60 rounded-xl text-xs text-indigo-900 dark:text-indigo-200">
              <div className="flex items-center gap-2 font-semibold mb-1 text-indigo-700 dark:text-indigo-300">
                <BookOpen className="w-4 h-4" />
                <span>MCP 双向通信原理解析与接入教程</span>
              </div>
              <p className="text-[11px] leading-relaxed text-indigo-700/80 dark:text-indigo-300/80">
                <b>标准交互流:</b> 客户端首先发起 <code className="font-mono bg-indigo-100 dark:bg-indigo-900/60 px-1 py-0.5 rounded">GET /mcp/sse</code> 建立长连接握手；随后将左侧的 <b>JSON-RPC 2.0 请求报文</b> 通过 <code className="font-mono bg-indigo-100 dark:bg-indigo-900/60 px-1 py-0.5 rounded">POST /mcp/messages</code> 投递至网关。网关安全审计并调度工具后，将右侧的 <b>响应结果</b> 实时返回给客户端。
              </p>
            </div>

            {/* 鉴权 Token 凭证与目标服务配置栏 */}
            <div className="p-3 bg-gray-50 dark:bg-gray-900/60 rounded-xl border border-gray-200 dark:border-gray-800 space-y-2.5">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <Lock className="w-4 h-4 text-indigo-500" />
                  <span className="text-xs font-semibold text-gray-800 dark:text-gray-200">
                    鉴权凭证 (Authorization Token / 虚拟 API Key)
                  </span>
                  <span className="text-[10px] text-gray-400">
                    (支持 Bearer Token 与 X-API-Key 多租户隔离)
                  </span>
                </div>
                {keys && keys.length > 0 && (
                  <div className="flex items-center gap-1.5 text-xs">
                    <span className="text-[11px] text-gray-400">快捷填充:</span>
                    <select
                      onChange={(e) => {
                        if (e.target.value) setProbeToken(e.target.value);
                      }}
                      className="px-2 py-1 text-[11px] rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-200"
                      defaultValue=""
                    >
                      <option value="" disabled>选择网关已发行 Key...</option>
                      {keys.map(k => (
                        <option key={k.key} value={k.key}>
                          {k.name ? `${k.name} (${k.key.slice(0, 10)}...)` : k.key}
                        </option>
                      ))}
                    </select>
                  </div>
                )}
              </div>
              <input
                type="text"
                value={probeToken}
                onChange={(e) => setProbeToken(e.target.value)}
                placeholder="输入虚拟 API Key 或管理员 Token (如 sk-airoute-...)"
                className="w-full px-3 py-1.5 font-mono text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-850 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-indigo-500"
              />
            </div>

            {/* 方法、服务与参数选择 */}
            <div className="space-y-3">
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                    目标 MCP 服务 (Target Server)
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
                    className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white"
                  >
                    <option value="all">🌐 统一 MCP 聚合网关 (全部已启用服务)</option>
                    {servers.map(s => (
                      <option key={s.id} value={s.id}>
                        {s.name} ({s.id})
                      </option>
                    ))}
                  </select>
                </div>

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
                      onChange={(e) => handleSelectTool(e.target.value)}
                      className="w-full px-3 py-2 text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-white font-mono"
                    >
                      {/* 如果当前指定了服务，将其工具置顶 */}
                      {probeServer && probeServer.tools && probeServer.tools.length > 0 && (
                        <optgroup label={`★ 当前服务 [${probeServer.name || probeServer.id}] 工具集`}>
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
                        <optgroup key={grp} label={grp}>
                          {toolList.map(t => (
                            <option key={t.name} value={t.name}>
                              {t.label}
                            </option>
                          ))}
                        </optgroup>
                      ))}

                      {/* 容底项：若选中工具不在模板库中 */}
                      {!TOOL_TEMPLATES[probeToolName] && (!probeServer?.tools?.includes(probeToolName)) && (
                        <optgroup label="自定义探针工具">
                          <option value={probeToolName}>{probeToolName} (当前选定工具)</option>
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
                      工具输入参数 (JSON Arguments)
                    </label>
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <button
                        type="button"
                        onClick={() => handleSelectTool('airoute_data_redact')}
                        className="text-[10px] text-indigo-600 dark:text-indigo-400 hover:underline"
                      >
                        敏感脱敏
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => handleSelectTool('airoute_sql_security_check')}
                        className="text-[10px] text-rose-600 dark:text-rose-400 hover:underline"
                      >
                        SQL拦截
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => handleSelectTool('puppeteer_navigate')}
                        className="text-[10px] text-emerald-600 dark:text-emerald-400 hover:underline"
                      >
                        Puppeteer导航
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => handleSelectTool('read_query')}
                        className="text-[10px] text-amber-600 dark:text-amber-400 hover:underline"
                      >
                        只读SQL
                      </button>
                      <span className="text-gray-300 dark:text-gray-600">|</span>
                      <button
                        type="button"
                        onClick={() => handleSelectTool('sequentialthinking')}
                        className="text-[10px] text-purple-600 dark:text-purple-400 hover:underline"
                      >
                        深度推理
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

              {/* 双向报文链路对比: 发送报文 (左) 与 响应结果 (右) */}
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 pt-2">
                {/* 1. 发送请求报文 */}
                <div className="flex flex-col space-y-1.5">
                  <div className="flex items-center justify-between text-xs font-semibold text-gray-700 dark:text-gray-300">
                    <div className="flex items-center gap-1.5">
                      <span className="flex items-center gap-1 text-indigo-600 dark:text-indigo-400">
                        <Send className="w-3.5 h-3.5" />
                        发送请求报文
                      </span>
                      <div className="flex items-center bg-gray-100 dark:bg-gray-800 rounded-lg p-0.5 ml-1 text-[10px]">
                        <button
                          type="button"
                          onClick={() => setRequestViewTab('wire')}
                          className={`px-1.5 py-0.5 rounded ${requestViewTab === 'wire' ? 'bg-white dark:bg-gray-700 text-indigo-600 dark:text-indigo-300 shadow-2xs font-medium' : 'text-gray-500 hover:text-gray-800'}`}
                        >
                          HTTP报文
                        </button>
                        <button
                          type="button"
                          onClick={() => setRequestViewTab('json')}
                          className={`px-1.5 py-0.5 rounded ${requestViewTab === 'json' ? 'bg-white dark:bg-gray-700 text-indigo-600 dark:text-indigo-300 shadow-2xs font-medium' : 'text-gray-500 hover:text-gray-800'}`}
                        >
                          JSON-RPC
                        </button>
                        <button
                          type="button"
                          onClick={() => setRequestViewTab('curl')}
                          className={`px-1.5 py-0.5 rounded ${requestViewTab === 'curl' ? 'bg-white dark:bg-gray-700 text-indigo-600 dark:text-indigo-300 shadow-2xs font-medium' : 'text-gray-500 hover:text-gray-800'}`}
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
                      className="inline-flex items-center gap-1 text-[11px] font-normal text-gray-500 hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors"
                    >
                      {copiedKey === 'probe-req' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                      复制请求
                    </button>
                  </div>
                  <div className="bg-gray-900 text-gray-100 p-3.5 rounded-xl font-mono text-xs overflow-x-auto min-h-[170px] max-h-64 border border-gray-800 shadow-inner">
                    <pre className="whitespace-pre-wrap leading-relaxed">
                      {requestViewTab === 'wire' && getWireRequestText()}
                      {requestViewTab === 'json' && JSON.stringify(lastSentPayload || getCurrentRpcPayload(), null, 2)}
                      {requestViewTab === 'curl' && getCurlRequestText()}
                    </pre>
                  </div>
                  <div className="text-[11px] text-gray-400 flex items-center justify-between">
                    <span>传输端点: POST {messagesUrl}</span>
                    <span>鉴权: {probeToken ? 'Bearer Token (已配置)' : '未配置 Token'}</span>
                  </div>
                </div>

                {/* 2. 网关接收响应 */}
                <div className="flex flex-col space-y-1.5">
                  <div className="flex items-center justify-between text-xs font-semibold text-gray-700 dark:text-gray-300">
                    <span className="flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
                      <CheckCircle2 className="w-3.5 h-3.5" />
                      接收响应 (Response Payload: HTTP 200 OK)
                    </span>
                    {probeResult && (
                      <button
                        type="button"
                        onClick={() => handleCopyText(JSON.stringify(probeResult, null, 2), 'probe-res')}
                        className="inline-flex items-center gap-1 text-[11px] font-normal text-gray-500 hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors"
                      >
                        {copiedKey === 'probe-res' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                        复制响应
                      </button>
                    )}
                  </div>
                  <div className="bg-gray-900 text-gray-100 p-3.5 rounded-xl font-mono text-xs overflow-x-auto min-h-[170px] max-h-64 border border-gray-800 shadow-inner flex flex-col justify-start">
                    {probeResult ? (
                      <pre className="whitespace-pre-wrap leading-relaxed">{JSON.stringify(probeResult, null, 2)}</pre>
                    ) : (
                      <div className="h-full flex flex-col items-center justify-center text-gray-500 text-center py-8">
                        <Terminal className="w-8 h-8 mb-2 opacity-40 text-indigo-400" />
                        <p className="text-xs font-medium text-gray-400">等待发送探针</p>
                        <p className="text-[11px] text-gray-500 mt-1">点击上方“发送 JSON-RPC 2.0 报文”发起实时调用测试</p>
                      </div>
                    )}
                  </div>
                  <div className="text-[11px] text-gray-400 flex items-center justify-between">
                    <span>网关延迟: {probeLatency !== null ? `${probeLatency} ms` : '--'}</span>
                    <span>状态: {probeResult ? (probeResult.error ? 'RPC 异常' : '成功返回') : '待调用'}</span>
                  </div>
                </div>
              </div>
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
