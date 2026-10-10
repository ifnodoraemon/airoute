import React, { useState, useEffect, useRef } from 'react';
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
import QuickStartModal from './components/QuickStartModal';
import LogDetailModal from './components/LogDetailModal';
import CurlExportModal from './components/CurlExportModal';
import ModelChipManager from './components/ModelChipManager';
import ModelMappingEditor from './components/ModelMappingEditor';
import CapabilitiesSelector from './components/CapabilitiesSelector';
import LandingPage from './components/LandingPage';
import AuthPage from './components/AuthPage';
import LoginModal from './components/LoginModal';
import CommandPalette from './components/CommandPalette';
import AccountManageModal from './components/AccountManageModal';
import ModelRoutesManager from './components/ModelRoutesManager';
import ServiceStatus from './components/ServiceStatus';
import PricingManager from './components/PricingManager';
import SkillHubView from './components/SkillHubView';
import McpIntegrationView from './components/McpIntegrationView';
import UserManagementView from './components/UserManagementView';
import WalletManagementView from './components/WalletManagementView';
import ChannelsView from './components/ChannelsView';
import KeysView from './components/KeysView';
import LogsView from './components/LogsView';
import DashboardView from './components/DashboardView';
import PlaygroundView from './components/PlaygroundView';
import DocsView from './components/DocsView';
import OnboardingModal from './components/OnboardingModal';
import VisualTopologyModal from './components/VisualTopologyModal';
import { translations, I18nContext } from './i18n';
import { getGatewayOrigin, getGatewayBaseUrl, setPublicGatewayUrl } from './config';

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
  const [theme, setTheme] = useState(() => localStorage.getItem('airoute_theme') || 'dark');

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
    'dashboard', 'models', 'pricing', 'channels',
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

    if (route === 'landing' || route === 'home') {
      return { viewMode: 'landing', tab: defaultTab, authTab: 'login' };
    }

    if (ALL_CONSOLE_TABS.includes(route)) {
      if (token) {
        const adminOnlyTabs = ['channels', 'users', 'skills', 'mcp', 'dashboard'];
        if (role !== 'admin' && adminOnlyTabs.includes(route)) {
          return { viewMode: 'console', tab: 'wallet', authTab: 'login' };
        }
        return { viewMode: 'console', tab: route, authTab: 'login' };
      } else {
        return { viewMode: 'landing', tab: defaultTab, authTab: 'login' };
      }
    }

    if (token) {
      return { viewMode: 'console', tab: defaultTab, authTab: 'login' };
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
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [showOnboardingModal, setShowOnboardingModal] = useState(false);
  const [showTopologyModal, setShowTopologyModal] = useState(false);
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
          showToast('登录凭证已失效，请重新登录', 'warning');
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
    showToast('已安全退出账号', 'info');
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
      }
    } catch (e) {}
  };

  const [activeQuickKey, setActiveQuickKey] = useState(null);
  const [activeLogDetail, setActiveLogDetail] = useState(null);
  const [curlExportCmd, setCurlExportCmd] = useState('');
  const [showApiKeyPlain, setShowApiKeyPlain] = useState(false);
  const [batchTesting, setBatchTesting] = useState(false);
  const [channelLatencies, setChannelLatencies] = useState({});

  const [currentTab, setCurrentTab] = useState(() => initialRoute.tab);

  // Synchronize browser address bar URL hash with active view and tab
  useEffect(() => {
    let targetHash = '';
    if (viewMode === 'landing') {
      if (window.location.hash && window.location.hash !== '#/' && window.location.hash !== '#/landing') {
        targetHash = '#/';
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
      showToast('请先输入下游服务的 Base URL', 'warning');
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
            : `✨ 连通测试成功 (耗时: ${d.latency_ms}ms)！已自动匹配 [${d.type}] 引擎，获取到 ${d.models?.length || 0} 个在线模型并勾选对应协议。`,
        });
        showToast(
          isAuthWarning
            ? '服务已连通，但下游开启了凭证鉴权 (HTTP 401)，请输入 API Key 提取在线模型'
            : `连通测试成功！已同步 ${d.models?.length || 0} 个模型`,
          isAuthWarning ? 'warning' : 'success'
        );
      } else {
        setProbeAlert({
          type: 'error',
          text: `❌ 探测失败: ${data.error || '无法连通指定上游服务，请检查网络连通性或服务端口'}`,
        });
        showToast('连通测试失败，请检查服务地址', 'error');
      }
    } catch (e) {
      setProbeAlert({
        type: 'error',
        text: `探测网络异常: ${e.message}`,
      });
      showToast(`网络请求异常: ${e.message}`, 'error');
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
        showToast('已载入 GPUStack 企业私有算力模板，点击探测或填入 API Key 即可拉取模型', 'info');
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
        showToast('已载入 DeepSeek 官方模板及模型列表', 'info');
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
        showToast('已载入 智谱 GLM 官方模板及模型列表', 'info');
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
        showToast('已载入 字节火山豆包 官方模板及模型列表', 'info');
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
        showToast('已载入 月之暗面 Kimi 官方模板及模型列表', 'info');
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
        showToast('已载入 OpenAI 官方模板及全能力模型', 'info');
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
        showToast('已载入 Claude 官方模板及模型列表', 'info');
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
        showToast('已载入 Google Gemini 官方模板及模型列表', 'info');
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
        showToast('已载入 Ollama 模型列表模板，请输入您的端点地址', 'info');
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
        showToast('已载入 vLLM 模型列表模板，请输入您的端点地址', 'info');
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
        showToast('已载入 Sub2API 聚合模板', 'info');
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
        showToast('已载入自定义下游模板，请输入您的服务端点', 'info');
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
    showToast('正在并发检测所有渠道连通性...', 'info');
    const results = {};
    await Promise.all(channels.map(async (ch) => {
      try {
        const res = await adminFetch(`/api/v1/admin/channels/${ch.id}/test`, { method: 'POST' });
        const data = await res.json();
        results[ch.id] = {
          latency_ms: data.latency_ms || 0,
          success: data.code === 0,
          error: data.error || (data.code === 0 ? '' : '连接失败')
        };
      } catch (e) {
        results[ch.id] = { latency_ms: 0, success: false, error: e.message };
      }
    }));
    setChannelLatencies(results);
    setBatchTesting(false);
    const successCount = Object.values(results).filter(r => r.success).length;
    showToast(`体检完成: ${successCount}/${channels.length} 个渠道响应正常`, successCount === channels.length ? 'success' : 'warning');
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
      showToast('Provider 更新成功，内存已热重载', 'success');
    } else {
      await adminFetch('/api/v1/admin/channels', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      showToast('Provider 创建成功，内存已热重载', 'success');
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
    if (!window.confirm('确认注销该模型提供商 (Provider)？')) return;
    await adminFetch(`/api/v1/admin/channels/${id}`, { method: 'DELETE' });
    showToast('Provider 已注销，内存已原子更新', 'info');
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
        showToast(`✅ [${ch.name}] 连通测试成功！耗时: ${data.latency_ms} ms`, 'success');
      } else {
        setChannelLatencies(prev => ({
          ...prev,
          [ch.id]: { latency_ms: 0, success: false, error: data.error || '连通失败' }
        }));
        showToast(`❌ [${ch.name}] 测试失败: ${data.error || '无法连通'}`, 'error');
      }
    } catch (e) {
      showToast(`请求异常: ${e.message}`, 'error');
    } finally {
      setTestingId(null);
    }
  };

  // Batch Log Operations
  const handleBatchDeleteLogs = async () => {
    if (selectedLogIds.length === 0) return;
    if (!window.confirm(`确认批量删除选中的 ${selectedLogIds.length} 条调用日志？`)) return;
    try {
      const res = await adminFetch('/api/v1/admin/logs/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedLogIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || '已批量删除所选日志', 'success');
        setSelectedLogIds([]);
        fetchLogs();
      } else {
        showToast(data.error || '批量删除日志失败', 'error');
      }
    } catch (e) {
      showToast('请求异常: ' + e.message, 'error');
    }
  };

  const handleClearLogs = async () => {
    if (!window.confirm('⚠️ 确认清空所有调用日志？此操作无法撤销！')) return;
    try {
      const res = await adminFetch('/api/v1/admin/logs/clear', { method: 'POST' });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast('已清空全部调用历史日志', 'success');
        setSelectedLogIds([]);
        fetchLogs();
      } else {
        showToast(data.error || '清空日志失败', 'error');
      }
    } catch (e) {
      showToast('请求异常: ' + e.message, 'error');
    }
  };

  const handleDeleteSingleLog = async (id) => {
    try {
      const res = await adminFetch(`/api/v1/admin/logs/${id}`, { method: 'DELETE' });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast('已删除该条调用日志', 'success');
        setSelectedLogIds(prev => prev.filter(x => x !== id));
        fetchLogs();
      } else {
        showToast(data.error || '删除日志失败', 'error');
      }
    } catch (e) {
      showToast('请求异常: ' + e.message, 'error');
    }
  };

  // Batch Key Operations
  const handleBatchDeleteKeys = async () => {
    if (selectedKeyIds.length === 0) return;
    if (!window.confirm(`确认批量注销选中的 ${selectedKeyIds.length} 个 API 密钥？`)) return;
    try {
      const res = await adminFetch('/api/v1/admin/keys/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedKeyIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || '已批量注销密钥', 'success');
        setSelectedKeyIds([]);
        fetchData();
      } else {
        showToast(data.error || '批量注销失败', 'error');
      }
    } catch (e) {
      showToast('请求异常: ' + e.message, 'error');
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
        showToast(data.message || '批量状态更新成功', 'success');
        setSelectedKeyIds([]);
        fetchData();
      } else {
        showToast(data.error || '更新失败', 'error');
      }
    } catch (e) {
      showToast('请求异常: ' + e.message, 'error');
    }
  };

  // Batch Channel Operations
  const handleBatchDeleteChannels = async () => {
    if (selectedChannelIds.length === 0) return;
    if (!window.confirm(`确认批量删除选中的 ${selectedChannelIds.length} 个服务商渠道？`)) return;
    try {
      const res = await adminFetch('/api/v1/admin/channels/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedChannelIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || '已批量删除服务商渠道', 'success');
        setSelectedChannelIds([]);
        fetchData();
      } else {
        showToast(data.error || '批量删除失败', 'error');
      }
    } catch (e) {
      showToast('请求异常: ' + e.message, 'error');
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
        showToast(data.message || '批量更新状态成功', 'success');
        setSelectedChannelIds([]);
        fetchData();
      } else {
        showToast(data.error || '更新失败', 'error');
      }
    } catch (e) {
      showToast('请求异常: ' + e.message, 'error');
    }
  };

  // Handle Key Modal Opening with Auto-Generated Credentials
  const handleOpenCreateKey = () => {
    const rand = 'sk-airoute-' + Math.random().toString(36).substring(2, 10) + Math.random().toString(36).substring(2, 6);
    const tenantNum = (keys?.length || 0) + 1;
    setNewKey({
      tenant_id: `密钥-${tenantNum}`,
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
      showToast('请输入密钥名称', 'warning');
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
        showToast('API 访问密钥创建成功', 'success');
        setShowKeyModal(false);
        fetchKeys();
      } else {
        showToast('创建失败: ' + (data.error || '未知错误'), 'error');
      }
    } catch (e) {
      showToast('请求异常: ' + e.message, 'error');
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
        showToast(`密钥计费分组已成功调整为 [${newGroup}]`, 'success');
        fetchData();
      } else {
        showToast(`调整分组失败: ${data.error || '未知错误'}`, 'error');
      }
    } catch (err) {
      showToast(`请求异常: ${err.message}`, 'error');
    }
  };

  // Handle Key Deletion
  const handleDeleteKey = async (id) => {
    if (!window.confirm('确认注销该客户端访问密钥？')) return;
    await adminFetch(`/api/v1/admin/keys/${id}`, { method: 'DELETE' });
    showToast('客户端访问密钥已注销', 'info');
    fetchData();
  };

  // Handle Key Status Toggle (Enable/Disable)
  const handleToggleKeyStatus = async (k) => {
    const nextStatus = k.status === 'disabled' ? 'active' : 'disabled';
    const actionText = nextStatus === 'active' ? '启用' : '禁用/停用';
    try {
      await adminFetch(`/api/v1/admin/keys/${k.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: nextStatus }),
      });
      showToast(`已${actionText}密钥 ${k.key}，集群毫秒级热同步完成`, 'success');
      fetchData();
    } catch (err) {
      showToast(`${actionText}失败: ${err.message}`, 'error');
    }
  };

  // Copy text helper
  const copyToClipboard = (txt) => {
    navigator.clipboard.writeText(txt);
    setCopiedKey(txt);
    showToast('已成功复制到剪贴板！', 'success');
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
              setCurrentTab('docs');
              if (adminToken) {
                setViewMode('console');
              } else {
                setAuthTab('login');
                setViewMode('auth');
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
        </div>
      </I18nContext.Provider>
    );
  }

  if (viewMode === 'auth') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-slate-50 text-slate-800 font-sans selection:bg-indigo-500 selection:text-white">
          <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />
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
        </div>
      </I18nContext.Provider>
    );
  }

  if (viewMode === 'status') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-slate-50 text-slate-800 font-sans selection:bg-indigo-500 selection:text-white">
          <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />
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
        </div>
      </I18nContext.Provider>
    );
  }

  return (
    <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
      <div className="flex h-screen overflow-hidden bg-slate-50 dark:bg-[#0b0f19] text-slate-800 dark:text-slate-100 font-sans selection:bg-indigo-500 selection:text-white transition-colors duration-200">
      {/* Toast Notification */}
      <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />

      {/* Quick Start Modal */}
      {activeQuickKey && (
        <QuickStartModal
          apiKey={activeQuickKey}
          onClose={() => setActiveQuickKey(null)}
          onCopy={(txt) => {
            navigator.clipboard.writeText(txt);
            showToast(isZh ? '调用代码已复制到剪贴板！' : 'Integration code copied to clipboard!', 'success');
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
            navigator.clipboard.writeText(txt);
            showToast(isZh ? '已复制到剪贴板！' : 'Copied to clipboard!', 'success');
          }}
          onFilterBySession={(sid) => {
            setSessionFilter(sid);
            setLogOffset(0);
            fetchLogs({ sessionFilter: sid, offset: 0 });
            showToast(isZh ? `已按会话 ID: ${sid} 筛选` : `Filtered by session ID: ${sid}`, 'info');
          }}
          onDeleteLog={handleDeleteSingleLog}
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
            navigator.clipboard.writeText(txt);
            showToast(isZh ? 'cURL 命令已复制到剪贴板！' : 'cURL command copied to clipboard!', 'success');
          }}
          lang={lang}
          t={t}
        />
      )}

      {/* Mobile Drawer Backdrop */}
      {mobileMenuOpen && (
        <div
          onClick={() => setMobileMenuOpen(false)}
          className="fixed inset-0 bg-slate-900/60 z-30 md:hidden backdrop-blur-xs animate-in fade-in"
        />
      )}

      {/* Sidebar */}
      <aside
        className={`bg-white/95 dark:bg-[#111726]/95 border-r border-slate-200/80 dark:border-slate-800/80 flex flex-col backdrop-blur-xl shadow-xs shrink-0 transition-all duration-200 z-30 ${
          sidebarCollapsed ? 'w-20' : 'w-64'
        } ${
          mobileMenuOpen ? 'fixed inset-y-0 left-0 w-64 shadow-2xl flex' : 'hidden md:flex'
        }`}
      >
        <div className="p-4 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
          <div
            onClick={() => setViewMode('landing')}
            className="flex items-center space-x-3 cursor-pointer group transition duration-150 hover:opacity-90"
            title={lang === 'zh' ? '点击返回门户首页' : 'Return to product portal'}
          >
            <div className="w-10 h-10 rounded-2xl bg-gradient-to-tr from-indigo-600 via-indigo-500 to-sky-400 flex items-center justify-center shadow-lg shadow-indigo-500/25 text-white shrink-0 group-hover:shadow-indigo-500/40 transition">
              <Zap className="w-5 h-5 fill-white text-white" />
            </div>
            {!sidebarCollapsed && (
              <div>
                <h1 className="font-bold text-base text-slate-900 dark:text-slate-100 tracking-tight group-hover:text-indigo-600 transition">
                  Airoute
                </h1>
                <span className="text-[11px] text-indigo-600 dark:text-indigo-400 font-semibold tracking-wide block">
                  {t.brandSubtitle}
                </span>
              </div>
            )}
          </div>

          <button
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            className="hidden md:flex p-1.5 rounded-xl text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
            title={sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'}
          >
            {sidebarCollapsed ? <PanelLeft className="w-4 h-4" /> : <PanelLeftClose className="w-4 h-4" />}
          </button>
        </div>

        {/* Command Palette Trigger */}
        <div className="px-4 py-2.5 border-b border-slate-100">
          <button
            onClick={() => setShowCommandPalette(true)}
            className="w-full flex items-center justify-between px-3 py-2 bg-slate-100/80 hover:bg-slate-200/70 border border-slate-200/80 rounded-xl text-xs text-slate-500 hover:text-slate-800 transition cursor-pointer"
            title={isZh ? '快捷搜索 / 指令面板 (⌘K)' : 'Command Palette (⌘K)'}
          >
            <div className="flex items-center space-x-2">
              <Search className="w-3.5 h-3.5 text-slate-400" />
              <span>{isZh ? '快速检索 / 指令' : 'Quick Search / Command'}</span>
            </div>
            <kbd className="text-[10px] bg-white border border-slate-200 rounded px-1.5 py-0.5 font-mono font-semibold text-slate-400">
              ⌘K
            </kbd>
          </button>
        </div>

        <nav className="flex-1 p-3 space-y-4 overflow-y-auto">
          {adminUser?.role === 'admin' ? (
            /* Admin Grouped Enterprise Navigation */
            <div className="space-y-4">
              {/* Group 1: 监控与分析 */}
              <div>
                <span className="px-3 text-[10px] font-bold tracking-wider text-slate-400 uppercase block mb-1">
                  {isZh ? '监控与分析' : 'Monitoring & Analytics'}
                </span>
                <div className="space-y-1">
                  <button
                    onClick={() => setCurrentTab('dashboard')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'dashboard'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <BarChart3 className="w-4 h-4 text-indigo-500" />
                      <span>{t.navDashboard}</span>
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('logs')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'logs'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <History className="w-4 h-4 text-sky-500" />
                      <span>{t.navLogs}</span>
                    </div>
                    {logs.length > 0 && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded-md font-mono bg-slate-100 text-slate-500">
                        {logs.length}
                      </span>
                    )}
                  </button>
                </div>
              </div>

              {/* Group 2: 模型网关与调度 */}
              <div>
                <span className="px-3 text-[10px] font-bold tracking-wider text-slate-400 uppercase block mb-1">
                  {isZh ? '模型网关与调度' : 'Gateway & Routing'}
                </span>
                <div className="space-y-1">
                  <button
                    onClick={() => setCurrentTab('models')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'models'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Cpu className="w-4 h-4 text-pink-500" />
                      <span>{t.navModels}</span>
                    </div>
                    {models.length > 0 && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded-md font-mono bg-slate-100 text-slate-500">
                        {models.length}
                      </span>
                    )}
                  </button>

                  <button
                    onClick={() => setCurrentTab('channels')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'channels'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Server className="w-4 h-4 text-emerald-500" />
                      <span>{t.navChannels}</span>
                    </div>
                    {channels.length > 0 && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded-md font-mono bg-slate-100 text-slate-500">
                        {channels.length}
                      </span>
                    )}
                  </button>

                  <button
                    onClick={() => setCurrentTab('pricing')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'pricing'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <DollarSign className="w-4 h-4 text-emerald-600" />
                      <span>{t.navPricing}</span>
                    </div>
                  </button>
                </div>
              </div>

              {/* Group 3: 访问控制与资产 */}
              <div>
                <span className="px-3 text-[10px] font-bold tracking-wider text-slate-400 uppercase block mb-1">
                  {isZh ? '安全与身份资产' : 'Security & Identity'}
                </span>
                <div className="space-y-1">
                  <button
                    onClick={() => setCurrentTab('keys')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'keys'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Key className="w-4 h-4 text-amber-500" />
                      <span>{t.navKeys}</span>
                    </div>
                    {keys.length > 0 && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded-md font-mono bg-slate-100 text-slate-500">
                        {keys.length}
                      </span>
                    )}
                  </button>

                  <button
                    onClick={() => setCurrentTab('users')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'users'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <ShieldCheck className="w-4 h-4 text-indigo-600" />
                      <span>{t.navUsers}</span>
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('wallet')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'wallet'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Wallet className="w-4 h-4 text-teal-600" />
                      <span>{isZh ? '卡密与充值' : 'Wallet & Credits'}</span>
                    </div>
                  </button>
                </div>
              </div>

              {/* Group 4: 演练与扩展 */}
              <div>
                <span className="px-3 text-[10px] font-bold tracking-wider text-slate-400 uppercase block mb-1">
                  {isZh ? '演练与协议生态' : 'Playground & Ecosystem'}
                </span>
                <div className="space-y-1">
                  <button
                    onClick={() => setCurrentTab('playground')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'playground'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Terminal className="w-4 h-4 text-purple-500" />
                      <span>{t.navPlayground}</span>
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('skills')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'skills'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Sparkles className="w-4 h-4 text-amber-500" />
                      <span>{t.navSkills || 'Skill Hub'}</span>
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('mcp')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'mcp'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Cpu className="w-4 h-4 text-indigo-500" />
                      <span>{t.navMcp || 'MCP 广场'}</span>
                    </div>
                  </button>
                </div>
              </div>
            </div>
          ) : (
            /* Regular User Navigation */
            <div className="space-y-4">
              <div>
                <span className="px-3 text-[10px] font-bold tracking-wider text-slate-400 uppercase block mb-1">
                  {isZh ? '资产与凭据' : 'Assets & Credentials'}
                </span>
                <div className="space-y-1">
                  <button
                    onClick={() => setCurrentTab('wallet')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'wallet'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Wallet className="w-4 h-4 text-emerald-600" />
                      <span>{t.navWallet || (isZh ? '我的钱包' : 'My Wallet')}</span>
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('keys')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'keys'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Key className="w-4 h-4 text-amber-500" />
                      <span>{t.navKeys}</span>
                    </div>
                    {keys.length > 0 && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded-md font-mono bg-slate-100 text-slate-500">
                        {keys.length}
                      </span>
                    )}
                  </button>
                </div>
              </div>

              <div>
                <span className="px-3 text-[10px] font-bold tracking-wider text-slate-400 uppercase block mb-1">
                  {isZh ? '演练与使用' : 'Playground & Usage'}
                </span>
                <div className="space-y-1">
                  <button
                    onClick={() => setCurrentTab('playground')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'playground'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <Terminal className="w-4 h-4 text-purple-500" />
                      <span>{t.navPlayground}</span>
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('pricing')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'pricing'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <DollarSign className="w-4 h-4 text-emerald-500" />
                      <span>{t.navPricing}</span>
                    </div>
                  </button>

                  <button
                    onClick={() => setCurrentTab('logs')}
                    className={`w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl border text-xs font-medium transition-all cursor-pointer ${
                      currentTab === 'logs'
                        ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-bold'
                        : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                    }`}
                  >
                    <div className="flex items-center space-x-2.5">
                      <History className="w-4 h-4 text-sky-500" />
                      <span>{t.navLogs}</span>
                    </div>
                  </button>
                </div>
              </div>
            </div>
          )}

          <div className="pt-3 mt-2 border-t border-slate-100">
            <button
              onClick={() => {
                setViewMode('landing');
                setTimeout(() => {
                  const el = document.getElementById('docs');
                  if (el) el.scrollIntoView({ behavior: 'smooth' });
                }, 100);
              }}
              className="w-full flex items-center justify-between px-4 py-2.5 rounded-xl border border-slate-200/70 bg-slate-50/50 hover:bg-indigo-50/50 hover:border-indigo-200 text-xs font-semibold text-slate-600 hover:text-indigo-600 transition cursor-pointer group"
              title={isZh ? '前往门户首页查阅交互式开发文档' : 'Go to portal to view interactive developer documentation'}
            >
              <div className="flex items-center space-x-2.5">
                <BookOpen className="w-3.5 h-3.5 text-slate-400 group-hover:text-indigo-500" />
                <span>{t.navViewDocs || (isZh ? '开发文档 ↗' : 'API Docs ↗')}</span>
              </div>
              <ExternalLink className="w-3 h-3 opacity-50 group-hover:opacity-100" />
            </button>
          </div>
        </nav>

        {/* Sidebar bottom status */}
        <div className="p-4 border-t border-slate-100 text-xs text-slate-500 flex flex-col space-y-2 bg-slate-50/60">
          <div className="flex items-center justify-between">
            <button
              type="button"
              onClick={() => setViewMode('status')}
              className="flex items-center space-x-1.5 text-emerald-600 hover:text-emerald-700 font-medium text-xs cursor-pointer hover:underline"
              title={isZh ? '查看公开对外服务健康状态页面' : 'View public service status page'}
            >
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
              <span>{t.statusOperationalBadge}</span>
            </button>
            <button
              type="button"
              onClick={() => setViewMode('status')}
              className="text-[11px] text-indigo-600 hover:text-indigo-800 hover:underline font-medium cursor-pointer"
              title={isZh ? '查看公开对外服务健康状态页面' : 'View public service status page'}
            >
              {isZh ? '对外状态页 ↗' : 'Public Status ↗'}
            </button>
          </div>
        </div>
      </aside>

      {/* Main Container */}
      <main className="flex-1 flex flex-col min-w-0 overflow-y-auto">
        {/* Top Header */}
        <header className="h-16 bg-white/90 dark:bg-[#111726]/90 backdrop-blur-md border-b border-slate-200/80 dark:border-slate-800/80 flex items-center justify-between px-4 sm:px-8 sticky top-0 z-20 shadow-xs">
          <div className="flex items-center space-x-3">
            {/* Mobile Hamburger Toggle */}
            <button
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="md:hidden p-2 rounded-xl text-slate-500 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
              title={isZh ? '打开导航栏' : 'Toggle navigation menu'}
            >
              <Menu className="w-5 h-5" />
            </button>

            <h2 className="text-base font-bold text-slate-900 dark:text-slate-100 tracking-tight">
              {currentTab === 'dashboard' && t.navDashboard}
              {currentTab === 'models' && t.navModels}
              {currentTab === 'pricing' && t.navPricing}
              {currentTab === 'channels' && t.navChannels}
              {currentTab === 'keys' && t.navKeys}
              {currentTab === 'wallet' && (adminUser?.role === 'admin' ? (isZh ? '卡密与充值管理' : 'Wallet & Credits') : (t.navWallet || (isZh ? '我的钱包' : 'My Wallet')))}
              {currentTab === 'logs' && t.navLogs}
              {currentTab === 'skills' && (t.navSkills || 'Skill Hub')}
              {currentTab === 'mcp' && (t.navMcp || 'MCP 广场')}
              {currentTab === 'users' && (t.navUsers || (isZh ? '用户管理' : 'User Management'))}
              {currentTab === 'playground' && t.navPlayground}
              {currentTab === 'docs' && t.navDocs}
            </h2>

            <button
              onClick={() => setShowCommandPalette(true)}
              className="hidden lg:flex items-center space-x-2 px-3 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800/80 hover:bg-slate-200/80 dark:hover:bg-slate-700 border border-slate-200 dark:border-slate-700 text-xs text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200 transition cursor-pointer shadow-2xs ml-3"
              title={isZh ? '全局快捷检索与指令面板 (⌘K)' : 'Command Palette (⌘K)'}
            >
              <Search className="w-3.5 h-3.5 text-slate-400" />
              <span>{isZh ? '快速搜索指令或模型...' : 'Quick search commands or models...'}</span>
              <kbd className="text-[10px] bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded px-1.5 py-0.5 font-mono font-semibold text-slate-400">
                ⌘K
              </kbd>
            </button>
          </div>

          <div className="flex items-center space-x-2 sm:space-x-2.5">
            {/* Visual Topology Modal Trigger */}
            <button
              onClick={() => setShowTopologyModal(true)}
              className="hidden sm:flex text-xs px-2.5 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/60 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 font-semibold items-center space-x-1.5 transition border border-indigo-200 dark:border-indigo-800 shadow-2xs cursor-pointer"
              title={isZh ? '查看可视化路由拓扑与容灾流程' : 'View Visual Routing Topology & Failover'}
            >
              <Layers className="w-3.5 h-3.5" />
              <span>{isZh ? '容灾拓扑' : 'Topology'}</span>
            </button>

            {/* Guided Onboarding Trigger */}
            <button
              onClick={() => setShowOnboardingModal(true)}
              className="text-xs px-2.5 py-1.5 rounded-xl bg-gradient-to-r from-indigo-600 to-sky-600 hover:from-indigo-700 hover:to-sky-700 text-white font-semibold flex items-center space-x-1.5 transition shadow-xs cursor-pointer"
              title={isZh ? '打开开发者极速上手指引向导' : 'Open Developer Guided Onboarding'}
            >
              <Sparkles className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">{isZh ? '新手向导' : 'Onboarding'}</span>
            </button>

            {/* Theme Toggle Button */}
            <button
              onClick={toggleTheme}
              className="p-2 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 transition border border-slate-200 dark:border-slate-700 shadow-2xs cursor-pointer"
              title={theme === 'dark' ? (isZh ? '切换为浅色模式' : 'Switch to Light Mode') : (isZh ? '切换为深色模式' : 'Switch to Dark Mode')}
            >
              {theme === 'dark' ? <Sun className="w-3.5 h-3.5 text-amber-400" /> : <Moon className="w-3.5 h-3.5 text-indigo-600" />}
            </button>

            {/* Language Switcher Pill */}
            <button
              onClick={toggleLang}
              className="text-xs px-2.5 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 font-semibold flex items-center space-x-1 transition border border-slate-200 dark:border-slate-700 shadow-2xs cursor-pointer"
              title={lang === 'zh' ? 'Switch to English' : '切换为简体中文'}
            >
              <Globe className="w-3.5 h-3.5 text-indigo-600 dark:text-indigo-400" />
              <span>{t.langToggle}</span>
            </button>

            <a
              href="https://github.com/ifnodoraemon/airoute"
              target="_blank"
              rel="noreferrer"
              className="hidden md:flex text-xs px-3 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 font-medium items-center space-x-1.5 transition"
            >
              <ExternalLink className="w-3.5 h-3.5" />
              <span>GitHub</span>
            </a>

            {/* Account info & actions */}
            <div className="flex items-center space-x-2 sm:space-x-2.5 pl-2 border-l border-slate-200 dark:border-slate-800">
              {adminUser?.role === 'admin' ? (
                <div className="flex items-center space-x-2">
                  <button
                    onClick={() => setShowAccountModal(true)}
                    className="flex items-center space-x-1.5 text-xs text-slate-700 font-medium px-2.5 py-1 rounded-xl bg-slate-100 hover:bg-slate-200 border border-slate-200 transition cursor-pointer"
                    title={isZh ? '账号设置与个人中心' : 'Account Settings'}
                  >
                    <User className="w-3.5 h-3.5 text-indigo-600" />
                    <span>{adminUser?.username || 'admin'}</span>
                    <span className="text-[10px] text-purple-700 bg-purple-50 border border-purple-200 px-1.5 py-0.5 rounded font-mono font-bold">
                      {isZh ? '管理员' : 'Admin'}
                    </span>
                  </button>
                  <button
                    onClick={() => setCurrentTab('wallet')}
                    className="px-2.5 py-1 text-xs font-semibold text-slate-600 hover:text-indigo-600 bg-slate-50 hover:bg-slate-100 border border-slate-200 rounded-xl transition cursor-pointer"
                    title={isZh ? '卡密生成与充值中心' : 'Redeem Cards & Wallet'}
                  >
                    {isZh ? '卡密中心' : 'Redeem Center'}
                  </button>
                </div>
              ) : (
                <div className="flex items-center space-x-2">
                  <span className="text-[11px] font-semibold px-2 py-0.5 rounded-lg bg-indigo-50 text-indigo-700 border border-indigo-200">
                    {adminUser?.group_name ? `${adminUser.group_name} ${isZh ? '组' : 'Tier'}` : (isZh ? '默认组' : 'Default Tier')}
                  </span>
                  <button
                    onClick={() => setCurrentTab('wallet')}
                    className="flex items-center space-x-1.5 text-xs text-emerald-800 font-bold px-2.5 py-1 rounded-xl bg-emerald-50 hover:bg-emerald-100 border border-emerald-200 transition cursor-pointer"
                    title={isZh ? '点击管理钱包与充值' : 'Manage Wallet & Top-up'}
                  >
                    <Coins className="w-3.5 h-3.5 text-emerald-600" />
                    <span>¥{Number(adminUser?.balance || 0).toFixed(2)}</span>
                  </button>
                  <button
                    onClick={() => setShowAccountModal(true)}
                    className="flex items-center space-x-1.5 text-xs text-slate-700 hover:text-slate-900 px-2.5 py-1 bg-slate-100 hover:bg-slate-200 border border-slate-200 rounded-xl font-medium transition cursor-pointer"
                    title={isZh ? '账号设置与个人中心' : 'Account Settings'}
                  >
                    <User className="w-3.5 h-3.5 text-slate-500" />
                    <span>{adminUser?.username}</span>
                  </button>
                </div>
              )}
              <button
                onClick={handleLogout}
                className="p-1.5 rounded-xl bg-slate-100 hover:bg-rose-50 text-slate-600 hover:text-rose-600 transition cursor-pointer"
                title={isZh ? '退出登录' : 'Logout'}
              >
                <LogOut className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </header>

        <div className="p-8 max-w-7xl w-full mx-auto space-y-6">
          {/* Default Password Security Warning Banner for Admins */}
          {adminUser && adminUser.role === 'admin' && adminUser.is_default_password && (
            <div className="bg-rose-50 border border-rose-200 rounded-3xl p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 shadow-xs animate-in fade-in">
              <div className="flex items-center space-x-3.5">
                <div className="w-10 h-10 rounded-2xl bg-rose-100 text-rose-700 flex items-center justify-center shrink-0">
                  <ShieldAlert className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-bold text-rose-900">
                    {isZh ? '安全风险预警：当前管理员账号使用默认初始密码' : 'Security Alert: Admin account is using default initial password'}
                  </h4>
                  <p className="text-xs text-rose-700 mt-0.5">
                    {isZh ? '为保障网关控制台与算力资产安全，强烈建议您立即修改初始密码，避免未授权访问风险。' : 'To protect gateway security and compute assets, please change default password immediately.'}
                  </p>
                </div>
              </div>
              <button
                onClick={() => setShowAccountModal(true)}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-700 text-white rounded-xl text-xs font-bold transition shrink-0 cursor-pointer flex items-center space-x-1.5"
              >
                <Lock className="w-3.5 h-3.5" />
                <span>{isZh ? '立即修改密码' : 'Change Password'}</span>
              </button>
            </div>
          )}

          {/* Low Balance Warning Banner for Regular Users */}
          {adminUser && adminUser.role !== 'admin' && Number(adminUser.balance || 0) < 5 && (
            <div className="bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/60 rounded-3xl p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 shadow-xs animate-in fade-in">
              <div className="flex items-center space-x-3.5">
                <div className="w-10 h-10 rounded-2xl bg-amber-100 dark:bg-amber-900/60 text-amber-700 dark:text-amber-300 flex items-center justify-center shrink-0">
                  <AlertCircle className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-bold text-amber-900 dark:text-amber-200">
                    {Number(adminUser.balance || 0) <= 0 ? (isZh ? '钱包额度已耗尽 (余额不足)' : 'Wallet Balance Depleted') : (isZh ? '钱包余额偏低预警' : 'Low Wallet Balance Alert')}
                  </h4>
                  <p className="text-xs text-amber-700 dark:text-amber-400 mt-0.5">
                    {isZh 
                      ? `当前可用额度为 ¥${Number(adminUser.balance || 0).toFixed(4)}。为避免生产 API 接口调用中断，请及时充值或兑换卡密。`
                      : `Current balance is ¥${Number(adminUser.balance || 0).toFixed(4)}. Please top up or redeem cards to prevent API interruption.`}
                  </p>
                </div>
              </div>
              <div className="flex items-center space-x-2 shrink-0">
                <button
                  onClick={() => setCurrentTab('wallet')}
                  className="px-4 py-2 bg-amber-600 hover:bg-amber-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                >
                  {isZh ? '前往充值 / 兑换卡密 →' : 'Top up / Redeem Card →'}
                </button>
              </div>
            </div>
          )}

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
                navigator.clipboard.writeText(txt);
                showToast(isZh ? '已复制到剪贴板！' : 'Copied to clipboard!', 'success');
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
                navigator.clipboard.writeText(txt);
                showToast(isZh ? '已复制到剪贴板！' : 'Copied to clipboard!', 'success');
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
        </div>
      </main>

      {/* Modal: New Provider with Smart Auto-Probe */}
      {showChannelModal && (
        <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in">
          <form onSubmit={handleCreateChannel} className="bg-white dark:bg-[#111726] border border-slate-200 dark:border-slate-800 rounded-3xl p-6 max-w-xl w-full space-y-4 shadow-2xl animate-in zoom-in-95 duration-150 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800/80 pb-3">
              <div>
                <h3 className="font-bold text-base text-slate-900 dark:text-slate-100 tracking-tight">
                  {editingChannelId ? (isZh ? '编辑服务商' : 'Edit Upstream Provider') : (isZh ? '接入服务商' : 'Connect New Provider')}
                </h3>
              </div>
              <button
                type="button"
                onClick={() => setShowChannelModal(false)}
                className="p-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-xl text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* Probe Notification */}
            {probeAlert && (
              <div
                className={`p-3 rounded-xl text-xs font-medium border ${
                  probeAlert.type === 'success'
                    ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-800 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                    : probeAlert.type === 'warning'
                    ? 'bg-amber-50 dark:bg-amber-950/50 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800'
                    : 'bg-rose-50 dark:bg-rose-950/50 text-rose-800 dark:text-rose-300 border-rose-200 dark:border-rose-800'
                }`}
              >
                {probeAlert.text}
              </div>
            )}

            <div className="space-y-3.5 text-xs">
              {/* Field 0: 1-Click Provider Quick Presets */}
              {!editingChannelId && (
                <div className="p-3 bg-slate-50 dark:bg-slate-900/60 rounded-2xl border border-slate-200/80 dark:border-slate-800/80 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-slate-700 dark:text-slate-300 text-[11px] flex items-center space-x-1.5">
                      <Sparkles className="w-3.5 h-3.5 text-indigo-500" />
                      <span>{isZh ? '快捷预设服务商 (点击一键自动填入):' : 'Quick Presets (1-click autofill):'}</span>
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {[
                      { key: 'deepseek', label: 'DeepSeek', badge: isZh ? '官方' : 'Official' },
                      { key: 'gpustack', label: 'GPUStack', badge: isZh ? '私有集群' : 'Cluster' },
                      { key: 'zhipu', label: '智谱 GLM', badge: isZh ? '国产主流' : 'GLM' },
                      { key: 'doubao', label: '火山豆包', badge: isZh ? '多模态/视频' : 'Multimodal' },
                      { key: 'moonshot', label: '月之暗面 Kimi', badge: isZh ? '长文本' : 'Kimi' },
                      { key: 'openai', label: 'OpenAI', badge: isZh ? '全协议' : 'Full Suite' },
                      { key: 'anthropic', label: 'Claude', badge: 'Anthropic' },
                      { key: 'gemini', label: 'Google Gemini', badge: isZh ? '多模态' : 'Multimodal' },
                      { key: 'ollama', label: 'Ollama', badge: isZh ? '本地开源' : 'Local' },
                      { key: 'vllm', label: 'vLLM', badge: isZh ? '自建集群' : 'vLLM' },
                    ].map(p => (
                      <button
                        key={p.key}
                        type="button"
                        onClick={() => applyPreset(p.key)}
                        className="px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 hover:bg-indigo-50 dark:hover:bg-indigo-950/40 text-slate-700 dark:text-slate-300 hover:text-indigo-600 dark:hover:text-indigo-400 hover:border-indigo-300 dark:hover:border-indigo-700 text-xs font-medium transition flex items-center space-x-1 cursor-pointer shadow-2xs"
                      >
                        <span>{p.label}</span>
                        <span className="text-[10px] text-slate-400 dark:text-slate-500 font-normal">({p.badge})</span>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Field 1: Name */}
              <div>
                <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1">
                  {isZh ? '服务商名称' : 'Provider Name'} <span className="text-rose-500">*</span>
                </label>
                <input
                  required
                  value={newChannel.name}
                  onChange={(e) => setNewChannel({ ...newChannel, name: e.target.value })}
                  placeholder="gpustack-primary"
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                />
              </div>

              {/* Field 2: Base URL + Test Ping */}
              <div>
                <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1">
                  {isZh ? '服务商 Base URL' : 'Provider Base URL'} <span className="text-rose-500">*</span>
                </label>
                <div className="flex space-x-2">
                  <input
                    required
                    value={newChannel.base_url}
                    onChange={(e) => setNewChannel({ ...newChannel, base_url: e.target.value })}
                    placeholder={
                      newChannel.type === 'ollama' ? (isZh ? '例如: http://<宿主机IP或容器服务名>:11434/v1' : 'e.g. http://<host-ip-or-container>:11434/v1') :
                      newChannel.type === 'vllm' ? (isZh ? '例如: http://<宿主机IP或集群域名>:8000/v1' : 'e.g. http://<host-ip-or-cluster>:8000/v1') :
                      newChannel.type === 'gpustack' ? (isZh ? '例如: http://<GPUStack服务IP或集群域名>/v1-openai' : 'e.g. http://<gpustack-ip-or-domain>/v1-openai') :
                      (isZh ? 'https://api.deepseek.com 或私有网关端点' : 'https://api.deepseek.com or private gateway endpoint')
                    }
                    className="flex-1 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
                  />
                  <button
                    type="button"
                    onClick={handleProbeChannel}
                    disabled={probing}
                    className="px-3.5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold flex items-center space-x-1.5 shadow-2xs transition disabled:opacity-50 shrink-0 cursor-pointer"
                  >
                    {probing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
                    <span>{probing ? (isZh ? '连通中...' : 'Probing...') : (isZh ? '测试连通性' : 'Test Ping')}</span>
                  </button>
                </div>
              </div>

              {/* Field 3: API Key */}
              <div>
                <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1">
                  API Key
                </label>
                <div className="relative">
                  <input
                    type={showApiKeyPlain ? 'text' : 'password'}
                    value={newChannel.api_key}
                    onChange={(e) => setNewChannel({ ...newChannel, api_key: e.target.value })}
                    placeholder={isZh ? '留空或填 none 表示免密' : 'Leave empty or none for no auth'}
                    className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl pl-3 pr-10 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
                  />
                  <button
                    type="button"
                    onClick={() => setShowApiKeyPlain(!showApiKeyPlain)}
                    className="absolute right-3 top-2.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
                    title={showApiKeyPlain ? (isZh ? '隐藏密钥' : 'Hide Key') : (isZh ? '显示明文' : 'Show Plaintext')}
                  >
                    {showApiKeyPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              {/* Field 4: Models */}
              <ModelChipManager
                models={newChannel.models_str}
                onChange={(list) => setNewChannel(prev => ({ ...prev, models_str: list.join(', ') }))}
              />

              {/* Collapsible Advanced Settings */}
              <details className="group border border-slate-200 dark:border-slate-800 rounded-2xl p-3.5 bg-slate-50/60 dark:bg-slate-900/40">
                <summary className="font-semibold text-slate-700 dark:text-slate-300 cursor-pointer flex items-center justify-between select-none">
                  <span>{isZh ? '高级设置' : 'Advanced Settings'}</span>
                  <span className="text-slate-400 group-open:rotate-180 transition-transform text-xs">▼</span>
                </summary>

                <div className="pt-3 space-y-3.5 border-t border-slate-200/60 dark:border-slate-800 mt-2.5">
                  <div>
                    <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1">{isZh ? '服务引擎类型' : 'Engine Type'}</label>
                    <select
                      value={newChannel.type}
                      onChange={(e) => setNewChannel({ ...newChannel, type: e.target.value })}
                      className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                    >
                      <option value="gpustack">GPUStack</option>
                      <option value="deepseek">DeepSeek</option>
                      <option value="openai">{isZh ? 'OpenAI (及所有兼容服务商)' : 'OpenAI (and compatible)'}</option>
                      <option value="anthropic">Claude / Anthropic</option>
                      <option value="gemini">Gemini</option>
                      <option value="vllm">vLLM</option>
                      <option value="sglang">SGLang</option>
                      <option value="ollama">Ollama</option>
                      <option value="sub2api">Sub2API</option>
                      <option value="custom">Custom</option>
                    </select>
                  </div>

                  <ModelMappingEditor
                    mappingStr={newChannel.mapping_str}
                    onChange={(val) => setNewChannel(prev => ({ ...prev, mapping_str: val }))}
                  />

                  <CapabilitiesSelector
                    protocols={newChannel.protocols}
                    onToggle={toggleProtocol}
                    onSetProtocols={(list) => setNewChannel(prev => ({ ...prev, protocols: list }))}
                  />

                  <div className="grid grid-cols-3 gap-3">
                    <div>
                      <label className="block text-[11px] text-slate-500 mb-1">{isZh ? '优先级 (1最先)' : 'Priority (1 highest)'}</label>
                      <input
                        type="number"
                        value={newChannel.priority}
                        onChange={(e) => setNewChannel({ ...newChannel, priority: parseInt(e.target.value) || 1 })}
                        className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1.5 font-mono text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                    <div>
                      <label className="block text-[11px] text-slate-500 mb-1">{isZh ? '负载权重' : 'Weight'}</label>
                      <input
                        type="number"
                        value={newChannel.weight}
                        onChange={(e) => setNewChannel({ ...newChannel, weight: parseInt(e.target.value) || 10 })}
                        className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1.5 font-mono text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                    <div>
                      <label className="block text-[11px] text-slate-500 mb-1">{isZh ? '超时时间 (秒)' : 'Timeout (s)'}</label>
                      <input
                        type="number"
                        value={newChannel.timeout_seconds || 60}
                        onChange={(e) => setNewChannel({ ...newChannel, timeout_seconds: parseInt(e.target.value) || 60 })}
                        className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1.5 font-mono text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                  </div>
                </div>
              </details>
            </div>

            <div className="flex justify-end space-x-3 pt-4 border-t border-slate-100 dark:border-slate-800/80">
              <button
                type="button"
                onClick={() => setShowChannelModal(false)}
                className="px-4 py-2 text-xs text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 font-medium transition cursor-pointer"
              >
                {isZh ? '取消' : 'Cancel'}
              </button>
              <button
                type="button"
                onClick={handleCreateChannel}
                className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm transition cursor-pointer"
              >
                {editingChannelId ? (isZh ? '保存配置' : 'Save Changes') : (isZh ? '确认接入' : 'Connect Provider')}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Modal: New API Key */}
      {showKeyModal && (
        <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in">
          <form onSubmit={handleCreateKey} className="bg-white dark:bg-[#111726] border border-slate-200 dark:border-slate-800 rounded-3xl p-6 max-w-md w-full space-y-4 shadow-2xl animate-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800/80 pb-3">
              <div>
                <h3 className="font-bold text-base text-slate-900 dark:text-slate-100 tracking-tight">{isZh ? '新建 API 密钥' : 'Create API Key'}</h3>
              </div>
              <button
                type="button"
                onClick={() => setShowKeyModal(false)}
                className="p-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-xl text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-4 text-xs">
              <div>
                <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1">
                  {isZh ? '密钥名称' : 'Key Name / Description'} <span className="text-rose-500">*</span>
                </label>
                <input
                  value={newKey.tenant_id}
                  onChange={(e) => setNewKey({ ...newKey, tenant_id: e.target.value })}
                  placeholder={isZh ? '如 个人开发 / Cursor / Claude Code' : 'e.g. Cursor, Claude Code, Dev Team'}
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                />
              </div>

              <div>
                <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1 text-xs">
                  {isZh ? '计费分组 (Pricing Group)' : 'Pricing Group'}
                </label>
                {adminUser?.role === 'admin' ? (
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-1.5">
                      {pricingGroups.map(g => (
                        <button
                          key={g}
                          type="button"
                          onClick={() => setNewKey({ ...newKey, group_name: g })}
                          className={`px-3 py-1.5 rounded-xl text-xs font-medium border transition cursor-pointer ${
                            (newKey.group_name || 'default') === g
                              ? 'bg-indigo-600 text-white border-indigo-600 font-semibold shadow-xs'
                              : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-50'
                          }`}
                        >
                          {g === 'default' ? (isZh ? '默认组 (default)' : 'Default (default)') : g === 'vip' ? (isZh ? 'VIP组 (vip)' : 'VIP (vip)') : g === 'enterprise' ? (isZh ? '企业组 (enterprise)' : 'Enterprise (enterprise)') : `${g} ${isZh ? '组' : 'Group'}`}
                        </button>
                      ))}
                    </div>
                    <input
                      type="text"
                      placeholder={isZh ? '或输入自定义计费分组标识' : 'or enter custom pricing group ID'}
                      value={newKey.group_name || ''}
                      onChange={(e) => setNewKey({ ...newKey, group_name: e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, '') })}
                      className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-1.5 font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                    />
                  </div>
                ) : (
                  <div className="space-y-2">
                    {adminUser?.group_name && adminUser.group_name !== 'default' ? (
                      <div className="grid grid-cols-2 gap-2">
                        <button
                          type="button"
                          onClick={() => setNewKey({ ...newKey, group_name: 'default' })}
                          className={`p-3 rounded-2xl border text-left transition cursor-pointer ${
                            (newKey.group_name || 'default') === 'default'
                              ? 'bg-indigo-50/70 dark:bg-indigo-950/40 border-indigo-500 text-indigo-950 dark:text-indigo-200 ring-1 ring-indigo-500'
                              : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 hover:bg-slate-50'
                          }`}
                        >
                          <div className="text-xs font-bold">{isZh ? '默认基础组 (default)' : 'Default Group (default)'}</div>
                          <div className="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">{isZh ? '标准定价，适合常规调用或测试环境' : 'Standard rate, ideal for general dev and testing'}</div>
                        </button>
                        <button
                          type="button"
                          onClick={() => setNewKey({ ...newKey, group_name: adminUser.group_name })}
                          className={`p-3 rounded-2xl border text-left transition cursor-pointer ${
                            newKey.group_name === adminUser.group_name
                              ? 'bg-amber-50/70 dark:bg-amber-950/40 border-amber-500 text-amber-950 dark:text-amber-200 ring-1 ring-amber-500'
                              : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 hover:bg-slate-50'
                          }`}
                        >
                          <div className="text-xs font-bold text-amber-600 dark:text-amber-400">
                            {adminUser.group_name.toUpperCase()} {isZh ? '专属保障组' : 'Tier Group'}
                          </div>
                          <div className="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">{isZh ? '享受您账号签约的特权与优惠费率' : 'Enjoy your account negotiated discount rates'}</div>
                        </button>
                      </div>
                    ) : (
                      <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 flex items-center justify-between">
                        <div>
                          <div className="text-xs font-semibold text-slate-800 dark:text-slate-200">{isZh ? '默认计费组 (Standard)' : 'Default Group (Standard)'}</div>
                          <div className="text-[11px] text-slate-400 mt-0.5">{isZh ? '该密钥调用将按通用标准费率结算扣费' : 'Usage will be billed under general standard rates'}</div>
                        </div>
                        <span className="px-2 py-0.5 rounded-lg text-[11px] font-semibold bg-indigo-50 text-indigo-700 border border-indigo-200">
                          {isZh ? '标准费率' : 'Standard'}
                        </span>
                      </div>
                    )}
                  </div>
                )}
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="font-semibold text-slate-700 dark:text-slate-300">{isZh ? 'API 密钥' : 'API Key'}</label>
                  <button
                    type="button"
                    onClick={() => {
                      const rand = 'sk-airoute-' + Math.random().toString(36).substring(2, 10) + Math.random().toString(36).substring(2, 6);
                      setNewKey(prev => ({ ...prev, key: rand }));
                    }}
                    className="text-[11px] text-indigo-600 dark:text-indigo-400 hover:underline flex items-center space-x-1 cursor-pointer"
                  >
                    <span>{isZh ? '随机生成' : 'Generate'}</span>
                  </button>
                </div>
                <input
                  value={newKey.key}
                  onChange={(e) => setNewKey({ ...newKey, key: e.target.value })}
                  placeholder={isZh ? '留空自动生成' : 'Leave empty to auto-generate'}
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
                />
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="font-semibold text-slate-700 dark:text-slate-300">{isZh ? '额度限制 (CNY, 0 为不限)' : 'Quota Limit (CNY, 0 for unlimited)'}</label>
                  <div className="flex items-center space-x-1.5 text-[11px]">
                    {[
                      { label: isZh ? '不限' : 'Unlimited', val: 0 },
                      { label: '¥10', val: 10 },
                      { label: '¥50', val: 50 },
                      { label: '¥100', val: 100 },
                    ].map(p => (
                      <button
                        key={p.label}
                        type="button"
                        onClick={() => setNewKey(prev => ({ ...prev, budget: p.val }))}
                        className="text-indigo-600 dark:text-indigo-400 hover:underline px-1 cursor-pointer"
                      >
                        {p.label}
                      </button>
                    ))}
                  </div>
                </div>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={newKey.budget ?? 0}
                  onChange={(e) => setNewKey({ ...newKey, budget: parseFloat(e.target.value) || 0 })}
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
                />
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="font-semibold text-slate-700 dark:text-slate-300">{isZh ? '速率限制 (RPM, 0 为不限)' : 'Rate Limit (RPM, 0 for unlimited)'}</label>
                  <div className="flex items-center space-x-1.5 text-[11px]">
                    {[
                      { label: isZh ? '不限' : 'Unlimited', val: 0 },
                      { label: '60', val: 60 },
                      { label: '120', val: 120 },
                      { label: '600', val: 600 },
                    ].map(p => (
                      <button
                        key={p.label}
                        type="button"
                        onClick={() => setNewKey(prev => ({ ...prev, rpm: p.val }))}
                        className="text-indigo-600 dark:text-indigo-400 hover:underline px-1 cursor-pointer"
                      >
                        {p.label}
                      </button>
                    ))}
                  </div>
                </div>
                <input
                  type="number"
                  value={newKey.rpm}
                  onChange={(e) => setNewKey({ ...newKey, rpm: parseInt(e.target.value) || 0 })}
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
                />
              </div>
            </div>

            <div className="flex justify-end space-x-3 pt-4 border-t border-slate-100 dark:border-slate-800/80">
              <button
                type="button"
                onClick={() => setShowKeyModal(false)}
                className="px-4 py-2 text-xs text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 font-medium transition cursor-pointer"
              >
                {isZh ? '取消' : 'Cancel'}
              </button>
              <button
                type="submit"
                className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm transition cursor-pointer"
              >
                {isZh ? '确认创建' : 'Create Key'}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Account & Security Modal */}
      <AccountManageModal
        isOpen={showAccountModal}
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
      />
    </div>
    </I18nContext.Provider>
  );
}
