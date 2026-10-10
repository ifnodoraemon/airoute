package storage

import (
	"encoding/json"
)

// SeedDefaultSkills ensures standard open Agent Skills exist, complying with agentskills.io standard.
func (r *Repository) SeedDefaultSkills() error {
	// Clean up legacy skills to keep it clean and focused
	_, _ = r.db.Exec(`DELETE FROM system_skills WHERE id NOT IN ('git-workflow', 'test-driven-development', 'browser-automation', 'security-audit') AND id NOT LIKE 'skill_%' AND id NOT LIKE 'custom_%'`)

	defaults := []SkillRecord{
		{
			ID:          "git-workflow",
			Name:        "Git 规范协作与代码审查工作流",
			Description: "标准化 Git 分支管理、Conventional Commits 提交规范、冲突解决与 GitHub PR 审查流",
			Category:    "dev",
			Tools:       []string{"run_command", "git", "view_file"},
			LoadingMode: "lazy",
			Author:      "Community Standard",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: git-workflow
description: 标准化 Git 分支管理、Conventional Commits 提交规范、冲突解决与 GitHub PR 审查流。当用户需要提交代码、排查冲突、发起或审查 Pull Request 时自动激活。
category: dev
author: Community Standard
version: 1.0.0
loading_mode: lazy
allowed-tools:
  - run_command
  - git
  - view_file
---

# Git 协作与代码审查标准作业程序 (git-workflow)

规范智能体在工程协同中的 Git 操作流程，杜绝杂乱提交与破坏性操作。

## 适用场景
- 用户要求提交代码变更、整理 commit 历史
- 分支合并、rebase 与代码冲突排障
- 发起或自动化审查 Pull Request

## SOP 执行工作流
1. **工作区状态探查**：执行 run_command("git status") 与 run_command("git diff --stat")，确认所有待提交文件，严格禁止将 .env、临时文件或编译产物加入暂存区。
2. **规范化提交信息**：遵循 Conventional Commits 规范：
   - feat: 新增功能
   - fix: 缺陷修复
   - refactor: 代码重构（不改变外部行为）
   - test: 单元测试与用例补充
   - chore: 依赖更新与配置维护
3. **冲突解决策略**：先拉取远程最新主干，使用 rebase 模式合并，逐个文件对比解决冲突后执行测试验证。
4. **代码审查清单**：审查 PR 时核验改动行数、边界保护、向下兼容性与测试覆盖。
`,
		},
		{
			ID:          "test-driven-development",
			Name:        "TDD 测试驱动开发与缺陷排查",
			Description: "红-绿-重构闭环（Red-Green-Refactor）、单元测试用例构造、边界条件防御与防回归验证",
			Category:    "test",
			Tools:       []string{"run_command", "view_file", "replace_file_content"},
			LoadingMode: "lazy",
			Author:      "Kent Beck / Community",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: test-driven-development
description: 严谨的测试驱动开发（TDD）与质量保障规范。通过“先写失败测试、最小实现、安全重构”确保代码正确性与可维护性。当用户要求实现新功能、修复 Bug 或增加单元测试时激活。
category: test
author: Kent Beck / Community
version: 1.0.0
loading_mode: lazy
allowed-tools:
  - run_command
  - view_file
  - replace_file_content
---

# TDD 测试驱动开发标准作业程序 (test-driven-development)

确保代码正确性、防止 AI 盲目修改与消除回归缺陷的黄金法则。

## 核心法则
1. **先写断言测试**：在修改或新增业务逻辑前，必须先在对应 *_test.go 文件中编写重现缺陷或覆盖新功能的失败用例。
2. **验证测试失败**：运行测试命令确认新测试确实按预期失败（证明测试具备探测力）。
3. **最小修改原则**：仅修改足以使测试通过的最少代码，禁止过度设计。
4. **验证测试通过**：重新运行测试用例确认其变为绿色（PASS）。
5. **安全整洁重构**：消除坏味道、解耦过长函数，重构过程中持续运行测试确保无行为劣化。
`,
		},
		{
			ID:          "browser-automation",
			Name:        "浏览器无头自动化与页面端到端测试",
			Description: "基于 Puppeteer/Playwright 的现代 Web 页面渲染、表单交互模拟与 DOM 断言",
			Category:    "dev",
			Tools:       []string{"puppeteer_navigate", "puppeteer_screenshot", "puppeteer_click", "puppeteer_fill", "puppeteer_evaluate"},
			LoadingMode: "lazy",
			Author:      "Browser Automation Team",
			Version:     "1.1.0",
			Enabled:     true,
			Manifest: `---
name: browser-automation
description: 浏览器端到端交互、单页应用（SPA）渲染排查、UI 自动化测试与页面状态截图诊断。当用户要求自动化操作网页或排障前端界面时激活。
category: dev
author: Browser Automation Team
version: 1.1.0
loading_mode: lazy
allowed-tools:
  - puppeteer_navigate
  - puppeteer_screenshot
  - puppeteer_click
  - puppeteer_fill
  - puppeteer_evaluate
---

# 浏览器自动化标准作业程序 (browser-automation)

## 规范步骤
1. **导航与页面加载等待**：使用 puppeteer_navigate 访问目标 URL，等待网络空闲或关键 DOM 锚点就绪。
2. **状态检查与取证**：遇到非预期渲染时，第一时间调用 puppeteer_screenshot 保存全景图或视口快照。
3. **安全交互执行**：交互前检查元素可见性，避免对未挂载的虚拟 DOM 节点盲目触发点击。
4. **结果提取**：通过 puppeteer_evaluate 在页面沙箱内执行提取函数，结构化回传结果数据。
`,
		},
		{
			ID:          "security-audit",
			Name:        "企业级代码安全审计与合规检查",
			Description: "SQL 注入模式检测、硬编码密钥脱敏、SSRF 过滤与鉴权边界防护",
			Category:    "ops",
			Tools:       []string{"airoute_data_redact", "airoute_sql_security_check", "view_file"},
			LoadingMode: "eager",
			Author:      "Security Guild",
			Version:     "2.0.0",
			Enabled:     true,
			Manifest: `---
name: security-audit
description: 全面审查代码库与系统运行时的数据安全漏洞，包括敏感凭据泄露、SQL 注入、越权与未授权访问。
category: ops
author: Security Guild
version: 2.0.0
loading_mode: eager
allowed-tools:
  - airoute_data_redact
  - airoute_sql_security_check
  - view_file
---

# 企业代码安全审计规范 (security-audit)

## 审查重点
1. **凭证隔离**：杜绝在源码中硬编码私钥、API Key、数据库密码，使用环境变量驱动。
2. **入参净化**：所有入参必须经过模式校验或参数化绑定，严禁拼接 SQL 语句。
3. **敏感信息脱敏**：日志输出与异常栈中涉及手机号、身份证、邮箱等 PII 数据必须脱敏处理。
`,
		},
	}

	for _, s := range defaults {
		toolsJSON, _ := json.Marshal(s.Tools)
		enabledInt := 0
		if s.Enabled {
			enabledInt = 1
		}
		_, _ = r.db.Exec(`
			INSERT INTO system_skills (id, name, description, category, tools, loading_mode, manifest, author, version, enabled, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				category = excluded.category,
				tools = excluded.tools,
				loading_mode = excluded.loading_mode,
				manifest = excluded.manifest,
				author = excluded.author,
				version = excluded.version
		`, s.ID, s.Name, s.Description, s.Category, string(toolsJSON), s.LoadingMode, s.Manifest, s.Author, s.Version, enabledInt)
	}
	return nil
}
