package controlplane

import (
	"fmt"
	"strings"
)

// Format-validation levels a key may carry. Empty means "follow the gateway
// default" (configs/config.yaml model_validation.default_level), which is what the
// resolver in internal/config already does with an empty key-level value.
const (
	formatValidationFollowGlobal = ""
	formatValidationOff          = "off"
	formatValidationLenient      = "lenient"
	formatValidationStrict       = "strict"
)

// normalizeFormatValidation validates and canonicalises a key's format-validation
// level.
//
// The field travelled from request to storage to response without anything
// reading or checking it, so any string was accepted and silently persisted: a
// typo such as "stirct" would look configured while the resolver ignored it. The
// accepted set is exactly what internal/config.ResolveValidationLevel understands.
func normalizeFormatValidation(value string) (string, error) {
	switch level := strings.ToLower(strings.TrimSpace(value)); level {
	case formatValidationFollowGlobal, formatValidationOff, formatValidationLenient, formatValidationStrict:
		return level, nil
	default:
		return "", &ValidationError{Message: fmt.Sprintf(
			"format_validation 只能为 \"\" (跟随全局)、\"off\"、\"lenient\" 或 \"strict\"，收到 %q", value)}
	}
}
