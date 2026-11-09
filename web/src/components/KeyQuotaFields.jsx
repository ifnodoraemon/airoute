import {
  SELF_SERVICE_DEFAULT_RPM,
  SELF_SERVICE_MAX_RPM,
  SELF_SERVICE_DEFAULT_TPM,
  SELF_SERVICE_MAX_TPM,
} from '../hooks/useKeyActions';

function quotaFields(isAdmin, isZh, t) {
  const unlimited = isZh ? '不限' : 'Unlimited';
  const rpmLabel = t?.keyRpmLabel || (isZh ? '速率上限 (RPM)' : 'Rate limit (RPM)');
  const tpmLabel = t?.keyTpmLabel || (isZh ? 'Token 上限 (TPM)' : 'Token limit (TPM)');
  return [
    {
      name: 'budget', label: isZh ? '额度限制 (CNY, 0 为不限)' : 'Quota Limit (CNY, 0 for unlimited)',
      min: 0, step: '0.01', presets: [[unlimited, 0], ['¥10', 10], ['¥50', 50], ['¥100', 100]],
    },
    {
      name: 'rpm', label: isAdmin ? rpmLabel : `${rpmLabel} (${SELF_SERVICE_DEFAULT_RPM}–${SELF_SERVICE_MAX_RPM})`,
      min: isAdmin ? 0 : 1, max: isAdmin ? undefined : SELF_SERVICE_MAX_RPM, step: '1',
      presets: isAdmin ? [[unlimited, 0], ['60', 60], ['120', 120], ['600', 600]] : [['60', 60], ['120', 120], ['600', 600], ['6000', 6000]],
      hint: isAdmin
        ? (t?.keyRpmAdminHint || (isZh ? '0 表示不限速（仅管理员可创建不限速密钥）。' : '0 means unlimited — only an administrator can create an unlimited key.'))
        : (t?.keyRpmSelfServiceHint || (isZh ? '默认 60 RPM，自助创建最高 6000 RPM；如需不限速请联系管理员。' : 'Defaults to 60 RPM; self-service keys cap at 6000 RPM. Contact an administrator for unlimited.')),
    },
    {
      name: 'tpm', label: isAdmin ? tpmLabel : `${tpmLabel} (${SELF_SERVICE_DEFAULT_TPM}–${SELF_SERVICE_MAX_TPM})`,
      min: isAdmin ? 0 : 1, max: isAdmin ? undefined : SELF_SERVICE_MAX_TPM, step: '1',
      presets: isAdmin ? [[unlimited, 0], ['100K', 100000], ['1M', 1000000]] : [['100K', 100000], ['1M', 1000000], ['10M', SELF_SERVICE_MAX_TPM]],
      hint: isAdmin
        ? (t?.keyTpmAdminHint || (isZh ? '0 表示不限量（仅管理员可创建不限量密钥）。' : '0 means unlimited — only an administrator can create an unlimited key.'))
        : (t?.keyTpmSelfServiceHint || (isZh ? '默认 100,000 TPM，自助创建最高 10,000,000 TPM；如需更高请联系管理员。' : 'Defaults to 100,000 TPM; self-service keys cap at 10,000,000 TPM. Contact an administrator for more.')),
    },
  ];
}

function QuotaField({ field, newKey, setNewKey }) {
  const setValue = (value) => setNewKey((prev) => ({ ...prev, [field.name]: value }));
  const parse = field.name === 'budget' ? parseFloat : (value) => parseInt(value, 10);
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <label className="font-semibold text-[#202124] dark:text-[#F1F2F4]">{field.label}</label>
        <div className="flex items-center space-x-1.5 text-xs">
          {field.presets.map(([label, value]) => (
            <button key={label} type="button" onClick={() => setValue(value)} className="text-[#355CFF] dark:text-[#91A7FF] hover:underline px-1 cursor-pointer">
              {label}
            </button>
          ))}
        </div>
      </div>
      <input type="number" min={field.min} max={field.max} step={field.step} value={newKey[field.name] ?? 0}
        onChange={(e) => setValue(parse(e.target.value) || 0)} className="ui-field font-mono" />
      {field.hint && <p className="ui-meta-sm mt-1">{field.hint}</p>}
    </div>
  );
}

export default function KeyQuotaFields({ newKey, setNewKey, isAdmin, isZh, t }) {
  return quotaFields(isAdmin, isZh, t).map((field) => (
    <QuotaField key={field.name} field={field} newKey={newKey} setNewKey={setNewKey} />
  ));
}
