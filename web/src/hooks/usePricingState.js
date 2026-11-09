import useUrlFilters from './useUrlFilters';
import { useState } from 'react';
import useModalA11y from './useModalA11y';
import useFormDirty from './useFormDirty';
import useUnsavedGuard from './useUnsavedGuard';

export default function usePricingState({
  groupMultipliers, isAdmin, isZh
}) {
  const [pricingFilters, setPricingFilters] = useUrlFilters('pricing', { view: 'models', channel: '', model: '' });
  const pricingSubTab = ['models', 'groups', ...(isAdmin ? ['suppliers'] : [])].includes(pricingFilters.view) ? pricingFilters.view : 'models';
  const setPricingSubTab = view => setPricingFilters({ view });
  const [localGroupMultipliers, setLocalGroupMultipliers] = useState(groupMultipliers || { default: 1.0, vip: 0.8, enterprise: 1.5 });
  const [savingGroups, setSavingGroups] = useState(false);
  const [showAddGroupModal, setShowAddGroupModal] = useState(false);
  const [newGroupInputKey, setNewGroupInputKey] = useState('');
  const [newGroupInputMultiplier, setNewGroupInputMultiplier] = useState(0.8);
  const [tierPreviewGroup, setTierPreviewGroup] = useState('default');
  const [prices, setPrices] = useState([]);
  const [loading, setLoading] = useState(false);
  const [showModal, setShowModal] = useState(false);
  useModalA11y(showAddGroupModal, () => setShowAddGroupModal(false));
  const [editingPrice, setEditingPrice] = useState(null);
  // The gateway reports its own clock because off-peak windows are evaluated in
  // the gateway's time zone, not the browser's. The header renders these next to
  // the rate table so "off-peak active" is interpretable.
  const [serverTime, setServerTime] = useState('');
  const [serverWeekday, setServerWeekday] = useState('');
  const [isWeekend, setIsWeekend] = useState(false);
  const [platformModels, setPlatformModels] = useState([]);
  const [selectedKeys, setSelectedKeys] = useState([]);
  const [groupFilter, setGroupFilter] = useState('all');

  // Clean, flexible form state (supports multiple arbitrary time windows & user group tiers)
  const [formData, setFormData] = useState({
    model: '',
    group_name: 'default',
    prompt_price: 2.0,
    completion_price: 8.0,
    cache_read_price: 0.2,
    fixed_price: 0.0,
    currency: 'CNY',
    off_peak_enabled: true,
    weekend_all_day: true,
    slots: [
      { id: 'slot-1', name: isZh ? '时段 1' : 'Slot 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
    ]
  });
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');

  // Dismissing the add/edit rule sheet (backdrop / X / Cancel / Esc) must not
  // silently drop a half-configured price and its off-peak slots. A successful
  // save closes directly, because there is nothing left to discard.
  const pricingFormDirty = useFormDirty(showModal, formData);
  const {
    confirming: pricingConfirming,
    requestClose: requestClosePricing,
    keepEditing: keepEditingPricing,
    discard: discardPricing
  } = useUnsavedGuard({ isOpen: showModal, isDirty: pricingFormDirty, onClose: () => setShowModal(false) });
  useModalA11y(showModal, requestClosePricing);
  return {
    discardPricing, editingPrice, formData, formError, groupFilter, isWeekend, keepEditingPricing, loading,
    localGroupMultipliers, newGroupInputKey, newGroupInputMultiplier, platformModels, prices, pricingConfirming,
    pricingFilters, pricingSubTab, requestClosePricing, saving, savingGroups, selectedKeys, serverTime,
    serverWeekday, setEditingPrice, setFormData, setFormError, setGroupFilter, setIsWeekend, setLoading,
    setLocalGroupMultipliers, setNewGroupInputKey, setNewGroupInputMultiplier, setPlatformModels, setPrices,
    setPricingFilters, setPricingSubTab, setSaving, setSavingGroups, setSelectedKeys, setServerTime,
    setServerWeekday, setShowAddGroupModal, setShowModal, setTierPreviewGroup, showAddGroupModal, showModal,
    tierPreviewGroup
  };
}
