import { useEffect } from 'react';

export default function usePricingEditor({
  fetchPrices, formData, groupFilter, isZh, parseSlotsFromPrice, setEditingPrice, setFormData, setFormError,
  setShowModal
}) {
  const handleSelectModelCandidate = (m) => {
    const lower = m.toLowerCase();
    let promptP = 2.0;
    let compP = 8.0;
    let cacheP = 0.2;
    if (lower.includes('image') || lower.includes('seedream') || lower.includes('dall-e') || lower.includes('flux')) {
      promptP = 0.10;
      compP = 0.10;
      cacheP = 0.01;
    } else if (lower.includes('video') || lower.includes('seedance') || lower.includes('wan') || lower.includes('kling') || lower.includes('cogvideo') || lower.includes('sora')) {
      promptP = 0.50;
      compP = 0.50;
      cacheP = 0.05;
    } else if (lower.includes('embed') || lower.includes('bge-') || lower.includes('rerank')) {
      promptP = 0.05;
      compP = 0.05;
      cacheP = 0.005;
    }
    setFormData(prev => ({
      ...prev,
      model: m,
      prompt_price: promptP,
      completion_price: compP,
      cache_read_price: cacheP
    }));
  };

  useEffect(() => {
    fetchPrices();
  }, [fetchPrices]);

  const handleOpenAdd = () => {
    setEditingPrice(null);
    setFormData({
      model: '',
      group_name: groupFilter !== 'all' ? groupFilter : 'default',
      prompt_price: 2.0,
      completion_price: 8.0,
      cache_read_price: 0.2,
      fixed_price: 0.0,
      currency: 'CNY',
      off_peak_enabled: true,
      weekend_all_day: true,
      slots: [
        { id: `slot-${Date.now()}`, name: isZh ? '时段 1' : 'Slot 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
      ]
    });
    setFormError('');
    setShowModal(true);
  };

  const handleOpenEdit = (p) => {
    setEditingPrice(p);
    setFormData({
      model: p.model,
      group_name: p.group_name || 'default',
      prompt_price: p.prompt_price ?? 2.0,
      completion_price: p.completion_price ?? 8.0,
      cache_read_price: p.cache_read_price ?? 0.2,
      fixed_price: p.fixed_price ?? 0.0,
      currency: p.currency || 'CNY',
      off_peak_enabled: p.off_peak_enabled !== false,
      weekend_all_day: p.weekend_all_day !== false,
      slots: parseSlotsFromPrice(p, isZh)
    });
    setFormError('');
    setShowModal(true);
  };

  const handleAddSlot = () => {
    const nextId = `slot-${Date.now()}-${formData.slots.length + 1}`;
    setFormData(prev => ({
      ...prev,
      slots: [
        ...prev.slots,
        {
          id: nextId,
          name: isZh ? `时段 ${prev.slots.length + 1}` : `Slot ${prev.slots.length + 1}`,
          start: '12:00',
          end: '14:00',
          discount: 0.8,
          days: []
        }
      ]
    }));
  };

  const handleRemoveSlot = (slotId) => {
    setFormData(prev => ({
      ...prev,
      slots: prev.slots.filter(s => s.id !== slotId)
    }));
  };

  const handleUpdateSlot = (slotId, field, value) => {
    setFormData(prev => ({
      ...prev,
      slots: prev.slots.map(s => s.id === slotId ? { ...s, [field]: value } : s)
    }));
  };

  const isDayActive = (slotDays, day) => {
    if (!slotDays || slotDays.length === 0) return true;
    return slotDays.includes(day);
  };

  const toggleSlotDay = (slotId, day) => {
    setFormData(prev => ({
      ...prev,
      slots: prev.slots.map(s => {
        if (s.id !== slotId) return s;
        const current = (!s.days || s.days.length === 0) ? [1, 2, 3, 4, 5, 6, 7] : [...s.days];
        let updated;
        if (current.includes(day)) {
          if (current.length === 1) return s; // Keep at least one day
          updated = current.filter(d => d !== day);
        } else {
          updated = [...current, day].sort((a, b) => a - b);
        }
        if (updated.length === 7) updated = [];
        return { ...s, days: updated };
      })
    }));
  };
  return {
    handleAddSlot, handleOpenAdd, handleOpenEdit, handleRemoveSlot, handleSelectModelCandidate,
    handleUpdateSlot, isDayActive, toggleSlotDay
  };
}
