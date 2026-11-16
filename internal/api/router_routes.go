package api

import (
	"github.com/gin-gonic/gin"
)

// registerDataPlaneRoutes wires the protocol ingress once and applies it to every
// prefix a client may use: /v1 (the OpenAI/Anthropic convention) and the bare root
// for SDKs configured with a BaseURL that has no /v1 suffix. One list keeps the
// two surfaces from drifting apart.
func registerDataPlaneRoutes(g *gin.RouterGroup, handler *Handler, mmHandler *MultimodalHandler) {
	// OpenAI ingress (Chat completions + Responses + Models + Embeddings + Rerank + Moderations)
	g.POST("/chat/completions", handler.HandleChatCompletions)
	g.POST("/responses", handler.HandleResponses)
	g.GET("/models", handler.HandleModels)
	g.GET("/models/*model", handler.HandleModelDetail)
	g.POST("/moderations", handler.HandleModerations)
	g.POST("/embeddings", handler.HandleEmbeddings)
	g.POST("/rerank", handler.HandleRerank)

	// Anthropic Claude Messages API ingress + Token Counting
	g.POST("/messages", handler.HandleAnthropicMessages)
	g.POST("/messages/count_tokens", handler.HandleAnthropicCountTokens)

	// Multimodal ingress (Image, Audio TTS/STT/Translation, Video)
	g.POST("/images/generations", mmHandler.HandleImageGenerations)
	g.POST("/audio/speech", mmHandler.HandleAudioSpeech)
	g.POST("/audio/transcriptions", mmHandler.HandleAudioTranscriptions)
	g.POST("/audio/translations", mmHandler.HandleAudioTranslations)
	g.POST("/videos/generations", mmHandler.HandleVideoGenerations)
	g.GET("/videos/tasks/:id", mmHandler.HandleVideoTask)
}

// securityHeadersMiddleware applies enterprise CORS and security headers.
// Extracted from SetupRouter to keep the router assembly under the size red line.
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH, HEAD")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, x-api-key, X-API-Key, anthropic-version, x-goog-api-key, x-admin-token, X-Session-ID, X-Request-ID, X-Airoute-Trace-Id, MCP-Protocol-Version, Stripe-Signature, X-Goog-Upload-Protocol, X-Goog-Upload-Command, X-Goog-Upload-Offset, X-Goog-Upload-Header-Content-Type, X-Goog-Upload-Header-Content-Length, Range, If-Match")
		c.Header("Access-Control-Expose-Headers", "Content-Disposition, Content-Length, X-Airoute-Trace-Id, X-Request-Id, X-Airoute-Cost, X-Airoute-Off-Peak, X-Airoute-Off-Peak-Discount, X-Airoute-Cached-Tokens, X-Airoute-Saved-Cost, X-Cache-Status, X-RateLimit-Limit-Requests, Retry-After, X-Goog-Upload-URL, X-Goog-Upload-Status, X-Goog-Upload-Size-Received, Content-Range")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// The console is a self-hosted SPA that loads Inter/JetBrains Mono from
		// Google Fonts and keeps inline style attributes for themed components.
		c.Header("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com data:; "+
				"img-src 'self' data: blob: https:; "+
				"media-src 'self' data: blob: https:; "+
				"connect-src 'self'; "+
				"base-uri 'self'; form-action 'self'; frame-ancestors 'self'")
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// metricsAuthGuard rejects scrapes the access policy does not allow. The
// policy itself lives in metrics_access.go.
func metricsAuthGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if status := metricsDeniedStatus(c); status != 0 {
			c.AbortWithStatus(status)
			return
		}
		c.Next()
	}
}