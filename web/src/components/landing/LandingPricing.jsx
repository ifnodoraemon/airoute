const rows = [
  ['GPT-4.1 / Gemini 2.5 Pro', 'tokens'],
  ['Claude Sonnet / DeepSeek-R1', 'tokens'],
  ['GPT Image / FLUX', 'images'],
  ['Whisper / TTS', 'audio'],
  ['CogVideoX', 'video'],
];

export default function LandingPricing({ isZh }) {
  const units = isZh ? { tokens: '每百万 Token', images: '按张 / Token', audio: '按音频时长 / Token', video: '按任务 / 时长' }
    : { tokens: 'Per million tokens', images: 'Per image / token', audio: 'Per duration / token', video: 'Per task / duration' };
  return (
    <section id="pricing" className="py-24 px-6 max-w-6xl mx-auto w-full space-y-10 border-t border-light">
      <div className="text-center max-w-2xl mx-auto space-y-3">
        <h2 className="text-3xl font-bold tracking-tight">{isZh ? '模型矩阵与透明计费' : 'Model Matrix & Transparent Pricing'}</h2>
        <p className="text-sm text-secondary">{isZh ? '实际费率、缓存优惠和分时折扣以本部署控制台的模型定价为准。' : 'Rates, cache discounts, and off-peak schedules follow this deployment’s console pricing.'}</p>
      </div>
      <div className="ui-card overflow-x-auto">
        <table className="w-full text-left text-xs">
          <thead><tr className="bg-subtle">
            <th className="ui-th">{isZh ? '模型系列示例' : 'Example model families'}</th>
            <th className="ui-th">{isZh ? '计费单位' : 'Billing unit'}</th>
            <th className="ui-th">{isZh ? '输入 / 输出费率' : 'Input / output rates'}</th>
            <th className="ui-th">{isZh ? '缓存与闲时优惠' : 'Cache & off-peak pricing'}</th>
          </tr></thead>
          <tbody>{rows.map(([name, unit]) => <tr key={name} className="ui-tr-hover">
            <td className="ui-td font-semibold">{name}</td>
            <td className="ui-td">{units[unit]}</td>
            <td className="ui-td">{isZh ? '按模型定价配置' : 'Configured per model'}</td>
            <td className="ui-td text-brand-active">{isZh ? '按渠道能力与计费策略' : 'Per channel and billing policy'}</td>
          </tr>)}</tbody>
        </table>
      </div>
    </section>
  );
}
