package controlplane

import (
	"fmt"
	"net/http"
	"strings"
	stdSync "sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// ProbeResultItem describes latency and health status for a single channel.
type ProbeResultItem struct {
	ChannelID int64  `json:"channel_id"`
	Name      string `json:"name"`
	LatencyMs int64  `json:"latency_ms"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// ProbeModelRoute tests connectivity and latency for all providers of a specific model in parallel.
func (h *AdminHandler) ProbeModelRoute(c *gin.Context) {
	var req struct {
		Model string `json:"model" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	channels := h.dispatcher.GetChannelsForModel(req.Model)
	results := make([]ProbeResultItem, len(channels))
	var wg stdSync.WaitGroup
	client := &http.Client{Timeout: 4 * time.Second}

	for i, ch := range channels {
		wg.Add(1)
		go func(idx int, c model.ChannelConfig) {
			defer wg.Done()
			start := time.Now()
			probeURL := strings.TrimRight(c.BaseURL, "/") + "/models"
			httpReq, err := http.NewRequest("GET", probeURL, nil)
			if err != nil {
				results[idx] = ProbeResultItem{
					ChannelID: c.ID,
					Name:      c.Name,
					Status:    "error",
					Error:     err.Error(),
				}
				return
			}
			if c.APIKey != "" && c.APIKey != "none" {
				httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
				httpReq.Header.Set("x-api-key", c.APIKey)
			}
			resp, err := client.Do(httpReq)
			lat := time.Since(start).Milliseconds()
			if err != nil {
				results[idx] = ProbeResultItem{
					ChannelID: c.ID,
					Name:      c.Name,
					LatencyMs: lat,
					Status:    "unreachable",
					Error:     err.Error(),
				}
				return
			}
			defer resp.Body.Close()
			status := "healthy"
			if resp.StatusCode >= 400 && resp.StatusCode != 404 && resp.StatusCode != 405 {
				status = fmt.Sprintf("status_%d", resp.StatusCode)
			}
			results[idx] = ProbeResultItem{
				ChannelID: c.ID,
				Name:      c.Name,
				LatencyMs: lat,
				Status:    status,
			}
		}(i, *ch)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": results})
}
