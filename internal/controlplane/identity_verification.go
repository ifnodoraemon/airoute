package controlplane

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// isEmailVerificationRequired checks if email verification is mandatory in this deployment.
func isEmailVerificationRequired() bool {
	return config.GetGlobalConfig().IsEmailVerificationRequired()
}

// sendVerificationEmail sends a 6-digit verification code via SMTP if configured.
func sendVerificationEmail(targetEmail, code, purpose string) error {
	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if smtpHost == "" {
		telemetry.Logger.Info("email verification code generated (SMTP not configured, logged for audit)",
			"email", targetEmail, "purpose", purpose, "code", code)
		return nil
	}

	smtpPort := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if smtpPort == "" {
		smtpPort = "587"
	}
	smtpUser := strings.TrimSpace(os.Getenv("SMTP_USER"))
	smtpPass := os.Getenv("SMTP_PASS")
	smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if smtpFrom == "" {
		if smtpUser != "" {
			smtpFrom = smtpUser
		} else {
			smtpFrom = "noreply@" + smtpHost
		}
	}

	subject := "Airoute 验证码"
	actionName := "注册新账号"
	if purpose == "reset" {
		subject = "Airoute 密码重置验证码"
		actionName = "重置登录密码"
	}

	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n"+
		"<!DOCTYPE html><html><body style=\"font-family:sans-serif;line-height:1.6;color:#333;\">"+
		"<div style=\"max-width:540px;margin:20px auto;padding:24px;border:1px solid #e2e8f0;border-radius:16px;\">"+
		"<h2 style=\"color:#4f46e5;margin-top:0;\">Airoute 智能网关安全验证</h2>"+
		"<p>您正在进行 <strong>%s</strong> 操作，本次安全验证码为：</p>"+
		"<div style=\"font-size:32px;font-weight:bold;letter-spacing:6px;color:#1e293b;background:#f1f5f9;padding:16px;text-align:center;border-radius:12px;margin:20px 0;\">%s</div>"+
		"<p style=\"color:#64748b;font-size:13px;\">验证码在 10 分钟内有效。如非本人操作，请忽略此邮件。</p>"+
		"</div></body></html>", smtpFrom, targetEmail, subject, actionName, code)

	addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)
	var auth smtp.Auth
	if smtpUser != "" && smtpPass != "" {
		auth = smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	}

	return smtp.SendMail(addr, auth, smtpFrom, []string{targetEmail}, []byte(body))
}

// SendVerificationCodeRequest defines request to send email verification code.
type SendVerificationCodeRequest struct {
	Email   string `json:"email" binding:"required"`
	Purpose string `json:"purpose"` // "register" or "reset"
}

// SendVerificationCode generates and stores a 6-digit email verification code.
func (h *AdminHandler) SendVerificationCode(c *gin.Context) {
	var req SendVerificationCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效的电子邮箱地址"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "邮箱格式不正确"})
		return
	}

	purpose := req.Purpose
	if purpose == "" {
		purpose = "register"
	}

	// Generate 6-digit secure numeric code
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	var code string
	if err != nil {
		code = fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	} else {
		code = fmt.Sprintf("%06d", n.Int64()+100000)
	}

	// Store code with 10-minute expiry
	if err := h.repo.SaveVerificationCode(email, code, purpose, 10*time.Minute); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "保存验证码失败: " + err.Error()})
		return
	}

	// Dispatch email via SMTP if configured
	if err := sendVerificationEmail(email, code, purpose); err != nil {
		telemetry.Logger.Error("failed to dispatch verification email via SMTP", "email", email, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "发送邮件失败: " + err.Error()})
		return
	}

	// In automated test or development mode, return dev_code for testing convenience
	isDevOrTest := strings.HasSuffix(os.Args[0], ".test") || strings.Contains(os.Args[0], "/_test/") || os.Getenv("ENV") == "development"
	resp := gin.H{
		"code":    0,
		"message": fmt.Sprintf("验证码已成功发送至邮箱 %s (10分钟内有效)", email),
	}
	if isDevOrTest {
		resp["dev_code"] = code
	}
	c.JSON(http.StatusOK, resp)
}
