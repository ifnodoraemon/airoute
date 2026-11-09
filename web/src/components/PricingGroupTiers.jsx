import { Plus, Trash2, X, Sliders } from 'lucide-react';

export default function PricingGroupTiers({
  getGroupBadge, handleCreateNewGroupTier, handleDeleteGroupTier, handleSaveGroupMultipliers,
  handleUpdateSingleMultiplier, isAdmin, isZh, localGroupMultipliers, newGroupInputKey,
  newGroupInputMultiplier, savingGroups, setNewGroupInputKey, setNewGroupInputMultiplier,
  setShowAddGroupModal, showAddGroupModal
}) {
  return (
    <div className="space-y-6">
      {/* Architecture Banner */}
      <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl p-5 shadow-xs">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
          <div className="space-y-1">
            <div className="flex items-center space-x-2 text-[#202124] dark:text-[#F1F2F4] font-semibold text-sm">
              <Sliders className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF] shrink-0" />
              <span>{isZh ? '业务客户分组体系' : 'Group Discount & Tier Pricing'}</span>
            </div>
            <div className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-mono">
              {isZh
                ? '扣费公式: 实际调用单价 = 默认对外单价 × 密钥所属分组倍率 (× 闲时时段折扣)'
                : 'Charge Formula: Base Model Rate * Group Multiplier (* Off-Peak Discount)'}
            </div>
          </div>

          {isAdmin && (
            <div className="flex items-center space-x-2 shrink-0">
              <button
                type="button"
                onClick={() => setShowAddGroupModal(true)}
                className="ui-btn-secondary ui-btn-sm"
              >
                <Plus className="w-3.5 h-3.5" />
                <span>{isZh ? '新建分组' : 'New Group Tier'}</span>
              </button>
              <button
                type="button"
                disabled={savingGroups}
                onClick={() => handleSaveGroupMultipliers()}
                className="ui-btn-primary ui-btn-sm disabled:opacity-50"
              >
                <span>{savingGroups ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存全部倍率' : 'Save Multipliers')}</span>
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Group Tiers Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {Object.entries(localGroupMultipliers).map(([grp, mult]) => {
          const isDefault = grp === 'default';
          const isVip = grp === 'vip';
          const isEnterprise = grp === 'enterprise';
          const discountPercent = Math.round((1 - mult) * 100);

          return (
            <div
              key={grp}
              className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl p-5 space-y-4 hover:border-[#355CFF]/40 dark:hover:border-[#91A7FF]/40 transition flex flex-col justify-between"
            >
              <div className="space-y-3">
                <div className="flex items-start justify-between">
                  <div>
                    <div className="flex items-center space-x-2">
                      <span className="font-bold text-[#202124] dark:text-[#F1F2F4] text-sm font-mono">
                        {grp.toUpperCase()}
                      </span>
                      <span className={`px-2 py-0.5 rounded-md text-[10px] font-bold border ${getGroupBadge(grp).className}`}>
                        {getGroupBadge(grp).label}
                      </span>
                    </div>
                    <div className="text-[11px] text-[#656B75] dark:text-[#A1A7B0] mt-1">
                      {isDefault
                        ? (isZh ? '默认对外售价 (100% 基线)' : 'Default retail rate (100%)')
                        : isVip
                        ? (isZh ? 'VIP专享优惠，全模型适用' : 'VIP Discount Tier')
                        : isEnterprise
                        ? (isZh ? '企业专属保障，含高并发与SLA' : 'Enterprise Tier with SLA')
                        : (isZh ? '自定义业务折扣组' : 'Custom Discount Group')}
                    </div>
                  </div>

                  {!isDefault && isAdmin && (
                    <button
                      type="button"
                      onClick={() => handleDeleteGroupTier(grp)}
                      className="p-1.5 text-[#656B75] hover:text-rose-600 dark:text-[#A1A7B0] dark:hover:text-rose-400 rounded-lg hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition cursor-pointer"
                      title={isZh ? '删除该分组' : 'Delete Group'}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>

                {/* Multiplier display & adjustments */}
                <div className="bg-[#F0F1F2] dark:bg-[#22252B] rounded-lg p-3 border border-[#E4E6E9] dark:border-[#2C3038] space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-medium">
                      {isZh ? '计费倍数 (Multiplier)' : 'Rate Multiplier'}
                    </span>
                    <div className="flex items-center space-x-1.5">
                      {isDefault ? (
                        <span className="font-mono font-bold text-sm text-[#202124] dark:text-[#F1F2F4]">
                          1.00x
                        </span>
                      ) : (
                        <div className="flex items-center space-x-1">
                          <input
                            type="number"
                            step="0.05"
                            min="0.01"
                            max="10"
                            disabled={!isAdmin}
                            value={mult}
                            onChange={(e) => handleUpdateSingleMultiplier(grp, e.target.value)}
                            className="w-20 px-2 py-1 text-right font-mono font-bold text-sm ui-field"
                          />
                          <span className="text-xs font-mono text-[#656B75] dark:text-[#A1A7B0]">x</span>
                        </div>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center justify-between text-[11px]">
                    <span className="text-[#656B75] dark:text-[#A1A7B0]">
                      {isZh ? '折算比例:' : 'Ratio:'}
                    </span>
                    <span className={`font-semibold ${mult < 1.0 ? 'text-emerald-600 dark:text-emerald-400' : mult > 1.0 ? 'text-amber-600 dark:text-amber-400' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}>
                      {mult === 1.0
                        ? (isZh ? '默认对外售价 (100%)' : 'Standard 100%')
                        : mult < 1.0
                        ? (isZh ? `${Math.round(mult * 10)}折优惠 (-${discountPercent}%)` : `${discountPercent}% Discount`)
                        : (isZh ? `${Math.round(mult * 100)}% 上浮 (+${Math.round((mult - 1) * 100)}%)` : `+${Math.round((mult - 1) * 100)}% Surcharge`)}
                    </span>
                  </div>

                  {!isDefault && isAdmin && (
                    <div className="pt-2 flex flex-wrap gap-1 border-t border-[#E4E6E9]/60 dark:border-[#2C3038]/60">
                      {[0.5, 0.7, 0.8, 0.9, 1.0, 1.2, 1.5].map((preset) => (
                        <button
                          key={preset}
                          type="button"
                          onClick={() => handleUpdateSingleMultiplier(grp, preset)}
                          className={`px-2 py-0.5 rounded text-[10px] font-mono font-medium transition cursor-pointer ${
                            mult === preset
                              ? 'bg-[#355CFF] text-white font-bold'
                              : 'bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'
                          }`}
                        >
                          {preset}x
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Add Group Modal */}
      {showAddGroupModal && (
        <div
          className="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50 backdrop-blur-xs animate-in fade-in"
          onClick={(e) => {
            if (e.target === e.currentTarget) setShowAddGroupModal(false);
          }}
        >
          <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl p-6 max-w-md w-full shadow-xl space-y-4 animate-in zoom-in-95">
            <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
              <h3 className="font-semibold text-[#202124] dark:text-[#F1F2F4] text-sm">
                {isZh ? '新建计费分组' : 'New Pricing Group Tier'}
              </h3>
              <button
                type="button"
                onClick={() => setShowAddGroupModal(false)}
                className="p-1 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-lg text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] transition"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <form onSubmit={handleCreateNewGroupTier} className="space-y-4 text-xs">
              <div>
                <label className="block text-[#202124] dark:text-[#F1F2F4] font-medium mb-1">
                  {isZh ? '分组英文标识 (Group ID)' : 'Group Identifier'} <span className="text-rose-500">*</span>
                </label>
                <input
                  type="text"
                  required
                  placeholder={isZh ? '如 partner / test_tier / svip' : 'e.g. partner / svip'}
                  value={newGroupInputKey}
                  onChange={(e) => setNewGroupInputKey(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ''))}
                  className="w-full ui-field font-mono text-xs"
                />
                <div className="ui-meta mt-1">
                  {isZh ? '仅支持小写字母、数字、下划线及连字符' : 'Lowercase letters, digits, hyphen, underscore only'}
                </div>
              </div>

              <div>
                <label className="block text-[#202124] dark:text-[#F1F2F4] font-medium mb-1">
                  {isZh ? '费率倍数 (Multiplier)' : 'Multiplier'} <span className="text-rose-500">*</span>
                </label>
                <div className="flex items-center space-x-2">
                  <input
                    type="number"
                    step="0.05"
                    min="0.01"
                    max="10"
                    required
                    value={newGroupInputMultiplier}
                    onChange={(e) => setNewGroupInputMultiplier(e.target.value)}
                    className="w-full ui-field font-mono text-xs"
                  />
                  <span className="font-mono text-[#656B75] dark:text-[#A1A7B0]">x</span>
                </div>
                <div className="ui-meta mt-1">
                  {isZh ? '0.8 为 8折，1.5 为 1.5倍。该分组密钥将按此倍率乘以默认对外单价扣费。' : 'Base model prices will be multiplied by this factor.'}
                </div>
              </div>

              <div className="flex justify-end space-x-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowAddGroupModal(false)}
                  className="ui-btn-secondary ui-btn-sm"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  className="ui-btn-primary ui-btn-sm"
                >
                  {isZh ? '确认添加' : 'Add Group'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
