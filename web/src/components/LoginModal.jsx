import React, { useState } from 'react';
import {
  X,
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
  ShieldCheck
} from 'lucide-react';

export default function LoginModal({ isOpen, onClose, onLoginSuccess }) {
  if (!isOpen) return null;

  const [activeTab, setActiveTab] = useState('login'); // 'login' or 'register'

  // Login state
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('admin123');
  const [showPassword, setShowPassword] = useState(false);

  // Register state
  const [regUsername, setRegUsername] = useState('');
  const [regEmail, setRegEmail] = useState('');
  const [regPassword, setRegPassword] = useState('');
  const [regCode, setRegCode] = useState('');
  const [sendingCode, setSendingCode] = useState(false);
  const [codeCountdown, setCodeCountdown] = useState(0);

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [successMsg, setSuccessMsg] = useState('');

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
        body: JSON.stringify({ username, password })
      });
      const data = await res.json();

      if (res.ok && data.code === 0 && data.data?.token) {
        localStorage.setItem('nano_admin_token', data.data.token);
        localStorage.setItem('nano_admin_user', JSON.stringify(data.data.user));
        onLoginSuccess(data.data.token, data.data.user);
        onClose();
      } else {
        setError(data.error || '登录失败，请检查账号或密码');
      }
    } catch (err) {
      setError('网络请求失败，请确保网关服务正在运行: ' + err.message);
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
          // Auto fill dev code for convenient local testing
          setRegCode(data.dev_code);
          setSuccessMsg(`验证码已发送！(测试环境已自动为您填充: ${data.dev_code})`);
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
        onClose();
      } else {
        setError(data.error || '注册失败');
      }
    } catch (err) {
      setError('网络请求失败: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  // 1-Click OAuth Login (GitHub / Google)
  const handleOAuthLogin = async (provider) => {
    setError('');
    setLoading(true);
    try {
      const res = await fetch(`/api/v1/auth/oauth/${provider}/callback`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ simulated: true })
      });
      const data = await res.json();
      if (res.ok && data.code === 0 && data.data?.token) {
        localStorage.setItem('nano_admin_token', data.data.token);
        localStorage.setItem('nano_admin_user', JSON.stringify(data.data.user));
        onLoginSuccess(data.data.token, data.data.user);
        onClose();
      } else {
        setError(data.error || `${provider} 授权登录失败`);
      }
    } catch (err) {
      setError('OAuth 登录异常: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  const fillDefaultCredentials = () => {
    setUsername('admin');
    setPassword('admin123');
    setError('');
  };

  return (
    <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in">
      <div className="bg-white border border-slate-200 rounded-3xl p-6 sm:p-8 max-w-md w-full shadow-2xl space-y-5 animate-in zoom-in-95 duration-150">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-100 pb-3">
          <div className="flex items-center space-x-2.5">
            <div className="w-10 h-10 rounded-2xl bg-indigo-50 border border-indigo-200 flex items-center justify-center text-indigo-600 font-bold text-lg">
              ✨
            </div>
            <div>
              <h3 className="font-bold text-base text-slate-900 tracking-tight">
                Nano Gateway 统一门户
              </h3>
              <p className="text-[11px] text-slate-500">统一身份验证与额度管理</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-slate-100 rounded-xl text-slate-400 hover:text-slate-600 transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Tab Switcher */}
        <div className="grid grid-cols-2 gap-2 bg-slate-100 p-1 rounded-2xl text-xs font-bold">
          <button
            type="button"
            onClick={() => {
              setActiveTab('login');
              setError('');
              setSuccessMsg('');
            }}
            className={`py-2 rounded-xl transition cursor-pointer ${
              activeTab === 'login'
                ? 'bg-white text-indigo-600 shadow-xs'
                : 'text-slate-500 hover:text-slate-800'
            }`}
          >
            账号登录
          </button>
          <button
            type="button"
            onClick={() => {
              setActiveTab('register');
              setError('');
              setSuccessMsg('');
            }}
            className={`py-2 rounded-xl transition cursor-pointer flex items-center justify-center space-x-1 ${
              activeTab === 'register'
                ? 'bg-white text-indigo-600 shadow-xs'
                : 'text-slate-500 hover:text-slate-800'
            }`}
          >
            <span>注册新用户</span>
            <span className="text-[10px] px-1.5 py-0.2 bg-indigo-50 text-indigo-600 rounded-full font-normal">送¥5</span>
          </button>
        </div>

        {error && (
          <div className="p-3 rounded-2xl bg-rose-50 border border-rose-200 text-rose-800 text-xs flex items-center space-x-2">
            <AlertCircle className="w-4 h-4 shrink-0 text-rose-600" />
            <span>{error}</span>
          </div>
        )}

        {successMsg && (
          <div className="p-3 rounded-2xl bg-emerald-50 border border-emerald-200 text-emerald-800 text-xs flex items-center space-x-2">
            <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-600" />
            <span>{successMsg}</span>
          </div>
        )}

        {/* TAB: LOGIN */}
        {activeTab === 'login' ? (
          <form onSubmit={handleLoginSubmit} className="space-y-4">
            <div>
              <label className="block text-xs font-semibold text-slate-700 mb-1">
                用户名或绑定邮箱
              </label>
              <div className="relative">
                <User className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                <input
                  required
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="admin / 注册用户名 / 邮箱"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl pl-9 pr-3 py-2.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white"
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-700 mb-1">
                登录密码
              </label>
              <div className="relative">
                <Lock className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                <input
                  required
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="密码"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl pl-9 pr-10 py-2.5 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-3 text-slate-400 hover:text-slate-600 transition cursor-pointer"
                  title={showPassword ? '隐藏密码' : '显示密码'}
                >
                  {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>

            <div className="flex items-center justify-between pt-1">
              <button
                type="button"
                onClick={fillDefaultCredentials}
                className="text-[11px] text-indigo-600 hover:underline font-medium flex items-center space-x-1 cursor-pointer"
              >
                <span>填入默认管理员 (admin / admin123)</span>
              </button>
            </div>

            <div className="pt-2">
              <button
                type="submit"
                disabled={loading}
                className="w-full py-2.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2 disabled:opacity-50 cursor-pointer"
              >
                {loading ? (
                  <>
                    <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    <span>正在登录...</span>
                  </>
                ) : (
                  <>
                    <span>登录平台</span>
                    <ArrowRight className="w-3.5 h-3.5" />
                  </>
                )}
              </button>
            </div>

            {/* OAuth Separator */}
            <div className="pt-2">
              <div className="relative flex py-2 items-center">
                <div className="flex-grow border-t border-slate-200"></div>
                <span className="flex-shrink mx-3 text-[10px] text-slate-400 uppercase font-semibold">快速三方登录</span>
                <div className="flex-grow border-t border-slate-200"></div>
              </div>

              <div className="grid grid-cols-2 gap-2.5 mt-2">
                <button
                  type="button"
                  onClick={() => handleOAuthLogin('github')}
                  disabled={loading}
                  className="py-2 px-3 border border-slate-200 hover:bg-slate-50 text-slate-700 rounded-xl text-xs font-semibold flex items-center justify-center space-x-2 transition cursor-pointer"
                >
                  <svg className="w-4 h-4 fill-current" viewBox="0 0 24 24">
                    <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
                  </svg>
                  <span>GitHub</span>
                </button>
                <button
                  type="button"
                  onClick={() => handleOAuthLogin('google')}
                  disabled={loading}
                  className="py-2 px-3 border border-slate-200 hover:bg-slate-50 text-slate-700 rounded-xl text-xs font-semibold flex items-center justify-center space-x-2 transition cursor-pointer"
                >
                  <svg className="w-4 h-4" viewBox="0 0 24 24">
                    <path fill="#4285F4" d="M23.745 12.27c0-.7-.06-1.4-.19-2.07H12v4.51h6.6c-.29 1.52-1.14 2.82-2.4 3.68v3.05h3.88c2.27-2.09 3.665-5.17 3.665-9.17z" />
                    <path fill="#34A853" d="M12 24c3.24 0 5.95-1.08 7.93-2.91l-3.88-3.05c-1.08.72-2.45 1.16-4.05 1.16-3.12 0-5.77-2.1-6.72-4.93H1.25v3.15C3.26 21.36 7.33 24 12 24z" />
                    <path fill="#FBBC05" d="M5.28 14.27c-.25-.72-.38-1.49-.38-2.27s.13-1.55.38-2.27V6.58H1.25C.45 8.18 0 9.99 0 12s.45 3.82 1.25 5.42l4.03-3.15z" />
                    <path fill="#EA4335" d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.33 0 3.26 2.64 1.25 6.58l4.03 3.15c.95-2.83 3.6-4.98 6.72-4.98z" />
                  </svg>
                  <span>Google</span>
                </button>
              </div>
            </div>
          </form>
        ) : (
          /* TAB: REGISTER */
          <form onSubmit={handleRegisterSubmit} className="space-y-3.5">
            <div>
              <label className="block text-xs font-semibold text-slate-700 mb-1">
                用户名
              </label>
              <div className="relative">
                <User className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                <input
                  required
                  type="text"
                  value={regUsername}
                  onChange={(e) => setRegUsername(e.target.value)}
                  placeholder="字母/数字，至少 3 字符"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl pl-9 pr-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white"
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-700 mb-1">
                邮箱地址
              </label>
              <div className="relative">
                <Mail className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                <input
                  required
                  type="email"
                  value={regEmail}
                  onChange={(e) => setRegEmail(e.target.value)}
                  placeholder="your-name@company.com"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl pl-9 pr-3 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white"
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-700 mb-1">
                邮箱验证码
              </label>
              <div className="flex space-x-2">
                <input
                  required
                  type="text"
                  maxLength={6}
                  value={regCode}
                  onChange={(e) => setRegCode(e.target.value)}
                  placeholder="6 位验证码"
                  className="flex-1 bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 text-xs font-mono text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white"
                />
                <button
                  type="button"
                  onClick={handleSendCode}
                  disabled={sendingCode || codeCountdown > 0}
                  className="px-3 py-2 rounded-xl bg-slate-100 hover:bg-slate-200 text-indigo-700 text-xs font-bold transition whitespace-nowrap disabled:opacity-50 cursor-pointer"
                >
                  {sendingCode ? (
                    <RefreshCw className="w-3.5 h-3.5 animate-spin mx-auto" />
                  ) : codeCountdown > 0 ? (
                    `${codeCountdown}s 后重试`
                  ) : (
                    '获取验证码'
                  )}
                </button>
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-700 mb-1">
                设置密码
              </label>
              <div className="relative">
                <Lock className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                <input
                  required
                  type={showPassword ? 'text' : 'password'}
                  value={regPassword}
                  onChange={(e) => setRegPassword(e.target.value)}
                  placeholder="至少 6 位字符"
                  className="w-full bg-slate-50 border border-slate-200 rounded-xl pl-9 pr-10 py-2 text-xs text-slate-900 focus:outline-none focus:border-indigo-500 focus:bg-white"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-2.5 text-slate-400 hover:text-slate-600 transition cursor-pointer"
                >
                  {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>

            <div className="p-2.5 bg-indigo-50/70 border border-indigo-200/80 rounded-xl text-[11px] text-indigo-700 flex items-center space-x-2">
              <Sparkles className="w-4 h-4 shrink-0 text-indigo-600" />
              <span>注册即送 ¥5.00 新手体验额度并自动生成网关专属 API Key！</span>
            </div>

            <div className="pt-2">
              <button
                type="submit"
                disabled={loading}
                className="w-full py-2.5 bg-gradient-to-r from-indigo-600 to-sky-600 hover:from-indigo-700 hover:to-sky-700 text-white rounded-xl text-xs font-bold shadow-md hover:shadow-lg transition flex items-center justify-center space-x-2 disabled:opacity-50 cursor-pointer"
              >
                {loading ? (
                  <>
                    <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    <span>正在创建账号...</span>
                  </>
                ) : (
                  <>
                    <span>立即注册并登录</span>
                    <ArrowRight className="w-3.5 h-3.5" />
                  </>
                )}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
