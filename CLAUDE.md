## 说明

- 本文档是 Claude Code 的记忆文件，方便 AI 快速了解项目
- 详细文档见 README.md

## 项目类型

Auralens 平台的配套 Agent CLI 工具（Go + Cobra），供 Agent 在 Windows/macOS/Linux 上操作 Auralens 平台。

## 技术栈

- 语言：Go，框架：Cobra，颜色：fatih/color
- 二进制名：`auralens`，模块路径：`github.com/lisiting01/auralens-cli`
- 配置：`~/.auralens/config.json`（name + token + base_url + env）
- 守护状态：`~/.auralens/workers/` 、 `~/.auralens/schedulers/`
- 发布：GitHub Releases，tag `v*` 自动触发 GoReleaser（goreleaser-action@v6，`~> v2`）

## 命令结构

```
auralens auth    register / login / status / logout
auralens research  list / view / result
auralens agent   run / status / stop
auralens agent schedule  [start] / status / stop
```

## 核心架构

- `internal/config`：读写 `~/.auralens/config.json`
- `internal/api`：HTTP 客户端，认证头 `X-Agent-Name` + `X-Agent-Token`
- `internal/daemon`：engine（Claude/Codex/Generic）、worker/scheduler 状态、系统提示词
- `cmd/`：cobra 命令；detach_*/terminate_* 处理跨平台进程分离

## 关键设计点

- `agent run` 通过 re-exec 自身实现 `--daemon` 模式（不依赖外部进程管理器）
- `agent schedule` 是轻量调度器：完成后等 interval 秒再 spawn 全新实例，不堆叠
- `buildEnv()` 继承 `os.Environ()` 并叠加 config `env` 字段（最高优先级），解决 Windows 上 `CLAUDE_CODE_GIT_BASH_PATH` 丢失问题
- 系统提示词注入 Auralens Research API 完整文档，Agent 可直接调用 HTTP 接口

## 当前版本

v0.0.2（https://github.com/lisiting01/auralens-cli）
