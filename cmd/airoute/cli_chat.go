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
func handleChat(endpoint, token string, args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	modelFlag := fs.String("m", "deepseek-chat", "指定对话模型标识 (默认: deepseek-chat)")
	sessionFlag := fs.String("s", "", "指定会话标识 (启用会话亲和与前缀缓存命中)")
	_ = fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) > 0 {
		// One-shot prompt
		prompt := strings.Join(remaining, " ")
		sendOneShotChat(endpoint, token, *modelFlag, *sessionFlag, prompt)
		return
	}

	// Interactive terminal REPL
	fmt.Printf("%s进入 Airoute 终端交互式对话 (模型: %s)%s\n", colorBold+colorCyan, *modelFlag, colorReset)
	fmt.Printf("输入 'exit' 或 'quit' 退出\n\n")

	reader := bufio.NewReader(os.Stdin)
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

		sendOneShotChat(endpoint, token, *modelFlag, *sessionFlag, prompt)
		fmt.Println()
	}
}

func sendOneShotChat(endpoint, token, modelName, sessionID, prompt string) {
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
	if token != "" {
		req.Header.Set("Authorization", getAuthHeader(token))
	} else {
		// Use default test key
		req.Header.Set("Authorization", "Bearer sk-airoute-production-enterprise-cluster-key")
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s请求失败: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s调用错误 (HTTP %d): %s%s\n", colorRed, resp.StatusCode, string(body), colorReset)
		return
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

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err == nil && len(chatResp.Choices) > 0 {
		fmt.Printf("%sAiroute [%s]:%s\n%s\n", colorBold+colorBlue, modelName, colorReset, chatResp.Choices[0].Message.Content)
	} else {
		fmt.Printf("%s未收到模型回复选择%s\n", colorYellow, colorReset)
	}
}
