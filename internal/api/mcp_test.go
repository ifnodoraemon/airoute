package api

import (
	"context"
	"strings"
	"testing"
)

func TestMCPProtocolBuilders(t *testing.T) {
	t.Run("newMCPResultResponse", func(t *testing.T) {
		resp := newMCPResultResponse("123", map[string]string{"status": "ok"}, map[string]string{"ver": "1"})
		if resp.JSONRPC != "2.0" || resp.ID != "123" {
			t.Fatalf("unexpected response structure: %+v", resp)
		}
		if resp.Error != nil {
			t.Fatalf("expected nil error")
		}
		if resp.Meta == nil {
			t.Fatalf("expected non-nil meta")
		}
	})

	t.Run("newMCPErrorResponse", func(t *testing.T) {
		resp := newMCPErrorResponse("456", -32600, "invalid request")
		if resp.JSONRPC != "2.0" || resp.ID != "456" {
			t.Fatalf("unexpected response structure: %+v", resp)
		}
		if resp.Error == nil || resp.Error.Code != -32600 || resp.Error.Message != "invalid request" {
			t.Fatalf("unexpected error payload: %+v", resp.Error)
		}
	})

	t.Run("newMCPToolResponse", func(t *testing.T) {
		resp := newMCPToolResponse("789", "tool executed successfully", false)
		if resp.JSONRPC != "2.0" || resp.ID != "789" {
			t.Fatalf("unexpected response structure: %+v", resp)
		}
		res, ok := resp.Result.(mcpToolResult)
		if !ok || len(res.Content) != 1 || res.Content[0].Text != "tool executed successfully" || res.IsError {
			t.Fatalf("unexpected tool result: %+v", resp.Result)
		}
	})
}

func TestEvalSimpleMath(t *testing.T) {
	tests := []struct {
		expr     string
		expected float64
		hasErr   bool
	}{
		{"1 + 2", 3, false},
		{"2 * 3 + 4", 10, false},
		{"2 * (3 + 4)", 14, false},
		{"10 / 2 - 1", 4, false},
		{"2 ^ 3", 8, false},
		{"(2 + 3) * (4 - 1)", 15, false},
		{"", 0, true},
		{"10 / 0", 0, true},
		{"2 + * 3", 0, true},
	}

	for _, tt := range tests {
		val, err := evalSimpleMath(tt.expr)
		if tt.hasErr {
			if err == nil {
				t.Fatalf("expected error for expr '%s', got val %f", tt.expr, val)
			}
		} else {
			if err != nil {
				t.Fatalf("unexpected error for expr '%s': %v", tt.expr, err)
			}
			if val != tt.expected {
				t.Fatalf("expr '%s': expected %f, got %f", tt.expr, tt.expected, val)
			}
		}
	}
}

func TestMCPSecurityAndUtilityTools(t *testing.T) {
	handler := &MCPHandler{}
	ctx := context.Background()

	t.Run("airoute_data_redact", func(t *testing.T) {
		text := "联系电话: 13812345678, 邮箱: test@example.com, 密钥: sk-1234567890abcdef123456"
		out, isErr, handled := handler.executeSecurityTools(ctx, "airoute_data_redact", map[string]interface{}{
			"text": text,
		})
		if !handled || isErr {
			t.Fatalf("expected handled without error, got isErr=%v, handled=%v, out=%s", isErr, handled, out)
		}
		if strings.Contains(out, "13812345678") {
			t.Fatalf("phone number was not redacted: %s", out)
		}
		if strings.Contains(out, "sk-1234567890abcdef123456") {
			t.Fatalf("api key was not redacted: %s", out)
		}
	})

	t.Run("airoute_sql_security_check critical", func(t *testing.T) {
		out, isErr, handled := handler.executeSecurityTools(ctx, "airoute_sql_security_check", map[string]interface{}{
			"sql": "DELETE FROM users",
		})
		if !handled || isErr {
			t.Fatalf("expected handled without error, got isErr=%v, handled=%v, out=%s", isErr, handled, out)
		}
		if !strings.Contains(out, "CRITICAL") {
			t.Fatalf("expected CRITICAL risk level for DELETE without WHERE, got: %s", out)
		}
	})

	t.Run("nano_calc_eval via utility tools", func(t *testing.T) {
		out, isErr, handled := handler.executeUtilityAndMockTools(ctx, "nano_calc_eval", map[string]interface{}{
			"expression": "100 / 4 + 5",
		})
		if !handled || isErr {
			t.Fatalf("expected handled, got isErr=%v, handled=%v, out=%s", isErr, handled, out)
		}
		if !strings.Contains(out, "30") {
			t.Fatalf("expected result 30 in output: %s", out)
		}
	})
}
