import { FileText, Check, Copy, X, Terminal, Code2, DownloadCloud, HardDrive, Server, Folder, FolderTree } from 'lucide-react';

export default function SkillManifestModal({
  copiedKey, handleCopyText, isZh, manifestSkill, manifestTab, origin, setClientGuideModalOpen,
  setManifestSkill, setManifestTab, setSelectedSkillForGuide, storageStatus
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === e.currentTarget) setManifestSkill(null);
      }}
    >
      <div className="bg-white dark:bg-[#191B1F] rounded-xl shadow-2xl border border-gray-200 dark:border-slate-800 max-w-3xl w-full p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        {/* 模态框头部 */}
        <div className="flex items-center justify-between pb-3 border-b border-gray-200 dark:border-gray-800">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-amber-100 dark:bg-amber-900/50 text-amber-600 dark:text-amber-400 rounded-lg">
              <FileText className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
                {manifestSkill.name}
                <span className="text-xs font-mono font-normal text-amber-600 bg-amber-50 dark:bg-amber-950/60 px-1.5 py-0.5 rounded-sm border border-amber-200 dark:border-amber-800">
                  {manifestSkill.id}
                </span>
              </h3>
              <p className="ui-meta">
                {isZh ? '遵循 agentskills.io 开放标准的智能体 SOP 标准包与元数据契约' : 'Standard SOP package and metadata contract adhering to agentskills.io'}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <a
              href={`/api/v1/skills/${manifestSkill.id}/download`}
              download={`${manifestSkill.id}.zip`}
              className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-semibold bg-emerald-600 hover:bg-emerald-700 text-white rounded-lg transition-colors shadow-sm"
              title={isZh ? '下载包含 SKILL.md 与脚本的 agentskills.io 标准 ZIP 压缩包' : 'Download standard ZIP package containing SKILL.md and scripts'}
            >
              <DownloadCloud className="w-3.5 h-3.5" />
              <span>{isZh ? '下载 .zip 包' : 'Download .zip'}</span>
            </a>
            <button
              onClick={() => { setSelectedSkillForGuide(manifestSkill); setClientGuideModalOpen(true); }}
              className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-medium text-[#355CFF] dark:text-[#91A7FF] bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/30 dark:border-[#91A7FF]/30 rounded-lg hover:bg-[#E0E7FF] dark:hover:bg-[#2e3a5e] transition-colors"
            >
              <Terminal className="w-3.5 h-3.5" />
              <span>{isZh ? '接入客户端' : 'Connect Client'}</span>
            </button>
            <button
              onClick={() => setManifestSkill(null)}
              className="ui-icon-btn"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>

        {/* Tab 导航 */}
        <div className="flex items-center justify-between border-b border-gray-200 dark:border-gray-800 pb-2">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setManifestTab('skill_md')}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                manifestTab === 'skill_md'
                  ? 'bg-amber-600 text-white shadow-sm'
                  : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
              }`}
            >
              <FileText className="w-3.5 h-3.5" />
              <span>{isZh ? 'SKILL.md (纯净 SOP)' : 'SKILL.md (Pure SOP)'}</span>
            </button>
            <button
              onClick={() => setManifestTab('bundle_tree')}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                manifestTab === 'bundle_tree'
                  ? 'bg-amber-600 text-white shadow-sm'
                  : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
              }`}
            >
              <FolderTree className="w-3.5 h-3.5" />
              <span>{isZh ? '归档包目录树 (ZIP Tree)' : 'Archive Tree (ZIP Tree)'}</span>
            </button>
            <button
              onClick={() => setManifestTab('metadata')}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                manifestTab === 'metadata'
                  ? 'bg-amber-600 text-white shadow-sm'
                  : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
              }`}
            >
              <Code2 className="w-3.5 h-3.5" />
              <span>metadata.json</span>
            </button>
          </div>

          {manifestTab === 'skill_md' && (
            <button
              onClick={() => handleCopyText(manifestSkill.manifest, 'manifest-full')}
              className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors"
            >
              {copiedKey === 'manifest-full' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
              <span>{copiedKey === 'manifest-full' ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制 SOP' : 'Copy SOP')}</span>
            </button>
          )}
        </div>

        {/* Tab 1: SKILL.md */}
        {manifestTab === 'skill_md' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between ui-meta px-1">
              <span>{isZh ? '纯业务 SOP 指令契约 (不包含客户端下载逻辑，三级渐进披露规范)' : 'Pure business SOP contract (3-tier progressive disclosure)'}</span>
              <span className="font-mono">{manifestSkill.manifest.length} {isZh ? '字符' : 'chars'}</span>
            </div>
            <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto max-h-[55vh] border border-gray-800">
              <pre>{manifestSkill.manifest}</pre>
            </div>
          </div>
        )}

        {/* Tab 2: 归档包目录树 (ZIP Tree) */}
        {manifestTab === 'bundle_tree' && (
          <div className="space-y-4">
            {/* 存储后端与加速说明 */}
            <div className="p-3 bg-linear-to-r from-gray-50 to-slate-100 dark:from-gray-900 dark:to-[#191B1F] rounded-xl border border-gray-200 dark:border-gray-800 flex items-start justify-between gap-3 text-xs">
              <div className="space-y-1">
                <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center gap-1.5">
                  {storageStatus?.driver === 's3' ? (
                    <>
                      <Server className="w-4 h-4 text-emerald-600" />
                      <span>{isZh ? `分布式对象存储已激活 (RustFS / S3: bucket ` : `Distributed Object Storage Active (S3: bucket `}<code>{storageStatus.s3_bucket || 'skills'}</code>)</span>
                    </>
                  ) : (
                    <>
                      <HardDrive className="w-4 h-4 text-slate-500" />
                      <span>{isZh ? `本地文件驱动已激活 (Local: ` : `Local File Driver Active (Local: `}<code>{storageStatus?.local_path || 'data/skills_storage'}</code>)</span>
                    </>
                  )}
                </div>
                <p className="ui-meta">
                  {storageStatus?.driver === 's3'
                    ? (isZh ? '下载请求由网关直接签发 AWS SigV4 预签名临时 URL，302 重定向直连 RustFS 高速传输，完全释放网关带宽。' : 'Download requests are signed with AWS SigV4 presigned URLs and redirected 302 to RustFS high-speed transfer, offloading gateway bandwidth.')
                    : (isZh ? '归档包由网关本地磁盘存储提供流式下载与持久化。配置 STORAGE_DRIVER=s3 可无缝切换至 RustFS 对象存储。' : 'Archive packages are streamed directly from gateway local disk. Set STORAGE_DRIVER=s3 to switch to RustFS.')}
                </p>
              </div>
              <span className="px-2 py-0.5 rounded-sm text-xs font-mono font-medium bg-emerald-100 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800 shrink-0">
                S3 Key: skills/{manifestSkill.id}.zip
              </span>
            </div>

            {/* 目录树预览 */}
            <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs border border-gray-800 space-y-1.5">
              <div className="text-amber-400 font-bold flex items-center gap-1.5 pb-2 border-b border-gray-800">
                <Folder className="w-4 h-4" />
                <span>{manifestSkill.id}.zip ({isZh ? 'agentskills.io 标准封装包' : 'agentskills.io standard bundle'})</span>
              </div>
              <div className="pl-4 space-y-1 text-gray-300 text-xs">
                <div className="text-blue-400 flex items-center gap-1.5">
                  <span>└── 📁 {manifestSkill.id}/</span>
                </div>
                <div className="pl-6 space-y-1 text-gray-300">
                  <div className="flex items-center justify-between text-emerald-300">
                    <span>├── 📄 SKILL.md</span>
                    <span className="text-gray-400 text-xs">{isZh ? '# 核心 SOP 流程契约与三级执行阶段' : '# Core SOP contract & 3-stage execution'}</span>
                  </div>
                  <div className="text-blue-300">
                    <span>└── 📁 references/</span>
                  </div>
                  <div className="pl-6 flex items-center justify-between text-sky-300">
                    <span>    └── 📑 metadata.json</span>
                    <span className="text-gray-400 text-xs">{isZh ? '# 技能描述、版本与作者元数据' : '# Skill description, version, and author metadata'}</span>
                  </div>
                </div>
              </div>
            </div>

            {/* 快速拉取命令 */}
            <div className="p-3 bg-gray-50 dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 space-y-1.5 text-xs">
              <div className="flex items-center justify-between text-gray-700 dark:text-gray-300 font-medium">
                <span>{isZh ? '终端下载并解压命令 (Terminal Quickstart)' : 'Terminal Quickstart Download & Unzip'}</span>
                <button
                  onClick={() => handleCopyText(`curl -sL ${origin}/api/v1/skills/${manifestSkill.id}/download -o ${manifestSkill.id}.zip && unzip -q -o ${manifestSkill.id}.zip -d skills/`, 'tree-curl')}
                  className="text-xs text-amber-600 dark:text-amber-400 hover:underline inline-flex items-center gap-1"
                >
                  {copiedKey === 'tree-curl' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                  {isZh ? '复制命令' : 'Copy Command'}
                </button>
              </div>
              <pre className="p-2 bg-gray-900 text-gray-200 rounded-sm font-mono text-xs overflow-x-auto">
    {`curl -sL ${origin}/api/v1/skills/${manifestSkill.id}/download -o ${manifestSkill.id}.zip
    unzip -q -o ${manifestSkill.id}.zip -d skills/`}
              </pre>
            </div>
          </div>
        )}

        {/* Tab 3: metadata.json */}
        {manifestTab === 'metadata' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between ui-meta px-1">
              <span>{isZh ? 'agentskills.io 元数据声明契约' : 'agentskills.io Metadata Contract'}</span>
              <button
                onClick={() => handleCopyText(JSON.stringify({
                  schema_version: "agentskills.io/v1",
                  id: manifestSkill.id,
                  name: manifestSkill.name,
                  version: manifestSkill.version || "1.0.0",
                  author: manifestSkill.author || "Community Standard",
                  category: manifestSkill.category,
                  loading_mode: manifestSkill.loading_mode || "lazy",
                  tools: manifestSkill.tools || [],
                  enabled: manifestSkill.enabled,
                  storage: {
                    artifact_key: `skills/${manifestSkill.id}.zip`,
                    driver: storageStatus?.driver || 'local'
                  }
                }, null, 2), 'meta-json')}
                className="inline-flex items-center gap-1 text-amber-600 dark:text-amber-400 hover:underline"
              >
                {copiedKey === 'meta-json' ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                <span>{copiedKey === 'meta-json' ? (isZh ? '已复制' : 'Copied') : (isZh ? '复制 JSON' : 'Copy JSON')}</span>
              </button>
            </div>
            <div className="bg-gray-900 text-gray-100 p-4 rounded-xl font-mono text-xs overflow-x-auto max-h-[55vh] border border-gray-800">
              <pre>
    {JSON.stringify({
    schema_version: "agentskills.io/v1",
    id: manifestSkill.id,
    name: manifestSkill.name,
    description: manifestSkill.description,
    version: manifestSkill.version || "1.0.0",
    author: manifestSkill.author || "Community Standard",
    category: manifestSkill.category,
    loading_mode: manifestSkill.loading_mode || "lazy",
    tools: manifestSkill.tools || [],
    enabled: manifestSkill.enabled,
    storage: {
    artifact_key: `skills/${manifestSkill.id}.zip`,
    driver: storageStatus?.driver || 'local'
    }
    }, null, 2)}
              </pre>
            </div>
          </div>
        )}

        {/* 模态框底部 */}
        <div className="flex justify-end pt-2 border-t border-gray-100 dark:border-gray-800">
          <button
            onClick={() => setManifestSkill(null)}
            className="px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700"
          >
            {isZh ? '关闭' : 'Close'}
          </button>
        </div>
      </div>
    </div>
  );
}
