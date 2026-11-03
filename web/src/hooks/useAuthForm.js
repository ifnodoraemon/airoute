import { useEffect, useState } from 'react';

// Shared by the dedicated auth page and the portal's login dialog.
export function useAuthForm({ initialTab = 'login', isOpen = true, lang = 'zh', onLoginSuccess, onClose }) {
  const isZh = lang === 'zh';
  const [activeTab, setActiveTab] = useState(initialTab);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
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
  const [sysConfig, setSysConfig] = useState({ allow_registration: true, require_email_verification: false, oauth_github_enabled: false, oauth_google_enabled: false });

  useEffect(() => {
    if (!isOpen) return undefined;
    const controller = new AbortController();
    fetch('/api/v1/public/status', { signal: controller.signal })
      .then(res => res.json())
      .then(data => {
        if (!controller.signal.aborted && data.code === 0 && data.data) setSysConfig({
          allow_registration: data.data.allow_registration !== false,
          require_email_verification: !!data.data.require_email_verification,
          oauth_github_enabled: !!data.data.oauth_github_enabled,
          oauth_google_enabled: !!data.data.oauth_google_enabled,
        });
      }).catch(() => {});
    return () => controller.abort();
  }, [isOpen]);

  useEffect(() => { setError(''); setSuccessMsg(''); }, [activeTab]);
  useEffect(() => {
    if (!sysConfig.allow_registration && activeTab === 'register') setActiveTab('login');
  }, [sysConfig.allow_registration, activeTab]);
  const countingDown = isOpen && codeCountdown > 0;
  useEffect(() => {
    if (!countingDown) return undefined;
    const timer = setInterval(() => setCodeCountdown(prev => Math.max(0, prev - 1)), 1000);
    return () => clearInterval(timer);
  }, [countingDown]);

  const finishLogin = data => {
    localStorage.setItem('airoute_user', JSON.stringify(data.user));
    onLoginSuccess(data.token, data.user);
    onClose?.();
  };

  const handleLoginSubmit = async event => {
    event.preventDefault(); setError(''); setSuccessMsg(''); setLoading(true);
    try {
      const response = await fetch('/api/v1/auth/login', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: username.trim(), password }),
      });
      const data = await response.json();
      if (response.ok && data.code === 0 && data.data?.token) finishLogin(data.data);
      else setError(data.error || (isZh ? '登录失败，请核对用户名/邮箱与密码' : 'Login failed. Check your username/email and password.'));
    } catch (failure) { setError((isZh ? '网络请求失败: ' : 'Network request failed: ') + failure.message); }
    finally { setLoading(false); }
  };

  const handleSendCode = async () => {
    if (!regEmail.trim()) { setError(isZh ? '请先填写有效的电子邮箱' : 'Please enter a valid email address'); return; }
    setError(''); setSuccessMsg(''); setSendingCode(true);
    try {
      const response = await fetch('/api/v1/auth/send-verification-code', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: regEmail.trim(), purpose: 'register' }),
      });
      const data = await response.json();
      if (response.ok && data.code === 0) {
        setSuccessMsg(data.message || (isZh ? '验证码已发送至邮箱' : 'Verification code sent to email'));
        if (data.dev_code) setRegCode(data.dev_code);
        setCodeCountdown(60);
      } else setError(data.error || (isZh ? '发送验证码失败' : 'Failed to send verification code'));
    } catch (failure) { setError((isZh ? '请求异常: ' : 'Request error: ') + failure.message); }
    finally { setSendingCode(false); }
  };

  const handleRegisterSubmit = async event => {
    event.preventDefault(); setError(''); setSuccessMsg(''); setLoading(true);
    try {
      const payload = { username: regUsername.trim(), email: regEmail.trim(), password: regPassword };
      if (sysConfig.require_email_verification || regCode.trim()) payload.code = regCode.trim();
      const response = await fetch('/api/v1/auth/register', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
      });
      const data = await response.json();
      if (response.ok && data.code === 0 && data.data?.token) {
        if (data.data.api_key) { setRegistrationResult(data.data); setRegPassword(''); }
        else finishLogin(data.data);
      } else setError(data.error || (isZh ? '注册失败，请检查填写内容' : 'Registration failed. Check your details.'));
    } catch (failure) { setError((isZh ? '网络请求失败: ' : 'Network request failed: ') + failure.message); }
    finally { setLoading(false); }
  };

  const completeRegistration = () => {
    if (!registrationResult) return;
    const result = registrationResult;
    setRegistrationResult(null);
    finishLogin(result);
  };

  const handleOAuthLogin = async provider => {
    setError('');
    try {
      const response = await fetch(`/api/v1/auth/oauth/${provider}`, { credentials: 'same-origin' });
      const data = await response.json().catch(() => ({}));
      if (response.ok && data.auth_url) window.location.href = data.auth_url;
      else setError(data.error || (isZh ? `企业未配置 ${provider.toUpperCase()} 登录通道` : `${provider.toUpperCase()} SSO login is not configured`));
    } catch (failure) { setError((isZh ? 'OAuth 异常: ' : 'OAuth error: ') + failure.message); }
  };

  return {
    activeTab, setActiveTab, username, setUsername, password, setPassword, showPassword, setShowPassword,
    regUsername, setRegUsername, regEmail, setRegEmail, regPassword, setRegPassword, regCode, setRegCode,
    sendingCode, codeCountdown, loading, error, setError, successMsg, setSuccessMsg, sysConfig, registrationResult,
    handleLoginSubmit, handleSendCode, handleRegisterSubmit, handleOAuthLogin, completeRegistration,
  };
}
