package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
)

func printHelp() {
	fmt.Printf(`%sAiroute%s - 企业级高性能大模型智能路由器与网关 (AI Gateway & Router) v%s

%s使用方法 (Usage):%s
  airoute [flags]                    启动网关 HTTP 服务 (默认模式)
  airoute server [flags]             启动网关 HTTP 服务 (显式模式)
  airoute status                     查看网关健康状态、节点信息与延迟
  airoute models                     查询所有已接入的模型拓扑与渠道状态
  airoute chat [flags] <message>     在终端直接与大模型对话交互
  airoute keys [subcommand]          管理 API Key (list, create)
  airoute users [subcommand]         管理系统用户账号与权限 (list, create, delete, passwd)
  airoute skills [subcommand]        管理 MCP 扩展技能 (list, enable, disable)
  airoute mcp [subcommand]           MCP 本地客户端接入与 stdio 桥接
  airoute version                    查看版本信息
  airoute help                       查看帮助信息

%s网关服务常用参数 (Server Flags):%s
  -config <path>    YAML 配置文件路径 (默认: configs/config.yaml 或 $GATEWAY_CONFIG)
  -db <path>        SQLite 数据库路径 (默认: data/gateway.db 或 $GATEWAY_DB_DSN)

%s环境变量 (Environment Variables):%s
  AIROUTE_ENDPOINT  CLI 连接的网关地址 (默认: 动态解析或 http://$GATEWAY_HOST:$GATEWAY_PORT)
  AIROUTE_TOKEN     CLI 认证 Token
  DATABASE_URL      PostgreSQL 连接串 (配置后自动激活集群分布式存储)
  REDIS_URL         Redis 连接串 (配置后自动激活多节点分布式缓存与热更新)

%s示例 (Examples):%s
  # 1. 启动网关服务
  airoute -config configs/config.yaml

  # 2. 检查集群连通性
  airoute status

  # 3. 终端极速对话
  airoute chat -m deepseek-chat "你好，请自我介绍"
`,
		colorBold+colorCyan, colorReset, Version,
		colorBold, colorReset,
		colorBold, colorReset,
		colorBold, colorReset,
		colorBold, colorReset,
	)
}

func getAuthHeader(token string) string {
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer ") {
		return token
	}
	return "Bearer " + token
}

func ensureAdminToken(endpoint, token string) string {
	if token != "" {
		return token
	}
	cfg := config.GetGlobalConfig()
	adminUser := cfg.GetAdminUsername()
	adminPass := cfg.GetAdminPassword()

	// Attempt auto-login with configured admin credentials if no token provided
	loginPayload, _ := json.Marshal(map[string]string{
		"username": adminUser,
		"password": adminPass,
	})
	resp, err := http.Post(endpoint+"/api/v1/auth/login", "application/json", bytes.NewReader(loginPayload))
	if err == nil {
		defer resp.Body.Close()
		var res struct {
			Code int `json:"code"`
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Code == 0 && res.Data.Token != "" {
			return res.Data.Token
		}
	}
	return ""
}
