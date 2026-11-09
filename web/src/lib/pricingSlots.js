

export function formatDaysLabel(days, isZh = true) {
  if (!days || days.length === 0 || days.length === 7) return '';
  const dayNames = isZh
    ? { 1: '周一', 2: '周二', 3: '周三', 4: '周四', 5: '周五', 6: '周六', 7: '周日' }
    : { 1: 'Mon', 2: 'Tue', 3: 'Wed', 4: 'Thu', 5: 'Fri', 6: 'Sat', 7: 'Sun' };
  const sorted = [...days].sort((a, b) => a - b);
  if (sorted.length === 5 && sorted.every((d, i) => d === i + 1)) return isZh ? '工作日' : 'Weekdays';
  if (sorted.length === 2 && sorted[0] === 6 && sorted[1] === 7) return isZh ? '周末' : 'Weekends';
  return sorted.map(d => dayNames[d] || d).join(isZh ? '、' : ', ');
}

export function parseMinutes(t) {
  if (!t) return null;
  const parts = t.trim().split(':');
  if (parts.length !== 2) return null;
  const h = parseInt(parts[0], 10);
  const m = parseInt(parts[1], 10);
  if (isNaN(h) || isNaN(m) || h < 0 || h > 24 || m < 0 || m > 59) return null;
  return h * 60 + m;
}

export function checkSlotsOverlap(slots, isZh = true) {
  if (!slots || slots.length < 2) return null;
  const dayNames = isZh
    ? { 1: '周一', 2: '周二', 3: '周三', 4: '周四', 5: '周五', 6: '周六', 7: '周日' }
    : { 1: 'Mon', 2: 'Tue', 3: 'Wed', 4: 'Thu', 5: 'Fri', 6: 'Sat', 7: 'Sun' };

  for (let i = 0; i < slots.length; i++) {
    const s1 = slots[i];
    const s1Start = parseMinutes(s1.start);
    const s1End = parseMinutes(s1.end);
    if (s1Start === null || s1End === null) {
      return isZh ? `时段「${s1.name || i + 1}」时间格式无效 (格式需为 HH:MM)` : `Slot "${s1.name || i + 1}" format invalid (must be HH:MM)`;
    }
    if (s1Start === s1End) {
      return isZh ? `时段「${s1.name || i + 1}」开始时间与结束时间不能相同` : `Slot "${s1.name || i + 1}" start and end time cannot be the same`;
    }

    const segs1 = s1Start < s1End
      ? [{ s: s1Start, e: s1End }]
      : [{ s: s1Start, e: 1440 }, { s: 0, e: s1End }];

    const days1 = (s1.days && s1.days.length > 0) ? s1.days : [1, 2, 3, 4, 5, 6, 7];

    for (let j = i + 1; j < slots.length; j++) {
      const s2 = slots[j];
      const s2Start = parseMinutes(s2.start);
      const s2End = parseMinutes(s2.end);
      if (s2Start === null || s2End === null) continue;
      if (s2Start === s2End) {
        return isZh ? `时段「${s2.name || j + 1}」开始时间与结束时间不能相同` : `Slot "${s2.name || j + 1}" start and end time cannot be the same`;
      }

      const days2 = (s2.days && s2.days.length > 0) ? s2.days : [1, 2, 3, 4, 5, 6, 7];
      const commonDays = days1.filter(d => days2.includes(d));
      if (commonDays.length === 0) {
        continue;
      }

      const segs2 = s2Start < s2End
        ? [{ s: s2Start, e: s2End }]
        : [{ s: s2Start, e: 1440 }, { s: 0, e: s2End }];

      for (const seg1 of segs1) {
        for (const seg2 of segs2) {
          if (Math.max(seg1.s, seg2.s) < Math.min(seg1.e, seg2.e)) {
            const overlapDaysStr = commonDays.length === 7 ? (isZh ? '每天' : 'Everyday') : commonDays.map(d => dayNames[d] || d).join(isZh ? '、' : ', ');
            return isZh
              ? `时段「${s1.name || i + 1}」(${s1.start}-${s1.end}) 与 时段「${s2.name || j + 1}」(${s2.start}-${s2.end}) 在【${overlapDaysStr}】存在时间重叠，请调整时段避免冲突`
              : `Slot "${s1.name || i + 1}" (${s1.start}-${s1.end}) overlaps with slot "${s2.name || j + 1}" (${s2.start}-${s2.end}) on [${overlapDaysStr}]. Please adjust to resolve.`;
          }
        }
      }
    }
  }
  return null;
}

export function parseSlotsFromPrice(p, isZh = true) {
  if (p && p.off_peak_slots) {
    try {
      const parsed = JSON.parse(p.off_peak_slots);
      if (Array.isArray(parsed) && parsed.length > 0) {
        return parsed.map((s, idx) => ({
          id: `slot-${idx + 1}-${Date.now()}`,
          name: s.name || (isZh ? `时段 ${idx + 1}` : `Slot ${idx + 1}`),
          start: s.start || '00:00',
          end: s.end || '08:30',
          discount: s.discount !== undefined ? parseFloat(s.discount) : 0.5,
          days: Array.isArray(s.days) ? s.days : []
        }));
      }
    } catch (e) {}
  }
  if (p && (p.off_peak_start || p.off_peak_end)) {
    return [{
      id: `slot-1-${Date.now()}`,
      name: isZh ? '时段 1' : 'Slot 1',
      start: p.off_peak_start || '00:00',
      end: p.off_peak_end || '08:30',
      discount: p.off_peak_discount ?? 0.5,
      days: []
    }];
  }
  return [
    { id: `slot-1-${Date.now()}`, name: isZh ? '时段 1' : 'Slot 1', start: '00:00', end: '08:30', discount: 0.5, days: [] }
  ];
}
