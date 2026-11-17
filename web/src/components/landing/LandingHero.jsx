import { ArrowRight, Code, Server } from 'lucide-react';

export default function LandingHero({ isZh, isLoggedIn, onEnterConsole, onOpenLogin, onViewStatus, onViewDocs, scrollToSection }) {
  return (
    <section className="relative pt-20 pb-24 px-6 overflow-hidden bg-page border-b border-light">
      <div className="max-w-4xl mx-auto text-center space-y-8">
        <button type="button" onClick={onViewStatus} className="ui-btn-secondary rounded-full" title={isZh ? '查看状态页的实时检测结果' : 'View live checks on the status page'}>
          <span className="w-2 h-2 rounded-full bg-brand" />
          {isZh ? '统一网关 · 多提供商与降级路由 · 查看状态详情 →' : 'Unified Gateway · Multi-Provider Routing · View status →'}
        </button>
        <h1 className="text-4xl sm:text-5xl lg:text-6xl font-extrabold tracking-tight leading-tight">
          {isZh ? '大模型与多模态' : 'LLM & Multimodal'}<br />
          <span className="text-brand-active">{isZh ? '统一调度与高性能接入网关' : 'Unified AI Router & Gateway'}</span>
        </h1>
        <p className="text-base sm:text-lg text-secondary max-w-2xl mx-auto leading-relaxed">
          {isZh ? '多协议接入 · 流式转发 · 首字前故障回退 · 用量与成本审计' : 'Multi-protocol access · Streaming relay · Pre-content failover · Usage and cost audit'}<br />
          {isZh ? '统一收口 OpenAI、Claude、Gemini、DeepSeek 与全模态生成管线。' : 'Connect OpenAI, Claude, Gemini, DeepSeek, and multimodal pipelines through one gateway.'}
        </p>
        <div className="flex flex-col sm:flex-row items-center justify-center gap-4 pt-2">
          <button type="button" onClick={isLoggedIn ? onEnterConsole : onOpenLogin} className="ui-btn-primary w-full sm:w-auto px-6 py-2.5 text-sm">
            <Server className="w-4 h-4" />{isLoggedIn ? (isZh ? '进入工作台' : 'Enter Console') : (isZh ? '进入企业控制台' : 'Enter Admin Console')}<ArrowRight className="w-4 h-4" />
          </button>
          <button type="button" onClick={() => onViewDocs ? onViewDocs() : scrollToSection('docs')} className="ui-btn-secondary w-full sm:w-auto px-6 py-2.5 text-sm">
            <Code className="w-4 h-4 text-brand-active" />{isZh ? '查阅接入指南' : 'View Documentation'}
          </button>
        </div>
      </div>
    </section>
  );
}
