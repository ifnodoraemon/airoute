import React, { useState } from 'react';
import {
  Shield,
  Cpu,
  Layers,
  Server,
  Code,
  Key,
  Check,
  Copy,
  ArrowRight,
  MessageSquare,
  Image as ImageIcon,
  Volume2,
  Video
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
    model="gpt-6",  # 原生支持 GPT-6 / Claude Opus 5.5 / Gemini 4 Argon / DeepSeek V4.1
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
    model="claude-opus-5.5",  # 支持 Claude 5.5 (Opus / Sonnet / Haiku / Fable) 全系模型
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
    "model": "gpt-6",
    "messages": [{"role": "user", "content": "你好，请介绍系统优势"}],
    "stream": true
  }'`;

  const multimodalSnippet = `# 1. 图像生成 (FLUX.1-Pro - 原生 2K/4K 与精确微调)
curl -X POST "${origin}/v1/images/generations" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "flux-1.1-pro", "prompt": "极简科技风云原生 AI 路由器", "size": "1024x1024"}'

# 2. 实时流式语音转写 (GPT Live Transcribe，支持 16kHz mono Opus 压缩格式)
curl -X POST "${origin}/v1/audio/transcriptions" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -F file="@speech.opus" \\
  -F model="gpt-live-transcribe"

# 3. 影视级视频生成与状态轮询 (CogVideoX-5B / Kling 4.0)
curl -X POST "${origin}/v1/videos/generations" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "cogvideox-5b", "prompt": "未来赛博朋克城市雨夜飞车", "aspect_ratio": "16:9"}'

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
        </div>
      </section>

      {/* Core Architectural Capabilities */}
      <section id="features" className="py-28 px-6 max-w-6xl mx-auto w-full space-y-14">
        <div className="text-center max-w-2xl mx-auto">
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            工业级可靠性与核心优势
          </h2>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-indigo-400 transition space-y-3">
            <div className="w-11 h-11 rounded-2xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
              <Shield className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-base text-slate-900">
              首字前无感容灾
            </h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              遭遇上游 429 限流或网络异常时，首字发出前毫秒内切换到备份渠道，客户端连接不中断。
            </p>
          </div>

          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-emerald-400 transition space-y-3">
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

          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-purple-400 transition space-y-3">
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

          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs hover:border-sky-400 transition space-y-3">
            <div className="w-11 h-11 rounded-2xl bg-sky-50 border border-sky-100 flex items-center justify-center text-sky-600">
              <Server className="w-5 h-6" />
            </div>
            <h3 className="font-bold text-base text-slate-900">
              多活热备与故障自愈
            </h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              集群节点多活互备与实时健康巡检。发生网络抖动或服务异常时毫秒级自动隔离与接管。
            </p>
          </div>
        </div>
      </section>

      {/* Unified Multimodal Matrix */}
      <section id="multimodal" className="py-28 px-6 max-w-6xl mx-auto w-full space-y-14 border-t border-slate-200/70">
        <div className="text-center max-w-2xl mx-auto">
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            全模态统一调度与极低资源消耗
          </h2>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          {/* 1. Chat */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4 hover:border-indigo-400 transition">
            <div className="w-11 h-11 rounded-2xl bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
              <MessageSquare className="w-5 h-5" />
            </div>
            <h4 className="font-bold text-base text-slate-900">文本与深度推理</h4>
            <p className="text-xs text-slate-600 leading-relaxed">
              聚合 GPT-6、Claude Opus 5.5 与 DeepSeek-R1 全系列。基于会话哈希锁定同一 Provider，享受 90% Prompt 缓存优惠。
            </p>
            <div className="pt-2 font-mono text-[11px] text-indigo-600 font-semibold">
              POST /v1/chat/completions
            </div>
          </div>

          {/* 2. Image */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4 hover:border-amber-400 transition">
            <div className="w-11 h-11 rounded-2xl bg-amber-50 border border-amber-100 flex items-center justify-center text-amber-600">
              <ImageIcon className="w-5 h-5" />
            </div>
            <h4 className="font-bold text-base text-slate-900">超清生图与视觉</h4>
            <p className="text-xs text-slate-600 leading-relaxed">
              支持 FLUX.1-Pro 超清生图与视觉分析。支持 OSS/S3 预签名直传，二进制绕行 CDN，网关带宽消耗直降 99%。
            </p>
            <div className="pt-2 font-mono text-[11px] text-amber-600 font-semibold">
              POST /v1/images/generations
            </div>
          </div>

          {/* 3. Audio */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4 hover:border-teal-400 transition">
            <div className="w-11 h-11 rounded-2xl bg-teal-50 border border-teal-100 flex items-center justify-center text-teal-600">
              <Volume2 className="w-5 h-5" />
            </div>
            <h4 className="font-bold text-base text-slate-900">实时语音与转写</h4>
            <p className="text-xs text-slate-600 leading-relaxed">
              支持 16kHz mono Opus 压缩格式（体积立减 95%）；Whisper-Large-V3-Turbo 毫秒级语音转文字。
            </p>
            <div className="pt-2 font-mono text-[11px] text-teal-600 font-semibold">
              POST /v1/audio/transcriptions
            </div>
          </div>

          {/* 4. Video */}
          <div className="bg-white border border-slate-200/80 rounded-3xl p-6 shadow-xs space-y-4 hover:border-purple-400 transition">
            <div className="w-11 h-11 rounded-2xl bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600">
              <Video className="w-5 h-5" />
            </div>
            <h4 className="font-bold text-base text-slate-900">影视级视频生成</h4>
            <p className="text-xs text-slate-600 leading-relaxed">
              集成 Sora 2 影视级视频生成，原生支持异步任务分发、生命周期状态轮询与视频直链获取。
            </p>
            <div className="pt-2 font-mono text-[11px] text-purple-600 font-semibold">
              POST /v1/videos/generations
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
      </section>

      {/* Model Pricing & Prompt Caching Section */}
      <section id="pricing" className="py-28 px-6 max-w-6xl mx-auto w-full space-y-12 border-t border-slate-200/80">
        <div className="text-center max-w-2xl mx-auto space-y-3">
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            2026 旗舰模型矩阵与基准费率
          </h2>
          <p className="text-sm text-slate-500">
            支持 Prompt 缓存优惠（立减 90%）与灵活分时策略优惠（支持多时段与周末半价）
          </p>
        </div>

        {/* Pricing Matrix Table */}
        <div className="bg-white border border-slate-200/90 rounded-3xl overflow-hidden shadow-sm">
          <div className="p-5 border-b border-slate-100 bg-slate-50/50 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <h3 className="font-bold text-sm text-slate-900">主流模型费率对照表 (Rates per 1M Tokens)</h3>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/80 text-slate-400 uppercase tracking-wider text-[11px]">
                  <th className="py-3 px-5 font-semibold">模型系列 (Series)</th>
                  <th className="py-3 px-4 font-semibold">基准输入 (/1M)</th>
                  <th className="py-3 px-4 font-semibold">基准输出 (/1M)</th>
                  <th className="py-3 px-4 font-semibold">闲时优惠折率</th>
                  <th className="py-3 px-4 font-semibold">Prompt 缓存命中 (/1M)</th>
                  <th className="py-3 px-4 font-semibold">固定单次费用</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 text-slate-700 font-mono">
                {/* 1. OpenAI GPT-6 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-emerald-600"></span>
                    <span>GPT-6 / GPT-6 Luna (OpenAI 系列)</span>
                  </td>
                  <td className="py-3.5 px-4">¥12.00 ~ ¥25.00</td>
                  <td className="py-3.5 px-4">¥48.00 ~ ¥100.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-400">标准计费</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥6.00 ~ ¥12.50</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                </tr>

                {/* 2. Anthropic Claude 5.5 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-amber-600"></span>
                    <span>Claude Opus 5.5 / Sonnet 5.5 (Claude 系列)</span>
                  </td>
                  <td className="py-3.5 px-4">¥15.00 ~ ¥30.00</td>
                  <td className="py-3.5 px-4">¥75.00 ~ ¥150.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-400">标准计费</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥1.50 ~ ¥3.00</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                </tr>

                {/* 3. Google Gemini 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-blue-600"></span>
                    <span>Gemini 4 Argon / 3.8 Flash (Gemini 系列)</span>
                  </td>
                  <td className="py-3.5 px-4">¥1.00 ~ ¥18.00</td>
                  <td className="py-3.5 px-4">¥4.00 ~ ¥72.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-400">标准计费</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.25 ~ ¥4.50</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                </tr>

                {/* 4. DeepSeek 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-indigo-600"></span>
                    <span>DeepSeek V4.1-Flash / R1 (DeepSeek 系列)</span>
                  </td>
                  <td className="py-3.5 px-4">¥1.50 ~ ¥4.00</td>
                  <td className="py-3.5 px-4">¥6.00 ~ ¥16.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.75 / ¥2.00 (半价)</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.35 ~ ¥1.00</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                </tr>

                {/* 5. 阿里通义千问 Qwen 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-teal-600"></span>
                    <span>通义千问 Qwen-3.8 / Qwen-Coder</span>
                  </td>
                  <td className="py-3.5 px-4">¥3.50 ~ ¥5.00</td>
                  <td className="py-3.5 px-4">¥14.00 ~ ¥20.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-400">标准计费</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.85 ~ ¥1.25</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                </tr>

                {/* 6. 智谱 GLM 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-sky-600"></span>
                    <span>智谱 GLM-5.3 Agent 系列</span>
                  </td>
                  <td className="py-3.5 px-4">¥6.00</td>
                  <td className="py-3.5 px-4">¥24.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-400">标准计费</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥1.50</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                </tr>

                {/* 7. FLUX 图像生成系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-rose-500"></span>
                    <span>FLUX.1-Pro / FLUX.1-Schnell (图像系列)</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.10 / 张</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">¥0.05 ~ ¥0.15 / 张</td>
                </tr>

                {/* 8. 视频生成系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-purple-600"></span>
                    <span>CogVideoX-5B / Kling 4.0 (视频系列)</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.50 / 次</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">¥0.80 / 次</td>
                </tr>

                {/* 9. 实时语音与 Whisper 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-cyan-600"></span>
                    <span>GPT Live Transcribe / Whisper-Large-V3</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">¥0.015 / 分钟</td>
                  <td className="py-3.5 px-4 text-slate-400">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">¥0.02 ~ ¥0.03 / 分钟</td>
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
