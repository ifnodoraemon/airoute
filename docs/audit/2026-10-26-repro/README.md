# 2026-10-26 功能细节审查复现材料

这些探针断言的是期望行为；审查时在未修复代码上得到 **17 个预期失败**，用于证明报告中的缺陷。它们不是既有回归套件的失败，也不表示已经修复。

源文件以 `.txt` 保存，避免自动进入正常 CI。仅使用内存数据库、模拟 HTTP Transport、jsdom 和模拟请求，不连接真实 OAuth、支付或模型服务。

| 文件 | 内容 |
|---|---|
| `backend_review_test.go.txt` | 6 项：OAuth 身份碰撞、改密旧会话、币种保留、定价读取失败、渠道体检协议、USD 钱包扣款 |
| `api_review_test.go.txt` | 2 项：多模态 URL 拼接、视频任务创建/查询渠道一致性 |
| `frontend_review.test.jsx.txt` | 9 项：映射、探测竞态、导出、SSE 错误、仪表盘、公告、未保存退出、视频模型参数和停止轮询 |
| `backend-results.log` / `api-results.log` / `frontend-results.log` | 原始运行结果；失败消息包含实际值与期望值 |
| `source-hashes.txt` | 定向复现所对应的业务源码 SHA-256；可在仓库根目录运行 `sha256sum -c docs/audit/2026-10-26-repro/source-hashes.txt` 核对 |

Go 探针使用 `go test -overlay` 注入，不修改业务包。例如在仓库根目录执行：

```bash
python3 - <<'PY'
from pathlib import Path
import json

root = Path.cwd()
archive = root / 'docs/audit/2026-10-26-repro'
target = Path('/tmp/airoute-audit-replay')
target.mkdir(exist_ok=True)
replacements = {}
for package, filename in [('controlplane', 'backend_review_test.go'), ('api', 'api_review_test.go')]:
    source = target / filename
    source.write_bytes((archive / (filename + '.txt')).read_bytes())
    replacements[str(root / 'internal' / package / 'audit_review_test.go')] = str(source)
(target / 'overlay.json').write_text(json.dumps({'Replace': replacements}))
PY

/usr/local/go/bin/go test -overlay /tmp/airoute-audit-replay/overlay.json \
  ./internal/controlplane ./internal/api -run '^TestAuditReview' -count=1 -v
```

前端探针可用下列命令临时加载。脚本使用随机文件名，并在结束时清理自己创建的文件，不影响现有测试：

```bash
python3 - <<'PY'
from pathlib import Path
import subprocess
import tempfile

root = Path.cwd()
archive = root / 'docs/audit/2026-10-26-repro/frontend_review.test.jsx.txt'
with tempfile.NamedTemporaryFile(prefix='audit-replay-', suffix='.test.jsx', dir=root / 'web/src', delete=False) as f:
    probe = Path(f.name)
    f.write(archive.read_bytes())
try:
    result = subprocess.run(['npm', 'test', '--', str(probe.relative_to(root / 'web'))], cwd=root / 'web')
finally:
    probe.unlink()
raise SystemExit(result.returncode)
PY
```

业务代码修复后，这些期望行为应通过。正式纳入回归套件前，应将探针拆入对应模块的测试文件；OAuth 测试必须继续使用模拟身份服务，禁止对真实用户账号执行。
