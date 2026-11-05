export default function ModelSetupGuide({ lang = 'zh' }) {
  const isZh = lang === 'zh';
  const rows = isZh
    ? [
        ['渠道模型', '思考能力、采购价'],
        ['模型路由', '上游绑定、默认思考、对外售价'],
        ['本次调用', '选择思考档位'],
      ]
    : [
        ['Channel model', 'Capabilities and purchase prices'],
        ['Model route', 'Bindings, defaults and customer prices'],
        ['Request', 'Thinking selection'],
      ];
  return (
    <details
      className="text-xs text-slate-500 py-2"
      aria-label={isZh ? '渠道、路由与调用的关系' : 'Channel, route and request relationship'}
    >
      <summary className="cursor-pointer w-fit">{isZh ? '配置说明' : 'Setup guide'}</summary>
      <dl className="grid gap-y-3 gap-x-6 sm:grid-cols-[auto_1fr] pt-4 pb-2">
        {rows.map(([title, description]) => (
          <div key={title} className="contents">
            <dt className="font-medium">{title}</dt>
            <dd>{description}</dd>
          </div>
        ))}
      </dl>
    </details>
  );
}
