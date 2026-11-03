import { describe, it, expect } from 'vitest';
import { inferClientModality } from './modelModality';

// Modality decides which playground/API surface a model is offered on, so a
// misclassification routes audio requests to the wrong handler. The regression
// this file pins: "sensevoice" contains the substring "voice", so the generic
// speech/voice branch used to swallow it as text-to-speech before the
// transcription check could see it.

describe('inferClientModality audio models', () => {
  it('classifies SenseVoice as audio_transcription, not speech', () => {
    // The word "voice" inside "sensevoice" is the trap.
    expect(inferClientModality('sensevoice')).toBe('audio_transcription');
    expect(inferClientModality('iic/SenseVoiceSmall')).toBe('audio_transcription');
    expect(inferClientModality('FunAudioLLM/SenseVoice-Small')).toBe('audio_transcription');
  });

  it('classifies Whisper as audio_transcription', () => {
    expect(inferClientModality('whisper-1')).toBe('audio_transcription');
    expect(inferClientModality('whisper-large-v3')).toBe('audio_transcription');
  });

  it('keeps text-to-speech models as audio_speech', () => {
    expect(inferClientModality('tts-1')).toBe('audio_speech');
    expect(inferClientModality('tts-1-hd')).toBe('audio_speech');
    expect(inferClientModality('cosyvoice-v2')).toBe('audio_speech');
    expect(inferClientModality('qwen-tts')).toBe('audio_speech');
  });

  it('does not let the speech branch shadow other transcription aliases', () => {
    expect(inferClientModality('asr')).toBe('audio_transcription');
    expect(inferClientModality('gpt-4o-transcription')).toBe('audio_transcription');
  });
});
