import { useState, useEffect, useCallback } from 'react';
import useModalA11y from './useModalA11y';
import { resolveGatewayUrl } from '../config';

export default function useMcpState({
  getDefaultArgsForTool
}) {
  const [servers, setServers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [togglingServer, setTogglingServer] = useState({});
  const [copiedKey, setCopiedKey] = useState('');

  // Token Management.
  //
  // The snippets used to be pre-filled with a stored API key, which rendered a saved
  // credential into the page (and into every copied cURL/config block). Plaintext is
  // shown once, at creation, so the fields start with the placeholder and the
  // operator either pastes a key they kept or explicitly selects one below.
  const getInitialToken = useCallback(() => '', []);

  const [probeToken, setProbeToken] = useState('');
  const [configToken, setConfigToken] = useState('');
  const [requestViewTab, setRequestViewTab] = useState('wire'); // 'wire' | 'json' | 'curl'

  useEffect(() => {
    const initTok = getInitialToken();
    if (!probeToken && initTok) setProbeToken(initTok);
    if (!configToken && initTok) setConfigToken(initTok);
    // The guards above make the extra dependencies harmless: once a token is set,
    // re-running cannot overwrite it.
  }, [getInitialToken, probeToken, configToken]);

  // MCP Master Switch & Stats
  const [mcpSettings, setMcpSettings] = useState({
    mcp_enabled: true,
    enabled_skills_count: 6,
    active_tools_count: 9,
    servers_total: 8,
    servers_enabled: 7
  });
  const [togglingGlobal, setTogglingGlobal] = useState(false);

  // Modals
  const [configModalOpen, setConfigModalOpen] = useState(false);
  const [activeConfigClient, setActiveConfigClient] = useState('cursor');
  const [selectedServerForConfig, setSelectedServerForConfig] = useState(null);

  const [addServerModalOpen, setAddServerModalOpen] = useState(false);
  const [editingServerId, setEditingServerId] = useState('');
  const [savingServer, setSavingServer] = useState(false);
  const [newServer, setNewServer] = useState({
    id: '',
    name: '',
    description: '',
    category: 'dev',
    transport: 'stdio',
    endpoint: '',
    author: 'Community',
    version: '1.0.0',
    tools: '',
    prompts: '',
    resources: '',
    env_vars: '',
    enabled: true
  });

  // Probe Modal
  const [probeModalOpen, setProbeModalOpen] = useState(false);
  const [probeServer, setProbeServer] = useState(null);
  const [probeMethod, setProbeMethod] = useState('server/discover');
  const [probeToolName, setProbeToolName] = useState('airoute_data_redact');
  const [probeToolArgs, setProbeToolArgs] = useState(getDefaultArgsForTool('airoute_data_redact'));
  const [probeRunning, setProbeRunning] = useState(false);
  const [probeResult, setProbeResult] = useState(null);
  const [probeLatency, setProbeLatency] = useState(null);
  const [probeHttpStatus, setProbeHttpStatus] = useState(null);
  const probeFailed = !!probeResult?.error || !!probeResult?.result?.isError || probeHttpStatus >= 400;
  const [lastSentPayload, setLastSentPayload] = useState(null);

  useModalA11y(configModalOpen, () => setConfigModalOpen(false));
  useModalA11y(addServerModalOpen, () => setAddServerModalOpen(false));
  useModalA11y(probeModalOpen, () => setProbeModalOpen(false));

  const sseUrl = resolveGatewayUrl(mcpSettings?.sse_endpoint || '/mcp/sse');
  const messagesUrl = resolveGatewayUrl(mcpSettings?.messages_endpoint || '/mcp/messages');
  return {
    activeConfigClient, addServerModalOpen, configModalOpen, configToken, copiedKey, editingServerId,
    getInitialToken, lastSentPayload, loading, mcpSettings, messagesUrl, newServer, probeFailed,
    probeHttpStatus, probeLatency, probeMethod, probeModalOpen, probeResult, probeRunning, probeServer,
    probeToken, probeToolArgs, probeToolName, requestViewTab, savingServer, searchQuery, selectedCategory,
    selectedServerForConfig, servers, setActiveConfigClient, setAddServerModalOpen, setConfigModalOpen,
    setConfigToken, setCopiedKey, setEditingServerId, setLastSentPayload, setLoading, setMcpSettings,
    setNewServer, setProbeHttpStatus, setProbeLatency, setProbeMethod, setProbeModalOpen, setProbeResult,
    setProbeRunning, setProbeServer, setProbeToken, setProbeToolArgs, setProbeToolName, setRequestViewTab,
    setSavingServer, setSearchQuery, setSelectedCategory, setSelectedServerForConfig, setServers,
    setTogglingGlobal, setTogglingServer, sseUrl, togglingGlobal, togglingServer
  };
}
