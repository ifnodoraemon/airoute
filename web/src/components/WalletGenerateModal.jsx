import { Gift, RefreshCw, Sparkles } from 'lucide-react';

export default function WalletGenerateModal({
  genAmount, genCount, genName, generating, handleGenerateRedemptions, isZh, setGenAmount, setGenCount,
  setGenName, setShowGenModal
}) {
  return (
    <div
      className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) setShowGenModal(false);
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-6 max-w-md w-full shadow-2xl space-y-5 animate-in zoom-in-95 duration-150">
        <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
          <div className="flex items-center space-x-2">
            <div className="w-8 h-8 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] flex items-center justify-center">
              <Gift className="w-4 h-4" />
            </div>
            <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">{isZh ? '批量生成额度兑换卡' : 'Batch Generate Gift Cards'}</h3>
          </div>
          <button
            onClick={() => setShowGenModal(false)}
            className="ui-icon-btn"
          >
            ✕
          </button>
        </div>

        <form onSubmit={handleGenerateRedemptions} className="space-y-4 text-xs">
          <div>
            <label className="ui-label">{isZh ? '兑换卡名称' : 'Card Name'}</label>
            <input
              type="text"
              value={genName}
              onChange={(e) => setGenName(e.target.value)}
              placeholder={isZh ? '例如: 2026 新年礼品卡' : 'e.g. 2026 Promo Gift Card'}
              className="ui-field"
            />
          </div>

          <div>
            <label className="ui-label">{isZh ? '单张充值面额 (CNY)' : 'Amount per Card (CNY)'}</label>
            <input
              type="number"
              min="0.1"
              step="0.1"
              required
              value={genAmount}
              onChange={(e) => setGenAmount(e.target.value)}
              placeholder="50"
              className="ui-field font-mono tabular-nums"
            />
          </div>

          <div>
            <label className="ui-label">{isZh ? '生成数量 (张)' : 'Quantity'}</label>
            <input
              type="number"
              min="1"
              max="100"
              required
              value={genCount}
              onChange={(e) => setGenCount(e.target.value)}
              placeholder="5"
              className="ui-field font-mono tabular-nums"
            />
            <p className="ui-meta mt-1">{isZh ? '一次最多可生成 100 张随机加密卡密。' : 'Up to 100 encrypted gift codes can be generated per batch.'}</p>
          </div>

          <div className="pt-3 flex items-center justify-end space-x-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
            <button
              type="button"
              onClick={() => setShowGenModal(false)}
              className="ui-btn-secondary"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={generating}
              className="ui-btn-primary"
            >
              {generating ? (
                <RefreshCw className="w-3.5 h-3.5 animate-spin" />
              ) : (
                <>
                  <Sparkles className="w-3.5 h-3.5" />
                  <span>{isZh ? '确认生成' : 'Generate Cards'}</span>
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
