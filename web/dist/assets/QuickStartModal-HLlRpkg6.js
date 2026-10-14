import{n as e}from"./rolldown-runtime-CbXtAM7H.js";import{At as t,Bt as n,It as r,_ as i,_t as a,n as o,p as s,xt as c,yt as l}from"./icons-DoBEcx0S.js";import{t as u}from"./vendor-BpAbo39q.js";import{n as d}from"./index-vP1hu_Rl.js";import{t as f}from"./useModalA11y-CVMnY7Ii.js";var p=e(n(),1),m=u();function h({apiKey:e,onClose:n,onCopy:u,lang:h=`zh`,t:g}){if(f(!!e,n),!e)return null;let _=h===`zh`,[v,y]=(0,p.useState)(`cursor`),[b,x]=(0,p.useState)(``),S=d(),C=S?`${S}/v1`:`/v1`,w=e.key||`sk-airoute-xxxx`,T=`from openai import OpenAI

client = OpenAI(
    base_url="${C}",
    api_key="${w}"
)

# 1. 聊天补全 (OpenAI / DeepSeek / Claude / Gemini 统一协议)
response = client.chat.completions.create(
    model="deepseek-chat",
    messages=[
        {"role": "system", "content": "You are a helpful coding assistant."},
        {"role": "user", "content": "用 Go 实现快速排序"}
    ],
    stream=True
)

for chunk in response:
    content = chunk.choices[0].delta.content or ""
    print(content, end="", flush=True)

# 2. 官方新代智能体 Responses API (/v1/responses)
# resp = client.responses.create(
#     model="gpt-6",
#     input="分析微服务与单体架构的选型考量",
#     instructions="You are an enterprise system architect."
# )
# print(resp.output[0].content[0].text)
`,E=`# 1. 测试 Chat Completions (对话流式补全)
curl -X POST "${C}/chat/completions" \\
  -H "Authorization: Bearer ${w}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "你好，请做个自我介绍"}],
    "stream": true
  }'

# 2. 测试 OpenAI Responses API (新代智能体协议 /v1/responses)
curl -X POST "${C}/responses" \\
  -H "Authorization: Bearer ${w}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "deepseek-chat",
    "input": "请用一句话介绍微服务网关的核心价值",
    "instructions": "You are an infrastructure expert.",
    "stream": true
  }'

# 3. 探测已挂载模型列表
curl -X GET "${C}/models" \\
  -H "Authorization: Bearer ${w}"
`,D=`import OpenAI from "openai";

const openai = new OpenAI({
  baseURL: "${C}",
  apiKey: "${w}",
});

async function main() {
  const stream = await openai.chat.completions.create({
    model: "deepseek-chat",
    messages: [{ role: "user", content: "解释量子计算的基本原理" }],
    stream: true,
  });

  for await (const chunk of stream) {
    process.stdout.write(chunk.choices[0]?.delta?.content || "");
  }
}

main();
`,O=`# ==============================================================================
# Cursor IDE AI 编程助手开箱配置指南
# ==============================================================================

# 步骤 1：打开 Cursor 设置 (Settings -> Models)
# 步骤 2：开启 "OpenAI API Key" 并配置自定义 Base URL：
Override OpenAI Base URL: ${C}
API Key: ${w}

# 步骤 3：在模型列表中添加当前网关已启用的模型：
# - deepseek-chat
# - deepseek-reasoner
# - claude-3-5-sonnet
# - gpt-4o

# 步骤 4：(推荐) 在项目根目录创建 .cursorrules 注入网关规则：
# """
# All LLM completions route through high-availability gateway (Airoute).
# Zero-hallucination principle: Keep code accurate and self-contained.
# """
`,k=`# ==============================================================================
# 1. Claude Code CLI 终端配置 (Anthropic 协议透明分发)
# ==============================================================================
export ANTHROPIC_BASE_URL="${S}"
export ANTHROPIC_API_KEY="${w}"

# 终端直接启动
claude

# ==============================================================================
# 2. Claude Desktop MCP 配置 (~/.config/Claude/claude_desktop_config.json)
# ==============================================================================
# {
#   "mcpServers": {
#     "airoute-gateway": {
#       "command": "npx",
#       "args": ["-y", "@modelcontextprotocol/server-fetch"],
#       "env": {
#         "AIROUTE_BASE_URL": "${S}",
#         "AIROUTE_API_KEY": "${w}"
#       }
#     }
#   }
# }
`,A=`# ==============================================================================
# LangChain & LlamaIndex AI 智能体框架调用示例
# ==============================================================================
from langchain_openai import ChatOpenAI
from langchain_core.messages import HumanMessage, SystemMessage

# 初始化模型客户端 (经由 Airoute 自适应负载均衡与断路器调度)
llm = ChatOpenAI(
    base_url="${C}",
    api_key="${w}",
    model="deepseek-chat",
    temperature=0.3,
    streaming=True
)

messages = [
    SystemMessage(content="You are an enterprise AI system architect."),
    HumanMessage(content="请比较双活负载均衡与主备故障转移的权衡考量。")
]

# 流式生成推导
for chunk in llm.stream(messages):
    print(chunk.content, end="", flush=True)
`,j=`# 常用第三方客户端配置指南 (NextChat / Chatbox / LobeChat / Cherry Studio)
# 
# 1. 接口地址 (API Host / Base URL):
#    ${C}
# 
# 2. API 访问密钥 (API Key):
#    ${w}
# 
# 3. 自定义模型列表:
#    在客户端模型设置中填入当前网关已启用的模型名称 (例如 deepseek-chat, gpt-4o, claude-3-5-sonnet)
# 
# 4. 特色体验:
#    网关原生支持 DeepSeek-R1 思维链流式展开与全双工协议转换。
`,M=()=>{switch(v){case`cursor`:return O;case`claude`:return k;case`langchain`:return A;case`python`:return T;case`curl`:return E;case`node`:return D;case`client`:return j;default:return O}};return(0,m.jsx)(`div`,{className:`fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm animate-in fade-in`,onClick:e=>{e.target===e.currentTarget&&n()},children:(0,m.jsxs)(`div`,{className:`bg-white dark:bg-[#111726] border border-slate-200 dark:border-slate-800 rounded-2xl w-full max-w-3xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]`,children:[(0,m.jsxs)(`div`,{className:`p-6 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between bg-slate-50/50 dark:bg-slate-900/50`,children:[(0,m.jsxs)(`div`,{className:`flex items-center space-x-3`,children:[(0,m.jsx)(`div`,{className:`w-10 h-10 rounded-xl bg-indigo-50 dark:bg-indigo-900/30 border border-indigo-200 dark:border-indigo-700/50 flex items-center justify-center text-indigo-600 dark:text-indigo-400`,children:(0,m.jsx)(i,{className:`w-5 h-5`})}),(0,m.jsxs)(`div`,{children:[(0,m.jsx)(`h3`,{className:`font-bold text-base text-slate-900 dark:text-slate-100 tracking-tight`,children:_?`接入代码示例`:`Integration Code Examples`}),(0,m.jsxs)(`p`,{className:`text-xs text-slate-500 dark:text-slate-400 mt-0.5`,children:[_?`密钥: `:`Key: `,(0,m.jsx)(`code`,{className:`font-mono text-indigo-600 dark:text-indigo-400 font-semibold`,children:w}),` (`,_?`应用: `:`App: `,e.tenant_id,`)`]})]})]}),(0,m.jsx)(`button`,{onClick:n,className:`p-2 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition`,children:(0,m.jsx)(o,{className:`w-5 h-5`})})]}),(0,m.jsxs)(`div`,{className:`px-6 pt-4 flex items-center justify-between border-b border-slate-100 dark:border-slate-800/80 bg-white dark:bg-[#111726]`,children:[(0,m.jsxs)(`div`,{className:`flex flex-wrap gap-2`,children:[(0,m.jsxs)(`button`,{onClick:()=>y(`cursor`),className:`px-3.5 py-2 text-xs font-semibold rounded-lg transition flex items-center space-x-1.5 cursor-pointer ${v===`cursor`?`bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60`:`text-slate-500 hover:text-slate-800 dark:hover:text-slate-200`}`,children:[(0,m.jsx)(i,{className:`w-3.5 h-3.5 text-amber-500`}),(0,m.jsx)(`span`,{children:`Cursor IDE`})]}),(0,m.jsxs)(`button`,{onClick:()=>y(`claude`),className:`px-3.5 py-2 text-xs font-semibold rounded-lg transition flex items-center space-x-1.5 cursor-pointer ${v===`claude`?`bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60`:`text-slate-500 hover:text-slate-800 dark:hover:text-slate-200`}`,children:[(0,m.jsx)(s,{className:`w-3.5 h-3.5`}),(0,m.jsx)(`span`,{children:`Claude Code / MCP`})]}),(0,m.jsxs)(`button`,{onClick:()=>y(`langchain`),className:`px-3.5 py-2 text-xs font-semibold rounded-lg transition flex items-center space-x-1.5 cursor-pointer ${v===`langchain`?`bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60`:`text-slate-500 hover:text-slate-800 dark:hover:text-slate-200`}`,children:[(0,m.jsx)(a,{className:`w-3.5 h-3.5`}),(0,m.jsx)(`span`,{children:`LangChain / Agents`})]}),(0,m.jsxs)(`button`,{onClick:()=>y(`python`),className:`px-3.5 py-2 text-xs font-semibold rounded-lg transition flex items-center space-x-1.5 cursor-pointer ${v===`python`?`bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60`:`text-slate-500 hover:text-slate-800 dark:hover:text-slate-200`}`,children:[(0,m.jsx)(c,{className:`w-3.5 h-3.5`}),(0,m.jsx)(`span`,{children:`Python SDK`})]}),(0,m.jsxs)(`button`,{onClick:()=>y(`node`),className:`px-3.5 py-2 text-xs font-semibold rounded-lg transition flex items-center space-x-1.5 cursor-pointer ${v===`node`?`bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60`:`text-slate-500 hover:text-slate-800 dark:hover:text-slate-200`}`,children:[(0,m.jsx)(r,{className:`w-3.5 h-3.5`}),(0,m.jsx)(`span`,{children:`Node / TS`})]}),(0,m.jsxs)(`button`,{onClick:()=>y(`curl`),className:`px-3.5 py-2 text-xs font-semibold rounded-lg transition flex items-center space-x-1.5 cursor-pointer ${v===`curl`?`bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60`:`text-slate-500 hover:text-slate-800 dark:hover:text-slate-200`}`,children:[(0,m.jsx)(s,{className:`w-3.5 h-3.5`}),(0,m.jsx)(`span`,{children:`cURL`})]}),(0,m.jsx)(`button`,{onClick:()=>y(`client`),className:`px-3.5 py-2 text-xs font-semibold rounded-lg transition flex items-center space-x-1.5 cursor-pointer ${v===`client`?`bg-indigo-50 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700/60`:`text-slate-500 hover:text-slate-800 dark:hover:text-slate-200`}`,children:(0,m.jsx)(`span`,{children:_?`桌面客户端`:`Desktop Apps`})})]}),(0,m.jsx)(`button`,{onClick:()=>{u(M()),x(v),setTimeout(()=>x(``),2e3)},className:`px-3.5 py-1.5 text-xs rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white font-medium flex items-center space-x-1.5 transition shadow-sm`,children:b===v?(0,m.jsxs)(m.Fragment,{children:[(0,m.jsx)(t,{className:`w-3.5 h-3.5 text-emerald-300`}),(0,m.jsx)(`span`,{children:_?`已复制！`:`Copied!`})]}):(0,m.jsxs)(m.Fragment,{children:[(0,m.jsx)(l,{className:`w-3.5 h-3.5`}),(0,m.jsx)(`span`,{children:_?`复制代码`:`Copy Code`})]})})]}),(0,m.jsx)(`div`,{className:`p-6 overflow-y-auto flex-1 bg-slate-950 font-mono text-xs text-slate-200 leading-relaxed`,children:(0,m.jsx)(`pre`,{className:`whitespace-pre overflow-x-auto`,children:M()})}),(0,m.jsx)(`div`,{className:`p-4 bg-slate-50 dark:bg-slate-900/50 border-t border-slate-100 dark:border-slate-800/80 flex items-center justify-end text-xs`,children:(0,m.jsx)(`button`,{onClick:n,className:`px-4 py-2 bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-lg font-medium transition cursor-pointer`,children:_?`关闭`:`Close`})})]})})}export{h as default};