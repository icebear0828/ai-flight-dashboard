# 🗺️ 桌面端演进路线图 (Desktop App Roadmap)

> **核心愿景**：告别极客式的终端命令行，将 Token Ray 升级为**"双击安装、开机自启、托盘常驻"**的现代化跨平台桌面客户端，真正实现面向普通用户的"开箱即用"。

---

## 下一阶段：开箱即用差异化补漏

> **产品定位**：Token Ray 是一个打开即用的本地 AI 用量黑匣子。用户不需要配置 hook、路径、端口或命令；打开 App 后自动发现 Claude/Codex/Gemini，自动回填历史，后台持续追踪，并把数据可信度讲清楚。

### P0：首次启动自动发现与回填
- [ ] **First Run Experience**：首次打开 App 自动检测 Claude Code、Codex、Gemini CLI 的默认数据目录，并在第一屏展示每个来源的状态：`detected` / `importing` / `watching` / `no_data` / `needs_permission` / `unsupported`。
- [ ] **自动历史回填**：首次启动自动执行安全的历史导入语义，复用现有 scanner / Codex session totals / dedup / superseded 机制，不要求用户手动运行 `repair-history`。
- [ ] **导入进度与结果摘要**：展示“已导入 X 条记录、覆盖最近 X 天、最近 7/30 天成本、最高成本项目/模型”，让用户 60 秒内看到明确价值。
- [ ] **强空状态诊断**：无数据时不显示空 dashboard；明确说明原因，例如未安装、目录不存在、有目录但无 session、权限不足、schema 不兼容、日志格式暂不支持。
- [ ] **数据健康状态**：为每个来源展示可信度与覆盖情况，例如 Claude 完整、Codex sessions 完整或 telemetry fallback、Gemini 已去重、Antigravity 暂不支持并说明原因。
- [ ] **后台采集默认可见**：App 启动后自动开始持续监听/轮询，关窗口不停止采集；菜单栏/托盘可看到当前采集状态。

### P1：零配置日常使用体验
- [x] **Source Coverage Cards**：Dashboard 顶部增加来源卡片，按 Claude / Codex / Gemini 展示 detected、imported、watching、last seen、records、cost、health。
- [ ] **本机优先，多设备延后引导**：首次体验只强调本机账本；检测到 LAN peer 或用户进入设置时，再提示加入局域网同步，避免第一屏被多设备概念打断。
- [ ] **开机自启与菜单栏入口产品化**：设置页提供开机自启、关窗继续运行、菜单栏显示/隐藏等开关，并能解释当前运行状态。
- [ ] **本地报表快捷入口**：在 GUI 中提供 Today / 7d / 30d / Projects / Models / Sessions 快捷视图，优先服务开箱查看，不把 CLI 报表作为主入口。
- [ ] **可恢复的修复动作**：对权限、路径、旧数据目录、导入失败提供一键修复或一键重试；所有自动写入都保留可解释日志和失败原因。
- [ ] **可选增强区**：StatusLine、CLI `daily/weekly/monthly/sessions`、高级 LAN/Forwarder 放入可选增强，不进入首次启动关键路径。

## 阶段一：Wails/Tauri 架构融合 (GUI 化)
- [x] **技术栈融合**：引入 [Wails](https://wails.io/) 框架，将现有的 Go 核心侦听服务与 React 前端无缝缝合为原生桌面应用。
- [x] **免终端双击启动**：打包输出原生格式包（macOS `.dmg`，Windows `.exe` 安装程序，Linux `.AppImage`）。 *(Phase 4 完善)*
- [x] **标准桌面窗口**：
  - 应用启动后会**直接弹出主界面**，方便查看实时消耗，不再默认隐藏。
  - 保留"开机自启动"能力（可通过操作系统设置），后台探针无感运行。

## 阶段二：配置与操作的完全可视化 (Zero CLI)
- [x] **GUI 参数配置台**：
  - 在设置页可视化修改各个模型的折算价格（取代目前需要改 `pricing_table.json` 然后重新编译的做法）。
  - 可视化修改设备名称 (Device Alias)。
- [x] **路径可视化管理**：允许用户在界面上点击按钮，选择需要额外监听的日志文件夹（而不仅限于默认的 Claude/Gemini 路径）。

## 阶段三：局域网傻瓜式配对 (LAN Auto-Discovery GUI)
- [x] **图形化雷达配对**：彻底废弃通过 `curl` 脚本加入局域网的做法。
  - 新设备安装 APP 并打开后，通过 UDP 广播自动扫描局域网。
  - 界面弹出提示：*"发现局域网主节点 [MacBook-Pro]，是否一键加入该数据雷达网？"*
  - 点击【加入】，底层自动建立数据同传机制。
- [x] **集群拓扑图**：在 UI 界面上绘制出一个酷炫的"雷达地图"，实时显示当前局域网内有哪些节点在线，以及它们正在燃烧多少 Token。

## 阶段四：跨平台分发与自动更新 (Auto-Updater)
- [x] **内置热更新 (OTA)**：检查到 GitHub Release 有新版本时，在应用内一键下载并静默重启完成更新。*(骨架已就位，需接入 selfupdate 实现二进制替换)*
- [x] **移动端前瞻**：PWA `manifest.json` 已就位，局域网内手机可通过浏览器 "Add to Home Screen" 安装。

## 技术债务与重构建议 (Code Review Findings)
- [x] **动态取消监听 (Unwatch)**：`Watcher.UnwatchDir()` 已实现，Settings 移除目录时实时解除监听，无需重启。
- [x] **前端组件拆分**：`SettingsModal.tsx` 已拆分为 `PricingTab` 和 `SystemConfigTab` 子组件。
