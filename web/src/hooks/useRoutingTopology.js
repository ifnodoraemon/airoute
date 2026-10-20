import { useCallback, useEffect, useRef, useState } from 'react';
import { apiFetch, extractErrorMessage } from '../api/client';

export function useRoutingTopology(isOpen, adminFetch = apiFetch) {
  const [snapshot, setSnapshot] = useState(null);
  const [selectedModel, setSelectedModel] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [probing, setProbing] = useState('');
  const [probes, setProbes] = useState({});
  const modelRef = useRef('');
  const requestRef = useRef(null);
  const probeRef = useRef(null);

  const refresh = useCallback(async () => {
    requestRef.current?.abort();
    const controller = new AbortController();
    requestRef.current = controller;
    setLoading(true);
    try {
      const query = new URLSearchParams();
      if (modelRef.current) query.set('model', modelRef.current);
      const response = await adminFetch(`/api/v1/admin/models/topology?${query}`, { signal: controller.signal });
      const body = await response.json();
      if (!response.ok || body.code !== 0 || !body.data?.routing) throw new Error(extractErrorMessage(body, '拓扑数据读取失败'));
      if (controller.signal.aborted) return;
      modelRef.current = body.data.routing.model;
      setSnapshot(body.data);
      setError('');
    } catch (failure) {
      if (!controller.signal.aborted) setError(failure.message);
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [adminFetch]);

  useEffect(() => {
    if (!isOpen) return undefined;
    refresh();
    const timer = autoRefresh ? setInterval(refresh, 10000) : null;
    return () => {
      clearInterval(timer);
      requestRef.current?.abort();
    };
  }, [isOpen, selectedModel, autoRefresh, refresh]);

  useEffect(() => () => {
      probeRef.current?.abort();
      probeRef.current = null;
      setProbing('');
  }, [isOpen, selectedModel]);

  useEffect(() => {
    if (isOpen) return;
    modelRef.current = '';
    setSelectedModel('');
    setSnapshot(null);
    setProbes({});
    setError('');
  }, [isOpen]);

  const selectModel = (value) => {
    if (value === modelRef.current) return;
    requestRef.current?.abort();
    probeRef.current?.abort();
    probeRef.current = null;
    modelRef.current = value;
    setSelectedModel(value);
    setSnapshot(null);
    setProbes({});
    setProbing('');
    setError('');
  };

  const probe = async (model, channelId) => {
    if (probeRef.current) return;
    const controller = new AbortController();
    probeRef.current = controller;
    const key = `${model}:${channelId}`;
    setProbing(key);
    try {
      const response = await adminFetch('/api/v1/admin/models/topology/probe', {
        method: 'POST', signal: controller.signal, headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model, channel_id: channelId }),
      });
      const body = await response.json();
      if (!response.ok || body.code !== 0) throw new Error(extractErrorMessage(body, '模型验证失败'));
      if (!controller.signal.aborted) setProbes(previous => ({ ...previous, [key]: body.data }));
    } catch (failure) {
      if (!controller.signal.aborted) setProbes(previous => ({ ...previous, [key]: { success: false, error: failure.message } }));
    } finally {
      if (probeRef.current === controller) { probeRef.current = null; setProbing(''); }
    }
  };

  return { snapshot, loading, error, refresh, autoRefresh, setAutoRefresh, selectModel, probing, probes, probe };
}
