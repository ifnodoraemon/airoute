package api

import (
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/controlplane"
	"github.com/ifnodoraemon/nano-gateway/internal/middleware"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"github.com/ifnodoraemon/nano-gateway/web"
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

	// Mount embedded Web UI
	web.RegisterStaticRoutes(r)

	handler := NewHandler(dispatcher)

	// Public health and observability endpoints
	r.GET("/health", handler.HandleHealth)
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
		}

		// Public Payment Webhooks
		r.POST("/api/v1/public/stripe/webhook", adminHandler.StripeWebhook)

		// Regular User Self-Service APIs
		userGroup := r.Group("/api/v1/user")
		userGroup.Use(adminHandler.AdminAuthMiddleware())
		{
			userGroup.GET("/wallet", adminHandler.GetUserWallet)
			userGroup.POST("/wallet/redeem", adminHandler.RedeemWalletCode)
			userGroup.POST("/wallet/recharge/stripe/session", adminHandler.CreateStripeRechargeSession)
			userGroup.POST("/wallet/recharge/sandbox", adminHandler.SandboxRecharge)
			userGroup.GET("/wallet/orders", adminHandler.ListUserRechargeOrders)
			userGroup.GET("/keys", adminHandler.ListUserKeys)
			userGroup.POST("/keys", adminHandler.CreateUserKey)
			userGroup.DELETE("/keys/:id", adminHandler.DeleteUserKey)
		}

		admin := r.Group("/api/v1/admin")
		{
			// Public Auth Endpoint (backward compatibility)
			admin.POST("/auth/login", adminHandler.Login)

			// Protected Admin API group
			protected := admin.Group("")
			protected.Use(adminHandler.AdminAuthMiddleware())
			{
				protected.GET("/auth/me", adminHandler.GetMe)
				protected.POST("/auth/password", adminHandler.ChangePassword)
				protected.GET("/users", adminHandler.ListUsers)
				protected.POST("/users", adminHandler.CreateUser)
				protected.DELETE("/users/:username", adminHandler.DeleteUser)
				protected.POST("/users/:username/password", adminHandler.ResetUserPassword)
				protected.POST("/users/:username/status", adminHandler.UpdateUserStatus)
				protected.POST("/users/:username/balance", adminHandler.UpdateUserBalance)
				protected.POST("/users/:username/group", adminHandler.UpdateUserGroup)

				protected.GET("/redemptions", adminHandler.ListRedemptions)
				protected.POST("/redemptions/generate", adminHandler.GenerateRedemptions)
				protected.DELETE("/redemptions/:id", adminHandler.DeleteRedemption)

				protected.GET("/channels", adminHandler.ListChannels)
				protected.POST("/channels", adminHandler.CreateChannel)
				protected.PUT("/channels/:id", adminHandler.UpdateChannel)
				protected.DELETE("/channels/:id", adminHandler.DeleteChannel)
				protected.POST("/channels/:id/test", adminHandler.TestChannel)
				protected.POST("/channels/probe", adminHandler.ProbeChannel)
				protected.POST("/channels/batch-delete", adminHandler.BatchDeleteChannels)
				protected.POST("/channels/batch-status", adminHandler.BatchStatusChannels)

				protected.GET("/keys", adminHandler.ListVirtualKeys)
				protected.POST("/keys", adminHandler.CreateVirtualKey)
				protected.PUT("/keys/:id", adminHandler.UpdateVirtualKey)
				protected.DELETE("/keys/:id", adminHandler.DeleteVirtualKey)
				protected.POST("/keys/batch-delete", adminHandler.BatchDeleteVirtualKeys)
				protected.POST("/keys/batch-status", adminHandler.BatchStatusVirtualKeys)

				protected.GET("/stats/overview", adminHandler.GetStatsOverview)
				protected.GET("/models", adminHandler.ListModels)
				protected.GET("/models/routes", adminHandler.GetModelRoutes)
				protected.POST("/models/routes", adminHandler.UpdateModelRoute)
				protected.DELETE("/models/routes/:model", adminHandler.DeleteModelRoute)
				protected.POST("/models/routes/batch-delete", adminHandler.BatchDeleteModelRoutes)
				protected.POST("/models/routes/probe", adminHandler.ProbeModelRoute)

				protected.GET("/pricing", adminHandler.GetPricingRates)
				protected.POST("/pricing", adminHandler.SavePricingRate)
				protected.DELETE("/pricing/:model", adminHandler.DeletePricingRate)
				protected.POST("/pricing/batch-delete", adminHandler.BatchDeletePricingRates)

				protected.GET("/skills", adminHandler.ListSkills)
				protected.POST("/skills/:id/toggle", adminHandler.ToggleSkill)
				protected.GET("/mcp/settings", adminHandler.GetMCPSettings)
				protected.POST("/mcp/settings", adminHandler.UpdateMCPSettings)

				protected.GET("/logs", adminHandler.ListLogs)
				protected.DELETE("/logs/:id", adminHandler.DeleteLog)
				protected.POST("/logs/batch-delete", adminHandler.BatchDeleteLogs)
				protected.POST("/logs/clear", adminHandler.ClearLogs)
			}
		}
	}

	// OpenAI & Anthropic v1 Data Plane API group
	v1 := r.Group("/v1")
	v1.Use(middleware.AuthMiddleware())
	v1.Use(middleware.RateLimitMiddleware())
	{
		// OpenAI ingress (Chat completions + Responses + Text completions + Models + Embeddings + Rerank + Moderations)
		v1.POST("/chat/completions", handler.HandleChatCompletions)
		v1.POST("/responses", handler.HandleResponses)
		v1.POST("/completions", handler.HandleCompletions)
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

	// Google Gemini v1beta Ingress group
	v1beta := r.Group("/v1beta")
	{
		v1beta.GET("/models", handler.HandleGeminiModels)
		v1beta.GET("/models/*modelAction", handler.HandleGeminiModelDetail)
		v1beta.POST("/models/*modelAction", handler.HandleGeminiAction)
	}

	return r
}
