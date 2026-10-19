import React, { useState, useEffect } from 'react';
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
  Video,
  Globe
} from 'lucide-react';
import { getGatewayOrigin, copyToClipboard } from '../config';

export default function LandingPage({
  isLoggedIn,
  adminUser,
  onOpenLogin,
  onEnterConsole,
  onViewDocs,
  onViewStatus,
  stats,
  channelsCount,
  modelsCount,
  lang = 'zh',
  setLang,
  toggleLang,
  t
}) {
  const isZh = lang === 'zh';
  const [copiedSnippet, setCopiedSnippet] = useState('');
  const [activeSnippetTab, setActiveSnippetTab] = useState('python');

  const scrollToSection = (id) => {
    const el = document.getElementById(id);
    if (el) {
      const headerOffset = 70;
      const elementPosition = el.getBoundingClientRect().top;
      const offsetPosition = elementPosition + window.pageYOffset - headerOffset;
      window.scrollTo({
        top: offsetPosition,
        behavior: 'smooth'
      });
    }
  };

  useEffect(() => {
    const hash = window.location.hash.replace(/^#\/?/, '').trim();
    if (['features', 'multimodal', 'docs', 'pricing'].includes(hash)) {
      setTimeout(() => {
        scrollToSection(hash);
      }, 150);
    }
  }, []);

  const origin = getGatewayOrigin();

  const copyCode = async (text, key) => {
    await copyToClipboard(text);
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
          <div
            onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
            className="flex items-center space-x-3 cursor-pointer select-none"
          >
            <div className="w-9 h-9 rounded-xl bg-linear-to-tr from-indigo-600 to-sky-500 flex items-center justify-center text-white shadow-sm font-black text-lg">
              ⚡
            </div>
            <span className="font-extrabold text-lg tracking-tight bg-linear-to-r from-indigo-700 via-purple-700 to-sky-600 bg-clip-text text-transparent">
              {isZh ? 'AI 路由器' : 'Airoute'}
            </span>
          </div>

          <nav className="hidden md:flex items-center space-x-8 text-xs font-semibold text-slate-600">
            <a
              href="#features"
              onClick={(e) => {
                e.preventDefault();
                scrollToSection('features');
              }}
              className="hover:text-indigo-600 transition cursor-pointer"
            >
              {isZh ? '核心特性' : 'Features'}
            </a>
            <a
              href="#multimodal"
              onClick={(e) => {
                e.preventDefault();
                scrollToSection('multimodal');
              }}
              className="hover:text-indigo-600 transition cursor-pointer"
            >
              {isZh ? '全模态矩阵' : 'Multimodal'}
            </a>
            <a
              href="#docs"
              onClick={(e) => {
                e.preventDefault();
                scrollToSection('docs');
              }}
              className="hover:text-indigo-600 transition cursor-pointer"
            >
              {isZh ? '接入指南' : 'Guides'}
            </a>
            <a
              href="#pricing"
              onClick={(e) => {
                e.preventDefault();
                scrollToSection('pricing');
              }}
              className="hover:text-indigo-600 transition cursor-pointer"
            >
              {isZh ? '模型定价' : 'Pricing'}
            </a>
          </nav>

          <div className="flex items-center space-x-3">
            {toggleLang && (
              <button
                type="button"
                onClick={toggleLang}
                className="flex text-xs px-3 py-1.5 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold items-center space-x-1.5 transition border border-slate-200 cursor-pointer"
              >
                <Globe className="w-3.5 h-3.5 text-indigo-600" />
                <span>{isZh ? 'EN' : '中'}</span>
              </button>
            )}

            <button
              type="button"
              onClick={onViewStatus}
              className="flex text-xs px-3 py-1.5 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-medium items-center space-x-1.5 transition border border-slate-200 cursor-pointer"
            >
              <span className="w-2 h-2 rounded-full bg-slate-400"></span>
              <span>{isZh ? '服务状态' : 'Status'}</span>
            </button>

            {isLoggedIn ? (
              <button
                onClick={onEnterConsole}
                className="px-4 py-2 bg-linear-to-r from-indigo-600 to-sky-600 hover:from-indigo-700 hover:to-sky-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
              >
                <Server className="w-3.5 h-3.5" />
                <span>{isZh ? '进入工作台 →' : 'Console →'}</span>
              </button>
            ) : (
              <button
                onClick={onOpenLogin}
                className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
              >
                <Key className="w-3.5 h-3.5" />
                <span>{isZh ? '登录 / 注册' : 'Sign In / Register'}</span>
              </button>
            )}
          </div>
        </div>
      </header>

      {/* Hero Section */}
      <section className="relative pt-20 pb-24 px-6 overflow-hidden bg-linear-to-b from-indigo-50/50 via-white to-slate-50 border-b border-slate-200/60">
        <div className="max-w-4xl mx-auto text-center space-y-8">
          {/* This page has no live health probe, so the badge states configuration
              facts plus an explicit unknown status instead of claiming the
              cluster is running. */}
          <div
            onClick={onViewStatus}
            title={isZh
              ? '本页未进行健康探测，"状态未知"仅表示未在此处检测运行状况，实际状态请查看状态页'
              : 'This page does not probe health. "Status unknown" means no check ran here; see the status page for live state.'}
            className="inline-flex flex-wrap items-center justify-center gap-x-2 gap-y-1 px-4 py-1.5 rounded-full bg-white border border-slate-200 shadow-sm text-xs font-semibold text-slate-700 hover:border-indigo-400 hover:shadow-md cursor-pointer transition"
          >
            <span className="w-2 h-2 rounded-full bg-slate-400"></span>
            <span>{isZh ? '统一网关 · 多提供商与降级路由' : 'Unified Gateway · Multi-Provider Routing'}</span>
            <span className="text-slate-300">|</span>
            <span className="text-slate-500 font-mono">{isZh ? '运行状态未知' : 'Runtime status unknown'}</span>
            <span className="text-slate-300">|</span>
            <span className="text-indigo-600 font-medium">{isZh ? '查看状态详情 →' : 'View status →'}</span>
          </div>

          <h1 className="text-4xl sm:text-5xl lg:text-6xl font-black text-slate-900 tracking-tight leading-tight">
            {isZh ? (
              <>
                2026 旗舰大模型与多模态<br />
                <span className="bg-linear-to-r from-indigo-600 via-purple-600 to-sky-600 bg-clip-text text-transparent">
                  统一调度与高性能接入网关
                </span>
              </>
            ) : (
              <>
                Next-Gen LLM & Multimodal<br />
                <span className="bg-linear-to-r from-indigo-600 via-purple-600 to-sky-600 bg-clip-text text-transparent">
                  Unified AI Router & Gateway
                </span>
              </>
            )}
          </h1>

          <p className="text-base sm:text-lg text-slate-600 max-w-2xl mx-auto leading-relaxed">
            {isZh ? (
              <>
                多协议接入 · 流式转发 · 首字前故障回退 · 用量与成本审计<br />
                统一收口 OpenAI、Claude、Gemini、DeepSeek 与全模态生成管线。
              </>
            ) : (
              <>
                Multi-protocol access · Streaming relay · Pre-content failover · Usage and cost audit<br />
                Unified aggregation for OpenAI, Claude, Gemini, DeepSeek, and multimodal pipelines.
              </>
            )}
          </p>

          <div className="flex flex-col sm:flex-row items-center justify-center gap-4 pt-2">
            {isLoggedIn ? (
              <button
                onClick={onEnterConsole}
                className="w-full sm:w-auto px-6 py-3 bg-slate-900 hover:bg-slate-800 text-white rounded-lg text-sm font-medium shadow-sm transition flex items-center justify-center space-x-2 cursor-pointer"
              >
                <Server className="w-4 h-4" />
                <span>{isZh ? '进入工作台' : 'Enter Console'}</span>
                <ArrowRight className="w-4 h-4" />
              </button>
            ) : (
              <button
                onClick={onOpenLogin}
                className="w-full sm:w-auto px-6 py-3 bg-slate-900 hover:bg-slate-800 text-white rounded-lg text-sm font-medium shadow-sm transition flex items-center justify-center space-x-2 cursor-pointer"
              >
                <Key className="w-4 h-4" />
                <span>{isZh ? '进入企业控制台' : 'Enter Admin Console'}</span>
                <ArrowRight className="w-4 h-4" />
              </button>
            )}

            <button
              onClick={() => {
                if (onViewDocs) {
                  onViewDocs();
                } else {
                  scrollToSection('docs');
                }
              }}
              className="w-full sm:w-auto px-6 py-3 bg-white hover:bg-slate-50 text-slate-700 border border-slate-200 rounded-lg text-sm font-medium shadow-sm transition flex items-center justify-center space-x-2 cursor-pointer"
            >
              <Code className="w-4 h-4 text-indigo-600" />
              <span>{isZh ? '查阅接入指南' : 'View Documentation'}</span>
            </button>
          </div>
        </div>
      </section>

      {/* Core Architectural Capabilities */}
      <section id="features" className="py-28 px-6 max-w-6xl mx-auto w-full space-y-14">
        <div className="text-center max-w-2xl mx-auto">
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            {isZh ? '工业级可靠性与核心优势' : 'Industrial-Grade Reliability & Core Advantages'}
          </h2>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-sm hover:border-indigo-400 transition space-y-3">
            <div className="w-10 h-10 rounded-lg bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
              <Shield className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-base text-slate-900">
              {isZh ? '首字前无感容灾' : 'Pre-Token Failover'}
            </h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              {isZh
                ? '遭遇上游 429 限流或网络异常时，首字发出前毫秒内切换到备份渠道，客户端连接不中断。'
                : 'Instantly reroutes to backup channels within milliseconds on upstream 429 or network errors before the first token.'}
            </p>
          </div>

          <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-sm hover:border-emerald-400 transition space-y-3">
            <div className="w-10 h-10 rounded-lg bg-emerald-50 border border-emerald-100 flex items-center justify-center text-emerald-600">
              <Cpu className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-base text-slate-900">
              {isZh ? '全协议双向转译' : 'Bi-Directional Translation'}
            </h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              {isZh
                ? '支持 OpenAI、Claude Messages 和 Gemini 接入；原生 Anthropic 渠道保留签名思考块，跨协议转换按目标格式处理。'
                : 'Supports OpenAI, Claude Messages, and Gemini clients. Native Anthropic routes preserve signed thinking blocks; cross-protocol conversion follows the target schema.'}
            </p>
          </div>

          <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-sm hover:border-purple-400 transition space-y-3">
            <div className="w-10 h-10 rounded-lg bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600">
              <Layers className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-base text-slate-900">
              {isZh ? '全模态统一管道' : 'Unified Multimodal Pipe'}
            </h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              {isZh
                ? 'Chat 对话、AI 生图、TTS 语音合成、Whisper 语音转写与视频生成采用统一分发管道，共享流控与计费。'
                : 'Chat, Image generation, TTS, Whisper transcription, and Video synthesis share unified routing and metering.'}
            </p>
          </div>

          <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-sm hover:border-sky-400 transition space-y-3">
            <div className="w-10 h-10 rounded-lg bg-sky-50 border border-sky-100 flex items-center justify-center text-sky-600">
              <Server className="w-5 h-5" />
            </div>
            <h3 className="font-bold text-base text-slate-900">
              {isZh ? '多活热备与故障自愈' : 'Active-Active Resilience'}
            </h3>
            <p className="text-xs text-slate-600 leading-relaxed">
              {isZh
                ? '集群节点多活互备与实时健康巡检。发生网络抖动或服务异常时毫秒级自动隔离与接管。'
                : 'Cluster nodes run active-active with live health inspections, automatically isolating failing upstreams.'}
            </p>
          </div>
        </div>
      </section>

      {/* Unified Multimodal Matrix */}
      <section id="multimodal" className="py-28 px-6 max-w-6xl mx-auto w-full space-y-14 border-t border-slate-200/70">
        <div className="text-center max-w-2xl mx-auto">
          <h2 className="text-3xl font-black text-slate-900 tracking-tight">
            {isZh ? '全模态统一调度与极低资源消耗' : 'Unified Multimodal Dispatch & Minimal Overhead'}
          </h2>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          {/* 1. Chat */}
          <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-sm space-y-4 hover:border-indigo-400 transition">
            <div className="w-10 h-10 rounded-lg bg-indigo-50 border border-indigo-100 flex items-center justify-center text-indigo-600">
              <MessageSquare className="w-5 h-5" />
            </div>
            <h4 className="font-bold text-base text-slate-900">{isZh ? '文本与深度推理' : 'Text & Reasoning'}</h4>
            <p className="text-xs text-slate-600 leading-relaxed">
              {isZh
                ? '聚合 GPT-6、Claude Opus 5.5 与 DeepSeek-R1 全系列。基于会话哈希锁定同一 Provider，享受 90% Prompt 缓存优惠。'
                : 'Aggregates GPT-6, Claude Opus 5.5, and DeepSeek-R1. Session affinity hash pins same provider for 90% prompt cache discounts.'}
            </p>
            <div className="pt-2 font-mono text-xs text-indigo-600 font-semibold">
              POST /v1/chat/completions
            </div>
          </div>

          {/* 2. Image */}
          <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-sm space-y-4 hover:border-amber-400 transition">
            <div className="w-10 h-10 rounded-lg bg-amber-50 border border-amber-100 flex items-center justify-center text-amber-600">
              <ImageIcon className="w-5 h-5" />
            </div>
            <h4 className="font-bold text-base text-slate-900">{isZh ? '超清生图与视觉' : 'Image & Vision'}</h4>
            <p className="text-xs text-slate-600 leading-relaxed">
              {isZh
                ? '支持 FLUX.1-Pro 超清生图与视觉分析。支持 OSS/S3 预签名直传，二进制绕行 CDN，网关带宽消耗直降 99%。'
                : 'Supports FLUX.1-Pro 2K/4K generation. Pre-signed OSS/S3 direct upload bypasses gateway bandwidth by 99%.'}
            </p>
            <div className="pt-2 font-mono text-xs text-amber-600 font-semibold">
              POST /v1/images/generations
            </div>
          </div>

          {/* 3. Audio */}
          <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-sm space-y-4 hover:border-teal-400 transition">
            <div className="w-10 h-10 rounded-lg bg-teal-50 border border-teal-100 flex items-center justify-center text-teal-600">
              <Volume2 className="w-5 h-5" />
            </div>
            <h4 className="font-bold text-base text-slate-900">{isZh ? '实时语音与转写' : 'Audio & Speech'}</h4>
            <p className="text-xs text-slate-600 leading-relaxed">
              {isZh
                ? '支持语音合成与转录请求转发；实际格式、速度和模型能力由上游提供商决定。'
                : 'Forwards speech generation and transcription requests; formats, latency, and model capabilities depend on the upstream provider.'}
            </p>
            <div className="pt-2 font-mono text-xs text-teal-600 font-semibold">
              POST /v1/audio/transcriptions
            </div>
          </div>

          {/* 4. Video */}
          <div className="bg-white border border-slate-200/80 rounded-2xl p-6 shadow-sm space-y-4 hover:border-purple-400 transition">
            <div className="w-10 h-10 rounded-lg bg-purple-50 border border-purple-100 flex items-center justify-center text-purple-600">
              <Video className="w-5 h-5" />
            </div>
            <h4 className="font-bold text-base text-slate-900">{isZh ? '影视级视频生成' : 'Cinematic Video'}</h4>
            <p className="text-xs text-slate-600 leading-relaxed">
              {isZh
                ? '集成 Sora 2 影视级视频生成，原生支持异步任务分发、生命周期状态轮询与视频直链获取。'
                : 'Integrates Sora 2 cinematic video generation, native async job queueing, state polling, and direct asset delivery.'}
            </p>
            <div className="pt-2 font-mono text-xs text-purple-600 font-semibold">
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
            {isZh ? '极简接入与快速开始' : 'Quick Integration & Developer Guide'}
          </h2>
          <p className="text-sm text-slate-500">
            {isZh ? (
              <>仅需调整 <code className="px-1.5 py-0.5 rounded-sm bg-slate-100 font-mono text-indigo-600 font-semibold">base_url</code> 与密钥，即刻享受高可用调度与分流</>
            ) : (
              <>Simply configure <code className="px-1.5 py-0.5 rounded-sm bg-slate-100 font-mono text-indigo-600 font-semibold">base_url</code> and API Key for instant high-availability routing</>
            )}
          </p>
        </div>

        {/* Tabbed Code Box */}
        <div className="bg-white border border-slate-200/90 rounded-2xl overflow-hidden shadow-sm">
          <div className="flex flex-wrap items-center justify-between border-b border-slate-200 bg-slate-50/80 px-4 py-2 gap-2">
            <div className="flex flex-wrap items-center gap-1.5">
              {[
                { id: 'python', label: 'Python (OpenAI)' },
                { id: 'claude', label: 'Anthropic Claude' },
                { id: 'curl', label: isZh ? 'cURL 命令行' : 'cURL CLI' },
                { id: 'multimodal', label: isZh ? '全模态 API' : 'Multimodal API' },
                { id: 'session', label: isZh ? '会话缓存 (90% 降本)' : 'Session Cache (90% Off)' }
              ].map(t => (
                <button
                  key={t.id}
                  onClick={() => setActiveSnippetTab(t.id)}
                  className={`px-3 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
                    activeSnippetTab === t.id
                      ? 'bg-white text-indigo-600 shadow-sm border border-slate-200/80'
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
              className="px-3.5 py-1.5 rounded-lg bg-white border border-slate-200 hover:border-indigo-400 text-xs text-slate-700 font-medium flex items-center space-x-1.5 transition shadow-sm cursor-pointer"
            >
              {copiedSnippet === activeSnippetTab ? (
                <>
                  <Check className="w-3.5 h-3.5 text-emerald-600" />
                  <span className="text-emerald-600 font-semibold">{isZh ? '已复制' : 'Copied'}</span>
                </>
              ) : (
                <>
                  <Copy className="w-3.5 h-3.5 text-slate-500" />
                  <span>{isZh ? '复制代码' : 'Copy Code'}</span>
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
            {isZh ? '2026 旗舰模型矩阵与基准费率' : '2026 Model Matrix & Reference Pricing'}
          </h2>
          <p className="text-sm text-slate-500">
            {isZh
              ? '支持 Prompt 缓存优惠（立减 90%）与灵活分时策略优惠（支持多时段与周末半价）'
              : 'Supports Prompt Caching discounts (up to 90% off) and off-peak / weekend discounts'}
          </p>
        </div>

        {/* Pricing Matrix Table */}
        <div className="bg-white border border-slate-200/90 rounded-2xl overflow-hidden shadow-sm">
          <div className="p-5 border-b border-slate-100 bg-slate-50/50 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <h3 className="font-bold text-sm text-slate-900">
              {isZh ? '主流模型费率对照表 (Rates per 1M Tokens)' : 'Benchmark Model Rates (per 1M Tokens)'}
            </h3>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/80 text-slate-500 uppercase tracking-wider text-xs">
                  <th className="py-3 px-5 font-semibold">{isZh ? '模型系列 (Series)' : 'Model Series'}</th>
                  <th className="py-3 px-4 font-semibold">{isZh ? '基准输入 (/1M)' : 'Base Input (/1M)'}</th>
                  <th className="py-3 px-4 font-semibold">{isZh ? '基准输出 (/1M)' : 'Base Output (/1M)'}</th>
                  <th className="py-3 px-4 font-semibold">{isZh ? '闲时优惠折率' : 'Off-Peak Rate'}</th>
                  <th className="py-3 px-4 font-semibold">{isZh ? 'Prompt 缓存命中 (/1M)' : 'Cache Hit (/1M)'}</th>
                  <th className="py-3 px-4 font-semibold">{isZh ? '固定单次费用' : 'Fixed Unit Cost'}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 text-slate-700 font-mono">
                {/* 1. OpenAI GPT-6 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-emerald-600"></span>
                    <span>GPT-6 / GPT-6 Luna (OpenAI)</span>
                  </td>
                  <td className="py-3.5 px-4">¥12.00 ~ ¥25.00</td>
                  <td className="py-3.5 px-4">¥48.00 ~ ¥100.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-500">{isZh ? '标准计费' : 'Standard'}</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥6.00 ~ ¥12.50</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                </tr>

                {/* 2. Anthropic Claude 5.5 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-amber-600"></span>
                    <span>Claude Opus 5.5 / Sonnet 5.5 (Claude)</span>
                  </td>
                  <td className="py-3.5 px-4">¥15.00 ~ ¥30.00</td>
                  <td className="py-3.5 px-4">¥75.00 ~ ¥150.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-500">{isZh ? '标准计费' : 'Standard'}</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥1.50 ~ ¥3.00</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                </tr>

                {/* 3. Google Gemini 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-blue-600"></span>
                    <span>Gemini 4 Argon / 3.8 Flash (Gemini)</span>
                  </td>
                  <td className="py-3.5 px-4">¥1.00 ~ ¥18.00</td>
                  <td className="py-3.5 px-4">¥4.00 ~ ¥72.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-500">{isZh ? '标准计费' : 'Standard'}</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.25 ~ ¥4.50</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                </tr>

                {/* 4. DeepSeek 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-indigo-600"></span>
                    <span>DeepSeek V4.1-Flash / R1 (DeepSeek)</span>
                  </td>
                  <td className="py-3.5 px-4">¥1.50 ~ ¥4.00</td>
                  <td className="py-3.5 px-4">¥6.00 ~ ¥16.00</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">{isZh ? '¥0.75 / ¥2.00 (半价)' : '¥0.75 / ¥2.00 (50% Off)'}</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.35 ~ ¥1.00</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                </tr>

                {/* 5. 阿里通义千问 Qwen 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-teal-600"></span>
                    <span>{isZh ? '通义千问 Qwen-3.8 / Qwen-Coder' : 'Alibaba Qwen-3.8 / Qwen-Coder'}</span>
                  </td>
                  <td className="py-3.5 px-4">¥3.50 ~ ¥5.00</td>
                  <td className="py-3.5 px-4">¥14.00 ~ ¥20.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-500">{isZh ? '标准计费' : 'Standard'}</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥0.85 ~ ¥1.25</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                </tr>

                {/* 6. 智谱 GLM 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-sky-600"></span>
                    <span>{isZh ? '智谱 GLM-5.3 Agent 系列' : 'Zhipu GLM-5.3 Agent'}</span>
                  </td>
                  <td className="py-3.5 px-4">¥6.00</td>
                  <td className="py-3.5 px-4">¥24.00</td>
                  <td className="py-3.5 px-4 font-semibold text-slate-500">{isZh ? '标准计费' : 'Standard'}</td>
                  <td className="py-3.5 px-4 text-emerald-600 font-bold">¥1.50</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                </tr>

                {/* 7. FLUX 图像生成系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-rose-500"></span>
                    <span>FLUX.1-Pro / FLUX.1-Schnell ({isZh ? '图像系列' : 'Image'})</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">{isZh ? '¥0.10 / 张' : '¥0.10 / img'}</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">{isZh ? '¥0.05 ~ ¥0.15 / 张' : '¥0.05 ~ ¥0.15 / img'}</td>
                </tr>

                {/* 8. 视频生成系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-purple-600"></span>
                    <span>CogVideoX-5B / Kling 4.0 ({isZh ? '视频系列' : 'Video'})</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">{isZh ? '¥0.50 / 次' : '¥0.50 / task'}</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">{isZh ? '¥0.80 / 次' : '¥0.80 / task'}</td>
                </tr>

                {/* 9. 实时语音与 Whisper 系列 */}
                <tr className="hover:bg-slate-50/60 transition">
                  <td className="py-3.5 px-5 font-bold text-slate-900 font-sans flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-cyan-600"></span>
                    <span>GPT Live Transcribe / Whisper-Large-V3</span>
                  </td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 font-semibold text-indigo-600">{isZh ? '¥0.015 / 分钟' : '¥0.015 / min'}</td>
                  <td className="py-3.5 px-4 text-slate-500">-</td>
                  <td className="py-3.5 px-4 font-bold text-slate-900">{isZh ? '¥0.02 ~ ¥0.03 / 分钟' : '¥0.02 ~ ¥0.03 / min'}</td>
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
            <span>· © {new Date().getFullYear()} {isZh ? '极简高性能 AI 路由器' : 'Minimalist High-Performance AI Router'}</span>
          </div>

          <div className="flex items-center space-x-6">
            {isLoggedIn ? (
              <button onClick={onEnterConsole} className="text-indigo-600 hover:underline font-semibold cursor-pointer">
                {isZh ? '进入工作台' : 'Enter Console'}
              </button>
            ) : (
              <button onClick={onOpenLogin} className="text-indigo-600 hover:underline font-semibold cursor-pointer">
                {isZh ? '登录 / 注册' : 'Sign In / Register'}
              </button>
            )}
            <button onClick={onViewStatus} className="hover:text-slate-800 cursor-pointer">
              {isZh ? '服务运行状态 (SLA)' : 'Service Status (SLA)'}
            </button>
            <a href="https://github.com/ifnodoraemon/airoute" target="_blank" rel="noreferrer" className="hover:text-slate-800">
              {isZh ? 'GitHub 源码' : 'GitHub Source'}
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}
