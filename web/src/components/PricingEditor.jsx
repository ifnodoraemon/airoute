import { RefreshCw, AlertCircle, X } from 'lucide-react';
import PricingOffPeakFields from './PricingOffPeakFields';
import DiscardChangesDialog from './DiscardChangesDialog';

export default function PricingEditor({
  availableGroups, candidateModels, discardPricing, editingPrice, formData, formError, formatDaysLabel,
  getGroupBadge, handleAddSlot, handleRemoveSlot, handleSave, handleSelectModelCandidate, handleUpdateSlot,
  isDayActive, isZh, keepEditingPricing, lang, pricingConfirming, requestClosePricing, saving, setFormData,
  toggleSlotDay, unpricedModels
}) {
  return (
    <div
      data-testid="pricing-modal-overlay"
      className="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50 backdrop-blur-xs animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) requestClosePricing();
      }}
    >
      <div className="relative bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl p-6 sm:p-8 max-w-xl w-full shadow-xl space-y-5 animate-in zoom-in-95 duration-150 max-h-[90vh] overflow-y-auto">
        <div className="border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3 flex items-center justify-between">
          <div>
            <h3 className="font-semibold text-[#202124] dark:text-[#F1F2F4] text-base">
              {editingPrice
                ? (isZh ? `编辑 [${editingPrice.model}] 定价` : `Edit [${editingPrice.model}] Pricing`)
                : (isZh ? '新建模型定价与分时规则' : 'New Pricing & Off-Peak Rule')}
            </h3>
          </div>
          <button
            type="button"
            onClick={() => requestClosePricing()}
            aria-label={isZh ? '关闭' : 'Close'}
            className="p-1.5 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-lg text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {formError && (
          <div className="p-3 bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 rounded-lg text-xs text-rose-700 dark:text-rose-300 flex items-center space-x-2">
            <AlertCircle className="w-4 h-4 shrink-0" />
            <span>{formError}</span>
          </div>
        )}

        <form onSubmit={handleSave} className="space-y-4 text-xs">
          {/* Field 1: Model Identifier */}
          <div>
            <label className="block text-[#202124] dark:text-[#F1F2F4] font-medium mb-1">
              {isZh ? '模型名称 (Model)' : 'Model ID'} <span className="text-rose-500">*</span>
            </label>
            <input
              type="text"
              required
              disabled={!!editingPrice}
              list="unpriced-models-datalist"
              placeholder={isZh ? '如 deepseek-chat 或点击下方一键选择' : 'e.g. deepseek-chat or select below'}
              value={formData.model}
              onChange={(e) => setFormData({ ...formData, model: e.target.value })}
              className="w-full ui-field font-mono text-xs disabled:opacity-60"
            />
            <datalist id="unpriced-models-datalist">
              {unpricedModels.map(m => (
                <option key={m} value={m} />
              ))}
            </datalist>
            {!editingPrice && candidateModels.length > 0 && (
              <div className="mt-2.5 space-y-1.5">
                <div className="text-[11px] text-[#656B75] dark:text-[#A1A7B0] font-medium flex items-center justify-between">
                  <span>{isZh ? '快捷选择模型 (点击自动适配建议费率):' : 'Quick Select (auto-prefill rate):'}</span>
                  {candidateModels.some(c => c.isUnpriced) && (
                    <span className="text-amber-600 dark:text-amber-400 text-[10px] flex items-center gap-1">
                      <span className="w-1.5 h-1.5 rounded-full bg-amber-500 inline-block animate-pulse" />
                      {isZh ? '含未配费率模型' : 'Unpriced available'}
                    </span>
                  )}
                </div>
                <div className="flex flex-wrap gap-1.5 max-h-32 overflow-y-auto pr-0.5">
                  {candidateModels.map(({ model: m, isUnpriced }) => (
                    <button
                      key={m}
                      type="button"
                      onClick={() => handleSelectModelCandidate(m)}
                      className={`px-2 py-1 rounded-lg text-xs font-mono border transition cursor-pointer flex items-center space-x-1.5 ${
                        formData.model === m
                          ? 'bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border-[#355CFF]/30 font-semibold shadow-xs'
                          : isUnpriced
                          ? 'bg-amber-50/70 dark:bg-amber-950/40 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800/80 hover:border-amber-400'
                          : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border-[#E4E6E9] dark:border-[#2C3038] hover:border-[#355CFF]/40'
                      }`}
                      title={isUnpriced ? (isZh ? '当前组尚未配置该模型专属费率' : 'Unpriced in this group') : (isZh ? '点击自动填入并预设费率' : 'Click to select and prefill rate')}
                    >
                      <span>{m}</span>
                      {isUnpriced && (
                        <span className="w-1.5 h-1.5 rounded-full bg-amber-500 inline-block shrink-0" title={isZh ? '待配置专属费率' : 'Unpriced'} />
                      )}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>

          {/* Field: User Group Tier */}
          <div>
            <label className="block text-[#202124] dark:text-[#F1F2F4] font-medium mb-1">
              {isZh ? '定价范围与分组 (Scope)' : 'Pricing Scope'} <span className="text-rose-500">*</span>
            </label>
            <div className="ui-meta mb-1.5 text-[#656B75] dark:text-[#A1A7B0]">
              {isZh
                ? '推荐保留为 [默认对外售价 (default)]。所有业务分组（如 VIP 8折、企业组 1.5倍）自动按倍率继承此单价；仅当此模型在某分组需特殊单独议价覆盖时才选择特定组。'
                : 'Recommended: Default (default). All groups automatically inherit via their multiplier.'}
            </div>
            {editingPrice ? (
              <div className="flex items-center space-x-2 py-1">
                <span className={`px-2.5 py-1 rounded-lg text-xs font-semibold border ${getGroupBadge(formData.group_name).className}`}>
                  {getGroupBadge(formData.group_name).label} ({formData.group_name})
                </span>
              </div>
            ) : (
              <div className="space-y-2">
                <div className="flex flex-wrap items-center gap-1.5">
                  {availableGroups.map(g => (
                    <button
                      key={g}
                      type="button"
                      onClick={() => setFormData({ ...formData, group_name: g })}
                      className={`px-3 py-1.5 rounded-lg text-xs font-medium border transition cursor-pointer ${
                        formData.group_name === g
                          ? 'bg-[#355CFF] text-white border-[#355CFF] font-semibold shadow-xs'
                          : 'bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border-[#E4E6E9] dark:border-[#2C3038] text-[#202124] dark:text-[#F1F2F4]'
                      }`}
                    >
                      {g === 'default'
                        ? (isZh ? '默认组 (default)' : 'Default (default)')
                        : g === 'vip'
                          ? (isZh ? 'VIP组 (vip)' : 'VIP (vip)')
                          : g === 'enterprise'
                            ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)')
                            : `${g} ${isZh ? '组' : 'Tier'}`}
                    </button>
                  ))}
                </div>
                <input
                  type="text"
                  placeholder={isZh ? '或输入自定义用户组标识 (如 partner / test 等)' : 'Or input custom group identifier (e.g. partner / test)'}
                  value={formData.group_name}
                  onChange={(e) => setFormData({ ...formData, group_name: e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, '') })}
                  className="w-full ui-field font-mono text-xs"
                />
              </div>
            )}
          </div>

          {/* Field 2: Base Rates */}
          <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038] space-y-3">
            <span className="font-semibold text-[#202124] dark:text-[#F1F2F4] block text-xs">
              {isZh ? '基准单价 (每 1,000,000 Tokens)' : 'Base Price (per 1,000,000 Tokens)'}
            </span>
            <div className="grid grid-cols-3 gap-3">
              <div>
                <label className="block text-[#656B75] dark:text-[#A1A7B0] font-medium mb-1">
                  {isZh ? '输入单价 (¥)' : 'Prompt Price (¥)'}
                </label>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  required
                  value={formData.prompt_price}
                  onChange={(e) => {
                    const val = parseFloat(e.target.value) || 0;
                    setFormData({
                      ...formData,
                      prompt_price: e.target.value,
                      cache_read_price: (val * 0.1).toFixed(2)
                    });
                  }}
                  className="w-full ui-field font-mono text-xs"
                />
              </div>

              <div>
                <label className="block text-[#656B75] dark:text-[#A1A7B0] font-medium mb-1">
                  {isZh ? '输出单价 (¥)' : 'Completion Price (¥)'}
                </label>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  required
                  value={formData.completion_price}
                  onChange={(e) => setFormData({ ...formData, completion_price: e.target.value })}
                  className="w-full ui-field font-mono text-xs"
                />
              </div>

              <div>
                <label className="block text-emerald-700 dark:text-emerald-400 font-medium mb-1">
                  {isZh ? '缓存命中 (¥)' : 'Cache Hit (¥)'}
                </label>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={formData.cache_read_price}
                  onChange={(e) => setFormData({ ...formData, cache_read_price: e.target.value })}
                  className="w-full ui-field font-mono text-xs border-emerald-300 dark:border-emerald-800"
                />
              </div>
            </div>
          </div>

          {/* Field 3: Time-of-Use Multi-Slot Flexible Configuration */}
          <PricingOffPeakFields {...{
            formData, formatDaysLabel, handleAddSlot, handleRemoveSlot, handleUpdateSlot, isDayActive, isZh,
            setFormData, toggleSlotDay
          }} />

          {/* Actions */}
          <div className="pt-2 flex items-center justify-end space-x-2.5">
            <button
              type="button"
              onClick={() => requestClosePricing()}
              className="ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={saving}
              className="ui-btn-primary ui-btn-sm disabled:opacity-50"
            >
              {saving ? (
                <>
                  <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                  <span>{isZh ? '正在保存...' : 'Saving...'}</span>
                </>
              ) : (
                <span>{isZh ? '保存定价' : 'Save Pricing'}</span>
              )}
            </button>
          </div>
        </form>

        <DiscardChangesDialog
          open={pricingConfirming}
          onKeepEditing={keepEditingPricing}
          onDiscard={discardPricing}
          lang={lang}
          title={isZh ? '放弃未保存的定价修改？' : 'Discard these pricing edits?'}
        />
      </div>
    </div>
  );
}
