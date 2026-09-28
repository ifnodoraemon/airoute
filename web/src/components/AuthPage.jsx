import React, { useState, useEffect } from 'react';
import {
  Lock,
  User,
  Mail,
  AlertCircle,
  ArrowRight,
  RefreshCw,
  Eye,
  EyeOff,
  CheckCircle2,
  Sparkles,
  Zap,
  ArrowLeft,
  ShieldCheck
} from 'lucide-react';

export default function AuthPage({ initialTab = 'login', onLoginSuccess, onBackHome }) {
  const [activeTab, setActiveTab] = useState(initialTab); // 'login' | 'register'

  // Login form state
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  // Register form state
  const [regUsername, setRegUsername] = useState('');
  const [regEmail, setRegEmail] = useState('');
  const [regPassword, setRegPassword] = useState('');
  const [regCode, setRegCode] = useState('');
  const [sendingCode, setSendingCode] = useState(false);
  const [codeCountdown, setCodeCountdown] = useState(0);

  // Feedback state
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [successMsg, setSuccessMsg] = useState('');

  useEffect(() => {
    setError('');
    setSuccessMsg('');
  }, [activeTab]);

  // Handle Login Submit
  const handleLoginSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setSuccessMsg('');
    setLoading(true);

    try {
      const res = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: username.trim(), password })
      });
      const data = await res.json();

      if (res.ok && data.code === 0 && data.data?.token) {
        localStorage.setItem('nano_admin_token', data.data.token);
        localStorage.setItem('nano_admin_user', JSON.stringify(data.data.user));
        onLoginSuccess(data.data.token, data.data.user);
      } else {
        setError(data.error || '登录失败，请核对用户名/邮箱与密码');
      }
    } catch (err) {
      setError('网络请求失败，请确保服务正常运行: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  // Send Email Verification Code
  const handleSendCode = async () => {
    if (!regEmail.trim()) {
      setError('请先填写有效的电子邮箱');
      return;
    }
    setError('');
    setSuccessMsg('');
    setSendingCode(true);

    try {
      const res = await fetch('/api/v1/auth/send-verification-code', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: regEmail.trim(), purpose: 'register' })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setSuccessMsg(data.message || '验证码已发送至邮箱');
        if (data.dev_code) {
          setRegCode(data.dev_code);
          setSuccessMsg(`验证码已发送！(开发沙箱已自动填充: ${data.dev_code})`);
        }
        setCodeCountdown(60);
        const timer = setInterval(() => {
          setCodeCountdown((prev) => {
            if (prev <= 1) {
              clearInterval(timer);
              return 0;
            }
            return prev - 1;
          });
        }, 1000);
      } else {
        setError(data.error || '发送验证码失败');
      }
    } catch (err) {
      setError('请求异常: ' + err.message);
    } finally {
      setSendingCode(false);
    }
  };

  // Handle Register Submit
  const handleRegisterSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setSuccessMsg('');
    setLoading(true);

    try {
      const res = await fetch('/api/v1/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: regUsername.trim(),
          email: regEmail.trim(),
          password: regPassword,
          code: regCode.trim()
        })
      });
      const data = await res.json();

      if (res.ok && data.code === 0 && data.data?.token) {
        localStorage.setItem('nano_admin_token', data.data.token);
        localStorage.setItem('nano_admin_user', JSON.stringify(data.data.user));
        onLoginSuccess(data.data.token, data.data.user);
      } else {
        setError(data.error || '注册失败，请检查填写内容');
      }
    } catch (err) {
      setError('网络请求失败: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  // Quick fill demo admin
  const handleQuickFillAdmin = () => {
    setUsername('admin');
    setPassword('admin123');
    setError('');
  };

  // 1-Click OAuth Login (GitHub / Google)
  const handleOAuthLogin = async (provider) => {
    setError('');
    try {
      const res = await fetch(`/api/v1/auth/oauth/${provider}`);
      const data = await res.json();
      if (res.ok && data.auth_url) {
        if (data.mode === 'sandbox_simulation') {
          const simRes = await fetch(`/api/v1/auth/oauth/${provider}/callback`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ code: 'simulated_oauth_code' })
          });
          const simData = await simRes.json();
          if (simRes.ok && simData.code === 0 && simData.data?.token) {
            localStorage.setItem('nano_admin_token', simData.data.token);
            localStorage.setItem('nano_admin_user', JSON.stringify(simData.data.user));
            onLoginSuccess(simData.data.token, simData.data.user);
            return;
          }
        }
        window.location.href = data.auth_url;
      } else {
        setError(data.error || `${provider} 登录初始化失败`);
      }
    } catch (err) {
      setError('OAuth 异常: ' + err.message);
    }
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-50 via-indigo-50/30 to-slate-100 flex flex-col justify-between py-10 px-4 sm:px-6 lg:px-8 selection:bg-indigo-500 selection:text-white">
      {/* Top Bar with Brand & Back */}
      <div className="max-w-md w-full mx-auto flex items-center justify-between">
        <button
          onClick={onBackHome}
          className="inline-flex items-center space-x-1.5 text-xs font-semibold text-slate-500 hover:text-slate-900 transition py-2 px-3 rounded-xl hover:bg-white/80 cursor-pointer"
        >
          <ArrowLeft className="w-4 h-4" />
          <span>返回产品门户</span>
        </button>

        <div className="flex items-center space-x-2">
          <div className="w-7 h-7 rounded-xl bg-indigo-600 flex items-center justify-center text-white shadow-sm shadow-indigo-500/30">
            <Zap className="w-4 h-4 fill-white" />
          </div>
          <span className="font-bold text-sm text-slate-900 tracking-tight">AI 路由器</span>
        </div>
      </div>

      {/* Main Card Container */}
      <div className="max-w-md w-full mx-auto my-auto">
        <div className="bg-white/95 backdrop-blur-xl border border-slate-200/90 rounded-3xl p-8 shadow-xl shadow-slate-200/50 space-y-6">
          {/* Header */}
          <div className="text-center space-y-1.5">
            <h2 className="text-2xl font-black text-slate-900 tracking-tight">
              {activeTab === 'login' ? '欢迎回来' : '开启极简 AI 之旅'}
            </h2>
            <p className="text-xs text-slate-500">
              {activeTab === 'login'
                ? '登录账号，管理专属 API 密钥与模型路由'
                : '新用户注册即可获得免费体验额度及专属 API 密钥'}
            </p>
          </div>

          {/* Tab Switcher */}
          <div className="flex p-1 bg-slate-100/90 rounded-2xl">
            <button
              type="button"
              onClick={() => setActiveTab('login')}
              className={`flex-1 py-2 text-xs font-bold rounded-xl transition cursor-pointer ${
                activeTab === 'login'
                  ? 'bg-white text-indigo-700 shadow-xs'
                  : 'text-slate-500 hover:text-slate-800'
              }`}
            >
              账号登录
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('register')}
              className={`flex-1 py-2 text-xs font-bold rounded-xl transition cursor-pointer ${
                activeTab === 'register'
                  ? 'bg-white text-indigo-700 shadow-xs'
                  : 'text-slate-500 hover:text-slate-800'
              }`}
            >
              新用户注册
            </button>
          </div>

          {/* Feedback Messages */}
          {error && (
            <div className="p-3.5 rounded-2xl bg-rose-50 border border-rose-200 text-rose-700 text-xs flex items-center space-x-2 animate-in fade-in">
              <AlertCircle className="w-4 h-4 shrink-0 text-rose-600" />
              <span>{error}</span>
            </div>
          )}
          {successMsg && (
            <div className="p-3.5 rounded-2xl bg-emerald-50 border border-emerald-200 text-emerald-700 text-xs flex items-center space-x-2 animate-in fade-in">
              <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-600" />
              <span>{successMsg}</span>
            </div>
          )}

          {/* LOGIN FORM */}
          {activeTab === 'login' && (
            <form onSubmit={handleLoginSubmit} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1.5">
                  用户名 / 绑定邮箱
                </label>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <User className="w-4 h-4" />
                  </div>
                  <input
                    type="text"
                    required
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    placeholder="输入用户名或登录邮箱"
                    className="w-full bg-slate-50 border border-slate-200 rounded-2xl pl-10 pr-4 py-2.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white transition"
                  />
                </div>
              </div>

              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <label className="text-xs font-semibold text-slate-700">登录密码</label>
                  <button
                    type="button"
                    onClick={handleQuickFillAdmin}
                    className="text-[11px] text-indigo-600 hover:underline flex items-center space-x-1"
                  >
                    <Sparkles className="w-3 h-3" />
                    <span>填入体验管理员</span>
                  </button>
                </div>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <Lock className="w-4 h-4" />
                  </div>
                  <input
                    type={showPassword ? 'text' : 'password'}
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="输入账户密码"
                    className="w-full bg-slate-50 border border-slate-200 rounded-2xl pl-10 pr-10 py-2.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white transition font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3.5 top-3 text-slate-400 hover:text-slate-600 cursor-pointer"
                  >
                    {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-xs font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2 disabled:opacity-50 cursor-pointer"
              >
                {loading ? (
                  <RefreshCw className="w-4 h-4 animate-spin" />
                ) : (
                  <>
                    <span>登录工作台</span>
                    <ArrowRight className="w-4 h-4" />
                  </>
                )}
              </button>
            </form>
          )}

          {/* REGISTER FORM */}
          {activeTab === 'register' && (
            <form onSubmit={handleRegisterSubmit} className="space-y-3.5">
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">
                  登录用户名 *
                </label>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <User className="w-4 h-4" />
                  </div>
                  <input
                    type="text"
                    required
                    value={regUsername}
                    onChange={(e) => setRegUsername(e.target.value)}
                    placeholder="至少 3 个字符 (字母、数字)"
                    className="w-full bg-slate-50 border border-slate-200 rounded-2xl pl-10 pr-4 py-2.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white transition"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">
                  电子邮箱 *
                </label>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <Mail className="w-4 h-4" />
                  </div>
                  <input
                    type="email"
                    required
                    value={regEmail}
                    onChange={(e) => setRegEmail(e.target.value)}
                    placeholder="name@example.com"
                    className="w-full bg-slate-50 border border-slate-200 rounded-2xl pl-10 pr-4 py-2.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white transition"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">
                  邮箱验证码 *
                </label>
                <div className="flex space-x-2">
                  <input
                    type="text"
                    required
                    maxLength={6}
                    value={regCode}
                    onChange={(e) => setRegCode(e.target.value)}
                    placeholder="6 位数字验证码"
                    className="flex-1 bg-slate-50 border border-slate-200 rounded-2xl px-4 py-2.5 text-xs font-mono text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white transition tracking-widest text-center"
                  />
                  <button
                    type="button"
                    disabled={sendingCode || codeCountdown > 0}
                    onClick={handleSendCode}
                    className="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 text-slate-700 border border-slate-200 rounded-2xl text-xs font-semibold transition shrink-0 disabled:opacity-50 cursor-pointer"
                  >
                    {sendingCode ? (
                      <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    ) : codeCountdown > 0 ? (
                      `${codeCountdown}s 后重发`
                    ) : (
                      '获取验证码'
                    )}
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">
                  设置密码 *
                </label>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <Lock className="w-4 h-4" />
                  </div>
                  <input
                    type={showPassword ? 'text' : 'password'}
                    required
                    value={regPassword}
                    onChange={(e) => setRegPassword(e.target.value)}
                    placeholder="至少 6 个字符"
                    className="w-full bg-slate-50 border border-slate-200 rounded-2xl pl-10 pr-10 py-2.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white transition font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3.5 top-3 text-slate-400 hover:text-slate-600 cursor-pointer"
                  >
                    {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div className="p-3 bg-indigo-50/70 rounded-2xl border border-indigo-100 text-[11px] text-indigo-700 space-y-0.5">
                <div className="font-semibold flex items-center space-x-1">
                  <Sparkles className="w-3.5 h-3.5" />
                  <span>注册专享权益</span>
                </div>
                <p className="text-slate-600">完成注册立即到账 ¥5.00 新手体验金，自动生成专属调用 API 密钥。</p>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 text-white rounded-2xl text-xs font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2 disabled:opacity-50 cursor-pointer"
              >
                {loading ? (
                  <RefreshCw className="w-4 h-4 animate-spin" />
                ) : (
                  <>
                    <span>完成注册并进入工作台</span>
                    <ArrowRight className="w-4 h-4" />
                  </>
                )}
              </button>
            </form>
          )}

          {/* OAuth 1-Click Fast Login */}
          <div className="pt-2">
            <div className="relative flex py-2 items-center">
              <div className="flex-grow border-t border-slate-200"></div>
              <span className="flex-shrink mx-3 text-[11px] text-slate-400 font-medium">第三方快捷接入</span>
              <div className="flex-grow border-t border-slate-200"></div>
            </div>

            <div className="grid grid-cols-2 gap-3 mt-3">
              <button
                type="button"
                onClick={() => handleOAuthLogin('github')}
                className="flex items-center justify-center space-x-2 py-2.5 px-3 border border-slate-200 hover:bg-slate-50 text-slate-700 rounded-2xl text-xs font-semibold transition cursor-pointer"
              >
                <svg className="w-4 h-4 text-slate-800" fill="currentColor" viewBox="0 0 24 24">
                  <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
                </svg>
                <span>GitHub 登录</span>
              </button>

              <button
                type="button"
                onClick={() => handleOAuthLogin('google')}
                className="flex items-center justify-center space-x-2 py-2.5 px-3 border border-slate-200 hover:bg-slate-50 text-slate-700 rounded-2xl text-xs font-semibold transition cursor-pointer"
              >
                <svg className="w-4 h-4" viewBox="0 0 24 24">
                  <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
                  <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
                  <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z" />
                  <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z" />
                </svg>
                <span>Google 登录</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Footer copyright */}
      <div className="max-w-md w-full mx-auto text-center text-xs text-slate-400">
        © 2026 Nano Gateway · 企业级大模型与多模态极简路由网关
      </div>
    </div>
  );
}
