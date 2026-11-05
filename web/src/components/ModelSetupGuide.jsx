export default function ModelSetupGuide({ lang = 'zh' }) {
  const isZh = lang === 'zh';
  const steps = isZh
    ? [
        [
          '渠道模型：我们买入',
          '登记实际上游模型、思考能力和采购报价。同一个渠道模型可以被多条路由共用。',
        ],
        [
          '模型路由：我们对外提供',
          '把公开模型名称绑定到渠道模型，设置对外售价、默认思考方式和允许范围。',
        ],
        [
          '本次调用：用户怎么用',
          '用户可在允许范围内临时选择。系统只调用满足要求的渠道，切换渠道也保留要求。',
        ],
      ]
    : [
        [
          'Channel model: what we buy',
          'Actual upstream model, thinking capability and purchase quote. Shared across routes.',
        ],
        [
          'Public route: what we offer',
          'Public model name, upstream bindings, customer price, thinking defaults and restrictions.',
        ],
        [
          'Request: what the user chooses',
          'One-call preferences within the allowed range. Failover preserves the requirements.',
        ],
      ];
  return (
    <section
      aria-label={isZh ? '渠道、路由与调用的关系' : 'Channel, route and request relationship'}
      className="grid gap-3 md:grid-cols-3"
    >
      {steps.map(([title, description], index) => (
        <div
          key={title}
          className="rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-[#191B1F] p-4"
        >
          <div className="flex items-center gap-2 text-xs font-semibold">
            <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-indigo-50 text-indigo-600 dark:bg-indigo-950 dark:text-indigo-300">
              {index + 1}
            </span>
            {title}
          </div>
          <p className="mt-2 text-xs leading-relaxed text-slate-500">{description}</p>
        </div>
      ))}
    </section>
  );
}
