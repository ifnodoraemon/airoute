/**
 * copilotHarness.js
 * Standard Agent Harness for Airoute AI Copilot
 * Conforming to OpenAI Function Calling / OpenCode / Codex / Gemini / DeepSeek tool protocol.
 */

import { API_KEY_PLACEHOLDER } from '../lib/apiKey';

// 1. Formal Tool Contracts (OpenAI Standard Schema)
export const COPILOT_HARNESS_TOOLS = [
  {
    type: 'function',
    function: {
      name: 'get_cluster_telemetry',
      description: '获取 Airoute 网关当前集群的实时遥测快照，包括通道运行状态、熔断隔离、挂载模型列表、首字延迟(TTFT)、缓存命中率及总消费成本',
      parameters: {
        type: 'object',
        properties: {
          include_channels: {
            type: 'boolean',
            description: '是否包含各上游服务商渠道的详细状态与优先级配置，默认 true'
          },
          include_metrics: {
            type: 'boolean',
            description: '是否包含 TTFT、缓存命中率和请求量等统计指标，默认 true'
          }
        }
      }
    }
  },
  {
    type: 'function',
    function: {
      name: 'diagnose_recent_errors',
      description: '查询并深度分析网关近期异常请求审计日志（HTTP 状态码 >= 400），提取报错分布、错误特征及根本原因(RCA)线索',
      parameters: {
        type: 'object',
        properties: {
          status_code_filter: {
            type: 'number',
            description: '按指定 HTTP 状态码过滤（如 429 速率限制、500 服务端异常、502 网关错误、504 超时）'
          },
          limit: {
            type: 'number',
            description: '返回的异常日志条数上限，默认 5 条'
          }
        }
      }
    }
  },
  {
    type: 'function',
    function: {
      name: 'audit_routing_spof',
      description: '全面审计网关的模型路由拓扑架构与计费费率对应关系，检测单点故障 (SPOF) 风险以及路由与定价规则是否一一匹配',
      parameters: {
        type: 'object',
        properties: {
          target_model: {
            type: 'string',
            description: '指定待审计的模型名称。如果不填，则对当前挂载的全量模型路由进行全量审计'
          }
        }
      }
    }
  },
  {
    type: 'function',
    function: {
      name: 'probe_upstream_channel',
      description: '巡检并探查指定或全部上游服务商通道的断路器熔断隔离状态、权重配比、优先级及关联的模型映射',
      parameters: {
        type: 'object',
        properties: {
          channel_name: {
            type: 'string',
            description: '目标上游通道名称（如 aliyun-bailian、volcengine、deepseek-official）。如果不填则探查全部通道'
          }
        }
      }
    }
  },
  {
    type: 'function',
    function: {
      name: 'navigate_console_view',
      description: '在 Airoute 前端控制台中自动导航切换视图模块（如通道管理、模型路由、定价费率、审计日志、对账流水、Playground、API密钥、MCP广场、SkillHub 等）',
      parameters: {
        type: 'object',
        properties: {
          target_tab: {
            type: 'string',
            enum: ['dashboard', 'models', 'pricing', 'channels', 'reconciliation', 'keys', 'logs', 'skills', 'mcp', 'users', 'wallet', 'playground', 'docs'],
            description: '目标视图模块标识'
          },
          reason: {
            type: 'string',
            description: '导航跳转至该模块的原因或上下文说明'
          }
        },
        required: ['target_tab']
      }
    }
  },
  {
    type: 'function',
    function: {
      name: 'generate_client_config',
      description: '为现代 AI 编程智能体与客户端（Cursor IDE、Claude Code CLI、Python OpenAI SDK、cURL）生成开箱即用的接入配置和环境变量',
      parameters: {
        type: 'object',
        properties: {
          client_type: {
            type: 'string',
            enum: ['cursor', 'claude_code', 'python', 'curl', 'all'],
            description: '目标客户端类型'
          },
          preferred_model: {
            type: 'string',
            description: '指定优先绑定的模型标识'
          }
        },
        required: ['client_type']
      }
    }
  }
];

// 2. Standard Benchmark / Task Presets (SWE-bench / DeepSeek Eval Suite style)
export const HARNESS_BENCHMARKS = [
  {
    id: 'sre_health_audit',
    code: 'SRE-01',
    label: '🩺 集群健康巡检',
    enLabel: '🩺 Cluster SRE Audit',
    description: '自动执行 get_cluster_telemetry 与 probe_upstream_channel，评估通道健康与熔断状态',
    prompt: '请启动集群全链路 SRE 巡检任务：获取当前所有上游通道的活跃度、断路器熔断状态和首字延迟(TTFT)，并给出健康度综合评估。'
  },
  {
    id: 'spof_topology_audit',
    code: 'SPOF-02',
    label: '💡 SPOF 拓扑审计',
    enLabel: '💡 Routing SPOF Audit',
    description: '自动执行 audit_routing_spof，扫描全量模型路由，识别缺乏备份服务商的脆弱模型与计费闭环',
    prompt: '请执行模型路由单点故障 (SPOF) 深度审计：核验挂载的各个模型是否具备主备容灾渠道，指出高危模型并提供主备优先级与故障转移优化建议。'
  },
  {
    id: 'finops_pricing_audit',
    code: 'FINOPS-03',
    label: '💰 路由与定价匹配',
    enLabel: '💰 Route-Price Match',
    description: '核验模型路由与模型定价是否一一匹配，排查未挂载路由的定价和未配定价的路由',
    prompt: '请核验当前网关的模型路由与计费定价是否一一匹配：检查哪些路由未配置专属费率（正在按基准价计费）、哪些定价规则未挂载路由（无法分发使用），并给出闭环修复方案。'
  },
  {
    id: 'error_rca_diagnostic',
    code: 'RCA-04',
    label: '⚠️ 异常报错根因剖析',
    enLabel: '⚠️ Error RCA Diagnostic',
    description: '自动执行 diagnose_recent_errors，聚合 4xx/5xx 报错特征，定位 429 限流或 504 超时根因',
    prompt: '请分析网关近期记录的异常报错日志，按状态码分类归纳，并重点剖析 429 速率限制或 5xx 故障的根本原因，给出修复配置指引。'
  },
  {
    id: 'agent_onboarding_suite',
    code: 'SETUP-05',
    label: '🚀 Cursor / Claude 接入',
    enLabel: '🚀 Client Setup Suite',
    description: '自动执行 generate_client_config，生成 Cursor、Claude Code 与 Python 统一配置',
    prompt: '请为我生成 Cursor IDE、Claude Code CLI 以及 Python SDK 的全套标准化接入配置与环境变量说明。'
  }
];

// 3. Harness Tool Execution Sandbox
export async function executeHarnessTool(toolName, args = {}, context = {}) {
  const startTime = performance.now();
  const {
    channels = [],
    models = [],
    modelRoutes = [],
    logs = [],
    stats = {},
    prices = [],
    origin = window.location.origin,
    baseUrl = `${window.location.origin}/v1`,
    setCurrentTab,
    setViewMode,
    showToast,
    isZh = true
  } = context;

  let safeArgs = {};
  if (typeof args === 'string') {
    try {
      safeArgs = JSON.parse(args || '{}');
    } catch {
      safeArgs = {};
    }
  } else if (args && typeof args === 'object') {
    safeArgs = args;
  }

  try {
    switch (toolName) {
      case 'get_cluster_telemetry': {
        const totalChans = channels.length;
        const activeChans = channels.filter(c => c.status === 'active').length;
        // The gateway serialises upstream breaker state as "OPEN"/"HALF-OPEN"/
        // "CLOSED" (router/circuit_breaker.go). Comparing it case-sensitively to
        // 'open' never matched, so the copilot used to report a perfect health score
        // while channels were actually tripped.
        const breakerOf = (c) => String(c?.breaker_status || '').toUpperCase();
        const trippedChans = channels.filter(c => breakerOf(c) === 'OPEN' || breakerOf(c) === 'HALF-OPEN' || breakerOf(c) === 'TRIPPED').length;
        // Cache-hit rate is not a field the stats endpoint returns; it is derived
        // from the cached prompt tokens. No prompt tokens means no sample, not 0%.
        const promptTokens = Number(stats?.prompt_tokens) || 0;
        const cachedTokens = Number(stats?.total_cached_tokens) || 0;
        const cacheHitRate = promptTokens > 0 ? `${((cachedTokens / promptTokens) * 100).toFixed(1)}%` : 'n/a';
        // TTFT: 0 means "no samples yet", which must stay distinguishable from a
        // measured zero instead of being filled with an invented 120 ms baseline.
        const ttftSamples = Number(stats?.ttft_samples) || 0;
        const avgTtft = ttftSamples > 0 || Number(stats?.avg_ttft_ms) > 0 ? Number(stats.avg_ttft_ms) : null;

        const result = {
          timestamp: new Date().toISOString(),
          cluster_summary: {
            total_channels: totalChans,
            active_channels: activeChans,
            tripped_channels: trippedChans,
            health_score: totalChans > 0 ? Math.round(((totalChans - trippedChans) / totalChans) * 100) : 100,
            mounted_models_count: models.length,
            active_routes_count: modelRoutes.length,
            ha_status: 'Dual-Gateway Load Balanced Active'
          },
          performance_metrics: {
            total_requests: stats?.total_requests || 0,
            // null, not a fabricated number: the report says "no samples".
            avg_ttft_ms: avgTtft,
            ttft_samples: ttftSamples,
            cache_hit_rate: cacheHitRate,
            // Wallet and billing amounts are CNY everywhere in this gateway; naming
            // the value "usd" made every report off by a currency.
            total_cost: Number(stats?.total_cost) || 0,
            currency: 'CNY'
          }
        };

        if (safeArgs.include_channels !== false) {
          result.channels = channels.map(c => ({
            name: c.name,
            provider: c.provider,
            status: c.status,
            breaker_status: c.breaker_status || 'closed',
            priority: c.priority ?? 1,
            weight: c.weight ?? 100,
            models: c.models || []
          }));
        }

        const duration_ms = Math.round(performance.now() - startTime);
        return {
          status: 'success',
          tool: toolName,
          args: safeArgs,
          duration_ms,
          result
        };
      }

      case 'diagnose_recent_errors': {
        const limit = safeArgs.limit || 5;
        const filterCode = safeArgs.status_code_filter ? Number(safeArgs.status_code_filter) : null;

        let filteredLogs = (logs || []).filter(l => l.status_code >= 400);
        if (filterCode) {
          filteredLogs = filteredLogs.filter(l => l.status_code === filterCode);
        }

        const distribution = {};
        filteredLogs.forEach(l => {
          const code = l.status_code || 500;
          distribution[code] = (distribution[code] || 0) + 1;
        });

        const samples = filteredLogs.slice(0, limit).map(l => ({
          timestamp: l.created_at,
          status_code: l.status_code,
          model: l.model || 'unknown',
          channel: l.channel || 'direct',
          error_message: l.error_message || 'Upstream Error Response'
        }));

        const result = {
          total_error_count: filteredLogs.length,
          status_code_distribution: distribution,
          error_samples: samples,
          common_rca_patterns: [
            distribution[429] ? 'HTTP 429: 上游提供商触发 TPM/RPM 速率限制，需增加备用提供商通道或配置平滑重试' : null,
            distribution[504] ? 'HTTP 504: 上游网关推理响应超时，建议提升超时时间限制或开启流式首字保活' : null,
            distribution[400] ? 'HTTP 400: 请求上下文超出上游模型窗口上限或参数不兼容' : null,
            distribution[502] ? 'HTTP 502: 上游 API 节点网络震荡或提供商暂时性宕机' : null
          ].filter(Boolean)
        };

        const duration_ms = Math.round(performance.now() - startTime);
        return {
          status: 'success',
          tool: toolName,
          args: safeArgs,
          duration_ms,
          result
        };
      }

      case 'audit_routing_spof': {
        const targetModel = safeArgs.target_model;
        let routesToAudit = modelRoutes || [];
        if (targetModel) {
          routesToAudit = routesToAudit.filter(r => r.model === targetModel);
        }

        const spofVulnerable = [];
        const redundantSafe = [];

        // Provider rows from /api/v1/admin/models/routes name the channel
        // `channel_name` (router.ModelProviderInfo) — not `channel`. Reading the
        // wrong field made every advisory name the fictional channel "default".
        const providerChannel = (p) => p?.channel_name || p?.name || (p?.channel_id != null ? `channel#${p.channel_id}` : 'unknown');

        routesToAudit.forEach(r => {
          const providerCount = (r.providers || []).length;
          if (providerCount <= 1) {
            spofVulnerable.push({
              model: r.model,
              provider_count: providerCount,
              primary_channel: r.providers?.[0] ? providerChannel(r.providers[0]) : 'no-provider',
              risk_level: 'HIGH (SPOF)',
              recommendation: '建议在模型路由中添加第二上游提供商作为备用通道，并设置优先级 2 及故障转移策略'
            });
          } else {
            redundantSafe.push({
              model: r.model,
              provider_count: providerCount,
              providers: r.providers.map(p => ({ channel: providerChannel(p), priority: p.priority, weight: p.weight }))
            });
          }
        });

        // Cross-check with Pricing Rules (Route <-> Price 1-to-1 matching)
        const unpricedRoutes = routesToAudit.filter(r => !prices.some(p => p.model === r.model || p.model === '*'));
        const unroutedPrices = (prices || []).filter(p => !modelRoutes.some(r => r.model === p.model) && p.model !== '*');

        const result = {
          total_routes_audited: routesToAudit.length,
          spof_vulnerable_count: spofVulnerable.length,
          redundant_safe_count: redundantSafe.length,
          spof_vulnerabilities: spofVulnerable,
          safe_routes_sample: redundantSafe.slice(0, 3),
          pricing_consistency: {
            unpriced_routes_count: unpricedRoutes.length,
            unpriced_routes_sample: unpricedRoutes.slice(0, 4).map(r => r.model),
            unrouted_prices_count: unroutedPrices.length,
            unrouted_prices_sample: unroutedPrices.slice(0, 4).map(p => p.model),
            evaluation: unpricedRoutes.length === 0 && unroutedPrices.length === 0
              ? '✅ 模型路由与计费定价 100% 对应闭环'
              : `⚠️ 计费一致性提示：${unpricedRoutes.length} 个路由未配专属定价（按基准计费），${unroutedPrices.length} 个定价规则未挂载路由（无法分发）`
          },
          remediation_summary: spofVulnerable.length > 0
            ? `发现 ${spofVulnerable.length} 个模型仅依赖单一提供商。一旦上游出现网络故障或 429 限流，请求将直接失败。`
            : '全量模型均已配置多提供商冗余通道，具备高可用容灾能力。'
        };

        const duration_ms = Math.round(performance.now() - startTime);
        return {
          status: 'success',
          tool: toolName,
          args: safeArgs,
          duration_ms,
          result
        };
      }

      case 'probe_upstream_channel': {
        const targetName = safeArgs.channel_name;
        let matchedChannels = channels || [];
        if (targetName) {
          matchedChannels = matchedChannels.filter(c =>
            (c.name || '').toLowerCase().includes(targetName.toLowerCase()) ||
            (c.provider || '').toLowerCase().includes(targetName.toLowerCase())
          );
        }

        const probeResults = matchedChannels.map(c => ({
          name: c.name,
          provider: c.provider,
          status: c.status,
          breaker_status: c.breaker_status || 'CLOSED',
          priority: c.priority ?? 1,
          weight: c.weight ?? 100,
          associated_models_count: (c.models || []).length,
          models_sample: (c.models || []).slice(0, 5),
          // Same casing rule as the telemetry tool: the gateway reports "OPEN".
          health_eval: ['OPEN', 'HALF-OPEN', 'TRIPPED'].includes(String(c.breaker_status || '').toUpperCase())
            ? 'TRIPPED (熔断隔离中)'
            : 'HEALTHY (健康活跃)'
        }));

        const duration_ms = Math.round(performance.now() - startTime);
        return {
          status: 'success',
          tool: toolName,
          args: safeArgs,
          duration_ms,
          result: {
            probed_count: matchedChannels.length,
            channels: probeResults
          }
        };
      }

      case 'navigate_console_view': {
        const targetTab = safeArgs.target_tab;
        const reason = safeArgs.reason || 'AI 智能副驾驶推荐视图导航';

        if (setCurrentTab && targetTab) {
          setCurrentTab(targetTab);
          if (setViewMode) setViewMode('console');
          if (showToast) {
            showToast(isZh ? `已为你切换至【${targetTab}】模块` : `Navigated to [${targetTab}]`, 'info');
          }
        }

        const duration_ms = Math.round(performance.now() - startTime);
        return {
          status: 'success',
          tool: toolName,
          args: safeArgs,
          duration_ms,
          result: {
            navigated_to: targetTab,
            reason,
            status: 'active'
          }
        };
      }

      case 'generate_client_config': {
        const clientType = safeArgs.client_type || 'all';
        // Key rows only carry a public key ID (credentials are stored as digests),
        // so a generated config must not pretend to hold the real credential: the
        // operator pastes it themselves. A key ID in a snippet is unusable and
        // teaches the wrong copy/paste habit.
        const snippetKey = API_KEY_PLACEHOLDER;
        const modelName = safeArgs.preferred_model || models[0] || 'deepseek-v4-flash';

        const configs = {};

        if (clientType === 'cursor' || clientType === 'all') {
          configs.cursor = {
            base_url: baseUrl,
            api_key: snippetKey,
            model: modelName,
            instruction: '在 Cursor Settings -> Models 中添加或覆盖 OpenAI Base URL'
          };
        }

        if (clientType === 'claude_code' || clientType === 'all') {
          configs.claude_code = {
            bash_env: `export ANTHROPIC_BASE_URL="${origin}"\nexport ANTHROPIC_API_KEY="${snippetKey}"\nclaude`,
            instruction: '在终端中配置环境变量后直接运行 claude 命令行'
          };
        }

        if (clientType === 'python' || clientType === 'all') {
          configs.python = {
            code_snippet: `from openai import OpenAI\n\nclient = OpenAI(\n    base_url="${baseUrl}",\n    api_key="${snippetKey}"\n)\n\nresponse = client.chat.completions.create(\n    model="${modelName}",\n    messages=[{"role": "user", "content": "Hello Airoute!"}],\n    stream=True\n)\nfor chunk in response:\n    print(chunk.choices[0].delta.content or "", end="", flush=True)`
          };
        }

        if (clientType === 'curl' || clientType === 'all') {
          configs.curl = {
            command: `curl -X POST "${baseUrl}/chat/completions" \\\n  -H "Authorization: Bearer ${snippetKey}" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "${modelName}",\n    "messages": [{"role": "user", "content": "Ping"}],\n    "stream": false\n  }'`
          };
        }

        const duration_ms = Math.round(performance.now() - startTime);
        return {
          status: 'success',
          tool: toolName,
          args: safeArgs,
          duration_ms,
          result: configs
        };
      }

      default:
        throw new Error(`未知工具指令: ${toolName}`);
    }
  } catch (err) {
    const duration_ms = Math.round(performance.now() - startTime);
    return {
      status: 'error',
      tool: toolName,
      args: safeArgs,
      duration_ms,
      error: err.message || String(err)
    };
  }
}
