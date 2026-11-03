export default function ThinkingStatus({ observation, reasoning = '', complete = true, lang = 'zh' }) {
  if (!complete || !observation) return null;
  const isZh = lang === 'zh';
  const tokens = observation.reasoning_tokens;
  const hasCount = Number.isFinite(tokens);
  const hasSummary = Boolean(reasoning || observation.has_summary);
  let text;
  if (hasCount && tokens > 0) text = isZh ? `上游报告 ${tokens} 个思考 token${hasSummary ? '' : '，未返回可展示的思考内容'}` : `${tokens} reported reasoning tokens${hasSummary ? '' : '; no visible thinking content returned'}`;
  else if (hasCount) text = isZh ? '上游报告本次思考 token 为 0' : 'Upstream reported 0 reasoning tokens';
  else text = hasSummary ? (isZh ? '已返回思考内容 / 摘要；上游未单独报告思考用量' : 'Thinking content / summary returned; no separate token count') : (isZh ? '上游未报告思考用量；没有摘要不代表没有思考' : 'No reasoning token count reported; absent summary does not establish absence of thinking');
  const modeLabel=isZh ? ({auto:'路由 / 上游默认',on:'开启',off:'关闭',native:'原生参数'}[observation.mode] || observation.mode) : observation.mode;
  const sourceLabel=isZh ? ({configured:'渠道配置',catalog:'官方资料推断',unknown:'能力未确认',mixed:'多个来源'}[observation.source] || observation.source) : observation.source;
  const truncated = ['length', 'MAX_TOKENS', 'max_tokens'].includes(observation.finish_reason);
  return <div className="mt-2 space-y-1 text-[11px] text-slate-500" aria-label={isZh ? '思考状态' : 'Thinking status'}>
    <p>{text}</p>
    {observation.mode && <p>{isZh ? '生效设置：' : 'Applied: '}{modeLabel}{observation.effort ? ` · ${observation.effort}` : ''}{observation.source ? ` · ${sourceLabel}` : ''}</p>}
    {observation.mode === 'off' && ((hasCount && tokens > 0) || hasSummary) && <p className="text-amber-700 dark:text-amber-300">{isZh ? '已请求关闭，但上游仍返回思考信息，请检查渠道能力配置。' : 'Thinking was disabled but the upstream returned thinking evidence. Check channel configuration.'}</p>}
    {truncated && <p className="text-amber-700 dark:text-amber-300">{isZh ? '达到输出上限；思考可能占用答案预算，可提高输出上限或降低思考强度。' : 'Output limit reached. Thinking may consume the answer budget; increase the limit or lower effort.'}</p>}
  </div>;
}
