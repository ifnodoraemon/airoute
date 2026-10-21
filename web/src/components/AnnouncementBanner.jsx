import { useState, useEffect, useCallback } from 'react';
import { Megaphone, AlertTriangle, AlertCircle, Info, X } from 'lucide-react';
import { apiFetch } from '../api/client';

export default function AnnouncementBanner({ lang = 'zh', adminFetch }) {
  const isZh = lang === 'zh';
  const [announcement, setAnnouncement] = useState(null);
  const [dismissed, setDismissed] = useState(false);

  const fetchAnnouncement = useCallback(async () => {
    try {
      const fetcher = adminFetch || apiFetch;
      const res = await fetcher('/api/v1/announcement');
      if (res && res.ok) {
        const json = await res.json();
        if (json && json.code === 0 && json.data && json.data.enabled) {
          const ann = json.data;
          setAnnouncement(ann);
          // Check if previously dismissed
          try {
            const savedDismissedId = localStorage.getItem('airoute_dismissed_announcement');
            if (savedDismissedId === ann.id) {
              setDismissed(true);
            } else {
              setDismissed(false);
            }
          } catch {
            setDismissed(false);
          }
        } else {
          setAnnouncement(null);
        }
      }
    } catch {
      // Quietly ignore
    }
  }, [adminFetch]);

  useEffect(() => {
    fetchAnnouncement();
    // Poll every 60s for new announcements
    const timer = setInterval(fetchAnnouncement, 60000);
    return () => clearInterval(timer);
  }, [fetchAnnouncement]);

  const handleDismiss = () => {
    if (announcement) {
      try {
        localStorage.setItem('airoute_dismissed_announcement', announcement.id);
      } catch {}
      setDismissed(true);
    }
  };

  if (!announcement || dismissed || !announcement.enabled) {
    return null;
  }

  const levels = {
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

  const style = levels[announcement.level] || levels.info;
  const LevelIcon = style.icon;

  return (
    <div className={`w-full border-b px-4 py-2.5 transition-all duration-200 ${style.bar}`}>
      <div className="max-w-[1536px] mx-auto flex items-center justify-between gap-3 text-xs">
        <div className="flex items-center gap-2.5 flex-1 min-w-0">
          <LevelIcon className={`w-4 h-4 shrink-0 ${style.iconColor}`} />
          <span className={`px-1.5 py-0.5 rounded text-[11px] font-semibold tracking-wide border shrink-0 ${style.badge}`}>
            {style.label}
          </span>
          <div className="flex items-baseline gap-2 truncate">
            {announcement.title && (
              <span className="font-semibold shrink-0">
                {announcement.title}:
              </span>
            )}
            <span className="truncate opacity-90">
              {announcement.content}
            </span>
          </div>
        </div>

        <button
          onClick={handleDismiss}
          className="p-1 rounded-md text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-black/5 dark:hover:bg-white/5 transition shrink-0 cursor-pointer"
          title={isZh ? '关闭公告（本条不再提示）' : 'Dismiss announcement'}
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  );
}
