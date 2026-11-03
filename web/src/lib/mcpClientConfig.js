// Split an executable without interpreting a shell, mirroring ParseMCPCommand.
export function parseMCPCommand(command) {
  const words = [];
  let word = '', quote = '', escaped = false, started = false;
  for (const char of command) {
    if (escaped) { word += char; escaped = false; continue; }
    if (char === '\\' && quote !== "'") { escaped = true; started = true; continue; }
    if (quote) { if (char === quote) quote = ''; else word += char; continue; }
    if (char === "'" || char === '"') { quote = char; started = true; }
    else if (/\s/.test(char)) {
      if (started) { words.push(word); word = ''; started = false; }
    } else { word += char; started = true; }
  }
  if (quote || escaped) throw new Error('命令引号或转义符未闭合');
  if (started) words.push(word);
  if (!words[0]) throw new Error('MCP 命令不能为空');
  return words;
}

export function buildMCPClientConfig({ client, gatewayUrl, token, server }) {
  const headers = { Authorization: `Bearer ${token?.trim() || 'YOUR_AIROUTE_KEY'}` };
  const remote = (url, gateway = false) => (client === 'claude' || (!gateway && client === 'cline'))
    ? { command: 'npx', args: ['-y', 'mcp-remote', url, ...(gateway ? ['--header', `Authorization: ${headers.Authorization}`] : [])] }
    : { url, ...(gateway ? { headers } : {}) };
  const servers = { 'airoute-gateway': remote(gatewayUrl, true) };
  if (client === 'cline') servers['airoute-gateway'].type = 'sse';
  if (server && server.id !== 'airoute-gateway') {
    if (server.transport === 'stdio') {
      const [command, ...args] = parseMCPCommand(server.endpoint);
      const env = server.env_vars?.trim() ? JSON.parse(server.env_vars) : {};
      if (!env || Array.isArray(env) || typeof env !== 'object' || Object.values(env).some(value => typeof value !== 'string')) {
        throw new Error('环境变量必须是字符串值的 JSON 对象');
      }
      servers[server.id] = { command, args, ...(Object.keys(env).length ? { env } : {}) };
    } else {
      // A gateway API key authenticates only to the gateway, never to third parties.
      servers[server.id] = remote(server.endpoint);
    }
  }
  return JSON.stringify({ mcpServers: servers }, null, 2);
}
