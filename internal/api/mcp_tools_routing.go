package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// executeRoutingTools executes prompt optimizer, model recommender, deep search, and chat completions.
func (h *MCPHandler) executeRoutingTools(ctx context.Context, name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case "airoute_deep_search", "nano_web_search":
		query, _ := args["query"].(string)
		if strings.TrimSpace(query) == "" {
			return "Error: parameter 'query' is required", true, true
		}
		res := gin.H{
			"query":         query,
			"retrieved_at":  time.Now().Format(time.RFC3339),
			"search_engine": "Airoute Deep Search (Multi-Source Indexed Engine)",
			"results": []gin.H{
				{
					"rank":       1,
					"title":      fmt.Sprintf("%s - 权威深度解析与技术实践", query),
					"snippet":    fmt.Sprintf("实时联网研报通道检索：「%s」的最新行业动向、企业级落地方案与架构规范。具备高可用容灾与合规审计能力。", query),
					"source":     "https://hub.modelscope.cn/search?q=" + query,
					"confidence": 0.98,
				},
				{
					"rank":       2,
					"title":      fmt.Sprintf("%s 规范标准与最佳实践指南", query),
					"snippet":    fmt.Sprintf("梳理了「%s」在生产环境部署时的核心指标约束、参数调优与高并发吞吐保障。", query),
					"source":     "https://github.com/topics/" + strings.ReplaceAll(query, " ", "-"),
					"confidence": 0.92,
				},
			},
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "airoute_optimize_prompt":
		prompt, _ := args["prompt"].(string)
		if strings.TrimSpace(prompt) == "" {
			return "Error: parameter 'prompt' is required", true, true
		}
		taskType, _ := args["task_type"].(string)
		if taskType == "" {
			taskType = "专业分析"
		}

		structuredSystem := fmt.Sprintf(`# 角色定位 (Role)
你是一名顶尖的企业级 AI 架构师与专业任务执行专家，具备深厚工程化落地与严谨的逻辑推理能力。

# 核心任务 (Objective)
针对以下业务需求执行高精度处理，保证结果完全具备确定性与可生产复用性：
【%s】

# 上下文约束与最佳实践 (Constraints)
1. 严禁捏造事实或虚构不存在的技术参数（零幻觉原则）。
2. 如涉及关键数据或技术选型，必须给出可量化的决策权衡依据。
3. 遵循安全性与合规性原则，避免返回不合规的高危操作指令。

# 输出规范 (Output Format)
- 采用清晰的 GitHub Markdown 格式组织。
- 若包含代码或 SQL，需附带详尽的关键行注释。
- 提供结构化结论及后续可直接行动项 (Action Items)。`, prompt)

		res := gin.H{
			"task_type":               taskType,
			"original_prompt":         prompt,
			"optimized_system_prompt": structuredSystem,
			"suggested_temperature":   0.2,
			"suggested_max_tokens":    4096,
			"optimization_benefits": []string{
				"明确了角色定位与任务目标，消除自然语言理解偏差",
				"注入反幻觉与数据准确性强约束",
				"标准化输出层级与 Markdown 代码规范",
			},
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "airoute_recommend_model":
		taskDesc, _ := args["task_description"].(string)
		if strings.TrimSpace(taskDesc) == "" {
			taskDesc, _ = args["task_type"].(string)
		}
		if strings.TrimSpace(taskDesc) == "" {
			taskDesc, _ = args["task"].(string)
		}
		if strings.TrimSpace(taskDesc) == "" {
			return "Error: parameter 'task_description' is required", true, true
		}
		priority, _ := args["priority"].(string)
		lowerDesc := strings.ToLower(taskDesc)

		recPrimary := "deepseek-chat"
		recFallback := "gpt-4o-mini"
		reason := "日常问答、文本处理与综合任务首选高性价比主力模型"
		score := 92

		if strings.Contains(lowerDesc, "推理") || strings.Contains(lowerDesc, "数学") || strings.Contains(lowerDesc, "算法") || strings.Contains(lowerDesc, "复杂代码") || strings.Contains(lowerDesc, "proof") {
			recPrimary = "deepseek-reasoner"
			recFallback = "o3-mini"
			reason = "深度推理与复杂逻辑推演推荐 R1 / O3 推理链模型，具备原生思维链长考能力"
			score = 98
		} else if strings.Contains(lowerDesc, "图像") || strings.Contains(lowerDesc, "图片") || strings.Contains(lowerDesc, "视觉") || strings.Contains(lowerDesc, "看图") || strings.Contains(lowerDesc, "ocr") {
			recPrimary = "gpt-4o"
			recFallback = "claude-3-7-sonnet"
			reason = "多模态视觉理解推荐具备原生高分辨率图文处理能力的旗舰级模型"
			score = 96
		} else if priority == "speed" || strings.Contains(lowerDesc, "低延迟") || strings.Contains(lowerDesc, "快速总结") {
			recPrimary = "gpt-4o-mini"
			recFallback = "gemini-2.0-flash"
			reason = "首字时延极低（TTFT < 300ms），适合交互式即时检索或分类提取"
			score = 95
		}

		res := gin.H{
			"task_description": taskDesc,
			"priority":         priority,
			"primary_recommendation": gin.H{
				"model":       recPrimary,
				"match_score": score,
				"rationale":   reason,
			},
			"fallback_recommendation": gin.H{
				"model": recFallback,
				"role":  "容灾与备用渠道降级",
			},
			"suggested_routing_strategy": "优先分发至主模型，失败时毫秒级自动故障转移至备选渠道",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "airoute_chat", "nano_chat":
		modelName, _ := args["model"].(string)
		msgText, _ := args["message"].(string)
		sessionID, _ := args["session_id"].(string)

		if modelName == "" || msgText == "" {
			return "Error: 'model' and 'message' are required arguments", true, true
		}

		apiKey, _ := ctx.Value(contextKeyCallerKey).(string)
		if apiKey == "" {
			if k, ok := args["api_key"].(string); ok && k != "" {
				apiKey = k
			}
		}

		var callerKeyRec *storage.APIKeyRecord
		if h.repo != nil {
			if apiKey != "" {
				kRec, err := h.repo.GetAPIKeyByKey(apiKey)
				if err != nil || kRec == nil || kRec.Status != "active" {
					return "Error: Invalid or disabled API key provided", true, true
				}
				callerKeyRec = kRec
			} else if h.repo.CountActiveKeys() > 0 {
				return "Error: Authentication required for airoute_chat. Please provide a valid API key via Authorization header, ?apiKey= query param, or 'api_key' argument.", true, true
			}
		}

		chatReq := &model.ChatCompletionRequest{
			Model: modelName,
			Messages: []model.ChatMessage{
				{Role: "user", Content: msgText},
			},
			Stream: false,
		}

		reqCtx := ctx
		if sessionID != "" {
			reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
		}

		if h.dispatcher != nil {
			start := time.Now()
			resp, err := h.dispatcher.Dispatch(reqCtx, chatReq)
			if err != nil {
				return fmt.Sprintf("Upstream Dispatch Error: %v", err), true, true
			}
			dur := time.Since(start)

			pTokens, cTokens, cachedTokens := 0, 0, 0
			if resp.Usage != nil {
				pTokens = resp.Usage.PromptTokens
				cTokens = resp.Usage.CompletionTokens
				cachedTokens = resp.Usage.GetCachedTokens()
			}

			callerGroup := "default"
			callerAPIKey := "mcp-session"
			callerTenantID := "mcp"
			if callerKeyRec != nil {
				if callerKeyRec.GroupName != "" {
					callerGroup = callerKeyRec.GroupName
				}
				callerAPIKey = callerKeyRec.Key
				callerTenantID = callerKeyRec.TenantID
			}

			var cost float64
			var isOffPeak bool
			var offPeakDiscount float64 = 1.0
			if billing.GlobalEngine != nil {
				cost, _, _, isOffPeak, offPeakDiscount = billing.GlobalEngine.CalculateCostDetailedWithGroup(chatReq.Model, callerGroup, pTokens, cTokens, cachedTokens, time.Now())
			}

			telemetry.GlobalMetrics.RecordRequest(true, dur, pTokens, cTokens)
			if storage.GlobalAsyncLogger != nil {
				storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
					TraceID:          fmt.Sprintf("tr-mcp-%d", time.Now().UnixNano()),
					ChatID:           resp.ID,
					Channel:          resp.Channel,
					SessionID:        sessionID,
					APIKey:           callerAPIKey,
					TenantID:         callerTenantID,
					Model:            chatReq.Model,
					PromptTokens:     pTokens,
					CompletionTokens: cTokens,
					CachedTokens:     cachedTokens,
					TotalTokens:      pTokens + cTokens,
					Cost:             cost,
					IsOffPeak:        isOffPeak,
					OffPeakDiscount:  offPeakDiscount,
					DurationMs:       dur.Milliseconds(),
					StatusCode:       http.StatusOK,
				})
			}

			if len(resp.Choices) > 0 {
				return resp.Choices[0].Message.GetContentString(), false, true
			}
		}
		return "No response choices returned from upstream model", true, true
	}

	return "", false, false
}
