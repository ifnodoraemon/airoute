import { describe, expect, it } from 'vitest';
import { assistantHistory, reasoningTokens, thinkingRequest, thinkingResult, thinkingOutputLimit, thinkingOptions, validThinkingEffort } from './thinking';

describe('thinking contracts', () => {
  it('excludes minimal from supplied capabilities and custom levels', () => {
    expect(thinkingOptions({ on: true, off: true, efforts: ['minimal', 'low', 'ultra'] })).toEqual(['auto', 'off', 'low', 'ultra']);
    expect(validThinkingEffort('minimal')).toBe(false);
    expect(validThinkingEffort('extended')).toBe(true);
  });
  it('keeps missing reasoning usage distinct from a reported zero across provider schemas', () => {
    expect(reasoningTokens({completion_tokens:10})).toBeNull();
    expect(reasoningTokens({reasoning_tokens:0})).toBe(0);
    expect(reasoningTokens({thoughtsTokenCount:121})).toBe(121);
    expect(reasoningTokens({completion_tokens_details:{reasoning_tokens:7}})).toBe(7);
  });
  it('retains explicit requirements across capability changes and requests summary independently', () => {
    const cap={known:true,on:true,off:false,efforts:['low','high'],summary:true};
    expect(thinkingRequest(cap,{mode:'on',effort:'high'})).toEqual({thinking_control:{mode:'on',effort:'high',include_summary:true}});
    expect(thinkingRequest(cap,{mode:'off'})).toEqual({thinking_control:{mode:'off',include_summary:true}});
    expect(thinkingRequest(undefined,undefined)).toEqual({});
  });
  it('reserves enough answer space for a required thinking budget without changing other models', () => {
    const cap={minimum_output_tokens:{default:1025,high:24577},policy:{default_mode:'on',default_effort:'high'}};
    expect(thinkingOutputLimit(cap,{mode:'on',effort:'high'},16384)).toBeGreaterThan(24577);
    expect(thinkingOutputLimit(cap,{mode:'auto'},16384)).toBeGreaterThan(24577);
    expect(thinkingOutputLimit(cap,{mode:'off'},16384)).toBe(16384);
    expect(thinkingOutputLimit({}, {mode:'on',effort:'high'},16384)).toBe(16384);
  });
  it('retains signed protocol history and explicit DeepSeek reasoning without replaying UI metadata', () => {
    const message={role:'assistant',content:'answer',reasoning:'summary',extra_content:{google:{state_ref:'owned'}},thinking:{reasoning_tokens:12},isError:false};
    expect(assistantHistory(message)).toEqual({role:'assistant',content:'answer',extra_content:message.extra_content});
    expect(assistantHistory(message,true).reasoning_content).toBe('summary');
    expect(assistantHistory({...message,model:'model-a'},true,'model-b').extra_content).toBeUndefined();
    expect(thinkingResult({choices:[],usage:{reasoning_tokens:0}},{mode:'off'})).toEqual({mode:'off',reasoning_tokens:0});
  });
});
