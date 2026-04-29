# ✈️ AI Flight Dashboard

> 极简、零依赖的 AI 资产终端飞行仪表盘

AI Flight Dashboard 是一个基于 Go 语言构建的**命令行 UI (TUI) 工具**。利用**“被动雷达监听”**机制，它能在完全不侵入各类 AI CLI 工具（如 Claude Code, Gemini CLI, Aider, Cursor）源代码的前提下，实时捕获 Token 消耗并在你的终端底部以丝滑的动画刷新当前会话成本。

## ✨ 核心特性

- 🎯 **被动雷达监听**: 采用 `fsnotify` 监听文件增量流，只要底层工具将日志写入磁盘（例如 `~/.claude/projects/` 或 `~/.gemini/tmp/`），系统瞬间就能捕获。
- ⚡ **极致性能**: 采用 Go 语言 + [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建，单文件可执行分发，不依赖 Python 或 Node 环境，极速启动。
- 💰 **实时成本折算**: 内置计费引擎，将枯燥的 Token 数字根据不同模型实时折算为美元 (USD) 成本。
- 💾 **SQLite 数据脱水**: 所有捕获的消耗流会自动 upsert 进入 `stats/usage.db`，为你沉淀长期的代码资产分析数据。

## 🚀 快速体验 (E2E)

### 1. 构建与运行
```bash
# 进入目录
cd /Users/c/wiki/token/ai-flight-dashboard

# 编译为二进制文件
go build -o dashboard ./cmd/dashboard

# 启动常驻仪表盘
./dashboard
```
> **提示**：建议将它作为一个常驻的分屏（如 Tmux）挂在终端底部或侧边栏。

### 2. 模拟触发雷达
在仪表盘运行的同时，开启另一个终端并执行以下命令，向当前目录写入一行模拟日志：
```bash
echo '{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 1000, "output_tokens": 500, "cache_read_input_tokens": 0}}' >> session.jsonl
```
> **效果**：你的 HUD 仪表盘会立刻捕捉增量跳动，并将交互存入数据库！

## ⚙️ 费率配置

所有的模型单价配置保存在运行时生成的 `stats/pricing_table.json` 中，你可以根据厂商（Anthropic, Google 等）的价格变动自行调整：

```json
{
  "models": {
    "gemini-2.5-pro": {
      "input_price_per_m": 1.25,
      "cached_price_per_m": 0.31,
      "output_price_per_m": 5.00
    },
    "claude-3-7-sonnet-20250219": {
      "input_price_per_m": 3.00,
      "cached_price_per_m": 0.30,
      "output_price_per_m": 15.00
    }
  }
}
```

## 🏗 架构设计
本项目严格遵循 TDD (测试驱动开发) 规范构建，各个模块独立解耦：
- **`cmd/dashboard`**: CLI 入口主程序与打通全链路的装配层。
- **`internal/tui`**: 负责终端动画的 Bubble Tea 模型渲染。
- **`internal/watcher`**: fsnotify 核心监听模块，处理被动雷达的日志解析。
- **`internal/calculator`**: Token 与美元（USD）计费转化引擎。
- **`internal/db`**: SQLite 持久层接口与 Schema 自动初始化。

## 🗺 路线图 (Roadmap)
- [x] **Phase 1: 极客 HUD 层** (终端多线程安全刷新，展现常驻底部的视觉效果)
- [x] **Phase 2: 结构化持久层** (实时日志拦截与 SQLite 入库)
- [ ] **Phase 3: 全键盘终端看板** (后续规划：通过 `Tab` 键一键切换，直接在终端内渲染图表热力图与项目耗资排行榜)
