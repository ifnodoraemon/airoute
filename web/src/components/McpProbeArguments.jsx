
export default function McpProbeArguments({
  handleSelectTool, isZh, probeToolArgs, setProbeToolArgs
}) {
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <label className="text-xs font-medium text-gray-700 dark:text-gray-300">
          {isZh ? '工具输入参数 (JSON Arguments)' : 'Tool Input Arguments (JSON Arguments)'}
        </label>
        <div className="flex items-center gap-1.5 flex-wrap">
          <button
            type="button"
            onClick={() => handleSelectTool('airoute_data_redact')}
            className="text-xs text-[#355CFF] dark:text-[#91A7FF] hover:underline"
          >
            {isZh ? '敏感脱敏' : 'Redaction'}
          </button>
          <span className="text-gray-300 dark:text-gray-600">|</span>
          <button
            type="button"
            onClick={() => handleSelectTool('airoute_sql_security_check')}
            className="text-xs text-rose-600 dark:text-rose-400 hover:underline"
          >
            {isZh ? 'SQL拦截' : 'SQL Guard'}
          </button>
          <span className="text-gray-300 dark:text-gray-600">|</span>
          <button
            type="button"
            onClick={() => handleSelectTool('puppeteer_navigate')}
            className="text-xs text-emerald-600 dark:text-emerald-400 hover:underline"
          >
            {isZh ? 'Puppeteer导航' : 'Puppeteer'}
          </button>
          <span className="text-gray-300 dark:text-gray-600">|</span>
          <button
            type="button"
            onClick={() => handleSelectTool('read_query')}
            className="text-xs text-amber-600 dark:text-amber-400 hover:underline"
          >
            {isZh ? '只读SQL' : 'Read-only SQL'}
          </button>
          <span className="text-gray-300 dark:text-gray-600">|</span>
          <button
            type="button"
            onClick={() => handleSelectTool('sequentialthinking')}
            className="text-xs text-sky-600 dark:text-sky-400 hover:underline"
          >
            {isZh ? '深度推理' : 'Thinking'}
          </button>
        </div>
      </div>
      <textarea
        rows="3"
        value={probeToolArgs}
        onChange={(e) => setProbeToolArgs(e.target.value)}
        className="w-full px-3 py-2 font-mono text-xs rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] bg-[#111214] text-gray-100"
      />
    </div>
  );
}
