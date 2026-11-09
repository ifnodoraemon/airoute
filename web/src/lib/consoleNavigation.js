import { Server, Key, Terminal, Cpu, Sparkles, History, BarChart3, ShieldCheck, DollarSign, Coins, Wallet, Bell, BookOpen } from 'lucide-react';

export const CONSOLE_NAV = {
  dashboard: { paletteLabel: (isZh) => isZh ? '用量与业务概览大盘' : 'Telemetry & SLA Cockpit', paletteAdminOnly: true, icon: BarChart3, label: (t) => t.navDashboard },
  // The loaded-row badge has always been an admin-only detail; the regular-user
  // sidebar rendered logs without it.
  logs: { paletteLabel: (isZh) => isZh ? '实时调用日志与对话审计' : 'Audit Logs & Traces', paletteAdminOnly: false, icon: History, label: (t) => t.navLogs, count: (d, isAdmin) => (isAdmin ? d.logs : 0) },
  models: { paletteLabel: (isZh) => isZh ? '模型路由与拓扑' : 'Model Routes & Topo', paletteAdminOnly: true, icon: Cpu, label: (t) => t.navModels, count: (d) => d.models },
  channels: { paletteLabel: (isZh) => isZh ? '上游服务商接入与体检' : 'Upstream Channels & Health', paletteAdminOnly: true, icon: Server, label: (t) => t.navChannels, count: (d) => d.channels },
  reconciliation: { title: (t, isZh) => t.navReconciliation || (isZh ? '供应商对账' : 'Reconciliation'), paletteLabel: (isZh) => isZh ? '供应商整体对账与结算' : 'Supplier Reconciliation & Settlement', paletteAdminOnly: true, icon: Coins, label: (t) => t.navReconciliation },
  pricing: { paletteLabel: (isZh) => isZh ? '模型费率与阶梯定价' : 'Model Pricing & Rates', paletteAdminOnly: true, icon: DollarSign, label: (t) => t.navPricing },
  wallet: { paletteLabel: (isZh, isAdmin) => isAdmin ? (isZh ? '卡密生成与充值中心' : 'Card Codes & Billing') : (isZh ? '我的钱包与充值卡密' : 'Wallet & Top-up'), paletteAdminOnly: false,
    icon: Wallet,
    label: (t, isZh, isAdmin) =>
      isAdmin ? (isZh ? '卡密与充值' : 'Wallet & Credits') : (t.navWallet || (isZh ? '我的钱包' : 'My Wallet')),
  },
  keys: { paletteLabel: (isZh) => isZh ? 'API 访问密钥管理' : 'API Key Management', paletteAdminOnly: false, icon: Key, label: (t) => t.navKeys, count: (d) => d.keys },
  users: { paletteLabel: (isZh) => isZh ? '用户管理' : 'User Management', paletteAdminOnly: true,
    icon: ShieldCheck,
    label: (t, isZh) => t.navUsers || (isZh ? '用户管理' : 'Users'),
    title: (t, isZh) => t.navUsers || (isZh ? '用户管理' : 'User Management'),
  },
  playground: { paletteLabel: (isZh) => isZh ? '多模态调试演练工作台' : 'Multimodal Playground', paletteAdminOnly: false, icon: Terminal, label: (t) => t.navPlayground },
  skills: { paletteLabel: (isZh) => isZh ? 'Skill Hub 智能体技能中心' : 'Skill Hub', paletteAdminOnly: true, icon: Sparkles, label: (t) => t.navSkills || 'Skill Hub' },
  mcp: { paletteLabel: (isZh) => isZh ? 'MCP 协议广场与连接中心' : 'MCP Plaza & Connectors', paletteAdminOnly: true, icon: Cpu, label: (t, isZh) => t.navMcp || (isZh ? 'MCP 广场' : 'MCP Hub') },
  notifications: { paletteLabel: (isZh) => isZh ? '全站公告与个人消息' : 'Announcements & personal messages', paletteAdminOnly: false, icon: Bell, label: (t, isZh) => isZh ? '公告与消息' : 'Announcements & messages', count: d => d.notifications },
  docs: { icon: BookOpen, label: (t) => t.navDocs },
};

const PALETTE_ORDER = ["notifications","dashboard","models","channels","reconciliation","pricing","users","skills","mcp","keys","wallet","logs","playground"];

export function consoleTabTitle(tab, t, isZh, isAdmin) {
  const item = CONSOLE_NAV[tab];
  return (item?.title || item?.label)?.(t, isZh, isAdmin) || '';
}

export function paletteNavigation(isZh, isAdmin, category, navigate) {
  return PALETTE_ORDER.filter(tab => isAdmin || !CONSOLE_NAV[tab].paletteAdminOnly).map(tab => ({
    id: 'nav-' + tab, category, icon: CONSOLE_NAV[tab].icon,
    label: CONSOLE_NAV[tab].paletteLabel(isZh, isAdmin), action: () => navigate(tab),
  }));
}
