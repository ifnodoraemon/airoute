import { formatDaysLabel, checkSlotsOverlap, parseSlotsFromPrice } from '../lib/pricingSlots';
export { checkSlotsOverlap } from '../lib/pricingSlots';
import usePricingMutations from '../hooks/usePricingMutations';
import usePricingEditor from '../hooks/usePricingEditor';
import usePricingModels from '../hooks/usePricingModels';
import usePricingGroups from '../hooks/usePricingGroups';
import usePricingData from '../hooks/usePricingData';
import usePricingState from '../hooks/usePricingState';
import PricingNavigation from './PricingNavigation';
import PricingModels from './PricingModels';
import PricingEditor from './PricingEditor';

import PricingGroupTiers from './PricingGroupTiers';

import { Plus, RefreshCw, Trash2 } from 'lucide-react';

import SupplierCostPanel from './SupplierCostPanel';
import ModelSetupGuide from './ModelSetupGuide';

export default function PricingManager({
  adminFetch,
  showToast,
  isAdmin = true,
  lang = 'zh',
  modelRoutes = [],
  channels = [],
  setCurrentTab,
  groupMultipliers,
  onUpdateGroupMultipliers
}) {
  const isZh = lang === 'zh';
  const {
    discardPricing, editingPrice, formData, formError, groupFilter, isWeekend, keepEditingPricing, loading,
    localGroupMultipliers, newGroupInputKey, newGroupInputMultiplier, platformModels, prices, pricingConfirming,
    pricingFilters, pricingSubTab, requestClosePricing, saving, savingGroups, selectedKeys, serverTime,
    serverWeekday, setEditingPrice, setFormData, setFormError, setGroupFilter, setIsWeekend, setLoading,
    setLocalGroupMultipliers, setNewGroupInputKey, setNewGroupInputMultiplier, setPlatformModels, setPrices,
    setPricingFilters, setPricingSubTab, setSaving, setSavingGroups, setSelectedKeys, setServerTime,
    setServerWeekday, setShowAddGroupModal, setShowModal, setTierPreviewGroup, showAddGroupModal, showModal,
    tierPreviewGroup
  } = usePricingState({
    groupMultipliers, isAdmin, isZh
  });

  // Wrapped so the mount effect can depend on it without re-running every render.
  // Its own dependencies are the props it reads, all of which are stable in
  // practice: the parent passes a module-level fetch and a memoised toast.
  const {
    availableGroups, fetchPrices
  } = usePricingData({
    adminFetch, groupMultipliers, isZh, localGroupMultipliers, prices, setIsWeekend, setLoading,
    setLocalGroupMultipliers, setPlatformModels, setPrices, setServerTime, setServerWeekday, showToast
  });

  const {
    handleCreateNewGroupTier, handleDeleteGroupTier, handleSaveGroupMultipliers, handleUpdateSingleMultiplier
  } = usePricingGroups({
    adminFetch, isZh, localGroupMultipliers, newGroupInputKey, newGroupInputMultiplier,
    onUpdateGroupMultipliers, setLocalGroupMultipliers, setNewGroupInputKey, setNewGroupInputMultiplier,
    setSavingGroups, setShowAddGroupModal, showToast
  });

  const {
    candidateModels, filteredPrices, getGroupBadge, mountedRouteModels, unpricedModels, unpricedRoutes,
    unroutedPrices
  } = usePricingModels({
    formData, groupFilter, isZh, modelRoutes, platformModels, prices
  });

  const {
    handleAddSlot, handleOpenAdd, handleOpenEdit, handleRemoveSlot, handleSelectModelCandidate,
    handleUpdateSlot, isDayActive, toggleSlotDay
  } = usePricingEditor({
    fetchPrices, formData, groupFilter, isZh, parseSlotsFromPrice, setEditingPrice, setFormData, setFormError,
    setShowModal
  });

  const {
    handleBatchDelete, handleDelete, handleSave
  } = usePricingMutations({
    adminFetch, checkSlotsOverlap, fetchPrices, formData, isZh, selectedKeys, setFormError, setSaving,
    setSelectedKeys, setShowModal, showToast
  });

  return (
    <div className="space-y-6 animate-in fade-in">
      {/* 1. Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h1 className="ui-page-title">
            {isZh ? '模型定价' : 'Model Pricing'}
          </h1>
          <p className="ui-page-subtitle">
            {isZh
              ? (isAdmin ? '管理对外售价、客户分组折扣与渠道采购报价' : '查看对外费率和客户分组折扣')
              : 'Manage customer prices, group discounts and channel purchase quotes'}
          </p>
        </div>

        <div className="flex items-center space-x-2 shrink-0">
          {isAdmin && (
            <button
              onClick={handleOpenAdd}
              className="ui-btn-primary ui-btn-sm"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>{isZh ? '新建定价规则' : 'New Pricing Rule'}</span>
            </button>
          )}
          <button
            onClick={fetchPrices}
            disabled={loading}
            className="ui-btn-secondary ui-btn-sm"
            title={isZh ? '刷新费率' : 'Refresh'}
          >
            <RefreshCw className={`w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {isAdmin && <ModelSetupGuide lang={lang} />}

      {/* 1.5 Sub Navigation: Model Baseline Pricing vs Group Discount Tiers vs Supplier Prices */}
      <PricingNavigation {...{
        isAdmin, isZh, localGroupMultipliers, prices, pricingSubTab, setPricingSubTab
      }} />

      {/* Batch Action Bar (Admin only) */}
      {isAdmin && selectedKeys.length > 0 && (
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/30 px-4 py-2.5 rounded-xl animate-in fade-in gap-3">
          <div className="flex items-center space-x-2 text-xs font-semibold text-[#355CFF] dark:text-[#91A7FF]">
            <span>{isZh ? `已选中 ${selectedKeys.length} 个模型定价规则` : `${selectedKeys.length} pricing rule(s) selected`}</span>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchDelete}
              className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white rounded-lg text-xs font-medium shadow-xs transition flex items-center space-x-1 cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>{isZh ? '批量删除' : 'Delete Selected'}</span>
            </button>
            <button
              onClick={() => setSelectedKeys([])}
              className="ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '取消选择' : 'Deselect'}
            </button>
          </div>
        </div>
      )}

      {/* 2. Rates Table (High Whitespace & Clean) */}
      {pricingSubTab === 'models' && (
        <PricingModels {...{
          availableGroups, filteredPrices, formatDaysLabel, getGroupBadge, groupFilter, handleDelete, handleOpenEdit,
          isAdmin, isWeekend, isZh, loading, localGroupMultipliers, mountedRouteModels, platformModels, prices,
          selectedKeys, serverTime, serverWeekday, setCurrentTab, setEditingPrice, setFormData, setGroupFilter,
          setPricingSubTab, setSelectedKeys, setShowModal, setTierPreviewGroup, tierPreviewGroup, unpricedRoutes,
          unroutedPrices
        }} />
  )}

      {/* 2.5 Group Discount Tiers Management View */}
      {pricingSubTab === 'groups' && (
        <PricingGroupTiers {...{
          getGroupBadge, handleCreateNewGroupTier, handleDeleteGroupTier, handleSaveGroupMultipliers,
          handleUpdateSingleMultiplier, isAdmin, isZh, localGroupMultipliers, newGroupInputKey,
          newGroupInputMultiplier, savingGroups, setNewGroupInputKey, setNewGroupInputMultiplier,
          setShowAddGroupModal, showAddGroupModal
        }} />
      )}

      {/* 2.6 Supplier Prices View: our selling price next to the supplier price.
          Both lists are loaded and edited independently; the table only pairs them
          by model so the spread (ours - supplier) is readable in one place. */}
      {isAdmin && pricingSubTab === 'suppliers' && (
        <SupplierCostPanel
          adminFetch={adminFetch}
          showToast={showToast}
          lang={lang}
          prices={prices}
          channels={channels}
          modelRoutes={modelRoutes}
          focusChannel={pricingFilters.channel}
          focusModel={pricingFilters.model}
          onClearTarget={() => setPricingFilters({ channel: '', model: '' })}
        />
      )}

      {/* 3. Flexible Multi-Slot Edit / Add Modal */}
      {showModal && (
        <PricingEditor {...{
          availableGroups, candidateModels, discardPricing, editingPrice, formData, formError, formatDaysLabel,
          getGroupBadge, handleAddSlot, handleRemoveSlot, handleSave, handleSelectModelCandidate, handleUpdateSlot,
          isDayActive, isZh, keepEditingPricing, lang, pricingConfirming, requestClosePricing, saving, setFormData,
          toggleSlotDay, unpricedModels
        }} />
      )}
    </div>
  );
}
