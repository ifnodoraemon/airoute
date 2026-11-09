import { RefreshCw, Coins } from 'lucide-react';
import DiscardChangesDialog from './DiscardChangesDialog';

export default function UserQuotaModal({
  adjustingQuota, discardQuota, handleAdjustQuota, isZh, keepEditingQuota, lang, quotaConfirming,
  quotaModalError, quotaMode, quotaTargetUser, quotaValue, requestCloseQuota, setQuotaMode, setQuotaValue
}) {
  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50 backdrop-blur-xs animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) requestCloseQuota();
      }}
    >
      <div className="relative bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl p-6 max-w-sm w-full shadow-xl space-y-4 animate-in zoom-in-95 duration-150">
        <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
          <div className="flex items-center space-x-2">
            <Coins className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF]" />
            <h3 className="font-semibold text-base text-[#202124] dark:text-[#F1F2F4]">{isZh ? '调整用户可用额度' : 'Adjust User Quota / Balance'}</h3>
          </div>
          <button
            onClick={() => requestCloseQuota()}
            className="p-1 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-lg text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] cursor-pointer"
          >
            ✕
          </button>
        </div>

        <div className="p-3 bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs space-y-1">
          <div className="ui-meta">{isZh ? '目标用户:' : 'Target User:'} <span className="font-semibold text-[#202124] dark:text-[#F1F2F4]">{quotaTargetUser.username}</span></div>
          <div className="ui-meta">{isZh ? '当前余额:' : 'Current Balance:'} <span className="font-semibold font-mono text-[#355CFF] dark:text-[#91A7FF]">¥ {Number(quotaTargetUser.balance || 0).toFixed(2)}</span></div>
        </div>

        {quotaModalError && (
          <div className="p-2.5 rounded-lg bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs">
            {quotaModalError}
          </div>
        )}

        <form onSubmit={handleAdjustQuota} className="space-y-3">
          <div className="grid grid-cols-2 gap-1 bg-[#F0F1F2] dark:bg-[#22252B] p-1 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs font-medium">
            <button
              type="button"
              onClick={() => setQuotaMode('delta')}
              className={`py-1.5 rounded-md transition cursor-pointer ${
                quotaMode === 'delta' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0]'
              }`}
            >
              {isZh ? '增减额度 (+/-)' : 'Add/Deduct (+/-)'}
            </button>
            <button
              type="button"
              onClick={() => setQuotaMode('exact')}
              className={`py-1.5 rounded-md transition cursor-pointer ${
                quotaMode === 'exact' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0]'
              }`}
            >
              {isZh ? '设定确切余额' : 'Set Exact Balance'}
            </button>
          </div>

          <div>
            <label className="block text-xs font-medium text-[#202124] dark:text-[#F1F2F4] mb-1">
              {quotaMode === 'delta' ? (isZh ? '调整额度数值 (可输入负数扣除)' : 'Adjustment Amount (negative to deduct)') : (isZh ? '设定新的账户余额' : 'New Account Balance')}
            </label>
            <input
              type="number"
              step="0.01"
              required
              value={quotaValue}
              onChange={(e) => setQuotaValue(e.target.value)}
              className="w-full ui-field font-mono text-base font-semibold"
            />
          </div>

          {/* Quick Preset Buttons */}
          <div className="flex items-center space-x-1.5 pt-1">
            {[10, 50, 100, 500].map((num) => (
              <button
                key={num}
                type="button"
                onClick={() => {
                  setQuotaMode('delta');
                  setQuotaValue(String(num));
                }}
                className="px-2 py-1 bg-[#F0F1F2] dark:bg-[#22252B] hover:bg-[#E4E6E9] dark:hover:bg-[#2C3038] rounded-md text-xs font-medium text-[#202124] dark:text-[#F1F2F4] cursor-pointer"
              >
                +{num}
              </button>
            ))}
            <button
              type="button"
              onClick={() => {
                setQuotaMode('exact');
                setQuotaValue('0');
              }}
              className="px-2 py-1 bg-rose-50 dark:bg-rose-950/40 hover:bg-rose-100 dark:hover:bg-rose-900/60 rounded-md text-xs font-medium text-rose-700 dark:text-rose-300 cursor-pointer"
            >
              {isZh ? '归零' : 'Zero'}
            </button>
          </div>

          <div className="pt-2 flex items-center justify-end space-x-2">
            <button
              type="button"
              onClick={() => requestCloseQuota()}
              className="ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={adjustingQuota}
              className="ui-btn-primary ui-btn-sm disabled:opacity-50"
            >
              {adjustingQuota ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <span>{isZh ? '确认更新' : 'Update Balance'}</span>}
            </button>
          </div>
        </form>

        <DiscardChangesDialog
          open={quotaConfirming}
          onKeepEditing={keepEditingQuota}
          onDiscard={discardQuota}
          lang={lang}
          title={isZh ? '放弃未保存的额度调整？' : 'Discard this quota change?'}
        />
      </div>
    </div>
  );
}
