import { useState, useEffect } from 'react';
import { Megaphone, X, Check, AlertTriangle, AlertCircle, Info, Eye } from 'lucide-react';
import { apiFetch } from '../api/client';

export default function AnnouncementModal({
  isOpen,
  onClose,
  adminFetch,
  showToast,
  lang = 'zh',
  onPublished
}) {
  const isZh = lang === 'zh';
  const [submitting, setSubmitting] = useState(false);
  const [enabled, setEnabled] = useState(false);
  const [level, setLevel] = useState('info');
  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    const fetchConfig = async () => {
      try {
        const fetcher = adminFetch || apiFetch;
        const res = await fetcher('/api/v1/admin/announcement/config');
        if (res && res.ok) {
          const json = await res.json();
          if (json && json.code === 0 && json.data) {
            const d = json.data;
            setEnabled(!!d.enabled);
            setLevel(d.level || 'info');
            setTitle(d.title || '');
            setContent(d.content || '');
          }
        }
      } catch (e) {
        showToast?.(isZh ? '加载公告配置失败' : 'Failed to load announcement config', 'error');
      }
    };
    fetchConfig();
  }, [isOpen, adminFetch, showToast, isZh]);

  if (!isOpen) return null;

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (enabled && (!title.trim() || !content.trim())) {
      showToast?.(isZh ? '开启公告时标题和内容不可为空' : 'Title and content are required when enabling', 'error');
      return;
    }

    setSubmitting(true);
    try {
      const fetcher = adminFetch || apiFetch;
      const res = await fetcher('/api/v1/admin/announcement', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title: title.trim(),
          content: content.trim(),
          level,
          enabled
        })
      });

      if (res && res.ok) {
        showToast?.(
          enabled
            ? (isZh ? '公告已成功发布并生效' : 'Announcement published successfully')
            : (isZh ? '公告已下线保存' : 'Announcement saved and unpublished'),
          'success'
        );
        onPublished?.();
        onClose();
      } else {
        const errJson = await res.json().catch(() => ({}));
        showToast?.(errJson.error || (isZh ? '保存公告失败' : 'Failed to save announcement'), 'error');
      }
    } catch (e) {
      showToast?.(e.message || (isZh ? '网络请求异常' : 'Network request error'), 'error');
    } finally {
      setSubmitting(false);
    }
  };

  const levelStyles = {
    info: {
      bar: 'bg-indigo-50/90 dark:bg-indigo-950/40 border-indigo-200/80 dark:border-indigo-900/60 text-indigo-900 dark:text-indigo-100',
      badge: 'bg-indigo-100 dark:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 border-indigo-200 dark:border-indigo-800',
      icon: Info,
      iconColor: 'text-indigo-600 dark:text-indigo-400',
      label: isZh ? '系统公告' : 'Announcement'
    },
    warning: {
      bar: 'bg-amber-50/90 dark:bg-amber-950/40 border-amber-200/80 dark:border-amber-900/60 text-amber-950 dark:text-amber-100',
      badge: 'bg-amber-100 dark:bg-amber-900/60 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800',
      icon: AlertTriangle,
      iconColor: 'text-amber-600 dark:text-amber-400',
      label: isZh ? '重要提醒' : 'Notice'
    },
    critical: {
      bar: 'bg-rose-50/90 dark:bg-rose-950/40 border-rose-200/80 dark:border-rose-900/60 text-rose-950 dark:text-rose-100',
      badge: 'bg-rose-100 dark:bg-rose-900/60 text-rose-700 dark:text-rose-300 border-rose-200 dark:border-rose-800',
      icon: AlertCircle,
      iconColor: 'text-rose-600 dark:text-rose-400',
      label: isZh ? '紧急公告' : 'Urgent'
    }
  };

  const currentLevelStyle = levelStyles[level] || levelStyles.info;
  const LevelIcon = currentLevelStyle.icon;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm animate-in fade-in duration-150">
      <div className="bg-white dark:bg-[#191B1F] rounded-2xl max-w-xl w-full border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xl overflow-hidden flex flex-col">
        {/* Header */}
        <div className="px-6 py-4 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between">
          <div className="flex items-center space-x-2.5">
            <div className="p-2 rounded-xl bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF]">
              <Megaphone className="w-4 h-4" />
            </div>
            <div>
              <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
                {isZh ? '系统级全员公告发布' : 'Publish System Announcement'}
              </h3>
              <p className="ui-meta mt-0.5">
                {isZh ? '发布后将在全平台所有用户界面顶部横幅置顶展示' : 'Broadcast banner shown on top for all users across the platform'}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-lg text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSubmit} className="p-6 space-y-5">
          {/* Status Toggle */}
          <div className="flex items-center justify-between p-3.5 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
            <div>
              <span className="text-xs font-semibold text-slate-900 dark:text-white block">
                {isZh ? '公告展示状态' : 'Announcement Status'}
              </span>
              <span className="ui-meta mt-0.5">
                {enabled
                  ? (isZh ? '当前状态：已开启并在全站置顶展示' : 'Status: Enabled and broadcasting on top')
                  : (isZh ? '当前状态：已关闭（全站隐藏，保留草稿）' : 'Status: Disabled (hidden from users)')}
              </span>
            </div>
            <button
              type="button"
              onClick={() => setEnabled(!enabled)}
              className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${
                enabled ? 'bg-indigo-600' : 'bg-slate-300 dark:bg-slate-700'
              }`}
            >
              <span
                className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out ${
                  enabled ? 'translate-x-5' : 'translate-x-0'
                }`}
              />
            </button>
          </div>

          {/* Level Selection */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-700 dark:text-slate-300">
              {isZh ? '公告级别' : 'Severity Level'}
            </label>
            <div className="grid grid-cols-3 gap-2.5">
              {[
                { id: 'info', label: isZh ? '普通通知 (Info)' : 'Info', color: 'border-indigo-300 dark:border-indigo-700 text-indigo-700 dark:text-indigo-300 bg-indigo-50/50 dark:bg-indigo-950/30' },
                { id: 'warning', label: isZh ? '重要提醒 (Warn)' : 'Warning', color: 'border-amber-300 dark:border-amber-700 text-amber-700 dark:text-amber-300 bg-amber-50/50 dark:bg-amber-950/30' },
                { id: 'critical', label: isZh ? '紧急公告 (Alert)' : 'Critical', color: 'border-rose-300 dark:border-rose-700 text-rose-700 dark:text-rose-300 bg-rose-50/50 dark:bg-rose-950/30' },
              ].map((lvl) => (
                <button
                  type="button"
                  key={lvl.id}
                  onClick={() => setLevel(lvl.id)}
                  className={`p-2.5 rounded-xl border text-xs font-medium transition cursor-pointer flex items-center justify-center space-x-1.5 ${
                    level === lvl.id
                      ? `${lvl.color} ring-2 ring-indigo-500/20 font-semibold shadow-sm`
                      : 'border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-600 dark:text-slate-400'
                  }`}
                >
                  {level === lvl.id && <Check className="w-3.5 h-3.5 shrink-0" />}
                  <span>{lvl.label}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Title */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-700 dark:text-slate-300">
              {isZh ? '公告标题' : 'Announcement Title'}
            </label>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder={isZh ? '例如：双活集群网络升级与维护预告' : 'e.g. Scheduled Network Upgrade Notice'}
              className="ui-field text-xs w-full"
            />
          </div>

          {/* Content */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-700 dark:text-slate-300">
              {isZh ? '公告详细内容' : 'Announcement Content'}
            </label>
            <textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              rows={3}
              placeholder={isZh ? '例如：我们将于今晚 24:00 进行节点路由升级，期间部分模型可能产生抖动，请提前调整。' : 'Details shown to users across the platform...'}
              className="ui-field text-xs w-full resize-none"
            />
          </div>

          {/* Live Preview */}
          <div className="space-y-1.5">
            <div className="flex items-center space-x-1.5 text-xs text-slate-500 dark:text-slate-400">
              <Eye className="w-3.5 h-3.5" />
              <span>{isZh ? '用户界面实时预览' : 'Live Preview Banner'}</span>
            </div>
            <div className={`p-3 rounded-xl border text-xs flex items-center justify-between gap-3 ${currentLevelStyle.bar}`}>
              <div className="flex items-center gap-2 flex-1 min-w-0">
                <LevelIcon className={`w-4 h-4 shrink-0 ${currentLevelStyle.iconColor}`} />
                <span className={`px-1.5 py-0.5 rounded text-[10px] font-semibold tracking-wide border shrink-0 ${currentLevelStyle.badge}`}>
                  {currentLevelStyle.label}
                </span>
                <span className="font-semibold shrink-0">{title || (isZh ? '公告标题' : 'Title')}:</span>
                <span className="truncate opacity-90">{content || (isZh ? '公告详细内容将展示在此处...' : 'Content details...')}</span>
              </div>
              <X className="w-3.5 h-3.5 text-slate-400 shrink-0" />
            </div>
          </div>

          {/* Actions */}
          <div className="pt-2 flex items-center justify-end space-x-2">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="ui-btn-primary ui-btn-sm"
            >
              {submitting
                ? (isZh ? '保存中...' : 'Saving...')
                : enabled
                ? (isZh ? '发布公告并置顶' : 'Publish Announcement')
                : (isZh ? '保存草稿并下线' : 'Save & Unpublish')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
