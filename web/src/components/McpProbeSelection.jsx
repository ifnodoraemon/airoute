
export default function McpProbeSelection({
  TOOL_TEMPLATES, getGroupLabel, getToolInfo, handleSelectTool, isZh, probeMethod, probeServer, probeToolName,
  servers, setProbeMethod, setProbeServer
}) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
      <div>
        <label className="ui-label">
          {isZh ? '工具来源（能力发现与列表始终查询聚合网关）' : 'Tool source (discovery and listing query the gateway)'}
        </label>
        <select
          value={probeServer ? probeServer.id : 'all'}
          onChange={(e) => {
            const sid = e.target.value;
            if (sid === 'all') {
              setProbeServer(null);
            } else {
              const found = servers.find(s => s.id === sid);
              setProbeServer(found || null);
              if (found && found.tools && found.tools.length > 0) {
                handleSelectTool(found.tools[0]);
              }
            }
          }}
          className="ui-field bg-gray-50 dark:bg-gray-900"
        >
          <option value="all">{isZh ? '🌐 统一 MCP 聚合网关 (全部已启用服务)' : '🌐 Unified MCP Gateway (All Enabled Services)'}</option>
          {servers.map(s => (
            <option key={s.id} value={s.id}>
              {s.name} ({s.id})
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className="ui-label">
          {isZh ? '探针方法 (JSON-RPC Method)' : 'Probe Method (JSON-RPC Method)'}
        </label>
        <select
          value={probeMethod}
          onChange={(e) => setProbeMethod(e.target.value)}
          className="ui-field bg-gray-50 dark:bg-gray-900"
        >
          <option value="server/discover">{isZh ? 'server/discover (服务与能力探测)' : 'server/discover (Discover Capabilities)'}</option>
          <option value="tools/list">{isZh ? 'tools/list (枚举工具列表与 Schema)' : 'tools/list (List Tools & Schema)'}</option>
          <option value="tools/call">{isZh ? 'tools/call (在线执行具体工具)' : 'tools/call (Execute Specific Tool)'}</option>
        </select>
      </div>

      {probeMethod === 'tools/call' && (
        <div>
          <label className="ui-label">
            {isZh ? '选择目标工具 (Tool Name)' : 'Select Target Tool (Tool Name)'}
          </label>
          <select
            value={probeToolName}
            onChange={(e) => handleSelectTool(e.target.value)}
            className="ui-field bg-gray-50 dark:bg-gray-900 font-mono"
          >
            {/* 如果当前指定了服务，将其工具置顶 */}
            {probeServer && probeServer.tools && probeServer.tools.length > 0 && (
              <optgroup label={isZh ? `★ 当前服务 [${probeServer.name || probeServer.id}] 工具集` : `★ Current Server [${probeServer.name || probeServer.id}] Tools`}>
                {probeServer.tools.map(t => (
                  <option key={`cur-${t}`} value={t}>
                    {getToolInfo(t).label}
                  </option>
                ))}
              </optgroup>
            )}

            {/* 按分类分组呈现内置与生态工具 */}
            {Object.entries(
              Object.entries(TOOL_TEMPLATES).reduce((acc, [name, info]) => {
                acc[info.group] = acc[info.group] || [];
                acc[info.group].push({ name, ...info });
                return acc;
              }, {})
            ).map(([grp, toolList]) => (
              <optgroup key={grp} label={getGroupLabel(grp, isZh)}>
                {toolList.map(t => (
                  <option key={t.name} value={t.name}>
                    {t.label}
                  </option>
                ))}
              </optgroup>
            ))}

            {/* 容底项：若选中工具不在模板库中 */}
            {!TOOL_TEMPLATES[probeToolName] && (!probeServer?.tools?.includes(probeToolName)) && (
              <optgroup label={isZh ? '自定义探针工具' : 'Custom Probe Tool'}>
                <option value={probeToolName}>{probeToolName} ({isZh ? '当前选定工具' : 'Selected'})</option>
              </optgroup>
            )}
          </select>
        </div>
      )}
    </div>
  );
}
