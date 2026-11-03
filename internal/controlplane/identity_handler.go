package controlplane

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// RegisterRequest defines user registration payload.
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Code     string `json:"code"`
}

// Registration rate limits (per minute, process-local buckets). Registration is
// the only unauthenticated write that mints spendable credit and a working API
// key, so it must not be loopable.
const (
	registerRatePerIPPerMinute       = 5
	registerRatePerUsernamePerMinute = 3
)

// registrationGrant reports whether a registration may receive the trial
// balance and the initial API key. A deployment that requires email
// verification only grants them to a verified address; one that explicitly
// disabled verification accepts unverified signups, so the grant still applies.
//
// The distinction exists so a future two-step flow (create account, verify
// later) cannot accidentally mint credit before the address is verified.
func registrationGrant(verificationRequired, verified bool) bool {
	return verified || !verificationRequired
}

// reservedUsernamePrefixes are namespaces owned by the OAuth providers. A local
// registration must not take them: gh_<github-id>/gg_<google-sub> is the name the
// OAuth flow prefers for that person, and squatting it used to make their first
// third-party login fail.
var reservedUsernamePrefixes = []string{"gh_", "gg_"}

// isReservedUsername reports whether a username falls in a provider namespace.
func isReservedUsername(username string) bool {
	lower := strings.ToLower(strings.TrimSpace(username))
	for _, prefix := range reservedUsernamePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// Register registers a new regular user account with email verification and creates initial balance.
func (h *AdminHandler) Register(c *gin.Context) {
	// Registration creates an account and signs the caller in; it is
	// unauthenticated and state-changing, so it carries the same cross-site gates
	// as login (CP-05).
	if !requireJSONBody(c) || !requireSameOrigin(c) {
		return
	}

	if !config.GetGlobalConfig().IsRegistrationAllowed() {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "企业已关闭自主注册通道，请联系管理员分配账号"})
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "用户名、邮箱与密码为必填项"})
		return
	}

	username := strings.TrimSpace(req.Username)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)

	if len(username) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "用户名至少需要 3 个字符"})
		return
	}
	if isReservedUsername(username) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该用户名前缀为第三方登录保留，请更换用户名"})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "密码长度至少需要 6 个字符"})
		return
	}

	// Throttle registration before any storage work. The client address is the
	// resolved one (trusted proxy hops only), so rotating a forwarding header
	// cannot mint a fresh bucket.
	client := middleware.ResolveClientIdentity(c)
	if !middleware.GlobalRateLimiter.GetBucket("register:ip:"+client.IP, registerRatePerIPPerMinute).Allow() {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "error": "注册请求过于频繁，请稍后再试"})
		return
	}
	if !middleware.GlobalRateLimiter.GetBucket("register:user:"+strings.ToLower(username), registerRatePerUsernamePerMinute).Allow() {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "error": "该用户名注册请求过于频繁，请稍后再试"})
		return
	}

	// Check if username or email already exists before consuming the email
	// verification code.
	//
	// These checks used to run after VerifyCode, so a caller who mistyped their
	// username (or raced another signup for the same address) lost a valid code to
	// a refusal and had to request a new one for a retry that would have succeeded
	// with the code they already had. The checks stay here, next to the throttle,
	// so a deterministic refusal costs no storage work and burns nothing. The
	// insert is still guarded by the unique indexes; a race is reported by
	// respondRegistrationFailure.
	existingUser, _ := h.repo.GetUserByUsername(username)
	if existingUser != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该用户名已被注册"})
		return
	}
	existingEmail, _ := h.repo.GetUserByEmail(email)
	if existingEmail != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "该邮箱已被绑定注册"})
		return
	}

	// Verify the email code when the deployment requires it. A code that is
	// supplied while verification is optional is still honoured (and must be
	// valid) instead of being silently discarded.
	verificationRequired := isEmailVerificationRequired()
	verified := false
	if verificationRequired {
		if code == "" || !h.repo.VerifyCode(email, code, "register") {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "验证码无效或已过期，请重新获取"})
			return
		}
		verified = true
	} else if code != "" {
		if !h.repo.VerifyCode(email, code, "register") {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "验证码无效或已过期，请重新获取"})
			return
		}
		verified = true
	}
	// Fail closed: an unverified registration must never be granted the trial
	// balance or an API key.
	if !registrationGrant(verificationRequired, verified) {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "注册需要先完成邮箱验证"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "密码加密失败"})
		return
	}

	trialBalance := config.GetGlobalConfig().GetInitialUserBalance()
	user := &storage.UserRecord{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
		Balance:      trialBalance,
		GroupName:    "default",
	}

	// Automatically generate an initial API key for the new user
	newKey, err := GenerateAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "安全随机源不可用，无法生成密钥"})
		return
	}

	if err := h.repo.CreateUserWithAPIKey(user, &storage.APIKeyRecord{
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
		respondRegistrationFailure(c, err)
		return
	}
	h.syncDataPlane()

	// Generate login token
	token, _ := GenerateAdminToken(user, 7*24*time.Hour)
	SetSessionCookies(c, token)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("注册成功！初始额度为 ¥%.2f，已自动生成 API 密钥", user.Balance),
		"data": gin.H{
			"token": token,
			"user": gin.H{
				"id":         user.ID,
				"username":   user.Username,
				"email":      user.Email,
				"role":       user.Role,
				"status":     user.Status,
				"balance":    user.Balance,
				"group_name": user.GroupName,
			},
			"api_key": newKey,
		},
	})
}

// respondRegistrationFailure renders the account-creation error.
//
// A duplicate email is a deterministic client error rather than a server
// failure. Registration checks whether the email exists and then inserts, so two
// concurrent requests carrying the same address can both pass the check; the
// unique index on LOWER(email) (migration 27) makes the database reject the
// loser, and that rejection is reported here as 409 instead of a generic 500 (or,
// before the index, a second account silently sharing the address).
func respondRegistrationFailure(c *gin.Context, err error) {
	if errors.Is(err, storage.ErrEmailAlreadyRegistered) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "error": "该邮箱已被绑定注册"})
		return
	}
	if errors.Is(err, storage.ErrUsernameAlreadyRegistered) {
		// Account identity is case-insensitive (migration 44): "Alice" and "alice"
		// are the same account name, and saying so is friendlier than a 500.
		c.JSON(http.StatusConflict, gin.H{"code": 409, "error": "该用户名已被注册（用户名不区分大小写）"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "创建账号及初始密钥失败，请重试"})
}
