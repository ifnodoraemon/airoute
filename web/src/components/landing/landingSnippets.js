export function getLandingSnippets(origin, isZh) {
  const prompt = isZh ? '请介绍统一 AI 网关的接入优势' : 'Explain the benefits of a unified AI gateway.';
  return {
    python: pythonSnippet(origin, prompt, isZh),
    claude: claudeSnippet(origin, prompt, isZh),
    curl: curlSnippet(origin, prompt, isZh),
    multimodal: multimodalSnippet(origin, isZh),
    session: sessionSnippet(isZh),
  };
}

function pythonSnippet(origin, prompt, isZh) {
  return `from openai import OpenAI

client = OpenAI(
    base_url="${origin}/v1",
    api_key="sk-airoute-your-api-key",
    default_headers={"X-Session-ID": "session_user_001"}
)

# ${isZh ? '使用部署中已配置的模型，例如 gpt-4.1、gemini-2.5-pro 或 deepseek-r1' : 'Use a configured model, such as gpt-4.1, gemini-2.5-pro, or deepseek-r1.'}
response = client.chat.completions.create(
    model="gpt-4.1",
    messages=[{"role": "user", "content": "${prompt}"}],
    stream=True,
)

for chunk in response:
    if chunk.choices:
        print(chunk.choices[0].delta.content or "", end="", flush=True)`;
}

function claudeSnippet(origin, prompt, isZh) {
  return `import anthropic

client = anthropic.Anthropic(
    base_url="${origin}",
    api_key="sk-airoute-your-api-key",
)

# ${isZh ? '通过 Messages API 调用已配置的 Claude 模型' : 'Call a configured Claude model through the Messages API.'}
message = client.messages.create(
    model="claude-sonnet-5-5",
    max_tokens=2048,
    messages=[{"role": "user", "content": "${prompt}"}]
)
print(message.content[0].text)`;
}

function curlSnippet(origin, prompt, isZh) {
  return `# ${isZh ? '标准对话推理与流式输出' : 'Chat Completions with streaming output.'}
curl -X POST "${origin}/v1/chat/completions" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -H "X-Session-ID: session_user_001" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4.1",
    "messages": [{"role": "user", "content": "${prompt}"}],
    "stream": true
  }'`;
}

function multimodalSnippet(origin, isZh) {
  return `# 1. ${isZh ? '图像生成；需先配置对应上游模型' : 'Image generation; configure the corresponding upstream model first.'}
curl -X POST "${origin}/v1/images/generations" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "gpt-image-1", "prompt": "${isZh ? '极简科技风 AI 路由器' : 'A minimalist AI router with a modern technology aesthetic'}", "size": "1024x1024"}'

# 2. ${isZh ? '音频转写' : 'Audio transcription.'}
curl -X POST "${origin}/v1/audio/transcriptions" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -F file="@speech.mp3" \\
  -F model="whisper-1"

# 3. ${isZh ? '视频生成与任务轮询' : 'Video generation and task polling.'}
curl -X POST "${origin}/v1/videos/generations" \\
  -H "Authorization: Bearer sk-airoute-your-api-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "cogvideox-5b", "prompt": "${isZh ? '未来城市的雨夜' : 'A rainy night in a futuristic city'}", "aspect_ratio": "16:9"}'

curl -X GET "${origin}/v1/videos/tasks/task_xxx" \\
  -H "Authorization: Bearer sk-airoute-your-api-key"`;
}

function sessionSnippet(isZh) {
  return isZh
      ? `# 会话黏连与 Prompt 缓存
# 在请求头中携带 X-Session-ID: <session_id>。
# 网关优先复用同一渠道；失败时可转移至备用渠道。
# 缓存命中、实际时延与折扣取决于上游和部署配置。`
      : `# Session affinity and prompt caching.
# Include X-Session-ID: <session_id> in request headers.
# The gateway prefers the same channel and can fail over to a backup.
# Cache hits, latency, and discounts depend on the upstream and deployment.`;
}

export function getSnippetTabs(isZh) {
  return [
    { id: 'python', label: 'Python (OpenAI)' },
    { id: 'claude', label: 'Anthropic Claude' },
    { id: 'curl', label: isZh ? 'cURL 命令行' : 'cURL CLI' },
    { id: 'multimodal', label: isZh ? '全模态 API' : 'Multimodal API' },
    { id: 'session', label: isZh ? '会话缓存' : 'Session Cache' },
  ];
}
