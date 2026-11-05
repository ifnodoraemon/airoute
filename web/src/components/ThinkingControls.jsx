import {
  thinkingChoices,
  thinkingControl,
  minimumThinkingOutput,
  thinkingSelection,
  thinkingFromSelection,
  thinkingOptions,
  thinkingOptionLabel,
} from '../lib/thinking';

export default function ThinkingControls({
  capability,
  value,
  onChange,
  tools = false,
  disabled = false,
  maxTokens,
  lang = 'zh',
}) {
  const isZh = lang === 'zh';
  const choices = thinkingChoices(capability, tools);
  const active = thinkingControl(capability, value);
  const selected = thinkingSelection(active);
  const options = thinkingOptions(choices, active);
  const unavailable = !options.includes(selected);
  const minimum = minimumThinkingOutput(capability, active);
  return (
    <div className="space-y-3 text-xs" data-testid="thinking-controls">
      <label className="block">
        {isZh ? '思考' : 'Thinking'}
        <select
          aria-label={isZh ? '思考' : 'Thinking'}
          className="ui-field mt-2"
          value={selected}
          disabled={disabled}
          title={!capability?.known ? (isZh ? '能力待确认' : 'Capability unconfirmed') : undefined}
          onChange={(event) =>
            onChange({
              ...active,
              ...thinkingFromSelection(event.target.value),
            })
          }
        >
          {unavailable && (
            <option value={selected} disabled>
              {thinkingOptionLabel(selected, isZh)} · {isZh ? '不可用' : 'Unavailable'}
            </option>
          )}
          {options.map((option) => (
            <option key={option} value={option}>
              {thinkingOptionLabel(option, isZh)}
            </option>
          ))}
        </select>
      </label>
      {unavailable && (
        <p role="alert" className="text-amber-700 dark:text-amber-300">
          {isZh
            ? '原设置已不可用，请重新选择。'
            : 'The previous setting is unavailable. Choose another.'}
        </p>
      )}
      {Number(maxTokens) > 0 && minimum > Number(maxTokens) && (
        <p role="alert" className="text-amber-700 dark:text-amber-300">
          {isZh
            ? `输出上限至少需要 ${minimum} token。`
            : `The output limit must be at least ${minimum} tokens.`}
        </p>
      )}
    </div>
  );
}
