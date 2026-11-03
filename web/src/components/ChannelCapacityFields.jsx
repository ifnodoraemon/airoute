export default function ChannelCapacityFields({ channel, onChange, isZh }) {
  const fields = [
    ['max_concurrency', isZh ? '每副本最大在途请求' : 'Max in-flight per replica', 10000],
    ['queue_limit', isZh ? '最多排队请求' : 'Max queued requests', 10000],
    ['queue_wait_seconds', isZh ? '最长排队时间（秒）' : 'Max queue wait (s)', 300],
  ];
  return (
    <div className="space-y-2">
      <div className="grid grid-cols-3 gap-3">
        {fields.map(([key, label, maximum]) => (
          <label key={key} className="ui-label-sm">
            {label}
            <input
              type="number"
              min="0"
              max={maximum}
              value={channel[key] ?? 0}
              onChange={event => onChange(previous => ({ ...previous, [key]: Number(event.target.value) }))}
              className="ui-field ui-field-sm font-mono mt-1"
            />
          </label>
        ))}
      </div>
      <p className="ui-meta-sm">
        {isZh
          ? '最大在途设为 0 表示不限；排队数为 0 时满载立即尝试备用渠道；排队时间为 0 使用 5 秒。同一上游地址和凭据共用限制，多副本分别计数。'
          : '0 in-flight means unlimited; 0 queued fails over immediately; 0 wait uses 5 seconds. Channels sharing an endpoint and credential share the limit. Each replica counts separately.'}
      </p>
    </div>
  );
}
