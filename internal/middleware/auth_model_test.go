package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
)

func TestValidateModelAllowedFailsClosed(t *testing.T) {
	cases := []struct {
		name           string
		value          any
		present, admin bool
		model          string
		allowed        bool
	}{
		{"missing", nil, false, false, "gpt-4.1", false},
		{"nil", nil, true, false, "gpt-4.1", false},
		{"wrong type", "unrestricted", true, false, "gpt-4.1", false},
		{"typed nil", (*model.APIKeyConfig)(nil), true, false, "gpt-4.1", false},
		{"empty allowlist", &model.APIKeyConfig{}, true, false, "gpt-4.1", true},
		{"allowlisted", &model.APIKeyConfig{AllowedModels: []string{"gpt-4.1"}}, true, false, "gpt-4.1", true},
		{"case mismatch", &model.APIKeyConfig{AllowedModels: []string{"gpt-4.1"}}, true, false, "GPT-4.1", false},
		{"not allowlisted", &model.APIKeyConfig{AllowedModels: []string{"gpt-4.1"}}, true, false, "other", false},
		{"verified admin without key", nil, false, true, "other", true},
		{"admin invalid binding", nil, true, true, "other", false},
		{"admin restricted binding", &model.APIKeyConfig{AllowedModels: []string{"gpt-4.1"}}, true, true, "other", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			if tc.present {
				c.Set(ContextKeyAPIKeyConfig, tc.value)
			}
			c.Set(ContextKeyAdminSession, tc.admin)
			require.Equal(t, tc.allowed, ValidateModelAllowed(c, tc.model))
		})
	}
	require.False(t, ValidateModelAllowed(nil, "gpt-4.1"))
}
