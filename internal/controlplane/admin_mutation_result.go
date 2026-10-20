package controlplane

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// A committed write is successful even if applying it failed. Keep that state
// visible without encouraging the caller to repeat an already saved write.
func writeHotReloadWarning(c *gin.Context, err error, payload gin.H) bool {
	var hot *HotReloadError
	if !errors.As(err, &hot) {
		return false
	}
	if payload == nil {
		payload = gin.H{}
	}
	payload["code"] = 0
	payload["warning"] = "配置已保存，但热加载未完全生效（" + hot.Stage + "）：" + hot.Err.Error()
	payload["message"] = "配置已保存，热加载未完全生效"
	c.JSON(http.StatusAccepted, payload)
	return true
}
