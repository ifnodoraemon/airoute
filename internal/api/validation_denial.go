package api

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/ifnodoraemon/airoute/internal/validator"
)

// rejectInvalidRequest renders a protocol-validation rejection and records why
// the request was refused.
//
// Every ingress handler used to call valRes.Error.WriteGinResponse and return:
// the client got a correct 400, but the usage row and the stdout access line
// carried no error_message, and nothing was logged at all. A validator
// regression was therefore invisible — the gateway answered "invalid payload"
// with no protocol, no field and no rule anywhere an operator could look.
//
// The reason is published on the gin context because both the access-log
// middleware (middleware.ContextKeyErrorMessage) and the audit recorder read it
// from there, then emitted once as a structured WARN keyed by trace_id so a
// rejected request can be followed to its log line.
func rejectInvalidRequest(c *gin.Context, valErr *validator.ValidationError) {
	if c == nil || valErr == nil {
		return
	}
	// The reason is client-influenced text (it can quote an offending value), so
	// it goes through the same scrubber as every other audit-bound message.
	reason := sanitizeErrorMessage(errors.New(valErr.Error()))
	c.Set(middleware.ContextKeyErrorMessage, reason)

	traceID := ""
	if c.Request != nil {
		traceID = telemetry.TraceIDFromContext(c.Request.Context())
	}
	telemetry.ScopedLogger("validation").Warn("request validation rejected",
		"trace_id", traceID,
		"protocol", valErr.Protocol,
		"field", valErr.Field,
		"code", valErr.Code,
		"status_code", valErr.StatusCode,
		"reason", reason,
	)

	valErr.WriteGinResponse(c)
}
