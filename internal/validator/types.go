package validator

const (
	LevelOff     = "off"
	LevelLenient = "lenient"
	LevelStrict  = "strict"

	ProtocolOpenAI    = "openai"
	ProtocolAnthropic = "anthropic"
	ProtocolGemini    = "gemini"
)

// ValidationResult carries the outcome of a validation run.
type ValidationResult struct {
	Level    string           `json:"level"`
	Valid    bool             `json:"valid"`
	Warnings []string         `json:"warnings,omitempty"`
	Error    *ValidationError `json:"error,omitempty"`
}

// AddWarning appends a warning message to the result.
func (r *ValidationResult) AddWarning(msg string) {
	if r == nil {
		return
	}
	r.Warnings = append(r.Warnings, msg)
}
