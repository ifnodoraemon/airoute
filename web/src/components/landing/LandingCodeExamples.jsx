import { Check, Code, Copy } from 'lucide-react';
import { getSnippetTabs } from './landingSnippets';

export default function LandingCodeExamples({ isZh, snippets, activeTab, setActiveTab, copiedSnippet, copyCode }) {
  return (
    <section id="docs" className="py-24 px-6 max-w-6xl mx-auto w-full space-y-10 border-t border-light">
      <div className="text-center max-w-2xl mx-auto space-y-3">
        <div className="ui-chip text-brand-active"><Code className="w-3.5 h-3.5" />Developer Guide</div>
        <h2 className="text-3xl font-bold tracking-tight">{isZh ? '极简接入与快速开始' : 'Quick Integration & Developer Guide'}</h2>
        <p className="text-sm text-secondary">{isZh ? '配置 base_url 与密钥，并选择部署中已接入的模型。' : 'Set your base_url and API key, then choose a model configured in your deployment.'}</p>
      </div>
      <div className="ui-card overflow-hidden">
        <div className="flex flex-wrap items-center justify-between border-b border-light bg-subtle px-4 py-2 gap-2">
          <div role="tablist" aria-label={isZh ? '代码示例' : 'Code examples'} className="flex flex-wrap gap-1.5">
            {getSnippetTabs(isZh).map(tab => (
              <button key={tab.id} id={`snippet-tab-${tab.id}`} type="button" role="tab" aria-selected={activeTab === tab.id} aria-controls="snippet-panel"
                onClick={() => setActiveTab(tab.id)} className={`ui-btn ui-btn-sm ${activeTab === tab.id ? 'bg-surface text-brand-active shadow-xs' : 'text-secondary hover:text-primary'}`}>
                {tab.label}
              </button>
            ))}
          </div>
          <button type="button" onClick={() => copyCode(snippets[activeTab], activeTab)} className="ui-btn-secondary ui-btn-sm" aria-live="polite">
            {copiedSnippet === activeTab ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
            {copiedSnippet === activeTab ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制代码' : 'Copy Code')}
          </button>
        </div>
        <div id="snippet-panel" role="tabpanel" aria-labelledby={`snippet-tab-${activeTab}`} className="p-6 bg-code-bg text-code-text font-mono text-xs overflow-x-auto leading-relaxed selection:bg-brand">
          <pre><code>{snippets[activeTab]}</code></pre>
        </div>
      </div>
    </section>
  );
}
