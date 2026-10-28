// Internationalization (i18n) Dictionary & Context for Airoute
// Clean, professional, product-first terminology for both Chinese and English

import { createContext } from 'react';

export const translations = {
  zh: {
    // Top Header & User
    langToggle: 'EN',

    // Nav Items (concise, natural 2-4 chars)
    navDashboard: '概览',
    navModels: '模型路由',
    navPricing: '模型定价',
    navChannels: '渠道管理',
    navReconciliation: '供应商对账',
    navKeys: 'API 密钥',
    navWallet: '我的钱包',
    navLogs: '调用日志',
    navSkills: 'Skill Hub',
    navMcp: 'MCP 广场',
    navUsers: '用户管理',
    navPlayground: '调试演练',
    navDocs: '开发文档',
    navViewDocs: '开发文档 ↗',

    // Dashboard Overview & SLA Cockpit

    // Status Page
    statusDegraded: '部分模型延迟偏高或降级',
    statusRefresh: '刷新状态',
    statusOperationalBadge: '运行正常',
    statusActivePool: '健康渠道提供商',
    statusCircuitBreakers: '上游提供商熔断与探针矩阵',
    statusChannelName: '渠道名称',
    statusChannelType: '协议类型',
    statusBreakerState: '熔断器状态',
    statusProbeLatency: '实时探测延迟',
    statusStateClosed: '健康闭合 (正常分发)',
    statusStateOpen: '已熔断跳闸 (旁路跳过)',

    // Playground 2.0

    // Onboarding Wizard

    // Visual Topology Modal

    // Docs View

    // Model Routes Manager

    // Channels Manager

    // Keys Manager

    // Plaza (Skill & MCP Hub)

    // Logs

    // Common
    // Key creation (server-owned tenant, self-service quota policy)
    keyTenantLabel: '租户标识 (Tenant)',
    keyTenantAdminHint: '管理员可指定该密钥归属的租户标识，将用于用量与账单归属。',
    keyTenantServerAssigned: '归属租户（由服务器分配）',
    keyTenantServerAssignedHint: '个人密钥自动归属您的账号，无需也无法自行填写。',
    keyRpmLabel: '速率上限 (RPM)',
    keyRpmAdminHint: '0 表示不限速（仅管理员可创建不限速密钥）。',
    keyRpmSelfServiceHint: '默认 60 RPM，自助创建最高 6000 RPM；如需不限速请联系管理员。',
    keyTpmLabel: 'Token 上限 (TPM)',
    keyTpmAdminHint: '0 表示不限量（仅管理员可创建不限量密钥）。',
    keyTpmSelfServiceHint: '默认 100,000 TPM，自助创建最高 10,000,000 TPM；如需更高请联系管理员。',
    keyQuotaClamped: '已按自助密钥上限调整速率/Token 配额',
    keyCreatedTitle: '密钥已创建',
    keyCreatedOnceWarning: '这是该密钥唯一一次显示明文，请立即复制并妥善保存；关闭后无法再次查看。',
    keyIdLabel: '密钥 ID',
    keyIdHint: '密钥 ID 仅用于在日志、账单和工单中识别密钥，不能用于鉴权；明文仅在创建时显示一次。',
    keyDismissCreated: '我已保存，关闭',
    keySecretUnavailable: '密钥明文仅在创建时显示一次',
  },

  en: {
    // Top Header & User
    langToggle: '中文',

    // Nav Items (concise)
    navDashboard: 'Overview',
    navModels: 'Routing',
    navPricing: 'Pricing',
    navChannels: 'Channels',
    navReconciliation: 'Reconciliation',
    navKeys: 'API Keys',
    navWallet: 'Wallet',
    navLogs: 'Logs',
    navSkills: 'Skill Hub',
    navMcp: 'MCP Hub',
    navUsers: 'Users',
    navPlayground: 'Playground',
    navDocs: 'Docs',
    navViewDocs: 'Docs ↗',

    // Dashboard Overview & SLA Cockpit

    // Status Page
    statusDegraded: 'Degraded on some models',
    statusRefresh: 'Refresh Status',
    statusOperationalBadge: 'Operational',
    statusActivePool: 'Healthy Providers',
    statusCircuitBreakers: 'Provider Health & Circuit Breaker Grid',
    statusChannelName: 'Channel Name',
    statusChannelType: 'Protocol Type',
    statusBreakerState: 'Breaker State',
    statusProbeLatency: 'Probe Latency',
    statusStateClosed: 'Closed (Healthy)',
    statusStateOpen: 'Open (Bypassed)',

    // Playground 2.0

    // Onboarding Wizard

    // Visual Topology Modal

    // Docs View

    // Model Routes Manager

    // Channels Manager

    // Keys Manager

    // Plaza (Skill & MCP Hub)

    // Logs

    // Common
    // Key creation (server-owned tenant, self-service quota policy)
    keyTenantLabel: 'Tenant identifier',
    keyTenantAdminHint: 'Administrators may set which tenant the key belongs to; usage and billing are attributed to it.',
    keyTenantServerAssigned: 'Tenant (assigned by the server)',
    keyTenantServerAssignedHint: 'A personal key always belongs to your account; this cannot be chosen.',
    keyRpmLabel: 'Rate limit (RPM)',
    keyRpmAdminHint: '0 means unlimited — only an administrator can create an unlimited key.',
    keyRpmSelfServiceHint: 'Defaults to 60 RPM; self-service keys cap at 6000 RPM. Contact an administrator for unlimited.',
    keyTpmLabel: 'Token limit (TPM)',
    keyTpmAdminHint: '0 means unlimited — only an administrator can create an unlimited key.',
    keyTpmSelfServiceHint: 'Defaults to 100,000 TPM; self-service keys cap at 10,000,000 TPM. Contact an administrator for more.',
    keyQuotaClamped: 'Rate/token quotas were adjusted to the self-service limits',
    keyCreatedTitle: 'Key created',
    keyCreatedOnceWarning: 'This is the only time the plaintext key is shown. Copy and store it now; it cannot be viewed again.',
    keyIdLabel: 'Key ID',
    keyIdHint: 'The key ID identifies the key in logs, billing and support tickets; it cannot authenticate a call. The plaintext is shown once, at creation.',
    keyDismissCreated: 'Saved — close',
    keySecretUnavailable: 'The plaintext key is shown once, at creation',
  },
};

// React Context for global i18n support
export const I18nContext = createContext({
  lang: 'zh',
  setLang: () => {},
  toggleLang: () => {},
  t: translations.zh,
});
