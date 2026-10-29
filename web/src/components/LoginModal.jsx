import React, { useState } from 'react';
import {
  X,
  Lock,
  User,
  Key,
  Mail,
  AlertCircle,
  ArrowRight,
  RefreshCw,
  Eye,
  EyeOff,
  CheckCircle2
} from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';

import OneTimeKeyDialog from './OneTimeKeyDialog';

export default function LoginModal({ isOpen, onClose, onLoginSuccess, lang = 'zh' }) {

  const isZh = lang === 'zh';
  const [activeTab, setActiveTab] = useState('login'); // 'login' or 'register'

  // Login state
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
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
  const [registrationResult, setRegistrationResult] = useState(null);

  const requestClose = () => { if (!loading && !registrationResult) onClose(); };
  useModalA11y(isOpen, requestClose);

  // Enterprise public capability configuration
  const [sysConfig, setSysConfig] = useState({
    allow_registration: true,
    require_email_verification: false,
    oauth_github_enabled: false,
    oauth_google_enabled: false
  });

  React.useEffect(() => {
    fetch('/api/v1/public/status')
      .then(res => res.json())
      .then(data => {
        if (data.code === 0 && data.data) {
          setSysConfig({
            allow_registration: data.data.allow_registration !== false,
            require_email_verification: !!data.data.require_email_verification,
            oauth_github_enabled: !!data.data.oauth_github_enabled,
            oauth_google_enabled: !!data.data.oauth_google_enabled
          });
        }
      })
      .catch(() => {});
  }, []);

  // The resend countdown owns its interval so the timer stops with the dialog. This
  // component stays mounted while closed (`if (!isOpen) return null`), so the guard
  // is `isOpen` as well as the remaining time — otherwise a closed dialog would keep
  // ticking for the rest of the minute.
  const isCountingDown = isOpen && codeCountdown > 0;
  React.useEffect(() => {
    if (!isCountingDown) return undefined;
    const timer = setInterval(() => {
      setCodeCountdown((prev) => (prev <= 1 ? 0 : prev - 1));
    }, 1000);
    return () => clearInterval(timer);
  }, [isCountingDown]);

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
        localStorage.setItem('airoute_user', JSON.stringify(data.data.user));
        onLoginSuccess(data.data.token, data.data.user);
        onClose();
      } else {
        setError(data.error || (isZh ? '登录失败，请检查账号或密码' : 'Sign in failed, please check credentials'));
      }
    } catch (err) {
      setError(isZh ? ('网络请求失败，请确保网关服务正在运行: ' + err.message) : ('Network request failed: ' + err.message));
    } finally {
      setLoading(false);
    }
  };

  // Send Email Verification Code
  const handleSendCode = async () => {
    if (!regEmail.trim()) {
      setError(isZh ? '请先填写有效的电子邮箱' : 'Please enter a valid email address first');
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
        setSuccessMsg(data.message || (isZh ? '验证码已发送至邮箱，请查收' : 'Verification code sent to your email'));
        setCodeCountdown(60);
      } else {
        setError(data.error || (isZh ? '发送验证码失败' : 'Failed to send code'));
      }
    } catch (err) {
      setError((isZh ? '请求异常: ' : 'Request error: ') + err.message);
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
          code: sysConfig.require_email_verification ? regCode.trim() : undefined
        })
      });
      const data = await res.json();

      if (res.ok && data.code === 0 && data.data?.token) {
        if (data.data.api_key) {
          setRegistrationResult(data.data);
          setRegPassword('');
        } else {
          onLoginSuccess(data.data.token, data.data.user);
          onClose();
        }
      } else {
        setError(data.error || (isZh ? '注册失败' : 'Registration failed'));
      }
    } catch (err) {
      setError((isZh ? '网络请求失败: ' : 'Network request failed: ') + err.message);
    } finally {
      setLoading(false);
    }
  };

  // 1-Click OAuth Login (GitHub / Google)
  const handleOAuthLogin = async (provider) => {
    setError('');
    try {
      // Always start the flow on the server: it mints the signed, cookie-bound
      // OAuth `state` that the callback now requires. Building the authorize
      // URL here bypassed that and the callback was rejected as CSRF.
      const res = await fetch(`/api/v1/auth/oauth/${provider}`, { credentials: 'same-origin' });
      const data = await res.json().catch(() => ({}));
      if (res.ok && data.auth_url) {
        window.location.href = data.auth_url;
        return;
      }
      setError(data.error || (isZh ? `企业未配置 ${provider.toUpperCase()} 登录通道` : `${provider.toUpperCase()} SSO login is not configured`));
    } catch (err) {
      setError((isZh ? 'OAuth 异常: ' : 'OAuth error: ') + err.message);
    }
  };

  if (!isOpen) return null;

  if (registrationResult) {
    return <OneTimeKeyDialog
      credential={registrationResult.api_key}
      username={registrationResult.user?.username}
      lang={lang}
      onClose={() => {
        const result = registrationResult;
        setRegistrationResult(null);
        onLoginSuccess(result.token, result.user);
        onClose();
      }}
    />;
  }



  return (
    <div
      className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) requestClose();
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-6 sm:p-8 max-w-md w-full shadow-2xl space-y-5 animate-in zoom-in-95 duration-150">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[#E4E6E9] dark:border-[#2C3038] pb-3">
          <div className="flex items-center space-x-2.5">
            <div className="w-9 h-9 rounded-lg bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/30 dark:border-[#91A7FF]/30 flex items-center justify-center text-[#355CFF] dark:text-[#91A7FF]">
              <Key className="w-4 h-4" />
            </div>
            <div>
              <h3 className="font-semibold text-base text-[#202124] dark:text-[#F1F2F4] tracking-tight">
                {isZh ? 'Airoute 统一门户' : 'Airoute Portal'}
              </h3>
              <p className="ui-meta">{isZh ? '统一身份验证与额度管理' : 'Unified Auth & Quota Management'}</p>
            </div>
          </div>
          <button
            onClick={requestClose}
            className="p-1.5 hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] rounded-lg text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Tab Switcher */}
        {sysConfig.allow_registration ? (
          <div className="grid grid-cols-2 gap-2 bg-[#F0F1F2] dark:bg-[#22252B] p-1 rounded-lg text-xs font-semibold border border-[#E4E6E9] dark:border-[#2C3038]">
            <button
              type="button"
              onClick={() => {
                setActiveTab('login');
                setError('');
                setSuccessMsg('');
              }}
              className={`py-2 rounded-md transition cursor-pointer ${
                activeTab === 'login'
                  ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              {isZh ? '账号登录' : 'Sign In'}
            </button>
            <button
              type="button"
              onClick={() => {
                setActiveTab('register');
                setError('');
                setSuccessMsg('');
              }}
              className={`py-2 rounded-md transition cursor-pointer flex items-center justify-center ${
                activeTab === 'register'
                  ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              <span>{isZh ? '注册账号' : 'Register'}</span>
            </button>
          </div>
        ) : null}

        {error && (
          <div className="p-3 rounded-2xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs flex items-center space-x-2">
            <AlertCircle className="w-4 h-4 shrink-0 text-rose-600 dark:text-rose-400" />
            <span>{error}</span>
          </div>
        )}

        {successMsg && (
          <div className="p-3 rounded-2xl bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 text-emerald-800 dark:text-emerald-300 text-xs flex items-center space-x-2">
            <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
            <span>{successMsg}</span>
          </div>
        )}

        {/* TAB: LOGIN */}
        {activeTab === 'login' ? (
          <form onSubmit={handleLoginSubmit} className="space-y-4">
            <div>
              <label className="ui-label font-semibold">
                {isZh ? '用户名或绑定邮箱' : 'Username or Email'}
              </label>
              <div className="relative">
                <User className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  required
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder={isZh ? 'admin / 注册用户名 / 邮箱' : 'admin / username / email'}
                  className="ui-field pl-9 pr-3"
                />
              </div>
            </div>

            <div>
              <label className="ui-label font-semibold">
                {isZh ? '登录密码' : 'Password'}
              </label>
              <div className="relative">
                <Lock className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  required
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={isZh ? '密码' : 'Password'}
                  className="ui-field pl-9 pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
                  title={showPassword ? (isZh ? '隐藏密码' : 'Hide password') : (isZh ? '显示密码' : 'Show password')}
                >
                  {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>

            <div className="pt-2">
              <button
                type="submit"
                disabled={loading}
                className="ui-btn-primary w-full gap-2 font-bold shadow-xs"
              >
                {loading ? (
                  <>
                    <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    <span>{isZh ? '正在登录...' : 'Signing in...'}</span>
                  </>
                ) : (
                  <>
                    <span>{isZh ? '登录平台' : 'Sign In'}</span>
                    <ArrowRight className="w-3.5 h-3.5" />
                  </>
                )}
              </button>
            </div>

            {/* OAuth Separator */}
            {(sysConfig.oauth_github_enabled || sysConfig.oauth_google_enabled) && (
              <div className="pt-2">
                <div className="relative flex py-2 items-center">
                  <div className="grow border-t border-slate-200 dark:border-slate-800"></div>
                  <span className="shrink mx-3 ui-section-title">
                    {isZh ? '快速三方登录' : 'Social Sign-in'}
                  </span>
                  <div className="grow border-t border-slate-200 dark:border-slate-800"></div>
                </div>

                <div className="grid grid-cols-2 gap-2.5 mt-2">
                  {sysConfig.oauth_github_enabled && (
                    <button
                      type="button"
                      onClick={() => handleOAuthLogin('github')}
                      disabled={loading}
                      className="ui-btn-secondary gap-2 font-semibold"
                    >
                      <svg className="w-4 h-4 fill-current" viewBox="0 0 24 24">
                        <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
                      </svg>
                      <span>GitHub</span>
                    </button>
                  )}
                  {sysConfig.oauth_google_enabled && (
                    <button
                      type="button"
                      onClick={() => handleOAuthLogin('google')}
                      disabled={loading}
                      className="ui-btn-secondary gap-2 font-semibold"
                    >
                      <svg className="w-4 h-4" viewBox="0 0 24 24">
                        <path fill="#4285F4" d="M23.745 12.27c0-.7-.06-1.4-.19-2.07H12v4.51h6.6c-.29 1.52-1.14 2.82-2.4 3.68v3.05h3.88c2.27-2.09 3.665-5.17 3.665-9.17z" />
                        <path fill="#34A853" d="M12 24c3.24 0 5.95-1.08 7.93-2.91l-3.88-3.05c-1.08.72-2.45 1.16-4.05 1.16-3.12 0-5.77-2.1-6.72-4.93H1.25v3.15C3.26 21.36 7.33 24 12 24z" />
                        <path fill="#FBBC05" d="M5.28 14.27c-.25-.72-.38-1.49-.38-2.27s.13-1.55.38-2.27V6.58H1.25C.45 8.18 0 9.99 0 12s.45 3.82 1.25 5.42l4.03-3.15z" />
                        <path fill="#EA4335" d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.33 0 3.26 2.64 1.25 6.58l4.03 3.15c.95-2.83 3.6-4.98 6.72-4.98z" />
                      </svg>
                      <span>Google</span>
                    </button>
                  )}
                </div>
              </div>
            )}
          </form>
        ) : (
          /* TAB: REGISTER */
          <form onSubmit={handleRegisterSubmit} className="space-y-3.5">
            <div>
              <label className="ui-label font-semibold">
                {isZh ? '用户名' : 'Username'}
              </label>
              <div className="relative">
                <User className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  required
                  type="text"
                  value={regUsername}
                  onChange={(e) => setRegUsername(e.target.value)}
                  placeholder={isZh ? '字母/数字，至少 3 字符' : 'Letters/digits, min 3 chars'}
                  className="ui-field pl-9 pr-3"
                />
              </div>
            </div>

            <div>
              <label className="ui-label font-semibold">
                {isZh ? '邮箱地址' : 'Email Address'}
              </label>
              <div className="relative">
                <Mail className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  required
                  type="email"
                  value={regEmail}
                  onChange={(e) => setRegEmail(e.target.value)}
                  placeholder="your-name@company.com"
                  className="ui-field pl-9 pr-3"
                />
              </div>
            </div>

            {sysConfig.require_email_verification && (
              <div>
                <label className="ui-label font-semibold">
                  {isZh ? '邮箱验证码' : 'Email Verification Code'}
                </label>
                <div className="flex space-x-2">
                  <input
                    required
                    type="text"
                    maxLength={6}
                    value={regCode}
                    onChange={(e) => setRegCode(e.target.value)}
                    placeholder={isZh ? '6 位验证码' : '6-digit code'}
                    className="ui-field flex-1 font-mono"
                  />
                  <button
                    type="button"
                    onClick={handleSendCode}
                    disabled={sendingCode || codeCountdown > 0}
                    className="ui-btn-secondary px-3 gap-2 border-transparent bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-indigo-700 dark:text-indigo-300 font-bold whitespace-nowrap"
                  >
                    {sendingCode ? (
                      <RefreshCw className="w-3.5 h-3.5 animate-spin mx-auto" />
                    ) : codeCountdown > 0 ? (
                      `${codeCountdown}s ${isZh ? '后重试' : 'retry'}`
                    ) : (
                      isZh ? '获取验证码' : 'Send Code'
                    )}
                  </button>
                </div>
              </div>
            )}

            <div>
              <label className="ui-label font-semibold">
                {isZh ? '设置密码' : 'Password'}
              </label>
              <div className="relative">
                <Lock className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  required
                  type={showPassword ? 'text' : 'password'}
                  value={regPassword}
                  onChange={(e) => setRegPassword(e.target.value)}
                  placeholder={isZh ? '至少 6 位字符' : 'At least 6 characters'}
                  className="ui-field pl-9 pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
                >
                  {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>

            <div className="pt-2">
              <button
                type="submit"
                disabled={loading}
                className="ui-btn-primary w-full gap-2 font-bold shadow-xs"
              >
                {loading ? (
                  <>
                    <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    <span>{isZh ? '正在创建账号...' : 'Creating account...'}</span>
                  </>
                ) : (
                  <>
                    <span>{isZh ? '立即注册并登录' : 'Register & Enter'}</span>
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
