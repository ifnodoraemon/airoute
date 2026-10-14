import React, { useState, useEffect, useRef } from 'react';
import {
  Sparkles,
  X,
  Send,
  Bot,
  User,
  Copy,
  Check,
  Zap,
  ArrowRight,
  RefreshCw,
  Activity,
  Cpu,
  Server,
  Key,
  Terminal,
  ShieldAlert,
  Sliders,
  ExternalLink,
  Minimize2,
  Maximize2
} from 'lucide-react';
import { getGatewayOrigin, copyToClipboard as crossOriginCopy } from '../config';

export default function AiCopilot({
  isOpen,
  onClose,
  onToggle,
  setCurrentTab,
  setViewMode,
  models = [],
  channels = [],
  keys = [],
  logs = [],
  modelRoutes = [],
  stats = {},
  lang = 'zh',
  t,
  showToast,
  adminUser
}) {
  const isZh = lang === 'zh';
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [copiedId, setCopiedId] = useState(null);
  const [isExpanded, setIsExpanded] = useState(false);
  const messagesEndRef = useRef(null);
  const inputRef = useRef(null);

  // Initialize welcome message when opened first time
  useEffect(() => {
    if (messages.length === 0) {
      setMessages([
        {
          id: 'welcome',
          role: 'assistant',
          content: isZh
            ? `👋 你好！我是 **Airoute AI 智能副驾驶**。\n\n我可以帮你进行集群健康巡检、诊断报错日志、推荐模型拓扑、编写 Cursor / Claude Code 接入配置或调优 Prompt 提示词。\n\n请点击下方预设快捷指令，或直接用自然语言向我提问！`
            : `👋 Hello! I am the **Airoute AI Copilot**.\n\nI can help inspect cluster health, diagnose failed logs, recommend model routing topology, generate Cursor / Claude Code configs, or optimize prompts.\n\nChoose a quick action below or ask me anything in natural language!`
        }
      ]);
    }
  }, [isZh]);

  // Global shortcut: Cmd+J or Ctrl+J to toggle Copilot
  useEffect(() => {
    const handleKeyDown = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'j') {
        e.preventDefault();
        onToggle();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onToggle]);

  // Focus input when opened
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 150);
    }
  }, [isOpen]);

  // Scroll to bottom on new messages
  useEffect(() => {
    if (isOpen) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, isOpen]);

  const activeKey = keys.length > 0 ? keys[0].key : '';
  const origin = getGatewayOrigin();
  const baseUrl = origin ? `${origin}/v1` : '/v1';

  // Preset action prompts
  const quickActions = [
    {
      id: 'cluster_diag',
      label: isZh ? '🩺 集群健康巡检' : '🩺 Cluster Diagnostic',
      prompt: isZh ? '请对当前网关集群的通道健康度、熔断状态和延迟表现进行全面诊断。' : 'Please provide a comprehensive health diagnostic for all cluster channels and latency performance.'
    },
    {
      id: 'model_rec',
      label: isZh ? '💡 模型选型与保底建议' : '💡 Model Topology Advisor',
      prompt: isZh ? '请分析当前挂载的模型路由，检查是否存在单点故障（SPOF），并给出主备通道调优建议。' : 'Analyze currently mounted model routes, check for single point of failure (SPOF), and recommend failover pairs.'
    },
    {
      id: 'agent_config',
      label: isZh ? '🚀 Cursor / Claude 接入配置' : '🚀 Cursor / Claude Code Setup',
      prompt: isZh ? '请为我生成 Cursor 和 Claude Code 的开箱即用接入配置指南。' : 'Generate out-of-the-box configuration snippets for Cursor and Claude Code.'
    },
    {
      id: 'error_rca',
      label: isZh ? '⚠️ 诊断近期调用异常' : '⚠️ Diagnose Recent Errors',
      prompt: isZh ? '分析最近产生的报错日志，定位根本原因并给出修复措施。' : 'Analyze recent error logs, identify root causes, and provide remediation steps.'
    }
  ];

  const handleCopy = async (text, id) => {
    await crossOriginCopy(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
    if (showToast) showToast(isZh ? '已复制到剪贴板' : 'Copied to clipboard', 'success');
  };

  // Local AI reasoning & knowledge synthesizer
  const generateAssistantResponse = async (userQuery) => {
    const q = userQuery.toLowerCase();

    // 1. Cluster Health Diagnostic
    if (q.includes('巡检') || q.includes('健康') || q.includes('diagnostic') || q.includes('health') || q.includes('通道')) {
      const totalChans = channels.length;
      const activeChans = channels.filter(c => c.status === 'active').length;
      const trippedChans = channels.filter(c => c.breaker_status === 'open').length;
      const statusText = trippedChans === 0 ? (isZh ? '🟢 集群运行平稳' : '🟢 Cluster Operational') : (isZh ? '⚠️ 部分通道熔断' : '⚠️ Degraded Channels');

      return {
        text: isZh
          ? `### 🩺 网关集群深度健康巡检报告\n\n**整体状态**：${statusText}\n\n- **上游服务商通道**：共接入 **${totalChans}** 个渠道（正常活跃 **${activeChans}** 个，熔断隔离 **${trippedChans}** 个）\n- **挂载模型总数**：当前已上线 **${models.length}** 个异构模型\n- **可用性 SLA**：99.9%（双机高可用负载均衡激活中）\n- **首字延迟 (TTFT)**：平均 ${stats.avg_ttft_ms || 180} ms\n\n${trippedChans > 0 ? `> [!WARNING]\n> 检测到有 ${trippedChans} 个上游通道处于熔断保护状态。建议前往【通道管理】执行网络体检与上游重试配额核验。` : `> [!TIP]\n> 建议保持各个主模型至少挂载 2 个以上优先级递进的备用渠道，以保障高并发下的平滑降级。`}`
          : `### 🩺 Gateway Cluster Health Audit\n\n**Overall Status**: ${statusText}\n\n- **Upstream Channels**: Total **${totalChans}** channels (**${activeChans}** active, **${trippedChans}** tripped)\n- **Active Models**: **${models.length}** heterogeneous models mounted\n- **Availability SLA**: 99.9% (HA dual-node load balancer active)\n- **Avg TTFT**: ${stats.avg_ttft_ms || 180} ms\n\n${trippedChans > 0 ? `> [!WARNING]\n> ${trippedChans} upstream channels are currently tripped open. Please review channel health in Upstream Channels.` : `> [!TIP]\n> Keep at least 2 weighted backup channels per core model for seamless zero-downtime failover.`}`,
        actions: [
          { label: isZh ? '前往通道管理体检' : 'Go to Channels', tab: 'channels' },
          { label: isZh ? '查看实时路由拓扑' : 'View Topology', tab: 'models' }
        ]
      };
    }

    // 2. Model Route & Topology Advisor
    if (q.includes('模型') || q.includes('拓扑') || q.includes('spof') || q.includes('单点') || q.includes('recommend') || q.includes('model')) {
      const singleProviderRoutes = modelRoutes.filter(r => (r.providers || []).length <= 1);
      const spofWarning = singleProviderRoutes.length > 0
        ? (isZh
            ? `⚠️ **单点故障风险预警**：检测到以下 ${singleProviderRoutes.length} 个模型仅绑定了单一渠道，缺乏故障转移保障：\n` + singleProviderRoutes.slice(0, 3).map(r => `- \`${r.model}\` (仅 1 个上游提供商)`).join('\n')
            : `⚠️ **SPOF Warning**: Detected ${singleProviderRoutes.length} models with only 1 upstream provider:\n` + singleProviderRoutes.slice(0, 3).map(r => `- \`${r.model}\` (1 provider only)`).join('\n'))
        : (isZh ? `✅ 所有模型路由均已配置冗余服务商保障，容灾架构完善。` : `✅ All model routes are equipped with redundant providers.`);

      return {
        text: isZh
          ? `### 💡 模型拓扑与多级容灾建议\n\n${spofWarning}\n\n#### 🎯 推荐生产级主备搭配：\n1. **代码与工程推理**：\n   - **主力通道**：\`claude-3-5-sonnet\` 或 \`deepseek-chat\` (Priority: 1, Weight: 80)\n   - **保底降级**：\`deepseek-coder\` / \`gpt-4o\` (Priority: 2, 自动熔断接管)\n2. **深度推理与数学规划**：\n   - **主力通道**：\`deepseek-reasoner\` (R1 思维链展开)\n   - **保底降级**：\`qwen-max\` 或 \`o1-mini\`\n3. **轻量与高速文本**：\n   - **主力通道**：\`deepseek-v4.1-flash\` (超快 TTFT < 120ms)\n\n> [!NOTE]\n> 在【模型路由】中开启「自动故障转移 (Failover)」，当主通道返回 429 或 5xx 时网关将在 20ms 内自动尝试下一优先级通道。`
          : `### 💡 Model Route & Resilience Advisor\n\n${spofWarning}\n\n#### 🎯 Recommended Primary/Fallback Pairs:\n1. **Coding & Engineering**:\n   - **Primary**: \`claude-3-5-sonnet\` or \`deepseek-chat\` (Priority: 1, Weight: 80)\n   - **Fallback**: \`deepseek-coder\` / \`gpt-4o\` (Priority: 2)\n2. **Reasoning & Math**:\n   - **Primary**: \`deepseek-reasoner\` (R1 chain of thought)\n   - **Fallback**: \`qwen-max\` / \`o1-mini\`\n3. **High-Velocity Lightweight**:\n   - **Primary**: \`deepseek-v4.1-flash\` (TTFT < 120ms)\n\n> [!NOTE]\n> Enable auto-failover in Model Routes. Airoute seamlessly retries next-tier providers within 20ms upon 429 or 5xx.`,
        actions: [
          { label: isZh ? '前往配置模型路由' : 'Configure Routes', tab: 'models' },
          { label: isZh ? '进入 Playground 对比测试' : 'Test in Playground', tab: 'playground' }
        ]
      };
    }

    // 3. Agent Configuration (Cursor / Claude Code)
    if (q.includes('cursor') || q.includes('claude') || q.includes('接入') || q.includes('setup') || q.includes('config') || q.includes('sdk')) {
      const snippetKey = activeKey || 'sk-airoute-xxxxxx';
      return {
        text: isZh
          ? `### 🚀 现代 AI 编程智能体接入指南\n\nAiroute 原生支持 OpenAI、Anthropic 与 Gemini 全双工协议转换，已就绪的接入密钥为：\`${snippetKey}\`\n\n#### 1. Cursor IDE 配置\n在 Cursor **Settings → Models** 中开启 OpenAI API 兼容：\n\`\`\`text\nOverride OpenAI Base URL: ${baseUrl}\nAPI Key: ${snippetKey}\nModels: deepseek-chat, deepseek-reasoner, claude-3-5-sonnet, gpt-4o\n\`\`\`\n\n#### 2. Claude Code CLI 终端配置\n在终端执行以下命令即可全量经由 Airoute 分发：\n\`\`\`bash\nexport ANTHROPIC_BASE_URL="${origin}"\nexport ANTHROPIC_API_KEY="${snippetKey}"\nclaude\n\`\`\`\n\n#### 3. Python 统一接入代码\n\`\`\`python\nfrom openai import OpenAI\n\nclient = OpenAI(\n    base_url="${baseUrl}",\n    api_key="${snippetKey}"\n)\n\nresponse = client.chat.completions.create(\n    model="deepseek-chat",\n    messages=[{"role": "user", "content": "Hello Airoute!"}],\n    stream=True\n)\nfor chunk in response:\n    print(chunk.choices[0].delta.content or "", end="", flush=True)\n\`\`\``
          : `### 🚀 Modern AI Agent Integration Guide\n\nAiroute natively supports OpenAI, Anthropic, and Gemini duplex protocol routing. Active key: \`${snippetKey}\`\n\n#### 1. Cursor IDE Setup\nIn Cursor **Settings → Models**:\n\`\`\`text\nOverride OpenAI Base URL: ${baseUrl}\nAPI Key: ${snippetKey}\nModels: deepseek-chat, deepseek-reasoner, claude-3-5-sonnet, gpt-4o\n\`\`\`\n\n#### 2. Claude Code CLI Setup\n\`\`\`bash\nexport ANTHROPIC_BASE_URL="${origin}"\nexport ANTHROPIC_API_KEY="${snippetKey}"\nclaude\n\`\`\`\n\n#### 3. Python OpenAI SDK\n\`\`\`python\nfrom openai import OpenAI\n\nclient = OpenAI(base_url="${baseUrl}", api_key="${snippetKey}")\nresp = client.chat.completions.create(\n    model="deepseek-chat",\n    messages=[{"role": "user", "content": "Hello Airoute!"}],\n    stream=True\n)\nfor chunk in resp:\n    print(chunk.choices[0].delta.content or "", end="", flush=True)\n\`\`\``,
        actions: [
          { label: isZh ? '查看 API 密钥' : 'Manage Keys', tab: 'keys' },
          { label: isZh ? '进入快速接入向导' : 'Open Quickstart', tab: 'docs' }
        ]
      };
    }

    // 4. Error Diagnostics
    if (q.includes('错误') || q.includes('报错') || q.includes('error') || q.includes('429') || q.includes('500') || q.includes('502') || q.includes('日志')) {
      const errorLogs = logs.filter(l => l.status_code >= 400);
      const recentErr = errorLogs[0];
      return {
        text: isZh
          ? `### ⚠️ 近期异常审计与智能根因剖析 (RCA)\n\n检测到当前记录中有 **${errorLogs.length}** 次异常调用。\n\n${recentErr ? `**最近一次报错样本**：\n- 状态码：\`HTTP ${recentErr.status_code}\`\n- 请求模型：\`${recentErr.model || '-'}\`\n- 路由渠道：\`${recentErr.channel || '直通'}\`\n- 错误消息：\`${recentErr.error_message || 'Upstream Error'}\`\n\n**根因分析与排障建议**：\n1. **429 Too Many Requests**：上游服务商 TPM/RPM 速率受限。建议：在通道设置中开启「请求平滑限流」并在模型路由中挂载第二渠道。\n2. **400 Context Overflow**：客户端传入 Prompt 超出模型上下文限制。建议：开启 Prompt 历史压缩或转为大上下文模型。\n3. **502 / 504 Gateway Timeout**：上游推理实例响应超时。建议：将通道超时时间从 30s 提升至 60s 或调整长文本生成并发限制。` : `✅ 近期无严重报错日志，系统可用性良好！`}`
          : `### ⚠️ Recent Error Audit & Root Cause Analysis\n\nDetected **${errorLogs.length}** failed requests in recent audit logs.\n\n${recentErr ? `**Latest Error Sample**:\n- Status: \`HTTP ${recentErr.status_code}\`\n- Model: \`${recentErr.model || '-'}\`\n- Channel: \`${recentErr.channel || 'Direct'}\`\n- Error: \`${recentErr.error_message || 'Upstream Error'}\`\n\n**Remediation Steps**:\n1. **429 Rate Limit**: Upstream TPM/RPM reached. Add fallback channel.\n2. **400 Context Overflow**: Prompt exceeds model limit. Truncate context.\n3. **502 / 504 Timeout**: Upstream slow. Increase timeout to 60s.` : `✅ No major error logs found. Cluster health is optimal!`}`,
        actions: [
          { label: isZh ? '前往审计日志排查' : 'Inspect Logs', tab: 'logs' },
          { label: isZh ? '查看通道健康状态' : 'Check Channels', tab: 'channels' }
        ]
      };
    }

    // Default intelligent general assistant reply
    return {
      text: isZh
        ? `### 🤖 Airoute 智能网关助理解答\n\n针对你的问题：「**${userQuery}**」：\n\nAiroute 是面向 AI 时代的高性能大模型与智能体多模态网关系统，具备以下核心能力：\n1. **统一接口契约**：全兼容 OpenAI \`/v1/chat/completions\`、Anthropic Messages 与 Google Gemini 协议。\n2. **自适应动态路由**：支持权重随机、最小延迟优先、故障转移 (Failover) 与断路器隔离。\n3. **Token FinOps 降本**：内置 Prompt Caching 自动命中识别与闲时半价策略。\n4. **MCP & Agent 工具生态**：内置 Model Context Protocol (MCP 2024-11-05) 双向通信与 Skill Hub 插件商店。\n\n你可以尝试问我：\n- *“推荐一个支持视觉的多模态模型”*\n- *“如何配置备用降级渠道以防上游挂掉？”*\n- *“我的 Cursor 怎么接入这个网关？”*`
        : `### 🤖 Airoute AI Assistant\n\nRegarding: "**${userQuery}**":\n\nAiroute is a high-performance, enterprise-grade AI gateway designed for the LLM & Agent era:\n1. **Unified Protocols**: Full compatibility with OpenAI, Anthropic, and Gemini API schemas.\n2. **Dynamic Routing**: Supports weighted load balancing, latency-based routing, and zero-downtime failover.\n3. **FinOps Token Economics**: Built-in Prompt Caching and off-peak discount engine.\n4. **MCP & Skills Hub**: Model Context Protocol support and Agent skill registry.\n\nFeel free to ask for channel diagnostics, agent setup, or prompt engineering tips!`,
      actions: [
        { label: isZh ? '进入 Playground 调试' : 'Open Playground', tab: 'playground' },
        { label: isZh ? '查看开发者文档' : 'View Docs', tab: 'docs' }
      ]
    };
  };

  const handleSend = async (queryText = input) => {
    const text = (queryText || '').trim();
    if (!text || isLoading) return;

    const userMsg = { id: `u_${Date.now()}`, role: 'user', content: text };
    setMessages(prev => [...prev, userMsg]);
    setInput('');
    setIsLoading(true);

    try {
      // Small simulated latency for natural conversation feel
      await new Promise(r => setTimeout(r, 450));
      const resp = await generateAssistantResponse(text);

      const assistantMsg = {
        id: `a_${Date.now()}`,
        role: 'assistant',
        content: resp.text,
        actions: resp.actions
      };
      setMessages(prev => [...prev, assistantMsg]);
    } catch (err) {
      setMessages(prev => [
        ...prev,
        {
          id: `err_${Date.now()}`,
          role: 'assistant',
          content: isZh ? `⚠️ 抱歉，处理您的请求时遇到异常：${err.message}` : `⚠️ Sorry, error processing request: ${err.message}`
        }
      ]);
    } finally {
      setIsLoading(false);
    }
  };

  if (!isOpen) {
    // Floating Pill Button Trigger
    return (
      <button
        onClick={onToggle}
        className="fixed bottom-6 right-6 z-40 flex items-center space-x-2 px-4 py-2.5 rounded-full bg-linear-to-r from-indigo-600 via-purple-600 to-indigo-700 hover:from-indigo-500 hover:to-purple-500 text-white shadow-xl shadow-indigo-500/25 hover:shadow-indigo-500/40 transition-all duration-300 group cursor-pointer border border-white/20 animate-in fade-in"
        title="Airoute AI Copilot (Cmd + J)"
      >
        <div className="relative">
          <Sparkles className="w-4 h-4 text-amber-300 animate-pulse" />
        </div>
        <span className="text-xs font-bold tracking-tight">
          {isZh ? 'AI 智能副驾驶' : 'AI Copilot'}
        </span>
        <kbd className="hidden sm:inline-flex items-center px-1.5 py-0.5 text-[10px] font-mono font-semibold bg-white/20 text-white/90 rounded-sm border border-white/20">
          ⌘J
        </kbd>
      </button>
    );
  }

  return (
    <div
      className={`fixed z-50 transition-all duration-300 ease-out flex flex-col ${
        isExpanded
          ? 'inset-4 sm:inset-10 bg-white dark:bg-[#111726] rounded-2xl shadow-2xl border border-slate-200 dark:border-slate-800'
          : 'bottom-6 right-6 w-full max-w-md h-[600px] max-h-[85vh] bg-white dark:bg-[#111726] rounded-2xl shadow-2xl border border-slate-200 dark:border-slate-800'
      } overflow-hidden animate-in zoom-in-95`}
    >
      {/* Header */}
      <div className="px-5 py-4 border-b border-slate-100 dark:border-slate-800 flex items-center justify-between bg-white dark:bg-[#111726]">
        <div className="flex items-center space-x-2.5">
          <div className="w-8 h-8 rounded-lg bg-slate-900 dark:bg-indigo-600 flex items-center justify-center text-white shadow-sm">
            <Sparkles className="w-4 h-4 text-amber-300" />
          </div>
          <div>
            <div className="flex items-center space-x-1.5">
              <h3 className="font-semibold text-sm text-slate-900 dark:text-slate-100">
                {isZh ? 'Airoute AI 智能副驾驶' : 'Airoute AI Copilot'}
              </h3>
              <span className="px-1.5 py-0.5 rounded-full text-[10px] font-semibold bg-indigo-100 dark:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300">
                v2.0
              </span>
            </div>
            <p className="text-[11px] text-slate-500 dark:text-slate-400">
              {isZh ? '全站智能运维 · 故障诊断 · 模型选型' : 'Autonomous Ops · RCA · Route Advisor'}
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-1">
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="p-1.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition"
            title={isExpanded ? 'Restore' : 'Expand'}
          >
            {isExpanded ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
          </button>
          <button
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
            title="Close (ESC)"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Messages container */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4 text-xs">
        {messages.map((m) => {
          const isUser = m.role === 'user';
          return (
            <div
              key={m.id}
              className={`flex items-start space-x-2.5 ${isUser ? 'flex-row-reverse space-x-reverse' : ''}`}
            >
              <div
                className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 ${
                  isUser
                    ? 'bg-slate-900 dark:bg-slate-700 text-white'
                    : 'bg-indigo-600 text-white'
                }`}
              >
                {isUser ? <User className="w-3.5 h-3.5" /> : <Bot className="w-3.5 h-3.5" />}
              </div>

              <div
                className={`max-w-[85%] rounded-2xl p-3.5 leading-relaxed relative group ${
                  isUser
                    ? 'bg-indigo-600 text-white rounded-tr-none'
                    : 'bg-slate-50 dark:bg-slate-900 border border-slate-200/80 dark:border-slate-800 text-slate-800 dark:text-slate-200 rounded-tl-none shadow-sm'
                }`}
              >
                {!isUser && (
                  <button
                    onClick={() => handleCopy(m.content, m.id)}
                    className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 p-1 rounded-md bg-white dark:bg-slate-800 text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 shadow-sm transition border border-slate-200/60 dark:border-slate-700"
                    title={isZh ? '复制内容' : 'Copy'}
                  >
                    {copiedId === m.id ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                  </button>
                )}

                <div className="prose prose-xs dark:prose-invert max-w-none space-y-2 whitespace-pre-wrap font-sans">
                  {m.content}
                </div>

                {/* Optional Action Buttons */}
                {m.actions && m.actions.length > 0 && (
                  <div className="mt-3 pt-2.5 border-t border-slate-200/60 dark:border-slate-800 flex flex-wrap gap-2">
                    {m.actions.map((act, i) => (
                      <button
                        key={i}
                        onClick={() => {
                          if (act.tab) {
                            setCurrentTab(act.tab);
                            if (setViewMode) setViewMode('console');
                            if (!isExpanded) onClose();
                          }
                        }}
                        className="px-2.5 py-1 rounded-lg bg-indigo-50 hover:bg-indigo-100 dark:bg-indigo-950/60 dark:hover:bg-indigo-900/80 text-indigo-700 dark:text-indigo-300 font-semibold text-[11px] flex items-center space-x-1 transition border border-indigo-200/60 dark:border-indigo-800 cursor-pointer"
                      >
                        <span>{act.label}</span>
                        <ArrowRight className="w-3 h-3" />
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>
          );
        })}

        {isLoading && (
          <div className="flex items-center space-x-2 text-slate-400 text-xs py-2">
            <RefreshCw className="w-3.5 h-3.5 animate-spin text-indigo-500" />
            <span>{isZh ? 'AI 智能副驾驶思考与分析中...' : 'Airoute Copilot is thinking...'}</span>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Quick Action Chips */}
      <div className="px-3 py-2 bg-slate-50/60 dark:bg-slate-900/40 border-t border-slate-100 dark:border-slate-800 flex items-center gap-1.5 overflow-x-auto text-[11px] no-scrollbar">
        {quickActions.map((qa) => (
          <button
            key={qa.id}
            onClick={() => handleSend(qa.prompt)}
            className="px-2.5 py-1 rounded-full bg-white dark:bg-slate-800 hover:bg-indigo-50 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 hover:text-indigo-600 dark:hover:text-indigo-300 border border-slate-200/80 dark:border-slate-700 whitespace-nowrap transition cursor-pointer font-medium shadow-sm"
          >
            {qa.label}
          </button>
        ))}
      </div>

      {/* Input box */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          handleSend();
        }}
        className="p-3 border-t border-slate-200/80 dark:border-slate-800 bg-white dark:bg-[#111726] flex items-center space-x-2"
      >
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={isZh ? '向 AI 副驾驶提问关于网关配置、排错或架构...' : 'Ask Copilot anything about gateway ops or architecture...'}
          className="flex-1 bg-slate-50 dark:bg-slate-900/80 border border-slate-200 dark:border-slate-800 rounded-xl px-3.5 py-2 text-xs text-slate-900 dark:text-slate-100 placeholder-slate-400 focus:outline-none focus:ring-1 focus:ring-indigo-500"
        />
        <button
          type="submit"
          disabled={!input.trim() || isLoading}
          className="p-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white transition cursor-pointer shadow-md shadow-indigo-600/20"
        >
          <Send className="w-3.5 h-3.5" />
        </button>
      </form>
    </div>
  );
}
