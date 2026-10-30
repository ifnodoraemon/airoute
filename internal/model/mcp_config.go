package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// ParseMCPCommand splits a configured executable and its arguments without a
// shell. Quoted paths and arguments keep their spaces; expansions never run.
func ParseMCPCommand(command string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, char := range command {
		if escaped {
			word.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped, started = true, true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
			continue
		}
		switch {
		case char == '\'' || char == '"':
			quote, started = char, true
		case unicode.IsSpace(char):
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("命令引号或转义符未闭合")
	}
	if started {
		words = append(words, word.String())
	}
	if len(words) == 0 || words[0] == "" {
		return nil, fmt.Errorf("MCP 命令不能为空")
	}
	return words, nil
}

// ParseMCPEnv accepts the JSON string stored with an MCP server configuration.
func ParseMCPEnv(raw string) (map[string]string, error) {
	env := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return env, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("环境变量必须是字符串值的 JSON 对象")
	}
	for key, data := range fields {
		var value string
		if string(data) == "null" {
			return nil, fmt.Errorf("环境变量值必须是字符串")
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, fmt.Errorf("环境变量值必须是字符串")
		}
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("环境变量名称或值无效")
		}
		env[key] = value
	}
	return env, nil
}
