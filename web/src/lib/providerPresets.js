// Provider onboarding templates.
//
// Extracted from App.jsx, where a 198-line switch of literals sat in the middle of
// the component. It is data plus a merge, so it belongs on its own and can be
// exercised without rendering anything.
//
// A preset never invents a credential or an endpoint: it fills in the model list
// and protocol set for a known provider, and keeps whatever base_url and api_key
// the operator has already typed.

// The full protocol *interface* set a channel starts with. `anthropic_native` is
// deliberately absent: it is a marker for an upstream that already speaks the
// Anthropic protocol, so it must be opted into per channel instead of being
// claimed by every new channel.
export const ALL_PROTOCOLS = [
  'openai_chat', 'openai_response', 'openai_text', 'anthropic_messages',
  'embeddings', 'rerank', 'images', 'audio_speech', 'audio_transcription', 'videos',
];

// A blank channel, as the create form starts and as the form resets after a save.
export function emptyChannel() {
  return {
    name: '',
    type: 'openai',
    base_url: '',
    // The form never carries an existing credential: `api_key` holds only a value
    // the operator typed in this session, and `api_key_configured` says whether the
    // channel already has one on the server (so "leave blank" means "keep it").
    api_key: '',
    api_key_configured: false,
    priority: 1,
    weight: 10,
    timeout_seconds: 60,
    models_str: '',
    mapping_rules: [],
    protocols: [...ALL_PROTOCOLS],
  };
}

const CHAT_ONLY = ['openai_chat', 'openai_response', 'openai_text'];
const CHAT_AND_TOOLS = ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'rerank', 'images'];

export const PROVIDER_PRESETS = {
  gpustack: {
    patch: {
      name: 'gpustack-cluster',
      type: 'gpustack',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      mapping_rules: [],
      protocols: CHAT_AND_TOOLS,
    },
    zh: '已载入 GPUStack 企业私有算力模板，点击探测或填入 API Key 即可拉取模型',
    en: 'Loaded GPUStack enterprise template. Click Probe or enter API Key to sync models.',
    keepEndpoint: true,
    keepModels: true,
  },
  deepseek: {
    patch: {
      name: 'deepseek-official',
      type: 'deepseek',
      base_url: 'https://api.deepseek.com',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'deepseek-v4.1-flash, deepseek-v4-pro, deepseek-r1, deepseek-chat, deepseek-reasoner',
      mapping_rules: [],
      protocols: CHAT_ONLY,
    },
    zh: '已载入 DeepSeek 官方模板及模型列表',
    en: 'Loaded DeepSeek official template and models',
  },
  zhipu: {
    patch: {
      name: 'zhipu-glm-official',
      type: 'openai',
      base_url: 'https://open.bigmodel.cn/api/paas/v4',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'glm-5.3, glm-5.3-flash, glm-4-plus, cogvideox-5b, cogview-3-plus',
      mapping_rules: [],
      protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'images', 'videos'],
    },
    zh: '已载入 智谱 GLM 官方模板及模型列表',
    en: 'Loaded Zhipu GLM official template and models',
  },
  doubao: {
    patch: {
      name: 'volcengine-doubao',
      type: 'openai',
      base_url: 'https://ark.cn-beijing.volces.com/api/v3',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'doubao-seed-2.1-pro, doubao-seed-2.1-turbo, doubao-1.5-pro, seedance-2.5',
      mapping_rules: [],
      protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'images', 'audio_speech', 'videos'],
    },
    zh: '已载入 字节火山豆包 官方模板及模型列表',
    en: 'Loaded Volcengine Doubao official template and models',
  },
  moonshot: {
    patch: {
      name: 'moonshot-kimi',
      type: 'openai',
      base_url: 'https://api.moonshot.cn/v1',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'kimi-k3, kimi-k3.1, kimi-latest, moonshot-v1-auto',
      mapping_rules: [],
      protocols: CHAT_ONLY,
    },
    zh: '已载入 月之暗面 Kimi 官方模板及模型列表',
    en: 'Loaded Moonshot Kimi official template and models',
  },
  openai: {
    patch: {
      name: 'openai-official',
      type: 'openai',
      base_url: 'https://api.openai.com/v1',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'gpt-6, gpt-6-luna, gpt-6.1-sol, gpt-5.5-instant, gpt-image-2.5, gpt-live-transcribe, text-embedding-3-small, whisper-large-v3-turbo',
      mapping_rules: [],
      protocols: [...ALL_PROTOCOLS],
    },
    zh: '已载入 OpenAI 官方模板及全能力模型',
    en: 'Loaded OpenAI official template and all-capability models',
  },
  anthropic: {
    patch: {
      name: 'anthropic-claude',
      type: 'anthropic',
      base_url: 'https://api.anthropic.com',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'claude-opus-5.5, claude-sonnet-5.5, claude-haiku-5.5, claude-3-7-sonnet',
      mapping_rules: [],
      // The official Anthropic endpoint speaks the Claude protocol natively, so the
      // template marks it instead of leaving the operator to guess.
      protocols: ['openai_chat', 'openai_response', 'anthropic_messages', 'anthropic_native'],
    },
    zh: '已载入 Claude 官方模板及模型列表',
    en: 'Loaded Claude official template and models',
  },
  gemini: {
    patch: {
      name: 'google-gemini',
      type: 'gemini',
      base_url: 'https://generativelanguage.googleapis.com',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'gemini-4-argon, gemini-3.8-flash, gemini-3.5-flash-lite, gemini-2.0-flash, embeddinggemma-2',
      mapping_rules: [],
      protocols: ['openai_chat', 'openai_response', 'anthropic_messages', 'embeddings'],
    },
    zh: '已载入 Google Gemini 官方模板及模型列表',
    en: 'Loaded Google Gemini official template and models',
  },
  ollama: {
    patch: {
      name: 'ollama-service',
      type: 'ollama',
      api_key: 'ollama',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'llama3.3, qwen3.8:27b, deepseek-r1:8b',
      mapping_rules: [],
      protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings'],
    },
    zh: '已载入 Ollama 模型列表模板，请输入您的端点地址',
    en: 'Loaded Ollama template. Please enter your endpoint URL',
    keepEndpoint: true,
  },
  vllm: {
    patch: {
      name: 'vllm-service',
      type: 'vllm',
      api_key: 'none',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: 'Qwen/Qwen3.8-27B-Instruct, deepseek-ai/DeepSeek-V4.1-Flash',
      mapping_rules: [],
      protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings'],
    },
    zh: '已载入 vLLM 模型列表模板，请输入您的端点地址',
    en: 'Loaded vLLM template. Please enter your endpoint URL',
    keepEndpoint: true,
  },
  sub2api: {
    patch: {
      name: 'sub2api-upstream',
      type: 'sub2api',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: '',
      mapping_rules: [],
      protocols: [...ALL_PROTOCOLS],
    },
    zh: '已载入 Sub2API 聚合模板',
    en: 'Loaded Sub2API aggregation template',
    keepEndpoint: true,
  },
  custom: {
    patch: {
      name: 'custom-downstream',
      type: 'custom',
      priority: 2,
      weight: 10,
      timeout_seconds: 60,
      models_str: '',
      mapping_rules: [],
      protocols: ['openai_chat', 'openai_response', 'embeddings', 'rerank', 'images', 'audio_speech', 'videos'],
    },
    zh: '已载入自定义下游模板，请输入您的服务端点',
    en: 'Loaded custom downstream template. Please enter endpoint URL',
    keepEndpoint: true,
  },
};

/**
 * Merges a preset onto the channel currently in the form.
 *
 * The operator's own endpoint and credential are preserved: a preset supplies the
 * model list and protocol set for a provider, not the connection details.
 *
 * @returns {{ channel: object, messageZh: string, messageEn: string } | null}
 *          null when the key is unknown, so the caller can do nothing.
 */
export function buildPresetChannel(key, previous) {
  const preset = PROVIDER_PRESETS[key];
  if (!preset) return null;

  const next = { ...previous, ...preset.patch };
  // Keep whatever the operator already typed for the connection.
  next.base_url = previous.base_url || preset.patch.base_url || '';
  next.api_key = previous.api_key || preset.patch.api_key || '';
  if (preset.keepModels) next.models_str = previous.models_str || '';
  return { channel: next, messageZh: preset.zh, messageEn: preset.en };
}
