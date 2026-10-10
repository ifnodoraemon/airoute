package api

import (
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/web"
)

// SetupRouter initializes and configures the Gin engine.
func SetupRouter(dispatcher *router.Dispatcher, adminHandler *controlplane.AdminHandler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global recovery
	r.Use(gin.Recovery())

	// Distributed Trace Correlation & Structured Access Logging
	r.Use(middleware.TraceMiddleware())
	r.Use(middleware.AccessLogMiddleware())

	// Enterprise CORS & Security Headers
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH, HEAD")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key, X-API-Key, anthropic-version, x-goog-api-key, x-admin-token, X-Session-ID, X-Request-ID, X-Airoute-Trace-Id, MCP-Protocol-Version, Stripe-Signature")
		c.Header("Access-Control-Expose-Headers", "Content-Disposition, Content-Length, X-Airoute-Trace-Id, X-Request-Id, X-Airoute-Cost, X-Airoute-Off-Peak, X-Airoute-Off-Peak-Discount, X-Airoute-Cached-Tokens, X-Airoute-Saved-Cost, X-Cache-Status, X-RateLimit-Limit-Requests, Retry-After")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("X-XSS-Protection", "1; mode=block")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Mount embedded Web UI
	web.RegisterStaticRoutes(r)

	handler := NewHandler(dispatcher)

	// Public health and observability endpoints
	r.GET("/health", handler.HandleHealth)
	r.HEAD("/health", handler.HandleHealth)
	r.GET("/metrics", handler.HandleMetrics)
	r.HEAD("/", func(c *gin.Context) {
		c.Status(200)
	})
	r.GET("/api/v1/public/status", handler.HandlePublicStatus)

	// Model Context Protocol (MCP) server endpoints for AI Agents (Cursor, Claude Desktop, Cline, etc.)
	var repo *storage.Repository
	if adminHandler != nil {
		repo = adminHandler.GetRepo()
	}
	mcpHandler := NewMCPHandler(dispatcher, repo)
	r.GET("/mcp/sse", mcpHandler.HandleMCPSSE)
	r.POST("/mcp/messages", mcpHandler.HandleMCPMessages)
	r.GET("/mcp", mcpHandler.HandleMCPInfo)
	r.GET("/v1/mcp/sse", mcpHandler.HandleMCPSSE)
	r.POST("/v1/mcp/messages", mcpHandler.HandleMCPMessages)
	r.GET("/v1/mcp", mcpHandler.HandleMCPInfo)

	// Admin & User Control Plane APIs
	if adminHandler != nil {
		// Unified Public Auth Endpoints
		authGroup := r.Group("/api/v1/auth")
		{
			authGroup.POST("/login", adminHandler.Login)
			authGroup.POST("/register", adminHandler.Register)
			authGroup.POST("/send-verification-code", adminHandler.SendVerificationCode)
			authGroup.GET("/oauth/:provider", adminHandler.OAuthInitiate)
			authGroup.POST("/oauth/:provider/callback", adminHandler.OAuthCallback)
			authGroup.GET("/oauth/:provider/callback", adminHandler.OAuthCallback)
			authGroup.GET("/me", adminHandler.AdminAuthMiddleware(), adminHandler.GetMe)
		}

		// Public Payment Webhooks
		r.POST("/api/v1/public/stripe/webhook", adminHandler.StripeWebhook)

		// Public Skill Bundle Download (agentskills.io ZIP format)
		r.GET("/api/v1/skills/:id/download", adminHandler.DownloadSkillZip)

		// Regular User Self-Service APIs
		userGroup := r.Group("/api/v1/user")
		userGroup.Use(adminHandler.AdminAuthMiddleware())
		{
			userGroup.GET("/me", adminHandler.GetMe)
			userGroup.GET("/wallet", adminHandler.GetUserWallet)
			userGroup.POST("/wallet/redeem", adminHandler.RedeemWalletCode)
			userGroup.POST("/wallet/recharge/stripe/session", adminHandler.CreateStripeRechargeSession)
			userGroup.POST("/wallet/recharge/sandbox", adminHandler.SandboxRecharge)
			userGroup.GET("/wallet/recharge/sandbox", adminHandler.SandboxRecharge)
			userGroup.GET("/wallet/orders", adminHandler.ListUserRechargeOrders)
			userGroup.GET("/keys", adminHandler.ListUserKeys)
			userGroup.POST("/keys", adminHandler.CreateUserKey)
			userGroup.DELETE("/keys/:id", adminHandler.DeleteUserKey)
			userGroup.POST("/password", adminHandler.ChangePassword)
		}

		admin := r.Group("/api/v1/admin")
		{
			// Protected Admin API group
			protected := admin.Group("")
			protected.Use(adminHandler.AdminAuthMiddleware())
			{
				// API Keys (scoped to owner for non-admins)
				protected.GET("/keys", adminHandler.ListAPIKeys)
				protected.POST("/keys", adminHandler.CreateAPIKey)
				protected.PUT("/keys/:id", adminHandler.UpdateAPIKey)
				protected.DELETE("/keys/:id", adminHandler.DeleteAPIKey)
				protected.POST("/keys/batch-delete", adminHandler.BatchDeleteAPIKeys)
				protected.POST("/keys/batch-status", adminHandler.BatchStatusAPIKeys)

				// Read-only inspection endpoints
				protected.GET("/stats/overview", adminHandler.GetStatsOverview)
				protected.GET("/system/middlewares", adminHandler.GetSystemMiddlewares)
				protected.GET("/models", adminHandler.ListModels)
				protected.GET("/models/routes", adminHandler.GetModelRoutes)
				protected.GET("/pricing", adminHandler.GetPricingRates)
				protected.GET("/skills", adminHandler.ListSkills)
				protected.GET("/skills/:id/download", adminHandler.DownloadSkillZip)
				protected.GET("/storage/status", adminHandler.GetStorageStatus)
				protected.GET("/mcp/settings", adminHandler.GetMCPSettings)
				protected.GET("/mcp/servers", adminHandler.ListMCPServers)
				protected.GET("/logs", adminHandler.ListLogs)

				// Protected super-admin operations
				adminOnly := protected.Group("")
				adminOnly.Use(adminHandler.RequireAdminRole())
				{
					adminOnly.GET("/users", adminHandler.ListUsers)
					adminOnly.POST("/users", adminHandler.CreateUser)
					adminOnly.DELETE("/users/:username", adminHandler.DeleteUser)
					adminOnly.POST("/users/:username/password", adminHandler.ResetUserPassword)
					adminOnly.POST("/users/:username/status", adminHandler.UpdateUserStatus)
					adminOnly.POST("/users/:username/balance", adminHandler.UpdateUserBalance)
					adminOnly.POST("/users/:username/group", adminHandler.UpdateUserGroup)
					adminOnly.POST("/users/:username/role", adminHandler.UpdateUserRole)

					adminOnly.GET("/redemptions", adminHandler.ListRedemptions)
					adminOnly.POST("/redemptions/generate", adminHandler.GenerateRedemptions)
					adminOnly.DELETE("/redemptions/:id", adminHandler.DeleteRedemption)

					adminOnly.GET("/channels", adminHandler.ListChannels)
					adminOnly.POST("/channels", adminHandler.CreateChannel)
					adminOnly.PUT("/channels/:id", adminHandler.UpdateChannel)
					adminOnly.DELETE("/channels/:id", adminHandler.DeleteChannel)
					adminOnly.POST("/channels/:id/test", adminHandler.TestChannel)
					adminOnly.POST("/channels/probe", adminHandler.ProbeChannel)
					adminOnly.POST("/channels/batch-delete", adminHandler.BatchDeleteChannels)
					adminOnly.POST("/channels/batch-status", adminHandler.BatchStatusChannels)

					adminOnly.POST("/models/routes", adminHandler.UpdateModelRoute)
					adminOnly.DELETE("/models/routes/:model", adminHandler.DeleteModelRoute)
					adminOnly.POST("/models/routes/batch-delete", adminHandler.BatchDeleteModelRoutes)
					adminOnly.POST("/models/routes/probe", adminHandler.ProbeModelRoute)

					adminOnly.POST("/pricing", adminHandler.SavePricingRate)
					adminOnly.DELETE("/pricing/:model", adminHandler.DeletePricingRate)
					adminOnly.POST("/pricing/batch-delete", adminHandler.BatchDeletePricingRates)

					adminOnly.POST("/skills", adminHandler.SaveSkill)
					adminOnly.DELETE("/skills/:id", adminHandler.DeleteSkill)
					adminOnly.POST("/skills/:id/toggle", adminHandler.ToggleSkill)
					adminOnly.POST("/mcp/settings", adminHandler.UpdateMCPSettings)
					adminOnly.POST("/mcp/toggle", adminHandler.UpdateMCPSettings)
					adminOnly.POST("/mcp/servers", adminHandler.SaveMCPServer)
					adminOnly.POST("/mcp/servers/:id/toggle", adminHandler.ToggleMCPServer)
					adminOnly.DELETE("/mcp/servers/:id", adminHandler.DeleteMCPServer)
					adminOnly.POST("/mcp/servers/:id/probe", adminHandler.ProbeMCPServer)

					adminOnly.DELETE("/logs/:id", adminHandler.DeleteLog)
					adminOnly.POST("/logs/batch-delete", adminHandler.BatchDeleteLogs)
					adminOnly.POST("/logs/clear", adminHandler.ClearLogs)
				}
			}
		}
	}

	// OpenAI & Anthropic v1 Data Plane API group
	v1 := r.Group("/v1")
	v1.Use(middleware.AuthMiddleware())
	v1.Use(middleware.RateLimitMiddleware())
	{
		// OpenAI ingress (Chat completions + Text completions + Responses + Models + Embeddings + Rerank + Moderations)
		v1.POST("/chat/completions", handler.HandleChatCompletions)
		v1.POST("/completions", handler.HandleCompletions)
		v1.POST("/responses", handler.HandleResponses)
		v1.GET("/models", handler.HandleModels)
		v1.GET("/models/:model", handler.HandleModelDetail)
		v1.POST("/moderations", handler.HandleModerations)
		v1.POST("/embeddings", handler.HandleEmbeddings)
		v1.POST("/rerank", handler.HandleRerank)

		// Anthropic Claude Messages API ingress + Token Counting
		v1.POST("/messages", handler.HandleAnthropicMessages)
		v1.POST("/messages/count_tokens", handler.HandleAnthropicCountTokens)

		// Multimodal Ingress (Image, Audio TTS/STT/Translation, Video)
		mmHandler := NewMultimodalHandler(dispatcher)
		v1.POST("/images/generations", mmHandler.HandleImageGenerations)
		v1.POST("/audio/speech", mmHandler.HandleAudioSpeech)
		v1.POST("/audio/transcriptions", mmHandler.HandleAudioTranscriptions)
		v1.POST("/audio/translations", mmHandler.HandleAudioTranslations)
		v1.POST("/videos/generations", mmHandler.HandleVideoGenerations)
		v1.GET("/videos/tasks/:id", mmHandler.HandleVideoTask)
	}

	// Google Gemini v1beta Ingress group (protected by unified Auth & RateLimit middlewares)
	v1beta := r.Group("/v1beta")
	v1beta.Use(middleware.AuthMiddleware())
	v1beta.Use(middleware.RateLimitMiddleware())
	{
		v1beta.GET("/models", handler.HandleGeminiModels)
		v1beta.GET("/models/*modelAction", handler.HandleGeminiModelDetail)
		v1beta.POST("/models/*modelAction", handler.HandleGeminiAction)
	}

	return r
}
