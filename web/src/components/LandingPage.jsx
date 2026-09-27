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

# 1. base_url 指向 Nano-Gateway 集群入口，使用管理员在控制台签发的客户端 Key
client = OpenAI(
    base_url="${origin}/v1",
    api_key="sk-nano-your-client-key",
    default_headers={
        # 传入统一会话 ID，网关自动启用一致性哈希 (FNV-1a) 锁定相同 Provider，复用前缀 KV Cache
        "X-Session-ID": "session_user_001"
    }
)

# 2. 发起对话推理请求 (支持毫秒级打字机流式输出与首字前无感容灾)
response = client.chat.completions.create(
    model="deepseek-chat",
    messages=[
        {"role": "system", "content": "你是一位资深架构师"},
        {"role": "user", "content": "请用 Go 实现高性能平滑加权轮询 (SWRR) 算法"}
    ],
    stream=True,
)

for chunk in response:
    print(chunk.choices[0].delta.content or "", end="", flush=True)`;

  const curlSnippet = `# 1. 标准对话补全 (Chat Completions)
curl -X POST "${origin}/v1/chat/completions" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -H "X-Session-ID: session_user_001" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "deepseek-chat",
    "messages": [
      {"role": "user", "content": "你好，请介绍 Nano-Gateway 的核心优势"}
    ],
    "stream": true
  }'`;

  const claudeSnippet = `import anthropic

# 原生 Claude 协议直连 Nano-Gateway
# 网关内部全双工实时转译，自动兼容 OpenAI 格式或 DeepSeek 格式的下游服务商！
client = anthropic.Anthropic(
    base_url="${origin}",
    api_key="sk-nano-your-client-key",
)

message = client.messages.create(
    model="claude-3-5-sonnet",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "解释分布式网关的数据面与控制面分离架构"}
    ]
)
print(message.content[0].text)`;

  const nodeSnippet = `import OpenAI from 'openai';

// Node.js / TypeScript 极速接入
const openai = new OpenAI({
  baseURL: '${origin}/v1',
  apiKey: 'sk-nano-your-client-key',
  defaultHeaders: {
    'X-Session-ID': 'conv_node_42' // 启用会话黏连，命中上游缓存
  }
});

const stream = await openai.chat.completions.create({
  model: 'deepseek-chat',
  messages: [{ role: 'user', content: '介绍大模型网关的 Pre-Token Fallback 机制' }],
  stream: true,
});

for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content || '');
}`;

  const multimodalSnippet = `# 1. 图像生成 (DALL-E 3 / Flux / SeaDream)
curl -X POST "${origin}/v1/images/generations" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "dall-e-3", "prompt": "极简科技风云原生分布式网关架构图", "n": 1, "size": "1024x1024"}'

# 2. 视频生成与异步任务轮询 (Sora / Kling / CogVideoX / 豆包 SeaDance)
curl -X POST "${origin}/v1/videos/generations" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "cogvideox", "prompt": "未来赛博朋克城市雨夜飞车", "aspect_ratio": "16:9"}'

# 轮询视频生成状态直到 SUCCESS 并获取播放下载直链：
curl -X GET "${origin}/v1/videos/tasks/task_xxx" \\
  -H "Authorization: Bearer sk-nano-your-client-key"

# 3. 语音合成 TTS (直接输出 audio/mpeg 二进制音频流)
curl -X POST "${origin}/v1/audio/speech" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "tts-1", "input": "欢迎使用 Nano-Gateway 企业级高可用网关系统", "voice": "alloy"}' \\
  --output audio_output.mp3

# 4. 语音识别 Whisper STT (语音文件转文本)
curl -X POST "${origin}/v1/audio/transcriptions" \\
  -H "Authorization: Bearer sk-nano-your-client-key" \\
  -F file="@audio_output.mp3" \\
  -F model="whisper-1"`;

  const sessionSnippet = `# 会话黏连 (Session Affinity) 与 Prompt Caching 降本指南
#
# 核心原理：
# 在客户端请求中携带请求头 X-Session-ID: <唯一会话ID> (或 user 字段)。
# Nano-Gateway 基于 FNV-1a 一致性哈希与 Redis 分布式会话钉选 (30分钟 TTL)，
# 确保多轮会话始终命中同一 Provider 节点，复用远端 Prefix KV Cache。
#
# 收益：
# 1. 首字时延 (TTFT) 降低 80%
# 2. DeepSeek / Claude Prompt Caching 享受高达 90% 计费折扣 (¥0.20/1M vs ¥2.00/1M)`;

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
            <a href="#architecture" className="hover:text-indigo-600 transition">集群架构</a>
            <a href="#multimodal" className="hover:text-indigo-600 transition">全模态管道</a>
            <a href="#pricing" className="hover:text-indigo-600 transition">模型定价</a>
            <a href="#docs" className="hover:text-indigo-600 transition font-bold text-indigo-600">开发者文档</a>
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
                <span>进入管理控制台 →</span>
              </button>
            ) : (
              <button
                onClick={onOpenLogin}
                className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition"
              >
                <Key className="w-3.5 h-3.5" />
                <span>管理员登录</span>
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
            企业级大模型与多模态<br />
            <span className="bg-gradient-to-r from-indigo-600 via-purple-600 to-sky-600 bg-clip-text text-transparent">
              统一调度与接入网关
            </span>
          </h1>

          <p className="text-base sm:text-lg text-slate-600 max-w-3xl mx-auto leading-relaxed">
            单核数万 QPS 极速分发 · 全双工流式协议转译 · 零停机原子热重载 · 首字分块前无感容灾兜底。<br className="hidden sm:inline" />
            统一收口 OpenAI、Claude、Gemini、GPUStack、vLLM 与多模态生成管线。
          </p>

          <div className="flex flex-col sm:flex-row items-center justify-center gap-3.5 pt-4">
            {isLoggedIn ? (
              <button
                onClick={onEnterConsole}
                className="w-full sm:w-auto px-6 py-3 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-sm font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2"
              >
                <Server className="w-4 h-4" />
                <span>进入管理控制台</span>
                <ArrowRight className="w-4 h-4" />
              </button>
            ) : (
              <button
                onClick={onOpenLogin}
                className="w-full sm:w-auto px-6 py-3 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-sm font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2"
              >
                <Key className="w-4 h-4" />
                <span>立即登录控制台</span>
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
                双机热备与 Zero-DB<br />
                <span className="text-xs text-sky-600 font-normal font-mono">(HA & Hot Reload)</span>
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                Nginx 负载均衡前置 + 2 节点计算副本轮询调度。数据面请求 100% 内存无锁运行，配置修改通过 WAL 模式毫秒级原子重载生效。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-sky-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>无单点故障 · 零 IO 延迟</span>
            </div>
          </div>
        </div>
      </section>

      {/* Cluster Topology Showcase */}
      <section id="architecture" className="py-16 px-6 bg-slate-100/60 border-y border-slate-200/70">
        <div className="max-w-6xl mx-auto space-y-8">
          <div className="text-center space-y-2">
            <span className="text-xs font-bold text-emerald-600 uppercase tracking-wider">Cluster Topology</span>
            <h2 className="text-2xl sm:text-3xl font-black text-slate-900">生产环境双机高可用拓扑</h2>
            <p className="text-xs sm:text-sm text-slate-500">统一前置负载均衡入口，计算节点水平扩展，提供零停机运维体验</p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
            <div className="bg-white p-5 rounded-2xl border border-slate-200 shadow-xs space-y-3">
              <div className="flex items-center justify-between">
                <span className="p-2 rounded-xl bg-sky-50 text-sky-600"><Globe className="w-5 h-5" /></span>
                <span className="text-xs font-mono font-bold px-2 py-0.5 rounded-lg bg-emerald-50 text-emerald-700">端口 :8080</span>
              </div>
              <h4 className="font-bold text-sm text-slate-900">Nginx 负载均衡层</h4>
              <p className="text-xs text-slate-500 leading-relaxed">
                对外统一暴露标准 8080 端口，采用加权轮询算法均衡流量至各计算节点，Keepalive 连接池长连接加速。
              </p>
            </div>

            <div className="bg-white p-5 rounded-2xl border border-slate-200 shadow-xs space-y-3">
              <div className="flex items-center justify-between">
                <span className="p-2 rounded-xl bg-indigo-50 text-indigo-600"><Server className="w-5 h-5" /></span>
                <span className="text-xs font-mono font-bold px-2 py-0.5 rounded-lg bg-indigo-50 text-indigo-700">节点 #1 (:8081)</span>
              </div>
              <h4 className="font-bold text-sm text-slate-900">Gateway 实例 1</h4>
              <p className="text-xs text-slate-500 leading-relaxed">
                全功能网关节点，内存维护全局路由字典与熔断计数器，处理流式分发与协议转换。
              </p>
            </div>

            <div className="bg-white p-5 rounded-2xl border border-slate-200 shadow-xs space-y-3">
              <div className="flex items-center justify-between">
                <span className="p-2 rounded-xl bg-purple-50 text-purple-600"><Server className="w-5 h-5" /></span>
                <span className="text-xs font-mono font-bold px-2 py-0.5 rounded-lg bg-purple-50 text-purple-700">节点 #2 (:8082)</span>
              </div>
              <h4 className="font-bold text-sm text-slate-900">Gateway 实例 2</h4>
              <p className="text-xs text-slate-500 leading-relaxed">
                双活热备副本，共享数据持久卷。任何单节点出现故障时由 Nginx 毫秒级剔除，服务零中断。
              </p>
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
                { id: 'session', label: '会话黏连 (Session Affinity)' }
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
              {activeSnippetTab === 'session' && sessionSnippet}
            </pre>
          </div>
        </div>

        {/* API Endpoints & Request Headers Matrix */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
          <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-2xs space-y-3">
            <div className="flex items-center space-x-2 text-slate-900 font-bold text-xs uppercase tracking-wider">
              <Terminal className="w-4 h-4 text-indigo-600" />
              <span>数据面统一端点速查 (Data Plane Endpoints)</span>
            </div>
            <div className="space-y-2 text-xs">
              <div className="flex items-center justify-between p-2 rounded-xl bg-slate-50 border border-slate-100">
                <span className="font-mono text-indigo-600 font-bold">POST /v1/chat/completions</span>
                <span className="text-slate-500">OpenAI 格式对话补全与流式打字机</span>
              </div>
              <div className="flex items-center justify-between p-2 rounded-xl bg-slate-50 border border-slate-100">
                <span className="font-mono text-purple-600 font-bold">POST /v1/messages</span>
                <span className="text-slate-500">Claude 原生全双工转译对话</span>
              </div>
              <div className="flex items-center justify-between p-2 rounded-xl bg-slate-50 border border-slate-100">
                <span className="font-mono text-amber-600 font-bold">POST /v1/images/generations</span>
                <span className="text-slate-500">DALL-E 3 / Flux 图像生成</span>
              </div>
              <div className="flex items-center justify-between p-2 rounded-xl bg-slate-50 border border-slate-100">
                <span className="font-mono text-rose-600 font-bold">POST /v1/audio/speech</span>
                <span className="text-slate-500">TTS 语音合成 (返回二进制音频流)</span>
              </div>
              <div className="flex items-center justify-between p-2 rounded-xl bg-slate-50 border border-slate-100">
                <span className="font-mono text-emerald-600 font-bold">POST /v1/audio/transcriptions</span>
                <span className="text-slate-500">Whisper STT 语音转写</span>
              </div>
            </div>
          </div>

          <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-2xs space-y-3">
            <div className="flex items-center space-x-2 text-slate-900 font-bold text-xs uppercase tracking-wider">
              <Shield className="w-4 h-4 text-indigo-600" />
              <span>关键请求头与高级调度参数</span>
            </div>
            <div className="space-y-2 text-xs">
              <div className="p-2 rounded-xl bg-slate-50 border border-slate-100">
                <div className="flex items-center justify-between">
                  <span className="font-mono font-bold text-slate-800">Authorization: Bearer sk-nano-...</span>
                  <span className="text-indigo-600 font-medium">必填</span>
                </div>
                <p className="text-[11px] text-slate-500 mt-0.5">控制台签发的虚拟 Key，毫秒级内存校验、租户限流与余额计费。</p>
              </div>

              <div className="p-2 rounded-xl bg-slate-50 border border-slate-100">
                <div className="flex items-center justify-between">
                  <span className="font-mono font-bold text-indigo-700">X-Session-ID: &lt;session_id&gt;</span>
                  <span className="text-emerald-600 font-semibold bg-emerald-50 px-1.5 py-0.5 rounded border border-emerald-200 text-[10px]">90% 降本推荐</span>
                </div>
                <p className="text-[11px] text-slate-500 mt-0.5">启用一致性哈希会话黏连，保持同一 Provider 锁定，享受高比例 Prefix KV 缓存命中。</p>
              </div>

              <div className="p-2 rounded-xl bg-slate-50 border border-slate-100">
                <div className="flex items-center justify-between">
                  <span className="font-mono font-bold text-slate-800">x-api-key: sk-nano-...</span>
                  <span className="text-slate-400 text-[10px]">兼容</span>
                </div>
                <p className="text-[11px] text-slate-500 mt-0.5">Anthropic SDK 原生认证头，网关全自动识别转译。</p>
              </div>
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
            基准模型费率 · Prompt 缓存 · 夜间闲时 5 折
          </h2>
          <p className="text-xs sm:text-sm text-slate-500 max-w-2xl mx-auto">
            支持针对不同模型差异化定价。原生支持 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-emerald-600 font-semibold">cached_tokens</code> 缓存读取优惠（立减 90%）与夜间闲时分时优惠（00:00 - 08:30 半价），折上折最高节约 95% 成本！
          </p>
        </div>

        {/* Pricing Matrix Table */}
        <div className="bg-white border border-slate-200/90 rounded-3xl overflow-hidden shadow-sm">
          <div className="p-5 border-b border-slate-100 bg-slate-50/50 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h3 className="font-bold text-sm text-slate-900">模型费率对照表 (Rates per 1M Tokens)</h3>
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
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-indigo-500"></span>
                    <span>deepseek-chat (V3)</span>
                  </td>
                  <td className="py-3.5 px-4">¥2.00</td>
                  <td className="py-3.5 px-4">¥8.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥1.00 / ¥4.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.20</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-indigo-500"></span>
                    <span>deepseek-reasoner (R1)</span>
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

                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-sky-500"></span>
                    <span>gpt-4o</span>
                  </td>
                  <td className="py-3.5 px-4">¥18.00</td>
                  <td className="py-3.5 px-4">¥72.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥9.00 / ¥36.00</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥9.00</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 50%
                    </span>
                  </td>
                </tr>

                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-purple-500"></span>
                    <span>claude-3-5-sonnet</span>
                  </td>
                  <td className="py-3.5 px-4">¥21.00</td>
                  <td className="py-3.5 px-4">¥105.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥10.50 / ¥52.50</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥2.10</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-5 font-sans">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 text-[11px] font-bold">
                      立省 95%
                    </span>
                  </td>
                </tr>

                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-amber-500"></span>
                    <span>dall-e-3</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.14 / 张</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">¥0.28 / 张</td>
                  <td className="py-3.5 px-5 font-sans text-slate-500">5 折优惠</td>
                </tr>

                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-rose-500"></span>
                    <span>tts-1 / whisper-1</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.05 / 次</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">¥0.10 / 次</td>
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
            <span>· 极简高性能 AI 路由器 & MCP 服务</span>
          </div>

          <div className="flex items-center space-x-4">
            {isLoggedIn ? (
              <button onClick={onEnterConsole} className="text-indigo-600 hover:underline font-semibold">
                进入管理控制台
              </button>
            ) : (
              <button onClick={onOpenLogin} className="text-indigo-600 hover:underline font-semibold">
                管理员登录
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
