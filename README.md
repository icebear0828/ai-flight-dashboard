# ✈️ AI Flight Dashboard

> 极简、零依赖的 AI 资产终端飞行仪表盘

[English](README.en.md)

AI Flight Dashboard 是一个基于 Go 语言构建的 **命令行 UI (TUI) + Web 双模式工具**。利用 **"被动雷达监听"** 机制，它能在完全不侵入各类 AI CLI 工具（如 Claude Code, Gemini CLI）源代码的前提下，实时捕获 Token 消耗并以丝滑的动画反馈当前会话成本。

## ✨ 核心特性

- 🎯 **被动雷达监听**: 采用 `fsnotify` 监听文件增量流，只要底层工具将日志写入磁盘（例如 `~/.claude/projects/` 或 `~/.gemini/tmp/`），系统瞬间就能捕获。
- ⚡ **极致性能**: Go 语言 + [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建，单文件可执行分发，极速启动。
- 💰 **实时成本折算**: 内置计费引擎，将 Token 数字根据不同模型实时折算为美元 (USD) 成本。
- 📊 **代码工程归因 (Project Tracking)**: 自动解析 Claude 工作区哈希前缀，精确统计各个代码项目的独立 Token 花费。
- 💾 **SQLite 数据脱水**: 所有捕获的消耗流会自动 upsert 进入 `stats/usage.db`，沉淀长期分析数据。
- 🌐 **Web 看板模式**: 通过 `--web` 启动 HTTP 服务，在浏览器中查看 React 驱动的可视化仪表盘。
- 📡 **局域网 P2P 互联**: 通过 `--lan` 启用纯去中心化 UDP 组播，免配置实现局域网内多台电脑 Token 实时共享与探测。
- 📦 **Fat Server 跨平台分发**: 提供内嵌式多端二进制分发。新设备只需执行 `curl -sL http://主节点IP:19100/install.sh | bash` 即可自适应拉取正确版本的本体并加入雷达网络。
- 🛰️ **远程集群监控**: 通过 `--forward-to` 将多个服务器探针的日志汇聚到统一主控面板。

## 🚀 快速体验

完整的配置指南与集群部署，请参考 [📚 使用手册 (docs/usage.md)](docs/usage.md)。

### macOS 安装 (推荐)

```bash
curl -sL https://github.com/icebear0828/ai-flight-dashboard/releases/latest/download/install-mac.sh | bash
```

> 自动检测 Apple Silicon / Intel，下载、安装到 `/Applications`，并解除 Gatekeeper 拦截。

### 一键部署 (服务器/后台)

对于服务器或后台运行环境，我们提供了一键部署脚本，自动完成编译并注册 Systemd 开机自启服务：

```bash
chmod +x ./scripts/deploy.sh
sudo ./scripts/deploy.sh
```

> 💡 **提示**：脚本支持交互式选择部署为主控端 (Server) 或 探针端 (Forwarder)，只需跟随提示输入 Token 等信息即可。

### 手动构建与运行

```bash
# 编译
go build -o dashboard ./cmd/dashboard

# TUI 模式 — 放在终端侧栏或 Tmux 分屏
./dashboard

# Web 模式 — 浏览器访问 http://localhost:19100
./dashboard --web

# 自定义端口 + 设备标识
./dashboard --web --port 8080 --device-id my-mac
```

### 模拟触发雷达

在仪表盘运行的同时，开启另一个终端写入一行模拟日志：

```bash
echo '{"type":"assistant", "model": "claude-sonnet-4-6", "usage": {"input_tokens": 1000, "output_tokens": 500, "cache_read_input_tokens": 200}}' >> session.jsonl
```

> HUD / Web 仪表盘会立刻捕捉增量跳动，并将交互存入数据库。

## ⚙️ 费率配置

模型单价通过 `cmd/dashboard/pricing_table.json` 嵌入二进制文件，无需依赖外部文件即可运行。如需修改价格，编辑该文件后重新编译：

```json
{
  "models": {
    "gemini-2.5-pro": {
      "input_price_per_m": 1.25,
      "cached_price_per_m": 0.31,
      "output_price_per_m": 5.00
    },
    "claude-sonnet-4-6": {
      "input_price_per_m": 3.00,
      "cached_price_per_m": 0.30,
      "output_price_per_m": 15.00
    }
  }
}
```

## 🏗 架构设计

```
cmd/dashboard/        CLI 入口 + 装配层 + 嵌入的 pricing_table.json
internal/
├── model/            共享数据结构 (TokenUsage)
├── watcher/          fsnotify 实时监听 + JSONL 解析
├── scanner/          历史日志全量/增量扫描
├── calculator/       Token → USD 计费引擎
├── db/               SQLite 持久层 (WAL 模式)
└── web/              HTTP API + 嵌入式 React SPA (go:embed)
```


| 模块 | 职责 | 测试 |
|---|---|:---:|
| `model` | 定义 `TokenUsage` 共享结构体 | — |
| `watcher` | fsnotify 监听 + Claude/Gemini 日志解析 | ✅ |
| `scanner` | 历史日志批量扫描，支持增量 + 文件截断检测 | ✅ |
| `calculator` | 按模型计费，支持文件加载和字节流初始化 | ✅ |
| `db` | SQLite 持久化，按时间窗口/设备聚合查询 | ✅ |
| `web` | REST API (`/api/stats`) + 静态文件服务 | ✅ |

## 📡 API

```
GET /api/stats              # 全量统计（包含 Project、Device、Period 分组）
GET /api/stats?device=my-mac  # 按设备过滤
POST /api/device-alias      # 命名设备别名 (body: {"device_id": "...", "display_name": "..."})
GET /download/dashboard     # 动态拉取适配当前平台的 Fat Server 本体
```

返回 `{ periods, sources, devices, projects }` — 详见 [dashboard-api.md](dashboard-api.md)。

## 🗺 路线图

- [x] **Phase 1: 极客 HUD 层** — 终端常驻 Bubble Tea 面板，实时闪烁更新
- [x] **Phase 2: 结构化持久层** — 实时日志拦截 + SQLite 入库 + 增量扫描
- [x] **Phase 2.5: Web 看板** — React SPA + HTTP API + 嵌入式分发
- [ ] **Phase 3: 全键盘终端看板** — `Tab` 切换，终端内渲染 ASCII 图表与排行榜

## 📜 License

MIT
