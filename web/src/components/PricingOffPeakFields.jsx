import { Plus, Trash2, Moon } from 'lucide-react';

export default function PricingOffPeakFields({
  formData, formatDaysLabel, handleAddSlot, handleRemoveSlot, handleUpdateSlot, isDayActive, isZh,
  setFormData, toggleSlotDay
}) {
  return (
    <div className="p-4 rounded-xl bg-[#EEF2FF]/40 dark:bg-[#242D48]/30 border border-[#355CFF]/20 space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <Moon className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
          <span className="font-semibold text-[#202124] dark:text-[#F1F2F4]">
            {isZh ? '开启分时优惠' : 'Enable Off-Peak Discount'}
          </span>
        </div>
        <label className="relative inline-flex items-center cursor-pointer">
          <input
            type="checkbox"
            checked={formData.off_peak_enabled}
            onChange={(e) => setFormData({ ...formData, off_peak_enabled: e.target.checked })}
            className="sr-only peer"
          />
          <div className="w-9 h-5 bg-[#E4E6E9] dark:bg-[#2C3038] peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white dark:after:bg-[#F1F2F4] after:border-[#E4E6E9] dark:after:border-[#2C3038] after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-[#355CFF]"></div>
        </label>
      </div>

      {formData.off_peak_enabled && (
        <div className="space-y-3 pt-1 animate-in fade-in">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4]">
              {isZh ? '优惠时间段列表 (支持自由配置任意多个时段)' : 'Time Windows (Multiple custom windows supported)'}
            </span>
            <button
              type="button"
              onClick={handleAddSlot}
              className="ui-btn-primary ui-btn-sm"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>{isZh ? '添加时间段' : 'Add Time Window'}</span>
            </button>
          </div>

          {/* Slots List */}
          <div className="space-y-2.5">
            {formData.slots.map((slot, index) => (
              <div key={slot.id || index} className="p-3 bg-white dark:bg-[#191B1F] rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] shadow-xs space-y-2.5">
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
                  <div className="flex items-center space-x-2 flex-1">
                    <span className="w-5 h-5 rounded-full bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] text-[10px] font-bold flex items-center justify-center shrink-0">
                      {index + 1}
                    </span>
                    <input
                      type="text"
                      placeholder={isZh ? '时段名称 (选填)' : 'Slot name (optional)'}
                      value={slot.name}
                      onChange={(e) => handleUpdateSlot(slot.id, 'name', e.target.value)}
                      className="w-28 ui-field text-xs"
                    />
                  </div>

                  <div className="flex items-center space-x-2">
                    <input
                      type="text"
                      placeholder="00:00"
                      value={slot.start}
                      onChange={(e) => handleUpdateSlot(slot.id, 'start', e.target.value)}
                      className="w-18 ui-field font-mono text-center text-xs"
                    />
                    <span className="text-[#656B75] dark:text-[#A1A7B0] text-xs">{isZh ? '至' : 'to'}</span>
                    <input
                      type="text"
                      placeholder="08:30"
                      value={slot.end}
                      onChange={(e) => handleUpdateSlot(slot.id, 'end', e.target.value)}
                      className="w-18 ui-field font-mono text-center text-xs"
                    />
                  </div>

                  <div className="flex items-center space-x-2">
                    <select
                      value={slot.discount}
                      onChange={(e) => handleUpdateSlot(slot.id, 'discount', parseFloat(e.target.value))}
                      className="ui-field text-xs font-mono"
                    >
                      <option value="0.1">{isZh ? '1 折 (10%)' : '90% Off (10%)'}</option>
                      <option value="0.2">{isZh ? '2 折 (20%)' : '80% Off (20%)'}</option>
                      <option value="0.3">{isZh ? '3 折 (30%)' : '70% Off (30%)'}</option>
                      <option value="0.4">{isZh ? '4 折 (40%)' : '60% Off (40%)'}</option>
                      <option value="0.5">{isZh ? '5 折 (50%)' : '50% Off (50%)'}</option>
                      <option value="0.6">{isZh ? '6 折 (60%)' : '40% Off (60%)'}</option>
                      <option value="0.7">{isZh ? '7 折 (70%)' : '30% Off (70%)'}</option>
                      <option value="0.8">{isZh ? '8 折 (80%)' : '20% Off (80%)'}</option>
                      <option value="0.85">{isZh ? '8.5 折 (85%)' : '15% Off (85%)'}</option>
                      <option value="0.9">{isZh ? '9 折 (90%)' : '10% Off (90%)'}</option>
                    </select>

                    {formData.slots.length > 1 && (
                      <button
                        type="button"
                        onClick={() => handleRemoveSlot(slot.id)}
                        className="p-1.5 text-[#656B75] hover:text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-950/40 rounded-lg transition cursor-pointer"
                        title={isZh ? '删除该时段' : 'Delete slot'}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                </div>

                {/* Zero-burden Weekday Selector */}
                <div className="flex items-center justify-between pt-1.5 border-t border-[#E4E6E9] dark:border-[#2C3038] text-xs">
                  <span className="ui-meta">{isZh ? '生效周期:' : 'Active on:'}</span>
                  <div className="flex items-center space-x-1">
                    {[
                      { label: isZh ? '一' : 'M', val: 1 },
                      { label: isZh ? '二' : 'T', val: 2 },
                      { label: isZh ? '三' : 'W', val: 3 },
                      { label: isZh ? '四' : 'T', val: 4 },
                      { label: isZh ? '五' : 'F', val: 5 },
                      { label: isZh ? '六' : 'S', val: 6 },
                      { label: isZh ? '日' : 'S', val: 7 }
                    ].map(({ label, val }) => {
                      const isActive = isDayActive(slot.days, val);
                      return (
                        <button
                          key={val}
                          type="button"
                          onClick={() => toggleSlotDay(slot.id, val)}
                          className={`w-6 h-6 rounded-md text-[11px] font-medium transition cursor-pointer flex items-center justify-center ${
                            isActive
                              ? 'bg-[#355CFF] text-white font-semibold shadow-xs'
                              : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#E4E6E9] dark:hover:bg-[#2C3038]'
                          }`}
                          title={isZh ? `周${label}` : `Day ${val}`}
                        >
                          {label}
                        </button>
                      );
                    })}
                    <span className="text-[11px] text-[#355CFF] dark:text-[#91A7FF] font-medium ml-1.5">
                      {formatDaysLabel(slot.days, isZh) || (isZh ? '每天' : 'Everyday')}
                    </span>
                  </div>
                </div>
              </div>
            ))}
          </div>

          <div className="pt-1">
            <label className="flex items-center space-x-2 text-[#202124] dark:text-[#F1F2F4] cursor-pointer">
              <input
                type="checkbox"
                checked={formData.weekend_all_day}
                onChange={(e) => setFormData({ ...formData, weekend_all_day: e.target.checked })}
                className="rounded border-[#E4E6E9] dark:border-[#2C3038] text-[#355CFF] focus:ring-[#355CFF]/30"
              />
              <span className="font-medium">{isZh ? '周六与周日全天享受优惠折扣' : 'All-day discount on Saturdays and Sundays'}</span>
            </label>
          </div>
        </div>
      )}
    </div>
  );
}
