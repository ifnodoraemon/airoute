package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func setUpstreamRetryAfter(c *gin.Context, err error) {
	if c == nil {
		return
	}
	if upstream, ok := provider.AsUpstreamError(err); ok && upstream.Status == http.StatusTooManyRequests {
		if wait := provider.RetryDelay(upstream.Headers, time.Now()); wait > 0 {
			c.Header("Retry-After", strconv.Itoa(max(1, int((wait+time.Second-1)/time.Second))))
		}
	}
}
