package controlplane

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// OAuthCallback completes the OAuth login (real exchange or guarded simulated
// flow), extracted from oauth_handler.go to keep the initiate/helpers file under
// the size red line.
func (h *AdminHandler) OAuthCallback(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))
	if provider != "github" && provider != "google" {
		h.oauthError(c, http.StatusBadRequest, "不支持的 OAuth 提供商")
		return
	}

	var req OAuthCallbackRequest
	_ = c.ShouldBindJSON(&req)

	// Code and state may arrive via query string on the browser redirect.
	if req.Code == "" {
		req.Code = c.Query("code")
	}
	stateParam := req.State
	if stateParam == "" {
		stateParam = c.Query("state")
	}

	clientID := os.Getenv(fmt.Sprintf("%s_CLIENT_ID", strings.ToUpper(provider)))
	clientSecret := os.Getenv(fmt.Sprintf("%s_CLIENT_SECRET", strings.ToUpper(provider)))
	redirectURI := os.Getenv(fmt.Sprintf("%s_REDIRECT_URI", strings.ToUpper(provider)))
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s/api/v1/auth/oauth/%s/callback", ResolvePublicBaseURL(c), provider)
	}

	isTestOrDev := strings.HasSuffix(os.Args[0], ".test") || strings.Contains(os.Args[0], "/_test/") || strings.EqualFold(os.Getenv("ENABLE_SIMULATED_OAUTH"), "true")

	var email, username, subject string
	realExchange := false

	if clientID != "" && clientSecret != "" && req.Code != "" && req.Code != "demo" && !req.Simulated {
		// Real production token exchange: the OAuth state must be valid, signed
		// for this provider, unexpired, and bound to this browser. This closes
		// the OAuth login-CSRF hole (an attacker-supplied code cannot be
		// replayed into a victim's session).
		if err := bindOAuthState(c, stateParam, provider); err != nil {
			h.oauthError(c, http.StatusForbidden, "OAuth 安全校验失败: "+err.Error())
			return
		}
		clearOAuthStateCookie(c)
		realExchange = true

		if provider == "github" {
			realUser, realEmail, err := exchangeGitHubOAuth(req.Code, clientID, clientSecret)
			if err != nil {
				h.oauthError(c, http.StatusBadRequest, "GitHub 授权失败: "+err.Error())
				return
			}
			subject = realUser
			// The username is allocated when the account is created: the obvious
			// gh_<id> name can be squatted by local registration.
			username = ""
			email = realEmail
		} else {
			realUser, realEmail, err := exchangeGoogleOAuth(req.Code, clientID, clientSecret, redirectURI)
			if err != nil {
				h.oauthError(c, http.StatusBadRequest, "Google 授权失败: "+err.Error())
				return
			}
			subject = realUser
			username = ""
			email = realEmail
		}
	} else if isTestOrDev {
		// Simulated authentication is only reachable in test binaries or when
		// ENABLE_SIMULATED_OAUTH=true is set explicitly. It must never be able to
		// take over a privileged account, so the requested identity is only
		// honoured for newly generated demo users.
		//
		// It also mints accounts without a password, so throttle it like login:
		// an open ENABLE_SIMULATED_OAUTH must not let one client mass-create
		// trial-balance accounts. The IP bucket applies behind a trusted proxy;
		// the account bucket curbs repeated attempts at the same username.
		simIdentity := middleware.ResolveClientIdentity(c)
		simClient := loginClient{Username: req.Username, IP: simIdentity.IP, IPIsForwarded: simIdentity.Forwarded}
		if ok, retry := globalLoginGuard.check(c.Request.Context(), simClient); !ok {
			c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			h.oauthError(c, http.StatusTooManyRequests, "模拟登录尝试过多，请稍后再试")
			return
		}
		globalLoginGuard.failure(c.Request.Context(), simClient)
		if req.Username != "" {
			existing, _ := h.repo.GetUserByUsername(req.Username)
			if existing != nil {
				h.oauthError(c, http.StatusForbidden, "模拟登录不能接管已存在的账号，请使用标准密码或真实 OAuth 登录")
				return
			}
			username = req.Username
		} else {
			username = fmt.Sprintf("%s_user_%d", provider, time.Now().Unix()%10000)
		}
		if req.Email != "" {
			email = req.Email
		} else {
			email = fmt.Sprintf("%s@%s.oauth.local", username, provider)
		}
	} else {
		h.oauthError(c, http.StatusForbidden, fmt.Sprintf("系统未配置 %s 生产应用密钥，禁止使用模拟快捷登录", strings.ToUpper(provider)))
		return
	}

	var user *storage.UserRecord
	var lookupErr error
	if realExchange {
		user, lookupErr = h.repo.GetUserByOAuthIdentity(provider, subject)
	} else {
		user, lookupErr = h.repo.GetUserByUsername(username)
	}
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		h.oauthError(c, http.StatusInternalServerError, "无法读取账号，请稍后重试")
		return
	}
	if !realExchange && user == nil && email != "" {
		user, lookupErr = h.repo.GetUserByEmail(email)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			h.oauthError(c, 500, "无法验证邮箱")
			return
		}
	}
	if !realExchange && user != nil {
		h.oauthError(c, http.StatusForbidden, "模拟登录不能接管已有账号")
		return
	}

	if user == nil && realExchange && email != "" {
		owner, err := h.repo.GetUserByEmail(email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			h.oauthError(c, 500, "无法验证邮箱")
			return
		}
		if owner != nil {
			h.oauthError(c, http.StatusConflict, "该邮箱已有账号，请使用原账号登录；第三方身份不会自动关联")
			return
		}
	}
	if user == nil {
		if !config.GetGlobalConfig().IsRegistrationAllowed() {
			h.oauthError(c, http.StatusForbidden, "自主注册已关闭，请联系管理员分配账号")
			return
		}
		// New OAuth user: create with trial balance
		randPassBytes := make([]byte, 16)
		if _, err := rand.Read(randPassBytes); err != nil {
			h.oauthError(c, http.StatusInternalServerError, "安全随机源不可用")
			return
		}
		hash, err := bcrypt.GenerateFromPassword(randPassBytes, bcrypt.DefaultCost)
		if err != nil {
			h.oauthError(c, http.StatusInternalServerError, "创建 OAuth 账户失败")
			return
		}

		allocated, allocErr := allocateOAuthUsername(h.repo, provider, subject, username)
		if allocErr != nil {
			h.oauthError(c, http.StatusInternalServerError, "无法为第三方账号分配用户名，请稍后重试")
			return
		}
		username = allocated
		user = &storage.UserRecord{
			Username:     username,
			Email:        email,
			PasswordHash: string(hash),
			Role:         "user",
			Status:       "active",
			Balance:      config.GetGlobalConfig().GetInitialUserBalance(),
			GroupName:    "default",
		}
		// Create default API key
		newKey, err := GenerateAPIKey()
		if err != nil {
			h.oauthError(c, http.StatusInternalServerError, "安全随机源不可用，无法生成密钥")
			return
		}
		if err := h.repo.CreateOAuthUserWithAPIKey(provider, subject, user, &storage.APIKeyRecord{
			Key:           newKey,
			TenantID:      username,
			UserID:        user.ID,
			GroupName:     "default",
			AllowedModels: []string{"*"},
			RPM:           60,
			TPM:           100000,
			Budget:        100.0,
			Status:        "active",
		}); err != nil {
			h.oauthError(c, http.StatusInternalServerError, "创建 OAuth 账号及初始密钥失败，请重试")
			return
		}
		h.syncDataPlane()
	}

	if strings.EqualFold(user.Status, "locked") || strings.EqualFold(user.Status, "deleted") {
		h.oauthError(c, http.StatusForbidden, "该账户已被锁定或删除，无法登录")
		return
	}

	token, err := GenerateAdminToken(user, 7*24*time.Hour)
	if err != nil {
		h.oauthError(c, http.StatusInternalServerError, "生成鉴权 Token 失败")
		return
	}
	SetSessionCookies(c, token)

	// The callback already installed the HttpOnly session. Any replica can
	// authenticate it; the SPA does not exchange a process-local code.
	if c.Request.Method == http.MethodGet {
		h.redirectOAuthResult(c, "1", "")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("通过 %s 授权登录成功", strings.ToUpper(provider)),
		"data": gin.H{
			"token": token,
			"user":  oauthUserPayload(user),
		},
	})
}