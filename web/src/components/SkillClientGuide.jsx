import { Check, Copy, X, Terminal, DownloadCloud } from 'lucide-react';

export default function SkillClientGuide({
  activeClientTab, copiedKey, handleCopyText, isZh, origin, selectedSkillForGuide, setActiveClientTab,
  setClientGuideModalOpen
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === e.currentTarget) setClientGuideModalOpen(false);
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-gray-200 dark:border-slate-800 max-w-2xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-linear-to-br from-amber-500 to-orange-600 text-white rounded-lg">
              <Terminal className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-gray-900 dark:text-white">
                {isZh ? '智能体客户端接入示例' : 'Agent Client Integration Examples'}
              </h3>
              <p className="ui-meta">
                {selectedSkillForGuide
                  ? (isZh ? `将 [${selectedSkillForGuide.name}] 导入您的 Agent 编程环境` : `Import [${selectedSkillForGuide.name}] into your Agent environment`)
                  : (isZh ? '将 Agent Skills 规范集成到各大主流智能体客户端' : 'Integrate Agent Skills standards into major AI clients')}
              </p>
            </div>
          </div>
          <button
            onClick={() => setClientGuideModalOpen(false)}
            className="ui-icon-btn"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 解耦规范与原理说明横幅 */}
        {(() => {
          const currentId = selectedSkillForGuide?.id || 'git-workflow';
          return (
            <div className="p-3 bg-amber-50 dark:bg-amber-950/40 rounded-lg border border-amber-200/80 dark:border-amber-800/60 text-xs text-amber-900 dark:text-amber-200 leading-relaxed space-y-1.5">
              <div className="font-semibold flex items-center justify-between text-amber-800 dark:text-amber-100">
                <div className="flex items-center gap-1.5">
                  <CheckCircle2 className="w-4 h-4 text-emerald-600" />
                  <span>{isZh ? 'SOP 规范正文与客户端接入完全解耦' : 'SOP Specification decoupled from client environments'}</span>
                </div>
                <a
                  href={`/api/v1/skills/${currentId}/download`}
                  download={`${currentId}.zip`}
                  className="ui-btn ui-btn-sm font-semibold bg-emerald-600 hover:bg-emerald-700 text-white"
                >
                  <DownloadCloud className="w-3.5 h-3.5" />
                  {isZh ? `一键下载 ${currentId}.zip` : `Download ${currentId}.zip`}
                </a>
              </div>
              <p>
                {isZh ? (
                  <>技能遵循 <strong>agentskills.io</strong> 开放标准以标准 ZIP 压缩包分发。<code className="font-mono bg-white/70 dark:bg-gray-800 px-1 py-0.5 rounded-sm">SKILL.md</code> 内部保持纯净的 SOP 规范与规则契约，不掺杂任何特定客户端安装脚本。工具由本机系统环境或 MCP 协议广场提供，<strong>零网络下载等待</strong>。</>
                ) : (
                  <>Skills follow open <strong>agentskills.io</strong> standard distributed as ZIP archives. <code className="font-mono bg-white/70 dark:bg-gray-800 px-1 py-0.5 rounded-sm">SKILL.md</code> maintains pure SOP contracts without vendor lock-in. Tools are provided locally or via MCP protocol, <strong>with zero wait</strong>.</>
                )}
              </p>
            </div>
          );
        })()}

        {/* 客户端选择 Tabs */}
        <div className="flex items-center gap-2 border-b border-gray-200 dark:border-gray-800 pb-2">
          {[
            { id: 'claude-code', label: 'Claude Code' },
            { id: 'opencode', label: 'OpenCode' },
            { id: 'codex', label: 'Codex / OpenAI SDK' },
            { id: 'cursor', label: 'Cursor / Cline' }
          ].map(c => (
            <button
              key={c.id}
              onClick={() => setActiveClientTab(c.id)}
              className={`px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${activeClientTab === c.id ? 'bg-amber-600 text-white' : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'}`}
            >
              {c.label}
            </button>
          ))}
        </div>

        {/* 接入指令与代码片段 */}
        {(() => {
          const currentId = selectedSkillForGuide?.id || 'git-workflow';
          return (
            <div className="space-y-3">
              {activeClientTab === 'claude-code' && (
                <div className="space-y-3 text-xs">
                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                      <span>{isZh ? '步骤 1：下载并解压标准技能包至 Claude Code 技能目录' : 'Step 1: Download & unzip standard package to Claude Code skills dir'}</span>
                      <button
                        onClick={() => handleCopyText(`mkdir -p .claude/skills && curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .claude/skills/ && rm -f ${currentId}.zip`, 'cmd-claude-curl')}
                        className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                      >
                        {copiedKey === 'cmd-claude-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                        {isZh ? '复制一键命令' : 'Copy Command'}
                      </button>
                    </div>
                    <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs overflow-x-auto">
    {`# ${isZh ? '一键下载解压标准包' : 'Quick download & unpack'} (${currentId}.zip)
    mkdir -p .claude/skills
    curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip
    unzip -q -o ${currentId}.zip -d .claude/skills/
    rm -f ${currentId}.zip`}
                    </pre>
                  </div>

                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200">
                      {isZh ? '步骤 2：启动 Claude Code，智能体将自动索引并渐进式激活该技能' : 'Step 2: Launch Claude Code; agent will auto-index and activate skill'}
                    </div>
                    <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs">
    {`claude
    # ${isZh ? '直接在对话中提出需求，Claude Code 会自动读取 .claude/skills/' + currentId + '/SKILL.md 标准流程执行！' : 'Prompt directly in chat; Claude Code auto-activates .claude/skills/' + currentId + '/SKILL.md!'}`}
                    </pre>
                  </div>
                </div>
              )}

              {activeClientTab === 'opencode' && (
                <div className="space-y-3 text-xs">
                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                      <span>{isZh ? '步骤 1：下载并解压标准技能包至 OpenCode 技能目录' : 'Step 1: Download & unzip package to OpenCode skills dir'}</span>
                      <button
                        onClick={() => handleCopyText(`mkdir -p .opencode/skills && curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .opencode/skills/ && rm -f ${currentId}.zip`, 'cmd-opencode-curl')}
                        className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                      >
                        {copiedKey === 'cmd-opencode-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                        {isZh ? '复制一键命令' : 'Copy Command'}
                      </button>
                    </div>
                    <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs overflow-x-auto">
    {`# ${isZh ? '一键下载解压标准包' : 'Quick download & unpack'} (${currentId}.zip)
    mkdir -p .opencode/skills
    curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip
    unzip -q -o ${currentId}.zip -d .opencode/skills/
    rm -f ${currentId}.zip`}
                    </pre>
                  </div>

                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200">
                      {isZh ? '步骤 2：启动 OpenCode' : 'Step 2: Launch OpenCode'}
                    </div>
                    <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs">
    {`opencode
    # ${isZh ? 'OpenCode 原生兼容 agentskills.io 开放标准，遇到触发词自动载入 SOP 并调用工具' : 'OpenCode natively adheres to agentskills.io, auto-loading SOP on keyword triggers'}`}
                    </pre>
                  </div>
                </div>
              )}

              {activeClientTab === 'codex' && (
                <div className="space-y-3 text-xs">
                  <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                    <span>{isZh ? 'Python SDK (OpenAI / Codex API) 载入与网关调用示例：' : 'Python SDK (OpenAI / Codex API) Gateway Integration Example:'}</span>
                    <button
                      onClick={() => handleCopyText(`import os\nimport urllib.request\nimport zipfile\nimport pathlib\nfrom openai import OpenAI\n\n# 1. Pull & unzip skill archive\nskill_id = "${currentId}"\nzip_path = f"{skill_id}.zip"\nurllib.request.urlretrieve(f"${origin}/api/v1/skills/{skill_id}/download", zip_path)\nwith zipfile.ZipFile(zip_path, "r") as z:\n    z.extractall(".skills/")\nos.remove(zip_path)\n\n# 2. Read pure SKILL.md as System SOP\nskill_sop = pathlib.Path(f".skills/{skill_id}/SKILL.md").read_text(encoding="utf-8")\n\n# 3. Connect via airoute unified gateway\nclient = OpenAI(base_url="${origin}/v1", api_key="sk-airoute-key")\nresponse = client.chat.completions.create(\n    model="claude-3-7-sonnet",\n    messages=[\n        {"role": "system", "content": f"Follow this Skill SOP:\\n{skill_sop}"},\n        {"role": "user", "content": "Execute task following workflow specification"}\n    ]\n)\nprint(response.choices[0].message.content)`, 'cmd-codex')}
                      className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                    >
                      {copiedKey === 'cmd-codex' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                      {isZh ? '复制代码' : 'Copy Code'}
                    </button>
                  </div>
                  <pre className="p-3 bg-gray-900 text-gray-100 rounded-lg font-mono text-xs overflow-x-auto">
    {`import os
    import urllib.request
    import zipfile
    import pathlib
    from openai import OpenAI

    # 1. Pull & unzip skill archive
    skill_id = "${currentId}"
    zip_path = f"{skill_id}.zip"
    urllib.request.urlretrieve(f"${origin}/api/v1/skills/{skill_id}/download", zip_path)
    with zipfile.ZipFile(zip_path, "r") as z:
    z.extractall(".skills/")
    os.remove(zip_path)

    # 2. Read pure SKILL.md as System SOP
    skill_sop = pathlib.Path(f".skills/{skill_id}/SKILL.md").read_text(encoding="utf-8")

    # 3. Connect via airoute unified gateway
    client = OpenAI(base_url="${origin}/v1", api_key="sk-airoute-key")
    response = client.chat.completions.create(
    model="claude-3-7-sonnet",
    messages=[
    {"role": "system", "content": f"Follow this Skill SOP:\\n{skill_sop}"},
    {"role": "user", "content": "Execute task following workflow specification"}
    ]
    )
    print(response.choices[0].message.content)`}
                  </pre>
                </div>
              )}

              {activeClientTab === 'cursor' && (
                <div className="space-y-3 text-xs">
                  <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 space-y-2">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center justify-between">
                      <span>{isZh ? 'Cursor / Cline 导入方案' : 'Cursor / Cline Import Options'}</span>
                      <button
                        onClick={() => handleCopyText(`mkdir -p .cursor/skills && curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip && unzip -q -o ${currentId}.zip -d .cursor/skills/ && rm -f ${currentId}.zip`, 'cmd-cursor')}
                        className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                      >
                        {copiedKey === 'cmd-cursor' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                        {isZh ? '复制一键命令' : 'Copy Command'}
                      </button>
                    </div>
                    <pre className="p-2.5 bg-gray-900 text-gray-100 rounded-sm font-mono text-xs overflow-x-auto">
    {`# ${isZh ? '方案 A: 解压至 Cursor 技能目录' : 'Option A: Unpack to Cursor skills directory'}
    mkdir -p .cursor/skills
    curl -sL ${origin}/api/v1/skills/${currentId}/download -o ${currentId}.zip
    unzip -q -o ${currentId}.zip -d .cursor/skills/
    rm -f ${currentId}.zip

    # ${isZh ? '方案 B: 直接追加至项目根目录的 .cursorrules' : 'Option B: Append directly to root .cursorrules'}
    cat .cursor/skills/${currentId}/SKILL.md >> .cursorrules`}
                    </pre>
                  </div>
                </div>
              )}
            </div>
          );
        })()}

        <div className="flex justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
          <button
            onClick={() => setClientGuideModalOpen(false)}
            className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
          >
            {isZh ? '关闭' : 'Close'}
          </button>
        </div>
      </div>
    </div>
  );
}
