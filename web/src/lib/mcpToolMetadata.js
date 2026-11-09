

export const TOOL_TEMPLATES = {
  // 网关原生安全与审计
  airoute_data_redact: {
    label: 'airoute_data_redact (敏感数据脱敏与隐私合规)',
    group: '网关原生安全审计',
    defaultArgs: '{\n  "text": "客户张三 手机13812345678 身份证110101199003072345 密钥sk-abcdef123456"\n}'
  },
  airoute_sql_security_check: {
    label: 'airoute_sql_security_check (SQL高危注入拦截与安全审计)',
    group: '网关原生安全审计',
    defaultArgs: '{\n  "query": "DROP TABLE users; -- 注入攻击"\n}'
  },
  airoute_cluster_status: {
    label: 'airoute_cluster_status (集群高可用健康探活与指标)',
    group: '网关原生运维',
    defaultArgs: '{\n}'
  },
  airoute_model_topology: {
    label: 'airoute_model_topology (模型路由矩阵与拓扑视图)',
    group: '网关原生运维',
    defaultArgs: '{\n  "modality": "chat"\n}'
  },
  airoute_deep_search: {
    label: 'airoute_deep_search (高质量结构化联网深度检索)',
    group: '网关原生增强',
    defaultArgs: '{\n  "query": "DeepSeek R1 模型推理架构与性能",\n  "max_results": 5\n}'
  },
  airoute_recommend_model: {
    label: 'airoute_recommend_model (智能模型选型与吞吐仲裁)',
    group: '网关原生增强',
    defaultArgs: '{\n  "task_description": "企业级高并发智能选型与路由仲裁",\n  "priority": "quality"\n}'
  },
  airoute_query_logs: {
    label: 'airoute_query_logs (统一审计日志快速检索)',
    group: '网关原生运维',
    defaultArgs: '{\n  "limit": 10\n}'
  },

  // Puppeteer 无头浏览器
  puppeteer_navigate: {
    label: 'puppeteer_navigate (无头浏览器页面导航与加载)',
    group: 'Puppeteer 浏览器渲染',
    defaultArgs: '{\n  "url": "https://news.ycombinator.com"\n}'
  },
  puppeteer_screenshot: {
    label: 'puppeteer_screenshot (页面视口全景截图捕获)',
    group: 'Puppeteer 浏览器渲染',
    defaultArgs: '{\n  "name": "homepage_preview",\n  "full_page": false\n}'
  },
  puppeteer_click: {
    label: 'puppeteer_click (DOM 节点模拟点击与交互)',
    group: 'Puppeteer 浏览器渲染',
    defaultArgs: '{\n  "selector": "button.submit"\n}'
  },
  puppeteer_evaluate: {
    label: 'puppeteer_evaluate (沙箱安全 JavaScript 表达式求值)',
    group: 'Puppeteer 浏览器渲染',
    defaultArgs: '{\n  "script": "document.title"\n}'
  },

  // 关系型与嵌入式数据库
  read_query: {
    label: 'read_query (Postgres 只读安全 SQL 查询)',
    group: '数据库与持久存储',
    defaultArgs: '{\n  "query": "SELECT id, name, status, latency_ms FROM public.channels LIMIT 5;"\n}'
  },
  list_tables: {
    label: 'list_tables (枚举数据表元数据清单)',
    group: '数据库与持久存储',
    defaultArgs: '{\n}'
  },
  describe_table: {
    label: 'describe_table (获取数据表字段与约束模式)',
    group: '数据库与持久存储',
    defaultArgs: '{\n  "table_name": "channels"\n}'
  },
  query: {
    label: 'query (通用数据库查询执行)',
    group: '数据库与持久存储',
    defaultArgs: '{\n  "query": "SELECT 1 as test_conn;"\n}'
  },

  // 深度推理
  sequentialthinking: {
    label: 'sequentialthinking (思维链深层分步逻辑推理)',
    group: '深度反思与推理',
    defaultArgs: '{\n  "thought": "分析分布式网关集群在流量洪峰下的限流与缓存降级拓扑",\n  "thoughtNumber": 1,\n  "totalThoughts": 3\n}'
  },

  // GitHub / Git 研发协作
  search_repositories: {
    label: 'search_repositories (GitHub 仓库检索)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n  "query": "airoute language:Go"\n}'
  },
  create_issue: {
    label: 'create_issue (创建规范化协同工单)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n  "title": "网关 MCP 协议探针联调测试",\n  "body": "企业级统一接入网关协议探测已顺利通过验证"\n}'
  },
  get_file_contents: {
    label: 'get_file_contents (获取仓库代码文件详情)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n  "owner": "airoute",\n  "repo": "gateway",\n  "path": "README.md"\n}'
  },
  create_pull_request: {
    label: 'create_pull_request (创建代码拉取审查请求)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n  "title": "feat: mcp probe auth token",\n  "base": "main",\n  "head": "feature/mcp"\n}'
  },
  git_status: {
    label: 'git_status (查询本地 Git 仓库工作树状态)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n}'
  },
  git_diff: {
    label: 'git_diff (查看变更差异对比)',
    group: '代码协作与版本控制',
    defaultArgs: '{\n}'
  },

  // DevOps & 容器
  docker_ps: {
    label: 'docker_ps (枚举当前运行中容器列表)',
    group: 'DevOps 与容器运维',
    defaultArgs: '{\n}'
  },
  docker_logs: {
    label: 'docker_logs (检索容器最新标准输出日志)',
    group: 'DevOps 与容器运维',
    defaultArgs: '{\n  "container": "airoute-gateway-1",\n  "tail": 50\n}'
  },

  // 长期记忆与知识图谱
  create_entities: {
    label: 'create_entities (向长期记忆图谱写入实体节点)',
    group: '长期记忆与知识图谱',
    defaultArgs: '{\n  "entities": [\n    {"name": "airoute_gateway", "type": "infrastructure", "observations": ["HA dual node", "healthy"]}\n  ]\n}'
  },
  read_graph: {
    label: 'read_graph (读取跨会话完整实体拓扑图谱)',
    group: '长期记忆与知识图谱',
    defaultArgs: '{\n}'
  },

  // 网页检索
  brave_web_search: {
    label: 'brave_web_search (Brave 隐私实时网页检索)',
    group: '网络检索与抓取',
    defaultArgs: '{\n  "query": "Model Context Protocol enterprise architecture"\n}'
  },
  web_fetch_markdown: {
    label: 'web_fetch_markdown (网页解析提炼结构化Markdown)',
    group: '网络检索与抓取',
    defaultArgs: '{\n  "url": "https://example.com"\n}'
  }
};

export const getToolInfo = (toolName) => {
  if (TOOL_TEMPLATES[toolName]) {
    return TOOL_TEMPLATES[toolName];
  }
  return {
    label: `${toolName} (MCP 工具)`,
    group: '扩展与自定义服务工具',
    defaultArgs: '{\n}'
  };
};

export const getGroupLabel = (group, isZh) => {
  if (group === '网关原生安全审计') return isZh ? '网关原生安全审计' : 'Gateway Security & Compliance';
  if (group === '网关原生运维') return isZh ? '网关原生运维' : 'Gateway Native Ops';
  if (group === '网关原生增强') return isZh ? '网关原生增强' : 'Gateway Native Augmentation';
  if (group === 'Puppeteer 浏览器渲染') return isZh ? 'Puppeteer 浏览器渲染' : 'Puppeteer Browser Automation';
  if (group === '数据库与持久存储') return isZh ? '数据库与持久存储' : 'Databases & Storage';
  if (group === '认知与深度推理') return isZh ? '认知与深度推理' : 'Cognitive & Deep Reasoning';
  if (group === '扩展与自定义服务工具') return isZh ? '扩展与自定义服务工具' : 'Extended & Custom Tools';
  return group;
};

export const getDefaultArgsForTool = (toolName) => {
  return getToolInfo(toolName).defaultArgs;
};
