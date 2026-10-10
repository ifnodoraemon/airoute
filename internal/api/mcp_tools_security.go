package api

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// executeSecurityTools executes PII data redaction and SQL injection / destruction guards.
func (h *MCPHandler) executeSecurityTools(_ context.Context, name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case "airoute_data_redact":
		text, _ := args["text"].(string)
		if text == "" {
			return "Error: parameter 'text' is required", true, true
		}

		phoneRegex := regexp.MustCompile(`(?:\+?86)?(1[3-9]\d)(\d{4})(\d{4})`)
		idRegex := regexp.MustCompile(`([1-9]\d{5})(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])(\d{3}[\dXx])`)
		emailRegex := regexp.MustCompile(`([a-zA-Z0-9._%+-]{1,2})([a-zA-Z0-9._%+-]+)(@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)
		keyRegex := regexp.MustCompile(`(sk-[a-zA-Z0-9]{4})([a-zA-Z0-9]{16,})`)
		cardRegex := regexp.MustCompile(`(\b\d{4})\d{8,11}(\d{4}\b)`)

		phoneCount := len(phoneRegex.FindAllString(text, -1))
		idCount := len(idRegex.FindAllString(text, -1))
		emailCount := len(emailRegex.FindAllString(text, -1))
		keyCount := len(keyRegex.FindAllString(text, -1))
		cardCount := len(cardRegex.FindAllString(text, -1))

		redacted := phoneRegex.ReplaceAllString(text, "$1****$3")
		redacted = idRegex.ReplaceAllString(redacted, "$1********$2")
		redacted = emailRegex.ReplaceAllString(redacted, "$1***$3")
		redacted = keyRegex.ReplaceAllString(redacted, "$1****************")
		redacted = cardRegex.ReplaceAllString(redacted, "$1********$2")

		totalDetected := phoneCount + idCount + emailCount + keyCount + cardCount
		verdict := "COMPLIANT_CLEAN"
		if totalDetected > 0 {
			verdict = "COMPLIANT_REDACTED"
		}

		res := gin.H{
			"original_length":    len(text),
			"redacted_text":      redacted,
			"compliance_verdict": verdict,
			"total_redactions":   totalDetected,
			"detected_types": gin.H{
				"phone_numbers": phoneCount,
				"national_ids":  idCount,
				"emails":        emailCount,
				"api_keys":      keyCount,
				"bank_cards":    cardCount,
			},
			"audited_at": time.Now().Format("2006-01-02 15:04:05"),
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "airoute_sql_security_check":
		sql, _ := args["sql"].(string)
		if strings.TrimSpace(sql) == "" {
			sql, _ = args["query"].(string)
		}
		if strings.TrimSpace(sql) == "" {
			return "Error: parameter 'sql' or 'query' is required", true, true
		}
		upper := strings.ToUpper(strings.TrimSpace(sql))

		var issues []string
		riskLevel := "SAFE"

		if (strings.Contains(upper, "DELETE") || strings.Contains(upper, "UPDATE")) && !strings.Contains(upper, "WHERE") {
			issues = append(issues, "致命风险：DELETE 或 UPDATE 操作缺少 WHERE 条件，将导致整表全量数据被擦除或篡改！")
			riskLevel = "CRITICAL"
		}
		if strings.Contains(upper, "DROP TABLE") || strings.Contains(upper, "DROP DATABASE") || strings.Contains(upper, "TRUNCATE") {
			issues = append(issues, "高危拦截：检测到不可逆的 DDL 结构破坏性指令 (DROP / TRUNCATE)！")
			riskLevel = "CRITICAL"
		}
		if strings.Contains(upper, "XP_CMDSHELL") || strings.Contains(upper, "EXEC(") || strings.Contains(upper, "INTO OUTFILE") {
			issues = append(issues, "高危拦截：检测到潜在命令执行或文件外泄注入特征！")
			riskLevel = "CRITICAL"
		}
		if strings.Contains(upper, "' OR '1'='1") || strings.Contains(upper, "' OR 1=1") || strings.Contains(upper, "UNION SELECT") {
			issues = append(issues, "高危拦截：检测到经典恒真条件 SQL 注入或联合查询绕过特征！")
			riskLevel = "CRITICAL"
		}

		if strings.HasPrefix(upper, "SELECT") && strings.Contains(upper, "*") && !strings.Contains(upper, "LIMIT") && !strings.Contains(upper, "WHERE") {
			issues = append(issues, "中度警告：SELECT * 全表查询未声明 WHERE 过滤或 LIMIT 截断，可能引发海量数据读取导致内存溢出。")
			if riskLevel == "SAFE" {
				riskLevel = "WARNING"
			}
		}

		rec := "该 SQL 语句未检测到高危安全隐患，可安全提交执行。"
		if riskLevel == "CRITICAL" {
			rec = "强烈建议立即拦截并驳回该 SQL 执行请求！必须增加严格的主键/索引 WHERE 条件或废弃破坏性 DDL。"
		} else if riskLevel == "WARNING" {
			rec = "建议改写为指定列名（避免 SELECT *），并显式追加 LIMIT 限制以防全表扫描慢查询。"
		}

		res := gin.H{
			"analyzed_sql":    sql,
			"risk_level":      riskLevel,
			"safe_to_execute": riskLevel == "SAFE",
			"issues_count":    len(issues),
			"issues":          issues,
			"recommendation":  rec,
			"audited_at":      time.Now().Format("2006-01-02 15:04:05"),
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true
	}

	return "", false, false
}
