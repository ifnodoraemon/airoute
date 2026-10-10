package main

import (
	"fmt"
	"os"
	"strings"
)

// Version is dynamically populated at build time via -ldflags="-X main.Version=vX.Y.Z"
var Version = "0.1.0"

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorRed    = "\033[31m"
)

func resolveDefaultCLIEndpoint() string {
	if ep := os.Getenv("AIROUTE_ENDPOINT"); ep != "" {
		return ep
	}
	if pub := os.Getenv("PUBLIC_URL"); pub != "" {
		return pub
	}
	if pub := os.Getenv("GATEWAY_PUBLIC_URL"); pub != "" {
		return pub
	}
	host := os.Getenv("GATEWAY_HOST")
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	port := "8080"
	if p := os.Getenv("GATEWAY_INGRESS_PORT"); p != "" {
		port = p
	} else if p := os.Getenv("GATEWAY_PORT"); p != "" {
		port = p
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func main() {
	if len(os.Args) < 2 {
		// Default action with no arguments: start the gateway server
		runServer(nil)
		return
	}

	command := os.Args[1]

	switch command {
	case "help", "-h", "--help":
		printHelp()
		return
	case "version", "-v", "--version":
		fmt.Printf("Airoute v%s (企业级大模型网关与智能路由器)\n", Version)
		return
	}

	// Flags (e.g. -config, -db) or 'server' / 'start' command directly boot the gateway server
	if strings.HasPrefix(command, "-") {
		runServer(os.Args[1:])
		return
	}
	if command == "server" || command == "start" {
		runServer(os.Args[2:])
		return
	}

	endpoint := resolveDefaultCLIEndpoint()
	token := os.Getenv("AIROUTE_TOKEN")

	switch command {
	case "status":
		handleStatus(endpoint, token)

	case "models":
		handleModels(endpoint, token)

	case "skills":
		handleSkills(endpoint, token, os.Args[2:])

	case "mcp":
		handleMCP(endpoint, token, os.Args[2:])

	case "chat":
		handleChat(endpoint, token, os.Args[2:])

	case "keys":
		handleKeys(endpoint, token, os.Args[2:])

	case "users":
		handleUsers(endpoint, token, os.Args[2:])

	default:
		fmt.Fprintf(os.Stderr, "%s未知命令 '%s'%s\n运行 'airoute help' 查看可用命令\n", colorRed, command, colorReset)
		os.Exit(1)
	}
}
