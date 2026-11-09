import { Plus, X } from 'lucide-react';

export default function SkillCreateModal({
  editingSkillId, handleCreateSkill, isZh, newSkill, savingSkill, setCreateModalOpen, setNewSkill
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === e.currentTarget) setCreateModalOpen(false);
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-gray-200 dark:border-slate-800 max-w-xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-linear-to-br from-amber-500 to-orange-600 text-white rounded-lg">
              <Plus className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-gray-900 dark:text-white">
                {editingSkillId ? (isZh ? '编辑 Agent 技能' : 'Edit Agent Skill') : (isZh ? '新建自定义 Agent 技能' : 'Create Custom Agent Skill')}
              </h3>
              <p className="ui-meta">
                {isZh ? '遵循 agentskills.io 规范编写自定义工作流 SOP' : 'Author custom workflow SOP adhering to agentskills.io standards'}
              </p>
            </div>
          </div>
          <button
            onClick={() => setCreateModalOpen(false)}
            className="ui-icon-btn"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleCreateSkill} className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label className="ui-label">
                {isZh ? '技能 ID (如: api-designer) *' : 'Skill ID (e.g. api-designer) *'}
              </label>
              <input
                type="text"
                required
                placeholder={isZh ? "如: api-designer" : "e.g. api-designer"}
                readOnly={!!editingSkillId}
                value={newSkill.id}
                onChange={(e) => setNewSkill({ ...newSkill, id: e.target.value })}
                className="ui-field bg-gray-50 dark:bg-gray-900"
              />
            </div>
            <div>
              <label className="ui-label">
                {isZh ? '技能名称 *' : 'Skill Name *'}
              </label>
              <input
                type="text"
                required
                placeholder={isZh ? '如: OpenAPI 3.0 接口规范设计' : 'e.g. OpenAPI 3.0 API Design'}
                value={newSkill.name}
                onChange={(e) => setNewSkill({ ...newSkill, name: e.target.value })}
                className="ui-field bg-gray-50 dark:bg-gray-900"
              />
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label className="ui-label">
                {isZh ? '分类领域' : 'Category'}
              </label>
              <select
                value={newSkill.category}
                onChange={(e) => setNewSkill({ ...newSkill, category: e.target.value })}
                className="ui-field bg-gray-50 dark:bg-gray-900"
              >
                <option value="dev">{isZh ? '研发工程 (dev)' : 'Dev Engineering (dev)'}</option>
                <option value="test">{isZh ? '质量保障 (test)' : 'QA & TDD (test)'}</option>
                <option value="automation">{isZh ? '端到端自动化 (automation)' : 'E2E Automation (automation)'}</option>
                <option value="security">{isZh ? '代码安全审计 (security)' : 'Security Audit (security)'}</option>
              </select>
            </div>
            <div>
              <label className="ui-label">
                {isZh ? '依赖工具 (逗号分隔)' : 'Required Tools (comma separated)'}
              </label>
              <input
                type="text"
                placeholder={isZh ? "如: run_command, view_file" : "e.g. run_command, view_file"}
                value={newSkill.tools}
                onChange={(e) => setNewSkill({ ...newSkill, tools: e.target.value })}
                className="ui-field bg-gray-50 dark:bg-gray-900 font-mono"
              />
            </div>
          </div>

          <div>
            <label className="ui-label">
              {isZh ? '简要描述与触发时机 *' : 'Description & Trigger Conditions *'}
            </label>
            <textarea
              rows="2"
              required
              placeholder={isZh ? '说明该技能的功能以及智能体在何时应当激活...' : 'Describe skill capabilities and trigger situations...'}
              value={newSkill.description}
              onChange={(e) => setNewSkill({ ...newSkill, description: e.target.value })}
              className="ui-field bg-gray-50 dark:bg-gray-900"
            />
          </div>

          <div>
            <label className="ui-label">
              {isZh ? 'SKILL.md 规范内容 (Markdown)' : 'SKILL.md Specification (Markdown)'}
            </label>
            <textarea
              rows="5"
              placeholder={`---
    name: ${newSkill.id || 'my-skill'}
    description: ${isZh ? '描述内容' : 'Skill description'}
    allowed-tools:
    - run_command
    ---

    # ${isZh ? '技能执行指引' : 'Skill Execution Instructions'}
    1. ${isZh ? '第一步...' : 'Step 1...'}
    2. ${isZh ? '第二步...' : 'Step 2...'}`}
              value={newSkill.manifest}
              onChange={(e) => setNewSkill({ ...newSkill, manifest: e.target.value })}
              className="w-full px-3 py-2 font-mono text-xs rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-900 text-gray-100"
            />
          </div>

          <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-gray-200 dark:border-gray-800">
            <button
              type="button"
              onClick={() => setCreateModalOpen(false)}
              className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
            >
              {isZh ? '取消' : 'Cancel'}
            </button>
            <button
              type="submit"
              disabled={savingSkill}
              className="px-4 py-2 text-xs font-medium text-white bg-amber-600 rounded-lg hover:bg-amber-700 shadow-sm"
            >
              {savingSkill ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存技能' : 'Save Skill')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
