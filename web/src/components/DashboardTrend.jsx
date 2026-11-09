
export default function DashboardTrend({
  chartMetric, chartSvgData, errorLogs, fmtPct, hoveredBucket, isZh, maxMetricVal, percentiles,
  setChartMetric, setHoveredBucket, setTimeframe, slaAvailability, timeBuckets, timeframe, totalErrorRequests,
  totalRequests
}) {
  return (
    <div className="ui-card p-6 space-y-6">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h4 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4] tracking-tight">
            {isZh ? '多维时序走势与分位监测' : 'Time-Series Telemetry & SLA Waveform'}
          </h4>
          <p className="ui-meta mt-0.5">
            {isZh ? '吞吐与首字时延趋势' : 'Throughput and TTFT trends'}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* Metric Selector Tabs */}
          <div className="flex items-center bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg text-xs font-medium">
            <button
              onClick={() => setChartMetric('requests')}
              className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'requests' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
            >
              {isZh ? '请求量 (QPS)' : 'Requests'}
            </button>
            <button
              onClick={() => setChartMetric('tokens')}
              className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'tokens' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
            >
              {isZh ? 'Token 吞吐' : 'Tokens'}
            </button>
            <button
              onClick={() => setChartMetric('rpm')}
              className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'rpm' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
            >
              {isZh ? '实时 RPM' : 'RPM'}
            </button>
            <button
              onClick={() => setChartMetric('tpm')}
              className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'tpm' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
            >
              {isZh ? '实时 TPM' : 'TPM'}
            </button>
            <button
              onClick={() => setChartMetric('latency')}
              className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'latency' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
            >
              {isZh ? '首字延迟 TTFT' : 'TTFT Latency'}
            </button>
            <button
              onClick={() => setChartMetric('errors')}
              className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'errors' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
            >
              {isZh ? '异常错误' : 'Errors'}
            </button>
          </div>

          {/* Timeframe Selector */}
          <div className="flex items-center bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg text-xs font-medium">
            <button
              onClick={() => setTimeframe('1h')}
              className={`px-2.5 py-1 rounded-md transition-colors ${timeframe === '1h' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}
            >
              1h
            </button>
            <button
              onClick={() => setTimeframe('24h')}
              className={`px-2.5 py-1 rounded-md transition-colors ${timeframe === '24h' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}
            >
              24h
            </button>
            <button
              onClick={() => setTimeframe('7d')}
              className={`px-2.5 py-1 rounded-md transition-colors ${timeframe === '7d' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}
            >
              7d
            </button>
          </div>
        </div>
      </div>

      {/* Clean SVG Trend Spline & Column Histogram (No nested gray box) */}
      <div className="relative h-60 w-full flex flex-col justify-between overflow-hidden pt-2">
        {/* Subtle Grid Lines */}
        <div className="absolute inset-0 pointer-events-none flex flex-col justify-between pb-8">
          <div className="w-full border-b border-[#E4E6E9]/60 dark:border-[#2C3038]/60"></div>
          <div className="w-full border-b border-[#E4E6E9]/60 dark:border-[#2C3038]/60"></div>
          <div className="w-full border-b border-[#E4E6E9]/60 dark:border-[#2C3038]/60"></div>
          <div className="w-full border-b border-[#E4E6E9]/60 dark:border-[#2C3038]/60"></div>
        </div>

        {/* Continuous SVG Trend Area Wave Overlay */}
        <div className="absolute inset-0 px-2 pt-4 pb-8 pointer-events-none">
          <svg viewBox="0 0 1000 140" preserveAspectRatio="none" className="w-full h-full overflow-visible">
            <defs>
              <linearGradient id="telemetryGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                <stop offset="0%" stopColor="#355CFF" stopOpacity="0.16" />
                <stop offset="100%" stopColor="#355CFF" stopOpacity="0.0" />
              </linearGradient>
            </defs>
            {chartSvgData.areaD && (
              <path d={chartSvgData.areaD} fill="url(#telemetryGradient)" />
            )}
            {chartSvgData.pointsStr && (
              <polyline
                points={chartSvgData.pointsStr}
                fill="none"
                stroke="#355CFF"
                strokeWidth="2.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            )}
            {chartSvgData.pointsArr.map((pt, i) => {
              const isHovered = hoveredBucket?.timestamp != null
                ? hoveredBucket.timestamp === pt.bucket.timestamp
                : hoveredBucket?.label === pt.bucket.label;
              return (
                <circle
                  key={i}
                  cx={pt.x}
                  cy={pt.y}
                  r={isHovered ? "4.5" : "2"}
                  fill={isHovered ? "#ffffff" : "#355CFF"}
                  stroke="#355CFF"
                  strokeWidth="2"
                  className="transition-all duration-150"
                />
              );
            })}
          </svg>
        </div>

        {/* Hover Tooltip Overlay */}
        {hoveredBucket && (
          <div className="absolute top-2 right-2 bg-[#191B1F] text-[#F1F2F4] text-xs px-3 py-1.5 rounded-lg border border-[#2C3038] shadow-lg pointer-events-none z-10 flex items-center gap-3">
            <span className="font-mono text-[#A1A7B0]">{hoveredBucket.label}</span>
            <span className="font-semibold text-[#91A7FF] font-mono">
              {isZh ? '请求' : 'Reqs'}: {hoveredBucket.requests} ({((hoveredBucket.rpm ?? 0) / 60).toFixed(1)} req/s · {hoveredBucket.rpm ?? 0} RPM)
            </span>
            <span className="font-semibold text-sky-400 font-mono">
              Tokens: {hoveredBucket.tokens.toLocaleString()} ({hoveredBucket.tpm ?? 0} TPM)
            </span>
            <span className="font-semibold text-amber-400 font-mono">
              TTFT: {hoveredBucket.avgTtft > 0 ? `${hoveredBucket.avgTtft}ms` : (isZh ? '无样本' : 'n/a')}
            </span>
            {hoveredBucket.errors > 0 && (
              <span className="font-semibold text-rose-400 font-mono">
                {isZh ? '错误' : 'Errors'}: {hoveredBucket.errors}
              </span>
            )}
          </div>
        )}

        {/* Bar / Column Chart Grid */}
        <div className="h-44 flex items-end justify-between gap-1.5 sm:gap-2 pt-4 px-1 z-1">
          {timeBuckets.map((bucket, idx) => {
            const val = chartMetric === 'requests'
              ? bucket.requests
              : chartMetric === 'tokens'
              ? bucket.tokens
              : chartMetric === 'rpm'
              ? (bucket.rpm || 0)
              : chartMetric === 'tpm'
              ? (bucket.tpm || 0)
              : chartMetric === 'latency'
              ? bucket.avgTtft
              : bucket.errors;

            const heightPct = Math.max(8, Math.min(100, Math.round((val / maxMetricVal) * 100)));

            return (
              <div
                key={idx}
                onMouseEnter={() => setHoveredBucket(bucket)}
                onMouseLeave={() => setHoveredBucket(null)}
                className="flex-1 h-full flex flex-col justify-end items-center group cursor-pointer"
              >
                <div
                  style={{ height: `${heightPct}%` }}
                  className="w-full max-w-[18px] rounded-t-sm transition-all duration-150 bg-[#E4E6E9] dark:bg-[#2C3038] group-hover:bg-[#355CFF]"
                />
              </div>
            );
          })}
        </div>

        {/* X-Axis Timeline Labels */}
        <div className="flex justify-between ui-meta font-mono pt-3 border-t border-[#E4E6E9] dark:border-[#2C3038] z-1">
          <span>{timeBuckets[0]?.label}</span>
          <span>{timeBuckets[Math.floor(timeBuckets.length / 2)]?.label}</span>
          <span>{isZh ? '当前 (实时)' : 'Live Now'}</span>
        </div>
      </div>

      {/* Latency Percentiles & SLA Quick Stat Bar */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 pt-1">
        <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
          <span className="ui-meta font-medium block">{isZh ? 'P50 中位时延' : 'P50 Median TTFT'}</span>
          <div className="flex items-baseline gap-1.5 mt-1">
            <span className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums">
              {fmtPct(percentiles.p50)}
            </span>
            <span className="ui-meta font-normal">(&lt; 200ms)</span>
          </div>
        </div>
        <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
          <span className="ui-meta font-medium block">{isZh ? 'P90 企业基准' : 'P90 Baseline'}</span>
          <div className="flex items-baseline gap-1.5 mt-1">
            <span className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums">
              {fmtPct(percentiles.p90)}
            </span>
            <span className="ui-meta font-normal">(&lt; 500ms)</span>
          </div>
        </div>
        <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
          <span className="ui-meta font-medium block">{isZh ? 'P99 极端长尾' : 'P99 Tail TTFT'}</span>
          <div className="flex items-baseline gap-1.5 mt-1">
            <span className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums">
              {fmtPct(percentiles.p99)}
            </span>
            <span className="ui-meta font-normal">(&lt; 1000ms)</span>
          </div>
        </div>
        <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
          <span className="ui-meta font-medium block">{isZh ? '服务可用率 (SLA)' : 'SLA Availability'}</span>
          <div className="flex items-baseline gap-1.5 mt-1">
            <span className="text-lg font-semibold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums">
              {slaAvailability}{slaAvailability === '—' ? '' : '%'}
            </span>
            <span className="ui-meta font-normal">{(totalRequests > 0 ? totalErrorRequests : errorLogs.length)} {isZh ? '次异常' : 'errors'}</span>
          </div>
        </div>
      </div>
    </div>
  );
}
