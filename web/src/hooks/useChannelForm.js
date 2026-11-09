

import { useCallback, useState, useRef, useEffect } from 'react';
import { emptyChannel } from '../lib/providerPresets';

export default function useChannelForm() {
  const [showChannelModal, setShowChannelModal] = useState(false);
  const [editingChannelId, setEditingChannelId] = useState(null);
  const [newChannel, setChannelDraft] = useState(() => emptyChannel());
  const [submittingChannel, setSubmittingChannel] = useState(false);
  const [probing, setProbing] = useState(false);
  const [probeAlert, setProbeAlert] = useState(null);
  const [testingId, setTestingId] = useState(null);
  const [batchTesting, setBatchTesting] = useState(false);
  const [channelLatencies, setChannelLatencies] = useState({});
  const [selectedChannelIds, setSelectedChannelIds] = useState([]);

  const probeRef = useRef(null);
  const cancelProbe = useCallback(() => {
    probeRef.current?.abort();
    probeRef.current = null;
    setProbing(false);
  }, []);
  const setNewChannel = useCallback((update) => {
    cancelProbe();
    setChannelDraft((previous) => {
      const next = typeof update === 'function' ? update(previous) : update;
      if (previous.base_url !== next.base_url || previous.type !== next.type) {
        const clean = { ...next };
        delete clean.detected_thinking_capabilities;
        return clean;
      }
      return next;
    });
  }, [cancelProbe]);
  useEffect(() => () => probeRef.current?.abort(), []);
  const setModalVisibility = useCallback((visible) => {
    cancelProbe();
    setShowChannelModal(visible);
  }, [cancelProbe]);
  return {
    batchTesting, cancelProbe, channelLatencies, editingChannelId, newChannel, probeAlert, probeRef, probing,
    selectedChannelIds, setBatchTesting, setChannelDraft, setChannelLatencies, setEditingChannelId,
    setModalVisibility, setNewChannel, setProbeAlert, setProbing, setSelectedChannelIds, setShowChannelModal,
    setSubmittingChannel, setTestingId, showChannelModal, submittingChannel, testingId
  };
}
