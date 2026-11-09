import { CheckCircle2, X } from 'lucide-react';

export default function DashboardAttention({
  attention, attentionItems, attentionTones, channels, dismissedAttentions, handleDismissAttention, isZh
}) {
  return (
    <section className="space-y-2.5">
      <div className="flex items-center justify-between">
        <h3 className="ui-section-title">{isZh ? '需要关注' : 'Needs attention'}</h3>
      </div>

      {(() => {
        const visibleAttentionItems = attentionItems.filter(item => !dismissedAttentions.includes(item.key));
        if (visibleAttentionItems.length === 0) {
          return (
            <div className="flex items-start gap-3 rounded-xl border border-l-2 border-emerald-200/70 dark:border-emerald-900/60 border-l-emerald-500 bg-emerald-50/50 dark:bg-emerald-950/20 px-4 py-3">
              <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0 mt-0.5" />
              <div>
                <div className="text-xs font-semibold text-emerald-900 dark:text-emerald-100">
                  {isZh ? '运行正常' : 'All clear'}
                </div>
                <p className="ui-meta mt-0.5">
                  {attention.recentTotal > 0
                    ? (isZh
                      ? `${channels.length} 个渠道的断路器均未触发，最近 ${attention.recentTotal} 条日志中没有失败请求。`
                      : `No breaker tripped across ${channels.length} channel(s), and no failed request among the last ${attention.recentTotal} logged calls.`)
                    : (isZh
                      ? `已配置 ${channels.length} 个渠道且断路器均未触发；暂无请求日志可判断成功率。`
                      : `${channels.length} channel(s) configured with no tripped breaker; no request logs yet to judge success rate.`)}
                </p>
                <p className="ui-meta mt-1">
                  {isZh ? '数据为最近一次探测结果。' : 'Based on the most recent probe.'}
                </p>
              </div>
            </div>
          );
        }

        return (
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3">
            {visibleAttentionItems.map(item => {
              const tone = attentionTones[item.tone];
              const ItemIcon = item.icon;
              return (
                <div key={item.key} className={`rounded-xl border border-l-2 px-3.5 py-3 flex flex-col relative group ${tone.card}`}>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDismissAttention(item.key);
                    }}
                    className="absolute top-2 right-2 p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-md transition opacity-70 group-hover:opacity-100 cursor-pointer"
                    title={isZh ? '忽略此项关注' : 'Dismiss notice'}
                  >
                    <X className="w-3.5 h-3.5" />
                  </button>
                  <div className="flex items-start gap-2 pr-5">
                    <ItemIcon className={`w-4 h-4 shrink-0 mt-0.5 ${tone.icon}`} />
                    <span className={`text-xs font-semibold leading-5 ${tone.title}`}>{item.title}</span>
                  </div>
                  <p className="ui-meta mt-1 flex-1">{item.detail}</p>
                  {item.actionLabel && (
                    <button
                      onClick={() => {
                        handleDismissAttention(item.key);
                        item.onAction?.();
                      }}
                      className="ui-btn-ghost ui-btn-sm -ml-2.5 mt-1 self-start text-indigo-600 dark:text-indigo-400 hover:text-indigo-700 dark:hover:text-indigo-300"
                    >
                      {item.actionLabel} →
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        );
      })()}
    </section>
  );
}
