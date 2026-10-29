import { describe, expect, it } from 'vitest';
import { buildMCPClientConfig, parseMCPCommand } from './mcpClientConfig';

const gatewayUrl = 'https://gateway.example/mcp/sse';
describe('MCP client configuration', () => {
  it.each(['cursor', 'claude', 'cline'])('preserves the registered stdio runtime, quoting and environment for %s', client => {
    const config = JSON.parse(buildMCPClientConfig({ client, gatewayUrl, token: 'sk-gateway', server: {
      id: 'python-server', transport: 'stdio', endpoint: 'uvx service --root "folder with spaces"', env_vars: '{"SERVICE_TOKEN":"service-key"}',
    } }));
    expect(config.mcpServers['python-server']).toEqual({ command: 'uvx', args: ['service', '--root', 'folder with spaces'], env: { SERVICE_TOKEN: 'service-key' } });
  });

  it.each(['cursor', 'claude', 'cline'])('keeps the gateway credential scoped to the gateway in %s configs', client => {
    const config = JSON.parse(buildMCPClientConfig({ client, gatewayUrl, token: 'sk-gateway', server: { id: 'external', transport: 'http', endpoint: 'https://external.example/mcp' } }));
    expect(JSON.stringify(config.mcpServers.external)).not.toContain('sk-gateway');
    expect(JSON.stringify(config.mcpServers['airoute-gateway'])).toContain('sk-gateway');
    expect(JSON.stringify(config.mcpServers.external)).toContain('https://external.example/mcp');
  });

  it('keeps empty quoted arguments and refuses malformed command lines', () => {
    expect(parseMCPCommand('python "folder with spaces/server.py" \'\'')).toEqual(['python', 'folder with spaces/server.py', '']);
    expect(() => parseMCPCommand('python "bad')).toThrow();
  });
});
