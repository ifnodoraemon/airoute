import { describe, expect, it } from 'vitest';
import { assistantHistory, reasoningTokens, thinkingRequest, thinkingResult } from './thinking';

describe('thinking contracts', () => {
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
  it('retains signed protocol history and explicit DeepSeek reasoning without replaying UI metadata', () => {
    const message={role:'assistant',content:'answer',reasoning:'summary',extra_content:{google:{state_ref:'owned'}},thinking:{reasoning_tokens:12},isError:false};
    expect(assistantHistory(message)).toEqual({role:'assistant',content:'answer',extra_content:message.extra_content});
    expect(assistantHistory(message,true).reasoning_content).toBe('summary');
    expect(assistantHistory({...message,model:'model-a'},true,'model-b').extra_content).toBeUndefined();
    expect(thinkingResult({choices:[],usage:{reasoning_tokens:0}},{mode:'off'})).toEqual({mode:'off',reasoning_tokens:0});
  });
});
