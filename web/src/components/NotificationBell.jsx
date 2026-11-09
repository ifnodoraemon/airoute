import { Bell } from 'lucide-react';

export default function NotificationBell({ counts, onClick, lang = 'zh' }) {
  const count = counts?.unread_count || 0;
  const label = lang === 'zh' ? `消息中心${count ? `，${count} 条未读` : ''}` : `Message center${count ? `, ${count} unread` : ''}`;
  return (
    <button type="button" onClick={onClick} title={label} aria-label={label}
      className="relative p-2 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs cursor-pointer">
      <Bell className="w-4 h-4 text-[#656B75] dark:text-[#A1A7B0]" />
      {count > 0 && <span className="absolute -right-1.5 -top-1.5 min-w-4 px-1 h-4 rounded-full bg-rose-600 text-white text-[9px] font-semibold flex items-center justify-center">{count > 99 ? '99+' : count}</span>}
    </button>
  );
}
