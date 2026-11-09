export default function KeyPricingGroup({ newKey, setNewKey, pricingGroups, groupMultipliers, adminUser, isZh }) {
  const isAdmin = adminUser?.role === 'admin';
  const groups = [...new Set(['default', ...pricingGroups])].filter((group) => (
    isAdmin || group === 'default' || group === adminUser?.group_name
  ));
  const selected = newKey.group_name || 'default';
  const validSelection = groups.includes(selected);
  const groupLabel = (group) => {
    const multiplier = groupMultipliers[group] ?? 1;
    if (group === 'default') return isZh ? '默认组 (1.0x 官方基准)' : 'Default (1.0x)';
    return `${group} (${multiplier}x)`;
  };

  return (
    <div>
      <label htmlFor="new-key-pricing-group" className="ui-label text-xs">
        {isZh ? '计费分组 (Pricing Group)' : 'Pricing Group'}
      </label>
      <select
        id="new-key-pricing-group"
        required
        value={validSelection ? selected : ''}
        onChange={(e) => setNewKey({ ...newKey, group_name: e.target.value })}
        className="ui-field cursor-pointer"
      >
        {!validSelection && <option value="" disabled>{isZh ? '请选择已有分组' : 'Choose an existing group'}</option>}
        {groups.map((group) => <option key={group} value={group}>{groupLabel(group)}</option>)}
      </select>
      <p className="ui-meta-sm mt-1">
        {isAdmin
          ? (isZh ? '仅可选择已有计费分组；新增或修改分组请前往定价管理。' : 'Choose an existing group. Create or edit groups in Pricing.')
          : (isZh ? '可选择默认组或账号已获授权的已有分组。' : 'Choose the default group or an existing group assigned to your account.')}
      </p>
    </div>
  );
}
