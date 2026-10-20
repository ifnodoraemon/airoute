// Infers a model's modality from its name.
//
// Extracted from App.jsx, where it was a local function used by the console data
// loader to work out which models can serve the playground. It is a pure string
// mapping, so it lives on its own and can be tested directly.

export function inferClientModality(modelName) {
  const lower = (modelName || '').toLowerCase();
  if (lower.includes('rerank') || lower.includes('bge-reranker')) return 'rerank';
  if (lower.includes('embed') || lower.includes('bge-') || lower.includes('text-embedding')) return 'embeddings';
  if (lower.includes('tts') || lower.includes('speech') || lower.includes('voice') || lower.includes('cosyvoice')) return 'audio_speech';
  if (lower.includes('whisper') || lower.includes('sensevoice') || lower.includes('transcription') || lower.includes('asr')) return 'audio_transcription';
  if (lower.includes('seedance') || lower.includes('sora') || lower.includes('wan') || lower.includes('cogvideox') || lower.includes('kling') || lower.includes('luma') || lower.includes('runway') || lower.includes('pika') || lower.includes('vidu') || lower.includes('video')) return 'videos';
  if (lower.includes('image') || lower.includes('dall-e') || lower.includes('flux') || lower.includes('midjourney') || lower.includes('stable-diffusion') || lower.includes('seedream') || lower.includes('sdxl')) return 'images';
  return 'chat';
}
