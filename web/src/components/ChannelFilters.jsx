import { Search } from 'lucide-react';
import { ColumnToggle } from './TableControls';

export default function ChannelFilters({
  controls, enablementCounts, engineFilter, hasActiveFilters, healthFilter, healthOptions, healthTotal, isZh,
  lang, resetFilters, searchQuery, setFilters, statusChipClass
}) {
  return (
    <div className="flex flex-col gap-2.5 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative flex-1 min-w-[180px] max-w-sm">
          <Search className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] absolute left-3 top-2.5" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setFilters({ q: e.target.value })}
            placeholder={isZh ? '搜索服务商名称 / 引擎 / 地址 / 模型...' : 'Search provider name, engine, URL, models...'}
            className="ui-field pl-8"
          />
        </div>

        {/* Engine Type Filter */}
        <select
          value={engineFilter}
          onChange={(e) => setFilters({ engine: e.target.value })}
          className="ui-field w-auto cursor-pointer font-medium"
          aria-label={isZh ? '按引擎筛选' : 'Filter by engine'}
        >
          <option value="all">{isZh ? '全部引擎' : 'All Engines'}</option>
          <option value="gpustack">GPUStack</option>
          <option value="openai">OpenAI</option>
          <option value="anthropic">Anthropic Claude</option>
          <option value="gemini">Google Gemini</option>
          <option value="sub2api">Sub2API</option>
        </select>

        {/* Enablement filter: only on/off, says nothing about health. */}
        <div className="inline-flex items-center bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038]">
          <span className="hidden sm:inline px-1.5 text-xs text-[#656B75] dark:text-[#A1A7B0]">
            {isZh ? '启用' : 'Enabled'}
          </span>
          <button type="button" onClick={() => setFilters({ status: 'all' })} className={statusChipClass('all')}>
            {isZh ? '全部' : 'All'}
          </button>
          <button type="button" onClick={() => setFilters({ status: 'active' })} className={statusChipClass('active')}>
            {isZh ? `已启用 (${enablementCounts.enabled})` : `Enabled (${enablementCounts.enabled})`}
          </button>
          <button type="button" onClick={() => setFilters({ status: 'disabled' })} className={statusChipClass('disabled')}>
            {isZh ? `已停用 (${enablementCounts.disabled})` : `Disabled (${enablementCounts.disabled})`}
          </button>
        </div>

        {/* Health filter: breaker state + last probe result, counted honestly. */}
        <label className="inline-flex items-center gap-1.5">
          <span className="text-xs text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '健康' : 'Health'}</span>
          <select
            value={healthFilter}
            onChange={(e) => setFilters({ health: e.target.value })}
            className={`ui-field w-auto cursor-pointer font-medium ${
              healthFilter === 'tripped' || healthFilter === 'failed'
                ? 'border-rose-300 dark:border-rose-900/60 text-rose-700 dark:text-rose-300'
                : healthFilter === 'half_open'
                ? 'border-amber-300 dark:border-amber-900/60 text-amber-700 dark:text-amber-300'
                : ''
            }`}
          >
            <option value="all">
              {isZh ? `全部健康状态 (${healthTotal})` : `All health states (${healthTotal})`}
            </option>
            {healthOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {`${option.label} (${option.count})`}
              </option>
            ))}
          </select>
        </label>

        <div className="flex items-center gap-2 sm:ml-auto">
          {hasActiveFilters && (
            <button type="button" onClick={resetFilters} className="ui-btn-ghost ui-btn-sm">
              {isZh ? '清除筛选' : 'Clear filters'}
            </button>
          )}
          <ColumnToggle controls={controls} lang={lang} />
        </div>
      </div>
    </div>
  );
}
