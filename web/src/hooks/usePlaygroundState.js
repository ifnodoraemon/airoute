import { useState, useEffect, useRef } from 'react';

export default function usePlaygroundState({
  arenaModelB, chatModels, embedModels, imgModels, initialModality, initialModel, initialScenario, isZh,
  playModel, rerankModels, setArenaModelB, setPlayModality, setPlayModel, sttModels, ttsModels, videoModels
}) {
  const [playStream, setPlayStream] = useState(true);
  const [thinkingByModel,setThinkingByModel]=useState({});
  const [playThinking,setPlayThinking]=useState(null);
  const [arenaThinkingB,setArenaThinkingB]=useState(null);
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState(2048);
  const [systemPrompt, setSystemPrompt] = useState(
    initialScenario?.systemPrompt || (isZh ? '你是一个极具专业素养的 AI 架构专家与全栈工程师，回答请严谨、精炼、富有洞见。' : 'You are a highly professional AI architecture expert and full-stack engineer. Answer concisely and with precision.')
  );
  const [showSystemPrompt, setShowSystemPrompt] = useState(!!initialScenario);

  // Chat conversation history & streaming output
  const [messages, setMessages] = useState([]);
  const [chatInput, setChatInput] = useState(initialScenario?.prompt || '');
  const [playLoading, setPlayLoading] = useState(false);
  // `playStopped` records a user-initiated cancellation so the UI can show a
  // neutral state instead of an error; `abortRef` holds the in-flight request.
  const [playStopped, setPlayStopped] = useState(false);
  const abortRef = useRef(null);
  useEffect(() => () => abortRef.current?.abort(), []);
  const chatInputRef = useRef(null);

  // Single Model Output & Live Telemetry
  const [playOutput, setPlayOutput] = useState('');
  const [playReasoningOutput, setPlayReasoningOutput] = useState('');
  const [playTTFTMs, setPlayTTFTMs] = useState(0);
  const [playDurationMs, setPlayDurationMs] = useState(0);
  const [playTokensPerSec, setPlayTokensPerSec] = useState(0);

  // Arena Contender B Output & Telemetry
  const [arenaOutputB, setArenaOutputB] = useState('');
  const [arenaReasoningB, setArenaReasoningB] = useState('');
  const [arenaTTFTB, setArenaTTFTB] = useState(0);
  const [arenaDurationB, setArenaDurationB] = useState(0);
  const [arenaTokensPerSecB, setArenaTokensPerSecB] = useState(0);

  // Multimodal sub-states
  const [imgModel, setImgModel] = useState(imgModels[0] || '');
  const [imgPrompt, setImgPrompt] = useState('');
  const [imgSize, setImgSize] = useState('1024x1024');
  const imgQuality = 'standard';
  const [imgResult, setImgResult] = useState(null);

  const [ttsModel, setTtsModel] = useState(ttsModels[0] || '');
  const [ttsVoice, setTtsVoice] = useState('alloy');
  const ttsSpeed = 1.0;
  const [ttsInput, setTtsInput] = useState('');
  const [ttsAudioUrl, setTtsAudioUrl] = useState(null);
  // Mirror of the synthesised audio object URL so unmount can release it. The
  // setter below only revokes the *previous* URL; without this cleanup, leaving
  // the view leaked one Blob URL per TTS generation.
  const ttsAudioUrlRef = useRef(null);
  useEffect(() => {
    ttsAudioUrlRef.current = ttsAudioUrl;
  }, [ttsAudioUrl]);
  useEffect(() => () => {
    if (ttsAudioUrlRef.current) URL.revokeObjectURL(ttsAudioUrlRef.current);
  }, []);

  // STT (audio transcription)
  const [sttModel, setSttModel] = useState(sttModels[0] || '');
  const [sttFile, setSttFile] = useState(null);
  const [sttLanguage, setSttLanguage] = useState('');
  const [sttResult, setSttResult] = useState(null);

  // Video generation (async task + polling)
  const [videoModel, setVideoModel] = useState(videoModels[0] || '');
  const [videoPrompt, setVideoPrompt] = useState('');
  const [videoResult, setVideoResult] = useState(null);
  const [videoTaskId, setVideoTaskId] = useState('');

  // Embeddings
  const [embedModel, setEmbedModel] = useState(embedModels[0] || '');
  const [embedInput, setEmbedInput] = useState('');
  const [embedResult, setEmbedResult] = useState(null);

  // Rerank
  const [rerankModel, setRerankModel] = useState(rerankModels[0] || '');
  const [rerankQuery, setRerankQuery] = useState('');
  const [rerankDocs, setRerankDocs] = useState('');
  const [rerankResult, setRerankResult] = useState(null);

  useEffect(() => {
    const setters = { chat: setPlayModel, images: setImgModel, audio_speech: setTtsModel, audio_transcription: setSttModel, videos: setVideoModel, embeddings: setEmbedModel, rerank: setRerankModel };
    if (initialModel) (setters[initialModality] || setPlayModel)(initialModel);
    setPlayModality(initialModality || 'chat');
  }, [initialModel, initialModality, setPlayModel, setPlayModality]);

  // Sync initial model across chat and multimodal categories
  useEffect(() => {
    if (!chatModels.includes(playModel)) {
      setPlayModel(chatModels[0] || '');
    }
    if (!chatModels.includes(arenaModelB)) {
      setArenaModelB(chatModels[1] || chatModels[0] || '');
    }
    if (!imgModels.includes(imgModel)) {
      setImgModel(imgModels[0] || '');
    }
    if (!ttsModels.includes(ttsModel)) {
      setTtsModel(ttsModels[0] || '');
    }
    if (!sttModels.includes(sttModel)) {
      setSttModel(sttModels[0] || '');
    }
    if (!videoModels.includes(videoModel)) {
      setVideoModel(videoModels[0] || '');
    }
    if (!embedModels.includes(embedModel)) {
      setEmbedModel(embedModels[0] || '');
    }
    if (!rerankModels.includes(rerankModel)) {
      setRerankModel(rerankModels[0] || '');
    }
  }, [chatModels, playModel, arenaModelB, imgModels, imgModel, ttsModels, ttsModel, sttModels, sttModel, videoModels, videoModel, embedModels, embedModel, rerankModels, rerankModel, setPlayModel, setArenaModelB]);
  return {
    abortRef, arenaDurationB, arenaOutputB, arenaReasoningB, arenaTTFTB, arenaThinkingB, arenaTokensPerSecB,
    chatInput, chatInputRef, embedInput, embedModel, embedResult, imgModel, imgPrompt, imgQuality, imgResult,
    imgSize, maxTokens, messages, playDurationMs, playLoading, playOutput, playReasoningOutput, playStopped,
    playStream, playTTFTMs, playThinking, playTokensPerSec, rerankDocs, rerankModel, rerankQuery, rerankResult,
    setArenaDurationB, setArenaOutputB, setArenaReasoningB, setArenaTTFTB, setArenaThinkingB,
    setArenaTokensPerSecB, setChatInput, setEmbedInput, setEmbedModel, setEmbedResult, setImgModel,
    setImgPrompt, setImgResult, setImgSize, setMaxTokens, setMessages, setPlayDurationMs, setPlayLoading,
    setPlayOutput, setPlayReasoningOutput, setPlayStopped, setPlayStream, setPlayTTFTMs, setPlayThinking,
    setPlayTokensPerSec, setRerankDocs, setRerankModel, setRerankQuery, setRerankResult, setShowSystemPrompt,
    setSttFile, setSttLanguage, setSttModel, setSttResult, setSystemPrompt, setTemperature, setThinkingByModel,
    setTtsAudioUrl, setTtsInput, setTtsModel, setTtsVoice, setVideoModel, setVideoPrompt, setVideoResult,
    setVideoTaskId, showSystemPrompt, sttFile, sttLanguage, sttModel, sttResult, systemPrompt, temperature,
    thinkingByModel, ttsAudioUrl, ttsInput, ttsModel, ttsSpeed, ttsVoice, videoModel, videoPrompt, videoResult,
    videoTaskId
  };
}
