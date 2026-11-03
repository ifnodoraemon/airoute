import { useState } from 'react';
import { copyToClipboard } from '../config';
import useModalA11y from '../hooks/useModalA11y';

// Account creation returns an initial key once. Keep the result in memory until
// the operator explicitly dismisses it, including when copying is unavailable.
export default function OneTimeKeyDialog({ credential, username, onClose, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState(false);
  useModalA11y(true, () => {});

  const copy = async () => {
    const ok = await copyToClipboard(credential);
    setCopied(ok);
    setCopyError(!ok);
  };

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-slate-900/60 p-4 backdrop-blur-sm">
      <section role="dialog" aria-modal="true" aria-labelledby="initial-key-title" className="ui-card w-full max-w-lg space-y-4 p-6 bg-white dark:bg-[#191B1F]">
        <h2 id="initial-key-title" className="text-base font-semibold">
          {isZh ? '账号已创建，请保存初始 API 密钥' : 'Account created — save the initial API key'}
        </h2>
        <p className="text-xs">{isZh ? '账号' : 'Account'}: <strong>{username}</strong></p>
        <p role="alert" className="text-xs text-amber-700 dark:text-amber-300">
          {isZh ? '明文仅显示这一次，关闭后无法再次查看。请保存后再继续。' : 'The plaintext is shown only once. Save it before continuing; it cannot be viewed again.'}
        </p>
        <code className="block select-all break-all rounded-lg bg-slate-100 dark:bg-slate-900 p-3 text-xs" data-testid="initial-api-key">{credential}</code>
        {copyError && <p role="alert" className="text-xs text-rose-600">{isZh ? '复制失败，请手动选中并复制密钥。' : 'Copy failed. Select and copy the key manually.'}</p>}
        <div className="flex justify-end gap-2">
          <button type="button" onClick={copy} className="ui-btn-secondary">{copied ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制 API 密钥' : 'Copy API key')}</button>
          <button type="button" onClick={onClose} className="ui-btn-primary">{isZh ? '我已保存，继续' : 'Saved — continue'}</button>
        </div>
      </section>
    </div>
  );
}
