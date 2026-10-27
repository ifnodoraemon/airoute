package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// handleChat provides terminal chat interactions (interactive REPL and one-shot).
// It returns a process exit code so a failed completion is visible to scripts.
func handleChat(endpoint, token string, args []string) int {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	modelFlag := fs.String("m", "deepseek-chat", "指定对话模型标识 (默认: deepseek-chat)")
	sessionFlag := fs.String("s", "", "指定会话标识 (启用会话亲和与前缀缓存命中)")
	_ = fs.Parse(args)

	// The chat command talks to /v1/chat/completions with a virtual API key, not
	// with the admin session used by `keys`/`users`. Without one there is nothing
	// to authenticate with: fail with instructions instead of silently sending a
	// hardcoded key that does not exist on any deployment.
	if strings.TrimSpace(token) == "" {
		fmt.Printf("%s未设置对话认证信息%s: 请设置环境变量 AIROUTE_TOKEN=<API Key>\n"+
			"（控制台 -> API 密钥 中签发；该命令使用 /v1/chat/completions）\n", colorRed, colorReset)
		return cliExitFail
	}

	remaining := fs.Args()
	if len(remaining) > 0 {
		// One-shot prompt
		prompt := strings.Join(remaining, " ")
		return sendOneShotChat(endpoint, token, *modelFlag, *sessionFlag, prompt)
	}

	// Interactive terminal REPL
	fmt.Printf("%s进入 Airoute 终端交互式对话 (模型: %s)%s\n", colorBold+colorCyan, *modelFlag, colorReset)
	fmt.Printf("输入 'exit' 或 'quit' 退出\n\n")

	reader := bufio.NewReader(os.Stdin)
	failed := false
	for {
		fmt.Printf("%s用户 > %s", colorGreen+colorBold, colorReset)
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		prompt := strings.TrimSpace(line)
		if prompt == "" {
			continue
		}
		if prompt == "exit" || prompt == "quit" {
			fmt.Println("退出对话。")
			break
		}

		if sendOneShotChat(endpoint, token, *modelFlag, *sessionFlag, prompt) != cliExitOK {
			failed = true
		}
		fmt.Println()
	}
	if failed {
		return cliExitFail
	}
	return cliExitOK
}

func sendOneShotChat(endpoint, token, modelName, sessionID, prompt string) int {
	client := &http.Client{Timeout: 90 * time.Second}
	chatReq := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}

	body, _ := json.Marshal(chatReq)
	req, _ := http.NewRequest(http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("X-Session-ID", sessionID)
	}
	req.Header.Set("Authorization", getAuthHeader(token))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s请求失败: %v%s\n", colorRed, err, colorReset)
		return cliExitFail
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		fmt.Printf("%s调用错误 (HTTP %d): %s%s\n", colorRed, resp.StatusCode, strings.TrimSpace(string(body)), colorReset)
		return cliExitFail
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil || len(chatResp.Choices) == 0 {
		fmt.Printf("%s未收到模型回复选择%s\n", colorYellow, colorReset)
		return cliExitFail
	}
	fmt.Printf("%sAiroute [%s]:%s\n%s\n", colorBold+colorBlue, modelName, colorReset, chatResp.Choices[0].Message.Content)
	return cliExitOK
}
