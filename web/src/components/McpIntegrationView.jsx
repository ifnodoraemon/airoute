import React, { useState, useEffect } from 'react';
import {
  Terminal,
  Check,
  Copy,
  Bot,
  Play,
  RefreshCw,
  Server,
  Globe,
  Clock,
  Calculator,
  Search,
  FileText,
  X,
  Layers,
  ArrowRight,
  ShieldCheck,
  CheckCircle2
} from 'lucide-react';

export default function McpIntegrationView({ adminFetch, onCopy, showToast }) {
  const [activeConfigTab, setActiveConfigTab] = useState('cli');
  const [copiedKey, setCopiedKey] = useState('');
  const [testLoading, setTestLoading] = useState(false);
  const [testOutput, setTestOutput] = useState(null);

  // Plaza Filter & Search
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');

  // Manifest Modal
  const [manifestSkill, setManifestSkill] = useState(null);

  // MCP Master Switch & Skills State
  const [mcpSettings, setMcpSettings] = useState({
    mcp_enabled: true,
    enabled_skills_count: 5,
    active_tools_count: 11
  });
  const [skills, setSkills] = useState([]);
  const [loadingSkills, setLoadingSkills] = useState(false);
  const [togglingSkill, setTogglingSkill] = useState({});
  const [togglingMcp, setTogglingMcp] = useState(false);

  const origin = window.location.origin || 'http://localhost:8080';
  const sseUrl = `${origin}/mcp/sse`;
  const messagesUrl = `${origin}/mcp/messages`;

  const fetchSkillsAndSettings = async () => {
    if (!adminFetch) return;
    setLoadingSkills(true);
    try {
      // 1. Fetch MCP Settings
      const setRes = await adminFetch('/api/v1/admin/mcp/settings');
      if (setRes.ok) {
        const setData = await setRes.json();
        if (setData.code === 0 && setData.data) {
          setMcpSettings(setData.data);
        }
      }

      // 2. Fetch Skills list
      const skRes = await adminFetch('/api/v1/admin/skills');
      if (skRes.ok) {
        const skData = await skRes.json();
        if (skData.code === 0 && skData.data) {
          setSkills(skData.data);
        }
      }
    } catch (e) {
      console.warn('Failed to load skills:', e);
    } finally {
      setLoadingSkills(false);
    }
  };

  useEffect(() => {
    fetchSkillsAndSettings();
  }, []);

  const handleToggleSkill = async (skillId, currentEnabled) => {
    if (!adminFetch) return;
    setTogglingSkill(prev => ({ ...prev, [skillId]: true }));
    try {
      const res = await adminFetch(`/api/v1/admin/skills/${skillId}/toggle`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !currentEnabled })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setSkills(prev => prev.map(s => s.id === skillId ? { ...s, enabled: !currentEnabled } : s));
        if (showToast) showToast(`技能 [${skillId}] 已${!currentEnabled ? '开启' : '关闭'}`, 'success');
        fetchSkillsAndSettings();
      } else {
        if (showToast) showToast(data.error || '切换技能失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    } finally {
      setTogglingSkill(prev => ({ ...prev, [skillId]: false }));
    }
  };

  const handleToggleMcpMaster = async () => {
    if (!adminFetch) return;
    const nextState = !mcpSettings.mcp_enabled;
    setTogglingMcp(true);
    try {
      const res = await adminFetch('/api/v1/admin/mcp/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mcp_enabled: nextState })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setMcpSettings(prev => ({ ...prev, mcp_enabled: nextState }));
        if (showToast) showToast(`MCP 服务已${nextState ? '开启' : '停用'}`, 'success');
      } else {
        if (showToast) showToast(data.error || '更新失败', 'error');
      }
    } catch (err) {
      if (showToast) showToast('请求异常: ' + err.message, 'error');
    } finally {
      setTogglingMcp(false);
    }
  };

  const handleCopy = (text, key) => {
    if (onCopy) {
      onCopy(text);
    } else {
      navigator.clipboard.writeText(text);
    }
    setCopiedKey(key);
    if (showToast) showToast('已复制到剪贴板', 'success');
    setTimeout(() => setCopiedKey(''), 2000);
  };

  const cliSnippet = `# 1. 编译或安装极简 CLI
go build -o /usr/local/bin/nano ./cmd/nano

# 2. 查看集群与模型状态
nano status
nano models

# 3. 命令行按需开启 / 关闭技能
nano skills list
nano skills enable web_search
nano skills disable web_search

# 4. 终端直接与模型对话
nano chat -m deepseek-chat "你好，请自我介绍"

# 5. 启动 Claude Desktop / Cursor 本地 stdio 桥接
nano mcp stdio`;

  const cursorConfig = JSON.stringify({
    mcpServers: {
      router: {
        url: sseUrl
      }
    }
  }, null, 2);

  const claudeConfig = JSON.stringify({
    mcpServers: {
      router: {
        command: "nano",
        args: ["mcp", "stdio"]
      }
    }
  }, null, 2);

  const clineConfig = JSON.stringify({
    mcpServers: {
      router: {
        url: sseUrl,
        disabled: false,
        autoApprove: ["airoute_search_skills", "airoute_inspect_skill", "airoute_get_skill_manifest"]
      }
    }
  }, null, 2);

  const pythonSnippet = `import asyncio
from mcp import ClientSession
from mcp.client.sse import sse_client

async def main():
    # 连接 AI 路由器的 MCP 渐进式服务
    async with sse_client("${sseUrl}") as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()

            # 阶段 1：搜索技能 (仅消耗 ~30 tokens，避免上下文臃肿)
            found = await session.call_tool("airoute_search_skills", {"query": "计算"})
            print("1. 搜索匹配技能:\\n", found)

            # 阶段 2：确认单个技能签名与触发条件 (~60 tokens)
            inspected = await session.call_tool("airoute_inspect_skill", {"skill_id": "code_runner"})
            print("2. 确认技能规范:\\n", inspected)

            # 阶段 3：按需拉取完整 SKILL.md 规范与指令
            manifest = await session.call_tool("airoute_get_skill_manifest", {"skill_id": "code_runner"})
            print("3. 完整指令清单:\\n", manifest)

            # 阶段 4：执行具体工具调用
            res = await session.call_tool("airoute_calc_eval", {"expression": "(128 * 1024) / 0.85"})
            print("4. 计算执行结果:", res)

asyncio.run(main())`;

  const runTestMcp = async (toolName = 'airoute_list_models', args = {}, customMethod = 'tools/call') => {
    setTestLoading(true);
    setTestOutput(null);
    try {
      const payload = {
        jsonrpc: '2.0',
        id: Date.now(),
        method: customMethod,
        params: customMethod === 'server/discover' ? {} : {
          name: toolName,
          arguments: args
        }
      };
      const res = await fetch(messagesUrl, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'MCP-Protocol-Version': '2026-07-28'
        },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      setTestOutput(data);
      const actionName = customMethod === 'server/discover' ? 'server/discover' : toolName;
      if (showToast) showToast(`探针测试 [${actionName}] 执行完成`, 'success');
    } catch (err) {
      setTestOutput({ error: err.message });
      if (showToast) showToast('探针请求异常: ' + err.message, 'error');
    } finally {
      setTestLoading(false);
    }
  };

  const getSkillIcon = (id) => {
    switch (id) {
      case 'web_search': return <Globe className="w-5 h-5 text-sky-500" />;
      case 'datetime_clock': return <Clock className="w-5 h-5 text-indigo-500" />;
      case 'code_runner': return <Calculator className="w-5 h-5 text-amber-500" />;
      case 'model_router': return <Bot className="w-5 h-5 text-purple-500" />;
      default: return <Server className="w-5 h-5 text-emerald-500" />;
    }
  };

  const categories = [
    { id: 'all', label: '全部' },
    { id: 'ops', label: '运维治理' },
    { id: 'agent', label: '智能协作' },
    { id: 'search', label: '信息检索' },
    { id: 'utility', label: '通用工具' }
  ];

  // Filter skills based on Category & Search Query
  const filteredSkills = skills.filter(s => {
    if (selectedCategory !== 'all' && s.category !== selectedCategory) {
      return false;
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      const matchName = s.name?.toLowerCase().includes(q);
      const matchId = s.id?.toLowerCase().includes(q);
      const matchDesc = s.description?.toLowerCase().includes(q);
      const matchTools = s.tools?.some(t => t.toLowerCase().includes(q));
      return matchName || matchId || matchDesc || matchTools;
    }
    return true;
  });

  return (
    <div className="space-y-8 animate-in fade-in pb-16">
      {/* 1. Header & Master Controls */}
      <div className="bg-white border border-slate-200/80 rounded-3xl p-7 shadow-xs">
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-6">
            <div className="flex items-center space-x-3.5">
              <div className="w-10 h-10 rounded-2xl bg-indigo-50 border border-indigo-200 flex items-center justify-center text-indigo-600 font-bold shrink-0">
                <Bot className="w-5 h-5" />
              </div>
              <div>
                <div className="flex items-center space-x-2.5">
                  <h2 className="text-xl font-bold text-slate-900 tracking-tight">
                    扩展广场
                  </h2>
                  <span className="px-2.5 py-0.5 rounded-full text-[10px] font-semibold bg-indigo-50 text-indigo-700 border border-indigo-200">
                    MCP 2026-07-28
                  </span>
                </div>
              </div>
            </div>

          {/* Master Switch */}
          <div className="flex items-center space-x-4 bg-slate-50 border border-slate-200 px-5 py-3 rounded-2xl">
            <div className="text-right">
              <span className="text-xs font-bold text-slate-800 block">
                MCP 服务
              </span>
              <span className="text-[11px] font-mono text-slate-500">
                {mcpSettings.mcp_enabled ? (
                  <span className="text-emerald-600 font-semibold">● 运行中</span>
                ) : (
                  <span className="text-slate-400">○ 已停用</span>
                )}
              </span>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                disabled={togglingMcp}
                checked={mcpSettings.mcp_enabled}
                onChange={handleToggleMcpMaster}
                className="sr-only peer"
              />
              <div className="w-10 h-5 bg-slate-300 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-indigo-600"></div>
            </label>
          </div>
        </div>
      </div>

      {/* 2. Plaza Filter Pills & Search Box */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex flex-wrap items-center gap-1.5 bg-slate-100/90 p-1.5 rounded-2xl w-fit">
          {categories.map(c => (
            <button
              key={c.id}
              onClick={() => setSelectedCategory(c.id)}
              className={`px-4 py-2 rounded-xl text-xs font-medium transition cursor-pointer ${
                selectedCategory === c.id
                  ? 'bg-white text-indigo-700 shadow-xs font-semibold'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              {c.label}
            </button>
          ))}
        </div>

        <div className="relative w-full sm:w-72">
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            placeholder="搜索技能名称或工具..."
            className="w-full pl-9 pr-4 py-2 bg-white border border-slate-200 rounded-xl text-xs text-slate-800 placeholder-slate-400 focus:outline-none focus:border-indigo-500"
          />
        </div>
      </div>

      {/* 3. Skill Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {filteredSkills.map((skill) => {
          const isToggling = togglingSkill[skill.id];
          return (
            <div
              key={skill.id}
              className={`p-6 rounded-3xl border transition flex flex-col justify-between ${
                skill.enabled
                  ? 'bg-white border-slate-200/90 shadow-xs hover:border-indigo-300'
                  : 'bg-slate-50/70 border-slate-200/60 opacity-60'
              }`}
            >
              <div>
                <div className="flex items-start justify-between gap-4">
                  <div className="flex items-center space-x-3.5">
                    <div className="w-11 h-11 rounded-2xl bg-slate-50 border border-slate-200 flex items-center justify-center shrink-0">
                      {getSkillIcon(skill.id)}
                    </div>
                    <div>
                      <h4 className="font-bold text-sm text-slate-900">
                        {skill.name}
                      </h4>
                      <div className="flex items-center space-x-2 mt-0.5">
                        <span className="text-[11px] text-slate-400 font-mono">
                          {skill.id}
                        </span>
                        <span className="text-[11px] text-slate-400 font-mono">
                          v{skill.version || '1.0.0'}
                        </span>
                      </div>
                    </div>
                  </div>

                  {/* Switch */}
                  <label className="relative inline-flex items-center cursor-pointer shrink-0">
                    <input
                      type="checkbox"
                      disabled={isToggling}
                      checked={skill.enabled}
                      onChange={() => handleToggleSkill(skill.id, skill.enabled)}
                      className="sr-only peer"
                    />
                    <div className="w-9 h-5 bg-slate-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-indigo-600"></div>
                  </label>
                </div>

                {/* Badges */}
                <div className="mt-3.5 flex items-center space-x-2">
                  <span className={`px-2.5 py-0.5 rounded-lg text-[10px] font-medium ${
                    skill.loading_mode === 'lazy'
                      ? 'bg-sky-50 text-sky-700 border border-sky-200/80'
                      : 'bg-amber-50 text-amber-700 border border-amber-200/80'
                  }`}>
                    {skill.loading_mode === 'lazy' ? '渐进式 (按需加载)' : '即时 (全量)'}
                  </span>
                  <span className="px-2.5 py-0.5 rounded-lg text-[10px] font-medium bg-slate-100 text-slate-600">
                    {categories.find(c => c.id === skill.category)?.label || skill.category}
                  </span>
                </div>

                <p className="text-xs text-slate-600 mt-3 line-clamp-2 leading-relaxed">
                  {skill.description}
                </p>

                {/* Tools tags */}
                <div className="mt-4 flex flex-wrap gap-1.5 items-center">
                  {skill.tools?.map(t => (
                    <span
                      key={t}
                      className={`px-2.5 py-0.5 rounded-lg text-[11px] font-mono font-medium ${
                        skill.enabled
                          ? 'bg-indigo-50/70 text-indigo-700 border border-indigo-100'
                          : 'bg-slate-200/70 text-slate-500'
                      }`}
                    >
                      {t}
                    </span>
                  ))}
                </div>
              </div>

              {/* Bottom action: View Manifest */}
              <div className="mt-5 pt-3.5 border-t border-slate-100 flex items-center justify-between">
                <span className="text-[11px] text-slate-400">
                  {skill.author || '官方发布'}
                </span>
                <button
                  type="button"
                  onClick={() => setManifestSkill(skill)}
                  className="text-xs text-indigo-600 hover:text-indigo-800 font-medium flex items-center space-x-1 cursor-pointer"
                >
                  <FileText className="w-3.5 h-3.5" />
                  <span>查看规范清单</span>
                </button>
              </div>
            </div>
          );
        })}
      </div>

      {filteredSkills.length === 0 && (
        <div className="text-center py-16 bg-white rounded-3xl border border-slate-200 text-slate-400 text-xs">
          没有找到匹配的技能或工具
        </div>
      )}

      {/* 4. Client Integration Tabs */}
      <div className="bg-white border border-slate-200/80 rounded-3xl p-7 shadow-xs space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-slate-100 pb-4">
          <div className="flex items-center space-x-2">
            <Terminal className="w-4 h-4 text-indigo-600" />
            <h3 className="font-bold text-sm text-slate-900">
              客户端接入
            </h3>
          </div>

          {/* Config Tabs */}
          <div className="flex items-center space-x-1 bg-slate-100 p-1 rounded-2xl text-xs">
            <button
              onClick={() => setActiveConfigTab('cli')}
              className={`px-3.5 py-1.5 rounded-xl font-semibold transition cursor-pointer ${
                activeConfigTab === 'cli'
                  ? 'bg-white text-indigo-600 shadow-xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              CLI 终端
            </button>
            <button
              onClick={() => setActiveConfigTab('claude')}
              className={`px-3.5 py-1.5 rounded-xl font-semibold transition cursor-pointer ${
                activeConfigTab === 'claude'
                  ? 'bg-white text-indigo-600 shadow-xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              Claude Desktop
            </button>
            <button
              onClick={() => setActiveConfigTab('cursor')}
              className={`px-3.5 py-1.5 rounded-xl font-semibold transition cursor-pointer ${
                activeConfigTab === 'cursor'
                  ? 'bg-white text-indigo-600 shadow-xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              Cursor
            </button>
            <button
              onClick={() => setActiveConfigTab('cline')}
              className={`px-3.5 py-1.5 rounded-xl font-semibold transition cursor-pointer ${
                activeConfigTab === 'cline'
                  ? 'bg-white text-indigo-600 shadow-xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              Cline
            </button>
            <button
              onClick={() => setActiveConfigTab('python')}
              className={`px-3.5 py-1.5 rounded-xl font-semibold transition cursor-pointer ${
                activeConfigTab === 'python'
                  ? 'bg-white text-indigo-600 shadow-xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              Python SDK
            </button>
          </div>
        </div>

        {/* Tab Contents */}
        {activeConfigTab === 'cli' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="text-slate-500">
                极简命令行：查看状态、模型、技能按需控制与本地 stdio 桥接
              </span>
              <button
                onClick={() => handleCopy(cliSnippet, 'cli')}
                className="px-2.5 py-1 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold transition flex items-center space-x-1 cursor-pointer"
              >
                {copiedKey === 'cli' ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{copiedKey === 'cli' ? '已复制' : '复制命令'}</span>
              </button>
            </div>
            <pre className="p-4 bg-slate-900 text-slate-100 rounded-2xl text-xs font-mono overflow-x-auto leading-relaxed">
              {cliSnippet}
            </pre>
          </div>
        )}

        {activeConfigTab === 'claude' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="text-slate-500">
                配置 Claude Desktop (claude_desktop_config.json):
              </span>
              <button
                onClick={() => handleCopy(claudeConfig, 'claude')}
                className="px-2.5 py-1 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold transition flex items-center space-x-1 cursor-pointer"
              >
                {copiedKey === 'claude' ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{copiedKey === 'claude' ? '已复制' : '复制 JSON'}</span>
              </button>
            </div>
            <pre className="p-4 bg-slate-900 text-slate-100 rounded-2xl text-xs font-mono overflow-x-auto">
              {claudeConfig}
            </pre>
          </div>
        )}

        {activeConfigTab === 'cursor' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="text-slate-500">
                Cursor 设置 (MCP Server SSE 模式):
              </span>
              <button
                onClick={() => handleCopy(cursorConfig, 'cursor')}
                className="px-2.5 py-1 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold transition flex items-center space-x-1 cursor-pointer"
              >
                {copiedKey === 'cursor' ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{copiedKey === 'cursor' ? '已复制' : '复制 JSON'}</span>
              </button>
            </div>
            <pre className="p-4 bg-slate-900 text-slate-100 rounded-2xl text-xs font-mono overflow-x-auto">
              {cursorConfig}
            </pre>
          </div>
        )}

        {activeConfigTab === 'cline' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="text-slate-500">Cline 插件配置:</span>
              <button
                onClick={() => handleCopy(clineConfig, 'cline')}
                className="px-2.5 py-1 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold transition flex items-center space-x-1 cursor-pointer"
              >
                {copiedKey === 'cline' ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{copiedKey === 'cline' ? '已复制' : '复制 JSON'}</span>
              </button>
            </div>
            <pre className="p-4 bg-slate-900 text-slate-100 rounded-2xl text-xs font-mono overflow-x-auto">
              {clineConfig}
            </pre>
          </div>
        )}

        {activeConfigTab === 'python' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="text-slate-500">Python mcp SDK 渐进式三阶段调用范式:</span>
              <button
                onClick={() => handleCopy(pythonSnippet, 'python')}
                className="px-2.5 py-1 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold transition flex items-center space-x-1 cursor-pointer"
              >
                {copiedKey === 'python' ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{copiedKey === 'python' ? '已复制' : '复制代码'}</span>
              </button>
            </div>
            <pre className="p-4 bg-slate-900 text-slate-100 rounded-2xl text-xs font-mono overflow-x-auto">
              {pythonSnippet}
            </pre>
          </div>
        )}
      </div>

      {/* 5. Interactive 3-Stage Probe */}
      <div className="bg-white border border-slate-200/80 rounded-3xl p-7 shadow-xs space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <Play className="w-4 h-4 text-emerald-600" />
            <h3 className="font-bold text-sm text-slate-900">
              在线探针演练 (三阶段渐进式验证)
            </h3>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2.5">
          <button
            onClick={() => runTestMcp('', {}, 'server/discover')}
            disabled={testLoading}
            className="px-3.5 py-2 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-xl text-xs font-mono font-semibold transition cursor-pointer"
          >
            <span>0. 协议发现: server/discover (2026 最新规范)</span>
          </button>
          <button
            onClick={() => runTestMcp('airoute_search_skills', { query: '计算' })}
            disabled={testLoading}
            className="px-3.5 py-2 bg-indigo-50 hover:bg-indigo-100 text-indigo-700 rounded-xl text-xs font-mono font-medium transition cursor-pointer"
          >
            <span>1. 搜索技能: airoute_search_skills("计算")</span>
          </button>
          <button
            onClick={() => runTestMcp('airoute_inspect_skill', { skill_id: 'code_runner' })}
            disabled={testLoading}
            className="px-3.5 py-2 bg-sky-50 hover:bg-sky-100 text-sky-700 rounded-xl text-xs font-mono font-medium transition cursor-pointer"
          >
            <span>2. 确认技能: airoute_inspect_skill("code_runner")</span>
          </button>
          <button
            onClick={() => runTestMcp('airoute_get_skill_manifest', { skill_id: 'code_runner' })}
            disabled={testLoading}
            className="px-3.5 py-2 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 rounded-xl text-xs font-mono font-medium transition cursor-pointer"
          >
            <span>3. 拉取规范: airoute_get_skill_manifest("code_runner")</span>
          </button>
          <button
            onClick={() => runTestMcp('airoute_calc_eval', { expression: '(128 * 1024) / 0.85' })}
            disabled={testLoading}
            className="px-3.5 py-2 bg-slate-100 hover:bg-slate-200 text-slate-800 rounded-xl text-xs font-mono transition cursor-pointer"
          >
            <span>4. 执行计算: airoute_calc_eval("(128*1024)/0.85")</span>
          </button>
        </div>

        {testOutput && (
          <div className="space-y-1.5 animate-in fade-in">
            <div className="flex items-center justify-between text-xs text-slate-500">
              <span>探针执行结果:</span>
              <button
                onClick={() => setTestOutput(null)}
                className="hover:underline text-[11px] cursor-pointer"
              >
                清除输出
              </button>
            </div>
            <pre className="p-4 bg-slate-950 text-emerald-400 rounded-2xl text-xs font-mono overflow-x-auto max-h-60 overflow-y-auto">
              {JSON.stringify(testOutput, null, 2)}
            </pre>
          </div>
        )}
      </div>

      {/* Manifest Modal */}
      {manifestSkill && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-xs animate-in fade-in">
          <div className="bg-white rounded-3xl max-w-2xl w-full p-6 shadow-2xl border border-slate-200 space-y-4 max-h-[85vh] flex flex-col">
            <div className="flex items-center justify-between border-b border-slate-100 pb-3">
              <div className="flex items-center space-x-2.5">
                <FileText className="w-5 h-5 text-indigo-600" />
                <div>
                  <h3 className="font-bold text-sm text-slate-900">
                    {manifestSkill.name} - 规范清单 (SKILL.md)
                  </h3>
                  <span className="text-[11px] text-slate-400 font-mono">
                    ID: {manifestSkill.id} · v{manifestSkill.version || '1.0.0'} · {manifestSkill.loading_mode === 'lazy' ? '渐进式加载' : '即时加载'}
                  </span>
                </div>
              </div>
              <button
                onClick={() => setManifestSkill(null)}
                className="p-1 rounded-xl hover:bg-slate-100 text-slate-400 hover:text-slate-600 transition cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto">
              <pre className="p-4 bg-slate-900 text-slate-100 rounded-2xl text-xs font-mono whitespace-pre-wrap leading-relaxed">
                {manifestSkill.manifest || `# ${manifestSkill.name}\n\n${manifestSkill.description}`}
              </pre>
            </div>

            <div className="flex items-center justify-between pt-2 border-t border-slate-100">
              <button
                onClick={() => handleCopy(manifestSkill.manifest || manifestSkill.description, 'manifest')}
                className="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-800 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition cursor-pointer"
              >
                {copiedKey === 'manifest' ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{copiedKey === 'manifest' ? '已复制' : '复制规范清单'}</span>
              </button>
              <button
                onClick={() => setManifestSkill(null)}
                className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold transition cursor-pointer"
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
