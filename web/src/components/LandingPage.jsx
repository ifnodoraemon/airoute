import React, { useState } from 'react';
import {
  Shield,
  Cpu,
  Layers,
  Server,
  Zap,
  Terminal,
  Code,
  Key,
  Check,
  Copy,
  Sparkles,
  ArrowRight,
  MessageSquare,
  Image as ImageIcon,
  Volume2,
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

client = OpenAI(
    base_url="${origin}/v1",
    api_key="sk-airoute-your-api-key",
    default_headers={"X-Session-ID": "session_user_001"}
)

response = client.chat.completions.create(
    model="gpt-6-astra",  # 支持 claude-opus-5.5 / deepseek-v4.1-flash / gemini-3.8-flash 等
    messages=[{"role": "user", "content": "请介绍系统的核心接入优势"}],
    stream=True,
)

for chunk in response:
    print(chunk.choices[0].delta.content or "", end="", flush=True)`;

  const claudeSnippet = `import anthropic

client = anthropic.Anthropic(
    base_url="${origin}",
    api_key="sk-airoute-your-api-key",
)

message = client.messages.create(
    model="claude-opus-5.5",
    max_tokens=2048,
    messages=[{"role": "user", "content": "介绍极简统一网关的高可用与流式转译"}]
)
print(message.content[0].text)`;

  const curlSnippet = `# 标准对话推理 (Chat Completions)
curl -X POST "${origin}/v1/chat/completions" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -H "X-Session-ID: session_user_001" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-6-astra",
    "messages": [{"role": "user", "content": "你好，请介绍系统优势"}],
    "stream": true
  }'`;

  const multimodalSnippet = `# 1. 图像生成 (FLUX.1-Pro)
curl -X POST "${origin}/v1/images/generations" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "flux-1.1-pro", "prompt": "极简科技风云原生 AI 路由器", "size": "1024x1024"}'

# 2. 语音转写 (Whisper-Large-V3-Turbo，支持 16kHz mono Opus 压缩格式)
curl -X POST "${origin}/v1/audio/transcriptions" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -F file="@speech.opus" \\
  -F model="whisper-large-v3-turbo"

# 3. 影视级视频生成与状态轮询 (Sora 2)
curl -X POST "${origin}/v1/videos/generations" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "sora-2", "prompt": "未来赛博朋克城市雨夜飞车", "aspect_ratio": "16:9"}'

curl -X GET "${origin}/v1/videos/tasks/task_xxx" \\
  -H "Authorization: Bearer sk-airoute-your-api-key"`;

  const sessionSnippet = `# 会话黏连与 Prompt 缓存降本
#
# 在请求头中携带 X-Session-ID: <session_id>
# 网关自动基于哈希钉选同一 Provider，复用 Prefix KV Cache。
#
# 收益：首字时延降低 80%，Prompt 缓存最高享 90% 计费折扣！`;

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900 flex flex-col font-sans selection:bg-indigo-500 selection:text-white">
      {/* Top Navigation Bar */}
      <header className="sticky top-0 z-40 bg-white/90 backdrop-blur-md border-b border-slate-200/80">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-gradient-to-tr from-indigo-600 to-sky-500 flex items-center justify-center text-white shadow-sm font-black text-lg">
              ⚡
            </div>
            <span className="font-extrabold text-lg tracking-tight bg-gradient-to-r from-indigo-700 via-purple-700 to-sky-600 bg-clip-text text-transparent">
              AI 路由器
            </span>
          </div>

          <nav className="hidden md:flex items-center space-x-8 text-xs font-semibold text-slate-600">
            <a href="#features" className="hover:text-indigo-600 transition">核心特性</a>
            <a href="#multimodal" className="hover:text-indigo-600 transition">全模态矩阵</a>
            <a href="#docs" className="hover:text-indigo-600 transition">接入指南</a>
            <a href="#pricing" className="hover:text-indigo-600 transition">模型定价</a>
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
                className="px-4 py-2 bg-gradient-to-r from-indigo-600 to-sky-600 hover:from-indigo-700 hover:to-sky-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
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
      <section className="relative pt-20 pb-24 px-6 overflow-hidden bg-gradient-to-b from-indigo-50/50 via-white to-slate-50 border-b border-slate-200/60">
        <div className="max-w-4xl mx-auto text-center space-y-8">
          <div
            onClick={onViewStatus}
            className="inline-flex items-center space-x-2 px-4 py-1.5 rounded-full bg-white border border-slate-200 shadow-xs text-xs font-semibold text-slate-700 hover:border-indigo-400 hover:shadow-sm cursor-pointer transition"
          >
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            <span>高可用集群运行中</span>
            <span className="text-slate-300">|</span>
            <span className="text-emerald-600 font-mono">SLA 99.99%</span>
            <span className="text-slate-300">|</span>
            <span className="text-indigo-600 font-medium">状态详情 →</span>
          </div>

          <h1 className="text-4xl sm:text-5xl lg:text-6xl font-black text-slate-900 tracking-tight leading-tight">
            2026 旗舰大模型与多模态<br />
            <span className="bg-gradient-to-r from-indigo-600 via-purple-600 to-sky-600 bg-clip-text text-transparent">
              统一调度与高性能接入网关
            </span>
          </h1>

          <p className="text-base sm:text-lg text-slate-600 max-w-2xl mx-auto leading-relaxed">
            单核万级吞吐 · 全协议流式转发 · 故障无感容灾 · 闲时与缓存降本 95%<br />
            统一收口 OpenAI、Claude、Gemini、DeepSeek 与全模态生成管线。
          </p>

          <div className="flex flex-col sm:flex-row items-center justify-center gap-4 pt-2">
            {isLoggedIn ? (
              <button
                onClick={onEnterConsole}
                className="w-full sm:w-auto px-7 py-3.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-sm font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2 cursor-pointer"
              >
                <Server className="w-4 h-4" />
                <span>进入工作台</span>
                <ArrowRight className="w-4 h-4" />
              </button>
            ) : (
              <button
                onClick={onOpenLogin}
                className="w-full sm:w-auto px-7 py-3.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-sm font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2 cursor-pointer"
              >
                <Key className="w-4 h-4" />
                <span>立即登录 / 免费注册</span>
                <ArrowRight className="w-4 h-4" />
              </button>
            )}

            <button
              onClick={onViewDocs}
              className="w-full sm:w-auto px-7 py-3.5 bg-white hover:bg-slate-50 text-slate-700 border border-slate-200 rounded-2xl text-sm font-semibold shadow-xs transition flex items-center justify-center space-x-2 cursor-pointer"
            >
              <Code className="w-4 h-4 text-indigo-600" />
              <span>查阅接入指南</span>
            </button>
          </div>

          {/* Quick Metrics Strip */}
          <div className="pt-10 grid grid-cols-2 sm:grid-cols-4 gap-4 max-w-3xl mx-auto text-left">
            <div className="p-5 bg-white rounded-2xl border border-slate-200/80 shadow-xs">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider block">累计请求吞吐</span>
              <span className="text-2xl font-black text-slate-900 font-mono mt-1 block">
                {stats.total_requests || 0}
              </span>
              <span className="text-[11px] text-emerald-600 font-medium">99.99% 可用性保障</span>
            </div>

            <div className="p-5 bg-white rounded-2xl border border-slate-200/80 shadow-xs">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider block">上游模型提供商</span>
              <span className="text-2xl font-black text-emerald-600 font-mono mt-1 block">
                {channelsCount || 0}
              </span>
              <span className="text-[11px] text-slate-500 font-medium">已接入 {modelsCount || 0} 个模型</span>
            </div>

            <div className="p-5 bg-white rounded-2xl border border-slate-200/80 shadow-xs">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider block">累计 Token 传输</span>
              <span className="text-2xl font-black text-sky-600 font-mono mt-1 block">
                {stats.total_tokens || 0}
              </span>
              <span className="text-[11px] text-slate-500 font-medium">零内存拷贝流式转发</span>
            </div>

            <div className="p-5 bg-white rounded-2xl border border-slate-200/80 shadow-xs">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider block">平均首字延迟 (TTFT)</span>
              <span className="text-2xl font-black text-amber-600 font-mono mt-1 block">
                {(stats.avg_ttft_ms || 0).toFixed(0)} <span className="text-xs text-slate-400 font-normal">ms</span>
              </span>
              <span className="text-[11px] text-slate-500 font-medium">毫秒级极速响应</span>
            </div>
          </div>
        </div>
      </section>

      {/* Core Architectural Capabilities */}
      <section id="features" className="py-24 px-6 max-w-6xl mx-auto w-full space-y-12">
        <div className="text-center max-w-2xl mx-auto space-y-3">
          <span className="text-xs font-bold text-indigo-600 tracking-wider uppercase px-3 py-1 rounded-full bg-indigo-50 border border-indigo-200">
            Core Capabilities
          </span>
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            工业级可靠性与核心优势
          </h2>
          <p className="text-sm text-slate-500">
            专为企业大模型落地研发，解决上游协议割裂、偶发限流与多模态分发难题
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-indigo-400 transition space-y-3 flex flex-col justify-between">
            <div className="space-y-3">
              <div className="w-11 h-11 rounded-2xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
                <Shield className="w-5 h-5" />
              </div>
              <h3 className="font-bold text-base text-slate-900">
                首字前无感容灾
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                遭遇上游 429 限流或网络异常时，首字发出前毫秒内切换到备份渠道，客户端连接不中断，完全无感。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-emerald-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>成功率稳定 99.99%</span>
            </div>
          </div>

          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-emerald-400 transition space-y-3 flex flex-col justify-between">
            <div className="space-y-3">
              <div className="w-11 h-11 rounded-2xl bg-emerald-50 border border-emerald-100 flex items-center justify-center text-emerald-600">
                <Cpu className="w-5 h-5" />
              </div>
              <h3 className="font-bold text-base text-slate-900">
                全协议双向转译
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                原生支持 OpenAI、Claude Messages、Gemini 与主流协议全双工转译，思维链深度思考内容零拷贝透传。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-indigo-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>主流 SDK 自由互通</span>
            </div>
          </div>

          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-purple-400 transition space-y-3 flex flex-col justify-between">
            <div className="space-y-3">
              <div className="w-11 h-11 rounded-2xl bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600">
                <Layers className="w-5 h-5" />
              </div>
              <h3 className="font-bold text-base text-slate-900">
                全模态统一管道
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                Chat 对话、AI 生图、TTS 语音合成、Whisper 语音转写与视频生成采用统一分发管道，共享流控与计费。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-purple-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>图/音/视/文本全覆盖</span>
            </div>
          </div>

          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-sky-400 transition space-y-3 flex flex-col justify-between">
            <div className="space-y-3">
              <div className="w-11 h-11 rounded-2xl bg-sky-50 border border-sky-100 flex items-center justify-center text-sky-600">
                <Server className="w-5 h-6" />
              </div>
              <h3 className="font-bold text-base text-slate-900">
                多活热备与故障自愈
              </h3>
              <p className="text-xs text-slate-600 leading-relaxed">
                集群节点多活互备与实时健康巡检。发生网络抖动或服务异常时毫秒级自动隔离与接管，配置变更实时生效。
              </p>
            </div>
            <div className="pt-3 border-t border-slate-100 text-[11px] font-mono text-sky-600 font-semibold flex items-center space-x-1">
              <Check className="w-3.5 h-3.5" />
              <span>无单点故障 · 业务零中断</span>
            </div>
          </div>
        </div>
      </section>

      {/* Unified Multimodal & Low Footprint Matrix */}
      <section id="multimodal" className="py-24 px-6 max-w-6xl mx-auto w-full space-y-12 border-t border-slate-200/70">
        <div className="text-center max-w-2xl mx-auto space-y-3">
          <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-purple-50 border border-purple-200 text-purple-700 text-xs font-semibold">
            <Zap className="w-3.5 h-3.5" />
            <span>Unified Multimodal & Efficiency</span>
          </div>
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            全模态统一调度与极低资源消耗
          </h2>
          <p className="text-sm text-slate-500">
            依托零缓冲穿透流与对象存储直传，高并发多模态媒体带宽消耗直降 99%
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          {/* 1. Chat */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4 flex flex-col justify-between hover:border-indigo-400 transition">
            <div className="space-y-3">
              <div className="w-11 h-11 rounded-2xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
                <MessageSquare className="w-5 h-5" />
              </div>
              <h4 className="font-bold text-base text-slate-900">文本与深度推理</h4>
              <p className="text-xs text-slate-600 leading-relaxed">
                聚合 GPT-6、Claude Opus 5.5 与 DeepSeek-R1 全系列。基于会话哈希锁定同一 Provider，享受 90% Prompt 缓存优惠。
              </p>
            </div>
            <div className="space-y-2 pt-3 border-t border-slate-100">
              <div className="font-mono text-[11px] text-indigo-600 font-semibold">
                POST /v1/chat/completions
              </div>
              <div className="text-[10px] text-emerald-600 font-medium bg-emerald-50 px-2 py-0.5 rounded w-fit">
                ⚡ 缓存立减 90%
              </div>
            </div>
          </div>

          {/* 2. Image */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4 flex flex-col justify-between hover:border-amber-400 transition">
            <div className="space-y-3">
              <div className="w-11 h-11 rounded-2xl bg-amber-50 border border-amber-100 flex items-center justify-center text-amber-600">
                <ImageIcon className="w-5 h-5" />
              </div>
              <h4 className="font-bold text-base text-slate-900">超清生图与视觉</h4>
              <p className="text-xs text-slate-600 leading-relaxed">
                支持 FLUX.1-Pro 超清生图与视觉分析。支持 OSS/S3 预签名直传，二进制绕行 CDN，网关带宽消耗直降 99%。
              </p>
            </div>
            <div className="space-y-2 pt-3 border-t border-slate-100">
              <div className="font-mono text-[11px] text-amber-600 font-semibold">
                POST /v1/images/generations
              </div>
              <div className="text-[10px] text-amber-700 font-medium bg-amber-50 px-2 py-0.5 rounded w-fit">
                💡 直传分流节省 99% 带宽
              </div>
            </div>
          </div>

          {/* 3. Audio */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4 flex flex-col justify-between hover:border-teal-400 transition">
            <div className="space-y-3">
              <div className="w-11 h-11 rounded-2xl bg-teal-50 border border-teal-100 flex items-center justify-center text-teal-600">
                <Volume2 className="w-5 h-5" />
              </div>
              <h4 className="font-bold text-base text-slate-900">实时语音与转写</h4>
              <p className="text-xs text-slate-600 leading-relaxed">
                支持 16kHz mono Opus 压缩格式（体积立减 95%）；Whisper-Large-V3-Turbo 毫秒级语音转文字。
              </p>
            </div>
            <div className="space-y-2 pt-3 border-t border-slate-100">
              <div className="font-mono text-[11px] text-teal-600 font-semibold">
                POST /v1/audio/transcriptions
              </div>
              <div className="text-[10px] text-teal-700 font-medium bg-teal-50 px-2 py-0.5 rounded w-fit">
                🎵 Opus 压缩体积缩减 95%
              </div>
            </div>
          </div>

          {/* 4. Video */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4 flex flex-col justify-between hover:border-purple-400 transition">
            <div className="space-y-3">
              <div className="w-11 h-11 rounded-2xl bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600">
                <Video className="w-5 h-5" />
              </div>
              <h4 className="font-bold text-base text-slate-900">影视级视频生成</h4>
              <p className="text-xs text-slate-600 leading-relaxed">
                集成 Sora 2 影视级视频生成，原生支持异步任务分发、生命周期状态轮询与视频直链获取。
              </p>
            </div>
            <div className="space-y-2 pt-3 border-t border-slate-100">
              <div className="font-mono text-[11px] text-purple-600 font-semibold">
                POST /v1/videos/generations
              </div>
              <div className="text-[10px] text-purple-700 font-medium bg-purple-50 px-2 py-0.5 rounded w-fit">
                🎬 异步任务自动轮询
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Developer Documentation Section */}
      <section id="docs" className="py-24 px-6 max-w-6xl mx-auto w-full space-y-12 border-t border-slate-200/70">
        <div className="text-center max-w-2xl mx-auto space-y-3">
          <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-indigo-50 border border-indigo-200 text-indigo-700 text-xs font-semibold">
            <Code className="w-3.5 h-3.5" />
            <span>Developer Guide</span>
          </div>
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            极简接入与快速开始
          </h2>
          <p className="text-sm text-slate-500">
            仅需调整 <code className="px-1.5 py-0.5 rounded bg-slate-100 font-mono text-indigo-600 font-semibold">base_url</code> 与密钥，即刻享受高可用调度与分流
          </p>
        </div>

        {/* Tabbed Code Box */}
        <div className="bg-white border border-slate-200/90 rounded-3xl overflow-hidden shadow-sm">
          <div className="flex flex-wrap items-center justify-between border-b border-slate-200 bg-slate-50/80 px-4 py-2 gap-2">
            <div className="flex flex-wrap items-center gap-1.5">
              {[
                { id: 'python', label: 'Python (OpenAI)' },
                { id: 'claude', label: 'Anthropic Claude' },
                { id: 'curl', label: 'cURL 命令行' },
                { id: 'multimodal', label: '全模态 API' },
                { id: 'session', label: '会话缓存 (90% 降本)' }
              ].map(t => (
                <button
                  key={t.id}
                  onClick={() => setActiveSnippetTab(t.id)}
                  className={`px-3 py-1.5 rounded-xl text-xs font-semibold transition cursor-pointer ${
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
                else if (activeSnippetTab === 'multimodal') code = multimodalSnippet;
                else if (activeSnippetTab === 'session') code = sessionSnippet;
                copyCode(code, activeSnippetTab);
              }}
              className="px-3.5 py-1.5 rounded-xl bg-white border border-slate-200 hover:border-indigo-400 text-xs text-slate-700 font-semibold flex items-center space-x-1.5 transition shadow-2xs cursor-pointer"
            >
              {copiedSnippet === activeSnippetTab ? (
                <>
                  <Check className="w-3.5 h-3.5 text-emerald-600" />
                  <span className="text-emerald-600 font-semibold">已复制</span>
                </>
              ) : (
                <>
                  <Copy className="w-3.5 h-3.5 text-slate-500" />
                  <span>复制代码</span>
                </>
              )}
            </button>
          </div>

          <div className="p-6 bg-slate-900 text-slate-100 font-mono text-xs overflow-x-auto leading-relaxed selection:bg-indigo-600">
            <pre>
              {activeSnippetTab === 'python' && pythonSnippet}
              {activeSnippetTab === 'claude' && claudeSnippet}
              {activeSnippetTab === 'curl' && curlSnippet}
              {activeSnippetTab === 'multimodal' && multimodalSnippet}
              {activeSnippetTab === 'session' && sessionSnippet}
            </pre>
          </div>
        </div>

        {/* Standardized 3-Tier URL Architecture */}
        <div className="space-y-4">
          <div className="flex items-center space-x-2 text-slate-900 font-bold text-xs uppercase tracking-wider">
            <Terminal className="w-4 h-4 text-indigo-600" />
            <span>三层规范化 URL 路由体系 (Standardized URL Architecture)</span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {/* 1. Data Plane */}
            <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-2xs space-y-4 flex flex-col justify-between">
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-xs text-indigo-700 bg-indigo-50 px-2.5 py-1 rounded-lg border border-indigo-200">
                    标准推理数据面 (/v1)
                  </span>
                  <span className="text-[10px] text-slate-400 font-mono">SDK 原生直连</span>
                </div>
                <p className="text-xs text-slate-500 leading-relaxed">
                  标准模型推理接口，完全兼容 OpenAI、Claude、Gemini 原生 SDK，支持极速流式打字机响应。
                </p>
                <div className="space-y-2 text-xs font-mono">
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-indigo-600 font-semibold text-[11px]">
                    POST /v1/chat/completions
                  </div>
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-purple-600 font-semibold text-[11px]">
                    POST /v1/messages
                  </div>
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-amber-600 font-semibold text-[11px]">
                    POST /v1/images/generations
                  </div>
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-emerald-600 font-semibold text-[11px]">
                    POST /v1/audio/transcriptions
                  </div>
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-rose-600 font-semibold text-[11px]">
                    POST /v1/videos/generations
                  </div>
                </div>
              </div>
              <div className="pt-3 border-t border-slate-100 text-[11px] text-slate-400">
                支持 Bearer Token 与 X-Session-ID
              </div>
            </div>

            {/* 2. Control Plane */}
            <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-2xs space-y-4 flex flex-col justify-between">
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-xs text-purple-700 bg-purple-50 px-2.5 py-1 rounded-lg border border-purple-200">
                    统一后端管控面 (/api)
                  </span>
                  <span className="text-[10px] text-slate-400 font-mono">Control Plane</span>
                </div>
                <p className="text-xs text-slate-500 leading-relaxed">
                  网关内部控制面与运营管理 API，严格统一收口在 /api/v1 路径下，支持权限隔离与网关同步。
                </p>
                <div className="space-y-2 text-xs font-mono">
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-purple-700 font-semibold text-[11px]">
                    /api/v1/auth/* (登录/注册/OAuth)
                  </div>
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-purple-700 font-semibold text-[11px]">
                    /api/v1/user/* (钱包/充值/密钥)
                  </div>
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-purple-700 font-semibold text-[11px]">
                    /api/v1/admin/* (路由/渠道/计费)
                  </div>
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-emerald-700 font-semibold text-[11px]">
                    /api/v1/public/status (公网 SLA)
                  </div>
                  <div className="p-2 rounded-xl bg-slate-50 border border-slate-100 text-slate-600 font-semibold text-[11px]">
                    /health (健康检测)
                  </div>
                </div>
              </div>
              <div className="pt-3 border-t border-slate-100 text-[11px] text-slate-400">
                全栈 RESTful API + JWT 鉴权
              </div>
            </div>

            {/* 3. Web Entry */}
            <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-2xs space-y-4 flex flex-col justify-between">
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-xs text-emerald-700 bg-emerald-50 px-2.5 py-1 rounded-lg border border-emerald-200">
                    极简应用访问入口 (Web)
                  </span>
                  <span className="text-[10px] text-slate-400 font-mono">SaaS Entry</span>
                </div>
                <p className="text-xs text-slate-500 leading-relaxed">
                  提供无缝直达的用户工作台，根路径直接载入 SPA 应用，即开即用。
                </p>
                <div className="space-y-2 text-xs font-mono">
                  <div className="p-2.5 rounded-xl bg-emerald-50/70 border border-emerald-200 text-emerald-800 font-semibold text-[11px] flex justify-between items-center">
                    <span>GET /</span>
                    <span className="font-sans font-bold text-[10px] text-emerald-600">根路径零跳转直达</span>
                  </div>
                  <div className="p-2.5 rounded-xl bg-slate-50 border border-slate-100 text-indigo-600 font-semibold text-[11px] flex justify-between items-center">
                    <span>GET /app/</span>
                    <span className="font-sans text-[10px] text-slate-500">标准应用工作台入口</span>
                  </div>
                </div>
              </div>
              <div className="pt-3 border-t border-slate-100 text-[11px] text-emerald-600 font-medium">
                干净纯粹 · 零重定向摩擦
              </div>
            </div>
          </div>
        </div>

        {/* Request Headers Strip */}
        <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-2xs space-y-3">
          <div className="flex items-center space-x-2 text-slate-900 font-bold text-xs uppercase tracking-wider">
            <Shield className="w-4 h-4 text-indigo-600" />
            <span>核心调度与认证请求头速查</span>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-xs">
            <div className="p-3.5 rounded-xl bg-slate-50 border border-slate-100 space-y-1">
              <div className="flex items-center justify-between">
                <span className="font-mono font-bold text-slate-800">Authorization: Bearer sk-airoute-...</span>
                <span className="text-indigo-600 font-semibold text-[10px] bg-indigo-50 px-1.5 py-0.5 rounded border border-indigo-200">必填</span>
              </div>
              <p className="text-[11px] text-slate-500">工作台签发的 API 密钥，毫秒级内存校验、租户限流与余额计费。</p>
            </div>

            <div className="p-3.5 rounded-xl bg-slate-50 border border-slate-100 space-y-1">
              <div className="flex items-center justify-between">
                <span className="font-mono font-bold text-indigo-700">X-Session-ID: &lt;session_id&gt;</span>
                <span className="text-emerald-600 font-semibold bg-emerald-50 px-1.5 py-0.5 rounded border border-emerald-200 text-[10px]">90% 降本推荐</span>
              </div>
              <p className="text-[11px] text-slate-500">启用一致性哈希会话黏连，保持同一 Provider 锁定，复用 Prefix KV 缓存。</p>
            </div>

            <div className="p-3.5 rounded-xl bg-slate-50 border border-slate-100 space-y-1">
              <div className="flex items-center justify-between">
                <span className="font-mono font-bold text-slate-800">x-api-key: sk-airoute-...</span>
                <span className="text-slate-400 text-[10px]">SDK 兼容</span>
              </div>
              <p className="text-[11px] text-slate-500">Anthropic Claude 原生 SDK 请求头，网关双向透明自动识别转译。</p>
            </div>
          </div>
        </div>
      </section>

      {/* Model Pricing & Prompt Caching Section */}
      <section id="pricing" className="py-24 px-6 max-w-6xl mx-auto w-full space-y-12 border-t border-slate-200/80">
        <div className="text-center max-w-2xl mx-auto space-y-3">
          <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-emerald-50 border border-emerald-200 text-emerald-700 text-xs font-semibold">
            <Sparkles className="w-3.5 h-3.5" />
            <span>2026 Model Matrix & Pricing</span>
          </div>
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            2026 旗舰模型矩阵与基准费率
          </h2>
          <p className="text-sm text-slate-500">
            支持 Prompt 缓存优惠（立减 90%）与灵活分时策略优惠（支持多时段与周末半价），折上折最高节约 95% 成本
          </p>
        </div>

        {/* Pricing Matrix Table */}
        <div className="bg-white border border-slate-200/90 rounded-3xl overflow-hidden shadow-sm">
          <div className="p-5 border-b border-slate-100 bg-slate-50/50 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h3 className="font-bold text-sm text-slate-900">主流模型费率对照表 (Rates per 1M Tokens)</h3>
              <p className="text-xs text-slate-500 mt-0.5">网关实时计算 Token 与时间段，并在响应头返回 X-Airoute-Cost 计费信息</p>
            </div>
            <div className="flex items-center space-x-2">
              <span className="text-[11px] font-semibold px-2.5 py-1 rounded-lg bg-indigo-50 text-indigo-700 border border-indigo-200 flex items-center space-x-1">
                <Moon className="w-3 h-3" />
                <span>🌙 灵活分时时段优惠</span>
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
                  <th className="py-3 px-4 font-semibold">🌙 闲时优惠折率</th>
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
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-slate-100 text-slate-600">Agentic</span>
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
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-amber-50 text-amber-700 border border-amber-200">实时语音模态</span>
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
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-slate-100 text-slate-600">深度推理</span>
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
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-amber-50 text-amber-700 border border-amber-200">超清生图</span>
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
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-rose-50 text-rose-700 border border-rose-200">视频生成</span>
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
                    <span className="text-[10px] font-sans font-normal px-1.5 py-0.5 rounded bg-teal-50 text-teal-700 border border-teal-200">极速 STT</span>
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
      <footer className="mt-auto border-t border-slate-200 bg-white py-12 px-6 text-xs text-slate-500">
        <div className="max-w-7xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center space-x-2">
            <span className="font-extrabold text-slate-800 text-sm">Airoute</span>
            <span>· © {new Date().getFullYear()} 极简高性能 AI 路由器</span>
          </div>

          <div className="flex items-center space-x-6">
            {isLoggedIn ? (
              <button onClick={onEnterConsole} className="text-indigo-600 hover:underline font-semibold cursor-pointer">
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
            <a href="https://github.com/ifnodoraemon/airoute" target="_blank" rel="noreferrer" className="hover:text-slate-800">
              GitHub 源码
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}
