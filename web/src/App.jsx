import React, { useState, useEffect, useRef, lazy, Suspense } from 'react';
import {
  Activity,
  Zap,
  Server,
  Key,
  Terminal,
  Shield,
  ShieldAlert,
  Send,
  Play,
  Trash2,
  Copy,
  Plus,
  ExternalLink,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Image as ImageIcon,
  Cpu,
  Layers,
  Sparkles,
  HelpCircle,
  ArrowRight,
  Volume2,
  Mic,
  Video,
  Download,
  FileAudio,
  MessageSquare,
  BookOpen,
  History,
  Sliders,
  Code,
  Search,
  Edit3,
  Check,
  Eye,
  EyeOff,
  Info,
  ChevronRight,
  X,
  Clock,
  BarChart3,
  Network,
  Globe,
  LogOut,
  User,
  Lock,
  ShieldCheck,
  DollarSign,
  Coins,
  Bot,
  Wallet,
  Sun,
  Moon,
  Menu,
  PanelLeftClose,
  PanelLeft,
  Split
} from 'lucide-react';
import Toast from './components/Toast';

// Lazy loaded views and heavy modals for code splitting and fast initial bundle loading
const QuickStartModal = lazy(() => import('./components/QuickStartModal'));
const LogDetailModal = lazy(() => import('./components/LogDetailModal'));
const CurlExportModal = lazy(() => import('./components/CurlExportModal'));
const ChannelModal = lazy(() => import('./components/ChannelModal'));
const KeyModal = lazy(() => import('./components/KeyModal'));
const LandingPage = lazy(() => import('./components/LandingPage'));
const AuthPage = lazy(() => import('./components/AuthPage'));
const LoginModal = lazy(() => import('./components/LoginModal'));
const CommandPalette = lazy(() => import('./components/CommandPalette'));
const AccountManageModal = lazy(() => import('./components/AccountManageModal'));
const ModelRoutesManager = lazy(() => import('./components/ModelRoutesManager'));
const ServiceStatus = lazy(() => import('./components/ServiceStatus'));
const PricingManager = lazy(() => import('./components/PricingManager'));
const SkillHubView = lazy(() => import('./components/SkillHubView'));
const McpIntegrationView = lazy(() => import('./components/McpIntegrationView'));
const UserManagementView = lazy(() => import('./components/UserManagementView'));
const WalletManagementView = lazy(() => import('./components/WalletManagementView'));
const ChannelsView = lazy(() => import('./components/ChannelsView'));
const SupplierReconciliationView = lazy(() => import('./components/SupplierReconciliationView'));
const KeysView = lazy(() => import('./components/KeysView'));
const LogsView = lazy(() => import('./components/LogsView'));
const DashboardView = lazy(() => import('./components/DashboardView'));
const PlaygroundView = lazy(() => import('./components/PlaygroundView'));
const DocsView = lazy(() => import('./components/DocsView'));
const OnboardingModal = lazy(() => import('./components/OnboardingModal'));
const VisualTopologyModal = lazy(() => import('./components/VisualTopologyModal'));
const AiCopilot = lazy(() => import('./components/AiCopilot'));

function ViewLoader() {
  return (
    <div className="flex flex-col items-center justify-center p-16 text-slate-400">
      <div className="w-7 h-7 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin mb-3"></div>
      <span className="text-xs font-medium tracking-wide">Loading module...</span>
    </div>
  );
}
import { translations, I18nContext } from './i18n';
import { getGatewayOrigin, setPublicGatewayUrl, copyToClipboard as crossOriginCopy } from './config';

// Audit-log pagination: server-side page size, kept in one place for the
// fetch limit, the offset stepping, and the page-number math in LogsView.
const LOG_PAGE_SIZE = 50;

export default function App() {
  // Language State (bilingual i18n)
  const [lang, setLang] = useState(() => localStorage.getItem('airoute_lang') || 'zh');
  const t = translations[lang] || translations.zh;
  const isZh = lang === 'zh';

  const toggleLang = () => {
    const nextLang = lang === 'zh' ? 'en' : 'zh';
    setLang(nextLang);
    localStorage.setItem('airoute_lang', nextLang);
  };

  // Theme State (Dark / Light)
  const [theme, setTheme] = useState(() => localStorage.getItem('airoute_theme') || 'light');

  const toggleTheme = () => {
    setTheme(prev => (prev === 'dark' ? 'light' : 'dark'));
  };

  // Synchronize theme to document element class & dynamic title
  useEffect(() => {
    const root = document.documentElement;
    if (theme === 'dark') {
      root.classList.add('dark');
      root.classList.remove('light');
    } else {
      root.classList.remove('dark');
      root.classList.add('light');
    }
    localStorage.setItem('airoute_theme', theme);
    document.title = lang === 'zh' ? 'Airoute · AI路由器' : 'Airoute';
  }, [theme, lang]);

  const [toast, setToast] = useState({ show: false, message: '', type: 'info' });
  const toastTimerRef = useRef(null);

  const showToast = (message, type = 'info') => {
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
    setToast({ show: true, message, type });
    toastTimerRef.current = setTimeout(() => {
      setToast(prev => ({ ...prev, show: false }));
    }, 3200);
  };

  const ALL_CONSOLE_TABS = [
    'dashboard', 'models', 'pricing', 'channels', 'reconciliation',
    'keys', 'wallet', 'logs', 'skills', 'mcp', 'users', 'playground', 'docs'
  ];

  const getStoredToken = () => {
    if (typeof window === 'undefined') return '';
    return localStorage.getItem('airoute_token') || localStorage.getItem('airoute_admin_token') || '';
  };

  const getStoredUser = () => {
    if (typeof window === 'undefined') return null;
    const raw = localStorage.getItem('airoute_user') || localStorage.getItem('airoute_admin_user');
    if (!raw) return null;
    try {
      return JSON.parse(raw);
    } catch {
      return null;
    }
  };

  const setStoredAuth = (token, user) => {
    if (typeof window === 'undefined') return;
    if (token) {
      localStorage.setItem('airoute_token', token);
    }
    if (user) {
      const userStr = typeof user === 'string' ? user : JSON.stringify(user);
      localStorage.setItem('airoute_user', userStr);
    }
  };

  const clearStoredAuth = () => {
    if (typeof window === 'undefined') return;
    localStorage.removeItem('airoute_token');
    localStorage.removeItem('airoute_admin_token');
    localStorage.removeItem('airoute_user');
    localStorage.removeItem('airoute_admin_user');
  };

  const getRouteFromURL = () => {
    if (typeof window === 'undefined') return { viewMode: 'landing', tab: 'dashboard', authTab: 'login' };
    const rawHash = window.location.hash.replace(/^#\/?/, '').trim().toLowerCase();
    const searchParams = new URLSearchParams(window.location.search);
    const tabParam = (searchParams.get('tab') || '').toLowerCase();
    const route = rawHash || tabParam || '';
    const token = getStoredToken();
    const savedUser = getStoredUser();
    let role = 'user';
    if (savedUser?.role) {
      role = savedUser.role;
    } else if (token) {
      role = 'user';
    }
    const defaultTab = role === 'admin' ? 'dashboard' : 'wallet';

    if (route === 'status' || route === 'service-status') {
      return { viewMode: 'status', tab: defaultTab, authTab: 'login' };
    }

    if (route === 'auth' || route === 'login') {
      return { viewMode: 'auth', tab: defaultTab, authTab: 'login' };
    }
    if (route === 'register') {
      return { viewMode: 'auth', tab: defaultTab, authTab: 'register' };
    }

    if (!route || route === 'landing' || route === 'home' || ['features', 'multimodal'].includes(route)) {
      return { viewMode: 'landing', tab: defaultTab, authTab: 'login' };
    }

    if (route === 'console') {
      if (token) {
        return { viewMode: 'console', tab: defaultTab, authTab: 'login' };
      } else {
        return { viewMode: 'auth', tab: defaultTab, authTab: 'login' };
      }
    }

    if (ALL_CONSOLE_TABS.includes(route)) {
      if (token) {
        const adminOnlyTabs = ['channels', 'reconciliation', 'users', 'skills', 'mcp', 'dashboard'];
        if (role !== 'admin' && adminOnlyTabs.includes(route)) {
          return { viewMode: 'console', tab: 'wallet', authTab: 'login' };
        }
        return { viewMode: 'console', tab: route, authTab: 'login' };
      } else {
        return { viewMode: 'landing', tab: defaultTab, authTab: 'login' };
      }
    }

    return { viewMode: 'landing', tab: defaultTab, authTab: 'login' };
  };

  const initialRoute = getRouteFromURL();

  // View mode: 'landing' (公共门户首页), 'console' (工作台), 'status' (对外状态页), 'auth' (认证页)
  const [viewMode, setViewMode] = useState(() => initialRoute.viewMode);

  // Account & Authentication state (Unified token & user based on role)
  const [adminToken, setAdminToken] = useState(() => getStoredToken());
  const [adminUser, setAdminUser] = useState(() => getStoredUser());
  const [showLoginModal, setShowLoginModal] = useState(false);
  const [showAccountModal, setShowAccountModal] = useState(false);
  const [accountModalTab, setAccountModalTab] = useState('profile');
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [showOnboardingModal, setShowOnboardingModal] = useState(false);
  const [showTopologyModal, setShowTopologyModal] = useState(false);
  const [showAiCopilot, setShowAiCopilot] = useState(false);
  const [copilotInitialQuery, setCopilotInitialQuery] = useState('');
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [authTab, setAuthTab] = useState(() => initialRoute.authTab);

  // Global Command Palette shortcut (Cmd+K / Ctrl+K)
  useEffect(() => {
    const handleGlobalKeyDown = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setShowCommandPalette(prev => !prev);
      } else if (e.key === 'Escape') {
        setShowCommandPalette(false);
      }
    };
    window.addEventListener('keydown', handleGlobalKeyDown);
    return () => window.removeEventListener('keydown', handleGlobalKeyDown);
  }, []);

  // Validate token on startup to prevent stale token UI issues
  useEffect(() => {
    const savedToken = getStoredToken();
    if (savedToken) {
      fetch('/api/v1/user/me', {
        headers: { 'Authorization': `Bearer ${savedToken}` }
      })
        .then(res => res.json())
        .then(data => {
          if (data.code === 0 && data.data) {
            setAdminUser(data.data);
            setStoredAuth(savedToken, data.data);
          } else {
            setAdminToken('');
            setAdminUser(null);
            clearStoredAuth();
            if (viewMode === 'console') {
              setViewMode('landing');
              setShowLoginModal(true);
            }
          }
        })
        .catch(() => {});
    }

    // Query gateway public status to resolve canonical external gateway URL
    fetch('/api/v1/public/status')
      .then(res => res.json())
      .then(data => {
        if (data && data.data && data.data.public_url) {
          setPublicGatewayUrl(data.data.public_url);
        }
      })
      .catch(() => {});
  }, []);

  // Authenticated fetch helper for Control Plane APIs
  const adminFetch = async (url, options = {}) => {
    const headers = { ...(options.headers || {}) };
    const effectiveToken = options.token || adminToken || getStoredToken();
    if (effectiveToken) {
      headers['Authorization'] = `Bearer ${effectiveToken}`;
    }
    try {
      const res = await fetch(url, { ...options, headers });
      if (res.status === 401 && viewMode === 'console') {
        if (effectiveToken) {
          setAdminToken('');
          setAdminUser(null);
          clearStoredAuth();
          setShowLoginModal(true);
          showToast(isZh ? '登录凭证已失效，请重新登录' : 'Session expired, please login again', 'warning');
        }
      }
      return res;
    } catch (e) {
      throw e;
    }
  };

  const handleLogout = () => {
    setAdminToken('');
    setAdminUser(null);
    clearStoredAuth();
    setViewMode('landing');
    showToast(isZh ? '已安全退出账号' : 'Successfully logged out', 'info');
  };

  const fetchUserProfile = async () => {
    const tok = adminToken || getStoredToken();
    if (!tok) return;
    try {
      const res = await fetch('/api/v1/user/me', {
        headers: { 'Authorization': `Bearer ${tok}` }
      });
      const data = await res.json();
      if (res.ok && data.code === 0 && data.data) {
        setAdminUser(data.data);
        setStoredAuth(tok, data.data);
      } else if (res.status === 401) {
        setAdminToken('');
        setAdminUser(null);
        clearStoredAuth();
        if (viewMode === 'console') {
          setViewMode('landing');
          setShowLoginModal(true);
          showToast(isZh ? '登录凭证已过期，请重新登录' : 'Session expired, please sign in again', 'warning');
        }
      }
    } catch (e) {}
  };

  const [activeQuickKey, setActiveQuickKey] = useState(null);
  const [activeLogDetail, setActiveLogDetail] = useState(null);
  const [curlExportCmd, setCurlExportCmd] = useState('');
  const [batchTesting, setBatchTesting] = useState(false);
  const [channelLatencies, setChannelLatencies] = useState({});

  const [currentTab, setCurrentTab] = useState(() => initialRoute.tab);

  // Synchronize browser address bar URL hash with active view and tab
  useEffect(() => {
    let targetHash = '';
    if (viewMode === 'landing') {
      if (window.location.hash && window.location.hash !== '#/' && window.location.hash !== '#/landing') {
        const curHash = window.location.hash.replace(/^#\/?/, '').trim();
        if (!['features', 'multimodal', 'docs', 'pricing'].includes(curHash)) {
          targetHash = '#/';
        }
      }
    } else if (viewMode === 'auth') {
      targetHash = authTab === 'register' ? '#/register' : '#/login';
    } else if (viewMode === 'status') {
      targetHash = '#/status';
    } else if (viewMode === 'console') {
      targetHash = `#/${currentTab}`;
    }

    if (targetHash && window.location.hash !== targetHash) {
      window.history.replaceState(null, '', targetHash);
    }
  }, [viewMode, currentTab, authTab]);

  // Listen to browser Back / Forward buttons (hashchange and popstate)
  useEffect(() => {
    const handleLocationChange = () => {
      const routeInfo = getRouteFromURL();
      setViewMode(prev => prev !== routeInfo.viewMode ? routeInfo.viewMode : prev);
      setCurrentTab(prev => prev !== routeInfo.tab ? routeInfo.tab : prev);
      setAuthTab(prev => prev !== routeInfo.authTab ? routeInfo.authTab : prev);
    };

    window.addEventListener('hashchange', handleLocationChange);
    window.addEventListener('popstate', handleLocationChange);
    return () => {
      window.removeEventListener('hashchange', handleLocationChange);
      window.removeEventListener('popstate', handleLocationChange);
    };
  }, []);

  // Automatically ensure regular user stays within permitted tabs
  useEffect(() => {
    if (adminUser && adminUser.role !== 'admin') {
      const allowedTabs = ['wallet', 'keys', 'playground', 'pricing', 'logs', 'docs', 'models'];
      if (!allowedTabs.includes(currentTab)) {
        setCurrentTab('wallet');
      }
    }
  }, [adminUser, currentTab]);

  const [stats, setStats] = useState({});
  const [channels, setChannels] = useState([]);
  const [keys, setKeys] = useState([]);
  const [models, setModels] = useState([]);
  const [modelRoutes, setModelRoutes] = useState([]);
  const [logs, setLogs] = useState([]);
  const [logOffset, setLogOffset] = useState(0);
  // Mirror of logOffset for the 8s polling interval: the interval closure
  // captures the fetchLogs of the render it was created in, so state reads
  // inside fetchLogs go stale — polls must fetch the page being *viewed*.
  const logOffsetRef = useRef(0);
  const [logLoading, setLogLoading] = useState(false);
  const [logFilter, setLogFilter] = useState('');
  const [timeRange, setTimeRange] = useState('all'); // 'all' | '1h' | 'today' | '7d' | 'custom'
  const [customStartTime, setCustomStartTime] = useState('');
  const [customEndTime, setCustomEndTime] = useState('');
  const [sessionFilter, setSessionFilter] = useState('');
  const [pricingGroups, setPricingGroups] = useState(['default']);
  const [prices, setPrices] = useState([]);

  // Modals & Forms
  const [showChannelModal, setShowChannelModal] = useState(false);
  const [editingChannelId, setEditingChannelId] = useState(null);
  const [copiedKey, setCopiedKey] = useState('');
  const [newChannel, setNewChannel] = useState({
    name: '',
    type: 'openai',
    base_url: '',
    api_key: '',
    priority: 1,
    weight: 10,
    timeout_seconds: 60,
    models_str: '',
    mapping_str: '',
    protocols: ['openai_chat', 'openai_response', 'openai_text', 'anthropic_messages', 'embeddings', 'rerank', 'images', 'audio_speech', 'audio_transcription', 'videos'],
  });

  const [probing, setProbing] = useState(false);
  const [probeAlert, setProbeAlert] = useState(null);

  const [showKeyModal, setShowKeyModal] = useState(false);
  const [newKey, setNewKey] = useState({
    tenant_id: '',
    key: '',
    rpm: 60,
  });

  const [testingId, setTestingId] = useState(null);

  // Batch selection states for logs, keys, channels
  const [selectedLogIds, setSelectedLogIds] = useState([]);
  const [selectedKeyIds, setSelectedKeyIds] = useState([]);
  const [selectedChannelIds, setSelectedChannelIds] = useState([]);

  // Playground Modality and Model selector state (for navigation from channels/routes)
  const [playModality, setPlayModality] = useState('chat');
  const [playModel, setPlayModel] = useState('');

  const inferClientModality = (m) => {
    const lower = (m || '').toLowerCase();
    if (lower.includes('rerank') || lower.includes('bge-reranker')) return 'rerank';
    if (lower.includes('embed') || lower.includes('bge-') || lower.includes('text-embedding')) return 'embeddings';
    if (lower.includes('tts') || lower.includes('speech') || lower.includes('voice') || lower.includes('cosyvoice')) return 'audio_speech';
    if (lower.includes('whisper') || lower.includes('sensevoice') || lower.includes('transcription') || lower.includes('asr')) return 'audio_transcription';
    if (lower.includes('seedance') || lower.includes('sora') || lower.includes('wan') || lower.includes('cogvideox') || lower.includes('kling') || lower.includes('luma') || lower.includes('runway') || lower.includes('pika') || lower.includes('vidu') || lower.includes('video')) return 'videos';
    if (lower.includes('image') || lower.includes('dall-e') || lower.includes('flux') || lower.includes('midjourney') || lower.includes('stable-diffusion') || lower.includes('seedream') || lower.includes('sdxl')) return 'images';
    return 'chat';
  };

  // Load backend data
  const fetchData = async (overrideToken) => {
    const opts = overrideToken ? { token: overrideToken } : {};
    try {
      const [chRes, keyRes, statsRes, mRes, routesRes, pricingRes] = await Promise.all([
        adminFetch('/api/v1/admin/channels', opts).then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/keys', opts).then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/stats/overview', opts).then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/models', opts).then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/models/routes', opts).then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/pricing', opts).then(r => r.json()).catch(() => ({ code: 1 })),
      ]);

      if (chRes.code === 0) setChannels(chRes.data || []);
      if (keyRes.code === 0) setKeys(keyRes.data || []);
      if (statsRes.code === 0) setStats(statsRes.data || {});
      if (pricingRes.code === 0 && Array.isArray(pricingRes.data)) {
        setPrices(pricingRes.data);
        const grpSet = new Set(['default']);
        pricingRes.data.forEach(p => {
          if (p.group_name) grpSet.add(p.group_name.trim().toLowerCase());
        });
        if (adminUser?.group_name) {
          grpSet.add(adminUser.group_name.trim().toLowerCase());
        }
        setPricingGroups(Array.from(grpSet));
      }
      if (routesRes.code === 0) setModelRoutes(routesRes.data || []);
      if (mRes.code === 0 && mRes.data?.length) {
        const allM = mRes.data;
        const routes = routesRes.data || [];
        setModels(allM);

        const getInferred = (mod) => allM.filter(m => inferClientModality(m) === mod);
        const getModModels = (mod) => {
          const matched = routes.filter(r => r.modality === mod).map(r => r.model);
          if (matched.length > 0) return matched;
          return getInferred(mod);
        };

        const effChat = getModModels('chat');
        setPlayModel(prev => (prev && effChat.includes(prev)) ? prev : (effChat[0] || ''));
      }
    } catch (e) {
      console.error('Fetch data failed:', e);
    }
  };

  const fetchLogs = async (override = {}, overrideToken) => {
    const opts = overrideToken ? { token: overrideToken } : {};
    setLogLoading(true);
    try {
      const p = new URLSearchParams();
      p.set('limit', String(LOG_PAGE_SIZE));
      // Server-side pagination. Offset resolution rules:
      //  - explicit override.offset wins (page buttons);
      //  - a filter override (timeRange/sessionFilter) resets to page 1 —
      //    the old offset is meaningless for the new result set;
      //  - otherwise keep the currently viewed page (polling refresh).
      const filterReset =
        override.timeRange !== undefined || override.sessionFilter !== undefined;
      const off =
        override.offset !== undefined ? override.offset : (filterReset ? 0 : logOffsetRef.current);
      p.set('offset', String(off));
      logOffsetRef.current = off;
      setLogOffset((prev) => (prev === off ? prev : off));

      const tr = override.timeRange !== undefined ? override.timeRange : timeRange;
      const sf = override.sessionFilter !== undefined ? override.sessionFilter : sessionFilter;

      if (sf && sf.trim()) {
        p.set('session_id', sf.trim());
      }

      const now = new Date();
      if (tr === '1h') {
        p.set('start_time', new Date(now.getTime() - 3600000).toISOString());
      } else if (tr === 'today') {
        const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 0, 0);
        p.set('start_time', startOfToday.toISOString());
      } else if (tr === '7d') {
        p.set('start_time', new Date(now.getTime() - 7 * 86400000).toISOString());
      } else if (tr === 'custom') {
        const st = override.customStartTime !== undefined ? override.customStartTime : customStartTime;
        const et = override.customEndTime !== undefined ? override.customEndTime : customEndTime;
        if (st) p.set('start_time', new Date(st).toISOString());
        if (et) p.set('end_time', new Date(et).toISOString());
      }

      const res = await adminFetch(`/api/v1/admin/logs?${p.toString()}`, opts);
      const data = await res.json();
      if (data.code === 0) {
        setLogs(data.data || []);
      }
    } catch (e) {
      console.error('Fetch logs failed:', e);
    } finally {
      setLogLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    fetchLogs();
    const interval = setInterval(() => {
      fetchData();
      if (currentTab === 'dashboard' || currentTab === 'logs') {
        fetchLogs();
      }
    }, 8000);
    return () => clearInterval(interval);
  }, [currentTab, adminToken]);

  useEffect(() => {
    if (currentTab === 'logs' || currentTab === 'dashboard') {
      setLogOffset(0);
      fetchLogs({ offset: 0 });
    }
  }, [currentTab, timeRange, sessionFilter]);

  // Server-side pagination for audit logs (offset-based, page size above).
  const handleLogPageChange = (newOffset) => {
    if (newOffset < 0) return;
    logOffsetRef.current = newOffset;
    setLogOffset(newOffset);
    fetchLogs({ offset: newOffset });
  };

  // Automatic Zero-Choice Debounced Probe
  const triggerProbe = async (url, apiKey, currentType) => {
    if (!url || !url.trim()) {
      showToast(isZh ? '请先输入下游服务的 Base URL' : 'Please enter the downstream Base URL first', 'warning');
      return;
    }
    setProbing(true);
    setProbeAlert(null);
    try {
      const res = await adminFetch('/api/v1/admin/channels/probe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          base_url: url.trim(),
          api_key: apiKey || '',
          type: currentType || 'openai',
        }),
      });
      const data = await res.json();
      if (data.code === 0 && data.data) {
        const d = data.data;
        setNewChannel(prev => {
          const shouldUpdateName = !prev.name || prev.name.includes('upstream') || prev.name.includes('cluster') || prev.name.includes('instance') || prev.name.includes('official') || prev.name.includes('direct');
          const shouldUpdateBaseUrl = d.suggested_base_url && (!prev.base_url.includes('/v1-openai') && !prev.base_url.includes('/v1'));
          return {
            ...prev,
            base_url: shouldUpdateBaseUrl ? d.suggested_base_url : prev.base_url,
            type: d.type || prev.type,
            name: shouldUpdateName ? (d.suggested_name || prev.name) : prev.name,
            models_str: (d.models && d.models.length > 0) ? d.models.join(', ') : prev.models_str,
            protocols: (d.protocols && d.protocols.length > 0) ? d.protocols : prev.protocols,
          };
        });
        const isAuthWarning = d.message && (d.message.includes('401') || d.message.includes('保护') || d.message.includes('凭证') || d.message.includes('鉴权'));
        setProbeAlert({
          type: isAuthWarning ? 'warning' : 'success',
          text: isAuthWarning
            ? `⚠️ ${d.message}`
            : (isZh
                ? `✨ 连通测试成功 (耗时: ${d.latency_ms}ms)！已自动匹配 [${d.type}] 引擎，获取到 ${d.models?.length || 0} 个在线模型并勾选对应协议。`
                : `✨ Probe passed (${d.latency_ms}ms)! Auto-matched [${d.type}] engine, discovered ${d.models?.length || 0} models and checked protocols.`),
        });
        showToast(
          isAuthWarning
            ? (isZh ? '服务已连通，但下游开启了凭证鉴权 (HTTP 401)，请输入 API Key 提取在线模型' : 'Connected, but upstream requires authentication (HTTP 401). Please enter API Key.')
            : (isZh ? `连通测试成功！已同步 ${d.models?.length || 0} 个模型` : `Connection passed! Synced ${d.models?.length || 0} models`),
          isAuthWarning ? 'warning' : 'success'
        );
      } else {
        setProbeAlert({
          type: 'error',
          text: `❌ ${isZh ? '探测失败: ' : 'Probe failed: '}${data.error || (isZh ? '无法连通指定上游服务，请检查网络连通性或服务端口' : 'Cannot connect to upstream endpoint, please check network or port')}`,
        });
        showToast(isZh ? '连通测试失败，请检查服务地址' : 'Connection test failed. Please verify endpoint URL', 'error');
      }
    } catch (e) {
      setProbeAlert({
        type: 'error',
        text: `${isZh ? '探测网络异常: ' : 'Network probe error: '}${e.message}`,
      });
      showToast(isZh ? `网络请求异常: ${e.message}` : `Network request error: ${e.message}`, 'error');
    } finally {
      setProbing(false);
    }
  };

  const handleProbeChannel = () => {
    triggerProbe(newChannel.base_url, newChannel.api_key, newChannel.type);
  };

  // Quick Presets for Provider (Never inject fake models!)
  const applyPreset = (presetKey) => {
    switch (presetKey) {
      case 'gpustack':
        setNewChannel(prev => ({
          ...prev,
          name: 'gpustack-cluster',
          type: 'gpustack',
          base_url: prev.base_url || '',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: prev.models_str || '',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'rerank', 'images'],
        }));
        showToast(isZh ? '已载入 GPUStack 企业私有算力模板，点击探测或填入 API Key 即可拉取模型' : 'Loaded GPUStack enterprise template. Click Probe or enter API Key to sync models.', 'info');
        break;
      case 'deepseek':
        setNewChannel(prev => ({
          ...prev,
          name: 'deepseek-official',
          type: 'deepseek',
          base_url: 'https://api.deepseek.com',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'deepseek-v4.1-flash, deepseek-v4-pro, deepseek-r1, deepseek-chat, deepseek-reasoner',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text'],
        }));
        showToast(isZh ? '已载入 DeepSeek 官方模板及模型列表' : 'Loaded DeepSeek official template and models', 'info');
        break;
      case 'zhipu':
        setNewChannel(prev => ({
          ...prev,
          name: 'zhipu-glm-official',
          type: 'openai',
          base_url: 'https://open.bigmodel.cn/api/paas/v4',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'glm-5.3, glm-5.3-flash, glm-4-plus, cogvideox-5b, cogview-3-plus',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'images', 'videos'],
        }));
        showToast(isZh ? '已载入 智谱 GLM 官方模板及模型列表' : 'Loaded Zhipu GLM official template and models', 'info');
        break;
      case 'doubao':
        setNewChannel(prev => ({
          ...prev,
          name: 'volcengine-doubao',
          type: 'openai',
          base_url: 'https://ark.cn-beijing.volces.com/api/v3',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'doubao-seed-2.1-pro, doubao-seed-2.1-turbo, doubao-1.5-pro, seedance-2.5',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'images', 'audio_speech', 'videos'],
        }));
        showToast(isZh ? '已载入 字节火山豆包 官方模板及模型列表' : 'Loaded Volcengine Doubao official template and models', 'info');
        break;
      case 'moonshot':
        setNewChannel(prev => ({
          ...prev,
          name: 'moonshot-kimi',
          type: 'openai',
          base_url: 'https://api.moonshot.cn/v1',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'kimi-k3, kimi-k3.1, kimi-latest, moonshot-v1-auto',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text'],
        }));
        showToast(isZh ? '已载入 月之暗面 Kimi 官方模板及模型列表' : 'Loaded Moonshot Kimi official template and models', 'info');
        break;
      case 'openai':
        setNewChannel(prev => ({
          ...prev,
          name: 'openai-official',
          type: 'openai',
          base_url: 'https://api.openai.com/v1',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'gpt-6, gpt-6-luna, gpt-6.1-sol, gpt-5.5-instant, gpt-image-2.5, gpt-live-transcribe, text-embedding-3-small, whisper-large-v3-turbo',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'anthropic_messages', 'embeddings', 'rerank', 'images', 'audio_speech', 'audio_transcription', 'videos'],
        }));
        showToast(isZh ? '已载入 OpenAI 官方模板及全能力模型' : 'Loaded OpenAI official template and all-capability models', 'info');
        break;
      case 'anthropic':
        setNewChannel(prev => ({
          ...prev,
          name: 'anthropic-claude',
          type: 'anthropic',
          base_url: 'https://api.anthropic.com',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'claude-opus-5.5, claude-sonnet-5.5, claude-haiku-5.5, claude-3-7-sonnet',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'anthropic_messages'],
        }));
        showToast(isZh ? '已载入 Claude 官方模板及模型列表' : 'Loaded Claude official template and models', 'info');
        break;
      case 'gemini':
        setNewChannel(prev => ({
          ...prev,
          name: 'google-gemini',
          type: 'gemini',
          base_url: 'https://generativelanguage.googleapis.com',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'gemini-4-argon, gemini-3.8-flash, gemini-3.5-flash-lite, gemini-2.0-flash, embeddinggemma-2',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'anthropic_messages', 'embeddings'],
        }));
        showToast(isZh ? '已载入 Google Gemini 官方模板及模型列表' : 'Loaded Google Gemini official template and models', 'info');
        break;
      case 'ollama':
        setNewChannel(prev => ({
          ...prev,
          name: 'ollama-service',
          type: 'ollama',
          base_url: prev.base_url || '',
          api_key: prev.api_key || 'ollama',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'llama3.3, qwen3.8:27b, deepseek-r1:8b',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings'],
        }));
        showToast(isZh ? '已载入 Ollama 模型列表模板，请输入您的端点地址' : 'Loaded Ollama template. Please enter your endpoint URL', 'info');
        break;
      case 'vllm':
        setNewChannel(prev => ({
          ...prev,
          name: 'vllm-service',
          type: 'vllm',
          base_url: prev.base_url || '',
          api_key: prev.api_key || 'none',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'Qwen/Qwen3.8-27B-Instruct, deepseek-ai/DeepSeek-V4.1-Flash',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings'],
        }));
        showToast(isZh ? '已载入 vLLM 模型列表模板，请输入您的端点地址' : 'Loaded vLLM template. Please enter your endpoint URL', 'info');
        break;
      case 'sub2api':
        setNewChannel(prev => ({
          ...prev,
          name: 'sub2api-upstream',
          type: 'sub2api',
          base_url: prev.base_url || '',
          api_key: prev.api_key || '',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: '',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'anthropic_messages', 'embeddings', 'rerank', 'images', 'audio_speech', 'audio_transcription', 'videos'],
        }));
        showToast(isZh ? '已载入 Sub2API 聚合模板' : 'Loaded Sub2API aggregation template', 'info');
        break;
      case 'custom':
        setNewChannel(prev => ({
          ...prev,
          name: 'custom-downstream',
          type: 'custom',
          base_url: prev.base_url || '',
          api_key: prev.api_key || '',
          priority: 2,
          weight: 10,
          timeout_seconds: 60,
          models_str: '',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'embeddings', 'rerank', 'images', 'audio_speech', 'videos'],
        }));
        showToast(isZh ? '已载入自定义下游模板，请输入您的服务端点' : 'Loaded custom downstream template. Please enter endpoint URL', 'info');
        break;
      default:
        break;
    }
  };

  const addModelChip = (modelName) => {
    const current = newChannel.models_str.split(',').map(s => s.trim()).filter(Boolean);
    if (!current.includes(modelName)) {
      const updated = [...current, modelName].join(', ');
      setNewChannel(prev => ({ ...prev, models_str: updated }));
    }
  };

  const handleBatchPing = async () => {
    if (channels.length === 0) return;
    setBatchTesting(true);
    showToast(isZh ? '正在并发检测所有渠道连通性...' : 'Testing all channels in parallel...', 'info');
    const results = {};
    await Promise.all(channels.map(async (ch) => {
      try {
        const res = await adminFetch(`/api/v1/admin/channels/${ch.id}/test`, { method: 'POST' });
        const data = await res.json();
        results[ch.id] = {
          latency_ms: data.latency_ms || 0,
          success: data.code === 0,
          error: data.error || (data.code === 0 ? '' : (isZh ? '连接失败' : 'Connection failed'))
        };
      } catch (e) {
        results[ch.id] = { latency_ms: 0, success: false, error: e.message };
      }
    }));
    setChannelLatencies(results);
    setBatchTesting(false);
    const successCount = Object.values(results).filter(r => r.success).length;
    showToast(
      isZh
        ? `体检完成: ${successCount}/${channels.length} 个渠道响应正常`
        : `Health check complete: ${successCount}/${channels.length} channels responsive`,
      successCount === channels.length ? 'success' : 'warning'
    );
  };
  const handleBatchTest = handleBatchPing;

  const generateCurlForPlayground = () => {
    const origin = getGatewayOrigin();
    const key = getEffectivePlayApiKey() || 'sk-airoute-your-key';
    return `curl -X POST "${origin}/v1/chat/completions" \\
  -H "Authorization: Bearer ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${playModel}",
    "messages": [
      {"role": "user", "content": ${JSON.stringify(playPrompt)}}
    ],
    "stream": ${playStream}
  }'`;
  };

  // Toggle protocol checkbox
  const toggleProtocol = (proto) => {
    setNewChannel(prev => {
      const exists = prev.protocols.includes(proto);
      if (exists) {
        return { ...prev, protocols: prev.protocols.filter(p => p !== proto) };
      } else {
        return { ...prev, protocols: [...prev.protocols, proto] };
      }
    });
  };

  // Handle Channel/Provider Creation & Editing
  const handleCreateChannel = async (e) => {
    e.preventDefault();
    const cleanUrl = (newChannel.base_url || '').trim();
    if (!cleanUrl.startsWith('http://') && !cleanUrl.startsWith('https://')) {
      showToast(isZh ? '接口地址必须以 http:// 或 https:// 开头' : 'Base URL must start with http:// or https://', 'warning');
      return;
    }

    const modelMapping = {};
    if (newChannel.mapping_str && newChannel.mapping_str.trim()) {
      const parts = newChannel.mapping_str.split(',');
      for (const p of parts) {
        const [k, v] = p.split(':').map(s => s.trim());
        if (k && v) {
          modelMapping[k] = v;
        }
      }
    }

    const payload = {
      ...newChannel,
      name: (newChannel.name || '').trim(),
      base_url: cleanUrl,
      models: newChannel.models_str.split(',').map(s => s.trim()).filter(Boolean),
      model_mapping: modelMapping,
    };
    delete payload.models_str;
    delete payload.mapping_str;

    if (editingChannelId) {
      await adminFetch(`/api/v1/admin/channels/${editingChannelId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      showToast(isZh ? 'Provider 更新成功，内存已热重载' : 'Provider updated successfully, memory hot-reloaded', 'success');
    } else {
      await adminFetch('/api/v1/admin/channels', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      showToast(isZh ? 'Provider 创建成功，内存已热重载' : 'Provider created successfully, memory hot-reloaded', 'success');
    }

    setShowChannelModal(false);
    setEditingChannelId(null);
    setProbeAlert(null);
    setNewChannel({
      name: '',
      type: 'openai',
      base_url: '',
      api_key: '',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: '',
      mapping_str: '',
      protocols: ['openai_chat', 'openai_response', 'openai_text', 'anthropic_messages', 'embeddings', 'rerank', 'images', 'audio_speech', 'audio_transcription', 'videos'],
    });
    fetchData();
  };

  // Handle Channel Editing
  const handleEditChannel = (ch) => {
    setEditingChannelId(ch.id);
    let mappingStr = '';
    if (ch.model_mapping) {
      mappingStr = Object.entries(ch.model_mapping).map(([k, v]) => `${k}:${v}`).join(',');
    }
    setNewChannel({
      name: ch.name || '',
      type: ch.type || 'openai',
      base_url: ch.base_url || '',
      api_key: ch.api_key || '',
      priority: ch.priority || 1,
      weight: ch.weight || 10,
      timeout_seconds: ch.timeout_seconds || 60,
      models_str: (ch.models || []).join(', '),
      mapping_str: mappingStr,
      protocols: ch.protocols || ['openai_chat', 'openai_response', 'openai_text', 'anthropic_messages', 'embeddings', 'rerank', 'images', 'audio_speech', 'audio_transcription', 'videos'],
    });
    setProbeAlert(null);
    setShowChannelModal(true);
  };

  // Handle Channel Deletion
  const handleDeleteChannel = async (id) => {
    if (!window.confirm(isZh ? '确认注销该模型提供商 (Provider)？' : 'Are you sure you want to remove this Provider?')) return;
    await adminFetch(`/api/v1/admin/channels/${id}`, { method: 'DELETE' });
    showToast(isZh ? 'Provider 已注销，内存已原子更新' : 'Provider removed, memory atomically updated', 'info');
    fetchData();
  };

  // Test Channel
  const handleTestChannel = async (ch) => {
    setTestingId(ch.id);
    try {
      const res = await adminFetch(`/api/v1/admin/channels/${ch.id}/test`, { method: 'POST' });
      const data = await res.json();
      if (data.code === 0) {
        setChannelLatencies(prev => ({
          ...prev,
          [ch.id]: { latency_ms: data.latency_ms, success: true, error: '' }
        }));
        showToast(isZh ? `✅ [${ch.name}] 连通测试成功！耗时: ${data.latency_ms} ms` : `✅ [${ch.name}] Test successful! Latency: ${data.latency_ms} ms`, 'success');
      } else {
        setChannelLatencies(prev => ({
          ...prev,
          [ch.id]: { latency_ms: 0, success: false, error: data.error || (isZh ? '连通失败' : 'Failed') }
        }));
        showToast(isZh ? `❌ [${ch.name}] 测试失败: ${data.error || '无法连通'}` : `❌ [${ch.name}] Test failed: ${data.error || 'Unreachable'}`, 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    } finally {
      setTestingId(null);
    }
  };

  // Batch Log Operations
  const handleBatchDeleteLogs = async () => {
    if (selectedLogIds.length === 0) return;
    if (!window.confirm(isZh ? `确认批量删除选中的 ${selectedLogIds.length} 条调用日志？` : `Confirm delete selected ${selectedLogIds.length} request logs?`)) return;
    try {
      const res = await adminFetch('/api/v1/admin/logs/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedLogIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '已批量删除所选日志' : 'Selected logs deleted successfully'), 'success');
        setSelectedLogIds([]);
        fetchLogs();
      } else {
        showToast(data.error || (isZh ? '批量删除日志失败' : 'Failed to delete logs'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  };

  const handleClearLogs = async () => {
    if (!window.confirm(isZh ? '⚠️ 确认清空所有调用日志？此操作无法撤销！' : '⚠️ Confirm clear all request logs? This action cannot be undone!')) return;
    try {
      const res = await adminFetch('/api/v1/admin/logs/clear', { method: 'POST' });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? '已清空全部调用历史日志' : 'All request logs cleared', 'success');
        setSelectedLogIds([]);
        fetchLogs();
      } else {
        showToast(data.error || (isZh ? '清空日志失败' : 'Failed to clear logs'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  };

  const handleDeleteSingleLog = async (id) => {
    try {
      const res = await adminFetch(`/api/v1/admin/logs/${id}`, { method: 'DELETE' });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? '已删除该条调用日志' : 'Request log deleted', 'success');
        setSelectedLogIds(prev => prev.filter(x => x !== id));
        fetchLogs();
      } else {
        showToast(data.error || (isZh ? '删除日志失败' : 'Failed to delete log'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  };

  // Batch Key Operations
  const handleBatchDeleteKeys = async () => {
    if (selectedKeyIds.length === 0) return;
    if (!window.confirm(isZh ? `确认批量注销选中的 ${selectedKeyIds.length} 个 API 密钥？` : `Confirm revoke selected ${selectedKeyIds.length} API keys?`)) return;
    try {
      const res = await adminFetch('/api/v1/admin/keys/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedKeyIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '已批量注销密钥' : 'Selected keys revoked'), 'success');
        setSelectedKeyIds([]);
        fetchData();
      } else {
        showToast(data.error || (isZh ? '批量注销失败' : 'Failed to revoke keys'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  };

  const handleBatchStatusKeys = async (status) => {
    if (selectedKeyIds.length === 0) return;
    try {
      const res = await adminFetch('/api/v1/admin/keys/batch-status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedKeyIds, status })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '批量状态更新成功' : 'Batch status update successful'), 'success');
        setSelectedKeyIds([]);
        fetchData();
      } else {
        showToast(data.error || (isZh ? '更新失败' : 'Update failed'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  };

  // Batch Channel Operations
  const handleBatchDeleteChannels = async () => {
    if (selectedChannelIds.length === 0) return;
    if (!window.confirm(isZh ? `确认批量删除选中的 ${selectedChannelIds.length} 个服务商渠道？` : `Confirm delete selected ${selectedChannelIds.length} channels?`)) return;
    try {
      const res = await adminFetch('/api/v1/admin/channels/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedChannelIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '已批量删除服务商渠道' : 'Selected channels deleted'), 'success');
        setSelectedChannelIds([]);
        fetchData();
      } else {
        showToast(data.error || (isZh ? '批量删除失败' : 'Failed to delete channels'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  };

  const handleBatchStatusChannels = async (status) => {
    if (selectedChannelIds.length === 0) return;
    try {
      const res = await adminFetch('/api/v1/admin/channels/batch-status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedChannelIds, status })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '批量更新状态成功' : 'Batch update status successful'), 'success');
        setSelectedChannelIds([]);
        fetchData();
      } else {
        showToast(data.error || (isZh ? '更新失败' : 'Update failed'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  };

  // Handle Key Modal Opening with Auto-Generated Credentials
  const handleOpenCreateKey = () => {
    const rand = 'sk-airoute-' + Math.random().toString(36).substring(2, 10) + Math.random().toString(36).substring(2, 6);
    const tenantNum = (keys?.length || 0) + 1;
    setNewKey({
      tenant_id: isZh ? `密钥-${tenantNum}` : `Key-${tenantNum}`,
      key: rand,
      rpm: 0,
      budget: 0,
      allowed_models: [],
      group_name: adminUser?.role === 'admin' ? 'default' : (adminUser?.group_name || 'default'),
    });
    setShowKeyModal(true);
  };

  // Handle Key Creation
  const handleCreateKey = async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    if (!newKey.tenant_id.trim()) {
      showToast(isZh ? '请输入密钥名称' : 'Please enter key name', 'warning');
      return;
    }
    try {
      const res = await adminFetch('/api/v1/admin/keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newKey),
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? 'API 访问密钥创建成功' : 'API access key created successfully', 'success');
        setShowKeyModal(false);
        fetchKeys();
      } else {
        showToast((isZh ? '创建失败: ' : 'Creation failed: ') + (data.error || (isZh ? '未知错误' : 'Unknown error')), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  };

  // Handle Key Group Update
  const handleUpdateKeyGroup = async (keyId, newGroup) => {
    try {
      const res = await adminFetch(`/api/v1/admin/keys/${keyId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ group_name: newGroup }),
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? `密钥计费分组已成功调整为 [${newGroup}]` : `Key billing group updated to [${newGroup}]`, 'success');
        fetchData();
      } else {
        showToast((isZh ? '调整分组失败: ' : 'Update group failed: ') + (data.error || (isZh ? '未知错误' : 'Unknown error')), 'error');
      }
    } catch (err) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + err.message, 'error');
    }
  };

  // Handle Key Deletion
  const handleDeleteKey = async (id) => {
    if (!window.confirm(isZh ? '确认注销该客户端访问密钥？' : 'Are you sure you want to revoke this API key?')) return;
    await adminFetch(`/api/v1/admin/keys/${id}`, { method: 'DELETE' });
    showToast(isZh ? '客户端访问密钥已注销' : 'Client API key revoked', 'info');
    fetchData();
  };

  // Handle Key Status Toggle (Enable/Disable)
  const handleToggleKeyStatus = async (k) => {
    const nextStatus = k.status === 'disabled' ? 'active' : 'disabled';
    const actionText = nextStatus === 'active' ? (isZh ? '启用' : 'enable') : (isZh ? '禁用/停用' : 'disable');
    try {
      await adminFetch(`/api/v1/admin/keys/${k.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: nextStatus }),
      });
      showToast(
        isZh
          ? `已${actionText}密钥 ${k.key}，集群毫秒级热同步完成`
          : `Key ${k.key} status set to ${nextStatus}, cluster synced`,
        'success'
      );
      fetchData();
    } catch (err) {
      showToast(`${actionText} ${isZh ? '失败: ' : 'failed: '}${err.message}`, 'error');
    }
  };

  // Copy text helper
  const copyToClipboard = async (txt) => {
    await crossOriginCopy(txt);
    setCopiedKey(txt);
    showToast(isZh ? '已成功复制到剪贴板！' : 'Copied to clipboard!', 'success');
    setTimeout(() => setCopiedKey(''), 2500);
  };


  const filteredLogs = logs.filter(l => {
    if (!logFilter) return true;
    const f = logFilter.toLowerCase();
    return (
      (l.trace_id && l.trace_id.toLowerCase().includes(f)) ||
      (l.chat_id && l.chat_id.toLowerCase().includes(f)) ||
      (l.session_id && l.session_id.toLowerCase().includes(f)) ||
      (l.model && l.model.toLowerCase().includes(f)) ||
      (l.channel && l.channel.toLowerCase().includes(f)) ||
      (l.tenant_id && l.tenant_id.toLowerCase().includes(f)) ||
      (l.api_key && l.api_key.toLowerCase().includes(f))
    );
  });

  if (viewMode === 'landing') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-slate-50 text-slate-800 font-sans selection:bg-indigo-500 selection:text-white">
          <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />
          <Suspense fallback={<ViewLoader />}>
            <LandingPage
              isLoggedIn={!!adminToken}
              adminUser={adminUser}
              lang={lang}
              setLang={setLang}
              toggleLang={toggleLang}
              t={t}
              onOpenLogin={() => {
                setAuthTab('login');
                setViewMode('auth');
              }}
              onEnterConsole={() => {
                if (adminToken) {
                  setViewMode('console');
                } else {
                  setAuthTab('login');
                  setViewMode('auth');
                }
              }}
              onViewDocs={() => {
                const el = document.getElementById('docs');
                if (el) {
                  const headerOffset = 70;
                  const elementPosition = el.getBoundingClientRect().top;
                  const offsetPosition = elementPosition + window.pageYOffset - headerOffset;
                  window.scrollTo({
                    top: offsetPosition,
                    behavior: 'smooth'
                  });
                }
              }}
              onViewStatus={() => setViewMode('status')}
              stats={stats}
              channelsCount={channels.length}
              modelsCount={models.length}
            />
            <LoginModal
              isOpen={showLoginModal}
              onClose={() => setShowLoginModal(false)}
              lang={lang}
              t={t}
              onLoginSuccess={(token, user) => {
                setStoredAuth(token, user);
                setAdminToken(token);
                setAdminUser(user);
                const defTab = user.role === 'admin' ? 'dashboard' : 'wallet';
                setCurrentTab(defTab);
                setShowLoginModal(false);
                setViewMode('console');
                showToast(isZh ? `欢迎回来，${user.username}！` : `Welcome back, ${user.username}!`, 'success');
                fetchData(token);
                fetchLogs({}, token);
              }}
            />
          </Suspense>
        </div>
      </I18nContext.Provider>
    );
  }

  if (viewMode === 'auth') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-slate-50 text-slate-800 font-sans selection:bg-indigo-500 selection:text-white">
          <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />
          <Suspense fallback={<ViewLoader />}>
            <AuthPage
              initialTab={authTab}
              lang={lang}
              setLang={setLang}
              toggleLang={toggleLang}
              t={t}
              onLoginSuccess={(token, user) => {
                setStoredAuth(token, user);
                setAdminToken(token);
                setAdminUser(user);
                const defTab = user.role === 'admin' ? 'dashboard' : 'wallet';
                setCurrentTab(defTab);
                setViewMode('console');
                showToast(isZh ? `欢迎回来，${user.username}！` : `Welcome back, ${user.username}!`, 'success');
                fetchData(token);
                fetchLogs({}, token);
              }}
              onBackHome={() => setViewMode('landing')}
            />
          </Suspense>
        </div>
      </I18nContext.Provider>
    );
  }

  if (viewMode === 'status') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-slate-50 text-slate-800 font-sans selection:bg-indigo-500 selection:text-white">
          <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />
          <Suspense fallback={<ViewLoader />}>
            <ServiceStatus
              isStandalone={true}
              onBackHome={() => setViewMode('landing')}
              onEnterConsole={() => {
                if (adminToken) {
                  setViewMode('console');
                } else {
                  setAuthTab('login');
                  setViewMode('auth');
                }
              }}
              onOpenLogin={() => {
                setAuthTab('login');
                setViewMode('auth');
              }}
              isLoggedIn={!!adminToken}
              lang={lang}
              setLang={setLang}
              t={t}
              modelRoutes={modelRoutes}
              channels={channels}
              onRefresh={fetchData}
              probeLatencies={channelLatencies}
              adminFetch={adminFetch}
              showToast={showToast}
              onBatchProbe={handleBatchPing}
              batchTesting={batchTesting}
            />
            <LoginModal
              isOpen={showLoginModal}
              onClose={() => setShowLoginModal(false)}
              lang={lang}
              t={t}
              onLoginSuccess={(token, user) => {
                setStoredAuth(token, user);
                setAdminToken(token);
                setAdminUser(user);
                const defTab = user.role === 'admin' ? 'dashboard' : 'wallet';
                setCurrentTab(defTab);
                setShowLoginModal(false);
                setViewMode('console');
                showToast(isZh ? `欢迎回来，${user.username}！` : `Welcome back, ${user.username}!`, 'success');
                fetchData(token);
                fetchLogs({}, token);
              }}
            />
          </Suspense>
        </div>
      </I18nContext.Provider>
    );
  }

  return (
    <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
      <div className="flex h-screen overflow-hidden bg-[#FAFAFA] dark:bg-[#0A0D14] text-slate-800 dark:text-slate-100 font-sans selection:bg-indigo-500 selection:text-white transition-colors duration-200">
      {/* Toast Notification */}
      <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />

      {/* Quick Start Modal */}
      {activeQuickKey && (
        <QuickStartModal
          apiKey={activeQuickKey}
          onClose={() => setActiveQuickKey(null)}
          onCopy={(txt) => {
            copyToClipboard(txt);
          }}
          lang={lang}
          t={t}
        />
      )}

      {/* Log Detail Inspector Modal */}
      {activeLogDetail && (
        <LogDetailModal
          log={activeLogDetail}
          onClose={() => setActiveLogDetail(null)}
          onCopy={(txt) => {
            copyToClipboard(txt);
          }}
          onFilterBySession={(sid) => {
            setSessionFilter(sid);
            setLogOffset(0);
            fetchLogs({ sessionFilter: sid, offset: 0 });
            showToast(isZh ? `已按会话 ID: ${sid} 筛选` : `Filtered by session ID: ${sid}`, 'info');
          }}
          onDeleteLog={handleDeleteSingleLog}
          onNavigate={(targetTab) => {
            setCurrentTab(targetTab);
            setViewMode('console');
            setActiveLogDetail(null);
          }}
          lang={lang}
          t={t}
        />
      )}

      {/* cURL Export Modal */}
      {curlExportCmd && (
        <CurlExportModal
          curlCmd={curlExportCmd}
          onClose={() => setCurlExportCmd('')}
          onCopy={(txt) => {
            copyToClipboard(txt);
          }}
          lang={lang}
          t={t}
        />
      )}

      {/* Mobile Drawer Backdrop */}
      {mobileMenuOpen && (
        <div
          onClick={() => setMobileMenuOpen(false)}
          className="fixed inset-0 bg-slate-900/60 z-30 md:hidden backdrop-blur-sm animate-in fade-in"
        />
      )}

      {/* Sidebar */}
      <aside
        className={`bg-white dark:bg-[#0F1420] border-r border-slate-200/70 dark:border-slate-800/80 flex flex-col shrink-0 transition-all duration-200 z-30 ${
          sidebarCollapsed ? 'w-16' : 'w-56'
        } ${
          mobileMenuOpen ? 'fixed inset-y-0 left-0 w-56 shadow-2xl flex' : 'hidden md:flex'
        }`}
      >
        <div className="h-16 px-4 border-b border-slate-100 dark:border-slate-800/70 flex items-center justify-between">
          <div
            onClick={() => setViewMode('landing')}
            className={`flex items-center ${sidebarCollapsed ? 'justify-center w-full' : 'space-x-2.5'} cursor-pointer group transition hover:opacity-90`}
            title={lang === 'zh' ? '点击返回门户首页' : 'Return to product portal'}
          >
            <div className="w-8 h-8 rounded-lg bg-slate-900 dark:bg-indigo-600 flex items-center justify-center text-white shrink-0 shadow-sm">
              <Zap className="w-4 h-4 fill-white text-white" />
            </div>
            {!sidebarCollapsed && (
              <div className="min-w-0">
                <h1 className="font-semibold text-sm text-slate-900 dark:text-slate-100 tracking-tight leading-none">
                  Airoute
                </h1>
                <span className="text-[10px] text-slate-400 font-mono tracking-tight block mt-1">
                  AI Gateway
                </span>
              </div>
            )}
          </div>

          {!sidebarCollapsed && (
            <button
              onClick={() => setSidebarCollapsed(true)}
              className="p-1 rounded-md text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
              title={isZh ? '收起侧边栏' : 'Collapse sidebar'}
            >
              <PanelLeftClose className="w-4 h-4" />
            </button>
          )}
        </div>

        {sidebarCollapsed && (
          <div className="p-2 border-b border-slate-100 dark:border-slate-800/70 flex justify-center">
            <button
              onClick={() => setSidebarCollapsed(false)}
              className="p-1.5 rounded-md text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
              title={isZh ? '展开侧边栏' : 'Expand sidebar'}
            >
              <PanelLeft className="w-4 h-4" />
            </button>
          </div>
        )}

        <nav className="flex-1 p-2 space-y-4 overflow-y-auto">
          {adminUser?.role === 'admin' ? (
            /* Admin Grouped Enterprise Navigation */
            <div className="space-y-4">
              {/* Group 1: 监控与分析 */}
              <div>
                {!sidebarCollapsed && (
                  <span className="px-2.5 text-[10px] font-semibold tracking-wider text-slate-400 uppercase block mb-1">
                    {isZh ? '监控与分析' : 'Monitoring'}
                  </span>
                )}
                <div className="space-y-0.5">
                  <button
                    onClick={() => setCurrentTab('dashboard')}
                    title={t.navDashboard}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'dashboard'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <BarChart3 className={`w-4 h-4 shrink-0 ${currentTab === 'dashboard' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navDashboard}</span>}
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('logs')}
                    title={t.navLogs}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'logs'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <History className={`w-4 h-4 shrink-0 ${currentTab === 'logs' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navLogs}</span>}
                    </div>
                    {!sidebarCollapsed && logs.length > 0 && (
                      <span className="text-xs font-mono text-slate-400">
                        {logs.length}
                      </span>
                    )}
                  </button>
                </div>
              </div>

              {/* Group 2: 模型网关与调度 */}
              <div>
                {!sidebarCollapsed && (
                  <span className="px-2.5 text-[10px] font-semibold tracking-wider text-slate-400 uppercase block mb-1">
                    {isZh ? '模型网关与调度' : 'Gateway'}
                  </span>
                )}
                <div className="space-y-0.5">
                  <button
                    onClick={() => setCurrentTab('models')}
                    title={t.navModels}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'models'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Cpu className={`w-4 h-4 shrink-0 ${currentTab === 'models' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navModels}</span>}
                    </div>
                    {!sidebarCollapsed && models.length > 0 && (
                      <span className="text-xs font-mono text-slate-400">
                        {models.length}
                      </span>
                    )}
                  </button>

                  <button
                    onClick={() => setCurrentTab('channels')}
                    title={t.navChannels}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'channels'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Server className={`w-4 h-4 shrink-0 ${currentTab === 'channels' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navChannels}</span>}
                    </div>
                    {!sidebarCollapsed && channels.length > 0 && (
                      <span className="text-xs font-mono text-slate-400">
                        {channels.length}
                      </span>
                    )}
                  </button>

                  <button
                    onClick={() => setCurrentTab('reconciliation')}
                    title={t.navReconciliation}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'reconciliation'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Coins className={`w-4 h-4 shrink-0 ${currentTab === 'reconciliation' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navReconciliation}</span>}
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('pricing')}
                    title={t.navPricing}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'pricing'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <DollarSign className={`w-4 h-4 shrink-0 ${currentTab === 'pricing' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navPricing}</span>}
                    </div>
                  </button>
                </div>
              </div>

              {/* Group 3: 访问控制与资产 */}
              <div>
                {!sidebarCollapsed && (
                  <span className="px-2.5 text-[10px] font-semibold tracking-wider text-slate-400 uppercase block mb-1">
                    {isZh ? '安全与资产' : 'Security'}
                  </span>
                )}
                <div className="space-y-0.5">
                  <button
                    onClick={() => setCurrentTab('keys')}
                    title={t.navKeys}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'keys'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Key className={`w-4 h-4 shrink-0 ${currentTab === 'keys' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navKeys}</span>}
                    </div>
                    {!sidebarCollapsed && keys.length > 0 && (
                      <span className="text-xs font-mono text-slate-400">
                        {keys.length}
                      </span>
                    )}
                  </button>

                  <button
                    onClick={() => setCurrentTab('users')}
                    title={t.navUsers || (isZh ? '用户管理' : 'User Management')}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'users'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <ShieldCheck className={`w-4 h-4 shrink-0 ${currentTab === 'users' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navUsers || (isZh ? '用户管理' : 'Users')}</span>}
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('wallet')}
                    title={isZh ? '卡密与充值' : 'Wallet & Credits'}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'wallet'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Wallet className={`w-4 h-4 shrink-0 ${currentTab === 'wallet' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{isZh ? '卡密与充值' : 'Wallet & Credits'}</span>}
                    </div>
                  </button>
                </div>
              </div>

              {/* Group 4: 演练与协议生态 */}
              <div>
                {!sidebarCollapsed && (
                  <span className="px-2.5 text-[10px] font-semibold tracking-wider text-slate-400 uppercase block mb-1">
                    {isZh ? '演练与生态' : 'Ecosystem'}
                  </span>
                )}
                <div className="space-y-0.5">
                  <button
                    onClick={() => setCurrentTab('playground')}
                    title={t.navPlayground}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'playground'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Terminal className={`w-4 h-4 shrink-0 ${currentTab === 'playground' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navPlayground}</span>}
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('skills')}
                    title={t.navSkills || 'Skill Hub'}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'skills'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Sparkles className={`w-4 h-4 shrink-0 ${currentTab === 'skills' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navSkills || 'Skill Hub'}</span>}
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('mcp')}
                    title={t.navMcp || (isZh ? 'MCP 广场' : 'MCP Hub')}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'mcp'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Cpu className={`w-4 h-4 shrink-0 ${currentTab === 'mcp' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navMcp || (isZh ? 'MCP 广场' : 'MCP Hub')}</span>}
                    </div>
                  </button>
                </div>
              </div>
            </div>
          ) : (
            /* Regular User Navigation */
            <div className="space-y-4">
              <div>
                {!sidebarCollapsed && (
                  <span className="px-2.5 text-[10px] font-semibold tracking-wider text-slate-400 uppercase block mb-1">
                    {isZh ? '资产与凭据' : 'Assets'}
                  </span>
                )}
                <div className="space-y-0.5">
                  <button
                    onClick={() => setCurrentTab('wallet')}
                    title={t.navWallet || (isZh ? '我的钱包' : 'My Wallet')}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'wallet'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Wallet className={`w-4 h-4 shrink-0 ${currentTab === 'wallet' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navWallet || (isZh ? '我的钱包' : 'My Wallet')}</span>}
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('keys')}
                    title={t.navKeys}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'keys'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Key className={`w-4 h-4 shrink-0 ${currentTab === 'keys' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navKeys}</span>}
                    </div>
                    {!sidebarCollapsed && keys.length > 0 && (
                      <span className="text-xs font-mono text-slate-400">
                        {keys.length}
                      </span>
                    )}
                  </button>
                </div>
              </div>

              <div>
                {!sidebarCollapsed && (
                  <span className="px-2.5 text-[10px] font-semibold tracking-wider text-slate-400 uppercase block mb-1">
                    {isZh ? '演练与使用' : 'Usage'}
                  </span>
                )}
                <div className="space-y-0.5">
                  <button
                    onClick={() => setCurrentTab('playground')}
                    title={t.navPlayground}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'playground'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Terminal className={`w-4 h-4 shrink-0 ${currentTab === 'playground' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navPlayground}</span>}
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('pricing')}
                    title={t.navPricing}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'pricing'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <DollarSign className={`w-4 h-4 shrink-0 ${currentTab === 'pricing' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navPricing}</span>}
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('logs')}
                    title={t.navLogs}
                    className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                      currentTab === 'logs'
                        ? 'bg-slate-100 dark:bg-slate-800/90 text-slate-900 dark:text-white font-semibold'
                        : 'text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/40 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <History className={`w-4 h-4 shrink-0 ${currentTab === 'logs' ? 'text-slate-900 dark:text-white' : 'text-slate-400'}`} />
                      {!sidebarCollapsed && <span>{t.navLogs}</span>}
                    </div>
                  </button>
                </div>
              </div>
            </div>
          )}

          <div className="pt-2 border-t border-slate-100 dark:border-slate-800/70">
            <button
              onClick={() => {
                setViewMode('landing');
                setTimeout(() => {
                  const el = document.getElementById('docs');
                  if (el) el.scrollIntoView({ behavior: 'smooth' });
                }, 100);
              }}
              className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white hover:bg-slate-50 dark:hover:bg-slate-800/40 transition cursor-pointer group`}
              title={isZh ? '查阅开发文档' : 'Documentation'}
            >
              <div className="flex items-center space-x-2.5">
                <BookOpen className="w-4 h-4 text-slate-400 group-hover:text-slate-600 dark:group-hover:text-slate-200" />
                {!sidebarCollapsed && <span>{t.navViewDocs || (isZh ? '开发文档 ↗' : 'Docs ↗')}</span>}
              </div>
              {!sidebarCollapsed && <ExternalLink className="w-3 h-3 opacity-40 group-hover:opacity-80" />}
            </button>
          </div>
        </nav>

        {/* Sidebar bottom status */}
        <div className="p-3 border-t border-slate-100 dark:border-slate-800/70 text-xs">
          <button
            type="button"
            onClick={() => setViewMode('status')}
            className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center' : 'space-x-2'} text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 transition cursor-pointer`}
            title={isZh ? '查看公开服务状态' : 'Public Status'}
          >
            <span className="w-2 h-2 rounded-full bg-emerald-500 shrink-0"></span>
            {!sidebarCollapsed && <span className="text-[11px] font-mono">{t.statusOperationalBadge}</span>}
          </button>
        </div>
      </aside>

      {/* Main Container */}
      <main className="flex-1 flex flex-col min-w-0 overflow-y-auto">
        {/* Top Header */}
        <header className="h-16 bg-[#FAFAFA]/85 dark:bg-[#0A0D14]/85 backdrop-blur-md border-b border-slate-200/60 dark:border-slate-800/60 flex items-center justify-between px-4 sm:px-6 lg:px-8 xl:px-10 sticky top-0 z-20">
          <div className="flex items-center space-x-3">
            {/* Mobile Hamburger Toggle */}
            <button
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="md:hidden p-2 rounded-lg text-slate-500 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
              title={isZh ? '打开导航栏' : 'Toggle navigation menu'}
            >
              <Menu className="w-5 h-5" />
            </button>

            {/* Clean Breadcrumb */}
            <div className="flex items-center space-x-2 text-xs">
              <span className="text-slate-400 font-normal">
                {adminUser?.role === 'admin'
                  ? ['dashboard', 'logs'].includes(currentTab)
                    ? (isZh ? '监控与分析' : 'Monitoring')
                    : ['models', 'channels', 'reconciliation', 'pricing'].includes(currentTab)
                    ? (isZh ? '模型网关与调度' : 'Gateway')
                    : ['keys', 'users', 'wallet'].includes(currentTab)
                    ? (isZh ? '安全与资产' : 'Security')
                    : (isZh ? '演练与生态' : 'Ecosystem')
                  : ['wallet', 'keys'].includes(currentTab)
                  ? (isZh ? '资产与凭据' : 'Assets')
                  : (isZh ? '演练与使用' : 'Usage')}
              </span>
              <span className="text-slate-300 dark:text-slate-700">/</span>
              <span className="font-semibold text-slate-900 dark:text-slate-100">
                {currentTab === 'dashboard' && t.navDashboard}
                {currentTab === 'models' && t.navModels}
                {currentTab === 'pricing' && t.navPricing}
                {currentTab === 'channels' && t.navChannels}
                {currentTab === 'reconciliation' && (t.navReconciliation || (isZh ? '供应商对账' : 'Reconciliation'))}
                {currentTab === 'keys' && t.navKeys}
                {currentTab === 'wallet' && (adminUser?.role === 'admin' ? (isZh ? '卡密与充值' : 'Wallet & Credits') : (t.navWallet || (isZh ? '我的钱包' : 'My Wallet')))}
                {currentTab === 'logs' && t.navLogs}
                {currentTab === 'skills' && (t.navSkills || 'Skill Hub')}
                {currentTab === 'mcp' && (t.navMcp || (isZh ? 'MCP 广场' : 'MCP Hub'))}
                {currentTab === 'users' && (t.navUsers || (isZh ? '用户管理' : 'User Management'))}
                {currentTab === 'playground' && t.navPlayground}
                {currentTab === 'docs' && t.navDocs}
              </span>
            </div>
          </div>

          <div className="flex items-center space-x-2.5">
            {/* Command Palette Trigger */}
            <button
              onClick={() => setShowCommandPalette(true)}
              className="hidden sm:flex items-center space-x-2 px-3 py-1.5 rounded-lg bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800/80 border border-slate-200/70 dark:border-slate-800/80 text-xs text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200 transition cursor-pointer shadow-sm"
              title={isZh ? '全局快捷检索与指令面板 (⌘K)' : 'Command Palette (⌘K)'}
            >
              <Search className="w-3.5 h-3.5 text-slate-400" />
              <span>{isZh ? '快速搜索...' : 'Search...'}</span>
              <kbd className="text-[10px] bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded px-1.5 py-0.5 font-mono text-slate-400">
                ⌘K
              </kbd>
            </button>

            {/* Visual Topology Modal Trigger */}
            <button
              onClick={() => setShowTopologyModal(true)}
              className="hidden sm:flex text-xs px-3 py-1.5 rounded-lg bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800/80 text-slate-700 dark:text-slate-300 font-medium items-center space-x-1.5 transition border border-slate-200/70 dark:border-slate-800/80 shadow-sm cursor-pointer"
              title={isZh ? '查看可视化路由拓扑与容灾流程' : 'View Visual Routing Topology & Failover'}
            >
              <Layers className="w-3.5 h-3.5 text-slate-400" />
              <span>{isZh ? '容灾拓扑' : 'Topology'}</span>
            </button>

            {/* Theme Toggle Button */}
            <button
              onClick={toggleTheme}
              className="p-2 rounded-lg bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800/80 text-slate-600 dark:text-slate-300 transition border border-slate-200/70 dark:border-slate-800/80 shadow-sm cursor-pointer"
              title={theme === 'dark' ? (isZh ? '切换为浅色模式' : 'Switch to Light Mode') : (isZh ? '切换为深色模式' : 'Switch to Dark Mode')}
            >
              {theme === 'dark' ? <Sun className="w-3.5 h-3.5 text-amber-400" /> : <Moon className="w-3.5 h-3.5 text-slate-500" />}
            </button>

            {/* Language Switcher Pill */}
            <button
              onClick={toggleLang}
              className="text-xs px-2.5 py-1.5 rounded-lg bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800/80 text-slate-700 dark:text-slate-300 font-medium flex items-center space-x-1 transition border border-slate-200/70 dark:border-slate-800/80 shadow-sm cursor-pointer"
              title={lang === 'zh' ? 'Switch to English' : '切换为简体中文'}
            >
              <Globe className="w-3.5 h-3.5 text-slate-400" />
              <span>{t.langToggle}</span>
            </button>

            {/* Account info & actions */}
            <div className="flex items-center space-x-2 pl-2 border-l border-slate-200 dark:border-slate-800">
              <button
                onClick={() => { setAccountModalTab('profile'); setShowAccountModal(true); }}
                className="flex items-center space-x-1.5 text-xs text-slate-700 dark:text-slate-300 font-medium px-2.5 py-1.5 rounded-lg bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800/80 border border-slate-200/70 dark:border-slate-800/80 shadow-sm transition cursor-pointer"
                title={isZh ? '账号设置' : 'Account Settings'}
              >
                <User className="w-3.5 h-3.5 text-slate-400" />
                <span>{adminUser?.username || 'admin'}</span>
              </button>

              <button
                onClick={handleLogout}
                className="p-1.5 rounded-lg hover:bg-rose-50 dark:hover:bg-rose-950/40 text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 transition cursor-pointer"
                title={isZh ? '退出登录' : 'Logout'}
              >
                <LogOut className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </header>

        <div className="px-4 sm:px-6 lg:px-8 xl:px-10 py-7 max-w-[1536px] w-full mx-auto space-y-7">
          {/* Default Password Security Warning Banner for Admins */}
          {adminUser && adminUser.role === 'admin' && adminUser.is_default_password && (
            <div className="bg-rose-50/80 dark:bg-rose-950/30 border border-rose-200/80 dark:border-rose-900/40 rounded-2xl p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 shadow-sm animate-in fade-in">
              <div className="flex items-center space-x-3.5">
                <div className="w-10 h-10 rounded-xl bg-rose-100 dark:bg-rose-900/50 text-rose-700 dark:text-rose-300 flex items-center justify-center shrink-0">
                  <ShieldAlert className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-semibold text-rose-900 dark:text-rose-200">
                    {isZh ? '安全风险预警：当前管理员账号使用默认初始密码' : 'Security Alert: Admin account is using default initial password'}
                  </h4>
                  <p className="text-xs text-rose-700 dark:text-rose-300/80 mt-0.5">
                    {isZh ? '为保障网关控制台与算力资产安全，强烈建议您立即修改初始密码，避免未授权访问风险。' : 'To protect gateway security and compute assets, please change default password immediately.'}
                  </p>
                </div>
              </div>
              <button
                onClick={() => { setAccountModalTab('password'); setShowAccountModal(true); }}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-700 text-white rounded-lg text-xs font-medium transition shrink-0 cursor-pointer flex items-center space-x-1.5"
              >
                <Lock className="w-3.5 h-3.5" />
                <span>{isZh ? '立即修改密码' : 'Change Password'}</span>
              </button>
            </div>
          )}

          {/* Low Balance Warning Banner for Regular Users */}
          {adminUser && adminUser.role !== 'admin' && Number(adminUser.balance || 0) < 5 && (
            <div className="bg-amber-50/80 dark:bg-amber-950/30 border border-amber-200/80 dark:border-amber-900/40 rounded-2xl p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 shadow-sm animate-in fade-in">
              <div className="flex items-center space-x-3.5">
                <div className="w-10 h-10 rounded-xl bg-amber-100 dark:bg-amber-900/50 text-amber-700 dark:text-amber-300 flex items-center justify-center shrink-0">
                  <AlertCircle className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-semibold text-amber-900 dark:text-amber-200">
                    {Number(adminUser.balance || 0) <= 0 ? (isZh ? '钱包额度已耗尽 (余额不足)' : 'Wallet Balance Depleted') : (isZh ? '钱包余额偏低预警' : 'Low Wallet Balance Alert')}
                  </h4>
                  <p className="text-xs text-amber-700 dark:text-amber-300/80 mt-0.5">
                    {isZh 
                      ? `当前可用额度为 ¥${Number(adminUser.balance || 0).toFixed(4)}。为避免生产 API 接口调用中断，请及时充值或兑换卡密。`
                      : `Current balance is ¥${Number(adminUser.balance || 0).toFixed(4)}. Please top up or redeem cards to prevent API interruption.`}
                  </p>
                </div>
              </div>
              <div className="flex items-center space-x-2 shrink-0">
                <button
                  onClick={() => setCurrentTab('wallet')}
                  className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white text-xs font-medium rounded-lg shadow-sm transition cursor-pointer"
                >
                  {isZh ? '前往充值 / 兑换卡密 →' : 'Top up / Redeem Card →'}
                </button>
              </div>
            </div>
          )}

          <Suspense fallback={<ViewLoader />}>
            {/* 1. ENTERPRISE BUSINESS & TELEMETRY DASHBOARD */}
            {currentTab === 'dashboard' && (
              <DashboardView
                stats={stats}
                channels={channels}
                logs={logs}
                channelLatencies={channelLatencies}
                testingId={testingId}
                batchTesting={batchTesting}
                handleBatchTest={handleBatchTest}
                handleTestChannel={handleTestChannel}
                fetchData={fetchData}
                fetchLogs={fetchLogs}
                showToast={showToast}
                setCurrentTab={setCurrentTab}
                setShowChannelModal={setShowChannelModal}
                setActiveLogDetail={setActiveLogDetail}
                adminUser={adminUser}
                onOpenTopology={() => setShowTopologyModal(true)}
                lang={lang}
                t={t}
              />
            )}

            {/* 1.5. MODEL ROUTES TAB */}
            {currentTab === 'models' && (
              <ModelRoutesManager
                adminFetch={adminFetch}
                showToast={showToast}
                lang={lang}
                t={t}
                setCurrentTab={setCurrentTab}
                onNavigateToPlayground={(m, modality) => {
                  setPlayModel(m);
                  if (modality && modality !== 'chat') {
                    setPlayModality(modality);
                  } else {
                    setPlayModality('chat');
                  }
                  setCurrentTab('playground');
                }}
              />
            )}

            {/* 1.6. MODEL PRICING & RATES TAB */}
            {currentTab === 'pricing' && (
              <PricingManager
                adminFetch={adminFetch}
                showToast={showToast}
                stats={stats}
                isAdmin={adminUser?.role === 'admin'}
                lang={lang}
                t={t}
                modelRoutes={modelRoutes}
                setCurrentTab={setCurrentTab}
              />
            )}

            {/* 2. CHANNELS / PROVIDERS TAB */}
            {currentTab === 'channels' && (
              <ChannelsView
                channels={channels}
                batchTesting={batchTesting}
                handleBatchPing={handleBatchPing}
                setEditingChannelId={setEditingChannelId}
                setNewChannel={setNewChannel}
                setProbeAlert={setProbeAlert}
                setShowChannelModal={setShowChannelModal}
                selectedChannelIds={selectedChannelIds}
                setSelectedChannelIds={setSelectedChannelIds}
                handleBatchStatusChannels={handleBatchStatusChannels}
                handleBatchDeleteChannels={handleBatchDeleteChannels}
                channelLatencies={channelLatencies}
                copyToClipboard={copyToClipboard}
                handleTestChannel={handleTestChannel}
                testingId={testingId}
                handleEditChannel={handleEditChannel}
                handleDeleteChannel={handleDeleteChannel}
                onNavigateToReconciliation={() => setCurrentTab('reconciliation')}
                lang={lang}
                t={t}
              />
            )}

            {/* 2.5. SUPPLIER RECONCILIATION TAB */}
            {currentTab === 'reconciliation' && (
              <SupplierReconciliationView
                adminFetch={adminFetch}
                showToast={showToast}
                onNavigateToLogs={(channelName) => {
                  setLogFilter(channelName);
                  setCurrentTab('logs');
                }}
                lang={lang}
                t={t}
              />
            )}

            {/* 3. API KEYS TAB */}
            {currentTab === 'keys' && (
              <KeysView
                keys={keys}
                handleOpenCreateKey={handleOpenCreateKey}
                selectedKeyIds={selectedKeyIds}
                setSelectedKeyIds={setSelectedKeyIds}
                handleBatchStatusKeys={handleBatchStatusKeys}
                handleBatchDeleteKeys={handleBatchDeleteKeys}
                copyToClipboard={copyToClipboard}
                copiedKey={copiedKey}
                handleToggleKeyStatus={handleToggleKeyStatus}
                setActiveQuickKey={setActiveQuickKey}
                handleDeleteKey={handleDeleteKey}
                handleUpdateKeyGroup={handleUpdateKeyGroup}
                adminUser={adminUser}
                pricingGroups={pricingGroups}
                onNavigateToPricing={() => setCurrentTab('pricing')}
                lang={lang}
                t={t}
              />
            )}

            {/* 4. AUDIT LOGS TAB */}
            {currentTab === 'logs' && (
              <LogsView
                sessionFilter={sessionFilter}
                setSessionFilter={setSessionFilter}
                logFilter={logFilter}
                setLogFilter={setLogFilter}
                fetchLogs={fetchLogs}
                logLoading={logLoading}
                handleClearLogs={handleClearLogs}
                timeRange={timeRange}
                setTimeRange={setTimeRange}
                customStartTime={customStartTime}
                setCustomStartTime={setCustomStartTime}
                customEndTime={customEndTime}
                setCustomEndTime={setCustomEndTime}
                selectedLogIds={selectedLogIds}
                setSelectedLogIds={setSelectedLogIds}
                handleBatchDeleteLogs={handleBatchDeleteLogs}
                filteredLogs={filteredLogs}
                logsLength={logs.length}
                logOffset={logOffset}
                logPageSize={LOG_PAGE_SIZE}
                onLogPageChange={handleLogPageChange}
                setActiveLogDetail={setActiveLogDetail}
                showToast={showToast}
                handleDeleteSingleLog={handleDeleteSingleLog}
                lang={lang}
                t={t}
              />
            )}

            {/* 5. SKILL HUB TAB */}
            {currentTab === 'skills' && (
              <SkillHubView
                adminFetch={adminFetch}
                onCopy={(txt) => {
                  copyToClipboard(txt);
                }}
                showToast={showToast}
                lang={lang}
                t={t}
              />
            )}

            {/* 6. MCP HUB TAB */}
            {currentTab === 'mcp' && (
              <McpIntegrationView
                adminFetch={adminFetch}
                adminToken={adminToken}
                keys={keys}
                onCopy={(txt) => {
                  copyToClipboard(txt);
                }}
                showToast={showToast}
                lang={lang}
                t={t}
              />
            )}

            {/* 6. USER MANAGEMENT TAB */}
            {currentTab === 'users' && (
              <UserManagementView
                adminUser={adminUser}
                adminToken={adminToken}
                adminFetch={adminFetch}
                showToast={showToast}
                lang={lang}
                t={t}
              />
            )}

            {/* 7. WALLET MANAGEMENT TAB */}
            {currentTab === 'wallet' && (
              <WalletManagementView
                adminUser={adminUser}
                adminToken={adminToken}
                adminFetch={adminFetch}
                showToast={showToast}
                onUserUpdated={(updatedUser) => {
                  setAdminUser(prev => {
                    const merged = prev ? { ...prev, ...updatedUser } : updatedUser;
                    localStorage.setItem('airoute_user', JSON.stringify(merged));
                    return merged;
                  });
                  fetchUserProfile();
                }}
                onBalanceUpdate={(newBal) => {
                  setAdminUser(prev => {
                    const merged = prev ? { ...prev, balance: newBal } : prev;
                    localStorage.setItem('airoute_user', JSON.stringify(merged));
                    return merged;
                  });
                  fetchUserProfile();
                }}
                lang={lang}
                t={t}
              />
            )}

            {/* 5. MULTIMODAL PLAYGROUND TAB */}
            {currentTab === 'playground' && (
              <PlaygroundView
                keys={keys}
                models={models}
                modelRoutes={modelRoutes}
                adminFetch={adminFetch}
                showToast={showToast}
                initialModel={playModel}
                initialModality={playModality}
                lang={lang}
                t={t}
              />
            )}

            {/* 6. DOCS TAB */}
            {currentTab === 'docs' && (
              <DocsView showToast={showToast} lang={lang} t={t} />
            )}
          </Suspense>
        </div>
      </main>

      <Suspense fallback={null}>
        {/* Modal: New Provider with Smart Auto-Probe */}
        <ChannelModal
          isOpen={showChannelModal}
          onClose={() => setShowChannelModal(false)}
          editingChannelId={editingChannelId}
          newChannel={newChannel}
          setNewChannel={setNewChannel}
          probeAlert={probeAlert}
          probing={probing}
          applyPreset={applyPreset}
          handleProbeChannel={handleProbeChannel}
          toggleProtocol={toggleProtocol}
          onSubmit={handleCreateChannel}
          lang={lang}
          t={t}
        />

        {/* Modal: New API Key */}
        <KeyModal
          isOpen={showKeyModal}
          onClose={() => setShowKeyModal(false)}
          newKey={newKey}
          setNewKey={setNewKey}
          pricingGroups={pricingGroups}
          adminUser={adminUser}
          onSubmit={handleCreateKey}
          lang={lang}
          t={t}
        />

        {/* Account & Security Modal */}
        <AccountManageModal
          isOpen={showAccountModal}
          initialTab={accountModalTab}
          onClose={() => setShowAccountModal(false)}
          adminUser={adminUser}
          adminToken={adminToken}
          adminFetch={adminFetch}
          showToast={showToast}
          lang={lang}
          t={t}
          onPasswordChanged={() => {
            showToast(isZh ? '密码修改成功，请牢记新密码' : 'Password changed successfully', 'success');
          }}
        />

        {/* Login Modal */}
        <LoginModal
          isOpen={showLoginModal}
          onClose={() => setShowLoginModal(false)}
          lang={lang}
          t={t}
          onLoginSuccess={(token, user) => {
            setStoredAuth(token, user);
            setAdminToken(token);
            setAdminUser(user);
            const defTab = user.role === 'admin' ? 'dashboard' : 'wallet';
            setCurrentTab(defTab);
            setShowLoginModal(false);
            showToast(isZh ? `欢迎回来，${user.username}！` : `Welcome back, ${user.username}!`, 'success');
            fetchData(token);
            fetchLogs({}, token);
          }}
        />

        {/* 3-Step Guided Developer Onboarding Modal */}
        <OnboardingModal
          isOpen={showOnboardingModal}
          onClose={() => setShowOnboardingModal(false)}
          keys={keys}
          models={models}
          onNavigateToPlayground={(m) => {
            setPlayModel(m);
            setCurrentTab('playground');
          }}
          showToast={showToast}
          lang={lang}
          t={t}
        />

        {/* Visual Routing Topology & Failover Modal */}
        <VisualTopologyModal
          isOpen={showTopologyModal}
          onClose={() => setShowTopologyModal(false)}
          channels={channels}
          modelRoutes={modelRoutes}
          onTestChannel={handleTestChannel}
          showToast={showToast}
          lang={lang}
          t={t}
        />

        {/* Global Command Palette (Cmd+K) */}
        <CommandPalette
          isOpen={showCommandPalette}
          onClose={() => setShowCommandPalette(false)}
          currentTab={currentTab}
          setCurrentTab={setCurrentTab}
          setViewMode={setViewMode}
          models={models}
          channels={channels}
          keys={keys}
          adminUser={adminUser}
          onOpenNewKey={() => setShowKeyModal(true)}
          onOpenNewChannel={() => {
            setEditingChannelId(null);
            setNewChannel({
              name: '',
              type: 'gpustack',
              base_url: '',
              api_key: '',
              priority: 1,
              weight: 10,
              timeout_seconds: 60,
              models_str: '',
              mapping_str: '',
              protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'rerank', 'images'],
            });
            setProbeAlert(null);
            setShowChannelModal(true);
          }}
          toggleLang={toggleLang}
          lang={lang}
          onLogout={handleLogout}
          showToast={showToast}
          onOpenCopilotWithQuery={(q) => {
            setCopilotInitialQuery(q || '');
            setShowAiCopilot(true);
          }}
        />

        {/* Airoute AI Copilot (Cmd+J / Floating Omnibar) */}
        <AiCopilot
          isOpen={showAiCopilot}
          onClose={() => setShowAiCopilot(false)}
          onToggle={() => setShowAiCopilot(prev => !prev)}
          setCurrentTab={setCurrentTab}
          setViewMode={setViewMode}
          models={models}
          channels={channels}
          keys={keys}
          logs={logs}
          modelRoutes={modelRoutes}
          prices={prices}
          stats={stats}
          lang={lang}
          t={t}
          showToast={showToast}
          adminUser={adminUser}
          adminFetch={adminFetch}
          initialQuery={copilotInitialQuery}
          onClearInitialQuery={() => setCopilotInitialQuery('')}
        />
      </Suspense>
    </div>
    </I18nContext.Provider>
  );
}
