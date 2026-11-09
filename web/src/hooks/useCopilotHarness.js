import { useMemo } from 'react';
import { executeHarnessTool } from '../components/copilotHarness';

export default function useCopilotHarness({
  adminFetch, baseUrl, channels, isZh, keys, logs, modelRoutes, models, origin, prices, setCurrentTab,
  setViewMode, showToast, stats
}) {
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
      const metrics = res.performance_metrics;
      // "No samples" must read as such instead of a made-up latency figure.
      const ttftText = metrics.avg_ttft_ms == null
        ? (isZh ? '无样本' : 'n/a')
        : `${metrics.avg_ttft_ms} ms`;

      return {
        trace,
        text: `### 🩺 Airoute 网关集群深度健康巡检报告\n\n**集群健康评分**：\`${res.cluster_summary.health_score} / 100\`\n\n- **上游服务商渠道**：共接入 **${res.cluster_summary.total_channels}** 个（活跃 **${res.cluster_summary.active_channels}** 个，熔断隔离 **${trippedCount}** 个）\n- **模型挂载总数**：当前已上线 **${res.cluster_summary.mounted_models_count}** 个异构模型\n- **性能基线指标**：平均首字延迟 (TTFT) 为 **${ttftText}**，缓存命中率 **${metrics.cache_hit_rate}**\n- **累计消费**：**${metrics.currency} ${Number(metrics.total_cost || 0).toFixed(4)}**\n- **高可用保障**：${res.cluster_summary.ha_status}\n\n${trippedCount > 0 ? `> [!WARNING]\n> 检测到有 ${trippedCount} 个上游通道处于熔断保护状态。建议前往【通道管理】执行网络探针核验并检查上游配额。` : `> [!TIP]\n> 当前所有上游通道均处于健康活跃状态，首字延迟与吞吐基线平稳。`}`
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
  return {
    buildHarnessSystemPrompt, deriveActionsFromContent, executeLocalHarnessDiagnostic, toolContext
  };
}
