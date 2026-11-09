import { useState } from 'react';
import { useConsoleKeySelection } from './useConsoleKeySelection';
import { inferClientModality } from '../lib/modelModality';
import { availableKeyModels } from '../lib/keyModels';

export default function usePlaygroundCatalog({
  initialModality, initialModel, isZh, keys, modelRoutes, models, prices, showToast
}) {
  const [playModality, setPlayModality] = useState(initialModality || 'chat');

  const [playMode, setPlayMode] = useState('single'); // 'single' | 'arena'
  const { availableKeys: contextKeys, selectedKey: selectedKeyObj, keyId: selectedKeyId, setKeyId: setSelectedKeyId } = useConsoleKeySelection(keys);
  const resolveCallCredentials = (modality, requestModels = []) => {
    if (!selectedKeyId) {
      showToast?.(isZh ? '请先创建并选择一把可用的 API 访问密钥。' : 'Create and select an active API key first.', 'warning');
      return null;
    }
    if (modality && requestModels.some(name => !name || !getPlaygroundModels(modality).includes(name))) {
      showToast?.(isZh ? '所选密钥暂无可用模型，请切换密钥或配置模型权限。' : 'No available model for this key. Select another key or update its model access.', 'warning');
      return null;
    }
    return { keyId: selectedKeyId };
  };

  const getPlaygroundModels = (modality) => {
    let list = [];
    if (modelRoutes && modelRoutes.length > 0) {
      const matched = modelRoutes.filter(r => r.modality === modality).map(r => r.model);
      if (matched.length > 0) list = matched;
    }
    if (list.length === 0 && models && models.length > 0) {
      const matched = models.filter(m => inferClientModality(m) === modality);
      if (matched.length > 0) list = matched;
      else if (modality === 'chat') list = models.filter(m => inferClientModality(m) === 'chat');
    }

    if (selectedKeyObj && selectedKeyObj.allowed_models && selectedKeyObj.allowed_models.length > 0) {
      if (!selectedKeyObj.allowed_models.includes('*')) {
        return availableKeyModels(selectedKeyObj, list);
      }
    }
    return list;
  };

  const chatModels = getPlaygroundModels('chat');
  const imgModels = getPlaygroundModels('images');
  const ttsModels = getPlaygroundModels('audio_speech');
  const sttModels = getPlaygroundModels('audio_transcription');
  const videoModels = getPlaygroundModels('videos');
  const embedModels = getPlaygroundModels('embeddings');
  const rerankModels = getPlaygroundModels('rerank');

  // Single Model state
  const [playModel, setPlayModel] = useState(initialModel || (chatModels[0] || ''));
  const [arenaModelB, setArenaModelB] = useState(chatModels[1] || chatModels[0] || '');

  // Real off-peak state for the currently selected chat model.
  //
  // The footer used to print a hardcoded "闲时费率 5 折激活中" banner regardless
  // of the model's actual configuration, which is a claim the UI cannot support
  // without the pricing row. We now read it from the pricing data and show
  // nothing when the model has no off-peak discount configured.
  //
  // This must stay below the playModel declaration: reading a `const` state
  // binding before its initialization throws a TDZ ReferenceError that kills
  // the whole playground on mount.
  const activeChatModel = playModel || (models.length > 0 ? models[0] : '');
  const activePrice = prices.find(p => p.model === activeChatModel && (p.group_name || 'default') === 'default')
    || prices.find(p => p.model === activeChatModel);
  const offPeak = (() => {
    if (!activePrice || activePrice.off_peak_enabled === false) return null;
    const discount = typeof activePrice.off_peak_discount === 'number' ? activePrice.off_peak_discount : null;
    if (discount === null || discount >= 1) return null;
    return { discount, label: `${Math.round(discount * 10)}` };
  })();
  return {
    activeChatModel, arenaModelB, chatModels, contextKeys, embedModels, imgModels, offPeak, playModality,
    playMode, playModel, rerankModels, resolveCallCredentials, selectedKeyId, selectedKeyObj, setArenaModelB,
    setPlayModality, setPlayMode, setPlayModel, setSelectedKeyId, sttModels, ttsModels, videoModels
  };
}
