
export default function useMcpProbe({
  getDefaultArgsForTool, getInitialToken, isZh, lastSentPayload, messagesUrl, probeMethod, probeServer,
  probeToken, probeToolArgs, probeToolName, setLastSentPayload, setProbeHttpStatus, setProbeLatency,
  setProbeMethod, setProbeModalOpen, setProbeResult, setProbeRunning, setProbeServer, setProbeToken,
  setProbeToolArgs, setProbeToolName, showToast
}) {
  const getCurrentRpcPayload = () => {
    if (probeMethod === 'server/discover') {
      return {
        jsonrpc: "2.0",
        id: "probe-discover-" + (probeServer ? probeServer.id : 'core'),
        method: "server/discover",
        params: {}
      };
    } else if (probeMethod === 'tools/list') {
      return {
        jsonrpc: "2.0",
        id: "probe-list-" + (probeServer ? probeServer.id : 'core'),
        method: "tools/list",
        params: {}
      };
    } else {
      let parsedArgs = {};
      try {
        parsedArgs = JSON.parse(probeToolArgs);
      } catch (e) {
        parsedArgs = { _syntax_hint: "JSON 待补全" };
      }
      return {
        jsonrpc: "2.0",
        id: "probe-call-" + (probeServer ? probeServer.id : 'core'),
        method: "tools/call",
        params: {
          name: probeToolName,
          arguments: parsedArgs
        }
      };
    }
  };

  const handleSelectTool = (name) => {
    setProbeToolName(name);
    setProbeToolArgs(getDefaultArgsForTool(name));
  };

  const handleOpenProbe = (server) => {
    setProbeServer(server);
    setProbeMethod(server ? 'tools/call' : 'server/discover');
    setProbeResult(null);
    setProbeLatency(null);
    setProbeHttpStatus(null);
    setLastSentPayload(null);
    const initialTool = (server && server.tools && server.tools.length > 0)
      ? server.tools[0]
      : 'airoute_data_redact';
    setProbeToolName(initialTool);
    setProbeToolArgs(getDefaultArgsForTool(initialTool));
    if (!probeToken) {
      setProbeToken(getInitialToken());
    }
    setProbeModalOpen(true);
  };

  const handleRunProbe = async () => {
    setProbeRunning(true);
    setProbeResult(null);
    setProbeHttpStatus(null);
    const start = performance.now();

    try {
      let rpcReq = {};
      if (probeMethod === 'server/discover') {
        rpcReq = {
          jsonrpc: "2.0",
          id: "probe-discover-" + Date.now(),
          method: "server/discover",
          params: {}
        };
      } else if (probeMethod === 'tools/list') {
        rpcReq = {
          jsonrpc: "2.0",
          id: "probe-list-" + Date.now(),
          method: "tools/list",
          params: {}
        };
      } else {
        let parsedArgs = {};
        try {
          parsedArgs = JSON.parse(probeToolArgs);
        } catch (e) {
          if (showToast) showToast((isZh ? '工具参数 JSON 语法错误: ' : 'Tool argument JSON syntax error: ') + e.message, 'error');
          setProbeRunning(false);
          return;
        }
        rpcReq = {
          jsonrpc: "2.0",
          id: "probe-call-" + Date.now(),
          method: "tools/call",
          params: {
            name: probeToolName,
            arguments: parsedArgs
          }
        };
      }

      setLastSentPayload(rpcReq);

      const headers = { 'Content-Type': 'application/json' };
      if (probeToken && probeToken.trim()) {
        headers['Authorization'] = `Bearer ${probeToken.trim()}`;
      }

      const res = await fetch(messagesUrl, {
        method: 'POST',
        headers,
        body: JSON.stringify(rpcReq)
      });
      setProbeHttpStatus(res.status);
      const data = await res.json();
      const duration = Math.round(performance.now() - start);
      setProbeLatency(duration);
      setProbeResult(data);
    } catch (err) {
      const duration = Math.round(performance.now() - start);
      setProbeLatency(duration);
      setProbeResult({
        jsonrpc: "2.0",
        id: "error",
        error: { code: -32603, message: (isZh ? "探针请求失败: " : "Probe request failed: ") + err.message }
      });
    } finally {
      setProbeRunning(false);
    }
  };

  const getWireRequestText = () => {
    const payload = lastSentPayload || getCurrentRpcPayload();
    let path = '/mcp/messages';
    let host = typeof window !== 'undefined' && window.location?.host ? window.location.host : 'api.airoute.local';
    try {
      const u = new URL(messagesUrl, window.location.origin);
      path = u.pathname + (u.search || '');
      host = u.host;
    } catch (e) {
      // fallback
    }
    const token = probeToken?.trim() || 'YOUR_AIROUTE_KEY';
    return `POST ${path} HTTP/1.1\nHost: ${host}\nContent-Type: application/json\nAuthorization: Bearer ${token}\n\n${JSON.stringify(payload, null, 2)}`;
  };

  const getCurlRequestText = () => {
    const payload = lastSentPayload || getCurrentRpcPayload();
    const token = probeToken?.trim() || 'YOUR_AIROUTE_KEY';
    return `curl -X POST "${messagesUrl}" \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer ${token}" \\\n  -d '${JSON.stringify(payload)}'`;
  };
  return {
    getCurlRequestText, getCurrentRpcPayload, getWireRequestText, handleOpenProbe, handleRunProbe,
    handleSelectTool
  };
}
