
export default function PlaygroundHyperparameters({
  isZh, maxTokens, playStream, setMaxTokens, setPlayStream, setTemperature, temperature
}) {
  return (
    <div className="space-y-3 pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
      <div>
        <div className="flex justify-between text-xs font-semibold text-[#656B75] dark:text-[#A1A7B0] mb-1">
          <span>{isZh ? '温度' : 'Temperature'}</span>
          <span className="font-mono text-[#355CFF] dark:text-[#91A7FF]">{temperature}</span>
        </div>
        <input
          type="range"
          min="0"
          max="2"
          step="0.1"
          value={temperature}
          onChange={(e) => setTemperature(parseFloat(e.target.value))}
          className="w-full accent-[#355CFF] cursor-pointer"
        />
      </div>

      <div>
        <div className="flex items-center justify-between text-xs font-semibold text-[#656B75] dark:text-[#A1A7B0] mb-1.5">
          <span>{isZh ? '输出上限' : 'Max Tokens'}</span>
          <div className="flex items-center space-x-1.5">
            <input
              type="number"
              min="1"
              max="1048576"
              step="256"
              placeholder={isZh ? '自动' : 'Auto'}
              value={maxTokens}
              onChange={(e) => {
                const val = e.target.value;
                setMaxTokens(val === '' ? '' : Math.max(1, parseInt(val) || ''));
              }}
              className="ui-field ui-field-sm w-20 text-right font-mono font-bold text-[#355CFF] dark:text-[#91A7FF]"
              title={isZh ? '输入任意正整数，留空表示不限制（自动按模型上限）' : 'Enter arbitrary number or leave empty for auto'}
            />
            <span className="ui-meta font-mono">
              {maxTokens ? '' : (isZh ? '(自动)' : '(Auto)')}
            </span>
          </div>
        </div>

        {/* Preset Quick Selection Buttons */}
        <div className="flex flex-wrap gap-1 mb-2">
          {[
            { label: isZh ? '自动' : 'Auto', val: '' },
            { label: '2K', val: 2048 },
            { label: '4K', val: 4096 },
            { label: '8K', val: 8192 },
            { label: '16K', val: 16384 },
            { label: '32K', val: 32768 },
            { label: '64K', val: 65536 },
            { label: '128K', val: 131072 }
          ].map((p, i) => {
            const isActive = (p.val === '' && (maxTokens === '' || maxTokens === 0)) || (p.val !== '' && Number(maxTokens) === p.val);
            return (
              <button
                key={i}
                type="button"
                onClick={() => setMaxTokens(p.val)}
                className={`px-2 py-0.5 text-[10px] font-mono rounded-md border transition cursor-pointer ${
                  isActive
                    ? 'bg-[#355CFF] text-white border-[#355CFF] font-bold shadow-xs'
                    : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border-[#E4E6E9] dark:border-[#2C3038] hover:bg-[#E4E6E9] dark:hover:bg-[#2C3038]'
                }`}
              >
                {p.label}
              </button>
            );
          })}
        </div>

        {/* Dynamic slider up to 131072 */}
        <input
          type="range"
          min="256"
          max="131072"
          step="256"
          value={maxTokens ? Math.min(131072, Math.max(256, Number(maxTokens))) : 2048}
          onChange={(e) => setMaxTokens(parseInt(e.target.value))}
          className="w-full accent-[#355CFF] cursor-pointer"
        />
        <div className="flex justify-between ui-meta font-mono mt-0.5">
          <span>256</span>
          <span>16K</span>
          <span>64K</span>
          <span>128K</span>
        </div>
      </div>

      <div className="flex items-center space-x-2 pt-1">
        <input
          type="checkbox"
          id="streamCheck"
          checked={playStream}
          onChange={(e) => setPlayStream(e.target.checked)}
          className="rounded-sm border-[#E4E6E9] dark:border-[#2C3038] text-[#355CFF] focus:ring-0 cursor-pointer"
        />
        <label htmlFor="streamCheck" className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4] cursor-pointer">
          {isZh ? '流式输出' : 'Enable SSE Streaming'}
        </label>
      </div>
    </div>
  );
}
