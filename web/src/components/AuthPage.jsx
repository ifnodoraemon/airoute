import { useState, useEffect } from 'react';
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
  Zap,
  ArrowLeft,
  Globe
} from 'lucide-react';

export default function AuthPage({
  initialTab = 'login',
  onLoginSuccess,
  onBackHome,
  lang = 'zh',
  toggleLang,
}) {
  const isZh = lang === 'zh';
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

  // System security and capabilities config
  const [sysConfig, setSysConfig] = useState({
    allow_registration: true,
    require_email_verification: false,
    oauth_github_enabled: false,
    oauth_google_enabled: false
  });

  // Feedback state
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [successMsg, setSuccessMsg] = useState('');

  useEffect(() => {
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

  useEffect(() => {
    setError('');
    setSuccessMsg('');
  }, [activeTab]);

  // The resend countdown owns its interval. Driving it from an effect ties the timer
  // to this component's lifetime: leaving the auth page (or the whole view being
  // replaced) clears it instead of leaving a 60-second tick running against an
  // unmounted instance.
  const isCountingDown = codeCountdown > 0;
  useEffect(() => {
    if (!isCountingDown) return undefined;
    const timer = setInterval(() => {
      setCodeCountdown((prev) => (prev <= 1 ? 0 : prev - 1));
    }, 1000);
    return () => clearInterval(timer);
  }, [isCountingDown]);

  useEffect(() => {
    if (!sysConfig.allow_registration && activeTab === 'register') {
      setActiveTab('login');
    }
  }, [sysConfig.allow_registration, activeTab]);

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
      } else {
        setError(data.error || (isZh ? '登录失败，请核对用户名/邮箱与密码' : 'Login failed, please check username/email and password'));
      }
    } catch (err) {
      setError((isZh ? '网络请求失败，请确保服务正常运行: ' : 'Network request failed: ') + err.message);
    } finally {
      setLoading(false);
    }
  };

  // Send Email Verification Code
  const handleSendCode = async () => {
    if (!regEmail.trim()) {
      setError(isZh ? '请先填写有效的电子邮箱' : 'Please enter a valid email address');
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
        setSuccessMsg(data.message || (isZh ? '验证码已发送至邮箱' : 'Verification code sent to email'));
        if (data.dev_code) {
          setRegCode(data.dev_code);
          setSuccessMsg(isZh ? `验证码已发送！已自动为您填入: ${data.dev_code}` : `Code sent! Auto-filled: ${data.dev_code}`);
        }
        setCodeCountdown(60);
      } else {
        setError(data.error || (isZh ? '发送验证码失败' : 'Failed to send verification code'));
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
      const payload = {
        username: regUsername.trim(),
        email: regEmail.trim(),
        password: regPassword
      };
      if (sysConfig.require_email_verification || regCode.trim()) {
        payload.code = regCode.trim();
      }

      const res = await fetch('/api/v1/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();

      if (res.ok && data.code === 0 && data.data?.token) {
        localStorage.setItem('airoute_user', JSON.stringify(data.data.user));
        onLoginSuccess(data.data.token, data.data.user);
      } else {
        setError(data.error || (isZh ? '注册失败，请检查填写内容' : 'Registration failed, please check inputs'));
      }
    } catch (err) {
      setError((isZh ? '网络请求失败: ' : 'Network request failed: ') + err.message);
    } finally {
      setLoading(false);
    }
  };

  // OAuth Login (GitHub / Google)
  const handleOAuthLogin = async (provider) => {
    setError('');
    try {
      const res = await fetch(`/api/v1/auth/oauth/${provider}`);
      const data = await res.json();
      if (res.ok && data.auth_url) {
        window.location.href = data.auth_url;
      } else {
        setError(data.error || (isZh ? `企业未配置 ${provider.toUpperCase()} 登录通道` : `${provider.toUpperCase()} SSO login is not configured`));
      }
    } catch (err) {
      setError((isZh ? 'OAuth 异常: ' : 'OAuth error: ') + err.message);
    }
  };

  return (
    <div className="min-h-screen bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] flex flex-col justify-between py-10 px-4 sm:px-6 lg:px-8 selection:bg-[#355CFF] selection:text-white">
      {/* Top Bar with Brand & Back */}
      <div className="max-w-md w-full mx-auto flex items-center justify-between">
        <button
          onClick={onBackHome}
          className="inline-flex items-center space-x-1.5 text-xs font-semibold text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] transition py-2 px-3 rounded-xl hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] cursor-pointer"
        >
          <ArrowLeft className="w-4 h-4" />
          <span>{isZh ? '返回产品门户' : 'Back to Portal'}</span>
        </button>

        <div className="flex items-center space-x-3">
          {toggleLang && (
            <button
              onClick={toggleLang}
              type="button"
              className="flex items-center space-x-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] text-[#202124] dark:text-[#F1F2F4] hover:text-[#355CFF] dark:hover:text-[#91A7FF] shadow-2xs transition cursor-pointer"
            >
              <Globe className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
              <span>{isZh ? 'EN' : '中'}</span>
            </button>
          )}

          <div className="flex items-center space-x-2">
            <div className="w-7 h-7 rounded-lg bg-[#355CFF] flex items-center justify-center text-white shadow-2xs">
              <Zap className="w-4 h-4 fill-white" />
            </div>
            <span className="font-bold text-sm text-[#202124] dark:text-[#F1F2F4] tracking-tight">Airoute</span>
          </div>
        </div>
      </div>

      {/* Main Card Container */}
      <div className="max-w-md w-full mx-auto my-auto">
        <div className="bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] rounded-2xl p-8 shadow-xl shadow-slate-900/5 dark:shadow-black/20 space-y-6">
          {/* Header */}
          <div className="text-center space-y-1.5">
            <h2 className="text-2xl font-bold text-[#202124] dark:text-[#F1F2F4] tracking-tight">
              {activeTab === 'login' ? (isZh ? '欢迎回来' : 'Welcome Back') : (isZh ? '账号注册' : 'Create Account')}
            </h2>
          </div>

          {/* Tab Switcher */}
          {sysConfig.allow_registration && (
            <div className="flex p-1 bg-[#F0F1F2] dark:bg-[#22252B] rounded-lg border border-[#E4E6E9] dark:border-[#2C3038]">
              <button
                type="button"
                onClick={() => setActiveTab('login')}
                className={`flex-1 py-2 text-xs font-semibold rounded-md transition cursor-pointer ${
                  activeTab === 'login'
                    ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs'
                    : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
                }`}
              >
                {isZh ? '账号登录' : 'Sign In'}
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('register')}
                className={`flex-1 py-2 text-xs font-semibold rounded-md transition cursor-pointer ${
                  activeTab === 'register'
                    ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs'
                    : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
                }`}
              >
                {isZh ? '新用户注册' : 'Register'}
              </button>
            </div>
          )}

          {/* Feedback Messages */}
          {error && (
            <div className="p-3.5 rounded-2xl bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-900/60 text-rose-700 dark:text-rose-300 text-xs flex items-center space-x-2 animate-in fade-in">
              <AlertCircle className="w-4 h-4 shrink-0 text-rose-600 dark:text-rose-400" />
              <span>{error}</span>
            </div>
          )}
          {successMsg && (
            <div className="p-3.5 rounded-2xl bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-900/60 text-emerald-700 dark:text-emerald-300 text-xs flex items-center space-x-2 animate-in fade-in">
              <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
              <span>{successMsg}</span>
            </div>
          )}

          {/* LOGIN FORM */}
          {activeTab === 'login' && (
            <form onSubmit={handleLoginSubmit} className="space-y-4">
              <div>
                <label className="ui-label font-semibold mb-1.5">
                  {isZh ? '用户名 / 绑定邮箱' : 'Username / Email'}
                </label>
                <div className="relative">
                  <div className="absolute left-0 top-1/2 -translate-y-1/2 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <User className="w-4 h-4" />
                  </div>
                  <input
                    type="text"
                    required
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    placeholder={isZh ? "输入用户名或登录邮箱" : "Enter username or email"}
                    className="ui-field pl-10 pr-4"
                  />
                </div>
              </div>

              <div>
                <label className="ui-label font-semibold mb-1.5">
                  {isZh ? '登录密码' : 'Password'}
                </label>
                <div className="relative">
                  <div className="absolute left-0 top-1/2 -translate-y-1/2 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <Lock className="w-4 h-4" />
                  </div>
                  <input
                    type={showPassword ? 'text' : 'password'}
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder={isZh ? "输入账户密码" : "Enter account password"}
                    className="ui-field pl-10 pr-10 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3.5 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 cursor-pointer"
                  >
                    {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="ui-btn-primary w-full py-3 bg-[#355CFF] hover:bg-[#2B4CE0] text-white shadow-xs"
              >
                {loading ? (
                  <RefreshCw className="w-4 h-4 animate-spin" />
                ) : (
                  <>
                    <span>{isZh ? '登录工作台' : 'Sign In to Console'}</span>
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
                <label className="ui-label font-semibold">
                  {isZh ? '登录用户名 *' : 'Username *'}
                </label>
                <div className="relative">
                  <div className="absolute left-0 top-1/2 -translate-y-1/2 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <User className="w-4 h-4" />
                  </div>
                  <input
                    type="text"
                    required
                    value={regUsername}
                    onChange={(e) => setRegUsername(e.target.value)}
                    placeholder={isZh ? "至少 3 个字符 (字母、数字)" : "At least 3 characters (letters, numbers)"}
                    className="ui-field pl-10 pr-4"
                  />
                </div>
              </div>

              <div>
                <label className="ui-label font-semibold">
                  {isZh ? '电子邮箱 *' : 'Email Address *'}
                </label>
                <div className="relative">
                  <div className="absolute left-0 top-1/2 -translate-y-1/2 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <Mail className="w-4 h-4" />
                  </div>
                  <input
                    type="email"
                    required
                    value={regEmail}
                    onChange={(e) => setRegEmail(e.target.value)}
                    placeholder="name@example.com"
                    className="ui-field pl-10 pr-4"
                  />
                </div>
              </div>

              {sysConfig.require_email_verification && (
                <div>
                  <label className="ui-label font-semibold">
                    {isZh ? '邮箱验证码 *' : 'Verification Code *'}
                  </label>
                  <div className="flex space-x-2">
                    <input
                      type="text"
                      required
                      maxLength={6}
                      value={regCode}
                      onChange={(e) => setRegCode(e.target.value)}
                      placeholder={isZh ? "6 位数字验证码" : "6-digit code"}
                      className="ui-field flex-1 px-4 font-mono tracking-widest text-center"
                    />
                    <button
                      type="button"
                      disabled={sendingCode || codeCountdown > 0}
                      onClick={handleSendCode}
                      className="ui-btn-secondary shrink-0"
                    >
                      {sendingCode ? (
                        <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                      ) : codeCountdown > 0 ? (
                        isZh ? `${codeCountdown}s 后重发` : `Resend in ${codeCountdown}s`
                      ) : (
                        isZh ? '获取验证码' : 'Get Code'
                      )}
                    </button>
                  </div>
                </div>
              )}

              <div>
                <label className="ui-label font-semibold">
                  {isZh ? '设置密码 *' : 'Set Password *'}
                </label>
                <div className="relative">
                  <div className="absolute left-0 top-1/2 -translate-y-1/2 pl-3.5 flex items-center pointer-events-none text-slate-400">
                    <Lock className="w-4 h-4" />
                  </div>
                  <input
                    type={showPassword ? 'text' : 'password'}
                    required
                    value={regPassword}
                    onChange={(e) => setRegPassword(e.target.value)}
                    placeholder={isZh ? "至少 6 个字符" : "At least 6 characters"}
                    className="ui-field pl-10 pr-10 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3.5 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 cursor-pointer"
                  >
                    {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="ui-btn-primary w-full py-3 bg-[#355CFF] hover:bg-[#2B4CE0] text-white shadow-xs"
              >
                {loading ? (
                  <RefreshCw className="w-4 h-4 animate-spin" />
                ) : (
                  <>
                    <span>{isZh ? '完成注册并进入工作台' : 'Complete Registration & Enter Console'}</span>
                    <ArrowRight className="w-4 h-4" />
                  </>
                )}
              </button>
            </form>
          )}

          {/* OAuth Fast Login */}
          {(sysConfig.oauth_github_enabled || sysConfig.oauth_google_enabled) && (
            <div className="pt-2">
              <div className="relative flex py-2 items-center">
                <div className="grow border-t border-[#E4E6E9] dark:border-[#2C3038]"></div>
                <span className="shrink mx-3 ui-meta font-medium">
                  {isZh ? '企业第三方单点登录' : 'Enterprise SSO Login'}
                </span>
                <div className="grow border-t border-[#E4E6E9] dark:border-[#2C3038]"></div>
              </div>

              <div className="grid grid-cols-2 gap-3 mt-3">
                {sysConfig.oauth_github_enabled && (
                  <button
                    type="button"
                    onClick={() => handleOAuthLogin('github')}
                    className="ui-btn-secondary px-3 py-2.5"
                  >
                    <svg className="w-4 h-4 text-slate-800 dark:text-slate-200" fill="currentColor" viewBox="0 0 24 24">
                      <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
                    </svg>
                    <span>{isZh ? 'GitHub 登录' : 'GitHub SSO'}</span>
                  </button>
                )}

                {sysConfig.oauth_google_enabled && (
                  <button
                    type="button"
                    onClick={() => handleOAuthLogin('google')}
                    className="ui-btn-secondary px-3 py-2.5"
                  >
                    <svg className="w-4 h-4" viewBox="0 0 24 24">
                      <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
                      <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
                      <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z" />
                      <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z" />
                    </svg>
                    <span>{isZh ? 'Google 登录' : 'Google SSO'}</span>
                  </button>
                )}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Footer copyright */}
      <div className="max-w-md w-full mx-auto text-center ui-meta">
        © {new Date().getFullYear()} Airoute · {isZh ? '极简高性能 AI 路由器' : 'Minimalist High-Performance AI Router'}
      </div>
    </div>
  );
}
