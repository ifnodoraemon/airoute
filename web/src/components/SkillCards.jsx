import { FileText, Trash2, Pencil, Play, Terminal, DownloadCloud, GitBranch, Bug, Globe, Lock } from 'lucide-react';

export default function SkillCards({
  filteredSkills, handleDeleteSkill, handleToggleSkill, isZh, setClientGuideModalOpen, setCreateModalOpen,
  setEditingSkillId, setManifestSkill, setManifestTab, setNewSkill, setSelectedSkillForGuide,
  setTestModalSkill, togglingSkill
}) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {filteredSkills.map(skill => {
        const isToggling = !!togglingSkill[skill.id];
        const isCustom = skill.id.startsWith('skill_') || skill.id.startsWith('custom_');

        return (
          <div
            key={skill.id}
            className={`flex flex-col justify-between p-6 rounded-2xl border transition-all duration-200 ${
              skill.enabled
                ? 'border-slate-200/80 dark:border-slate-800 bg-white dark:bg-[#191B1F] shadow-xs hover:shadow-md'
                : 'border-slate-200/50 dark:border-slate-800/50 bg-slate-50/50 dark:bg-slate-900/30 opacity-75'
            }`}
          >
            <div>
              {/* 头部：图标、名称与启停开关 */}
              <div className="flex items-start justify-between gap-4 mb-4">
                <div className="flex items-center gap-3.5 min-w-0">
                  <div className="w-11 h-11 rounded-2xl bg-amber-50/80 dark:bg-amber-950/50 text-amber-600 dark:text-amber-400 border border-amber-200/60 dark:border-amber-800/60 flex items-center justify-center shrink-0 shadow-2xs">
                    {skill.id === 'git-workflow' ? <GitBranch className="w-5 h-5" /> :
                     skill.id === 'test-driven-development' ? <Bug className="w-5 h-5" /> :
                     skill.id === 'browser-automation' ? <Globe className="w-5 h-5" /> :
                     <Lock className="w-5 h-5" />}
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <h3 className="text-sm font-semibold text-slate-900 dark:text-white truncate">
                        {skill.name}
                      </h3>
                      <span className="px-2 py-0.5 text-xs font-mono rounded-md bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400">
                        v{skill.version || '1.0.0'}
                      </span>
                    </div>
                    <div className="ui-meta font-mono truncate mt-0.5">
                      {skill.id}
                    </div>
                  </div>
                </div>

                <button
                  onClick={() => handleToggleSkill(skill.id, skill.enabled)}
                  disabled={isToggling}
                  className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${skill.enabled ? 'bg-amber-600' : 'bg-slate-300 dark:bg-slate-700'}`}
                  title={skill.enabled ? (isZh ? '点击禁用' : 'Click to disable') : (isZh ? '点击启用' : 'Click to enable')}
                >
                  <span
                    className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ease-in-out ${skill.enabled ? 'translate-x-4' : 'translate-x-0'}`}
                  />
                </button>
              </div>

              {/* 简介 */}
              <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed line-clamp-2 my-4">
                {skill.description}
              </p>

              {/* 适用工具清单 */}
              {skill.tools && skill.tools.length > 0 && (
                <div className="flex items-center gap-1.5 flex-wrap mb-5 text-xs">
                  <span className="ui-meta mr-0.5">{isZh ? '工具集:' : 'Tools:'}</span>
                  {skill.tools.slice(0, 5).map(t => (
                    <span
                      key={t}
                      className="px-2 py-0.5 font-mono text-xs bg-slate-50 dark:bg-slate-900 text-slate-600 dark:text-slate-300 rounded-md border border-slate-200/70 dark:border-slate-800"
                    >
                      {t}
                    </span>
                  ))}
                  {skill.tools.length > 5 && (
                    <span className="ui-meta font-mono font-medium">+{skill.tools.length - 5}</span>
                  )}
                </div>
              )}
            </div>

            {/* 卡片底部操作按钮 */}
            <div className="pt-4 mt-auto border-t border-slate-100 dark:border-slate-800/80 flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 flex-wrap">
                <a
                  href={`/api/v1/skills/${skill.id}/download`}
                  download={`${skill.id}.zip`}
                  className="ui-btn ui-btn-sm border border-emerald-200/80 dark:border-emerald-800/80 bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-100 dark:hover:bg-emerald-900/50 shadow-2xs"
                  title={isZh ? '直接下载 .zip 压缩包' : 'Direct download .zip archive'}
                >
                  <DownloadCloud className="w-3.5 h-3.5" />
                  <span>{isZh ? '下载 .zip' : 'Download .zip'}</span>
                </a>
                <button
                  onClick={() => { setManifestSkill(skill); setManifestTab('skill_md'); }}
                  className="ui-btn ui-btn-sm border border-slate-200/70 dark:border-slate-700/70 bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700 shadow-2xs"
                >
                  <FileText className="w-3.5 h-3.5" />
                  <span>{isZh ? '查看 SOP' : 'View SOP'}</span>
                </button>
                <button
                  onClick={() => { setSelectedSkillForGuide(skill); setClientGuideModalOpen(true); }}
                  className="ui-btn ui-btn-sm border border-[#355CFF]/30 dark:border-[#91A7FF]/30 bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] hover:bg-[#E0E7FF] dark:hover:bg-[#2e3a5e] shadow-2xs"
                >
                  <Terminal className="w-3.5 h-3.5" />
                  <span>{isZh ? '接入' : 'Connect'}</span>
                </button>
                <button
                  title={isZh ? '编辑技能' : 'Edit skill'}
                  onClick={() => { setEditingSkillId(skill.id); setNewSkill({ ...skill, tools: (skill.tools || []).join(', ') }); setCreateModalOpen(true); }}
                  className="ui-btn-secondary"
                ><Pencil className="w-3.5 h-3.5" />{isZh ? '编辑' : 'Edit'}</button>
                <button
                  onClick={() => setTestModalSkill(skill)}
                  className="ui-btn-ghost ui-btn-sm"
                >
                  <Play className="w-3 h-3" />
                  <span>{isZh ? '模型实测' : 'Model test'}</span>
                </button>
              </div>

              {isCustom && (
                <button
                  onClick={() => handleDeleteSkill(skill.id)}
                  className="ui-icon-btn hover:text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-950/40"
                  title={isZh ? '删除自定义技能' : 'Delete custom skill'}
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
