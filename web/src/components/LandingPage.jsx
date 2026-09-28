import React, { useState } from 'react';
import {
  Shield,
  Cpu,
  Layers,
  Server,
  Zap,
  Activity,
  Terminal,
  Code,
  Key,
  Check,
  Copy,
  ExternalLink,
  Sparkles,
  ArrowRight,
  Globe,
  Network,
  Clock,
  MessageSquare,
  Image as ImageIcon,
  Volume2,
  Mic,
  Video,
  Moon
} from 'lucide-react';

export default function LandingPage({
  isLoggedIn,
  adminUser,
  onOpenLogin,
  onEnterConsole,
  onViewDocs,
  onViewStatus,
  stats,
  channelsCount,
  modelsCount
}) {
  const [copiedSnippet, setCopiedSnippet] = useState('');
  const [activeSnippetTab, setActiveSnippetTab] = useState('python');

  const origin = window.location.origin || 'http://localhost:8080';

  const copyCode = (text, key) => {
    navigator.clipboard.writeText(text);
    setCopiedSnippet(key);
    setTimeout(() => setCopiedSnippet(''), 2500);
  };

  const pythonSnippet = `from openai import OpenAI

# 1. base_url 指向 Nano-Gateway 标准 /v1 入口，使用在工作台签发的 API 密钥
client = OpenAI(
    base_url="${origin}/v1",
    api_key="sk-nano-your-client-key",
    default_headers={
        # 传入统一会话 ID，网关基于一致性哈希 (FNV-1a) 锁定相同 Provider，复用前缀 KV Cache 享 90% 优惠
        "X-Session-ID": "session_user_2026_001"
    }
)

# 2. 发起 2026 旗舰模型对话推理 (支持打字机流式输出与首字前无感容灾 Pre-Token Fallback)
response = client.chat.completions.create(
    model="gpt-6-astra",  # 亦可指定 claude-opus-5.5 / deepseek-v4.1-flash / gemini-3.8-flash
    messages=[
        {"role": "system", "content": "你是一位资深架构师"},
        {"role": "user", "content": "请分析大模型网关多模态传输时的零缓冲穿透设计与成本优化"}
    ],
    stream=True,
)

for chunk in response:
    print(chunk.choices[0].delta.content or "", end="", flush=True)`;

  const curlSnippet = `# 1. 2026 标准对话补全 (Chat Completions) - 数据面标准 /v1 路径
curl -X POST "${origin}/v1/chat/completions" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -H "X-Session-ID: session_user_2026_001" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-6-astra",
    "messages": [
      {"role": "user", "content": "介绍 Nano-Gateway 2026 旗舰模型矩阵与资源优化方案"}
    ],
    "stream": true
  }'`;

  const claudeSnippet = `import anthropic

# 原生 Claude 协议直连 Nano-Gateway (全双工实时转译引擎)
# 网关自动解析 Claude Messages 协议，并智能路由至 Anthropic 或跨协议转译下游 Provider！
client = anthropic.Anthropic(
    base_url="${origin}",
    api_key="sk-nano-your-client-key",
)

message = client.messages.create(
    model="claude-opus-5.5",  # 2026 全球最强综合推理与 Coding 旗舰模型
    max_tokens=2048,
    messages=[
        {"role": "user", "content": "解释分布式网关的数据面(/v1)与控制面(/api/v1)分层治理架构"}
    ]
)
print(message.content[0].text)`;

  const nodeSnippet = `import OpenAI from 'openai';

// Node.js / TypeScript 2026 旗舰模型极速接入
const openai = new OpenAI({
  baseURL: '${origin}/v1',
  apiKey: 'sk-nano-your-client-key',
  defaultHeaders: {
    'X-Session-ID': 'conv_node_2026' // 启用会话黏连，稳定命中上游 Prefix KV 缓存
  }
});

const stream = await openai.chat.completions.create({
  model: 'deepseek-v4.1-flash',
  messages: [{ role: 'user', content: '介绍大模型网关的多模态分流优化机制' }],
  stream: true,
});

for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content || '');
}`;

  const multimodalSnippet = `# 1. 图像生成 (FLUX.1-Pro 旗舰画质)
curl -X POST "${origin}/v1/images/generations" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "flux-1.1-pro", "prompt": "极简科技风云原生分布式 AI 路由器架构图", "n": 1, "size": "1024x1024"}'

# 2. 视频生成与异步任务轮询 (Sora 2 旗舰影视级生成)
curl -X POST "${origin}/v1/videos/generations" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "sora-2", "prompt": "未来赛博朋克城市雨夜飞车", "aspect_ratio": "16:9"}'

# 轮询视频生成状态直到 SUCCESS 并获取播放下载直链：
curl -X GET "${origin}/v1/videos/tasks/task_xxx" \\
  -H "Authorization: Bearer sk-nano-your-client-key"

# 3. 语音转写 (Whisper-Large-V3-Turbo 极速高精语音转文本)
curl -X POST "${origin}/v1/audio/transcriptions" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -F file="@voice_speech.opus" \\
  -F model="whisper-large-v3-turbo"

# 4. 实时双向语音 (Gemini 3.8 Live / TTS 语音合成)
curl -X POST "${origin}/v1/audio/speech" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "gemini-3.8-live", "input": "Nano-Gateway 企业级高可用网关已就绪", "voice": "alloy"}' \\
  --output speech.mp3`;

  const resourceOptSnippet = `# ==============================================================================
# 多模态超低资源消耗实战 (图片直传 + 语音 Opus 压缩)
# 核心收益：网关带宽/内存开销直降 99%，上游 Vision Token 成本立省 70%！
# ==============================================================================

from openai import OpenAI

client = OpenAI(base_url="${origin}/v1", api_key="sk-nano-your-client-key")

# 1. 图像优化最佳实践：OSS/S3 云存储直传分流 (推荐)
# 客户端将图片直传至云存储 CDN，向网关仅传入 URL，彻底杜绝网关传输与内存膨胀
response = client.chat.completions.create(
    model="gpt-6-astra",
    messages=[{
        "role": "user",
        "content": [
            {"type": "text", "text": "请分析图中的系统拓扑与高可用架构"},
            {
                "type": "image_url",
                # 传入已缩放至 1024px WebP 格式的 CDN 预签名直链 (避免大 Base64 占用数 MB 内存)
                "image_url": {"url": "https://oss-bucket.cdn.domain/arch_1024.webp"}
            }
        ]
    }]
)

# 2. 语音优化最佳实践：采用 16kHz mono Opus 格式传输
# 相比传统 WAV (1.4 Mbps) 体积缩减 95% (仅 16-24 kbps)，且 Whisper 识别率无损！
# curl -X POST "${origin}/v1/audio/transcriptions" \\
#   -H "Authorization: Bearer sk-nano-your-client-key" \\
#   -F file="@audio_16k.opus" \\
#   -F model="whisper-large-v3-turbo"`;

  const sessionSnippet = `# 会话黏连 (Session Affinity) 与 Prompt Caching 降本指南
#
# 核心原理：
# 在客户端请求中携带请求头 X-Session-ID: <唯一会话ID> (或 user 字段)。
# Nano-Gateway 基于 FNV-1a 一致性哈希与 Redis 分布式会话钉选 (30分钟 TTL)，
# 确保多轮会话始终命中同一 Provider 节点，复用远端 Prefix KV Cache。
#
# 收益：
# 1. 首字时延 (TTFT) 降低 80%
# 2. 2026 旗舰模型 (Claude Opus 5.5 / GPT-6 Astra) Prompt 缓存享受高达 90% 计费折扣！`;

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900 flex flex-col font-sans selection:bg-indigo-500 selection:text-white">
      {/* Top Navigation Bar */}
      <header className="sticky top-0 z-40 bg-white/90 backdrop-blur-md border-b border-slate-200/80">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-gradient-to-tr from-indigo-600 to-sky-500 flex items-center justify-center text-white shadow-sm font-black text-lg">
              ⚡
            </div>
            <div>
              <span className="font-extrabold text-lg tracking-tight bg-gradient-to-r from-indigo-700 via-purple-700 to-sky-600 bg-clip-text text-transparent">
                AI 路由器
              </span>
            </div>
          </div>

          <nav className="hidden md:flex items-center space-x-6 text-xs font-semibold text-slate-600">
            <a href="#features" className="hover:text-indigo-600 transition">核心特性</a>
            <a href="#multimodal" className="hover:text-indigo-600 transition">全模态管道</a>
            <a href="#multimodal-optimization" className="hover:text-emerald-600 transition text-emerald-700 font-bold">资源极致优化</a>
            <a href="#pricing" className="hover:text-indigo-600 transition">2026模型定价</a>
            <a href="#docs" className="hover:text-indigo-600 transition font-bold text-indigo-600">URL规范与文档</a>
          </nav>

          <div className="flex items-center space-x-3">
            <button
              type="button"
              onClick={onViewStatus}
              className="flex text-xs px-3 py-1.5 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-medium items-center space-x-1.5 transition border border-slate-200 cursor-pointer"
            >
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
              <span>服务状态</span>
            </button>

            {isLoggedIn ? (
              <button
                onClick={onEnterConsole}
                className="px-4 py-2 bg-gradient-to-r from-indigo-600 to-sky-600 hover:from-indigo-700 hover:to-sky-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition"
              >
                <Server className="w-3.5 h-3.5" />
                <span>进入工作台 →</span>
              </button>
            ) : (
              <button
                onClick={onOpenLogin}
                className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
              >
                <Key className="w-3.5 h-3.5" />
                <span>登录 / 注册</span>
              </button>
            )}
          </div>
        </div>
      </header>

      {/* Hero Section */}
      <section className="relative pt-16 pb-20 px-6 overflow-hidden bg-gradient-to-b from-indigo-50/60 via-white to-slate-50 border-b border-slate-200/60">
        <div className="max-w-5xl mx-auto text-center space-y-6">
          {/* Live Cluster Badge */}
          <div
            onClick={onViewStatus}
            className="inline-flex items-center space-x-2 px-3.5 py-1.5 rounded-full bg-white border border-slate-200 shadow-xs text-xs font-semibold text-slate-700 hover:border-indigo-400 hover:shadow-sm cursor-pointer transition"
            title="点击查看各模型 30 天可用性 SLA 状态"
          >
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            <span>高可用集群健康运行中</span>
            <span className="text-slate-300">|</span>
            <span className="text-emerald-600 font-mono">SLA 99.99%</span>
            <span className="text-slate-300">|</span>
            <span className="text-indigo-600 font-medium flex items-center space-x-1">
              <span>状态详情 →</span>
            </span>
          </div>

          <h1 className="text-4xl sm:text-5xl lg:text-6xl font-black text-slate-900 tracking-tight leading-tight">
            2026 旗舰大模型与多模态<br />
            <span className="bg-gradient-to-r from-indigo-600 via-purple-600 to-sky-600 bg-clip-text text-transparent">
              统一调度与高性能接入网关
            </span>
          </h1>

          <p className="text-base sm:text-lg text-slate-600 max-w-3xl mx-auto leading-relaxed">
            单核数万 QPS 极速分发 · 2026 旗舰大模型全矩阵覆盖 · 多模态零缓冲穿透流与直传降耗 · 闲时 5 折与 Prompt 缓存立省 95%！<br className="hidden sm:inline" />
            统一收口 OpenAI、Claude、Gemini、DeepSeek、GPUStack、vLLM 与全模态生成管线。
          </p>

          <div className="flex flex-col sm:flex-row items-center justify-center gap-3.5 pt-4">
            {isLoggedIn ? (
              <button
                onClick={onEnterConsole}
                className="w-full sm:w-auto px-6 py-3 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-sm font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2"
              >
                <Server className="w-4 h-4" />
                <span>进入工作台</span>
                <ArrowRight className="w-4 h-4" />
              </button>
            ) : (
              <button
                onClick={onOpenLogin}
                className="w-full sm:w-auto px-6 py-3 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-sm font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2"
              >
                <Key className="w-4 h-4" />
                <span>立即登录 / 免费注册</span>
                <ArrowRight className="w-4 h-4" />
              </button>
            )}

            <button
              onClick={onViewDocs}
              className="w-full sm:w-auto px-6 py-3 bg-white hover:bg-slate-50 text-slate-700 border border-slate-200 rounded-2xl text-sm font-semibold shadow-xs transition flex items-center justify-center space-x-2"
            >
              <Code className="w-4 h-4 text-indigo-600" />
              <span>查阅开发者接入指南</span>
            </button>
          </div>

          {/* Quick Metrics Strip */}
          <div className="pt-8 grid grid-cols-2 sm:grid-cols-4 gap-4 max-w-4xl mx-auto text-left">
            <div className="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-xs">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider block">累计请求吞吐</span>
              <span className="text-2xl font-black text-slate-900 font-mono mt-1 block">
                {stats.total_requests || 0}
              </span>
              <span className="text-[11px] text-emerald-600 font-medium">99.99% 可用性保障</span>
            </div>

            <div className="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-xs">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider block">上游模型提供商</span>
              <span className="text-2xl font-black text-emerald-600 font-mono mt-1 block">
                {channelsCount || 0}
              </span>
              <span className="text-[11px] text-slate-500 font-medium">已接入 {modelsCount || 0} 个模型</span>
            </div>

            <div className="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-xs">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider block">累计 Token 传输</span>
              <span className="text-2xl font-black text-sky-600 font-mono mt-1 block">
                {stats.total_tokens || 0}
              </span>
              <span className="text-[11px] text-slate-500 font-medium">零内存拷贝流式转发</span>
            </div>

            <div className="p-4 bg-white rounded-2xl border border-slate-200/80 shadow-xs">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider block">平均首字延迟 (TTFT)</span>
              <span className="text-2xl font-black text-amber-600 font-mono mt-1 block">
                {(stats.avg_ttft_ms || 0).toFixed(0)} <span className="text-xs text-slate-400 font-normal">ms</span>
              </span>
              <span className="text-[11px] text-slate-500 font-medium">毫秒级极速响应</span>
            </div>
          </div>
        </div>
      </section>

      {/* Core Architectural Capabilities (Cards) */}
      <section id="features" className="py-20 px-6 max-w-7xl mx-auto w-full space-y-12">
        <div className="text-center max-w-3xl mx-auto space-y-3">
          <span className="text-xs font-bold text-indigo-600 tracking-wider uppercase px-3 py-1 rounded-full bg-indigo-50 border border-indigo-200">
            Enterprise Architecture
          </span>
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            工业级可靠性与核心技术底座
          </h2>
          <p className="text-sm text-slate-500">
            Nano-Gateway 专为企业大模型落地研发，解决上游异构协议割裂、偶发限流熔断与多模态分发难题。
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          {/* Card 1: Pre-Token Fallback */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-indigo-400 transition space-y-3 flex flex-col justify-between">
            <div className="space-y-3">
              <div className="w-12 h-12 rounded-2xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
                <Shield className="w-6 h-6" />
              </div>
              <h3 className="font-bold text-base text-slate-900">
                首字前无感容灾<br />
                <span className="text-xs text-indigo-600 font-normal font-mono">(Pre-Token Fallback)</span>
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                遭遇上游 429 并发限流、500 服务端异常或网络连接超时，首字分块发出前毫秒内切换到备份 Provider，客户端连接不中断，完全无感。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-emerald-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>成功率稳定 99.99%</span>
            </div>
          </div>

          {/* Card 2: Full-Duplex Translation */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-emerald-400 transition space-y-3 flex flex-col justify-between">
            <div className="space-y-3">
              <div className="w-12 h-12 rounded-2xl bg-emerald-50 border border-emerald-100 flex items-center justify-center text-emerald-600">
                <Cpu className="w-6 h-6" />
              </div>
              <h3 className="font-bold text-base text-slate-900">
                智能探测 & 协议转换<br />
                <span className="text-xs text-emerald-600 font-normal font-mono">(Full-Duplex Engine)</span>
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                自动识别 GPUStack、vLLM、Sub2API、Claude Messages、Gemini 与 OpenAI 原生协议，并实现全双工实时转译与思维链零拷贝透明透传。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-indigo-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>全协议全双工互通</span>
            </div>
          </div>

          {/* Card 3: Unified Multimodal Pipeline */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-purple-400 transition space-y-3 flex flex-col justify-between">
            <div className="space-y-3">
              <div className="w-12 h-12 rounded-2xl bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600">
                <Layers className="w-6 h-6" />
              </div>
              <h3 className="font-bold text-base text-slate-900">
                全模态统一调度管道<br />
                <span className="text-xs text-purple-600 font-normal font-mono">(Unified Pipeline)</span>
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                Chat 对话、AI 生图、TTS 语音合成、Whisper STT 语音识别与翻译、视频生成均采用统一分发管道，共享熔断限流与用量审计。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-purple-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>图/音/视/文本全覆盖</span>
            </div>
          </div>

          {/* Card 4: High Availability */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-sky-400 transition space-y-3 flex flex-col justify-between">
            <div className="space-y-3">
              <div className="w-12 h-12 rounded-2xl bg-sky-50 border border-sky-100 flex items-center justify-center text-sky-600">
                <Server className="w-6 h-6" />
              </div>
              <h3 className="font-bold text-base text-slate-900">
                多活热备与故障自愈<br />
                <span className="text-xs text-sky-600 font-normal font-mono">(High Availability)</span>
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                集群节点多活互备与秒级健康巡检。发生网络抖动或服务异常时毫秒级自动隔离与平滑接管，配置变更实时生效，保障业务 7×24 小时永续稳定运行。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-sky-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>无单点故障 · 业务零中断</span>
            </div>
          </div>
        </div>
      </section>

      {/* Multimodal Pipeline Section */}
      <section id="multimodal" className="py-20 px-6 max-w-6xl mx-auto w-full space-y-10">
        <div className="text-center space-y-3">
          <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-purple-50 border border-purple-200 text-purple-700 text-xs font-semibold">
            <Layers className="w-3.5 h-3.5" />
            <span>Unified Multimodal Engine</span>
          </div>
          <h2 className="text-2xl sm:text-3xl font-black text-slate-900 tracking-tight">
            全模态统一调度与多媒体处理管道
          </h2>
          <p className="text-xs sm:text-sm text-slate-500 max-w-2xl mx-auto">
            不仅是 LLM 文本网关，更为现代 AI Agent 提供跨文本、图像生成、视频生成与语音全双工统一分发入口
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
          {/* 1. Chat */}
          <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-xs space-y-3 flex flex-col justify-between">
            <div className="space-y-2.5">
              <div className="w-10 h-10 rounded-xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
                <MessageSquare className="w-5 h-5" />
              </div>
              <h4 className="font-bold text-sm text-slate-900">文本与推理对话</h4>
              <p className="text-xs text-slate-500 leading-relaxed">
                全双工流式转发、思维链无损透传、一致性哈希会话黏连与上下文 Prompt 缓存加速。
              </p>
            </div>
            <div className="pt-2.5 border-t border-slate-100 font-mono text-[11px] text-indigo-600 font-medium">
              /v1/chat/completions
            </div>
          </div>

          {/* 2. Video */}
          <div className="bg-white border border-purple-200/80 rounded-2xl p-5 shadow-xs space-y-3 flex flex-col justify-between relative overflow-hidden">
            <div className="absolute top-2.5 right-2.5 px-1.5 py-0.5 rounded text-[10px] font-bold bg-purple-100 text-purple-700">
              原生支持
            </div>
            <div className="space-y-2.5">
              <div className="w-10 h-10 rounded-xl bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600">
                <Video className="w-5 h-5" />
              </div>
              <h4 className="font-bold text-sm text-slate-900">视频生成与异步任务</h4>
              <p className="text-xs text-slate-500 leading-relaxed">
                支持 Sora、可灵 Kling、智谱 CogVideoX、阿里万象 Wan 2.1、豆包 SeaDance，提供任务提交与异步状态轮询。
              </p>
            </div>
            <div className="pt-2.5 border-t border-slate-100 font-mono text-[11px] text-purple-600 font-medium">
              /v1/videos/generations
            </div>
          </div>

          {/* 3. Image */}
          <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-xs space-y-3 flex flex-col justify-between">
            <div className="space-y-2.5">
              <div className="w-10 h-10 rounded-xl bg-amber-50 border border-amber-100 flex items-center justify-center text-amber-600">
                <ImageIcon className="w-5 h-5" />
              </div>
              <h4 className="font-bold text-sm text-slate-900">图像生成</h4>
              <p className="text-xs text-slate-500 leading-relaxed">
                支持 DALL-E 3、Flux、Stable Diffusion、豆包 SeaDream，支持分辨率、比例定制与上游多渠道平滑调度。
              </p>
            </div>
            <div className="pt-2.5 border-t border-slate-100 font-mono text-[11px] text-amber-600 font-medium">
              /v1/images/generations
            </div>
          </div>

          {/* 4. Audio */}
          <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-xs space-y-3 flex flex-col justify-between">
            <div className="space-y-2.5">
              <div className="w-10 h-10 rounded-xl bg-teal-50 border border-teal-100 flex items-center justify-center text-teal-600">
                <Volume2 className="w-5 h-5" />
              </div>
              <h4 className="font-bold text-sm text-slate-900">语音合成与转写</h4>
              <p className="text-xs text-slate-500 leading-relaxed">
                TTS 音频流极速输出，Whisper / SenseVoice 语音转写与国际化语音翻译，毫秒级流式下发。
              </p>
            </div>
            <div className="pt-2.5 border-t border-slate-100 font-mono text-[11px] text-teal-600 font-medium">
              /v1/audio/speech & stt
            </div>
          </div>
        </div>
      </section>

      {/* Multimodal Resource Optimization Section */}
      <section id="multimodal-optimization" className="py-20 px-6 max-w-6xl mx-auto w-full space-y-10 border-t border-slate-200/70">
        <div className="text-center space-y-3">
          <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-emerald-700 text-xs font-semibold">
            <Zap className="w-3.5 h-3.5" />
            <span>High Concurrency & Low Resource Footprint</span>
          </div>
          <h2 className="text-2xl sm:text-3xl font-black text-slate-900 tracking-tight">
            图像与语音场景：如何极致降低网关与集群资源消耗？
          </h2>
          <p className="text-xs sm:text-sm text-slate-500 max-w-3xl mx-auto leading-relaxed">
            面对超大分辨率图片与长音频文件，传统 API 网关极易因内存暴涨、带宽打满与上游大模型 Vision Token 消耗而导致成本失控。<br />
            Nano-Gateway 采用四大核心工业级架构技术，使网关在高频多模态负载下常驻内存恒定保持在 KB 级，传输开销直降 99%！
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Optimization Pillar 1: Zero-Buffer Stream Piping */}
          <div className="bg-white border border-slate-200/90 rounded-3xl p-6 shadow-xs space-y-4 hover:border-indigo-400 transition flex flex-col justify-between">
            <div className="space-y-3">
              <div className="flex items-center space-x-3">
                <div className="w-10 h-10 rounded-2xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600 font-bold text-sm">
                  1
                </div>
                <div>
                  <h3 className="font-bold text-base text-slate-900">内存零缓冲管道透传 (Zero-Buffer Streaming)</h3>
                  <span className="text-[11px] font-mono text-indigo-600">sync.Pool 32KB 内存池 · 杜绝全量 io.ReadAll</span>
                </div>
              </div>
              <p className="text-xs text-slate-600 leading-relaxed">
                针对音频或图像的多段表单上传，网关彻底舍弃全量 Body 内存缓冲，采用 Go 标准 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-indigo-600">sync.Pool</code> 环形内存池与 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-indigo-600">io.CopyBuffer</code> 将客户端数据实时穿透管道打向上游 Provider。无论上传 50MB 语音还是 4K 图像，单个连接网关内存常驻开销仅需 32KB，杜绝高并发 OOM 风险。
              </p>
            </div>
            <div className="p-3 bg-slate-50 rounded-xl border border-slate-100 text-xs font-mono text-slate-700 space-y-1">
              <div className="flex justify-between">
                <span className="text-slate-500">传统全量缓冲网关:</span>
                <span className="text-rose-600 font-bold">1000并发 × 10MB = 10GB 内存暴涨</span>
              </div>
              <div className="flex justify-between">
                <span className="text-emerald-600 font-bold">Nano-Gateway 管道透传:</span>
                <span className="text-emerald-600 font-bold">1000并发 × 32KB ≈ 32MB 恒定开销</span>
              </div>
            </div>
          </div>

          {/* Optimization Pillar 2: OSS / S3 Direct Upload Offloading */}
          <div className="bg-white border border-slate-200/90 rounded-3xl p-6 shadow-xs space-y-4 hover:border-emerald-400 transition flex flex-col justify-between">
            <div className="space-y-3">
              <div className="flex items-center space-x-3">
                <div className="w-10 h-10 rounded-2xl bg-emerald-50 border border-emerald-100 flex items-center justify-center text-emerald-600 font-bold text-sm">
                  2
                </div>
                <div>
                  <h3 className="font-bold text-base text-slate-900">云存储直传分流架构 (Presigned URL Offload)</h3>
                  <span className="text-[11px] font-mono text-emerald-600">网关仅承载轻量 JSON · 二进制绕行 CDN</span>
                </div>
              </div>
              <p className="text-xs text-slate-600 leading-relaxed">
                最佳实践推荐：客户端先将超大图片/录音直传至阿里云 OSS、腾讯云 COS 或 AWS S3 对象存储边缘节点，请求网关时仅在消息中携带轻量对象直链（<code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-emerald-700">image_url</code> 或 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-emerald-700">audio_url</code>）。网关只需解析极小文本 JSON，多媒体高带宽流量 100% 绕行，带宽和 CPU 消耗直降 99%。
              </p>
            </div>
            <div className="p-3 bg-emerald-50/60 rounded-xl border border-emerald-100 text-xs text-emerald-800 flex items-center justify-between">
              <span>💡 推荐传输格式:</span>
              <span className="font-mono font-bold">{"image_url: { url: 'https://cdn.../file.webp' }"}</span>
            </div>
          </div>

          {/* Optimization Pillar 3: Smart Codec & Clamping */}
          <div className="bg-white border border-slate-200/90 rounded-3xl p-6 shadow-xs space-y-4 hover:border-purple-400 transition flex flex-col justify-between">
            <div className="space-y-3">
              <div className="flex items-center space-x-3">
                <div className="w-10 h-10 rounded-2xl bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600 font-bold text-sm">
                  3
                </div>
                <div>
                  <h3 className="font-bold text-base text-slate-900">智能音频 Opus 压缩与图像 WebP 降维</h3>
                  <span className="text-[11px] font-mono text-purple-600">语音体积缩减 95% · Vision Token 节约 70%</span>
                </div>
              </div>
              <p className="text-xs text-slate-600 leading-relaxed">
                <strong>语音场景：</strong>弃用原始未压缩 PCM/WAV (1.4 Mbps) 或 MP3，推荐 16kHz mono Opus 格式 (仅 16-24 kbps)，在 Whisper/SenseVoice 准确率 99.8% 无损的同时，传输体积降低 95%！<br />
                <strong>图像场景：</strong>客户端前置转码为 WebP/AVIF 并钳制最大分辨率至 1024-1536px，既加快传输，又大幅缩减上游大模型的 Vision Tile 切片数量，推理成本立省 70%。
              </p>
            </div>
            <div className="p-3 bg-slate-50 rounded-xl border border-slate-100 text-xs font-mono text-slate-700 flex justify-between">
              <span className="text-slate-500">1分钟 PCM 录音: 10.5 MB</span>
              <span className="text-purple-600 font-bold">→ 16kHz Opus 录音: 仅 180 KB</span>
            </div>
          </div>

          {/* Optimization Pillar 4: Pre-flight Header Sniffing & Early Guards */}
          <div className="bg-white border border-slate-200/90 rounded-3xl p-6 shadow-xs space-y-4 hover:border-sky-400 transition flex flex-col justify-between">
            <div className="space-y-3">
              <div className="flex items-center space-x-3">
                <div className="w-10 h-10 rounded-2xl bg-sky-50 border border-sky-100 flex items-center justify-center text-sky-600 font-bold text-sm">
                  4
                </div>
                <div>
                  <h3 className="font-bold text-base text-slate-900">前置报头嗅探与快速早停熔断 (Early Guards)</h3>
                  <span className="text-[11px] font-mono text-sky-600">Nginx + 网关双层防护 · 杜绝恶意大包拖垮集群</span>
                </div>
              </div>
              <p className="text-xs text-slate-600 leading-relaxed">
                在客户端二进制数据到达内存之前，前置负载均衡与中间件首先嗅探 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-sky-700">Content-Length</code> 与 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-sky-700">Content-Type</code>。对于超规图像（&gt;10MB）或过长音频（&gt;25MB），在分配任何网络缓冲区前毫秒级直接响应 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-rose-600">413 Payload Too Large</code>，杜绝异常大包占用上游连接池与网关 Goroutine 协程。
              </p>
            </div>
            <div className="p-3 bg-slate-50 rounded-xl border border-slate-100 text-xs font-mono text-slate-700 flex justify-between">
              <span className="text-slate-500">超限检测拦截延迟:</span>
              <span className="text-sky-600 font-bold">&lt; 1 ms (零网络下行消耗)</span>
            </div>
          </div>
        </div>
      </section>

      {/* Developer Documentation Section */}
      <section id="docs" className="py-20 px-6 max-w-6xl mx-auto w-full space-y-10">
        <div className="text-center space-y-3">
          <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-indigo-50 border border-indigo-200 text-indigo-700 text-xs font-semibold">
            <Code className="w-3.5 h-3.5" />
            <span>Developer Documentation</span>
          </div>
          <h2 className="text-2xl sm:text-3xl font-black text-slate-900 tracking-tight">
            全协议极速接入与接口开发指南
          </h2>
          <p className="text-xs sm:text-sm text-slate-500 max-w-2xl mx-auto">
            无需重构已有业务代码，仅需调整 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-indigo-600 font-semibold">base_url</code> 与请求头，即刻享受高可用平滑分流与智能容灾
          </p>
        </div>

        {/* Tabbed Code Box */}
        <div className="bg-white border border-slate-200/90 rounded-3xl overflow-hidden shadow-sm">
          {/* Tabs bar */}
          <div className="flex flex-wrap items-center justify-between border-b border-slate-200 bg-slate-50/80 px-4 py-2 gap-2">
            <div className="flex flex-wrap items-center gap-1.5">
              {[
                { id: 'python', label: 'Python (OpenAI)' },
                { id: 'claude', label: 'Anthropic Claude' },
                { id: 'node', label: 'Node.js' },
                { id: 'curl', label: 'cURL' },
                { id: 'multimodal', label: '全模态 API' },
                { id: 'resource_opt', label: '⚡ 多模态低耗实践' },
                { id: 'session', label: '会话黏连 (90% 降本)' }
              ].map(t => (
                <button
                  key={t.id}
                  onClick={() => setActiveSnippetTab(t.id)}
                  className={`px-3 py-1.5 rounded-xl text-xs font-semibold transition ${
                    activeSnippetTab === t.id
                      ? 'bg-white text-indigo-600 shadow-xs border border-slate-200/80'
                      : 'text-slate-600 hover:text-slate-900'
                  }`}
                >
                  {t.label}
                </button>
              ))}
            </div>

            <button
              onClick={() => {
                let code = pythonSnippet;
                if (activeSnippetTab === 'curl') code = curlSnippet;
                else if (activeSnippetTab === 'claude') code = claudeSnippet;
                else if (activeSnippetTab === 'node') code = nodeSnippet;
                else if (activeSnippetTab === 'multimodal') code = multimodalSnippet;
                else if (activeSnippetTab === 'resource_opt') code = resourceOptSnippet;
                else if (activeSnippetTab === 'session') code = sessionSnippet;
                copyCode(code, activeSnippetTab);
              }}
              className="px-3.5 py-1.5 rounded-xl bg-white border border-slate-200 hover:border-indigo-400 text-xs text-slate-700 font-semibold flex items-center space-x-1.5 transition shadow-2xs"
            >
              {copiedSnippet === activeSnippetTab ? (
                <>
                  <Check className="w-3.5 h-3.5 text-emerald-600" />
                  <span className="text-emerald-600 font-semibold">已复制到剪贴板</span>
                </>
              ) : (
                <>
                  <Copy className="w-3.5 h-3.5 text-slate-500" />
                  <span>复制代码</span>
                </>
              )}
            </button>
          </div>

          {/* Code Viewer */}
          <div className="p-6 bg-slate-900 text-slate-100 font-mono text-xs overflow-x-auto leading-relaxed selection:bg-indigo-600">
            <pre>
              {activeSnippetTab === 'python' && pythonSnippet}
              {activeSnippetTab === 'claude' && claudeSnippet}
              {activeSnippetTab === 'node' && nodeSnippet}
              {activeSnippetTab === 'curl' && curlSnippet}
              {activeSnippetTab === 'multimodal' && multimodalSnippet}
              {activeSnippetTab === 'resource_opt' && resourceOptSnippet}
              {activeSnippetTab === 'session' && sessionSnippet}
            </pre>
          </div>
        </div>

        {/* Standardized 3-Tier URL Architecture */}
        <div className="space-y-4">
          <div className="flex items-center space-x-2 text-slate-900 font-bold text-xs uppercase tracking-wider">
            <Terminal className="w-4 h-4 text-indigo-600" />
            <span>三层规范化 URL 路由体系 (Standardized 3-Tier URL Architecture)</span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
            {/* 1. Data Plane */}
            <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-2xs space-y-3 flex flex-col justify-between">
              <div className="space-y-2.5">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-xs text-indigo-700 bg-indigo-50 px-2.5 py-1 rounded-lg border border-indigo-200">
                    标准推理数据面 (/v1)
                  </span>
                  <span className="text-[10px] text-slate-400 font-mono">SDK 原生直连</span>
                </div>
                <p className="text-[11px] text-slate-500 leading-relaxed">
                  标准模型推理接口，完全兼容 OpenAI、Claude、Gemini 原生 SDK，支持流式 SSE 与首字容灾。
                </p>
                <div className="space-y-1.5 text-xs font-mono">
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-indigo-600 font-semibold text-[11px]">
                    POST /v1/chat/completions
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-purple-600 font-semibold text-[11px]">
                    POST /v1/messages
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-amber-600 font-semibold text-[11px]">
                    POST /v1/images/generations
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-emerald-600 font-semibold text-[11px]">
                    POST /v1/audio/transcriptions
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-rose-600 font-semibold text-[11px]">
                    POST /v1/videos/generations
                  </div>
                </div>
              </div>
              <div className="pt-2 border-t border-slate-100 text-[11px] text-slate-400">
                支持 Bearer Token 与 X-Session-ID
              </div>
            </div>

            {/* 2. Control Plane */}
            <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-2xs space-y-3 flex flex-col justify-between">
              <div className="space-y-2.5">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-xs text-purple-700 bg-purple-50 px-2.5 py-1 rounded-lg border border-purple-200">
                    统一后端管控面 (/api)
                  </span>
                  <span className="text-[10px] text-slate-400 font-mono">Control Plane</span>
                </div>
                <p className="text-[11px] text-slate-500 leading-relaxed">
                  网关内部控制面与运营管理 API，严格统一收口在 /api/v1 路径下，支持权限隔离与网关多副本同步。
                </p>
                <div className="space-y-1.5 text-xs font-mono">
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-purple-700 font-semibold text-[11px]">
                    /api/v1/auth/* (登录/注册/OAuth)
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-purple-700 font-semibold text-[11px]">
                    /api/v1/user/* (钱包/充值/密钥)
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-purple-700 font-semibold text-[11px]">
                    /api/v1/admin/* (路由/渠道/计费)
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-emerald-700 font-semibold text-[11px]">
                    /api/v1/public/status (公网 SLA)
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-slate-600 font-semibold text-[11px]">
                    /health (K8s 探针存活检测)
                  </div>
                </div>
              </div>
              <div className="pt-2 border-t border-slate-100 text-[11px] text-slate-400">
                全栈 RESTful API + JWT 鉴权
              </div>
            </div>

            {/* 3. Web Entry */}
            <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-2xs space-y-3 flex flex-col justify-between">
              <div className="space-y-2.5">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-xs text-emerald-700 bg-emerald-50 px-2.5 py-1 rounded-lg border border-emerald-200">
                    极简应用访问入口 (Web)
                  </span>
                  <span className="text-[10px] text-slate-400 font-mono">SaaS Entry</span>
                </div>
                <p className="text-[11px] text-slate-500 leading-relaxed">
                  提供无缝直达的用户工作台，根路径直接载入 SPA 应用，告别旧式 ui 字眼，兼顾极简体验与历史兼容。
                </p>
                <div className="space-y-1.5 text-xs font-mono">
                  <div className="p-1.5 rounded-lg bg-emerald-50/70 border border-emerald-200 text-emerald-800 font-semibold text-[11px] flex justify-between">
                    <span>GET /</span>
                    <span className="font-sans font-bold text-[10px] text-emerald-600">根路径零跳转直达</span>
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-indigo-600 font-semibold text-[11px] flex justify-between">
                    <span>GET /app/</span>
                    <span className="font-sans text-[10px] text-slate-500">现代 SaaS 规范应用路径</span>
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-slate-700 font-semibold text-[11px] flex justify-between">
                    <span>GET /workspace/</span>
                    <span className="font-sans text-[10px] text-slate-400">工作台路径兼容</span>
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-slate-700 font-semibold text-[11px] flex justify-between">
                    <span>GET /console/</span>
                    <span className="font-sans text-[10px] text-slate-400">控制台路径兼容</span>
                  </div>
                  <div className="p-1.5 rounded-lg bg-slate-50 border border-slate-100 text-amber-700 font-semibold text-[11px] flex justify-between">
                    <span>GET /ui</span>
                    <span className="font-sans text-[10px] text-amber-600">301 自动跳转至 /app/</span>
                  </div>
                </div>
              </div>
              <div className="pt-2 border-t border-slate-100 text-[11px] text-emerald-600 font-medium">
                彻底废弃 "ui" 字眼，体验更加丝滑
              </div>
            </div>
          </div>
        </div>

        {/* Request Headers & Security Auth Strip */}
        <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-2xs space-y-3">
          <div className="flex items-center space-x-2 text-slate-900 font-bold text-xs uppercase tracking-wider">
            <Shield className="w-4 h-4 text-indigo-600" />
            <span>核心调度与认证请求头速查 (Header Matrix)</span>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-xs">
            <div className="p-3 rounded-xl bg-slate-50 border border-slate-100 space-y-1">
              <div className="flex items-center justify-between">
                <span className="font-mono font-bold text-slate-800">Authorization: Bearer sk-nano-...</span>
                <span className="text-indigo-600 font-semibold text-[10px] bg-indigo-50 px-1.5 py-0.5 rounded border border-indigo-200">必填</span>
              </div>
              <p className="text-[11px] text-slate-500">工作台签发的 API 密钥，毫秒级内存校验、租户限流与余额计费。</p>
            </div>

            <div className="p-3 rounded-xl bg-slate-50 border border-slate-100 space-y-1">
              <div className="flex items-center justify-between">
                <span className="font-mono font-bold text-indigo-700">X-Session-ID: &lt;session_id&gt;</span>
                <span className="text-emerald-600 font-semibold bg-emerald-50 px-1.5 py-0.5 rounded border border-emerald-200 text-[10px]">90% 降本推荐</span>
              </div>
              <p className="text-[11px] text-slate-500">启用一致性哈希会话黏连，保持同一 Provider 锁定，复用 Prefix KV 缓存。</p>
            </div>

            <div className="p-3 rounded-xl bg-slate-50 border border-slate-100 space-y-1">
              <div className="flex items-center justify-between">
                <span className="font-mono font-bold text-slate-800">x-api-key: sk-nano-...</span>
                <span className="text-slate-400 text-[10px]">SDK 兼容</span>
              </div>
              <p className="text-[11px] text-slate-500">Anthropic Claude 原生 SDK 请求头，网关双向透明自动识别转译。</p>
            </div>
          </div>
        </div>
      </section>

      {/* Model Pricing & Prompt Caching Section */}
      <section id="pricing" className="py-20 px-6 max-w-6xl mx-auto w-full space-y-10 border-t border-slate-200/80">
        <div className="text-center space-y-3">
          <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-emerald-700 text-xs font-semibold">
            <Sparkles className="w-3.5 h-3.5" />
            <span>企业级分时计费与成本节省引擎</span>
          </div>
          <h2 className="text-2xl sm:text-3xl font-black text-slate-900 tracking-tight">
            2026 旗舰模型矩阵 · 基准费率 · Prompt 缓存 · 夜间闲时 5 折
          </h2>
          <p className="text-xs sm:text-sm text-slate-500 max-w-2xl mx-auto">
            支持针对不同模型差异化定价。原生支持 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-emerald-600 font-semibold">cached_tokens</code> 缓存读取优惠（立减 90%）与夜间闲时分时优惠（00:00 - 08:30 半价），折上折最高节约 95% 成本！
          </p>
        </div>

        {/* Pricing Matrix Table */}
        <div className="bg-white border border-slate-200/90 rounded-3xl overflow-hidden shadow-sm">
          <div className="p-5 border-b border-slate-100 bg-slate-50/50 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h3 className="font-bold text-sm text-slate-900">2026 主流模型费率对照表 (Rates per 1M Tokens)</h3>
              <p className="text-xs text-slate-500 mt-0.5">网关实时计算 Token 与时间段，并在响应头实时返回 X-Nano-Cost 与 X-Nano-Off-Peak 标识</p>
            </div>
            <div className="flex items-center space-x-2">
              <span className="text-[11px] font-semibold px-2.5 py-1 rounded-lg bg-indigo-50 text-indigo-700 border border-indigo-200 flex items-center space-x-1">
                <Moon className="w-3 h-3" />
                <span>🌙 闲时 00:00-08:30 享 5 折</span>
              </span>
              <span className="text-[11px] font-semibold px-2.5 py-1 rounded-lg bg-emerald-50 text-emerald-700 border border-emerald-200">
                ⚡ 缓存省 90%
              </span>
            </div>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/80 text-slate-400 uppercase tracking-wider text-[11px]">
                  <th className="py-3 px-5 font-semibold">模型标识 (Model)</th>
                  <th className="py-3 px-4 font-semibold">基准输入 (/1M)</th>
                  <th className="py-3 px-4 font-semibold">基准输出 (/1M)</th>
                  <th className="py-3 px-4 font-semibold">🌙 分时优惠时段</th>
                  <th className="py-3 px-4 font-semibold">Prompt 缓存命中 (/1M)</th>
                  <th className="py-3 px-4 font-semibold">固定单次费用</th>
                  <th className="py-3 px-5 font-semibold">最高降本</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 text-slate-700 font-mono">
                {/* 1. claude-opus-5.5 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-purple-600"></span>
                    <span>claude-opus-5.5</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-purple-50 text-purple-700 border border-purple-200">2026 旗舰</span>
                  </td>
                  <td className="py-3.5 px-4">¥35.00</td>
                  <td className="py-3.5 px-4">¥175.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥17.50 / ¥87.50</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥3.50</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                {/* 2. gpt-6-astra */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-sky-600"></span>
                    <span>gpt-6-astra</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-sky-50 text-sky-700 border border-sky-200">OpenAI 2026</span>
                  </td>
                  <td className="py-3.5 px-4">¥30.00</td>
                  <td className="py-3.5 px-4">¥120.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥15.00 / ¥60.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥3.00</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                {/* 3. claude-fable-5.1 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-purple-500"></span>
                    <span>claude-fable-5.1</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-slate-100 text-slate-600">Agentic 推理</span>
                  </td>
                  <td className="py-3.5 px-4">¥18.00</td>
                  <td className="py-3.5 px-4">¥90.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥9.00 / ¥45.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥1.80</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                {/* 4. gemini-3.8-flash */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-blue-500"></span>
                    <span>gemini-3.8-flash</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-blue-50 text-blue-700 border border-blue-200">1M 上下文</span>
                  </td>
                  <td className="py-3.5 px-4">¥1.50</td>
                  <td className="py-3.5 px-4">¥6.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.75 / ¥3.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.15</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                {/* 5. gemini-3.8-live */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-blue-600"></span>
                    <span>gemini-3.8-live</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-amber-50 text-amber-700 border border-amber-200">双向实时语音</span>
                  </td>
                  <td className="py-3.5 px-4">¥5.00</td>
                  <td className="py-3.5 px-4">¥20.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥2.50 / ¥10.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.50</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                {/* 6. deepseek-v4.1-flash */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-indigo-500"></span>
                    <span>deepseek-v4.1-flash</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-indigo-50 text-indigo-700 border border-indigo-200">高并发 MoE</span>
                  </td>
                  <td className="py-3.5 px-4">¥1.00</td>
                  <td className="py-3.5 px-4">¥4.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.50 / ¥2.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.10</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                {/* 7. deepseek-r1 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-indigo-600"></span>
                    <span>deepseek-r1</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-slate-100 text-slate-600">深度长链思考</span>
                  </td>
                  <td className="py-3.5 px-4">¥4.00</td>
                  <td className="py-3.5 px-4">¥16.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥2.00 / ¥8.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.40</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                {/* 8. qwen-3.8-max */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-teal-500"></span>
                    <span>qwen-3.8-max</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-teal-50 text-teal-700 border border-teal-200">阿里通义旗舰</span>
                  </td>
                  <td className="py-3.5 px-4">¥6.00</td>
                  <td className="py-3.5 px-4">¥24.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥3.00 / ¥12.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥1.20</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 90%
                    </span>
                  </td>
                </tr>

                {/* 9. gpt-6-sol */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-sky-500"></span>
                    <span>gpt-6-sol</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-slate-100 text-slate-600">企业级高吞吐</span>
                  </td>
                  <td className="py-3.5 px-4">¥12.00</td>
                  <td className="py-3.5 px-4">¥48.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥6.00 / ¥24.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥1.20</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 90%
                    </span>
                  </td>
                </tr>

                {/* 10. flux-1.1-pro */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-amber-500"></span>
                    <span>flux-1.1-pro</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-amber-50 text-amber-700 border border-amber-200">旗舰超清生图</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.10 / 张</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">¥0.20 / 张</td>
                  <td className="py-3.5 px-5 font-sans text-slate-500">5 折优惠</td>
                </tr>

                {/* 11. sora-2 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-rose-500"></span>
                    <span>sora-2</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-rose-50 text-rose-700 border border-rose-200">影视级视频生成</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.75 / 次</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">¥1.50 / 次</td>
                  <td className="py-3.5 px-5 font-sans text-slate-500">5 折优惠</td>
                </tr>

                {/* 12. whisper-large-v3-turbo */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-teal-600"></span>
                    <span>whisper-large-v3-turbo</span>
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-teal-50 text-teal-700 border border-teal-200">极速高精 STT</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.02 / 分钟</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">¥0.04 / 分钟</td>
                  <td className="py-3.5 px-5 font-sans text-slate-500">5 折优惠</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="mt-auto border-t border-slate-200 bg-white py-10 px-6 text-xs text-slate-500">
        <div className="max-w-7xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center space-x-2">
            <span className="font-extrabold text-slate-800 text-sm">Nano</span>
            <span>· © {new Date().getFullYear()} 极简高性能 AI 路由器 & MCP 服务</span>
          </div>

          <div className="flex items-center space-x-4">
            {isLoggedIn ? (
              <button onClick={onEnterConsole} className="text-indigo-600 hover:underline font-semibold">
                进入工作台
              </button>
            ) : (
              <button onClick={onOpenLogin} className="text-indigo-600 hover:underline font-semibold cursor-pointer">
                登录 / 注册
              </button>
            )}
            <button onClick={onViewStatus} className="hover:text-slate-800 cursor-pointer">
              服务运行状态 (SLA)
            </button>
            <a href="https://github.com/ifnodoraemon/nano-gateway" target="_blank" rel="noreferrer" className="hover:text-slate-800">
              GitHub 源码
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}
