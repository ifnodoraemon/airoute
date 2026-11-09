import { fetchCopilotResponse } from './copilotResponse';
import { readCopilotStream } from './copilotStream';
import { executeHarnessTool } from '../components/copilotHarness';

export async function runCopilotIteration(options, run, messages) {
  const response = await fetchCopilotResponse(options, run, messages);
  const round = await readCopilotStream(response, options, run);
  const calls = Object.values(round.toolCalls).filter(call => call.name);
  if (!calls.length) return false;
  messages.push(assistantToolMessage(round, calls));
  for (const call of calls) {
    let args = {};
    try { args = JSON.parse(call.arguments || '{}'); } catch { args = {}; }
    const result = await executeHarnessTool(call.name, args, options.toolContext);
    run.trace.push(result);
    options.setMessages(previous => previous.map(message => message.id === run.id
      ? { ...message, harnessTrace: [...run.trace] } : message));
    messages.push({
      role: 'tool', tool_call_id: call.id, name: call.name,
      content: JSON.stringify(result.result || { error: result.error }),
    });
  }
  if (run.content && !run.content.endsWith('\n\n')) run.content += '\n\n';
  return true;
}

function assistantToolMessage(round, calls) {
  return {
    role: 'assistant', content: round.content, reasoning_content: round.reasoning,
    ...(round.extra ? { extra_content: round.extra } : {}),
    tool_calls: calls.map(call => ({
      id: call.id, type: 'function',
      ...(call.extra_content ? { extra_content: call.extra_content } : {}),
      function: { name: call.name, arguments: call.arguments },
    })),
  };
}
