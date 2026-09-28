import React, { useState, useEffect, useRef } from 'react';
import {
  Activity,
  Zap,
  Server,
  Key,
  Terminal,
  Shield,
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
  Wallet
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
import AccountManageModal from './components/AccountManageModal';
import ModelRoutesManager from './components/ModelRoutesManager';
import ServiceStatus from './components/ServiceStatus';
import PricingManager from './components/PricingManager';
import McpIntegrationView from './components/McpIntegrationView';
import UserManagementView from './components/UserManagementView';
import WalletManagementView from './components/WalletManagementView';
import MiddlewareStatusMatrix from './components/MiddlewareStatusMatrix';
import { translations } from './i18n';

export default function App() {
  // Language State (bilingual i18n)
  const [lang, setLang] = useState(() => localStorage.getItem('nano_gateway_lang') || 'zh');
  const t = translations[lang] || translations.zh;

  const toggleLang = () => {
    const nextLang = lang === 'zh' ? 'en' : 'zh';
    setLang(nextLang);
    localStorage.setItem('nano_gateway_lang', nextLang);
  };

  // Enforce pure light mode only & dynamic document title
  useEffect(() => {
    document.documentElement.classList.remove('dark');
    document.documentElement.classList.add('light');
    localStorage.setItem('nano_theme', 'light');
    document.title = lang === 'zh' ? 'AI路由器' : 'nano-gateway';
  }, [lang]);

  const [toast, setToast] = useState({ show: false, message: '', type: 'info' });
  const toastTimerRef = useRef(null);

  const showToast = (message, type = 'info') => {
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
    setToast({ show: true, message, type });
    toastTimerRef.current = setTimeout(() => {
      setToast(prev => ({ ...prev, show: false }));
    }, 3200);
  };

  // View mode: 'landing' (公共门户首页) or 'console' (工作台)
  const [viewMode, setViewMode] = useState(() => {
    return localStorage.getItem('nano_admin_token') ? 'console' : 'landing';
  });

  // Account & Authentication state
  const [adminToken, setAdminToken] = useState(() => localStorage.getItem('nano_admin_token') || '');
  const [adminUser, setAdminUser] = useState(() => {
    try {
      return JSON.parse(localStorage.getItem('nano_admin_user') || 'null');
    } catch {
      return null;
    }
  });
  const [showLoginModal, setShowLoginModal] = useState(false);
  const [showAccountModal, setShowAccountModal] = useState(false);
  const [authTab, setAuthTab] = useState('login');

  // Validate admin token on startup to prevent stale token UI issues
  useEffect(() => {
    const savedToken = localStorage.getItem('nano_admin_token');
    if (savedToken) {
      fetch('/api/v1/admin/auth/me', {
        headers: { 'Authorization': `Bearer ${savedToken}` }
      })
        .then(res => res.json())
        .then(data => {
          if (data.code === 0 && data.data) {
            setAdminUser(data.data);
            localStorage.setItem('nano_admin_user', JSON.stringify(data.data));
          } else {
            setAdminToken('');
            setAdminUser(null);
            localStorage.removeItem('nano_admin_token');
            localStorage.removeItem('nano_admin_user');
            if (viewMode === 'console') {
              setViewMode('landing');
              setShowLoginModal(true);
            }
          }
        })
        .catch(() => {});
    }
  }, []);

  // Authenticated fetch helper for Control Plane APIs
  const adminFetch = async (url, options = {}) => {
    const headers = { ...(options.headers || {}) };
    if (adminToken) {
      headers['Authorization'] = `Bearer ${adminToken}`;
    }
    try {
      const res = await fetch(url, { ...options, headers });
      if (res.status === 401 && viewMode === 'console') {
        setAdminToken('');
        setAdminUser(null);
        localStorage.removeItem('nano_admin_token');
        localStorage.removeItem('nano_admin_user');
        setShowLoginModal(true);
        showToast('登录凭证已失效，请重新登录', 'warning');
      }
      return res;
    } catch (e) {
      throw e;
    }
  };

  const handleLogout = () => {
    setAdminToken('');
    setAdminUser(null);
    localStorage.removeItem('nano_admin_token');
    localStorage.removeItem('nano_admin_user');
    setViewMode('landing');
    showToast('已安全退出账号', 'info');
  };

  const fetchUserProfile = async () => {
    if (!adminToken) return;
    try {
      const res = await fetch('/api/v1/admin/auth/me', {
        headers: { 'Authorization': `Bearer ${adminToken}` }
      });
      const data = await res.json();
      if (res.ok && data.code === 0 && data.data) {
        setAdminUser(data.data);
        localStorage.setItem('nano_admin_user', JSON.stringify(data.data));
      }
    } catch (e) {}
  };

  const [activeQuickKey, setActiveQuickKey] = useState(null);
  const [activeLogDetail, setActiveLogDetail] = useState(null);
  const [curlExportCmd, setCurlExportCmd] = useState('');
  const [showApiKeyPlain, setShowApiKeyPlain] = useState(false);
  const [batchTesting, setBatchTesting] = useState(false);
  const [channelLatencies, setChannelLatencies] = useState({});

  const [currentTab, setCurrentTab] = useState(() => {
    const savedUser = localStorage.getItem('nano_admin_user');
    if (savedUser) {
      try {
        const u = JSON.parse(savedUser);
        if (u && u.role !== 'admin') return 'wallet';
      } catch (e) {}
    }
    return 'dashboard';
  });

  // Automatically ensure regular user stays within permitted tabs
  useEffect(() => {
    if (adminUser && adminUser.role !== 'admin') {
      const allowedTabs = ['wallet', 'keys', 'playground', 'pricing', 'logs', 'docs'];
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
  const [logLoading, setLogLoading] = useState(false);
  const [logFilter, setLogFilter] = useState('');
  const [timeRange, setTimeRange] = useState('all'); // 'all' | '1h' | 'today' | '7d' | 'custom'
  const [customStartTime, setCustomStartTime] = useState('');
  const [customEndTime, setCustomEndTime] = useState('');
  const [sessionFilter, setSessionFilter] = useState('');

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

  // Playground Modality Switcher
  const [playModality, setPlayModality] = useState('chat'); // 'chat' | 'images' | 'audio_speech' | 'audio_transcription' | 'videos' | 'embeddings'
  const [playApiKey, setPlayApiKey] = useState('');
  const [playLoading, setPlayLoading] = useState(false);
  const [playDurationMs, setPlayDurationMs] = useState(0);
  const [playOutput, setPlayOutput] = useState('');

  // Auto-select first available API key for Playground
  useEffect(() => {
    if (!playApiKey && keys.length > 0) {
      setPlayApiKey(keys[0].key);
    }
  }, [keys, playApiKey]);

  // 1. Chat & Completions state
  const [playModel, setPlayModel] = useState('');
  const [playProtocol, setPlayProtocol] = useState('openai_chat');
  const [playStream, setPlayStream] = useState(true);
  const [playPrompt, setPlayPrompt] = useState('请用一句话介绍你自己和你的技术架构。');
  const [playImageUrl, setPlayImageUrl] = useState('');
  const [playTTFTMs, setPlayTTFTMs] = useState(0);
  const [playReasoningOutput, setPlayReasoningOutput] = useState('');

  // 2. Image Generation state
  const [imgModel, setImgModel] = useState('');
  const [imgPrompt, setImgPrompt] = useState('极简现代高科技数据中心，赛博光影质感，8k 渲染');
  const [imgSize, setImgSize] = useState('1024x1024');
  const [imgQuality, setImgQuality] = useState('standard');
  const [imgResult, setImgResult] = useState(null);

  // 3. Audio Speech (TTS) state
  const [ttsModel, setTtsModel] = useState('');
  const [ttsVoice, setTtsVoice] = useState('alloy');
  const [ttsSpeed, setTtsSpeed] = useState(1.0);
  const [ttsInput, setTtsInput] = useState('欢迎体验 AI 路由器极致性能企业级大模型与多模态网关系统。');
  const [ttsAudioUrl, setTtsAudioUrl] = useState(null);

  // 4. Audio Transcription (STT) state
  const [sttModel, setSttModel] = useState('');
  const [sttFile, setSttFile] = useState(null);
  const [sttResult, setSttResult] = useState('');

  // 5. Video Generation & Polling state
  const [videoModel, setVideoModel] = useState('');
  const [videoPrompt, setVideoPrompt] = useState('未来城市高空飞车俯瞰镜头，黎明晨光映照');
  const [videoAspectRatio, setVideoAspectRatio] = useState('16:9');
  const [videoTaskId, setVideoTaskId] = useState('');
  const [videoTaskStatus, setVideoTaskStatus] = useState('');
  const [videoResultUrl, setVideoResultUrl] = useState('');
  const [videoPollCount, setVideoPollCount] = useState(0);

  // 6. Vector Embedding state
  const [embedModel, setEmbedModel] = useState('');
  const [embedInput, setEmbedInput] = useState('AI 路由器高性能分布式网关，全双工零内存拷贝分发');
  const [embedResult, setEmbedResult] = useState(null);
  const [embedDim, setEmbedDim] = useState(0);

  // 7. Rerank state
  const [rerankModel, setRerankModel] = useState('');
  const [rerankQuery, setRerankQuery] = useState('什么是企业级大模型网关的高可用与容灾设计？');
  const [rerankDocs, setRerankDocs] = useState([
    'AI 路由器采用全双工流式转发与零内存拷贝架构，首字分块前支持透明故障转移与熔断兜底。',
    '今天天气非常晴朗，公园里的樱花盛开了，很适合去散步或野餐。',
    '基于 Raft 协议的分布式数据库能保证网络分区状态下的强一致性与多副本高可用。',
    '网关内置动态跨协议转换引擎，实现 OpenAI、Anthropic Claude 与 Gemini 协议全双工互转。',
    'Cross-Encoder 重排模型能够对初筛候选文档与查询进行精细全量语义交互打分。',
  ].join('\n---\n'));
  const [rerankTopN, setRerankTopN] = useState(3);
  const [rerankResult, setRerankResult] = useState(null);

  // Docs tab category
  const [docsSection, setDocsSection] = useState('architecture');

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

  // Helper to get platform-added models for a specific modality
  const getPlaygroundModels = (modality) => {
    if (!models || models.length === 0) return [];
    if (modelRoutes && modelRoutes.length > 0) {
      const matched = modelRoutes.filter(r => r.modality === modality).map(r => r.model);
      if (matched.length > 0) return matched;
    }
    const matched = models.filter(m => inferClientModality(m) === modality);
    if (matched.length > 0) return matched;
    if (modality === 'chat') {
      return models.filter(m => inferClientModality(m) === 'chat');
    }
    return [];
  };

  // Load backend data
  const fetchData = async () => {
    try {
      const [chRes, keyRes, statsRes, mRes, routesRes] = await Promise.all([
        adminFetch('/api/v1/admin/channels').then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/keys').then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/stats/overview').then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/models').then(r => r.json()).catch(() => ({ code: 1 })),
        adminFetch('/api/v1/admin/models/routes').then(r => r.json()).catch(() => ({ code: 1 })),
      ]);

      if (chRes.code === 0) setChannels(chRes.data || []);
      if (keyRes.code === 0) setKeys(keyRes.data || []);
      if (statsRes.code === 0) setStats(statsRes.data || {});
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
        const effImg = getModModels('images');
        const effTts = getModModels('audio_speech');
        const effStt = getModModels('audio_transcription');
        const effVid = getModModels('videos');
        const effEmb = getModModels('embeddings');
        const effRrk = getModModels('rerank');

        setPlayModel(prev => (prev && effChat.includes(prev)) ? prev : (effChat[0] || ''));
        setImgModel(prev => (prev && effImg.includes(prev)) ? prev : (effImg[0] || ''));
        setTtsModel(prev => (prev && effTts.includes(prev)) ? prev : (effTts[0] || ''));
        setSttModel(prev => (prev && effStt.includes(prev)) ? prev : (effStt[0] || ''));
        setVideoModel(prev => (prev && effVid.includes(prev)) ? prev : (effVid[0] || ''));
        setEmbedModel(prev => (prev && effEmb.includes(prev)) ? prev : (effEmb[0] || ''));
        setRerankModel(prev => (prev && effRrk.includes(prev)) ? prev : (effRrk[0] || ''));
      }
    } catch (e) {
      console.error('Fetch data failed:', e);
    }
  };

  const fetchLogs = async (override = {}) => {
    setLogLoading(true);
    try {
      const p = new URLSearchParams();
      p.set('limit', '50');

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

      const res = await adminFetch(`/api/v1/admin/logs?${p.toString()}`);
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
      fetchLogs();
    }
  }, [currentTab, timeRange, sessionFilter]);

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
          base_url: 'http://10.232.16.83/v1-openai',
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
          models_str: 'deepseek-chat, deepseek-reasoner',
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
          models_str: 'glm-4-plus, glm-4-air, glm-4-flash, cogview-3-plus',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'images'],
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
          models_str: 'doubao-seedance-2.0, doubao-pro-32k, doubao-lite-32k',
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
          models_str: 'moonshot-v1-8k, moonshot-v1-32k, moonshot-v1-128k',
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
          models_str: 'gpt-4o, gpt-4o-mini, text-embedding-3-small, dall-e-3, tts-1, whisper-1',
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
          models_str: 'claude-3-5-sonnet-20241022, claude-3-5-haiku-20241022',
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
          models_str: 'gemini-1.5-pro, gemini-1.5-flash',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'anthropic_messages', 'embeddings'],
        }));
        showToast('已载入 Google Gemini 官方模板及模型列表', 'info');
        break;
      case 'ollama':
        setNewChannel(prev => ({
          ...prev,
          name: 'ollama-local',
          type: 'ollama',
          base_url: 'http://localhost:11434/v1',
          api_key: prev.api_key || 'ollama',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'llama3.1, qwen2.5:7b, deepseek-r1:8b',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings'],
        }));
        showToast('已载入 Ollama 本地开源模板', 'info');
        break;
      case 'vllm':
        setNewChannel(prev => ({
          ...prev,
          name: 'vllm-local',
          type: 'vllm',
          base_url: 'http://localhost:8000/v1',
          api_key: prev.api_key || 'none',
          priority: 1,
          weight: 10,
          timeout_seconds: 60,
          models_str: 'Qwen/Qwen2.5-72B-Instruct',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings'],
        }));
        showToast('已载入 vLLM 本地集群模板', 'info');
        break;
      case 'sub2api':
        setNewChannel(prev => ({
          ...prev,
          name: 'sub2api-upstream',
          type: 'sub2api',
          base_url: 'https://your-sub2api.example.com/v1',
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
          base_url: 'http://localhost:8000/v1',
          api_key: prev.api_key || '',
          priority: 2,
          weight: 10,
          timeout_seconds: 60,
          models_str: '',
          mapping_str: '',
          protocols: ['openai_chat', 'openai_response', 'embeddings', 'rerank', 'images', 'audio_speech', 'videos'],
        }));
        showToast('已载入自定义下游模板', 'info');
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
    const origin = window.location.origin || 'http://localhost:8080';
    const key = playApiKey || (keys.length > 0 ? keys[0].key : 'sk-nano-your-key');
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
    if (!window.confirm('⚠️ 警告：确认彻底清空所有调用日志？此操作无法撤销！')) return;
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
    const rand = 'sk-nano-' + Math.random().toString(36).substring(2, 10) + Math.random().toString(36).substring(2, 6);
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

  // 1. Chat Execution
  const handleSendChat = async () => {
    if (!playPrompt.trim()) return;
    setPlayLoading(true);
    setPlayOutput('');
    setPlayReasoningOutput('');
    setPlayDurationMs(0);
    setPlayTTFTMs(0);

    const start = Date.now();
    let firstTokenTime = null;

    const headers = { 'Content-Type': 'application/json' };
    const activeKey = playApiKey || (keys.length > 0 ? keys[0].key : '');
    if (activeKey) {
      headers['Authorization'] = `Bearer ${activeKey}`;
      headers['x-api-key'] = activeKey;
    }

    let url = '/v1/chat/completions';
    let body = {};

    if (playProtocol === 'openai_response') {
      url = '/v1/responses';
      body = {
        model: playModel,
        input: playPrompt,
        instructions: 'You are a helpful and concise AI assistant.',
        stream: playStream,
      };
    } else if (playProtocol === 'anthropic_messages') {
      url = '/v1/messages';
      let content = playPrompt;
      if (playImageUrl) {
        content = [
          { type: 'text', text: playPrompt },
          { type: 'image_url', image_url: { url: playImageUrl } },
        ];
      }
      body = {
        model: playModel,
        messages: [{ role: 'user', content }],
        stream: playStream,
        max_tokens: 1024,
      };
    } else if (playProtocol === 'openai_text') {
      url = '/v1/completions';
      body = {
        model: playModel,
        prompt: playPrompt,
        stream: playStream,
        max_tokens: 1024,
      };
    } else {
      url = '/v1/chat/completions';
      let content = playPrompt;
      if (playImageUrl) {
        content = [
          { type: 'text', text: playPrompt },
          { type: 'image_url', image_url: { url: playImageUrl } },
        ];
      }
      body = {
        model: playModel,
        messages: [{ role: 'user', content }],
        stream: playStream,
      };
    }

    try {
      const res = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }));
        setPlayOutput(`Error: ${JSON.stringify(err, null, 2)}`);
        return;
      }

      if (!playStream) {
        const data = await res.json();
        setPlayDurationMs(Date.now() - start);
        if (playProtocol === 'openai_response') {
          setPlayOutput(data.output?.[0]?.content?.[0]?.text || JSON.stringify(data, null, 2));
        } else if (playProtocol === 'openai_text') {
          setPlayOutput(data.choices?.[0]?.text || '');
        } else if (playProtocol === 'anthropic_messages') {
          setPlayOutput(data.content?.[0]?.text || '');
        } else {
          setPlayOutput(data.choices?.[0]?.message?.content || '');
          if (data.choices?.[0]?.message?.reasoning_content) {
            setPlayReasoningOutput(data.choices[0].message.reasoning_content);
          }
        }
      } else {
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed.startsWith('data:')) continue;
            const dataContent = trimmed.substring(5).trim();
            if (dataContent === '[DONE]') continue;

            try {
              const chunk = JSON.parse(dataContent);
              let textDelta = '';
              let reasoningDelta = '';
              if (chunk.choices?.[0]?.delta?.content) {
                textDelta = chunk.choices[0].delta.content;
              } else if (chunk.choices?.[0]?.text) {
                textDelta = chunk.choices[0].text;
              } else if (chunk.delta?.text) {
                textDelta = chunk.delta.text;
              } else if (chunk.type === 'response.output_text.delta' && chunk.delta) {
                textDelta = chunk.delta;
              }

              if (chunk.choices?.[0]?.delta?.reasoning_content) {
                reasoningDelta = chunk.choices[0].delta.reasoning_content;
              }

              if (reasoningDelta) {
                setPlayReasoningOutput(prev => prev + reasoningDelta);
              }

              if (textDelta) {
                if (!firstTokenTime) {
                  firstTokenTime = Date.now();
                  setPlayTTFTMs(firstTokenTime - start);
                }
                setPlayOutput(prev => prev + textDelta);
              }
            } catch (e) {}
          }
        }
        setPlayDurationMs(Date.now() - start);
      }
    } catch (e) {
      setPlayOutput(`Request Failed: ${e.message}`);
    } finally {
      setPlayLoading(false);
    }
  };

  // 2. Image Generation Execution
  const handleGenerateImage = async () => {
    if (!imgPrompt.trim()) return;
    setPlayLoading(true);
    setImgResult(null);
    setPlayOutput('');
    setPlayDurationMs(0);
    const start = Date.now();

    try {
      const headers = { 'Content-Type': 'application/json' };
      const activeKey = playApiKey || (keys.length > 0 ? keys[0].key : '');
      if (activeKey) {
        headers['Authorization'] = `Bearer ${activeKey}`;
      }
      const res = await fetch('/v1/images/generations', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          model: imgModel,
          prompt: imgPrompt,
          size: imgSize,
          quality: imgQuality,
          n: 1,
        }),
      });
      setPlayDurationMs(Date.now() - start);
      const data = await res.json();
      if (!res.ok) {
        setPlayOutput(`生图请求失败 (${res.status}):\n${JSON.stringify(data, null, 2)}`);
      } else {
        const item = data.data?.[0];
        setImgResult(item || null);
        setPlayOutput(JSON.stringify(data, null, 2));
      }
    } catch (e) {
      setPlayOutput(`生图网络异常: ${e.message}`);
    } finally {
      setPlayLoading(false);
    }
  };

  // 3. Audio Speech Execution (TTS)
  const handleGenerateSpeech = async () => {
    if (!ttsInput.trim()) return;
    setPlayLoading(true);
    if (ttsAudioUrl) {
      URL.revokeObjectURL(ttsAudioUrl);
      setTtsAudioUrl(null);
    }
    setPlayOutput('');
    setPlayDurationMs(0);
    const start = Date.now();

    try {
      const headers = { 'Content-Type': 'application/json' };
      const activeKey = playApiKey || (keys.length > 0 ? keys[0].key : '');
      if (activeKey) {
        headers['Authorization'] = `Bearer ${activeKey}`;
      }
      const res = await fetch('/v1/audio/speech', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          model: ttsModel,
          input: ttsInput,
          voice: ttsVoice,
          speed: parseFloat(ttsSpeed) || 1.0,
          response_format: 'mp3',
        }),
      });
      setPlayDurationMs(Date.now() - start);
      if (!res.ok) {
        const err = await res.text();
        setPlayOutput(`语音合成失败 (${res.status}):\n${err}`);
      } else {
        const blob = await res.blob();
        const audioUrl = URL.createObjectURL(blob);
        setTtsAudioUrl(audioUrl);
        setPlayOutput(`✅ 语音合成成功！\n音频大小: ${(blob.size / 1024).toFixed(1)} KB\n格式: audio/mpeg (MP3)`);
      }
    } catch (e) {
      setPlayOutput(`语音合成异常: ${e.message}`);
    } finally {
      setPlayLoading(false);
    }
  };

  // 4. Audio Transcription Execution (STT)
  const handleTranscribeAudio = async () => {
    if (!sttFile) {
      alert('请先选择要转写的音频文件');
      return;
    }
    setPlayLoading(true);
    setSttResult('');
    setPlayOutput('');
    setPlayDurationMs(0);
    const start = Date.now();

    try {
      const formData = new FormData();
      formData.append('file', sttFile);
      formData.append('model', sttModel);

      const headers = {};
      const activeKey = playApiKey || (keys.length > 0 ? keys[0].key : '');
      if (activeKey) {
        headers['Authorization'] = `Bearer ${activeKey}`;
      }

      const res = await fetch('/v1/audio/transcriptions', {
        method: 'POST',
        headers,
        body: formData,
      });
      setPlayDurationMs(Date.now() - start);
      const data = await res.json();
      if (!res.ok) {
        setPlayOutput(`语音识别失败 (${res.status}):\n${JSON.stringify(data, null, 2)}`);
      } else {
        setSttResult(data.text || '');
        setPlayOutput(JSON.stringify(data, null, 2));
      }
    } catch (e) {
      setPlayOutput(`语音识别异常: ${e.message}`);
    } finally {
      setPlayLoading(false);
    }
  };

  // 5. Video Generation & Task Polling
  const pollVideoTask = (taskId, startTime) => {
    let attempts = 0;
    const interval = setInterval(async () => {
      attempts++;
      setVideoPollCount(attempts);
      try {
        const headers = {};
        const activeKey = playApiKey || (keys.length > 0 ? keys[0].key : '');
        if (activeKey) headers['Authorization'] = `Bearer ${activeKey}`;
        const res = await fetch(`/v1/videos/tasks/${taskId}`, { headers });
        const data = await res.json();
        setVideoTaskStatus(data.status || 'PROCESSING');
        setPlayOutput(`[轮询第 ${attempts} 次] 任务状态: ${data.status}\n` + JSON.stringify(data, null, 2));
        setPlayDurationMs(Date.now() - startTime);

        const st = (data.status || '').toLowerCase();
        if (st === 'success' || st === 'succeeded') {
          clearInterval(interval);
          setVideoResultUrl(data.video_url || '');
          setPlayLoading(false);
        } else if (st === 'failed' || attempts >= 30) {
          clearInterval(interval);
          setPlayLoading(false);
        }
      } catch (e) {
        clearInterval(interval);
        setPlayLoading(false);
      }
    }, 2000);
  };

  const handleGenerateVideo = async () => {
    if (!videoPrompt.trim()) return;
    setPlayLoading(true);
    setVideoTaskId('');
    setVideoTaskStatus('PENDING');
    setVideoResultUrl('');
    setVideoPollCount(0);
    setPlayOutput('');
    setPlayDurationMs(0);
    const start = Date.now();

    try {
      const headers = { 'Content-Type': 'application/json' };
      const activeKey = playApiKey || (keys.length > 0 ? keys[0].key : '');
      if (activeKey) {
        headers['Authorization'] = `Bearer ${activeKey}`;
      }
      const res = await fetch('/v1/videos/generations', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          model: videoModel,
          prompt: videoPrompt,
          aspect_ratio: videoAspectRatio,
        }),
      });
      setPlayDurationMs(Date.now() - start);
      const data = await res.json();
      if (!res.ok) {
        setPlayOutput(`视频生成提交失败 (${res.status}):\n${JSON.stringify(data, null, 2)}`);
        setPlayLoading(false);
        return;
      }
      setPlayOutput(JSON.stringify(data, null, 2));
      const tid = data.task_id || data.id;
      setVideoTaskId(tid);
      setVideoTaskStatus(data.status || 'PROCESSING');
      const initSt = (data.status || '').toLowerCase();
      if ((initSt === 'success' || initSt === 'succeeded') && data.video_url) {
        setVideoResultUrl(data.video_url);
        setPlayLoading(false);
      } else if (tid) {
        pollVideoTask(tid, start);
      } else {
        setPlayLoading(false);
      }
    } catch (e) {
      setPlayOutput(`创建视频任务异常: ${e.message}`);
      setPlayLoading(false);
    }
  };

  // 6. Vector Embedding Execution
  const handleGenerateEmbedding = async () => {
    if (!embedInput.trim()) return;
    setPlayLoading(true);
    setEmbedResult(null);
    setEmbedDim(0);
    setPlayOutput('');
    setPlayDurationMs(0);
    const start = Date.now();

    try {
      const headers = { 'Content-Type': 'application/json' };
      const activeKey = playApiKey || (keys.length > 0 ? keys[0].key : '');
      if (activeKey) {
        headers['Authorization'] = `Bearer ${activeKey}`;
      }
      const lines = embedInput.split('\n').map(l => l.trim()).filter(Boolean);
      const inputPayload = lines.length > 1 ? lines : embedInput;

      const res = await fetch('/v1/embeddings', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          model: embedModel,
          input: inputPayload,
        }),
      });
      setPlayDurationMs(Date.now() - start);
      const data = await res.json();
      if (!res.ok) {
        setPlayOutput(`向量化请求失败 (${res.status}):\n${JSON.stringify(data, null, 2)}`);
      } else {
        const firstEmb = data.data?.[0]?.embedding || [];
        setEmbedResult(data);
        setEmbedDim(firstEmb.length);
        setPlayOutput(JSON.stringify(data, null, 2));
      }
    } catch (e) {
      setPlayOutput(`向量化网络异常: ${e.message}`);
    } finally {
      setPlayLoading(false);
    }
  };

  // 7. Cross-Encoder Rerank Execution
  const handleExecuteRerank = async () => {
    if (!rerankQuery.trim() || !rerankDocs.trim()) {
      alert('请输入检索 Query 和待重排候选文档');
      return;
    }
    setPlayLoading(true);
    setRerankResult(null);
    setPlayOutput('');
    setPlayDurationMs(0);
    const start = Date.now();

    try {
      const headers = { 'Content-Type': 'application/json' };
      const activeKey = playApiKey || (keys.length > 0 ? keys[0].key : '');
      if (activeKey) {
        headers['Authorization'] = `Bearer ${activeKey}`;
      }
      let docList = [];
      if (rerankDocs.includes('\n---\n')) {
        docList = rerankDocs.split('\n---\n').map(s => s.trim()).filter(Boolean);
      } else {
        docList = rerankDocs.split('\n').map(s => s.trim()).filter(Boolean);
      }

      const res = await fetch('/v1/rerank', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          model: rerankModel,
          query: rerankQuery,
          documents: docList,
          top_n: parseInt(rerankTopN) || 3,
          return_documents: true,
        }),
      });
      setPlayDurationMs(Date.now() - start);
      const data = await res.json();
      if (!res.ok) {
        setPlayOutput(`重排请求失败 (${res.status}):\n${JSON.stringify(data, null, 2)}`);
      } else {
        setRerankResult(data);
        setPlayOutput(JSON.stringify(data, null, 2));
      }
    } catch (e) {
      setPlayOutput(`重排网络异常: ${e.message}`);
    } finally {
      setPlayLoading(false);
    }
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
      (l.virtual_key && l.virtual_key.toLowerCase().includes(f))
    );
  });

  if (viewMode === 'landing') {
    return (
      <div className="min-h-screen bg-slate-50 text-slate-800 font-sans selection:bg-indigo-500 selection:text-white">
        <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />
        <LandingPage
          isLoggedIn={!!adminToken}
          adminUser={adminUser}
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
          onLoginSuccess={(token, user) => {
            setAdminToken(token);
            setAdminUser(user);
            setShowLoginModal(false);
            setViewMode('console');
            showToast(`欢迎回来，${user.username}！`, 'success');
            fetchData();
            fetchLogs();
          }}
        />
      </div>
    );
  }

  if (viewMode === 'auth') {
    return (
      <div className="min-h-screen bg-slate-50 text-slate-800 font-sans selection:bg-indigo-500 selection:text-white">
        <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />
        <AuthPage
          initialTab={authTab}
          onLoginSuccess={(token, user) => {
            setAdminToken(token);
            setAdminUser(user);
            setViewMode('console');
            showToast(`欢迎回来，${user.username}！`, 'success');
            fetchData();
            fetchLogs();
          }}
          onBackHome={() => setViewMode('landing')}
        />
      </div>
    );
  }

  if (viewMode === 'status') {
    return (
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
        />
        <LoginModal
          isOpen={showLoginModal}
          onClose={() => setShowLoginModal(false)}
          onLoginSuccess={(token, user) => {
            setAdminToken(token);
            setAdminUser(user);
            setShowLoginModal(false);
            setViewMode('console');
            showToast(`欢迎回来，${user.username}！`, 'success');
            fetchData();
            fetchLogs();
          }}
        />
      </div>
    );
  }

  return (
    <div className="flex min-h-screen bg-slate-50 text-slate-800 font-sans selection:bg-indigo-500 selection:text-white transition-colors duration-200">
      {/* Toast Notification */}
      <Toast toast={toast} onClose={() => setToast(prev => ({ ...prev, show: false }))} />

      {/* Quick Start Modal */}
      {activeQuickKey && (
        <QuickStartModal
          virtualKey={activeQuickKey}
          onClose={() => setActiveQuickKey(null)}
          onCopy={(txt) => {
            navigator.clipboard.writeText(txt);
            showToast('调用代码已复制到剪贴板！', 'success');
          }}
        />
      )}

      {/* Log Detail Inspector Modal */}
      {activeLogDetail && (
        <LogDetailModal
          log={activeLogDetail}
          onClose={() => setActiveLogDetail(null)}
          onCopy={(txt) => {
            navigator.clipboard.writeText(txt);
            showToast('已复制到剪贴板！', 'success');
          }}
          onFilterBySession={(sid) => {
            setSessionFilter(sid);
            fetchLogs({ sessionFilter: sid });
            showToast(`已按对话 ID: ${sid} 筛选`, 'info');
          }}
          onDeleteLog={handleDeleteSingleLog}
        />
      )}

      {/* cURL Export Modal */}
      {curlExportCmd && (
        <CurlExportModal
          curlCmd={curlExportCmd}
          onClose={() => setCurlExportCmd('')}
          onCopy={(txt) => {
            navigator.clipboard.writeText(txt);
            showToast('cURL 命令已复制到剪贴板！', 'success');
          }}
        />
      )}

      {/* Sidebar */}
      <aside className="w-64 bg-white/95 border-r border-slate-200/80 flex flex-col backdrop-blur-xl shadow-xs shrink-0">
        <div className="p-5 border-b border-slate-100 flex items-center">
          <div
            onClick={() => setViewMode('landing')}
            className="flex items-center space-x-3 cursor-pointer group transition duration-150 hover:opacity-90"
            title={lang === 'zh' ? '点击返回门户首页' : 'Return to product portal'}
          >
            <div className="w-10 h-10 rounded-2xl bg-gradient-to-tr from-indigo-600 via-indigo-500 to-sky-400 flex items-center justify-center shadow-lg shadow-indigo-500/25 text-white shrink-0 group-hover:shadow-indigo-500/40 transition">
              <Zap className="w-5 h-5 fill-white text-white" />
            </div>
            <div>
              <h1 className="font-bold text-base text-slate-900 tracking-tight group-hover:text-indigo-600 transition">
                {lang === 'zh' ? 'AI路由器' : 'nano-gateway'}
              </h1>
              <span className="text-[11px] text-indigo-600 font-semibold tracking-wide block">
                {t.brandSubtitle}
              </span>
            </div>
          </div>
        </div>

        <nav className="flex-1 p-4 space-y-1.5">
          {adminUser?.role === 'admin' ? (
            /* Admin Full Navigation */
            <>
              <button
                onClick={() => setCurrentTab('dashboard')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'dashboard'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Activity className="w-4 h-4 text-indigo-500" />
                <span>{t.navDashboard}</span>
              </button>

              <button
                onClick={() => setCurrentTab('models')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'models'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Cpu className="w-4 h-4 text-pink-500" />
                <span>{t.navModels}</span>
              </button>

              <button
                onClick={() => setCurrentTab('pricing')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'pricing'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <DollarSign className="w-4 h-4 text-emerald-500" />
                <span>{t.navPricing}</span>
              </button>

              <button
                onClick={() => setCurrentTab('channels')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'channels'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Server className="w-4 h-4 text-emerald-500" />
                <span>{t.navChannels}</span>
              </button>

              <button
                onClick={() => setCurrentTab('keys')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'keys'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Key className="w-4 h-4 text-amber-500" />
                <span>{t.navKeys}</span>
              </button>

              <button
                onClick={() => setCurrentTab('wallet')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'wallet'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Wallet className="w-4 h-4 text-teal-600" />
                <span>卡密与充值</span>
              </button>

              <button
                onClick={() => setCurrentTab('logs')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'logs'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <History className="w-4 h-4 text-sky-500" />
                <span>{t.navLogs}</span>
              </button>

              <button
                onClick={() => setCurrentTab('mcp')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'mcp'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Bot className="w-4 h-4 text-violet-500" />
                <span>{t.navMcp}</span>
              </button>

              <button
                onClick={() => setCurrentTab('users')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'users'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <ShieldCheck className="w-4 h-4 text-indigo-600" />
                <span>{t.navUsers}</span>
              </button>

              <button
                onClick={() => setCurrentTab('playground')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'playground'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Terminal className="w-4 h-4 text-purple-500" />
                <span>{t.navPlayground}</span>
              </button>
            </>
          ) : (
            /* Regular User Navigation */
            <>
              <button
                onClick={() => setCurrentTab('wallet')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'wallet'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Wallet className="w-4 h-4 text-emerald-600" />
                <span>{t.navWallet || '我的钱包'}</span>
              </button>

              <button
                onClick={() => setCurrentTab('keys')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'keys'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Key className="w-4 h-4 text-amber-500" />
                <span>{t.navKeys}</span>
              </button>

              <button
                onClick={() => setCurrentTab('playground')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'playground'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <Terminal className="w-4 h-4 text-purple-500" />
                <span>{t.navPlayground}</span>
              </button>

              <button
                onClick={() => setCurrentTab('pricing')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'pricing'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <DollarSign className="w-4 h-4 text-emerald-500" />
                <span>{t.navPricing}</span>
              </button>

              <button
                onClick={() => setCurrentTab('logs')}
                className={`w-full flex items-center space-x-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all cursor-pointer ${
                  currentTab === 'logs'
                    ? 'bg-indigo-50/80 text-indigo-700 border-indigo-200 shadow-xs font-semibold'
                    : 'text-slate-600 hover:bg-slate-100/70 hover:text-slate-900 border-transparent'
                }`}
              >
                <History className="w-4 h-4 text-sky-500" />
                <span>{t.navLogs}</span>
              </button>
            </>
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
              title="前往门户首页查阅交互式开发文档"
            >
              <div className="flex items-center space-x-2.5">
                <BookOpen className="w-3.5 h-3.5 text-slate-400 group-hover:text-indigo-500" />
                <span>{t.navViewDocs}</span>
              </div>
              <ExternalLink className="w-3 h-3 opacity-50 group-hover:opacity-100" />
            </button>
          </div>
        </nav>

        {/* Sidebar bottom status */}
        <div className="p-4 border-t border-slate-100 text-xs text-slate-500 flex flex-col space-y-2 bg-slate-50/60">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-1.5 text-emerald-600 font-medium text-xs">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
              <span>{t.statusOperationalBadge}</span>
            </div>
            <button
              type="button"
              onClick={() => setViewMode('status')}
              className="text-[11px] text-indigo-600 hover:text-indigo-800 hover:underline font-medium cursor-pointer"
              title="查看公开对外服务健康状态页面"
            >
              对外状态页 ↗
            </button>
          </div>
        </div>
      </aside>

      {/* Main Container */}
      <main className="flex-1 flex flex-col min-w-0 overflow-y-auto">
        {/* Top Header */}
        <header className="h-16 bg-white/90 backdrop-blur-md border-b border-slate-200/80 flex items-center justify-between px-8 sticky top-0 z-20 shadow-xs">
          <div className="flex items-center space-x-3">
            <h2 className="text-base font-bold text-slate-900 tracking-tight">
              {currentTab === 'dashboard' && t.navDashboard}
              {currentTab === 'models' && t.navModels}
              {currentTab === 'pricing' && t.navPricing}
              {currentTab === 'channels' && t.navChannels}
              {currentTab === 'keys' && t.navKeys}
              {currentTab === 'wallet' && (adminUser?.role === 'admin' ? '卡密与充值管理' : (t.navWallet || '我的钱包'))}
              {currentTab === 'logs' && t.navLogs}
              {currentTab === 'mcp' && (t.navMcp || '扩展广场')}
              {currentTab === 'users' && (t.navUsers || '用户管理')}
              {currentTab === 'playground' && t.navPlayground}
              {currentTab === 'docs' && t.navDocs}
            </h2>
          </div>

          <div className="flex items-center space-x-2.5">
            {/* Language Switcher Pill */}
            <button
              onClick={toggleLang}
              className="text-xs px-2.5 py-1.5 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold flex items-center space-x-1.5 transition border border-slate-200 shadow-2xs cursor-pointer"
              title={lang === 'zh' ? 'Switch to English' : '切换为简体中文'}
            >
              <Globe className="w-3.5 h-3.5 text-indigo-600" />
              <span>{t.langToggle}</span>
            </button>

            <a
              href="https://github.com/ifnodoraemon/nano-gateway"
              target="_blank"
              rel="noreferrer"
              className="text-xs px-3 py-1.5 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 border border-slate-200 font-medium flex items-center space-x-1.5 transition"
            >
              <ExternalLink className="w-3.5 h-3.5" />
              <span>GitHub</span>
            </a>

            {/* Account info & actions */}
            <div className="flex items-center space-x-2.5 pl-2 border-l border-slate-200">
              {adminUser?.role === 'admin' ? (
                <div className="flex items-center space-x-2">
                  <button
                    onClick={() => setShowAccountModal(true)}
                    className="flex items-center space-x-1.5 text-xs text-slate-700 font-medium px-2.5 py-1 rounded-xl bg-slate-100 hover:bg-slate-200 border border-slate-200 transition cursor-pointer"
                    title="账号设置与个人中心"
                  >
                    <User className="w-3.5 h-3.5 text-indigo-600" />
                    <span>{adminUser?.username || 'admin'}</span>
                    <span className="text-[10px] text-purple-700 bg-purple-50 border border-purple-200 px-1.5 py-0.5 rounded font-mono font-bold">
                      管理员
                    </span>
                  </button>
                  <button
                    onClick={() => setCurrentTab('wallet')}
                    className="px-2.5 py-1 text-xs font-semibold text-slate-600 hover:text-indigo-600 bg-slate-50 hover:bg-slate-100 border border-slate-200 rounded-xl transition cursor-pointer"
                    title="卡密生成与充值中心"
                  >
                    卡密中心
                  </button>
                </div>
              ) : (
                <div className="flex items-center space-x-2">
                  <span className="text-[11px] font-semibold px-2 py-0.5 rounded-lg bg-indigo-50 text-indigo-700 border border-indigo-200">
                    {adminUser?.group_name ? `${adminUser.group_name} 组` : '默认组'}
                  </span>
                  <button
                    onClick={() => setCurrentTab('wallet')}
                    className="flex items-center space-x-1.5 text-xs text-emerald-800 font-bold px-2.5 py-1 rounded-xl bg-emerald-50 hover:bg-emerald-100 border border-emerald-200 transition cursor-pointer"
                    title="点击管理钱包与充值"
                  >
                    <Coins className="w-3.5 h-3.5 text-emerald-600" />
                    <span>¥{Number(adminUser?.balance || 0).toFixed(2)}</span>
                  </button>
                  <button
                    onClick={() => setShowAccountModal(true)}
                    className="flex items-center space-x-1.5 text-xs text-slate-700 hover:text-slate-900 px-2.5 py-1 bg-slate-100 hover:bg-slate-200 border border-slate-200 rounded-xl font-medium transition cursor-pointer"
                    title="账号设置与个人中心"
                  >
                    <User className="w-3.5 h-3.5 text-slate-500" />
                    <span>{adminUser?.username}</span>
                  </button>
                </div>
              )}
              <button
                onClick={handleLogout}
                className="p-1.5 rounded-xl bg-slate-100 hover:bg-rose-50 text-slate-600 hover:text-rose-600 transition cursor-pointer"
                title="退出登录"
              >
                <LogOut className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </header>

        <div className="p-8 max-w-7xl w-full mx-auto space-y-6">
          {/* Low Balance Warning Banner for Regular Users */}
          {adminUser && adminUser.role !== 'admin' && Number(adminUser.balance || 0) < 5 && (
            <div className="bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/60 rounded-3xl p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 shadow-xs animate-in fade-in">
              <div className="flex items-center space-x-3.5">
                <div className="w-10 h-10 rounded-2xl bg-amber-100 dark:bg-amber-900/60 text-amber-700 dark:text-amber-300 flex items-center justify-center shrink-0">
                  <AlertCircle className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-bold text-amber-900 dark:text-amber-200">
                    {Number(adminUser.balance || 0) <= 0 ? '钱包额度已耗尽 (余额不足)' : '钱包余额偏低预警'}
                  </h4>
                  <p className="text-xs text-amber-700 dark:text-amber-400 mt-0.5">
                    当前可用额度为 <strong className="font-mono font-bold">¥{Number(adminUser.balance || 0).toFixed(4)}</strong>。为避免生产 API 接口调用中断，请及时充值或兑换卡密。
                  </p>
                </div>
              </div>
              <div className="flex items-center space-x-2 shrink-0">
                <button
                  onClick={() => setCurrentTab('wallet')}
                  className="px-4 py-2 bg-amber-600 hover:bg-amber-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                >
                  前往充值 / 兑换卡密 →
                </button>
              </div>
            </div>
          )}

          {/* 1. ENTERPRISE OPERATIONS DASHBOARD */}
          {currentTab === 'dashboard' && (
            <div className="space-y-6">
              {/* 1.1 CLUSTER HIGH-AVAILABILITY TOPOLOGY */}
              <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-4">
                <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 pb-4 border-b border-slate-100 dark:border-slate-800/80">
                  <div className="flex items-center space-x-3.5">
                    <div className="w-10 h-10 rounded-2xl bg-indigo-50 dark:bg-indigo-950/60 border border-indigo-200 dark:border-indigo-800/60 flex items-center justify-center text-indigo-600 dark:text-indigo-400 shrink-0">
                      <Network className="w-5 h-5" />
                    </div>
                    <div className="flex items-center space-x-2">
                      <h3 className="font-bold text-base text-slate-900 dark:text-slate-100">
                        集群架构
                      </h3>
                      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60">
                        <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5"></span>
                        运行正常
                      </span>
                    </div>
                  </div>
                  <div className="flex items-center space-x-2.5">
                    <button
                      onClick={() => {
                        fetchData();
                        fetchLogs();
                        showToast('状态已刷新', 'info');
                      }}
                      className="px-3.5 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-xs font-semibold text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 transition flex items-center space-x-1.5"
                    >
                      <RefreshCw className="w-3.5 h-3.5" />
                      <span>刷新</span>
                    </button>
                    <button
                      onClick={() => setShowChannelModal(true)}
                      className="px-3.5 py-1.5 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-xs font-semibold text-white transition shadow-xs flex items-center space-x-1.5"
                    >
                      <Plus className="w-3.5 h-3.5" />
                      <span>接入服务商</span>
                    </button>
                  </div>
                </div>

                {/* Topology Nodes Grid */}
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4 pt-1">
                  {/* Node 1: Nginx LB */}
                  <div className="p-4 rounded-2xl bg-slate-50/70 dark:bg-slate-900/50 border border-slate-200/70 dark:border-slate-800/60 flex flex-col justify-between space-y-2">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-2">
                        <span className="p-1.5 rounded-xl bg-sky-100 dark:bg-sky-950/70 text-sky-600 dark:text-sky-400">
                          <Globe className="w-4 h-4" />
                        </span>
                        <span className="font-semibold text-xs text-slate-800 dark:text-slate-200">负载均衡 (LB)</span>
                      </div>
                      <span className="text-[11px] font-mono px-2 py-0.5 rounded-lg bg-emerald-100/70 dark:bg-emerald-950/70 text-emerald-700 dark:text-emerald-400 font-semibold">
                        :8080
                      </span>
                    </div>
                    <div className="text-[11px] text-slate-500 dark:text-slate-400 flex justify-between font-mono">
                      <span>Nginx 代理</span>
                      <span className="text-emerald-600 dark:text-emerald-400">Round-Robin</span>
                    </div>
                  </div>

                  {/* Node 2: Gateway Instance 1 */}
                  <div className="p-4 rounded-2xl bg-slate-50/70 dark:bg-slate-900/50 border border-slate-200/70 dark:border-slate-800/60 flex flex-col justify-between space-y-2">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-2">
                        <span className="p-1.5 rounded-xl bg-indigo-100 dark:bg-indigo-950/70 text-indigo-600 dark:text-indigo-400">
                          <Server className="w-4 h-4" />
                        </span>
                        <span className="font-semibold text-xs text-slate-800 dark:text-slate-200">计算节点 #1</span>
                      </div>
                      <span className="text-[11px] font-mono px-2 py-0.5 rounded-lg bg-indigo-100/70 dark:bg-indigo-950/70 text-indigo-700 dark:text-indigo-400 font-semibold">
                        :8081
                      </span>
                    </div>
                    <div className="text-[11px] text-slate-500 dark:text-slate-400 flex justify-between font-mono">
                      <span>gateway-1</span>
                      <span className="text-emerald-600 dark:text-emerald-400">Active</span>
                    </div>
                  </div>

                  {/* Node 3: Gateway Instance 2 */}
                  <div className="p-4 rounded-2xl bg-slate-50/70 dark:bg-slate-900/50 border border-slate-200/70 dark:border-slate-800/60 flex flex-col justify-between space-y-2">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-2">
                        <span className="p-1.5 rounded-xl bg-purple-100 dark:bg-purple-950/70 text-purple-600 dark:text-purple-400">
                          <Server className="w-4 h-4" />
                        </span>
                        <span className="font-semibold text-xs text-slate-800 dark:text-slate-200">计算节点 #2</span>
                      </div>
                      <span className="text-[11px] font-mono px-2 py-0.5 rounded-lg bg-purple-100/70 dark:bg-purple-950/70 text-purple-700 dark:text-purple-400 font-semibold">
                        :8082
                      </span>
                    </div>
                    <div className="text-[11px] text-slate-500 dark:text-slate-400 flex justify-between font-mono">
                      <span>gateway-2</span>
                      <span className="text-emerald-600 dark:text-emerald-400">Active</span>
                    </div>
                  </div>
                </div>
              </div>

              {/* 1.2 CORE SLA & TELEMETRY KPIS */}
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4">
                <div className="bg-white border border-slate-200/80 rounded-3xl p-5 shadow-xs hover:border-indigo-400 transition group">
                  <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">累计请求量</span>
                  <div className="mt-3 flex items-baseline justify-between">
                    <span className="text-2xl font-extrabold text-slate-900 font-mono">{stats.total_requests || 0}</span>
                    <div className="p-2 bg-indigo-50 rounded-xl text-indigo-600 group-hover:scale-110 transition">
                      <Zap className="w-4 h-4" />
                    </div>
                  </div>
                </div>

                <div className="bg-white border border-slate-200/80 rounded-3xl p-5 shadow-xs hover:border-emerald-400 transition group">
                  <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">活跃服务商</span>
                  <div className="mt-3 flex items-baseline justify-between">
                    <span className="text-2xl font-extrabold text-emerald-600 font-mono">{channels.length}</span>
                    <div className="p-2 bg-emerald-50 rounded-xl text-emerald-600 group-hover:scale-110 transition">
                      <Layers className="w-4 h-4" />
                    </div>
                  </div>
                </div>

                <div className="bg-white border border-slate-200/80 rounded-3xl p-5 shadow-xs hover:border-sky-400 transition group">
                  <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">累计 Token</span>
                  <div className="mt-3 flex items-baseline justify-between">
                    <span className="text-2xl font-extrabold text-sky-600 font-mono">{stats.total_tokens || 0}</span>
                    <div className="p-2 bg-sky-50 rounded-xl text-sky-600 group-hover:scale-110 transition">
                      <Activity className="w-4 h-4" />
                    </div>
                  </div>
                </div>

                <div className="bg-white border border-slate-200/80 rounded-3xl p-5 shadow-xs hover:border-amber-400 transition group">
                  <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">首字时延 (TTFT)</span>
                  <div className="mt-3 flex items-baseline justify-between">
                    <span className="text-2xl font-extrabold text-amber-600 font-mono">
                      {(stats.avg_ttft_ms || 0).toFixed(0)} <span className="text-xs text-slate-400 font-normal">ms</span>
                    </span>
                    <div className="p-2 bg-amber-50 rounded-xl text-amber-600 group-hover:scale-110 transition">
                      <Clock className="w-4 h-4" />
                    </div>
                  </div>
                </div>

                <div className="bg-white border border-slate-200/80 rounded-3xl p-5 shadow-xs hover:border-indigo-400 transition group">
                  <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">累计消费扣减</span>
                  <div className="mt-3 flex items-baseline justify-between">
                    <span className="text-2xl font-extrabold text-slate-900 font-mono">
                      ¥{(stats.total_cost || 0).toFixed(4)}
                    </span>
                    <div className="p-2 bg-indigo-50 rounded-xl text-indigo-600 group-hover:scale-110 transition">
                      <DollarSign className="w-4 h-4" />
                    </div>
                  </div>
                </div>

                <div className="bg-white border border-slate-200/80 rounded-3xl p-5 shadow-xs hover:border-emerald-400 transition group">
                  <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">缓存节省资金</span>
                  <div className="mt-3 flex items-baseline justify-between">
                    <span className="text-2xl font-extrabold text-emerald-600 font-mono">
                      ¥{(stats.saved_cost || 0).toFixed(4)}
                    </span>
                    <div className="p-2 bg-emerald-50 rounded-xl text-emerald-600 group-hover:scale-110 transition">
                      <Sparkles className="w-4 h-4" />
                    </div>
                  </div>
                </div>
              </div>

              {/* 1.25 MIDDLEWARE INFRASTRUCTURE HEALTH STATUS MATRIX */}
              <MiddlewareStatusMatrix adminFetch={adminFetch} showToast={showToast} />

              {/* 1.3 UPSTREAM PROVIDERS HEALTH & CIRCUIT BREAKER MATRIX */}
              <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
                <div className="p-5 border-b border-slate-100 dark:border-slate-800/80 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
                  <div>
                    <h3 className="font-bold text-sm text-slate-900 dark:text-slate-100 flex items-center space-x-2">
                      <Shield className="w-4 h-4 text-emerald-500" />
                      <span>上游服务商状态</span>
                    </h3>
                  </div>
                  <div className="flex items-center space-x-2">
                    <button
                      onClick={handleBatchTest}
                      disabled={batchTesting || channels.length === 0}
                      className="px-3.5 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-xs font-semibold text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 transition flex items-center space-x-1.5"
                    >
                      <Activity className={`w-3.5 h-3.5 ${batchTesting ? 'animate-spin text-indigo-500' : 'text-emerald-500'}`} />
                      <span>全渠道体检</span>
                    </button>
                    <button
                      onClick={() => setCurrentTab('channels')}
                      className="px-3.5 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/60 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800 text-xs font-semibold transition"
                    >
                      渠道配置 ({channels.length}) →
                    </button>
                  </div>
                </div>

                {channels.length === 0 ? (
                  <div className="p-10 text-center flex flex-col items-center justify-center space-y-3">
                    <div className="w-10 h-10 rounded-2xl bg-indigo-50 dark:bg-indigo-950/50 border border-indigo-200 dark:border-indigo-800/80 flex items-center justify-center text-indigo-500">
                      <Server className="w-5 h-5" />
                    </div>
                    <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">暂无服务商</h4>
                    <button
                      onClick={() => setShowChannelModal(true)}
                      className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-xs transition flex items-center space-x-1.5"
                    >
                      <Plus className="w-4 h-4" />
                      <span>接入服务商</span>
                    </button>
                  </div>
                ) : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-left border-collapse">
                      <thead>
                        <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/60 dark:bg-slate-900/60">
                          <th className="py-3 px-6 font-semibold">服务商名称</th>
                          <th className="py-3 px-6 font-semibold">协议类型</th>
                          <th className="py-3 px-6 font-semibold">接入 Base URL</th>
                          <th className="py-3 px-6 font-semibold">支持模型数</th>
                          <th className="py-3 px-6 font-semibold">熔断器状态</th>
                          <th className="py-3 px-6 font-semibold">最近延迟</th>
                          <th className="py-3 px-6 text-right font-semibold">体检操作</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-xs">
                        {channels.map((ch) => {
                          const latInfo = channelLatencies[ch.id];
                          const breaker = ch.breaker_status || 'CLOSED';
                          return (
                            <tr key={ch.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition">
                              <td className="py-3.5 px-6 font-semibold text-slate-800 dark:text-slate-200">
                                {ch.name}
                              </td>
                              <td className="py-3.5 px-6">
                                <span className="px-2.5 py-0.5 rounded-lg text-[11px] font-mono font-medium bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700">
                                  {ch.type}
                                </span>
                              </td>
                              <td className="py-3.5 px-6 font-mono text-[11px] text-slate-500 dark:text-slate-400 max-w-xs truncate">
                                {ch.base_url}
                              </td>
                              <td className="py-3.5 px-6 font-medium text-slate-700 dark:text-slate-300">
                                {ch.models?.length || 0} 个模型
                              </td>
                              <td className="py-3.5 px-6">
                                <span
                                  className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold border ${
                                    breaker === 'CLOSED'
                                      ? 'bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800/60'
                                      : breaker === 'HALF-OPEN'
                                      ? 'bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border-amber-200 dark:border-amber-800/60'
                                      : 'bg-rose-50 dark:bg-rose-950/60 text-rose-700 dark:text-rose-300 border-rose-200 dark:border-rose-800/60'
                                  }`}
                                >
                                  <span
                                    className={`w-1.5 h-1.5 rounded-full mr-1.5 ${
                                      breaker === 'CLOSED'
                                        ? 'bg-emerald-500'
                                        : breaker === 'HALF-OPEN'
                                        ? 'bg-amber-500 animate-pulse'
                                        : 'bg-rose-500'
                                    }`}
                                  ></span>
                                  {breaker === 'CLOSED' ? '正常' : breaker === 'HALF-OPEN' ? '半开恢复' : '熔断隔离'}
                                </span>
                              </td>
                              <td className="py-3.5 px-6 font-mono text-xs">
                                {latInfo ? (
                                  latInfo.success ? (
                                    <span className="text-emerald-600 dark:text-emerald-400 font-semibold">{latInfo.latencyMs} ms</span>
                                  ) : (
                                    <span className="text-rose-500">异常</span>
                                  )
                                ) : (
                                  <span className="text-slate-400">-</span>
                                )}
                              </td>
                              <td className="py-3.5 px-6 text-right">
                                <button
                                  onClick={() => handleTestChannel(ch)}
                                  disabled={testingId === ch.id}
                                  className="text-xs px-2.5 py-1 rounded-lg bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 border border-indigo-200 dark:border-indigo-800 font-medium transition inline-flex items-center space-x-1"
                                >
                                  {testingId === ch.id ? <RefreshCw className="w-3 h-3 animate-spin" /> : <Play className="w-3 h-3" />}
                                  <span>Ping</span>
                                </button>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>

              {/* 1.4 RECENT TRAFFIC AUDIT STREAM */}
              <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
                <div className="p-5 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
                  <div>
                    <h3 className="font-bold text-sm text-slate-900 dark:text-slate-100 flex items-center space-x-2">
                      <History className="w-4 h-4 text-sky-500" />
                      <span>最近请求</span>
                    </h3>
                  </div>
                  <button
                    onClick={() => setCurrentTab('logs')}
                    className="text-xs font-semibold text-indigo-600 dark:text-indigo-400 hover:underline"
                  >
                    全部审计日志 ({logs.length}) →
                  </button>
                </div>

                {logs.length === 0 ? (
                  <div className="p-8 text-center text-xs text-slate-400 dark:text-slate-500">
                    暂无请求记录
                  </div>
                ) : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-left border-collapse">
                      <thead>
                        <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/60 dark:bg-slate-900/60">
                          <th className="py-3 px-6 font-semibold">请求时间</th>
                          <th className="py-3 px-6 font-semibold">调用方 / 团队</th>
                          <th className="py-3 px-6 font-semibold">请求模型</th>
                          <th className="py-3 px-6 font-semibold">路由命中的上游</th>
                          <th className="py-3 px-6 font-semibold">状态码</th>
                          <th className="py-3 px-6 font-semibold">首字 (TTFT) / 总耗时</th>
                          <th className="py-3 px-6 font-semibold">Token 消耗</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-xs">
                        {logs.slice(0, 5).map((log) => (
                          <tr
                            key={log.id}
                            onClick={() => setSelectedLog(log)}
                            className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 cursor-pointer transition"
                          >
                            <td className="py-3 px-6 font-mono text-slate-500">
                              {new Date(log.created_at).toLocaleTimeString()}
                            </td>
                            <td className="py-3 px-6 font-medium text-slate-800 dark:text-slate-200">
                              {log.tenant_id || 'anonymous'}
                            </td>
                            <td className="py-3 px-6 font-mono font-semibold text-indigo-600 dark:text-indigo-400">
                              {log.model}
                            </td>
                            <td className="py-3 px-6 text-slate-600 dark:text-slate-300 font-medium">
                              {log.channel_name || '默认通道'}
                            </td>
                            <td className="py-3 px-6">
                              <span
                                className={`px-2 py-0.5 rounded-full font-mono font-bold text-[11px] ${
                                  log.status_code >= 200 && log.status_code < 300
                                    ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300'
                                    : 'bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300'
                                }`}
                              >
                                {log.status_code}
                              </span>
                            </td>
                            <td className="py-3 px-6 font-mono text-slate-700 dark:text-slate-300">
                              {log.ttft_ms} ms / {log.duration_ms} ms
                            </td>
                            <td className="py-3 px-6 font-mono text-slate-600 dark:text-slate-300">
                              {log.total_tokens}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* 1.5. MODEL ROUTES TAB */}
          {currentTab === 'models' && (
            <ModelRoutesManager
              adminFetch={adminFetch}
              showToast={showToast}
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
            />
          )}

          {/* 2. CHANNELS / PROVIDERS TAB */}
          {currentTab === 'channels' && (
            <div className="space-y-6">
              <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center bg-white dark:bg-[#111726] p-5 rounded-3xl border border-slate-200/80 dark:border-slate-800/80 shadow-xs gap-3">
                <div className="flex items-center space-x-2">
                  <Server className="w-4 h-4 text-emerald-500" />
                  <h3 className="font-semibold text-slate-900 dark:text-slate-100 text-sm">
                    模型服务商
                  </h3>
                </div>
                <div className="flex items-center space-x-2.5">
                  <button
                    onClick={handleBatchPing}
                    disabled={batchTesting || channels.length === 0}
                    className="px-3.5 py-2 bg-emerald-50 dark:bg-emerald-950/40 hover:bg-emerald-100 dark:hover:bg-emerald-900/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60 rounded-xl text-xs font-semibold shadow-xs flex items-center space-x-1.5 transition disabled:opacity-50"
                  >
                    <Activity className={`w-3.5 h-3.5 ${batchTesting ? 'animate-spin' : ''}`} />
                    <span>{batchTesting ? '体检中...' : '体检全部'}</span>
                  </button>
                  <button
                    onClick={() => {
                      setEditingChannelId(null);
                      setNewChannel({
                        name: '',
                        type: 'gpustack',
                        base_url: 'http://10.232.16.83/v1-openai',
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
                    className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition"
                  >
                    <Plus className="w-4 h-4" />
                    <span>接入服务商</span>
                  </button>
                </div>
              </div>

              {channels.length === 0 ? (
                <div className="bg-white dark:bg-[#111726] border border-dashed border-slate-300 dark:border-slate-800 rounded-3xl p-10 text-center flex flex-col items-center justify-center space-y-3">
                  <div className="w-10 h-10 rounded-2xl bg-indigo-50 dark:bg-indigo-950/60 border border-indigo-200 dark:border-indigo-800/80 flex items-center justify-center text-indigo-600 dark:text-indigo-400">
                    <Server className="w-5 h-5" />
                  </div>
                  <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">暂无服务商</h4>
                  <button
                    onClick={() => {
                      setEditingChannelId(null);
                      setNewChannel({
                        name: '',
                        type: 'gpustack',
                        base_url: 'http://10.232.16.83/v1-openai',
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
                    className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition"
                  >
                    <Plus className="w-4 h-4" />
                    <span>接入服务商</span>
                  </button>
                </div>
              ) : (
                <div className="space-y-3">
                  {selectedChannelIds.length > 0 && (
                    <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800/60 px-5 py-3 rounded-2xl animate-in fade-in gap-3">
                      <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
                        <span>已选中 {selectedChannelIds.length} 项</span>
                      </div>
                      <div className="flex items-center space-x-2">
                        <button
                          onClick={() => handleBatchStatusChannels('active')}
                          className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                        >
                          批量启用
                        </button>
                        <button
                          onClick={() => handleBatchStatusChannels('disabled')}
                          className="px-3 py-1.5 bg-amber-600 hover:bg-amber-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                        >
                          批量停用
                        </button>
                        <button
                          onClick={handleBatchDeleteChannels}
                          className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-semibold rounded-xl shadow-xs transition flex items-center space-x-1 cursor-pointer"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                          <span>批量删除</span>
                        </button>
                        <button
                          onClick={() => setSelectedChannelIds([])}
                          className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-xl hover:bg-slate-50 transition cursor-pointer"
                        >
                          取消选择
                        </button>
                      </div>
                    </div>
                  )}

                  <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
                    <table className="w-full text-left border-collapse">
                      <thead>
                        <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/80 dark:bg-slate-900/80">
                          <th className="py-3.5 px-4 w-10 text-center">
                            <input
                              type="checkbox"
                              className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                              checked={channels.length > 0 && selectedChannelIds.length === channels.length}
                              onChange={(e) => {
                                if (e.target.checked) {
                                  setSelectedChannelIds(channels.map(c => c.id));
                                } else {
                                  setSelectedChannelIds([]);
                                }
                              }}
                            />
                          </th>
                          <th className="py-3.5 px-6 font-semibold">服务商名称 & 健康状态</th>
                          <th className="py-3.5 px-6 font-semibold">服务引擎</th>
                          <th className="py-3.5 px-6 font-semibold">下游 Base URL</th>
                          <th className="py-3.5 px-6 font-semibold">开放功能模态</th>
                          <th className="py-3.5 px-6 font-semibold">挂载模型与别名</th>
                          <th className="py-3.5 px-6 font-semibold">路由优先级 / 权重</th>
                          <th className="py-3.5 px-6 text-right font-semibold">操作</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-sm">
                        {channels.map((ch) => (
                          <tr key={ch.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition">
                            <td className="py-4 px-4 text-center">
                              <input
                                type="checkbox"
                                className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                                checked={selectedChannelIds.includes(ch.id)}
                                onChange={(e) => {
                                  e.stopPropagation();
                                  if (e.target.checked) {
                                    setSelectedChannelIds(prev => [...prev, ch.id]);
                                  } else {
                                    setSelectedChannelIds(prev => prev.filter(x => x !== ch.id));
                                  }
                                }}
                              />
                            </td>
                            <td className="py-4 px-6 font-medium text-slate-900 dark:text-slate-100">
                              <div className="flex items-center space-x-2">
                              <span
                                className={`w-2.5 h-2.5 rounded-full ${
                                  ch.breaker_status === 'OPEN'
                                    ? 'bg-rose-500 animate-ping'
                                    : ch.status === 'active'
                                    ? 'bg-emerald-500'
                                    : 'bg-slate-400'
                                }`}
                              ></span>
                              <span className="font-semibold">{ch.name}</span>
                            </div>
                            <div className="flex items-center space-x-1.5 mt-1">
                              {ch.breaker_status && (
                                <span className={`inline-block text-[10px] px-2 py-0.5 rounded font-mono font-medium ${
                                  ch.breaker_status === 'OPEN'
                                    ? 'bg-rose-100 dark:bg-rose-950/60 text-rose-700 dark:text-rose-300'
                                    : ch.breaker_status === 'HALF-OPEN'
                                    ? 'bg-amber-100 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300'
                                    : 'bg-emerald-100 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300'
                                }`}>
                                  Breaker: {ch.breaker_status}
                                </span>
                              )}
                              {channelLatencies[ch.id] && (
                                <span className={`text-[10px] px-2 py-0.5 rounded font-mono font-medium ${
                                  channelLatencies[ch.id].success
                                    ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                                    : 'bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800'
                                }`}>
                                  {channelLatencies[ch.id].success ? `🟢 ${channelLatencies[ch.id].latency_ms}ms` : '🔴 异常'}
                                </span>
                              )}
                            </div>
                          </td>
                          <td className="py-4 px-6">
                            <span
                              className={`px-2.5 py-1 rounded-lg text-xs font-mono font-medium border ${
                                ch.type === 'gemini'
                                  ? 'bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300 border-blue-200 dark:border-blue-800'
                                  : ch.type === 'anthropic'
                                  ? 'bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300 border-amber-200 dark:border-amber-800'
                                  : ch.type === 'gpustack'
                                  ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                                  : ch.type === 'sub2api'
                                  ? 'bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 border-purple-200 dark:border-purple-800'
                                  : 'bg-indigo-50 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 border-indigo-200 dark:border-indigo-800'
                              }`}
                            >
                              {ch.type}
                            </span>
                          </td>
                          <td className="py-4 px-6 text-slate-600 dark:text-slate-400 font-mono text-xs max-w-xs">
                            <div className="flex items-center space-x-1.5">
                              <span className="truncate">{ch.base_url}</span>
                              <button
                                onClick={() => copyToClipboard(ch.base_url)}
                                className="text-slate-400 hover:text-indigo-500 transition p-1"
                                title="复制 Base URL"
                              >
                                <Copy className="w-3 h-3" />
                              </button>
                            </div>
                          </td>
                          <td className="py-4 px-6">
                            <div className="flex flex-wrap gap-1">
                              {(!ch.protocols || ch.protocols.length === 0) ? (
                                <span className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-[11px] text-slate-600 dark:text-slate-300 font-medium">全功能直通</span>
                              ) : (
                                ch.protocols.map(p => {
                                  let label = p;
                                  let colorClass = 'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700';
                                  if (p === 'openai_chat' || p === 'chat') { label = '💬 对话'; colorClass = 'bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800'; }
                                  else if (p === 'openai_response' || p === 'responses' || p === 'response') { label = '⚡ Responses'; colorClass = 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'; }
                                  else if (p === 'openai_text' || p === 'completion') { label = '📝 补全'; colorClass = 'bg-sky-50 dark:bg-sky-950/50 text-sky-700 dark:text-sky-300 border border-sky-200 dark:border-sky-800'; }
                                  else if (p === 'anthropic_messages' || p === 'messages') { label = '🧠 Claude'; colorClass = 'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800'; }
                                  else if (p === 'images' || p === 'image_generation') { label = '🎨 生图'; colorClass = 'bg-pink-50 dark:bg-pink-950/50 text-pink-700 dark:text-pink-300 border border-pink-200 dark:border-pink-800'; }
                                  else if (p === 'audio_speech' || p === 'tts') { label = '🔊 TTS'; colorClass = 'bg-cyan-50 dark:bg-cyan-950/50 text-cyan-700 dark:text-cyan-300 border border-cyan-200 dark:border-cyan-800'; }
                                  else if (p === 'audio_transcription' || p === 'stt') { label = '🎙️ STT'; colorClass = 'bg-teal-50 dark:bg-teal-950/50 text-teal-700 dark:text-teal-300 border border-teal-200 dark:border-teal-800'; }
                                  else if (p === 'videos' || p === 'video_generation') { label = '🎬 视频'; colorClass = 'bg-purple-50 dark:bg-purple-950/50 text-purple-700 dark:text-purple-300 border border-purple-200 dark:border-purple-800'; }
                                  else if (p === 'embeddings' || p === 'embedding') { label = '🧠 向量'; colorClass = 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'; }
                                  else if (p === 'rerank' || p === 'reranker') { label = '🎯 重排'; colorClass = 'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800'; }
                                  return (
                                    <span key={p} className={`px-2 py-0.5 rounded text-[11px] font-medium ${colorClass}`}>
                                      {label}
                                    </span>
                                  );
                                })
                              )}
                            </div>
                          </td>
                          <td className="py-4 px-6">
                            <div className="flex flex-wrap gap-1 max-w-xs">
                              {(!ch.models || ch.models.length === 0) ? (
                                <span className="text-xs text-slate-400">未同步模型</span>
                              ) : (
                                ch.models.slice(0, 4).map((m) => (
                                  <span key={m} className="px-2 py-0.5 rounded bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 text-xs font-mono border border-indigo-100 dark:border-indigo-800">
                                    {m}
                                  </span>
                                ))
                              )}
                              {ch.models && ch.models.length > 4 && (
                                <span className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 text-xs font-mono" title={ch.models.slice(4).join(', ')}>
                                  +{ch.models.length - 4} 更多
                                </span>
                              )}
                              {ch.model_mapping && Object.keys(ch.model_mapping).length > 0 && (
                                Object.entries(ch.model_mapping).map(([k, v]) => (
                                  <span key={k} className="px-2 py-0.5 rounded bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 text-[11px] font-mono border border-emerald-200 dark:border-emerald-800" title={`别名映射: ${k} -> ${v}`}>
                                    🔗 {k} → {v}
                                  </span>
                                ))
                              )}
                            </div>
                          </td>
                          <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-mono text-xs">
                            <span className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-800 dark:text-slate-200">优先级: {ch.priority}</span>
                            <span className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-800 dark:text-slate-200 ml-1">权重: {ch.weight}</span>
                          </td>
                          <td className="py-4 px-6 text-right space-x-2">
                            <button
                              onClick={() => handleTestChannel(ch)}
                              disabled={testingId === ch.id}
                              className="text-xs px-3 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 border border-indigo-200 dark:border-indigo-800 font-medium transition inline-flex items-center space-x-1"
                            >
                              {testingId === ch.id ? <RefreshCw className="w-3 h-3 animate-spin" /> : <Play className="w-3 h-3" />}
                              <span>Ping</span>
                            </button>
                            <button
                              onClick={() => handleEditChannel(ch)}
                              className="text-xs px-3 py-1.5 rounded-xl bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 hover:bg-amber-100 dark:hover:bg-amber-900/60 border border-amber-200 dark:border-amber-800 font-medium transition inline-flex items-center space-x-1"
                            >
                              <Edit3 className="w-3 h-3" />
                              <span>编辑</span>
                            </button>
                            <button
                              onClick={() => handleDeleteChannel(ch.id)}
                              className="text-xs px-2.5 py-1.5 text-rose-600 dark:text-rose-400 hover:text-rose-800 dark:hover:text-rose-300 transition font-medium"
                            >
                              注销
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                    </table>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* 3. API KEYS TAB */}
          {currentTab === 'keys' && (
            <div className="space-y-6">
              <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center bg-white dark:bg-[#111726] p-5 rounded-3xl border border-slate-200/80 dark:border-slate-800/80 shadow-xs gap-3">
                <div className="flex items-center space-x-2">
                  <Key className="w-4 h-4 text-amber-500" />
                  <h3 className="font-semibold text-slate-900 dark:text-slate-100 text-sm">
                    API 访问密钥
                  </h3>
                </div>
                <div className="flex items-center space-x-2">
                  <button
                    onClick={handleOpenCreateKey}
                    className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
                  >
                    <Plus className="w-4 h-4" />
                    <span>新建密钥</span>
                  </button>
                </div>
              </div>

              {keys.length === 0 ? (
                <div className="bg-white dark:bg-[#111726] border border-dashed border-slate-300 dark:border-slate-800 rounded-3xl p-10 text-center flex flex-col items-center justify-center space-y-3">
                  <div className="w-12 h-12 rounded-2xl bg-amber-50 dark:bg-amber-950/60 border border-amber-200 dark:border-amber-800/80 flex items-center justify-center text-amber-600 dark:text-amber-400 shadow-sm">
                    <Key className="w-6 h-6" />
                  </div>
                  <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">暂无 API 密钥</h4>
                  <button
                    onClick={handleOpenCreateKey}
                    className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
                  >
                    <Plus className="w-4 h-4" />
                    <span>新建密钥</span>
                  </button>
                </div>
              ) : (
                <div className="space-y-3">
                  {selectedKeyIds.length > 0 && (
                    <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800/60 px-5 py-3 rounded-2xl animate-in fade-in gap-3">
                      <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
                        <span>已选中 {selectedKeyIds.length} 个密钥</span>
                      </div>
                      <div className="flex items-center space-x-2">
                        <button
                          onClick={() => handleBatchStatusKeys('active')}
                          className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                        >
                          批量启用
                        </button>
                        <button
                          onClick={() => handleBatchStatusKeys('disabled')}
                          className="px-3 py-1.5 bg-amber-600 hover:bg-amber-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                        >
                          批量停用
                        </button>
                        <button
                          onClick={handleBatchDeleteKeys}
                          className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-semibold rounded-xl shadow-xs transition flex items-center space-x-1 cursor-pointer"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                          <span>批量注销</span>
                        </button>
                        <button
                          onClick={() => setSelectedKeyIds([])}
                          className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-xl hover:bg-slate-50 transition cursor-pointer"
                        >
                          取消选择
                        </button>
                      </div>
                    </div>
                  )}

                  <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
                    <table className="w-full text-left border-collapse">
                      <thead>
                        <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/80 dark:bg-slate-900/80">
                          <th className="py-3.5 px-4 w-10 text-center">
                            <input
                              type="checkbox"
                              className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                              checked={keys.length > 0 && selectedKeyIds.length === keys.length}
                              onChange={(e) => {
                                if (e.target.checked) {
                                  setSelectedKeyIds(keys.map(k => k.id));
                                } else {
                                  setSelectedKeyIds([]);
                                }
                              }}
                            />
                          </th>
                          <th className="py-3.5 px-6 font-semibold">密钥名称</th>
                          <th className="py-3.5 px-6 font-semibold">密钥 (API Key)</th>
                          <th className="py-3.5 px-6 font-semibold">计费分组</th>
                          <th className="py-3.5 px-6 font-semibold">已用 / 额度</th>
                          <th className="py-3.5 px-6 font-semibold">授权模型</th>
                          <th className="py-3.5 px-6 font-semibold">速率 (RPM)</th>
                          <th className="py-3.5 px-6 font-semibold">状态</th>
                          <th className="py-3.5 px-6 text-right font-semibold">操作</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-sm">
                        {keys.map((k) => (
                          <tr key={k.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition">
                            <td className="py-4 px-4 text-center">
                              <input
                                type="checkbox"
                                className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                                checked={selectedKeyIds.includes(k.id)}
                                onChange={(e) => {
                                  e.stopPropagation();
                                  if (e.target.checked) {
                                    setSelectedKeyIds(prev => [...prev, k.id]);
                                  } else {
                                    setSelectedKeyIds(prev => prev.filter(x => x !== k.id));
                                  }
                                }}
                              />
                            </td>
                            <td className="py-4 px-6 text-slate-800 dark:text-slate-100 font-semibold">{k.tenant_id}</td>
                            <td className="py-4 px-6 font-mono text-xs text-indigo-700 dark:text-indigo-400 font-semibold flex items-center space-x-2">
                              <span>{k.key}</span>
                              <button
                                onClick={() => copyToClipboard(k.key)}
                                className="p-1 hover:bg-indigo-50 dark:hover:bg-indigo-950/60 rounded-lg text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-300 transition flex items-center space-x-1 cursor-pointer"
                                title="复制 Key"
                              >
                                {copiedKey === k.key ? (
                                  <Check className="w-3.5 h-3.5 text-emerald-500" />
                                ) : (
                                  <Copy className="w-3.5 h-3.5" />
                                )}
                              </button>
                            </td>
                            <td className="py-4 px-6 text-xs">
                              <span className={`inline-flex items-center px-2.5 py-0.5 rounded-lg text-[11px] font-semibold border ${
                                (k.group_name === 'vip')
                                  ? 'bg-amber-50 dark:bg-amber-950/40 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800/60'
                                  : (k.group_name === 'enterprise')
                                  ? 'bg-purple-50 dark:bg-purple-950/40 text-purple-800 dark:text-purple-300 border-purple-200 dark:border-purple-800/60'
                                  : 'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border-slate-200 dark:border-slate-700'
                              }`}>
                                {(k.group_name === 'vip' ? '⭐ VIP 组' : k.group_name === 'enterprise' ? '👑 企业组' : (k.group_name && k.group_name !== 'default' ? `${k.group_name} 组` : '默认组'))}
                              </span>
                            </td>
                            <td className="py-4 px-6 text-xs font-mono">
                              <span className="text-slate-900 dark:text-slate-100 font-bold">¥{Number(k.used_cost || 0).toFixed(4)}</span>
                              <span className="text-slate-400 mx-1">/</span>
                              <span className={k.budget > 0 ? "text-indigo-600 dark:text-indigo-400 font-medium" : "text-slate-400"}>
                                {k.budget > 0 ? `¥${Number(k.budget).toFixed(2)}` : '不限'}
                              </span>
                            </td>
                            <td className="py-4 px-6 text-xs text-slate-600 dark:text-slate-300 font-medium">
                              {!k.allowed_models || k.allowed_models.length === 0 ? (
                                <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60 font-semibold">全部允许</span>
                              ) : (
                                <span className="font-mono">{k.allowed_models.join(', ')}</span>
                              )}
                            </td>
                            <td className="py-4 px-6 text-slate-600 dark:text-slate-400 font-mono text-xs">{k.rpm ? `${k.rpm} 次/分` : '不限'}</td>
                          <td className="py-4 px-6">
                            {k.status === 'disabled' ? (
                              <span className="px-2.5 py-0.5 rounded-full text-xs bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800/60 font-medium inline-flex items-center space-x-1.5">
                                <span className="w-1.5 h-1.5 rounded-full bg-rose-500 animate-pulse"></span>
                                <span>已停用</span>
                              </span>
                            ) : (
                              <span className="px-2.5 py-0.5 rounded-full text-xs bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60 font-medium inline-flex items-center space-x-1.5">
                                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
                                <span>正常</span>
                              </span>
                            )}
                          </td>
                          <td className="py-4 px-6 text-right space-x-2">
                            <button
                              onClick={() => handleToggleKeyStatus(k)}
                              className={`text-xs px-2.5 py-1.5 rounded-xl border font-medium transition ${
                                k.status === 'disabled'
                                  ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-100 border-emerald-200 dark:border-emerald-800/60'
                                  : 'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 hover:bg-amber-100 border-amber-200 dark:border-amber-800/60'
                              }`}
                              title={k.status === 'disabled' ? '恢复启用' : '暂停使用'}
                            >
                              {k.status === 'disabled' ? '启用' : '停用'}
                            </button>
                            <button
                              onClick={() => setActiveQuickKey(k)}
                              className="text-xs px-3 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 border border-indigo-200 dark:border-indigo-800/60 font-medium transition inline-flex items-center space-x-1"
                            >
                              <Sparkles className="w-3.5 h-3.5 text-indigo-500" />
                              <span>快速接入</span>
                            </button>
                            <button
                              onClick={() => handleDeleteKey(k.id)}
                              className="text-xs px-2.5 py-1.5 text-rose-600 dark:text-rose-400 hover:text-rose-800 dark:hover:text-rose-300 transition font-medium"
                            >
                              删除
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                    </table>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* 4. AUDIT LOGS TAB */}
          {currentTab === 'logs' && (
            <div className="space-y-6">
              <div className="bg-white dark:bg-[#111726] p-6 rounded-3xl border border-slate-200/80 dark:border-slate-800/80 shadow-xs space-y-4">
                <div className="flex flex-col lg:flex-row justify-between items-start lg:items-center gap-4">
                  <div className="flex items-center space-x-3">
                    <div className="w-9 h-9 rounded-2xl bg-sky-50 dark:bg-sky-950/60 text-sky-600 dark:text-sky-400 flex items-center justify-center border border-sky-200/60 dark:border-sky-800/60">
                      <History className="w-4 h-4" />
                    </div>
                    <div>
                      <h3 className="font-bold text-slate-900 dark:text-slate-100 text-sm">
                        对话审计与请求日志
                      </h3>
                    </div>
                  </div>

                  {/* Actions & Search */}
                  <div className="flex flex-wrap items-center gap-3 w-full lg:w-auto">
                    {/* Active Chat/Session/Trace Filter Tag */}
                    {sessionFilter && (
                      <div className="flex items-center space-x-2 px-3 py-1.5 rounded-xl bg-purple-50 dark:bg-purple-950/60 border border-purple-200 dark:border-purple-800 text-xs text-purple-700 dark:text-purple-300 animate-in fade-in">
                        <span className="text-[11px] text-slate-400">已锁定链路/对话:</span>
                        <span className="font-mono font-bold max-w-[140px] truncate">{sessionFilter}</span>
                        <button
                          onClick={() => {
                            setSessionFilter('');
                            fetchLogs({ sessionFilter: '' });
                          }}
                          className="hover:text-rose-500 ml-1 font-bold transition cursor-pointer"
                          title="清除筛选"
                        >
                          ✕
                        </button>
                      </div>
                    )}

                    <div className="relative flex-1 sm:w-64">
                      <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
                      <input
                        type="text"
                        value={logFilter}
                        onChange={(e) => setLogFilter(e.target.value)}
                        placeholder="筛选 Trace ID / 对话 ID / 模型 / 渠道..."
                        className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl pl-8 pr-3 py-2 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                      />
                    </div>

                    <button
                      onClick={() => fetchLogs()}
                      disabled={logLoading}
                      className="px-3.5 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition border border-slate-200 dark:border-slate-700 cursor-pointer"
                    >
                      <RefreshCw className={`w-3.5 h-3.5 ${logLoading ? 'animate-spin' : ''}`} />
                      <span>刷新</span>
                    </button>

                    <button
                      onClick={handleClearLogs}
                      className="px-3.5 py-2 bg-rose-50 dark:bg-rose-950/40 hover:bg-rose-100 dark:hover:bg-rose-900/60 text-rose-600 dark:text-rose-300 border border-rose-200 dark:border-rose-900/60 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition cursor-pointer"
                      title="清空全部审计调用日志"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      <span>清空日志</span>
                    </button>
                  </div>
                </div>

                {/* Time Range Filter Bar */}
                <div className="flex flex-wrap items-center justify-between pt-3 border-t border-slate-100 dark:border-slate-800/80 gap-3 text-xs">
                  <div className="flex items-center space-x-2">
                    <Clock className="w-3.5 h-3.5 text-slate-400" />
                    <span className="text-slate-500 dark:text-slate-400 font-medium">时间段筛选:</span>
                    <div className="inline-flex bg-slate-100 dark:bg-slate-900 p-0.5 rounded-xl border border-slate-200/80 dark:border-slate-800">
                      {[
                        { id: 'all', label: '全部时间' },
                        { id: '1h', label: '最近 1 小时' },
                        { id: 'today', label: '今天' },
                        { id: '7d', label: '最近 7 天' },
                        { id: 'custom', label: '自定义时间' },
                      ].map((item) => (
                        <button
                          key={item.id}
                          onClick={() => {
                            setTimeRange(item.id);
                            if (item.id !== 'custom') {
                              fetchLogs({ timeRange: item.id });
                            }
                          }}
                          className={`px-3 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                            timeRange === item.id
                              ? 'bg-white dark:bg-slate-800 text-indigo-600 dark:text-indigo-400 shadow-2xs font-semibold'
                              : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                          }`}
                        >
                          {item.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  {/* Custom time picker inputs */}
                  {timeRange === 'custom' && (
                    <div className="flex items-center space-x-2 animate-in fade-in">
                      <input
                        type="datetime-local"
                        value={customStartTime}
                        onChange={(e) => setCustomStartTime(e.target.value)}
                        className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1 text-xs text-slate-700 dark:text-slate-200"
                        title="开始时间"
                      />
                      <span className="text-slate-400">至</span>
                      <input
                        type="datetime-local"
                        value={customEndTime}
                        onChange={(e) => setCustomEndTime(e.target.value)}
                        className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1 text-xs text-slate-700 dark:text-slate-200"
                        title="结束时间"
                      />
                      <button
                        onClick={() => fetchLogs({ timeRange: 'custom' })}
                        className="px-3 py-1 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl font-medium transition cursor-pointer"
                      >
                        查询
                      </button>
                    </div>
                  )}
                </div>
              </div>

              {/* Batch Action Bar */}
              {selectedLogIds.length > 0 && (
                <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800/60 px-5 py-3 rounded-2xl animate-in fade-in gap-3">
                  <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
                    <span>已选中 {selectedLogIds.length} 条调用日志</span>
                  </div>
                  <div className="flex items-center space-x-2">
                    <button
                      onClick={handleBatchDeleteLogs}
                      className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-semibold rounded-xl shadow-xs transition flex items-center space-x-1 cursor-pointer"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      <span>批量删除</span>
                    </button>
                    <button
                      onClick={() => setSelectedLogIds([])}
                      className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-xl hover:bg-slate-50 transition cursor-pointer"
                    >
                      取消选择
                    </button>
                  </div>
                </div>
              )}

              <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
                <table className="w-full text-left border-collapse">
                  <thead>
                    <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/80 dark:bg-slate-900/80">
                      <th className="py-3.5 px-4 w-10 text-center">
                        <input
                          type="checkbox"
                          className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                          checked={filteredLogs.length > 0 && selectedLogIds.length === filteredLogs.length}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedLogIds(filteredLogs.map(l => l.id));
                            } else {
                              setSelectedLogIds([]);
                            }
                          }}
                        />
                      </th>
                      <th className="py-3.5 px-6 font-semibold">请求时间</th>
                      <th className="py-3.5 px-6 font-semibold">Trace ID / 对话 ID</th>
                      <th className="py-3.5 px-6 font-semibold">请求模型 (Model)</th>
                      <th className="py-3.5 px-6 font-semibold">命中渠道 (Provider)</th>
                      <th className="py-3.5 px-6 font-semibold">租户 / API 密钥</th>
                      <th className="py-3.5 px-6 font-semibold">Token (输入/输出/总)</th>
                      <th className="py-3.5 px-6 font-semibold">扣费 / 缓存命中</th>
                      <th className="py-3.5 px-6 font-semibold">耗时 / TTFT</th>
                      <th className="py-3.5 px-6 text-right font-semibold">状态与详情</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-sm font-mono text-xs">
                    {filteredLogs.map((log) => (
                      <tr
                        key={log.id}
                        onClick={() => setActiveLogDetail(log)}
                        className="hover:bg-indigo-50/40 dark:hover:bg-indigo-950/30 transition cursor-pointer group"
                      >
                        <td className="py-4 px-4 text-center" onClick={(e) => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                            checked={selectedLogIds.includes(log.id)}
                            onChange={(e) => {
                              e.stopPropagation();
                              if (e.target.checked) {
                                setSelectedLogIds(prev => [...prev, log.id]);
                              } else {
                                setSelectedLogIds(prev => prev.filter(x => x !== log.id));
                              }
                            }}
                          />
                        </td>
                        <td className="py-4 px-6 text-slate-500 dark:text-slate-400 font-sans whitespace-nowrap">
                          {log.created_at ? new Date(log.created_at).toLocaleTimeString() : '刚刚'}
                        </td>
                        <td className="py-4 px-6 font-mono text-xs">
                          <div className="flex items-center space-x-1.5">
                            <span className="text-purple-600 dark:text-purple-400 truncate max-w-[130px] font-semibold" title={log.trace_id || log.chat_id || log.session_id}>
                              {log.trace_id ? log.trace_id : (log.chat_id ? log.chat_id : (log.session_id ? log.session_id : <span className="text-slate-400 font-sans">-</span>))}
                            </span>
                            {(log.trace_id || log.chat_id || log.session_id) && (
                              <>
                                <button
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    const tid = log.trace_id || log.chat_id || log.session_id;
                                    navigator.clipboard.writeText(tid);
                                    showToast('Trace ID / 对话 ID 已复制', 'success');
                                  }}
                                  className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded text-slate-400 hover:text-slate-600 transition"
                                  title="复制 Trace / 对话 ID"
                                >
                                  <Copy className="w-3 h-3" />
                                </button>
                                <button
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    const targetId = log.trace_id || log.chat_id || log.session_id;
                                    setSessionFilter(targetId);
                                    fetchLogs({ sessionFilter: targetId });
                                    showToast(`已筛选: ${targetId}`, 'info');
                                  }}
                                  className="p-1 hover:bg-purple-50 dark:hover:bg-purple-900/50 rounded text-purple-600 dark:text-purple-400 transition"
                                  title="按 Trace/Chat ID 快速过滤"
                                >
                                  <Search className="w-3 h-3" />
                                </button>
                              </>
                            )}
                          </div>
                          {log.chat_id && log.trace_id && log.chat_id !== log.trace_id && (
                            <span className="block text-[10px] text-slate-400 truncate max-w-[130px]" title={`对话 ID: ${log.chat_id}`}>
                              对话: {log.chat_id}
                            </span>
                          )}
                          {log.session_id && log.session_id !== log.chat_id && log.session_id !== log.trace_id && (
                            <span className="block text-[10px] text-slate-400 truncate max-w-[130px]" title={`关联会话: ${log.session_id}`}>
                              会话: {log.session_id}
                            </span>
                          )}
                        </td>
                        <td className="py-4 px-6 font-semibold text-slate-900 dark:text-slate-100 font-mono">
                          <span className="px-2 py-0.5 bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 rounded-md border border-indigo-100 dark:border-indigo-800/60">
                            {log.model || '-'}
                          </span>
                        </td>
                        <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-sans">
                          {log.channel ? (
                            <span className="px-2 py-0.5 bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 rounded-md border border-emerald-100 dark:border-emerald-800/60">
                              {log.channel}
                            </span>
                          ) : (
                            <span className="text-slate-400">直通/多源</span>
                          )}
                        </td>
                        <td className="py-4 px-6 text-slate-600 dark:text-slate-300 font-sans">
                          <span className="font-semibold text-slate-800 dark:text-slate-200">{log.tenant_id || 'anonymous'}</span>
                          {log.virtual_key && (
                            <span className="block text-[10px] text-slate-400 font-mono mt-0.5 truncate max-w-[120px]">
                              {log.virtual_key}
                            </span>
                          )}
                        </td>
                        <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-sans">
                          {log.total_tokens > 0 ? (
                            <span>
                              {log.prompt_tokens} + {log.completion_tokens} = <strong className="text-indigo-600 dark:text-indigo-400">{log.total_tokens}</strong>
                            </span>
                          ) : (
                            <span className="text-slate-400">-</span>
                          )}
                        </td>
                        <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-sans">
                          <div className="flex items-center space-x-1.5">
                            <span className="font-bold text-slate-900 dark:text-slate-100 font-mono">
                              ¥{(log.cost || 0).toFixed(4)}
                            </span>
                            {log.is_off_peak && (
                              <span className="px-1.5 py-0.2 rounded text-[10px] font-bold bg-indigo-50 text-indigo-700 border border-indigo-200" title={`分时计费优惠 ${(log.off_peak_discount || 0.5) * 100}%`}>
                                🌙 闲时 {Math.round((log.off_peak_discount || 0.5) * 10)}折
                              </span>
                            )}
                          </div>
                          {log.cached_tokens > 0 ? (
                            <span className="block text-[10px] text-emerald-600 dark:text-emerald-400 font-semibold mt-0.5">
                              ⚡ 缓存: {log.cached_tokens} (省 90%)
                            </span>
                          ) : (
                            <span className="block text-[10px] text-slate-400 mt-0.5">无缓存命中</span>
                          )}
                        </td>
                        <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-sans">
                          <span className="font-bold text-slate-900 dark:text-slate-100">{log.duration_ms} ms</span>
                          {log.ttft_ms > 0 && (
                            <span className="block text-[11px] text-amber-600 dark:text-amber-400">TTFT: {log.ttft_ms} ms</span>
                          )}
                        </td>
                        <td className="py-4 px-6 text-right font-sans space-x-2">
                          <span
                            className={`px-2 py-0.5 rounded-full text-xs font-semibold ${
                              log.status_code === 200
                                ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                                : log.status_code === 429
                                ? 'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800'
                                : 'bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800'
                            }`}
                          >
                            {log.status_code || 200}
                          </span>
                          <span className="text-[11px] text-indigo-600 dark:text-indigo-400 group-hover:underline">
                            详情 →
                          </span>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleDeleteSingleLog(log.id);
                            }}
                            className="p-1 text-slate-400 hover:text-rose-600 transition rounded-lg hover:bg-rose-50 dark:hover:bg-rose-950/50 inline-flex items-center align-middle"
                            title="删除此记录"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </td>
                      </tr>
                    ))}
                    {filteredLogs.length === 0 && (
                      <tr>
                        <td colSpan="10" className="py-12 text-center text-slate-400 font-sans">
                          暂无匹配的审计调用记录
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* 5. MCP AGENT TAB */}
          {currentTab === 'mcp' && (
            <McpIntegrationView
              adminFetch={adminFetch}
              onCopy={(txt) => {
                navigator.clipboard.writeText(txt);
                showToast('已复制到剪贴板！', 'success');
              }}
              showToast={showToast}
            />
          )}

          {/* 6. USER MANAGEMENT TAB */}
          {currentTab === 'users' && (
            <UserManagementView
              adminUser={adminUser}
              adminToken={adminToken}
              adminFetch={adminFetch}
              showToast={showToast}
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
                  localStorage.setItem('nano_gateway_user', JSON.stringify(merged));
                  return merged;
                });
                fetchUserProfile();
              }}
              onBalanceUpdate={(newBal) => {
                setAdminUser(prev => {
                  const merged = prev ? { ...prev, balance: newBal } : prev;
                  localStorage.setItem('nano_gateway_user', JSON.stringify(merged));
                  return merged;
                });
                fetchUserProfile();
              }}
            />
          )}

          {/* 5. MULTIMODAL PLAYGROUND TAB */}
          {currentTab === 'playground' && (
            <div className="space-y-4">
              {/* Modality Selector Bar */}
              <div className="bg-white border border-slate-200/80 rounded-2xl p-1.5 shadow-xs flex flex-wrap items-center gap-1.5">
                {[
                  { id: 'chat', label: 'Chat 对话', icon: MessageSquare, activeColor: 'bg-indigo-600 text-white' },
                  { id: 'images', label: 'Images 绘图', icon: ImageIcon, activeColor: 'bg-pink-600 text-white' },
                  { id: 'audio_speech', label: 'TTS 语音合成', icon: Volume2, activeColor: 'bg-cyan-600 text-white' },
                  { id: 'audio_transcription', label: 'STT 语音转写', icon: Mic, activeColor: 'bg-teal-600 text-white' },
                  { id: 'videos', label: 'Video 视频生成', icon: Video, activeColor: 'bg-purple-600 text-white' },
                  { id: 'embeddings', label: 'Embedding 向量', icon: Cpu, activeColor: 'bg-emerald-600 text-white' },
                  { id: 'rerank', label: 'Rerank 重排', icon: Sliders, activeColor: 'bg-amber-600 text-white' },
                ].map(tab => {
                  const Icon = tab.icon;
                  return (
                    <button
                      key={tab.id}
                      onClick={() => setPlayModality(tab.id)}
                      className={`flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs font-semibold transition ${
                        playModality === tab.id
                          ? tab.activeColor
                          : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
                      }`}
                    >
                      <Icon className="w-3.5 h-3.5" />
                      <span>{tab.label}</span>
                    </button>
                  );
                })}
              </div>

              {/* Modality Layout: Controls + Output */}
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                {/* Left: Modality Controls */}
                <div className="bg-white border border-slate-200/80 rounded-2xl p-6 space-y-4 shadow-xs">
                  <div className="flex items-center justify-between border-b border-slate-100 pb-3">
                    <h3 className="font-bold text-slate-900 text-sm flex items-center space-x-2">
                      <Sparkles className="w-4 h-4 text-indigo-600" />
                      <span>参数设置</span>
                    </h3>
                  </div>

                  {/* Common: API Key input */}
                  <div>
                    <div className="flex items-center justify-between mb-1">
                      <label className="text-xs font-semibold text-slate-600 dark:text-slate-400">API 访问密钥 (可选)</label>
                      {keys.length > 0 && (
                        <button
                          type="button"
                          onClick={() => setPlayApiKey(keys[0].key)}
                          className="text-[11px] text-indigo-600 dark:text-indigo-400 hover:underline"
                        >
                          填入首个密钥 ({keys[0].tenant_id})
                        </button>
                      )}
                    </div>
                    <input
                      type="text"
                      value={playApiKey}
                      onChange={(e) => setPlayApiKey(e.target.value)}
                      placeholder={keys.length > 0 ? `留空默认使用: ${keys[0].key} (${keys[0].tenant_id || '首个密钥'})` : 'sk-nano-xxxx (留空将使用网关免密直通)'}
                      className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono"
                    />
                  </div>

                  {/* 1. CHAT CONTROLS */}
                  {playModality === 'chat' && (
                    <>
                      <div>
                        <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">测试接口协议</label>
                        <select
                          value={playProtocol}
                          onChange={(e) => setPlayProtocol(e.target.value)}
                          className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-sm text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                        >
                          <option value="openai_chat">OpenAI Chat (/v1/chat/completions 对话补全)</option>
                          <option value="openai_response">OpenAI Responses (/v1/responses 官方新代智能体协议)</option>
                          <option value="openai_text">OpenAI Text (/v1/completions 传统补全)</option>
                          <option value="anthropic_messages">Anthropic Claude (/v1/messages 原生协议)</option>
                        </select>
                      </div>

                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">目标模型 (已添加模型)</label>
                        {getPlaygroundModels('chat').length > 0 ? (
                          <select
                            value={getPlaygroundModels('chat').includes(playModel) ? playModel : (getPlaygroundModels('chat')[0] || '')}
                            onChange={(e) => setPlayModel(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-indigo-500 font-mono"
                          >
                            {getPlaygroundModels('chat').map((m) => (
                              <option key={m} value={m}>
                                {m}
                              </option>
                            ))}
                          </select>
                        ) : (
                          <div className="p-2.5 rounded-xl bg-amber-50 border border-amber-200 text-xs text-amber-800">
                            当前未挂载可用模型，请先前往【模型服务商】接入服务商并同步模型。
                          </div>
                        )}
                      </div>

                      {playProtocol !== 'openai_text' && (
                        <div>
                          <label className="block text-xs font-semibold text-slate-600 mb-1 flex items-center space-x-1">
                            <ImageIcon className="w-3.5 h-3.5 text-indigo-600" />
                            <span>多模态视觉图片 URL (可选)</span>
                          </label>
                          <input
                            type="text"
                            value={playImageUrl}
                            onChange={(e) => setPlayImageUrl(e.target.value)}
                            placeholder="https://... 或 data:image/png;base64,..."
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-indigo-500 focus:bg-white font-mono"
                          />
                        </div>
                      )}

                      <div className="flex items-center space-x-2 pt-2">
                        <input
                          type="checkbox"
                          id="streamCheck"
                          checked={playStream}
                          onChange={(e) => setPlayStream(e.target.checked)}
                          className="rounded border-slate-300 text-indigo-600 focus:ring-0"
                        />
                        <label htmlFor="streamCheck" className="text-sm font-medium text-slate-700 cursor-pointer">
                          开启 SSE 流式输出 (Streaming)
                        </label>
                      </div>
                    </>
                  )}

                  {/* 2. IMAGE CONTROLS */}
                  {playModality === 'images' && (
                    <>
                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">生图模型 (已添加模型)</label>
                        {getPlaygroundModels('images').length > 0 ? (
                          <select
                            value={getPlaygroundModels('images').includes(imgModel) ? imgModel : (getPlaygroundModels('images')[0] || '')}
                            onChange={(e) => setImgModel(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-pink-500 font-mono"
                          >
                            {getPlaygroundModels('images').map((m) => (
                              <option key={m} value={m}>{m}</option>
                            ))}
                          </select>
                        ) : (
                          <div className="p-2.5 rounded-xl bg-amber-50 border border-amber-200 text-xs text-amber-800">
                            平台暂无已添加的生图模型，请先前往【模型路由】配置。
                          </div>
                        )}
                      </div>

                      <div className="grid grid-cols-2 gap-2">
                        <div>
                          <label className="block text-xs font-semibold text-slate-600 mb-1">分辨率 (Size)</label>
                          <select
                            value={imgSize}
                            onChange={(e) => setImgSize(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-pink-500 focus:bg-white"
                          >
                            <option value="1024x1024">1024x1024 (方形)</option>
                            <option value="1792x1024">1792x1024 (横屏)</option>
                            <option value="1024x1792">1024x1792 (竖屏)</option>
                            <option value="512x512">512x512 (快速)</option>
                          </select>
                        </div>
                        <div>
                          <label className="block text-xs font-semibold text-slate-600 mb-1">画质 (Quality)</label>
                          <select
                            value={imgQuality}
                            onChange={(e) => setImgQuality(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-pink-500 focus:bg-white"
                          >
                            <option value="standard">standard (标准)</option>
                            <option value="hd">hd (高清渲染)</option>
                          </select>
                        </div>
                      </div>
                    </>
                  )}

                  {/* 3. TTS CONTROLS */}
                  {playModality === 'audio_speech' && (
                    <>
                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">TTS 语音合成模型 (已添加模型)</label>
                        {getPlaygroundModels('audio_speech').length > 0 ? (
                          <select
                            value={getPlaygroundModels('audio_speech').includes(ttsModel) ? ttsModel : (getPlaygroundModels('audio_speech')[0] || '')}
                            onChange={(e) => setTtsModel(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-cyan-500 font-mono"
                          >
                            {getPlaygroundModels('audio_speech').map((m) => (
                              <option key={m} value={m}>{m}</option>
                            ))}
                          </select>
                        ) : (
                          <div className="p-2.5 rounded-xl bg-amber-50 border border-amber-200 text-xs text-amber-800">
                            平台暂无已添加的语音合成模型，请先前往【模型路由】配置。
                          </div>
                        )}
                      </div>

                      <div className="grid grid-cols-2 gap-2">
                        <div>
                          <label className="block text-xs font-semibold text-slate-600 mb-1">音色 (Voice)</label>
                          <select
                            value={ttsVoice}
                            onChange={(e) => setTtsVoice(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-cyan-500 focus:bg-white"
                          >
                            <option value="alloy">alloy (自然中性)</option>
                            <option value="echo">echo (温和男声)</option>
                            <option value="fable">fable (英伦叙事)</option>
                            <option value="onyx">onyx (沉稳深邃)</option>
                            <option value="nova">nova (活泼清亮)</option>
                            <option value="shimmer">shimmer (清晰柔和)</option>
                          </select>
                        </div>
                        <div>
                          <label className="block text-xs font-semibold text-slate-600 mb-1">语速 (Speed: {ttsSpeed}x)</label>
                          <input
                            type="range"
                            min="0.5"
                            max="2.0"
                            step="0.1"
                            value={ttsSpeed}
                            onChange={(e) => setTtsSpeed(parseFloat(e.target.value))}
                            className="w-full mt-2 accent-cyan-600 cursor-pointer"
                          />
                        </div>
                      </div>
                    </>
                  )}

                  {/* 4. STT CONTROLS */}
                  {playModality === 'audio_transcription' && (
                    <>
                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">语音识别模型 (已添加模型)</label>
                        {getPlaygroundModels('audio_transcription').length > 0 ? (
                          <select
                            value={getPlaygroundModels('audio_transcription').includes(sttModel) ? sttModel : (getPlaygroundModels('audio_transcription')[0] || '')}
                            onChange={(e) => setSttModel(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-teal-500 font-mono"
                          >
                            {getPlaygroundModels('audio_transcription').map((m) => (
                              <option key={m} value={m}>{m}</option>
                            ))}
                          </select>
                        ) : (
                          <div className="p-2.5 rounded-xl bg-amber-50 border border-amber-200 text-xs text-amber-800">
                            平台暂无已添加的语音识别模型，请先前往【模型路由】配置。
                          </div>
                        )}
                      </div>

                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">选择录音 / 音频文件 (MP3, WAV, M4A)</label>
                        <input
                          type="file"
                          accept="audio/*,.mp3,.wav,.m4a,.webm"
                          onChange={(e) => setSttFile(e.target.files?.[0] || null)}
                          className="w-full text-xs text-slate-600 file:mr-3 file:py-2 file:px-4 file:rounded-xl file:border-0 file:text-xs file:font-semibold file:bg-teal-50 file:text-teal-700 hover:file:bg-teal-100 cursor-pointer border border-slate-200 rounded-xl p-2 bg-slate-50"
                        />
                      </div>
                    </>
                  )}

                  {/* 5. VIDEO CONTROLS */}
                  {playModality === 'videos' && (
                    <>
                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">视频生成模型 (已添加模型)</label>
                        {getPlaygroundModels('videos').length > 0 ? (
                          <select
                            value={getPlaygroundModels('videos').includes(videoModel) ? videoModel : (getPlaygroundModels('videos')[0] || '')}
                            onChange={(e) => setVideoModel(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-purple-500 font-mono"
                          >
                            {getPlaygroundModels('videos').map((m) => (
                              <option key={m} value={m}>{m}</option>
                            ))}
                          </select>
                        ) : (
                          <div className="p-2.5 rounded-xl bg-amber-50 border border-amber-200 text-xs text-amber-800">
                            平台暂无已添加的视频生成模型，请先前往【模型路由】配置。
                          </div>
                        )}
                      </div>

                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">视频画面比例 (Aspect Ratio)</label>
                        <select
                          value={videoAspectRatio}
                          onChange={(e) => setVideoAspectRatio(e.target.value)}
                          className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-purple-500 focus:bg-white"
                        >
                          <option value="16:9">16:9 (横屏电影感)</option>
                          <option value="9:16">9:16 (竖屏短视频)</option>
                          <option value="1:1">1:1 (方形)</option>
                        </select>
                      </div>
                    </>
                  )}

                  {/* 6. EMBEDDINGS CONTROLS */}
                  {playModality === 'embeddings' && (
                    <>
                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">向量模型 (已添加模型)</label>
                        {getPlaygroundModels('embeddings').length > 0 ? (
                          <select
                            value={getPlaygroundModels('embeddings').includes(embedModel) ? embedModel : (getPlaygroundModels('embeddings')[0] || '')}
                            onChange={(e) => setEmbedModel(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-emerald-500 font-mono"
                          >
                            {getPlaygroundModels('embeddings').map((m) => (
                              <option key={m} value={m}>{m}</option>
                            ))}
                          </select>
                        ) : (
                          <div className="p-2.5 rounded-xl bg-amber-50 border border-amber-200 text-xs text-amber-800">
                            平台暂无已添加的向量模型，请先前往【模型路由】配置。
                          </div>
                        )}
                      </div>
                    </>
                  )}

                  {/* 7. RERANK CONTROLS */}
                  {playModality === 'rerank' && (
                    <>
                      <div>
                        <label className="block text-xs font-semibold text-slate-600 mb-1">重排模型 (已添加模型)</label>
                        {getPlaygroundModels('rerank').length > 0 ? (
                          <select
                            value={getPlaygroundModels('rerank').includes(rerankModel) ? rerankModel : (getPlaygroundModels('rerank')[0] || '')}
                            onChange={(e) => setRerankModel(e.target.value)}
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-sm text-slate-800 focus:outline-none focus:border-amber-500 font-mono"
                          >
                            {getPlaygroundModels('rerank').map((m) => (
                              <option key={m} value={m}>{m}</option>
                            ))}
                          </select>
                        ) : (
                          <div className="p-2.5 rounded-xl bg-amber-50 border border-amber-200 text-xs text-amber-800">
                            平台暂无已添加的重排模型，请先前往【模型路由】配置。
                          </div>
                        )}
                      </div>

                      <div>
                        <div className="flex justify-between items-center mb-1">
                          <label className="text-xs font-semibold text-slate-600">截断输出条数 (Top N)</label>
                          <span className="font-mono text-xs font-bold text-amber-600">{rerankTopN} 条</span>
                        </div>
                        <input
                          type="range"
                          min="1"
                          max="10"
                          step="1"
                          value={rerankTopN}
                          onChange={(e) => setRerankTopN(parseInt(e.target.value) || 3)}
                          className="w-full accent-amber-600 cursor-pointer"
                        />
                      </div>
                    </>
                  )}

                  {/* Telemetry Footer */}
                  <div className="pt-4 border-t border-slate-100 text-xs text-slate-500 space-y-2">
                    {playModality === 'chat' && (
                      <div className="flex justify-between">
                        <span>首字延迟 (TTFT):</span>
                        <span className="font-mono font-bold text-amber-600">{playTTFTMs ? `${playTTFTMs} ms` : '-'}</span>
                      </div>
                    )}
                    <div className="flex justify-between">
                      <span>总执行耗时:</span>
                      <span className="font-mono font-bold text-indigo-600">{playDurationMs ? `${playDurationMs} ms` : '-'}</span>
                    </div>
                  </div>
                </div>

                {/* Right: Interactive Result & Output View */}
                <div className="lg:col-span-2 bg-white border border-slate-200/80 rounded-2xl p-6 flex flex-col h-[650px] shadow-xs">
                  {/* Result Body */}
                  <div className="flex-1 overflow-y-auto space-y-4 p-4 rounded-xl border border-slate-200 leading-relaxed bg-slate-50/60">
                    {/* Chat Result */}
                    {playModality === 'chat' && (
                      <div className="font-mono text-sm whitespace-pre-wrap text-slate-800 space-y-3">
                        {playReasoningOutput && (
                          <div className="p-3.5 bg-amber-50/70 border border-amber-200/80 rounded-xl text-xs text-amber-950 font-mono shadow-xs">
                            <div className="font-bold flex items-center space-x-1.5 text-amber-800 mb-1.5">
                              <Sparkles className="w-3.5 h-3.5 text-amber-600 animate-pulse" />
                              <span>深度思维链推理过程 (Reasoning Content)</span>
                            </div>
                            <div className="whitespace-pre-wrap leading-relaxed text-amber-900/90 text-[11px]">
                              {playReasoningOutput}
                            </div>
                          </div>
                        )}
                        <div>
                          {playOutput || (!playReasoningOutput && (
                            <div className="text-slate-400 text-center py-32 font-sans flex flex-col items-center justify-center space-y-2">
                              <Sparkles className="w-8 h-8 text-indigo-400 stroke-1" />
                              <span>暂无对话内容</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    {/* Image Result */}
                    {playModality === 'images' && (
                      <div className="h-full flex flex-col items-center justify-center">
                        {imgResult ? (
                          <div className="flex flex-col items-center space-y-3 w-full">
                            <div className="relative group max-h-[400px] overflow-hidden rounded-xl border border-slate-200 shadow-md bg-black">
                              <img
                                src={imgResult.url || `data:image/png;base64,${imgResult.b64_json}`}
                                alt="Generated"
                                className="max-h-[380px] w-auto object-contain mx-auto"
                              />
                            </div>
                            <div className="flex items-center space-x-3">
                              <a
                                href={imgResult.url || `data:image/png;base64,${imgResult.b64_json}`}
                                download="nano-gateway-generated.png"
                                target="_blank"
                                rel="noreferrer"
                                className="px-4 py-2 bg-pink-600 hover:bg-pink-700 text-white text-xs font-semibold rounded-xl flex items-center space-x-1.5 transition shadow-xs"
                              >
                                <Download className="w-3.5 h-3.5" />
                                <span>下载高清原图</span>
                              </a>
                              {imgResult.revised_prompt && (
                                <span className="text-xs text-slate-500 max-w-sm truncate" title={imgResult.revised_prompt}>
                                  Prompt: {imgResult.revised_prompt}
                                </span>
                              )}
                            </div>
                          </div>
                        ) : (
                          <div className="text-slate-400 text-center py-32 font-sans flex flex-col items-center justify-center space-y-2">
                            <ImageIcon className="w-8 h-8 text-pink-400 stroke-1" />
                            <span>暂无生成图片</span>
                          </div>
                        )}
                        {playOutput && (
                          <details className="w-full mt-4 text-xs font-mono bg-white p-3 rounded-xl border border-slate-200">
                            <summary className="cursor-pointer text-slate-500 font-semibold">查看接口完整 JSON 响应</summary>
                            <pre className="mt-2 text-slate-700 whitespace-pre-wrap">{playOutput}</pre>
                          </details>
                        )}
                      </div>
                    )}

                    {/* TTS Result */}
                    {playModality === 'audio_speech' && (
                      <div className="h-full flex flex-col items-center justify-center">
                        {ttsAudioUrl ? (
                          <div className="w-full max-w-md bg-white border border-slate-200 p-6 rounded-2xl shadow-sm text-center space-y-4">
                            <div className="w-12 h-12 bg-cyan-50 rounded-2xl flex items-center justify-center mx-auto text-cyan-600">
                              <Volume2 className="w-6 h-6" />
                            </div>
                            <div>
                              <h4 className="font-semibold text-slate-900">语音合成就绪</h4>
                              <p className="text-xs text-slate-500 mt-1 font-mono">模型: {ttsModel} · 音色: {ttsVoice}</p>
                            </div>
                            <audio controls autoPlay src={ttsAudioUrl} className="w-full" />
                            <a
                              href={ttsAudioUrl}
                              download="nano-gateway-speech.mp3"
                              className="inline-flex items-center space-x-2 px-4 py-2 bg-cyan-600 hover:bg-cyan-700 text-white rounded-xl text-xs font-semibold shadow-xs transition"
                            >
                              <Download className="w-3.5 h-3.5" />
                              <span>下载 MP3 音频文件</span>
                            </a>
                          </div>
                        ) : (
                          <div className="text-slate-400 text-center py-32 font-sans flex flex-col items-center justify-center space-y-2">
                            <Volume2 className="w-8 h-8 text-cyan-400 stroke-1" />
                            <span>暂无合成语音</span>
                          </div>
                        )}
                      </div>
                    )}

                    {/* STT Result */}
                    {playModality === 'audio_transcription' && (
                      <div className="h-full flex flex-col justify-between">
                        {sttResult ? (
                          <div className="space-y-3">
                            <div className="flex items-center justify-between">
                              <span className="text-xs font-semibold text-teal-700 uppercase tracking-wider">识别转写结果:</span>
                              <button
                                onClick={() => copyToClipboard(sttResult)}
                                className="px-3 py-1 bg-teal-50 hover:bg-teal-100 text-teal-700 rounded-lg text-xs font-medium flex items-center space-x-1"
                              >
                                <Copy className="w-3.5 h-3.5" />
                                <span>复制文本</span>
                              </button>
                            </div>
                            <div className="bg-white p-4 rounded-xl border border-slate-200 text-slate-900 font-sans leading-relaxed text-sm whitespace-pre-wrap">
                              {sttResult}
                            </div>
                          </div>
                        ) : (
                          <div className="text-slate-400 text-center py-32 font-sans flex flex-col items-center justify-center space-y-2">
                            <Mic className="w-8 h-8 text-teal-400 stroke-1" />
                            <span>暂无转写结果</span>
                          </div>
                        )}
                        {playOutput && (
                          <details className="w-full mt-4 text-xs font-mono bg-white p-3 rounded-xl border border-slate-200">
                            <summary className="cursor-pointer text-slate-500 font-semibold">查看 Whisper JSON 响应</summary>
                            <pre className="mt-2 text-slate-700 whitespace-pre-wrap">{playOutput}</pre>
                          </details>
                        )}
                      </div>
                    )}

                    {/* Video Result */}
                    {playModality === 'videos' && (
                      <div className="h-full flex flex-col items-center justify-center">
                        {videoTaskStatus ? (
                          <div className="w-full max-w-lg bg-white border border-slate-200 p-6 rounded-2xl shadow-sm text-center space-y-4">
                            <div className="flex items-center justify-between border-b border-slate-100 pb-3">
                              <span className="text-xs font-mono text-slate-500">Task: {videoTaskId || '创建中...'}</span>
                              <span className={`px-2.5 py-0.5 rounded text-xs font-mono font-semibold ${
                                videoTaskStatus === 'SUCCESS'
                                  ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
                                  : videoTaskStatus === 'FAILED'
                                  ? 'bg-rose-50 text-rose-700 border border-rose-200'
                                  : 'bg-purple-50 text-purple-700 border border-purple-200 animate-pulse'
                              }`}>
                                {videoTaskStatus} {videoPollCount > 0 && `(轮询: ${videoPollCount})`}
                              </span>
                            </div>

                            {videoResultUrl ? (
                              <div className="space-y-3">
                                <video controls autoPlay src={videoResultUrl} className="w-full rounded-xl max-h-[320px] bg-black" />
                                <a
                                  href={videoResultUrl}
                                  download="nano-gateway-video.mp4"
                                  target="_blank"
                                  rel="noreferrer"
                                  className="inline-flex items-center space-x-2 px-4 py-2 bg-purple-600 hover:bg-purple-700 text-white rounded-xl text-xs font-semibold shadow-xs transition"
                                >
                                  <Download className="w-3.5 h-3.5" />
                                  <span>下载视频</span>
                                </a>
                              </div>
                            ) : (
                              <div className="py-8 flex flex-col items-center space-y-2">
                                <RefreshCw className="w-8 h-8 text-purple-500 animate-spin" />
                                <p className="text-sm font-semibold text-slate-700">正在生成视频...</p>
                              </div>
                            )}
                          </div>
                        ) : (
                          <div className="text-slate-400 text-center py-32 font-sans flex flex-col items-center justify-center space-y-2">
                            <Video className="w-8 h-8 text-purple-400 stroke-1" />
                            <span>暂无生成视频</span>
                          </div>
                        )}
                        {playOutput && (
                          <details className="w-full mt-4 text-xs font-mono bg-white p-3 rounded-xl border border-slate-200">
                            <summary className="cursor-pointer text-slate-500 font-semibold">查看接口完整 JSON 响应</summary>
                            <pre className="mt-2 text-slate-700 whitespace-pre-wrap">{playOutput}</pre>
                          </details>
                        )}
                      </div>
                    )}

                    {/* Embeddings Result */}
                    {playModality === 'embeddings' && (
                      <div className="h-full flex flex-col justify-between">
                        {embedResult ? (
                          <div className="space-y-4">
                            <div className="flex items-center justify-between bg-white p-3 rounded-xl border border-slate-200">
                              <div className="flex items-center space-x-3 text-xs">
                                <span className="font-semibold text-slate-700">特征维度:</span>
                                <span className="px-2 py-0.5 bg-emerald-50 text-emerald-700 font-mono font-bold rounded border border-emerald-200">
                                   {embedDim} 维
                                </span>
                                <span className="font-semibold text-slate-700">条数:</span>
                                <span className="font-mono font-bold text-slate-900">
                                  {embedResult.data?.length || 1} 条
                                </span>
                                {embedResult.usage && (
                                  <>
                                    <span className="font-semibold text-slate-700">Prompt Tokens:</span>
                                    <span className="font-mono font-bold text-indigo-600">
                                      {embedResult.usage.prompt_tokens}
                                    </span>
                                  </>
                                )}
                              </div>
                              <button
                                onClick={() => copyToClipboard(JSON.stringify(embedResult.data, null, 2))}
                                className="px-3 py-1 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 rounded-lg text-xs font-medium flex items-center space-x-1"
                              >
                                <Copy className="w-3.5 h-3.5" />
                                <span>复制向量数据</span>
                              </button>
                            </div>

                            {/* Visual Vector Preview */}
                            <div className="bg-white p-4 rounded-xl border border-slate-200 space-y-2">
                              <span className="text-xs font-bold text-slate-700 block">首条特征前 16 维数值热度预览:</span>
                              <div className="grid grid-cols-4 sm:grid-cols-8 gap-1.5 font-mono text-[11px]">
                                {(embedResult.data?.[0]?.embedding || []).slice(0, 16).map((val, idx) => (
                                  <div
                                    key={idx}
                                    className={`p-1.5 rounded text-center font-semibold truncate ${
                                      val >= 0
                                        ? 'bg-emerald-50 text-emerald-800 border border-emerald-100'
                                        : 'bg-rose-50 text-rose-800 border border-rose-100'
                                    }`}
                                    title={`维度 #${idx}: ${val}`}
                                  >
                                    {val.toFixed(4)}
                                  </div>
                                ))}
                              </div>
                            </div>
                          </div>
                        ) : (
                          <div className="text-slate-400 text-center py-32 font-sans flex flex-col items-center justify-center space-y-2">
                            <Cpu className="w-8 h-8 text-emerald-400 stroke-1" />
                            <span>暂无向量特征数据</span>
                          </div>
                        )}

                        {playOutput && (
                          <details className="w-full mt-4 text-xs font-mono bg-white p-3 rounded-xl border border-slate-200">
                            <summary className="cursor-pointer text-slate-500 font-semibold">查看接口完整 JSON 响应</summary>
                            <pre className="mt-2 text-slate-700 whitespace-pre-wrap max-h-48 overflow-y-auto">{playOutput}</pre>
                          </details>
                        )}
                      </div>
                    )}

                    {/* 7. Rerank Result */}
                    {playModality === 'rerank' && (
                      <div className="h-full flex flex-col justify-between">
                        {rerankResult ? (
                          <div className="space-y-4">
                            <div className="flex items-center justify-between bg-white p-3 rounded-xl border border-slate-200">
                              <div className="flex items-center space-x-3 text-xs">
                                <span className="font-semibold text-slate-700">命中排序:</span>
                                <span className="px-2 py-0.5 bg-amber-50 text-amber-700 font-mono font-bold rounded border border-amber-200">
                                  Top {rerankResult.results?.length || 0}
                                </span>
                                {rerankResult.usage && (
                                  <>
                                    <span className="font-semibold text-slate-700">总 Token:</span>
                                    <span className="font-mono font-bold text-indigo-600">
                                      {rerankResult.usage.total_tokens || rerankResult.usage.prompt_tokens || 0}
                                    </span>
                                  </>
                                )}
                              </div>
                              <button
                                onClick={() => copyToClipboard(JSON.stringify(rerankResult.results, null, 2))}
                                className="px-3 py-1 bg-amber-50 hover:bg-amber-100 text-amber-700 rounded-lg text-xs font-medium flex items-center space-x-1"
                              >
                                <Copy className="w-3.5 h-3.5" />
                                <span>复制重排数据</span>
                              </button>
                            </div>

                            {/* Ranked Cards */}
                            <div className="space-y-3 overflow-y-auto max-h-[460px] pr-1">
                              {(rerankResult.results || []).map((item, idx) => {
                                const rawScore = Number(item.relevance_score || 0);
                                const pct = Math.min(100, Math.max(0, rawScore > 1 ? rawScore : rawScore * 100));
                                const docText = typeof item.document === 'object' ? item.document?.text : item.document;
                                return (
                                  <div key={idx} className="bg-white p-4 rounded-xl border border-slate-200/90 shadow-xs space-y-2.5">
                                    <div className="flex items-center justify-between">
                                      <div className="flex items-center space-x-2">
                                        <span className={`px-2.5 py-0.5 rounded text-xs font-bold font-mono ${
                                          idx === 0
                                            ? 'bg-amber-100 text-amber-800 border border-amber-300'
                                            : idx === 1
                                            ? 'bg-emerald-100 text-emerald-800 border border-emerald-300'
                                            : 'bg-indigo-50 text-indigo-700 border border-indigo-200'
                                        }`}>
                                          #{idx + 1}
                                        </span>
                                        <span className="text-xs text-slate-400 font-mono">原文档序号: #{item.index}</span>
                                      </div>
                                      <span className="text-xs font-mono font-bold text-slate-800">
                                        得分: <span className="text-amber-600">{rawScore.toFixed(4)}</span>
                                      </span>
                                    </div>
                                    <div className="w-full bg-slate-100 h-1.5 rounded-full overflow-hidden">
                                      <div
                                        className={`h-full rounded-full transition-all duration-500 ${
                                          idx === 0 ? 'bg-amber-500' : idx === 1 ? 'bg-emerald-500' : 'bg-indigo-500'
                                        }`}
                                        style={{ width: `${pct}%` }}
                                      />
                                    </div>
                                    {docText && (
                                      <div className="text-xs text-slate-700 leading-relaxed font-sans bg-slate-50/80 p-3 rounded-lg border border-slate-100">
                                        {docText}
                                      </div>
                                    )}
                                  </div>
                                );
                              })}
                            </div>
                          </div>
                        ) : (
                          <div className="text-slate-400 text-center py-32 font-sans flex flex-col items-center justify-center space-y-2">
                            <Sliders className="w-8 h-8 text-amber-400 stroke-1" />
                            <span>暂无重排结果</span>
                          </div>
                        )}

                        {playOutput && (
                          <details className="w-full mt-4 text-xs font-mono bg-white p-3 rounded-xl border border-slate-200">
                            <summary className="cursor-pointer text-slate-500 font-semibold">查看接口完整 JSON 响应</summary>
                            <pre className="mt-2 text-slate-700 whitespace-pre-wrap max-h-48 overflow-y-auto">{playOutput}</pre>
                          </details>
                        )}
                      </div>
                    )}
                  </div>

                  {/* Input & Action Bar */}
                  <div className="pt-4 flex space-x-3">
                    {playModality === 'chat' && (
                      <>
                        <textarea
                          rows={2}
                          value={playPrompt}
                          onChange={(e) => setPlayPrompt(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' && e.ctrlKey) handleSendChat();
                          }}
                          placeholder="输入测试提示词... (Ctrl+Enter 发送)"
                          className="flex-1 bg-slate-50 border border-slate-200 rounded-xl p-3 text-sm text-slate-800 focus:outline-none focus:border-indigo-500 focus:bg-white resize-none"
                        />
                        <button
                          onClick={handleSendChat}
                          disabled={playLoading}
                          className="px-6 bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white rounded-xl font-semibold flex items-center justify-center space-x-2 transition shadow-sm"
                        >
                          {playLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
                          <span>发送</span>
                        </button>
                      </>
                    )}

                    {playModality === 'images' && (
                      <>
                        <textarea
                          rows={2}
                          value={imgPrompt}
                          onChange={(e) => setImgPrompt(e.target.value)}
                          placeholder="输入画面描述词 Prompt..."
                          className="flex-1 bg-slate-50 border border-slate-200 rounded-xl p-3 text-sm text-slate-800 focus:outline-none focus:border-pink-500 focus:bg-white resize-none"
                        />
                        <button
                          onClick={handleGenerateImage}
                          disabled={playLoading}
                          className="px-6 bg-pink-600 hover:bg-pink-700 disabled:opacity-50 text-white rounded-xl font-semibold flex items-center justify-center space-x-2 transition shadow-sm"
                        >
                          {playLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <ImageIcon className="w-4 h-4" />}
                          <span>生成图片</span>
                        </button>
                      </>
                    )}

                    {playModality === 'audio_speech' && (
                      <>
                        <textarea
                          rows={2}
                          value={ttsInput}
                          onChange={(e) => setTtsInput(e.target.value)}
                          placeholder="输入要转成语音的文本内容..."
                          className="flex-1 bg-slate-50 border border-slate-200 rounded-xl p-3 text-sm text-slate-800 focus:outline-none focus:border-cyan-500 focus:bg-white resize-none"
                        />
                        <button
                          onClick={handleGenerateSpeech}
                          disabled={playLoading}
                          className="px-6 bg-cyan-600 hover:bg-cyan-700 disabled:opacity-50 text-white rounded-xl font-semibold flex items-center justify-center space-x-2 transition shadow-sm"
                        >
                          {playLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Volume2 className="w-4 h-4" />}
                          <span>合成语音</span>
                        </button>
                      </>
                    )}

                    {playModality === 'audio_transcription' && (
                      <button
                        onClick={handleTranscribeAudio}
                        disabled={playLoading || !sttFile}
                        className="w-full py-3 bg-teal-600 hover:bg-teal-700 disabled:opacity-50 text-white rounded-xl font-semibold flex items-center justify-center space-x-2 transition shadow-sm"
                      >
                        {playLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Mic className="w-4 h-4" />}
                        <span>开始语音识别并转录</span>
                      </button>
                    )}

                    {playModality === 'videos' && (
                      <>
                        <textarea
                          rows={2}
                          value={videoPrompt}
                          onChange={(e) => setVideoPrompt(e.target.value)}
                          placeholder="输入视频场景描述词 Prompt..."
                          className="flex-1 bg-slate-50 border border-slate-200 rounded-xl p-3 text-sm text-slate-800 focus:outline-none focus:border-purple-500 focus:bg-white resize-none"
                        />
                        <button
                          onClick={handleGenerateVideo}
                          disabled={playLoading}
                          className="px-6 bg-purple-600 hover:bg-purple-700 disabled:opacity-50 text-white rounded-xl font-semibold flex items-center justify-center space-x-2 transition shadow-sm"
                        >
                          {playLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Video className="w-4 h-4" />}
                          <span>创建视频任务</span>
                        </button>
                      </>
                    )}

                    {playModality === 'embeddings' && (
                      <>
                        <textarea
                          rows={2}
                          value={embedInput}
                          onChange={(e) => setEmbedInput(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' && e.ctrlKey) handleGenerateEmbedding();
                          }}
                          placeholder="输入待向量化文本，支持换行批量输入... (Ctrl+Enter 发送)"
                          className="flex-1 bg-slate-50 border border-slate-200 rounded-xl p-3 text-sm text-slate-800 focus:outline-none focus:border-emerald-500 focus:bg-white resize-none"
                        />
                        <button
                          onClick={handleGenerateEmbedding}
                          disabled={playLoading || !embedInput.trim()}
                          className="px-6 bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 text-white rounded-xl font-semibold flex items-center justify-center space-x-2 transition shadow-sm text-sm"
                        >
                          {playLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                          <span>生成向量</span>
                        </button>
                      </>
                    )}

                    {playModality === 'rerank' && (
                      <div className="flex-1 flex flex-col sm:flex-row space-y-2 sm:space-y-0 sm:space-x-3">
                        <div className="flex-1 space-y-2">
                          <input
                            type="text"
                            value={rerankQuery}
                            onChange={(e) => setRerankQuery(e.target.value)}
                            placeholder="输入检索 Query 查询语句..."
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs text-slate-800 focus:outline-none focus:border-amber-500 focus:bg-white font-medium"
                          />
                          <textarea
                            rows={2}
                            value={rerankDocs}
                            onChange={(e) => setRerankDocs(e.target.value)}
                            placeholder="输入候选文档列表（使用 --- 隔开每篇文档）..."
                            className="w-full bg-slate-50 border border-slate-200 rounded-xl p-2.5 text-xs text-slate-800 focus:outline-none focus:border-amber-500 focus:bg-white resize-none font-mono"
                          />
                        </div>
                        <button
                          onClick={handleExecuteRerank}
                          disabled={playLoading || !rerankQuery.trim() || !rerankDocs.trim()}
                          className="px-6 bg-amber-600 hover:bg-amber-700 disabled:opacity-50 text-white rounded-xl font-semibold flex items-center justify-center space-x-2 transition shadow-sm text-sm shrink-0 self-end sm:self-stretch"
                        >
                          {playLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Sliders className="w-4 h-4" />}
                          <span>执行重排</span>
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* 6. DOCS TAB */}
          {/* 6. DOCS TAB */}
          {currentTab === 'docs' && (
            <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
              {/* Category sidebar */}
              <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-4 space-y-1 shadow-xs h-fit">
                <span className="text-xs font-bold text-slate-400 dark:text-slate-500 uppercase tracking-wider px-3 mb-2 block">接入与规范文档</span>
                <button
                  onClick={() => setDocsSection('architecture')}
                  className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition ${
                    docsSection === 'architecture'
                      ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
                      : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <Shield className="w-3.5 h-3.5 text-indigo-500" />
                  <span>核心架构与高可用设计</span>
                </button>
                <button
                  onClick={() => setDocsSection('quickstart')}
                  className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition ${
                    docsSection === 'quickstart'
                      ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
                      : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <Code className="w-3.5 h-3.5" />
                  <span>OpenAI SDK 极速接入</span>
                </button>
                <button
                  onClick={() => setDocsSection('claude')}
                  className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition ${
                    docsSection === 'claude'
                      ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
                      : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <Terminal className="w-3.5 h-3.5" />
                  <span>Claude Messages API 接入</span>
                </button>
                <button
                  onClick={() => setDocsSection('multimodal')}
                  className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition ${
                    docsSection === 'multimodal'
                      ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
                      : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <ImageIcon className="w-3.5 h-3.5" />
                  <span>多模态 (图/音/视) 接口规范</span>
                </button>
                <button
                  onClick={() => setDocsSection('cascading')}
                  className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition ${
                    docsSection === 'cascading'
                      ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
                      : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <Layers className="w-3.5 h-3.5" />
                  <span>级联模型映射语法</span>
                </button>
                <button
                  onClick={() => setDocsSection('rerank')}
                  className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition ${
                    docsSection === 'rerank'
                      ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
                      : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <Sliders className="w-3.5 h-3.5" />
                  <span>Rerank 检索重排规范</span>
                </button>
                <button
                  onClick={() => setDocsSection('deploy')}
                  className={`w-full text-left px-3 py-2 rounded-xl text-xs font-semibold flex items-center space-x-2 transition ${
                    docsSection === 'deploy'
                      ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 font-bold border border-indigo-200 dark:border-indigo-800/60'
                      : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <Server className="w-3.5 h-3.5" />
                  <span>Docker & K8s 高可用部署</span>
                </button>
              </div>

              {/* Doc Content */}
              <div className="md:col-span-3 bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-6">
                {docsSection === 'architecture' && (
                  <div className="space-y-6">
                    <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
                      <div className="flex items-center space-x-2">
                        <Shield className="w-5 h-5 text-indigo-500" />
                        <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">
                          Nano-Gateway 核心架构与高可用设计规范
                        </h3>
                      </div>
                      <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                        为企业私有化部署打造的超高性能、全双工协议转换与零感知容灾大模型统一接入网关。
                      </p>
                    </div>

                    {/* Feature 1: Pre-Token Fallback */}
                    <div className="p-5 rounded-2xl bg-indigo-50/50 dark:bg-indigo-950/30 border border-indigo-200/80 dark:border-indigo-800/60 space-y-3">
                      <div className="flex items-center justify-between">
                        <h4 className="font-bold text-sm text-indigo-900 dark:text-indigo-200 flex items-center space-x-2">
                          <Shield className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
                          <span>1. 首字前无感容灾兜底 (Pre-Token Fallback)</span>
                        </h4>
                        <span className="text-[11px] font-mono px-2 py-0.5 rounded-md bg-indigo-100 dark:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 font-semibold">
                          Zero-Perception Failover
                        </span>
                      </div>
                      <p className="text-xs text-slate-700 dark:text-slate-300 leading-relaxed">
                        遭遇上游 429 (并发限流)、500/502/503 (服务端异常) 或网络连接超时，首字分块发出前毫秒内切换到备份 Provider，客户端完全无感，连接不中断，零错误率。
                      </p>
                      <div className="bg-white dark:bg-[#0b0f19] p-3.5 rounded-xl border border-indigo-100 dark:border-indigo-900/50 text-xs space-y-2">
                        <span className="font-semibold text-slate-800 dark:text-slate-200 block">容灾工作流时序:</span>
                        <div className="font-mono text-[11px] text-slate-600 dark:text-slate-400 space-y-1">
                          <div>[客户端请求] ➔ Nano-Gateway ➔ 首选渠道 A (Primary Provider)</div>
                          <div className="text-amber-600 dark:text-amber-400">↳ 发生 429 Rate Limit / 500 错误 (未输出首字 chunk)</div>
                          <div className="text-emerald-600 dark:text-emerald-400">↳ 网关拦截异常并在 15ms 内重定向至备份渠道 B (Backup Provider)</div>
                          <div>↳ 客户端正常接收首字分块及完整 SSE 数据流，业务层调用成功率稳定保持 100%</div>
                        </div>
                      </div>
                    </div>

                    {/* Feature 2: Protocol Translation */}
                    <div className="p-5 rounded-2xl bg-emerald-50/50 dark:bg-emerald-950/30 border border-emerald-200/80 dark:border-emerald-800/60 space-y-3">
                      <div className="flex items-center justify-between">
                        <h4 className="font-bold text-sm text-emerald-900 dark:text-emerald-200 flex items-center space-x-2">
                          <Cpu className="w-4 h-4 text-emerald-600 dark:text-emerald-400" />
                          <span>2. 智能探测 & 全双工协议转换 (Full-Duplex Protocol Matrix)</span>
                        </h4>
                        <span className="text-[11px] font-mono px-2 py-0.5 rounded-md bg-emerald-100 dark:bg-emerald-900/60 text-emerald-700 dark:text-emerald-300 font-semibold">
                          Bi-Directional Translation
                        </span>
                      </div>
                      <p className="text-xs text-slate-700 dark:text-slate-300 leading-relaxed">
                        自动识别 GPUStack、vLLM、Sub2API、Claude Messages、Gemini 与 OpenAI 原生协议，并实现全双工实时转译。客户端使用任何协议均可调用任意异构上游！
                      </p>
                      <div className="overflow-x-auto">
                        <table className="w-full text-left border-collapse text-xs">
                          <thead>
                            <tr className="border-b border-emerald-200/60 dark:border-emerald-800/60 text-slate-500 dark:text-slate-400">
                              <th className="py-2 px-3 font-semibold">下游客户端调用协议</th>
                              <th className="py-2 px-3 font-semibold">网关接入端点</th>
                              <th className="py-2 px-3 font-semibold">支持的上游后端提供商</th>
                            </tr>
                          </thead>
                          <tbody className="divide-y divide-emerald-100 dark:divide-emerald-900/40 text-[11px]">
                            <tr>
                              <td className="py-2 px-3 font-mono text-indigo-600 dark:text-indigo-400 font-semibold">OpenAI Chat</td>
                              <td className="py-2 px-3 font-mono">/v1/chat/completions</td>
                              <td className="py-2 px-3">OpenAI, DeepSeek, GPUStack, vLLM, Claude, Gemini</td>
                            </tr>
                            <tr>
                              <td className="py-2 px-3 font-mono text-indigo-600 dark:text-indigo-400 font-semibold">OpenAI Responses</td>
                              <td className="py-2 px-3 font-mono">/v1/responses</td>
                              <td className="py-2 px-3">新代智能体协议，支持全格式转译与流式推导</td>
                            </tr>
                            <tr>
                              <td className="py-2 px-3 font-mono text-indigo-600 dark:text-indigo-400 font-semibold">Claude Messages</td>
                              <td className="py-2 px-3 font-mono">/v1/messages</td>
                              <td className="py-2 px-3">Anthropic 官方、OpenAI 格式上游、GPUStack 集群</td>
                            </tr>
                            <tr>
                              <td className="py-2 px-3 font-mono text-indigo-600 dark:text-indigo-400 font-semibold">Google Gemini</td>
                              <td className="py-2 px-3 font-mono">/v1beta/models/*</td>
                              <td className="py-2 px-3">Gemini 官方、OpenAI 格式上游全双工转译</td>
                            </tr>
                          </tbody>
                        </table>
                      </div>
                    </div>

                    {/* Feature 3: Multimodal Pipeline */}
                    <div className="p-5 rounded-2xl bg-purple-50/50 dark:bg-purple-950/30 border border-purple-200/80 dark:border-purple-800/60 space-y-3">
                      <div className="flex items-center justify-between">
                        <h4 className="font-bold text-sm text-purple-900 dark:text-purple-200 flex items-center space-x-2">
                          <Layers className="w-4 h-4 text-purple-600 dark:text-purple-400" />
                          <span>3. 全模态统一管道 (Unified Pipeline)</span>
                        </h4>
                        <span className="text-[11px] font-mono px-2 py-0.5 rounded-md bg-purple-100 dark:bg-purple-900/60 text-purple-700 dark:text-purple-300 font-semibold">
                          Unified Pipeline
                        </span>
                      </div>
                      <p className="text-xs text-slate-700 dark:text-slate-300 leading-relaxed">
                        Chat 对话、AI 生图 (DALL-E / Flux)、语音合成 (TTS)、Whisper 语音转录与翻译、视频生成均采用统一调度分发，高内聚低耦合，共享熔断、鉴权与计量基础设施。
                      </p>
                      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-1 text-center">
                        <div className="p-2.5 rounded-xl bg-white dark:bg-[#0b0f19] border border-purple-100 dark:border-purple-900/50">
                          <span className="text-xs font-bold text-purple-700 dark:text-purple-300">💬 文本/对话</span>
                          <p className="text-[10px] text-slate-400 mt-0.5">SSE 零拷贝流式</p>
                        </div>
                        <div className="p-2.5 rounded-xl bg-white dark:bg-[#0b0f19] border border-purple-100 dark:border-purple-900/50">
                          <span className="text-xs font-bold text-pink-700 dark:text-pink-300">🎨 图像生成</span>
                          <p className="text-[10px] text-slate-400 mt-0.5">/v1/images/generations</p>
                        </div>
                        <div className="p-2.5 rounded-xl bg-white dark:bg-[#0b0f19] border border-purple-100 dark:border-purple-900/50">
                          <span className="text-xs font-bold text-cyan-700 dark:text-cyan-300">🔊 语音 TTS/STT</span>
                          <p className="text-[10px] text-slate-400 mt-0.5">音频二进制直通流</p>
                        </div>
                        <div className="p-2.5 rounded-xl bg-white dark:bg-[#0b0f19] border border-purple-100 dark:border-purple-900/50">
                          <span className="text-xs font-bold text-teal-700 dark:text-teal-300">🎬 视频生成</span>
                          <p className="text-[10px] text-slate-400 mt-0.5">异步任务与轮询</p>
                        </div>
                      </div>
                    </div>

                    {/* Feature 4: High Availability */}
                    <div className="p-5 rounded-2xl bg-sky-50/50 dark:bg-sky-950/30 border border-sky-200/80 dark:border-sky-800/60 space-y-2">
                      <h4 className="font-bold text-sm text-sky-900 dark:text-sky-200 flex items-center space-x-2">
                        <Server className="w-4 h-4 text-sky-600 dark:text-sky-400" />
                        <span>4. Zero-DB 热路径与双节点双活高可用架构</span>
                      </h4>
                      <p className="text-xs text-slate-700 dark:text-slate-300 leading-relaxed">
                        数据面读操作 100% 内存无锁运行，无任何数据库 IO 延迟；控制面配置变更通过 WAL 模式原子写入并毫秒级广播至内存。前置 Nginx 对 2 个网关副本进行加权轮询，实现节点故障秒级隔离与 99.99% 高可用。
                      </p>
                    </div>
                  </div>
                )}

                {docsSection === 'quickstart' && (
                  <div className="space-y-4">
                    <div className="border-b border-slate-100 dark:border-slate-800 pb-3">
                      <h3 className="text-base font-bold text-slate-900 dark:text-slate-100">Python OpenAI SDK 接入指南</h3>
                      <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">将官方 OpenAI SDK 的 base_url 直接指向 Nano-Gateway 网关入口即可。</p>
                    </div>

                    <div className="relative group">
                      <pre className="p-4 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto leading-relaxed">
{`from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",  # Nano-Gateway 负载均衡端口
    api_key="sk-nano-xxxx",               # 在工作台签发的客户端访问密钥
)

response = client.chat.completions.create(
    model="deepseek-v3",                 # 支持任意映射模型
    messages=[{"role": "user", "content": "你好！"}],
    stream=True,                         # 原生毫秒级 SSE 流式传输
)

for chunk in response:
    content = chunk.choices[0].delta.content or ""
    print(content, end="", flush=True)`}
                      </pre>
                      <button
                        onClick={() => copyToClipboard(`from openai import OpenAI\n\nclient = OpenAI(\n    base_url="http://localhost:8080/v1",\n    api_key="sk-nano-xxxx",\n)\n\nresponse = client.chat.completions.create(\n    model="deepseek-v3",\n    messages=[{"role": "user", "content": "你好！"}],\n    stream=True,\n)\n\nfor chunk in response:\n    content = chunk.choices[0].delta.content or ""\n    print(content, end="", flush=True)`)}
                        className="absolute top-3 right-3 px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-[11px] font-mono flex items-center space-x-1"
                      >
                        <Copy className="w-3 h-3" />
                        <span>复制</span>
                      </button>
                    </div>

                    <div className="p-4 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/60 rounded-xl text-xs text-amber-900 dark:text-amber-200 space-y-1">
                      <span className="font-bold flex items-center space-x-1">
                        <Sparkles className="w-3.5 h-3.5 text-amber-600 dark:text-amber-400" />
                        <span>DeepSeek-R1 / OpenAI o1 思维链透明透传:</span>
                      </span>
                      <p className="leading-relaxed">
                        网关在流式与非流式模式下均原生支持 <code>reasoning_content</code> 字段的零拷贝透传。前端应用或 NextChat / LobeChat 可直接原生渲染展开思维链推导卡片。
                      </p>
                    </div>

                    <div className="border-t border-slate-100 dark:border-slate-800 pt-4">
                      <h4 className="text-xs font-bold text-slate-800 dark:text-slate-200 mb-2">cURL 极速调试命令:</h4>
                      <pre className="p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl text-xs font-mono text-slate-700 dark:text-slate-300 overflow-x-auto">
{`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer sk-nano-xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "deepseek-v3", "messages": [{"role": "user", "content": "Ping"}], "stream": true}'`}
                      </pre>
                    </div>
                  </div>
                )}

                {docsSection === 'claude' && (
                  <div className="space-y-4">
                    <div className="border-b border-slate-100 pb-3">
                      <h3 className="text-base font-bold text-slate-900">Anthropic Claude Messages API 接入</h3>
                      <p className="text-xs text-slate-500 mt-0.5">原生支持 Claude Code、Cursor、Cline 等工具直接使用 Anthropic 原生协议调用任何异构下游！</p>
                    </div>

                    <pre className="p-4 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto leading-relaxed">
{`import anthropic

client = anthropic.Anthropic(
    base_url="http://localhost:8080",  # 网关根路径，将自动请求 /v1/messages
    api_key="sk-nano-xxxx",            # 在工作台签发的客户端访问密钥
)

message = client.messages.create(
    model="claude-3-7-sonnet",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "请介绍量子计算的核心原理。"}
    ]
)
print(message.content[0].text)`}
                    </pre>

                    <div className="p-4 bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800/60 rounded-xl text-xs text-indigo-900 dark:text-indigo-200 space-y-1">
                      <span className="font-bold">💡 全双工协议转换特性:</span>
                      <p>
                        即使您的下游供应商是仅支持 OpenAI 协议的私有 GPUStack 集群，客户端通过 Anthropic SDK 请求时，网关也会在内存零拷贝将 Claude Messages 双向转换为 OpenAI Completions 并在返回时转回 Anthropic 格式。
                      </p>
                    </div>

                    <div className="border-t border-slate-100 dark:border-slate-800 pt-4">
                      <h4 className="text-xs font-bold text-slate-800 dark:text-slate-200 mb-1">Anthropic Token 预估计算 (/v1/messages/count_tokens):</h4>
                      <pre className="p-3 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto">
{`curl -X POST http://localhost:8080/v1/messages/count_tokens \\
  -H "x-api-key: sk-nano-xxxx" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "claude-3-5-sonnet", "messages": [{"role": "user", "content": "Hello world"}]}'

# 响应示例:
# {"input_tokens": 12}`}
                      </pre>
                    </div>
                  </div>
                )}

                {docsSection === 'multimodal' && (
                  <div className="space-y-4">
                    <div className="border-b border-slate-100 pb-3">
                      <h3 className="text-base font-bold text-slate-900">多模态 API 接口规范</h3>
                      <p className="text-xs text-slate-500 mt-0.5">生图、语音合成 TTS、语音识别 STT、语音翻译、视频生成与轮询均通过统一熔断与分发管道提供。</p>
                    </div>

                    <div className="space-y-3 text-xs">
                      <div className="p-3 bg-slate-50 border border-slate-200 rounded-xl">
                        <span className="font-bold text-pink-700">🎨 1. AI 图像生成 (/v1/images/generations)</span>
                        <pre className="mt-1 font-mono text-slate-700">
{`POST /v1/images/generations
{"model": "dall-e-3", "prompt": "cyberpunk city, 8k", "size": "1024x1024"}`}
                        </pre>
                      </div>

                      <div className="p-3 bg-slate-50 border border-slate-200 rounded-xl">
                        <span className="font-bold text-cyan-700">🔊 2. 语音合成 TTS (/v1/audio/speech)</span>
                        <pre className="mt-1 font-mono text-slate-700">
{`POST /v1/audio/speech
{"model": "tts-1", "input": "你好世界", "voice": "alloy", "response_format": "mp3"}
(返回二进制流式音频流，零内存占用直连客户端)`}
                        </pre>
                      </div>

                      <div className="p-3 bg-slate-50 border border-slate-200 rounded-xl">
                        <span className="font-bold text-teal-700">🎙️ 3. Whisper 语音转录 (/v1/audio/transcriptions)</span>
                        <pre className="mt-1 font-mono text-slate-700">
{`POST /v1/audio/transcriptions (multipart/form-data)
file=@recording.mp3; model=whisper-1`}
                        </pre>
                      </div>

                      <div className="p-3 bg-slate-50 border border-slate-200 rounded-xl">
                        <span className="font-bold text-teal-700">🌐 4. Whisper 语音翻译 (/v1/audio/translations)</span>
                        <pre className="mt-1 font-mono text-slate-700">
{`POST /v1/audio/translations (multipart/form-data)
file=@foreign_speech.mp3; model=whisper-1
(将源语言音频直接翻译并转录为英文文本)`}
                        </pre>
                      </div>

                      <div className="p-3 bg-slate-50 border border-slate-200 rounded-xl">
                        <span className="font-bold text-purple-700">🎬 5. 视频生成与轮询 (/v1/videos/generations & /v1/videos/tasks/:id)</span>
                        <pre className="mt-1 font-mono text-slate-700">
{`POST /v1/videos/generations -> 返回 {"task_id": "task_xxx", "status": "PENDING"}
GET /v1/videos/tasks/:id    -> 轮询状态直到 SUCCESS 并返回 video_url`}
                        </pre>
                      </div>

                      <div className="p-3 bg-slate-50 border border-slate-200 rounded-xl">
                        <span className="font-bold text-emerald-700">🧠 6. 文本向量化 Embeddings (/v1/embeddings)</span>
                        <pre className="mt-1 font-mono text-slate-700">
{`POST /v1/embeddings
{"model": "text-embedding-3-small", "input": "企业级超高性能大模型网关"}
(支持单文本或数组批量输入，自动适配 GPUStack、vLLM、Ollama、Gemini 与 OpenAI 原生接口)`}
                        </pre>
                      </div>
                    </div>
                  </div>
                )}

                {docsSection === 'cascading' && (
                  <div className="space-y-4">
                    <div className="border-b border-slate-100 pb-3">
                      <h3 className="text-base font-bold text-slate-900">级联模型别名与通配映射</h3>
                      <p className="text-xs text-slate-500 mt-0.5">不设任何斜杠深度限制，支持多组织层级命名与任意前缀重写。</p>
                    </div>

                    <div className="p-4 bg-slate-50 border border-slate-200 rounded-xl text-xs space-y-3">
                      <h4 className="font-bold text-slate-900">映射格式与示例:</h4>
                      <ul className="list-disc pl-5 space-y-1.5 text-slate-700">
                        <li>
                          <strong>精确别名重写:</strong> <code>yy/xxx/xx:xxx/xx</code> <br />
                          客户端请求 <code>yy/xxx/xx</code>，发往上游时自动零拷贝重写为 <code>xxx/xx</code>。
                        </li>
                        <li>
                          <strong>前缀通配映射:</strong> <code>org/dept/*:*</code> <br />
                          客户端请求 <code>org/dept/v1/deepseek-ai/DeepSeek-V3</code>，自动剥离前缀发往目标集群。
                        </li>
                        <li>
                          <strong>服务商自动前缀:</strong> <code>&lt;ProviderName&gt;/&lt;Model&gt;</code> <br />
                          当存在多个提供商均提供 <code>gpt-4o</code> 时，客户端可直接指定 <code>openai-us/gpt-4o</code> 精准定向路由！
                        </li>
                      </ul>
                    </div>
                  </div>
                )}

                {docsSection === 'rerank' && (
                  <div className="space-y-4">
                    <div className="border-b border-slate-100 pb-3">
                      <h3 className="text-base font-bold text-slate-900">Rerank 检索重排 API 规范 (/v1/rerank)</h3>
                      <p className="text-xs text-slate-500 mt-0.5">全面兼容 Cohere、Hugging Face TEI、GPUStack、Xinference 与 Infinity 重排标准协议。</p>
                    </div>

                    <div className="space-y-3 text-xs">
                      <div className="p-4 bg-slate-50 border border-slate-200 rounded-xl space-y-2">
                        <span className="font-bold text-slate-900">1. 重排请求格式 (POST /v1/rerank):</span>
                        <pre className="p-3 bg-slate-900 text-slate-100 rounded-xl font-mono overflow-x-auto text-[11px] leading-relaxed">
{`curl -X POST http://localhost:8080/v1/rerank \\
  -H "Authorization: Bearer sk-nano-xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "bge-reranker-large",
    "query": "什么是企业级大模型网关的高可用与容灾设计？",
    "documents": [
      "Nano-Gateway 采用全双工流式转发，首字分块前支持透明故障转移与熔断兜底。",
      "今天天气非常晴朗，公园里的樱花盛开了，很适合去散步或野餐。",
      "基于 Raft 协议的分布式数据库能保证网络分区状态下的强一致性与多副本高可用。"
    ],
    "top_n": 2,
    "return_documents": true
  }'`}
                        </pre>
                      </div>

                      <div className="p-4 bg-slate-50 border border-slate-200 rounded-xl space-y-2">
                        <span className="font-bold text-slate-900">2. 重排结果响应格式 (JSON):</span>
                        <pre className="p-3 bg-slate-900 text-emerald-400 rounded-xl font-mono overflow-x-auto text-[11px] leading-relaxed">
{`{
  "id": "rerank-a8c1f9b2",
  "results": [
    {
      "index": 0,
      "relevance_score": 0.9856,
      "document": {
        "text": "Nano-Gateway 采用全双工流式转发，首字分块前支持透明故障转移与熔断兜底。"
      }
    },
    {
      "index": 2,
      "relevance_score": 0.3210,
      "document": {
        "text": "基于 Raft 协议的分布式数据库能保证网络分区状态下的强一致性与多副本高可用。"
      }
    }
  ],
  "usage": {
    "prompt_tokens": 128,
    "total_tokens": 128
  }
}`}
                        </pre>
                      </div>

                      <div className="p-4 bg-amber-50 border border-amber-200 rounded-xl text-xs text-amber-900 space-y-1">
                        <span className="font-bold">🎯 高可用熔断与透明兜底保障:</span>
                        <p className="leading-relaxed">
                          当首选重排提供商（如私有部署的 GPUStack 实例）发生 OOM、503 或网络异常时，Nano-Gateway 会在毫秒级内自动安全切换至备选重排提供商，为企业级 RAG 知识库检索流水线提供全天候 99.99% 的 SLA 稳定可用保障。
                        </p>
                      </div>
                    </div>
                  </div>
                )}

                {docsSection === 'deploy' && (
                  <div className="space-y-4">
                    <div className="border-b border-slate-100 pb-3">
                      <h3 className="text-base font-bold text-slate-900">生产环境高可用集群部署 (HA)</h3>
                      <p className="text-xs text-slate-500 mt-0.5">提供开箱即用的多副本 Docker Compose 与生产级 Kubernetes Helm Chart。</p>
                    </div>

                    <div className="space-y-3">
                      <h4 className="text-xs font-bold text-slate-800">1. Docker Compose (2 副本 Gateway + Nginx 负载均衡):</h4>
                      <pre className="p-3 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto">
docker compose up -d --build
                      </pre>

                      <h4 className="text-xs font-bold text-slate-800 pt-2">2. Kubernetes Helm 一键部署:</h4>
                      <pre className="p-3 bg-slate-900 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto">
helm install nano-gateway ./helm/nano-gateway -n gateway --create-namespace
                      </pre>
                    </div>
                  </div>
                )}
              </div>
            </div>
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
                  {editingChannelId ? '编辑服务商' : '接入服务商'}
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
                      <span>快捷预设服务商 (点击一键自动填入):</span>
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {[
                      { key: 'deepseek', label: 'DeepSeek', badge: '官方' },
                      { key: 'gpustack', label: 'GPUStack', badge: '私有集群' },
                      { key: 'zhipu', label: '智谱 GLM', badge: '国产主流' },
                      { key: 'doubao', label: '火山豆包', badge: '多模态/视频' },
                      { key: 'moonshot', label: '月之暗面 Kimi', badge: '长文本' },
                      { key: 'openai', label: 'OpenAI', badge: '全协议' },
                      { key: 'anthropic', label: 'Claude', badge: 'Anthropic' },
                      { key: 'gemini', label: 'Google Gemini', badge: '多模态' },
                      { key: 'ollama', label: 'Ollama', badge: '本地开源' },
                      { key: 'vllm', label: 'vLLM', badge: '自建集群' },
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
                  服务商名称 <span className="text-rose-500">*</span>
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
                  服务商 Base URL <span className="text-rose-500">*</span>
                </label>
                <div className="flex space-x-2">
                  <input
                    required
                    value={newChannel.base_url}
                    onChange={(e) => setNewChannel({ ...newChannel, base_url: e.target.value })}
                    placeholder="https://api.deepseek.com"
                    className="flex-1 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
                  />
                  <button
                    type="button"
                    onClick={handleProbeChannel}
                    disabled={probing}
                    className="px-3.5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold flex items-center space-x-1.5 shadow-2xs transition disabled:opacity-50 shrink-0 cursor-pointer"
                  >
                    {probing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
                    <span>{probing ? '连通中...' : '测试连通性'}</span>
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
                    placeholder="留空或填 none 表示免密"
                    className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl pl-3 pr-10 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
                  />
                  <button
                    type="button"
                    onClick={() => setShowApiKeyPlain(!showApiKeyPlain)}
                    className="absolute right-3 top-2.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
                    title={showApiKeyPlain ? '隐藏密钥' : '显示明文'}
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
                  <span>高级设置</span>
                  <span className="text-slate-400 group-open:rotate-180 transition-transform text-xs">▼</span>
                </summary>

                <div className="pt-3 space-y-3.5 border-t border-slate-200/60 dark:border-slate-800 mt-2.5">
                  <div>
                    <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1">服务引擎类型</label>
                    <select
                      value={newChannel.type}
                      onChange={(e) => setNewChannel({ ...newChannel, type: e.target.value })}
                      className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                    >
                      <option value="gpustack">GPUStack</option>
                      <option value="deepseek">DeepSeek</option>
                      <option value="openai">OpenAI (及所有兼容服务商)</option>
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
                      <label className="block text-[11px] text-slate-500 mb-1">优先级 (1最先)</label>
                      <input
                        type="number"
                        value={newChannel.priority}
                        onChange={(e) => setNewChannel({ ...newChannel, priority: parseInt(e.target.value) || 1 })}
                        className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1.5 font-mono text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                    <div>
                      <label className="block text-[11px] text-slate-500 mb-1">负载权重</label>
                      <input
                        type="number"
                        value={newChannel.weight}
                        onChange={(e) => setNewChannel({ ...newChannel, weight: parseInt(e.target.value) || 10 })}
                        className="w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1.5 font-mono text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                    <div>
                      <label className="block text-[11px] text-slate-500 mb-1">超时时间 (秒)</label>
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
                className="px-4 py-2 text-xs text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 font-medium transition"
              >
                取消
              </button>
              <button
                type="button"
                onClick={handleCreateChannel}
                className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm transition cursor-pointer"
              >
                {editingChannelId ? '保存配置' : '确认接入'}
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
                <h3 className="font-bold text-base text-slate-900 dark:text-slate-100 tracking-tight">新建 API 密钥</h3>
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
                  密钥名称 <span className="text-rose-500">*</span>
                </label>
                <input
                  value={newKey.tenant_id}
                  onChange={(e) => setNewKey({ ...newKey, tenant_id: e.target.value })}
                  placeholder="如 个人开发 / Cursor / Claude Code"
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs"
                />
              </div>

              <div>
                <label className="block font-semibold text-slate-700 dark:text-slate-300 mb-1">
                  计费分组 (Pricing Group)
                </label>
                {adminUser?.role === 'admin' ? (
                  <div className="flex items-center space-x-1.5">
                    {[
                      { id: 'default', label: '默认组 (default)' },
                      { id: 'vip', label: 'VIP组 (vip)' },
                      { id: 'enterprise', label: '企业组 (enterprise)' }
                    ].map(g => (
                      <button
                        key={g.id}
                        type="button"
                        onClick={() => setNewKey({ ...newKey, group_name: g.id })}
                        className={`px-3 py-1.5 rounded-xl text-xs font-medium border transition cursor-pointer ${
                          (newKey.group_name || 'default') === g.id
                            ? 'bg-indigo-600 text-white border-indigo-600 font-semibold shadow-xs'
                            : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-50'
                        }`}
                      >
                        {g.label}
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="p-2.5 rounded-2xl bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 flex items-center justify-between">
                    <div className="flex items-center space-x-2">
                      <span className="text-slate-600 dark:text-slate-300 font-medium">绑定您的账号等级:</span>
                      <span className="px-2 py-0.5 rounded-lg text-xs font-semibold bg-indigo-50 text-indigo-700 border border-indigo-200">
                        {adminUser?.group_name ? `${adminUser.group_name} 组` : '默认组'}
                      </span>
                    </div>
                  </div>
                )}
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="font-semibold text-slate-700 dark:text-slate-300">API 密钥</label>
                  <button
                    type="button"
                    onClick={() => {
                      const rand = 'sk-nano-' + Math.random().toString(36).substring(2, 10) + Math.random().toString(36).substring(2, 6);
                      setNewKey(prev => ({ ...prev, key: rand }));
                    }}
                    className="text-[11px] text-indigo-600 dark:text-indigo-400 hover:underline flex items-center space-x-1 cursor-pointer"
                  >
                    <span>随机生成</span>
                  </button>
                </div>
                <input
                  value={newKey.key}
                  onChange={(e) => setNewKey({ ...newKey, key: e.target.value })}
                  placeholder="留空自动生成"
                  className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 font-mono text-xs"
                />
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="font-semibold text-slate-700 dark:text-slate-300">额度限制 (CNY, 0 为不限)</label>
                  <div className="flex items-center space-x-1.5 text-[11px]">
                    {[
                      { label: '不限', val: 0 },
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
                  <label className="font-semibold text-slate-700 dark:text-slate-300">速率限制 (RPM, 0 为不限)</label>
                  <div className="flex items-center space-x-1.5 text-[11px]">
                    {[
                      { label: '不限', val: 0 },
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
                取消
              </button>
              <button
                type="submit"
                className="px-5 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm transition cursor-pointer"
              >
                确认创建
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
        onPasswordChanged={() => {
          showToast('密码修改成功，请牢记新密码', 'success');
        }}
      />

      {/* Login Modal */}
      <LoginModal
        isOpen={showLoginModal}
        onClose={() => setShowLoginModal(false)}
        onLoginSuccess={(token, user) => {
          setAdminToken(token);
          setAdminUser(user);
          setShowLoginModal(false);
          showToast(`欢迎回来，${user.username}！`, 'success');
          fetchData();
          fetchLogs();
        }}
      />
    </div>
  );
}
