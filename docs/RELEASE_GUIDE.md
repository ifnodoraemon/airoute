# 版本与发布规范

本规范适用于后续代码开发、二进制、容器镜像和前端包。

## 当前基线

历史标签 `v1.0.0` 不得移动或重新发布。当前产品基线为 **2.0.0**：移除公开的旧版 `/v1/completions` 接口属于兼容性破坏，因此升级主版本。正式标签由成功的主分支 CI 自动创建；未发版提交的构建标识为 `2.0.0-dev+g<提交号>`。

## 递进原则

使用 `MAJOR.MINOR.PATCH`，Git 标签加 `v`，程序和包版本不加 `v`。

| 级别 | 何时递增 | 示例 | 递增结果 |
| --- | --- | --- | --- |
| PATCH 修订 | 保持兼容的缺陷修复、性能优化、依赖修复或样式调整 | 修复删除路由 404、筛选漏记录、token 统计错误 | `1.0.0 → 1.0.1` |
| MINOR 次版本 | 向后兼容的新能力、可选配置、接口或工作流 | 增加真实容灾拓扑、独立路由策略 | `1.0.3 → 1.1.0` |
| MAJOR 主版本 | 用户必须修改调用方式、配置或数据，才能继续使用的兼容性破坏 | 移除公开接口、改变必填字段或进行无法兼容升级的数据改造 | `1.4.2 → 2.0.0` |

- 一次发布包含多项变更时，按其中最高级别定版本；MINOR 递增时 PATCH 归零，MAJOR 递增时 MINOR 和 PATCH 归零。
- 不按提交次数、修改文件数量、开发天数或重构规模递增正式版本。内部重构若无用户可见变化且保持兼容，不单独触发 MINOR；文档和测试变更不必单独发版。
- 数据库新增表或字段且支持现有数据升级，不自动视为 MAJOR。数据库迁移序号独立递增；迁移序号与产品版本号不得混用。
- `/v1` 等协议路径与产品版本独立。产品升到 `2.0.0` 不代表需要改 API 地址。
- 已发布的版本、标签和同名发布资产不可覆盖、回退或复用。发布后发现问题使用更高版本修复；回滚部署只切换已存在的旧构建。

## 开发版、候选版与正式版

| 构建 | 格式 | 用途 |
| --- | --- | --- |
| 开发版 | `2.0.0-dev+gabc1234` | 未正式发版的提交；本地有未提交修改时追加 `.dirty` |
| 候选版 | `2.0.0-rc.1`、`2.0.0-rc.2` | 计划发布内容的验收；候选编号从 1 递增 |
| 正式版 | `2.0.0` | 已验证、提交并用不可变标签标记的版本 |

`+g<提交号>` 是构建元数据，不构成另一个正式版本。候选版发现缺陷后增加候选编号；正式发布后修复用 PATCH，不能继续复用旧候选版或正式标签。

## 版本来源与自动发布

根目录 `VERSION` 是唯一版本来源。升级版本只执行一次：

```bash
node scripts/version.mjs 2.0.1
```

脚本同步 Go `BaseVersion`、前端 package/lock 文件、Dockerfile 和 Compose 的开发版默认值、Helm chart/appVersion；拒绝非法版本和倒退版本。`node scripts/version.mjs --check` 只检查、不写文件，并作为 CI 门禁。Helm 默认镜像标签取 chart 的 appVersion，可通过 `image.tag` 覆盖。

版本级别仍按上文的兼容性原则选择，不按提交次数自动增加版本号。日常修复在已发布版本之上继续开发时保留开发版标识；准备新版本时运行同步命令、提交并推送。

`Publish Version Tag` 工作流监听主分支 CI 的成功结果：

1. 再次核对当前主分支提交与成功 CI 的 SHA；过期结果、PR 或其他仓库的结果不能发版。
2. 读取 `VERSION`，创建指向该提交的附注标签 `v<版本>`；已有历史标签不会移动。版本没有变化且标签已存在时自动跳过。
3. 显式触发同一标签上的二进制和镜像发布工作流。GitHub 的 `GITHUB_TOKEN` 创建标签不会自动触发标签 push 工作流，因此这里使用 `workflow_dispatch`，不需要额外配置 PAT。见 [GitHub 官方触发规则](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow)。
4. 发布 Linux/macOS/Windows 二进制、GitHub Release 及 GHCR 多架构镜像。发布资产不覆盖已有同名文件。

镜像渠道约定：

- `main` 与 `sha-<提交号>`：开发构建。
- `2.0.0`、`2.0`：正式版本及次版本渠道。
- `latest`：正式发布版本，主分支开发构建不再覆盖它。
- 手工候选标签仍支持 `v2.0.0-rc.1`，其 Release 标记为 prerelease，不更新 `latest`。

发布工作流仍执行质量门禁。需要补跑当前提交的发布时，可在 `Publish Version Tag` 手工运行中设置 `retry=true`；已经前进到其他主分支提交时，按不可变标签直接补跑对应发布器：

```bash
gh workflow run release.yml --ref v2.0.0
gh workflow run docker.yml --ref v2.0.0
```

补跑用于完成缺失产物，不修改已有标签或同名资产。正式发布前仍应按功能实际启用情况完成外部服务验收。

本地构建容器时保留版本信息：

```bash
docker compose build --build-arg VERSION="$(sh scripts/build-version.sh)" gateway-1 gateway-2
```

升级到独立路由存储只新增迁移 16 的 `model_routes` 表。旧版程序会忽略新路由策略，回滚前应评估已删除或修改的路由是否会恢复旧行为；不要为回滚应用而删除数据库或迁移记录。

## 依赖容器漂移检查

`docker compose` 的 `ulimits` 与 `command` 只在容器**创建**时生效。只重建网关镜像不会重建 PostgreSQL / Redis / nginx，旧容器可能仍停留在 `nofile=1024`、PostgreSQL `max_connections=100`，而 compose 文件已经声明 65536 与 200。表现为升级后偶发 "too many open files" 或 "remaining connection slots are reserved"。

发布前后各执行一次只读检查：

```bash
make check-deploy-drift
# 或：sh scripts/check-deploy-drift.sh
```

脚本只报告漂移并给出 `docker compose up -d --force-recreate <容器>` 的修复命令，不修改任何状态；容器未运行时自动跳过。

## 2.0 升级事项

`POST /v1/completions` 已移除，客户端需迁移到 `/v1/chat/completions` 或 `/v1/responses`。迁移 47 清除渠道的旧协议声明，只声明退役协议的渠道会被停用；迁移 46 增加上游并发参数，迁移 48 增加带过期时间的原生资源、签名和任务计量状态。双副本必须连接同一个 PostgreSQL 数据库。回滚应用不需要删除新增列、表或迁移记录。
