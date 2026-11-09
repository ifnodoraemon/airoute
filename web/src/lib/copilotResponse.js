import { consoleFetch } from '../api/console';
import { extractErrorMessage } from '../api/client';
import { thinkingRequest, thinkingOutputLimit } from './thinking';
import { COPILOT_HARNESS_TOOLS } from '../components/copilotHarness';

export async function fetchCopilotResponse(options, run, messages) {
  const capability = options.thinkingCapabilities[run.model];
  const thinking = options.thinkingByModel[run.model];
  const response = await consoleFetch('/v1/chat/completions', run.keyId, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    signal: run.controller.signal,
    body: JSON.stringify({
      model: run.model, ...thinkingRequest(capability, thinking),
      stream_options: { include_usage: true }, messages,
      tools: COPILOT_HARNESS_TOOLS, stream: true, temperature: 0.5,
      max_tokens: thinkingOutputLimit(capability, thinking, 16384),
    }),
  });
  if (!response.ok || !response.body) throw await copilotResponseError(response);
  return response;
}

async function copilotResponseError(response) {
  let detail = '';
  let gatewayCode = '';
  try {
    const body = await response.text();
    try {
      const parsed = JSON.parse(body);
      detail = extractErrorMessage(parsed, '');
      gatewayCode = parsed?.error?.code || parsed?.error?.type || parsed?.code || '';
    } catch { detail = (body || '').trim().slice(0, 300); }
  } catch { detail = ''; }
  const failure = new Error(detail || `HTTP ${response.status}`);
  failure.gatewayCode = gatewayCode;
  failure.status = response.status;
  return failure;
}
