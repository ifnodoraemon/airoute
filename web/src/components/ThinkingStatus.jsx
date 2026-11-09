import { thinkingOptionLabel } from '../lib/thinking';

export default function ThinkingStatus({
  observation,
  reasoning = '',
  complete = true,
  lang = 'zh',
}) {
  if (!complete || !observation) return null;
  const isZh = lang === 'zh';
  const tokens = observation.reasoning_tokens;
  const hasCount = Number.isFinite(tokens);
  const hasSummary = Boolean(reasoning || observation.has_summary);
  const truncated = ['length', 'MAX_TOKENS', 'max_tokens'].includes(observation.finish_reason);
  if ((!observation.mode || observation.mode === 'auto') && !hasCount && !hasSummary && !truncated)
    return null;
  const text = hasCount
    ? `${isZh ? '思考' : 'Thinking'} ${tokens} token`
    : hasSummary
      ? isZh
        ? '已返回思考内容'
        : 'Thinking returned'
      : isZh
        ? '思考用量未返回'
        : 'Reasoning usage unavailable';
  const setting = observation.effort || observation.mode;
  return (
    <div
      className="mt-3 space-y-2 text-[11px] text-slate-500"
      aria-label={isZh ? '思考状态' : 'Thinking status'}
    >
      <p
        title={
          isZh
            ? (observation.mode === 'native' ? '客户端直接传入上游原生思考参数；具体设置由上游解释。' : '未返回思考内容或用量，不代表没有思考。')
            : (observation.mode === 'native' ? 'The client supplied provider-native thinking parameters, interpreted by the upstream.' : 'Missing reasoning text or usage does not establish an absence of thinking.')
        }
      >
        {text}
        {setting && setting !== 'auto' && ` · ${thinkingOptionLabel(setting, isZh)}`}
      </p>
      {observation.mode === 'off' && ((hasCount && tokens > 0) || hasSummary) && (
        <p className="text-amber-700 dark:text-amber-300">
          {isZh
            ? '已请求关闭，上游仍返回思考。'
            : 'Thinking was disabled, but the upstream still returned it.'}
        </p>
      )}
      {truncated && (
        <p className="text-amber-700 dark:text-amber-300">
          {isZh ? '已达输出上限' : 'Output limit reached'}
        </p>
      )}
    </div>
  );
}
