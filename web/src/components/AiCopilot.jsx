import React, { useState, useEffect, useRef, useMemo } from 'react';
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
  Maximize2,
  Trash2,
  StopCircle,
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  Layers,
  Wrench,
  CheckCircle2,
  Play,
  Code,
  Clock,
  Database,
  HelpCircle,
  GripVertical
} from 'lucide-react';
import { getGatewayOrigin, copyToClipboard as crossOriginCopy } from '../config';
import MarkdownRenderer from './MarkdownRenderer';
import {
  COPILOT_HARNESS_TOOLS,
  HARNESS_BENCHMARKS,
  executeHarnessTool
} from './copilotHarness';

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
  prices = [],
  stats = {},
  lang = 'zh',
  t,
  showToast,
  adminUser,
  adminFetch,
  initialQuery = '',
  onClearInitialQuery
}) {
  const isZh = lang === 'zh';
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [copiedId, setCopiedId] = useState(null);
  const [isExpanded, setIsExpanded] = useState(false);
  const [selectedModel, setSelectedModel] = useState('');
  const [expandedReasoningIds, setExpandedReasoningIds] = useState({});
  const [expandedTraceIds, setExpandedTraceIds] = useState({});
  const [expandedStepObsIds, setExpandedStepObsIds] = useState({});
  const [showToolsModal, setShowToolsModal] = useState(false);
  const [activeBenchmark, setActiveBenchmark] = useState(null);

  const messagesEndRef = useRef(null);
  const inputRef = useRef(null);
  const abortControllerRef = useRef(null);

  // Floating trigger button draggable state
  const [btnPos, setBtnPos] = useState(() => {
    try {
      const saved = localStorage.getItem('airoute_copilot_btn_pos');
      if (saved) {
        const parsed = JSON.parse(saved);
        if (typeof parsed.x === 'number' && typeof parsed.y === 'number') {
          return parsed;
        }
      }
    } catch (e) {}
    return null;
  });

  const [isBtnDragging, setIsBtnDragging] = useState(false);
  const btnDragRef = useRef({
    startX: 0,
    startY: 0,
    initialX: 0,
    initialY: 0,
    hasMoved: false
  });
  const justDraggedBtnRef = useRef(false);
  const btnRef = useRef(null);

  // Modal window draggable state (when not expanded)
  const [modalPos, setModalPos] = useState(null);
  const modalDragRef = useRef({
    startX: 0,
    startY: 0,
    initialX: 0,
    initialY: 0,
    hasMoved: false
  });
  const modalRef = useRef(null);

  // Initialize and ensure safe button position on window resize
  useEffect(() => {
    const computeSafeDefault = () => {
      const w = typeof window !== 'undefined' ? window.innerWidth : 1200;
      const h = typeof window !== 'undefined' ? window.innerHeight : 800;
      // Default: 96px from bottom (safely clears table pagination bar), 24px from right
      return {
        x: Math.max(16, w - 190),
        y: Math.max(16, h - 96)
      };
    };

    setBtnPos(prev => {
      if (!prev) return computeSafeDefault();
      const w = window.innerWidth;
      const h = window.innerHeight;
      return {
        x: Math.min(Math.max(16, prev.x), Math.max(16, w - 180)),
        y: Math.min(Math.max(16, prev.y), Math.max(16, h - 56))
      };
    });

    const handleResize = () => {
      setBtnPos(prev => {
        if (!prev) return computeSafeDefault();
        const w = window.innerWidth;
        const h = window.innerHeight;
        return {
          x: Math.min(Math.max(16, prev.x), Math.max(16, w - 180)),
          y: Math.min(Math.max(16, prev.y), Math.max(16, h - 56))
        };
      });
    };

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const handleBtnPointerDown = (e) => {
    if (e.button !== 0) return;
    e.preventDefault();

    const currentX = btnPos?.x ?? (window.innerWidth - 190);
    const currentY = btnPos?.y ?? (window.innerHeight - 96);

    btnDragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      initialX: currentX,
      initialY: currentY,
      hasMoved: false
    };

    const handlePointerMove = (moveEvent) => {
      const dx = moveEvent.clientX - btnDragRef.current.startX;
      const dy = moveEvent.clientY - btnDragRef.current.startY;

      if (!btnDragRef.current.hasMoved && Math.hypot(dx, dy) > 4) {
        btnDragRef.current.hasMoved = true;
        setIsBtnDragging(true);
      }

      if (btnDragRef.current.hasMoved) {
        const btnW = btnRef.current ? btnRef.current.offsetWidth : 160;
        const btnH = btnRef.current ? btnRef.current.offsetHeight : 44;
        const maxX = Math.max(12, window.innerWidth - btnW - 12);
        const maxY = Math.max(12, window.innerHeight - btnH - 12);

        const newX = Math.min(maxX, Math.max(12, btnDragRef.current.initialX + dx));
        const newY = Math.min(maxY, Math.max(12, btnDragRef.current.initialY + dy));

        setBtnPos({ x: newX, y: newY });
      }
    };

    const handlePointerUp = () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
      window.removeEventListener('pointercancel', handlePointerUp);

      if (btnDragRef.current.hasMoved) {
        setIsBtnDragging(false);
        justDraggedBtnRef.current = true;
        setTimeout(() => {
          justDraggedBtnRef.current = false;
        }, 120);

        setBtnPos(currentPos => {
          if (currentPos) {
            try {
              localStorage.setItem('airoute_copilot_btn_pos', JSON.stringify(currentPos));
            } catch (err) {}
          }
          return currentPos;
        });
      }
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', handlePointerUp);
    window.addEventListener('pointercancel', handlePointerUp);
  };

  const handleBtnClick = (e) => {
    if (justDraggedBtnRef.current || btnDragRef.current.hasMoved) {
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    onToggle();
  };

  const handleModalHeaderPointerDown = (e) => {
    if (isExpanded) return;
    if (e.target.closest('button, select, input')) return;
    if (e.button !== 0) return;
    e.preventDefault();

    const rect = modalRef.current?.getBoundingClientRect();
    const currentX = rect ? rect.left : (window.innerWidth - 440);
    const currentY = rect ? rect.top : (window.innerHeight - 660);

    modalDragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      initialX: currentX,
      initialY: currentY,
      hasMoved: false
    };

    const handlePointerMove = (moveEvent) => {
      const dx = moveEvent.clientX - modalDragRef.current.startX;
      const dy = moveEvent.clientY - modalDragRef.current.startY;

      if (!modalDragRef.current.hasMoved && Math.hypot(dx, dy) > 4) {
        modalDragRef.current.hasMoved = true;
      }

      if (modalDragRef.current.hasMoved) {
        const modalW = modalRef.current ? modalRef.current.offsetWidth : 420;
        const modalH = modalRef.current ? modalRef.current.offsetHeight : 640;
        const maxX = Math.max(12, window.innerWidth - modalW - 12);
        const maxY = Math.max(12, window.innerHeight - modalH - 12);

        const newX = Math.min(maxX, Math.max(12, modalDragRef.current.initialX + dx));
        const newY = Math.min(maxY, Math.max(12, modalDragRef.current.initialY + dy));

        setModalPos({ x: newX, y: newY });
      }
    };

    const handlePointerUp = () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
      window.removeEventListener('pointercancel', handlePointerUp);
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', handlePointerUp);
    window.addEventListener('pointercancel', handlePointerUp);
  };


  const origin = getGatewayOrigin();
  const baseUrl = origin ? `${origin}/v1` : '/v1';

  // Discover candidate chat models
  const inferClientModality = (m) => {
    const lower = (m || '').toLowerCase();
    if (lower.includes('rerank') || lower.includes('bge-reranker')) return 'rerank';
    if (lower.includes('embed') || lower.includes('bge-') || lower.includes('text-embedding')) return 'embeddings';
    if (lower.includes('tts') || lower.includes('speech') || lower.includes('voice')) return 'audio_speech';
    if (lower.includes('whisper') || lower.includes('transcription') || lower.includes('asr')) return 'audio_transcription';
    if (lower.includes('sora') || lower.includes('video')) return 'videos';
    if (lower.includes('image') || lower.includes('dall-e') || lower.includes('flux')) return 'images';
    return 'chat';
  };

  const candidateModels = useMemo(() => {
    let list = [];
    if (modelRoutes && modelRoutes.length > 0) {
      const chatRoutes = modelRoutes.filter(r => r.modality === 'chat' || !r.modality).map(r => r.model);
      if (chatRoutes.length > 0) list = chatRoutes;
    }
    if (list.length === 0 && models && models.length > 0) {
      const chatModels = models.filter(m => inferClientModality(m) === 'chat');
      list = chatModels.length > 0 ? chatModels : models;
    }
    return Array.from(new Set(list));
  }, [models, modelRoutes]);

  // Set default selected model
  useEffect(() => {
    if (!selectedModel && candidateModels.length > 0) {
      const preferred = candidateModels.find(m => m.includes('flash') || m.includes('deepseek') || m.includes('glm')) || candidateModels[0];
      setSelectedModel(preferred);
    }
  }, [candidateModels, selectedModel]);

  // Active Key resolution
  const activeKey = useMemo(() => {
    if (keys && keys.length > 0) {
      const activeObj = keys.find(k => k.status === 'active');
      return activeObj ? activeObj.key : keys[0].key;
    }
    return '';
  }, [keys]);

  // Welcome message with Harness context
  useEffect(() => {
    if (messages.length === 0) {
      setMessages([
        {
          id: 'welcome',
          role: 'assistant',
          content: isZh
            ? `👋 你好！我是 **Airoute AI 智能副驾驶**（标准 Agent Harness 架构）。\n\n我已装载标准的 **OpenAI Function Calling (Tools v1)** 契约规范与网关沙箱环境，支持 **ReAct 自主思考与工具循环调用**：\n\n- 🔧 **集群遥测沙箱** (\`get_cluster_telemetry\`)：实时洞察通道状态、活跃度与熔断隔离\n- 💡 **拓扑 SPOF 审计** (\`audit_routing_spof\`)：扫描全量模型路由单点脆弱性，推荐主备容灾拓扑\n- ⚠️ **报错根因剖析** (\`diagnose_recent_errors\`)：聚合 429 速率限制与 5xx 故障，输出 RCA 诊断\n- 🧭 **控制台联动巡航** (\`navigate_console_view\`)：支持根据指令自主切换通道、路由与日志页面\n- 🚀 **智能体开箱配置** (\`generate_client_config\`)：秒级生成 Cursor、Claude Code 与 Python 接入套件\n\n请点击下方预设的 **Harness 巡检基准任务**，或直接输入你的提问！`
            : `👋 Hello! I am the **Airoute AI Copilot** (Standard Agent Harness Architecture).\n\nEquipped with **OpenAI Function Calling (Tools v1)** protocol and the Airoute Console Sandbox, I operate via autonomous **ReAct multi-turn tool loops**:\n\n- 🔧 **Cluster Telemetry** (\`get_cluster_telemetry\`): Real-time channel availability & breaker isolation\n- 💡 **SPOF Audit** (\`audit_routing_spof\`): Single Point of Failure detection & failover advisory\n- ⚠️ **RCA Diagnostics** (\`diagnose_recent_errors\`): Aggregated root cause analysis for 429 & 5xx errors\n- 🧭 **Console Navigation** (\`navigate_console_view\`): Autonomous UI tab switching across console modules\n- 🚀 **Client Configs** (\`generate_client_config\`): Instant setups for Cursor, Claude Code & Python SDK\n\nClick any **Harness Benchmark Task** below or ask in natural language!`,
          harnessTrace: []
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

  // Handle incoming initial query
  useEffect(() => {
    if (isOpen && initialQuery && initialQuery.trim()) {
      handleSend(initialQuery.trim());
      if (onClearInitialQuery) onClearInitialQuery();
    }
  }, [isOpen, initialQuery]);

  const handleCopy = async (text, id) => {
    await crossOriginCopy(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
    if (showToast) showToast(isZh ? '已复制到剪贴板' : 'Copied to clipboard', 'success');
  };

  const handleClearHistory = () => {
    setMessages([
      {
        id: `welcome_${Date.now()}`,
        role: 'assistant',
        content: isZh
          ? `会话已重置。**Airoute Agent Harness** 就绪，请随时提问！`
          : `Conversation reset. **Airoute Agent Harness** ready for your questions!`,
        harnessTrace: []
      }
    ]);
  };

  const handleStopGeneration = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setIsLoading(false);
  };

  // Build System Prompt for the Harness Agent
  const buildHarnessSystemPrompt = () => {
    return isZh
      ? `你是内置于 Airoute 企业级 AI 网关控制中枢的专业自主智能体副驾驶 (Airoute Autonomous AI Ops Copilot)。
你运行在标准的 Agent Harness 运行时中，拥有直接调用网关遥测与控制沙箱的工具契约 (OpenAI Function Calling)。

【你的工具调用准则】：
1. 自主调用工具：当用户询问集群健康、通道状态、模型路由、单点故障(SPOF)、错误日志或客户端接入时，你应当优先调用相对应的工具（如 get_cluster_telemetry、audit_routing_spof、diagnose_recent_errors 等）以获取第一手实时数据。
2. 严谨分析与归纳：在获取工具观察结果 (Observation) 后，结合实际数据进行深入分析，给出清晰明确的架构评估、故障定位和调优建议。
3. 结构化呈现：使用规范的 Markdown 格式输出，包括各级标题、列表、重点加粗和语言标注的代码块。
4. 控制台联动：在适当时机可通过 navigate_console_view 工具指引用户跳转至具体管理页面。`
      : `You are the autonomous AI Ops Copilot built into the Airoute AI Gateway Control Plane, running on a standard Agent Harness architecture.
You have direct access to gateway telemetry and operations via OpenAI Function Calling tools.

【Guidelines】：
1. Autonomous Tool Execution: Always invoke appropriate tools (get_cluster_telemetry, audit_routing_spof, diagnose_recent_errors, etc.) to fetch authoritative live telemetry.
2. Grounded Analysis: Synthesize observations accurately with actionable recommendations.
3. Structured Output: Use clear GitHub-flavored Markdown.
4. Console Navigation: Use navigate_console_view when appropriate to help the user navigate to relevant console tabs.`;
  };

  // Derive interactive action buttons from text content
  const deriveActionsFromContent = (text = '') => {
    const actions = [];
    const lower = text.toLowerCase();

    if (lower.includes('通道管理') || lower.includes('通道') || lower.includes('channel')) {
      actions.push({ label: isZh ? '前往通道管理' : 'Go to Channels', tab: 'channels' });
    }
    if (lower.includes('模型路由') || lower.includes('路由') || lower.includes('拓扑') || lower.includes('spof') || lower.includes('route')) {
      actions.push({ label: isZh ? '前往模型路由' : 'Go to Routes', tab: 'models' });
    }
    if (lower.includes('审计日志') || lower.includes('日志') || lower.includes('报错') || lower.includes('error log')) {
      actions.push({ label: isZh ? '查看审计日志' : 'Inspect Logs', tab: 'logs' });
    }
    if (lower.includes('playground') || lower.includes('调试') || lower.includes('测试')) {
      actions.push({ label: isZh ? '进入 Playground' : 'Open Playground', tab: 'playground' });
    }
    if (lower.includes('api 密钥') || lower.includes('密钥') || lower.includes('cursor') || lower.includes('claude code') || lower.includes('api key')) {
      actions.push({ label: isZh ? '管理 API 密钥' : 'Manage Keys', tab: 'keys' });
    }
    if (lower.includes('mcp') || lower.includes('协议探针')) {
      actions.push({ label: isZh ? '前往 MCP 广场' : 'Go to MCP Hub', tab: 'mcp' });
    }
    if (lower.includes('skill') || lower.includes('技能') || lower.includes('sop')) {
      actions.push({ label: isZh ? '前往 SkillHub' : 'Go to SkillHub', tab: 'skills' });
    }

    const seen = new Set();
    return actions.filter(a => {
      if (seen.has(a.tab)) return false;
      seen.add(a.tab);
      return true;
    }).slice(0, 3);
  };

  // Context bundle for tool execution
  const toolContext = useMemo(() => ({
    channels,
    models,
    modelRoutes,
    logs,
    stats,
    keys,
    prices,
    origin,
    baseUrl,
    setCurrentTab,
    setViewMode,
    showToast,
    adminFetch,
    isZh
  }), [channels, models, modelRoutes, logs, stats, keys, prices, origin, baseUrl, setCurrentTab, setViewMode, showToast, adminFetch, isZh]);

  // Autonomous Fallback Harness: If upstream model or network fails, run the harness tool locally!
  const executeLocalHarnessDiagnostic = async (queryText) => {
    const q = (queryText || '').toLowerCase();
    const trace = [];

    if (q.includes('定价') || q.includes('费率') || q.includes('匹配') || q.includes('price') || q.includes('finops')) {
      const step1 = await executeHarnessTool('audit_routing_spof', {}, toolContext);
      trace.push(step1);
      const res = step1.result;
      const pc = res.pricing_consistency || {};

      return {
        trace,
        text: `### 💰 模型路由与计费定价闭环审计报告\n\n**整体闭环评估**：${pc.evaluation || '已核验'}\n\n- **未配专属定价路由**：共 **${pc.unpriced_routes_count || 0}** 个模型路由尚未配置专属费率（当前自动回退至系统基准定价 ¥2.0/¥8.0 计费）\n${(pc.unpriced_routes_sample || []).length > 0 ? pc.unpriced_routes_sample.map(m => `  - \`${m}\``).join('\n') : '  - 全量路由均已配置专属费率'}\n\n- **未挂载路由定价**：共 **${pc.unrouted_prices_count || 0}** 条定价规则未挂载任何上游服务商渠道（客户端请求这些模型将返回 404 无法使用）\n${(pc.unrouted_prices_sample || []).length > 0 ? pc.unrouted_prices_sample.map(m => `  - \`${m}\``).join('\n') : '  - 全量定价均已关联可用路由'}\n\n> [!TIP]\n> 建议保持模型路由与定价规则 1:1 闭环配置，避免出现调用无定价或定价无路由的情况。`
      };
    }

    if (q.includes('巡检') || q.includes('健康') || q.includes('通道') || q.includes('health') || q.includes('diagnostic') || q.includes('sre')) {
      const step1 = await executeHarnessTool('get_cluster_telemetry', {}, toolContext);
      trace.push(step1);
      const res = step1.result;
      const trippedCount = res.cluster_summary.tripped_channels;

      return {
        trace,
        text: `### 🩺 Airoute 网关集群深度健康巡检报告\n\n**集群健康评分**：\`${res.cluster_summary.health_score} / 100\`\n\n- **上游服务商渠道**：共接入 **${res.cluster_summary.total_channels}** 个（活跃 **${res.cluster_summary.active_channels}** 个，熔断隔离 **${trippedCount}** 个）\n- **模型挂载总数**：当前已上线 **${res.cluster_summary.mounted_models_count}** 个异构模型\n- **性能基线指标**：平均首字延迟 (TTFT) 为 **${res.performance_metrics.avg_ttft_ms} ms**，缓存命中率 **${res.performance_metrics.cache_hit_rate}**\n- **高可用保障**：${res.cluster_summary.ha_status}\n\n${trippedCount > 0 ? `> [!WARNING]\n> 检测到有 ${trippedCount} 个上游通道处于熔断保护状态。建议前往【通道管理】执行网络探针核验并检查上游配额。` : `> [!TIP]\n> 当前所有上游通道均处于健康活跃状态，首字延迟与吞吐基线平稳。`}`
      };
    }

    if (q.includes('路由') || q.includes('模型') || q.includes('spof') || q.includes('单点') || q.includes('拓扑') || q.includes('recommend')) {
      const step1 = await executeHarnessTool('audit_routing_spof', {}, toolContext);
      trace.push(step1);
      const res = step1.result;

      const spofDesc = res.spof_vulnerable_count > 0
        ? `⚠️ **单点故障风险预警**：检测到以下 ${res.spof_vulnerable_count} 个模型仅绑定单一上游提供商：\n` + res.spof_vulnerabilities.slice(0, 4).map(v => `- \`${v.model}\`：当前仅由 \`${v.primary_channel}\` 提供，缺乏灾备降级通道`).join('\n')
        : `✅ 全量模型路由均已配置冗余服务商保障，容灾架构完善。`;

      return {
        trace,
        text: `### 💡 模型路由单点故障 (SPOF) 与容灾拓扑审计\n\n${spofDesc}\n\n#### 🎯 生产级主备调优方案：\n1. **高并发旗舰模型**：为关键模型挂载两个以上异构渠道（如阿里百炼 + 火山引擎 + 官方源）。\n2. **权重与优先级梯队**：主通道设优先级 1、权重 80；备通道设优先级 2、权重 20，并开启「自动故障转移 (Failover)」。\n3. **熔断与降级**：在上游遭遇 429 速率限制或 5xx 故障时，网关将在毫秒级自动切换至备选渠道。`
      };
    }

    if (q.includes('cursor') || q.includes('claude') || q.includes('接入') || q.includes('setup') || q.includes('sdk')) {
      const step1 = await executeHarnessTool('generate_client_config', { client_type: 'all' }, toolContext);
      trace.push(step1);
      const configs = step1.result;

      return {
        trace,
        text: `### 🚀 现代 AI 编程智能体标准化接入指南\n\nAiroute 原生兼容 OpenAI、Anthropic 与 Gemini 统一协议。已为你生成专属配置：\n\n#### 1. Cursor IDE 配置\n在 Cursor **Settings → Models** 中开启 OpenAI API 兼容：\n\`\`\`text\nOverride OpenAI Base URL: ${configs.cursor?.base_url}\nAPI Key: ${configs.cursor?.api_key}\nModel: ${configs.cursor?.model}\n\`\`\`\n\n#### 2. Claude Code CLI 终端配置\n\`\`\`bash\n${configs.claude_code?.bash_env}\n\`\`\`\n\n#### 3. Python 统一接入代码\n\`\`\`python\n${configs.python?.code_snippet}\n\`\`\``
      };
    }

    if (q.includes('错误') || q.includes('报错') || q.includes('error') || q.includes('日志') || q.includes('429') || q.includes('500') || q.includes('502')) {
      const step1 = await executeHarnessTool('diagnose_recent_errors', { limit: 5 }, toolContext);
      trace.push(step1);
      const res = step1.result;

      return {
        trace,
        text: `### ⚠️ 近期异常审计与智能根因剖析 (RCA)\n\n检测到当前记录中共有 **${res.total_error_count}** 次异常调用。\n\n**状态码分布**：${Object.entries(res.status_code_distribution).map(([c, n]) => `\`HTTP ${c}\`: ${n}次`).join('，') || '暂无异常'}\n\n${res.error_samples.length > 0 ? `**最新异常调用样本**：\n- 状态码：\`HTTP ${res.error_samples[0].status_code}\`\n- 请求模型：\`${res.error_samples[0].model}\`\n- 路由渠道：\`${res.error_samples[0].channel}\`\n- 报错信息：\`${res.error_samples[0].error_message}\`` : `✅ 近期无严重报错日志，系统可用性良好！`}\n\n**根本原因推导与排障策略**：\n${res.common_rca_patterns.map(p => `- ${p}`).join('\n') || '- 系统稳定，未识别到异常波动模式。'}`
      };
    }

    // Default telemetry summary
    const step1 = await executeHarnessTool('get_cluster_telemetry', {}, toolContext);
    trace.push(step1);
    return {
      trace,
      text: `### 🤖 Airoute 智能网关架构解答\n\n针对你的问题：「**${queryText}**」：\n\nAiroute 是面向大模型与智能体时代的高性能企业级网关系统，内置以下核心机制：\n1. **标准 Agent Harness 协议**：原生提供 OpenAI Function Calling (Tools v1) 契约，支持 ReAct 循环。\n2. **自适应动态路由**：支持权重随机、最小延迟优先、故障转移 (Failover) 与断路器隔离。\n3. **Token FinOps 降本**：内置 Prompt Caching 自动命中识别与闲时半价策略。\n4. **MCP 扩展生态**：支持 Model Context Protocol 双向探针通信与 SkillHub 技能扩展。\n\n你可以随时尝试下方的 **Harness 巡检套件**，或要求我为你排查任何网关问题！`
    };
  };

  // Main ReAct Harness Loop
  const handleSend = async (queryText = input) => {
    const text = (queryText || '').trim();
    if (!text || isLoading) return;

    const userMsgId = `u_${Date.now()}`;
    const assistantMsgId = `a_${Date.now()}`;
    const userMsg = { id: userMsgId, role: 'user', content: text };

    setMessages(prev => [...prev, userMsg]);
    setInput('');
    setIsLoading(true);

    const controller = new AbortController();
    abortControllerRef.current = controller;

    // Initialize assistant message container
    setMessages(prev => [
      ...prev,
      {
        id: assistantMsgId,
        role: 'assistant',
        content: '',
        reasoning: '',
        harnessTrace: [],
        actions: []
      }
    ]);

    const targetModel = selectedModel || candidateModels[0] || 'deepseek-v4-flash';
    let fullContent = '';
    let fullReasoning = '';
    const accumulatedTrace = [];
    let usedLiveLlm = false;

    try {
      const headers = { 'Content-Type': 'application/json' };
      if (activeKey) {
        headers['Authorization'] = `Bearer ${activeKey}`;
      }

      // Build conversation history for payload
      const systemPrompt = buildHarnessSystemPrompt();
      const payloadMessages = [
        { role: 'system', content: systemPrompt },
        ...messages
          .filter(m => m.id !== 'welcome' && !m.id.startsWith('err_'))
          .slice(-6)
          .map(m => ({ role: m.role, content: m.content })),
        { role: 'user', content: text }
      ];

      // ReAct Iteration Loop (up to 4 steps)
      let iteration = 0;
      const MAX_ITERATIONS = 4;
      let shouldContinue = true;

      while (shouldContinue && iteration < MAX_ITERATIONS) {
        iteration++;

        const response = await fetch('/v1/chat/completions', {
          method: 'POST',
          headers,
          signal: controller.signal,
          body: JSON.stringify({
            model: targetModel,
            messages: payloadMessages,
            tools: COPILOT_HARNESS_TOOLS,
            stream: true,
            temperature: 0.5,
            max_tokens: 3500
          })
        });

        if (!response.ok || !response.body) {
          throw new Error(`HTTP ${response.status}`);
        }

        usedLiveLlm = true;
        const reader = response.body.getReader();
        const decoder = new TextDecoder('utf-8');
        let done = false;
        let buffer = '';
        let streamedToolCalls = {};
        let finishReason = null;

        while (!done) {
          const { value, done: readerDone } = await reader.read();
          done = readerDone;
          if (value) {
            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            for (const line of lines) {
              const trimmed = line.trim();
              if (trimmed.startsWith('data: ')) {
                const dataStr = trimmed.slice(6);
                if (dataStr === '[DONE]') continue;
                try {
                  const parsed = JSON.parse(dataStr);
                  const choice = parsed.choices?.[0];
                  if (!choice) continue;

                  if (choice.finish_reason) {
                    finishReason = choice.finish_reason;
                  }

                  const delta = choice.delta;
                  if (!delta) continue;

                  if (delta.reasoning_content) {
                    fullReasoning += delta.reasoning_content;
                  }
                  if (delta.content) {
                    fullContent += delta.content;
                    if (!fullReasoning && fullContent.includes('</think>')) {
                      const parts = fullContent.split('</think>');
                      fullReasoning = parts[0].replace(/^<think>/, '').trim();
                      fullContent = parts[1].replace(/^\n+/, '');
                    }
                  }

                  // Accumulate streamed tool calls
                  if (delta.tool_calls) {
                    for (const tc of delta.tool_calls) {
                      const idx = tc.index ?? 0;
                      if (!streamedToolCalls[idx]) {
                        streamedToolCalls[idx] = {
                          id: tc.id || `call_${Date.now()}_${idx}`,
                          name: '',
                          arguments: ''
                        };
                      }
                      if (tc.id) streamedToolCalls[idx].id = tc.id;
                      if (tc.function?.name) streamedToolCalls[idx].name += tc.function.name;
                      if (tc.function?.arguments) streamedToolCalls[idx].arguments += tc.function.arguments;
                    }
                  }

                  // Update UI reactively
                  setMessages(prev => prev.map(m => {
                    if (m.id === assistantMsgId) {
                      return {
                        ...m,
                        content: fullContent,
                        reasoning: fullReasoning,
                        harnessTrace: [...accumulatedTrace]
                      };
                    }
                    return m;
                  }));
                } catch (e) {
                  // Ignore JSON chunks parse errors
                }
              }
            }
          }
        }

        const toolCallsArray = Object.values(streamedToolCalls).filter(tc => tc.name);

        if (toolCallsArray.length > 0) {
          // Model decided to call tools!
          // 1. Append assistant call to payload
          payloadMessages.push({
            role: 'assistant',
            content: fullContent,
            tool_calls: toolCallsArray.map(tc => ({
              id: tc.id,
              type: 'function',
              function: {
                name: tc.name,
                arguments: tc.arguments
              }
            }))
          });

          // 2. Execute each tool in the harness sandbox
          for (const tc of toolCallsArray) {
            let parsedArgs = {};
            try {
              parsedArgs = JSON.parse(tc.arguments || '{}');
            } catch (err) {
              parsedArgs = {};
            }

            // Execute in harness sandbox
            const toolExecResult = await executeHarnessTool(tc.name, parsedArgs, toolContext);
            accumulatedTrace.push(toolExecResult);

            // Update UI with latest harness step
            setMessages(prev => prev.map(m => {
              if (m.id === assistantMsgId) {
                return {
                  ...m,
                  harnessTrace: [...accumulatedTrace]
                };
              }
              return m;
            }));

            // 3. Append tool observation message to payload
            payloadMessages.push({
              role: 'tool',
              tool_call_id: tc.id,
              name: tc.name,
              content: JSON.stringify(toolExecResult.result || { error: toolExecResult.error })
            });
          }

          // Prepare clean separation between pre-tool thought and final synthesis
          if (fullContent && !fullContent.endsWith('\n\n')) {
            fullContent += '\n\n';
          }

          // Continue loop to let the model synthesize the observation!
          shouldContinue = true;
        } else {
          // No more tool calls, model provided final synthesis!
          shouldContinue = false;
        }
      }
    } catch (err) {
      if (err.name === 'AbortError') return;

      // Graceful Autonomous Harness Fallback
      const fallbackResult = await executeLocalHarnessDiagnostic(text);
      fullContent = fallbackResult.text;
      accumulatedTrace.push(...fallbackResult.trace);

      setMessages(prev => prev.map(m => {
        if (m.id === assistantMsgId) {
          return {
            ...m,
            content: fallbackResult.text,
            harnessTrace: [...accumulatedTrace],
            isFallback: !usedLiveLlm
          };
        }
        return m;
      }));
    } finally {
      setIsLoading(false);
      abortControllerRef.current = null;

      // Automatically derive interactive console action buttons
      setMessages(prev => prev.map(m => {
        if (m.id === assistantMsgId && (!m.actions || m.actions.length === 0)) {
          return {
            ...m,
            actions: deriveActionsFromContent(m.content)
          };
        }
        return m;
      }));
    }
  };

  const toggleReasoning = (id) => {
    setExpandedReasoningIds(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const toggleTrace = (id) => {
    setExpandedTraceIds(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const toggleStepObs = (stepKey) => {
    setExpandedStepObsIds(prev => ({ ...prev, [stepKey]: !prev[stepKey] }));
  };

  if (!isOpen) {
    const style = btnPos
      ? { left: `${btnPos.x}px`, top: `${btnPos.y}px` }
      : { right: '24px', bottom: '96px' };

    return (
      <button
        ref={btnRef}
        type="button"
        onPointerDown={handleBtnPointerDown}
        onClick={handleBtnClick}
        style={style}
        className={`fixed z-40 flex items-center space-x-2 px-3.5 py-2.5 rounded-full bg-linear-to-r from-indigo-600 via-purple-600 to-indigo-700 hover:from-indigo-500 hover:to-purple-500 text-white shadow-xl shadow-indigo-500/25 hover:shadow-indigo-500/40 border border-white/20 select-none touch-none ${
          isBtnDragging ? 'cursor-grabbing scale-105 opacity-90 shadow-2xl' : 'cursor-grab hover:scale-[1.02] transition-transform transition-shadow duration-200'
        }`}
        title={isZh ? 'Airoute AI 智能副驾驶 (按住可拖动位置，点击打开，快捷键 Cmd+J)' : 'Airoute AI Copilot (Drag to move, click to open, Cmd+J)'}
      >
        <GripVertical className="w-3.5 h-3.5 text-white/50 -ml-1 mr-0.5 shrink-0" />
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
      ref={modalRef}
      style={!isExpanded && modalPos ? { left: `${modalPos.x}px`, top: `${modalPos.y}px`, bottom: 'auto', right: 'auto' } : undefined}
      className={`fixed z-50 transition-all duration-300 ease-out flex flex-col ${
        isExpanded
          ? 'inset-4 sm:inset-10 bg-white dark:bg-[#0F1420] rounded-2xl shadow-2xl border border-slate-200 dark:border-slate-800'
          : `${modalPos ? '' : 'bottom-6 right-6'} w-full max-w-[420px] h-[640px] max-h-[88vh] bg-white dark:bg-[#0F1420] rounded-2xl shadow-2xl border border-slate-200 dark:border-slate-800`
      } overflow-hidden animate-in zoom-in-95`}
    >
      {/* 头部控制栏 (可按住拖动) */}
      <div
        onPointerDown={handleModalHeaderPointerDown}
        className={`px-4 py-3 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between bg-white dark:bg-[#0F1420] ${
          isExpanded ? '' : 'cursor-grab active:cursor-grabbing select-none'
        }`}
        title={isExpanded ? '' : (isZh ? '按住此处可拖动窗口位置' : 'Drag header to move window')}
      >
        <div className="flex items-center space-x-2.5 min-w-0">
          <div className="w-8 h-8 rounded-xl bg-indigo-600 flex items-center justify-center text-white shadow-sm shrink-0">
            <Sparkles className="w-4 h-4 text-amber-300" />
          </div>
          <div className="min-w-0">
            <div className="flex items-center space-x-2">
              <h3 className="font-semibold text-sm text-slate-900 dark:text-slate-100 truncate">
                {isZh ? 'Airoute 智能副驾驶' : 'Airoute AI Copilot'}
              </h3>
              <span className="px-1.5 py-0.2 rounded-full text-[9px] font-mono font-semibold bg-indigo-50 dark:bg-indigo-950/60 text-indigo-600 dark:text-indigo-400 border border-indigo-200/60 dark:border-indigo-800/60 shrink-0">
                Harness
              </span>
            </div>

            {/* 模型与工具契约展示 */}
            <div className="flex items-center gap-2 mt-0.5 text-[11px] text-slate-400">
              <div className="flex items-center gap-1">
                <Cpu className="w-3 h-3 text-indigo-500 shrink-0" />
                {candidateModels.length > 0 ? (
                  <select
                    value={selectedModel}
                    onChange={(e) => setSelectedModel(e.target.value)}
                    className="bg-transparent text-slate-600 dark:text-slate-300 font-mono text-[11px] border-none focus:outline-none cursor-pointer truncate max-w-[130px]"
                    title={isZh ? '切换副驾驶驱动模型' : 'Switch Copilot Model'}
                  >
                    {candidateModels.map(m => (
                      <option key={m} value={m} className="bg-white dark:bg-slate-900 text-slate-900 dark:text-white">
                        {m}
                      </option>
                    ))}
                  </select>
                ) : (
                  <span className="font-mono text-slate-500 text-[10px]">auto-telemetry</span>
                )}
              </div>

              <span className="text-slate-300 dark:text-slate-700">•</span>

              <button
                type="button"
                onClick={() => setShowToolsModal(!showToolsModal)}
                className="inline-flex items-center gap-1 text-[10px] text-indigo-600 dark:text-indigo-400 hover:underline cursor-pointer"
                title={isZh ? '查看装载的标准工具契约' : 'View Registered Harness Tools'}
              >
                <Wrench className="w-2.5 h-2.5" />
                <span>{COPILOT_HARNESS_TOOLS.length} 工具就绪</span>
              </button>
            </div>
          </div>
        </div>

        <div className="flex items-center space-x-1 shrink-0">
          <button
            onClick={handleClearHistory}
            className="p-1.5 text-slate-400 hover:text-rose-500 dark:hover:text-rose-400 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition"
            title={isZh ? '清空会话' : 'Clear Chat'}
          >
            <Trash2 className="w-4 h-4" />
          </button>
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

      {/* 标准工具契约抽屉 (Tools Schema Modal) */}
      {showToolsModal && (
        <div className="px-4 py-2.5 bg-indigo-50/90 dark:bg-slate-900/95 border-b border-indigo-100 dark:border-indigo-950 text-xs text-slate-700 dark:text-slate-300 animate-in slide-in-from-top-2">
          <div className="flex items-center justify-between mb-1.5">
            <div className="flex items-center gap-1.5 font-semibold text-indigo-950 dark:text-indigo-300 text-[11px]">
              <Layers className="w-3.5 h-3.5 text-indigo-500" />
              <span>{isZh ? 'Harness 契约工具库 (OpenAI Tools v1)' : 'Harness Tool Registry'}</span>
            </div>
            <button
              onClick={() => setShowToolsModal(false)}
              className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
          <div className="space-y-1.5 max-h-36 overflow-y-auto pr-1 text-[11px]">
            {COPILOT_HARNESS_TOOLS.map((t, idx) => (
              <div key={idx} className="p-1.5 rounded-lg bg-white/80 dark:bg-slate-800/80 border border-indigo-100 dark:border-slate-700">
                <div className="font-mono font-semibold text-indigo-600 dark:text-indigo-400 text-[10px]">
                  {t.function.name}
                </div>
                <div className="text-[10px] text-slate-600 dark:text-slate-400 line-clamp-1">
                  {t.function.description}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 消息对话容器 */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4 text-xs">
        {messages.map((m) => {
          const isUser = m.role === 'user';
          const isReasoningOpen = !!expandedReasoningIds[m.id];
          const isTraceOpen = !!expandedTraceIds[m.id];
          const traceSteps = m.harnessTrace || [];

          return (
            <div
              key={m.id}
              className={`flex items-start space-x-2.5 ${isUser ? 'flex-row-reverse space-x-reverse' : ''}`}
            >
              <div
                className={`w-7 h-7 rounded-xl flex items-center justify-center shrink-0 shadow-2xs ${
                  isUser
                    ? 'bg-slate-900 dark:bg-slate-700 text-white'
                    : 'bg-indigo-600 text-white'
                }`}
              >
                {isUser ? <User className="w-3.5 h-3.5" /> : <Bot className="w-3.5 h-3.5" />}
              </div>

              <div
                className={`max-w-[88%] rounded-2xl p-4 leading-relaxed relative group ${
                  isUser
                    ? 'bg-indigo-600 text-white rounded-tr-xs shadow-xs'
                    : 'bg-slate-50 dark:bg-slate-900/90 border border-slate-200/80 dark:border-slate-800 text-slate-800 dark:text-slate-200 rounded-tl-xs shadow-xs'
                }`}
              >
                {!isUser && (
                  <button
                    onClick={() => handleCopy(m.content, m.id)}
                    className="absolute top-2.5 right-2.5 opacity-0 group-hover:opacity-100 p-1.5 rounded-lg bg-white dark:bg-slate-800 text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 shadow-2xs transition border border-slate-200/60 dark:border-slate-700 cursor-pointer"
                    title={isZh ? '复制内容' : 'Copy'}
                  >
                    {copiedId === m.id ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                  </button>
                )}

                {/* 离线降级提示：本地模板不是实时遥测 */}
                {!isUser && m.isFallback && (
                  <div className="mb-3 rounded-xl border border-amber-300/80 dark:border-amber-700/60 bg-amber-50 dark:bg-amber-950/40 px-3 py-2 text-[11px] leading-relaxed text-amber-900 dark:text-amber-200">
                    {isZh
                      ? '⚠️ 离线本地模板（非实时）：模型不可用，以下内容由静态诊断生成，未读取实时集群指标，请勿据此直接做运维决策。'
                      : '⚠️ Offline template (not live): the model was unavailable, so this reply comes from a static diagnostic and does not reflect live cluster metrics.'}
                  </div>
                )}

                {/* 深度思考过程 (Thinking Process) 折叠卡片 */}
                {!isUser && m.reasoning && (
                  <div className="mb-3 rounded-xl border border-amber-200/80 dark:border-amber-800/60 bg-amber-50/70 dark:bg-amber-950/30 overflow-hidden">
                    <button
                      type="button"
                      onClick={() => toggleReasoning(m.id)}
                      className="w-full flex items-center justify-between px-3 py-1.5 text-[11px] font-semibold text-amber-900 dark:text-amber-300 hover:bg-amber-100/50 transition cursor-pointer"
                    >
                      <div className="flex items-center space-x-1.5">
                        <Sparkles className="w-3 h-3 text-amber-500 animate-pulse" />
                        <span>{isZh ? '深度思考链 (Thinking Process)' : 'Thinking Process'}</span>
                      </div>
                      {isReasoningOpen ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                    </button>
                    {isReasoningOpen && (
                      <div className="px-3 py-2 border-t border-amber-200/60 dark:border-amber-800/40 text-[11px] font-mono leading-relaxed text-amber-950/80 dark:text-amber-200/80 whitespace-pre-wrap max-h-48 overflow-y-auto">
                        {m.reasoning}
                      </div>
                    )}
                  </div>
                )}

                {/* 标准 Agent Harness 执行轨迹 (Tool Execution Steps) */}
                {!isUser && traceSteps.length > 0 && (
                  <div className="mb-3 rounded-xl border border-indigo-200/80 dark:border-indigo-900/60 bg-indigo-50/50 dark:bg-indigo-950/20 overflow-hidden">
                    <button
                      type="button"
                      onClick={() => toggleTrace(m.id)}
                      className="w-full flex items-center justify-between px-3 py-1.5 text-[11px] font-semibold text-indigo-950 dark:text-indigo-300 hover:bg-indigo-100/40 transition cursor-pointer"
                    >
                      <div className="flex items-center space-x-1.5">
                        <Terminal className="w-3 h-3 text-indigo-500" />
                        <span>{isZh ? `Harness 执行轨迹 (${traceSteps.length} 步工具调用)` : `Harness Trace (${traceSteps.length} tools)`}</span>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-[10px] font-mono text-indigo-500/80">
                          {traceSteps.reduce((acc, s) => acc + (s.duration_ms || 0), 0)}ms
                        </span>
                        {isTraceOpen ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                      </div>
                    </button>

                    {isTraceOpen && (
                      <div className="px-3 py-2 border-t border-indigo-200/60 dark:border-indigo-900/40 space-y-2 max-h-60 overflow-y-auto">
                        {traceSteps.map((step, sIdx) => {
                          const stepKey = `${m.id}_step_${sIdx}`;
                          const isObsOpen = !!expandedStepObsIds[stepKey];
                          const isSuccess = step.status === 'success';

                          return (
                            <div key={sIdx} className="rounded-lg bg-white/90 dark:bg-slate-900/90 border border-slate-200 dark:border-slate-800 p-2 text-[10px]">
                              <div className="flex items-center justify-between">
                                <div className="flex items-center space-x-1.5 font-mono font-medium">
                                  {isSuccess ? (
                                    <CheckCircle2 className="w-3 h-3 text-emerald-500" />
                                  ) : (
                                    <AlertTriangle className="w-3 h-3 text-rose-500" />
                                  )}
                                  <span className="text-indigo-600 dark:text-indigo-400 font-semibold">{step.tool}</span>
                                </div>
                                <div className="flex items-center gap-1 font-mono text-slate-400 text-[9px]">
                                  <Clock className="w-2.5 h-2.5" />
                                  <span>{step.duration_ms || 0}ms</span>
                                </div>
                              </div>

                              {/* 参数概览 */}
                              {step.args && Object.keys(step.args).length > 0 && (
                                <div className="mt-1 text-slate-500 dark:text-slate-400 font-mono text-[9px] truncate">
                                  args: {JSON.stringify(step.args)}
                                </div>
                              )}

                              {/* 观察结果 (Observation) 折叠 */}
                              <div className="mt-1.5 pt-1 border-t border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
                                <button
                                  type="button"
                                  onClick={() => toggleStepObs(stepKey)}
                                  className="text-[9px] text-indigo-500 hover:text-indigo-600 flex items-center gap-0.5 cursor-pointer"
                                >
                                  <span>{isObsOpen ? (isZh ? '收起观测数据' : 'Hide Observation') : (isZh ? '查看观测数据 (Observation)' : 'View Observation')}</span>
                                  {isObsOpen ? <ChevronDown className="w-2.5 h-2.5" /> : <ChevronRight className="w-2.5 h-2.5" />}
                                </button>
                              </div>

                              {isObsOpen && (
                                <pre className="mt-1 p-2 rounded bg-slate-950 text-slate-200 font-mono text-[9px] overflow-x-auto max-h-36">
                                  {JSON.stringify(step.result || step.error, null, 2)}
                                </pre>
                              )}
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </div>
                )}

                {/* 消息正文 */}
                {isUser ? (
                  <div className="whitespace-pre-wrap font-sans text-xs leading-relaxed">{m.content}</div>
                ) : (
                  <MarkdownRenderer content={m.content} />
                )}

                {/* 交互式操作引导按钮 */}
                {m.actions && m.actions.length > 0 && (
                  <div className="mt-3.5 pt-2.5 border-t border-slate-200/60 dark:border-slate-800/80 flex flex-wrap gap-2">
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
                        className="px-3 py-1.5 rounded-xl bg-indigo-50 hover:bg-indigo-100 dark:bg-indigo-950/60 dark:hover:bg-indigo-900/80 text-indigo-700 dark:text-indigo-300 font-semibold text-[11px] flex items-center space-x-1.5 transition border border-indigo-200/60 dark:border-indigo-800 cursor-pointer shadow-2xs"
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
          <div className="flex items-center space-x-2 text-slate-400 text-xs py-2 px-1">
            <RefreshCw className="w-3.5 h-3.5 animate-spin text-indigo-500" />
            <span>{isZh ? 'Harness 正在调度工具与推理生成中...' : 'Harness is executing tools and reasoning...'}</span>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* 标准 Harness Benchmark 基准任务快捷栏 */}
      <div className="px-3 py-2 bg-slate-50/70 dark:bg-slate-900/50 border-t border-slate-100 dark:border-slate-800/80 flex items-center gap-2 overflow-x-auto text-[11px] no-scrollbar">
        {HARNESS_BENCHMARKS.map((bm) => (
          <button
            key={bm.id}
            onClick={() => handleSend(bm.prompt)}
            disabled={isLoading}
            className="px-2.5 py-1.5 rounded-xl bg-white dark:bg-slate-800 hover:bg-indigo-50 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 hover:text-indigo-600 dark:hover:text-indigo-300 border border-slate-200/80 dark:border-slate-700 whitespace-nowrap transition cursor-pointer font-medium shadow-2xs disabled:opacity-50 flex items-center gap-1.5"
            title={bm.description}
          >
            <span className="px-1 py-0.2 rounded text-[9px] font-mono bg-indigo-50 dark:bg-indigo-950/60 text-indigo-600 dark:text-indigo-400 border border-indigo-200/60 dark:border-indigo-800/60 font-semibold">
              {bm.code}
            </span>
            <span>{isZh ? bm.label : bm.enLabel}</span>
          </button>
        ))}
      </div>

      {/* 底部输入栏 */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          handleSend();
        }}
        className="p-3 border-t border-slate-200/80 dark:border-slate-800 bg-white dark:bg-[#0F1420] flex items-center space-x-2"
      >
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={isZh ? '向 Harness 发出运维巡检、排错或架构指令...' : 'Issue an SRE audit or ops instruction to Harness...'}
          className="flex-1 bg-slate-50 dark:bg-slate-900/80 border border-slate-200 dark:border-slate-800 rounded-xl px-3.5 py-2 text-xs text-slate-900 dark:text-slate-100 placeholder-slate-400 focus:outline-none focus:ring-1 focus:ring-indigo-500 shadow-2xs"
        />

        {isLoading ? (
          <button
            type="button"
            onClick={handleStopGeneration}
            className="p-2 rounded-xl bg-rose-600 hover:bg-rose-500 text-white transition cursor-pointer shadow-sm"
            title={isZh ? '中断任务' : 'Abort task'}
          >
            <StopCircle className="w-4 h-4" />
          </button>
        ) : (
          <button
            type="submit"
            disabled={!input.trim()}
            className="p-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white transition cursor-pointer shadow-md shadow-indigo-600/20"
            title={isZh ? '发送' : 'Send'}
          >
            <Send className="w-4 h-4" />
          </button>
        )}
      </form>
    </div>
  );
}
