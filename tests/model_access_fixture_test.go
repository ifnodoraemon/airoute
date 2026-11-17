package tests

import (
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// Handler-only protocol fixtures explicitly bind the test key's model policy.
func unrestrictedTestModels(c *gin.Context) {
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{})
	c.Next()
}
