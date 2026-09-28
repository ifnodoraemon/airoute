package validator

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ValidationError represents a protocol-specific request validation failure.
type ValidationError struct {
	Protocol   string `json:"protocol"`
	Field      string `json:"field,omitempty"`
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
	StatusCode int    `json:"status_code"`
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Field != "" {
		return fmt.Sprintf("[%s] invalid %s: %s", e.Protocol, e.Field, e.Message)
	}
	return fmt.Sprintf("[%s] %s", e.Protocol, e.Message)
}

// WriteGinResponse renders the error in the native format expected by clients of the target protocol.
func (e *ValidationError) WriteGinResponse(c *gin.Context) {
	if e == nil {
		return
	}
	statusCode := e.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusBadRequest
	}

	switch e.Protocol {
	case ProtocolAnthropic:
		// Anthropic Messages API standard error format
		c.JSON(statusCode, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": e.Message,
			},
		})
	case ProtocolGemini:
		// Google Gemini GenerativeLanguage API standard error format
		statusText := "INVALID_ARGUMENT"
		if statusCode == http.StatusNotFound {
			statusText = "NOT_FOUND"
		} else if statusCode == http.StatusForbidden {
			statusText = "PERMISSION_DENIED"
		}
		c.JSON(statusCode, gin.H{
			"error": gin.H{
				"code":    statusCode,
				"message": e.Message,
				"status":  statusText,
			},
		})
	default:
		// OpenAI standard error format
		errCode := e.Code
		if errCode == "" {
			errCode = "invalid_parameter"
		}
		c.JSON(statusCode, gin.H{
			"error": gin.H{
				"message": e.Message,
				"type":    "invalid_request_error",
				"param":   e.Field,
				"code":    errCode,
			},
		})
	}
}
