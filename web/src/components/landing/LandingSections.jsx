import { Cpu, Image, Layers, MessageSquare, Server, Shield, Video, Volume2 } from 'lucide-react';
import LandingPricing from './LandingPricing';

function capabilityCards(isZh) {
  return [
    { icon: Shield, title: isZh ? '首字前无感容灾' : 'Pre-Token Failover', detail: isZh ? '在流式内容提交前遇到上游限流或网络异常时，尝试可用的备用渠道。' : 'Try available backup channels on rate limits or network errors before committing stream content.' },
    { icon: Cpu, title: isZh ? '全协议双向转译' : 'Bi-Directional Translation', detail: isZh ? '支持 OpenAI、Claude Messages 和 Gemini 接入；原生 Anthropic 渠道保留签名思考块。' : 'Supports OpenAI, Claude Messages, and Gemini clients. Native Anthropic routes preserve signed thinking blocks.' },
    { icon: Layers, title: isZh ? '全模态统一管道' : 'Unified Multimodal Pipe', detail: isZh ? '对话、生图、语音与视频共享统一路由、流控与计费。' : 'Chat, images, speech, and video share routing, admission controls, and metering.' },
    { icon: Server, title: isZh ? '多活热备与故障自愈' : 'Active-Active Resilience', detail: isZh ? '多节点互备、健康探测与断路器隔离，支持故障后的恢复试探。' : 'Multiple nodes, health probes, and circuit breakers isolate failures and test recovery.' },
  ];
}

function modalityCards(isZh) {
  return [
    { icon: MessageSquare, title: isZh ? '文本与深度推理' : 'Text & Reasoning', path: '/v1/chat/completions', detail: isZh ? '接入 GPT-4.1、Claude、Gemini 与 DeepSeek-R1；通过会话亲和优先复用上游缓存。' : 'Connect GPT-4.1, Claude, Gemini, and DeepSeek-R1. Session affinity helps reuse upstream caches.' },
    { icon: Image, title: isZh ? '图像生成与视觉' : 'Image & Vision', path: '/v1/images/generations', detail: isZh ? '转发生图请求并支持对象存储直传，具体格式和能力由上游决定。' : 'Route image requests and support direct object storage uploads. Formats and capabilities depend on the upstream.' },
    { icon: Volume2, title: isZh ? '实时语音与转写' : 'Audio & Speech', path: '/v1/audio/transcriptions', detail: isZh ? '支持语音合成与转写；实际模型、音频格式和速度由提供商决定。' : 'Route speech generation and transcription; models, formats, and latency depend on the provider.' },
    { icon: Video, title: isZh ? '视频生成' : 'Video Generation', path: '/v1/videos/generations', detail: isZh ? '统一异步任务分发、生命周期状态轮询与视频资源获取。' : 'Dispatch asynchronous tasks, poll their status, and retrieve generated video assets.' },
  ];
}

function CardSection({ id, title, cards }) {
  return (
    <section id={id} className={`py-24 px-6 max-w-6xl mx-auto w-full space-y-12 ${id === 'multimodal' ? 'border-t border-light' : ''}`}>
      <h2 className="text-3xl font-bold tracking-tight text-center">{title}</h2>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        {cards.map(({ icon: Icon, title: cardTitle, detail, path }) => (
          <article key={cardTitle} className="ui-card p-6 hover:border-brand/40 transition space-y-3">
            <div className="w-10 h-10 rounded-lg bg-brand-selected text-brand-active flex items-center justify-center"><Icon className="w-5 h-5" /></div>
            <h3 className="font-semibold text-base">{cardTitle}</h3>
            <p className="text-xs text-secondary leading-relaxed">{detail}</p>
            {path && <div className="pt-2 font-mono text-xs text-brand-active font-semibold">
              <span className="block text-secondary text-[10px] mb-1">POST</span><code className="break-all">{path}</code>
            </div>}
          </article>
        ))}
      </div>
    </section>
  );
}

export default function LandingSections({ isZh, children }) {
  return <>
    <CardSection id="features" title={isZh ? '工业级可靠性与核心优势' : 'Industrial-Grade Reliability & Core Advantages'} cards={capabilityCards(isZh)} />
    <CardSection id="multimodal" title={isZh ? '全模态统一调度与极低资源消耗' : 'Unified Multimodal Dispatch & Minimal Overhead'} cards={modalityCards(isZh)} />
    {children}
    <LandingPricing isZh={isZh} />
  </>;
}
