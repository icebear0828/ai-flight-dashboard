# AI Flight Dashboard 进阶特性路线图 (ROADMAP)

为了在深度上对标并超越专门针对特定平台的风控探测工具（如 `Claude-Code-Usage-Monitor`），我们计划在 AI Flight Dashboard 中引入更深度的“**成本分析引擎**”与“**配额风控探测**”功能。

以下是具体的跟进深度设计文档：

## 1. 缓存命中率与实际花销估算 (Cache Hit & Cost Estimation)

目前大语言模型工具（尤其是 Claude Code）高度依赖 **Prompt Caching** 来降低大型项目的上下文重组成本。为了准确提供成本大盘，我们需要在现有的 Token 计数之上，增加精细化的计费因子统计。

### 核心功能点
*   **细粒度 Token 拆解**：
    将原本单一的 `Total Tokens` 拆解为四重维度：
    *   `Input Tokens` (常规输入)
    *   `Output Tokens` (模型输出)
    *   `Cache Creation Tokens` (缓存写入，通常价格与常规输入不同或一致)
    *   `Cache Read Tokens` (缓存命中读取，通常价格只有常规输入的 10%)
*   **多模型动态计价卡 (Price Card)**：
    系统内置常见模型的标准 API 定价表（如 Claude 3.5 Sonnet, GPT-4o, Gemini 1.5 Pro）。
    支持根据解析到的各维度 Token，实时计算真实的法币花销（USD）。
*   **UI/Web 仪表盘展现**：
    在 Web 看板中新增「成本节省分析面板」，直观展示：**“如果没有命中缓存，你需要花多少钱 vs 命中缓存后你实际花了多少钱，共为你节省了多少。”**

---

## 2. 区分官方订阅制与第三方按量计费 (Subscription vs Pay-As-You-Go API)

用户在使用终端 AI 工具时，通常有两类不同的额度账单模式。这两类模式的风控和限流逻辑完全不同，我们需要能够智能区分或允许用户手动声明。

### 模式 A：官方订阅制 (Official Subscription - 如 Claude Pro, Max5, Max20)
*   **特点**：按月交固定的订阅费，API 调用不产生额外金钱开销，但**受到极其严格的时间滚动窗口限制**。
*   **监控重点（防限流探测器）**：
    *   **5小时滚动窗口追踪**：引入 P90 算法或动态高水位线探测，评估用户在 5 小时内的峰值使用量。
    *   **限流预警 (Burn Rate Alert)**：根据过去一小时的使用速率，预测当前可用配额何时耗尽。
    *   **花销计算**：这种模式下，不显示具体的 API 成本（因为是免费额度），而是显示 **“额度消耗占比进度条”**。

### 模式 B：第三方/官方直连 API (Third-Party / Direct API)
*   **特点**：如使用 Anthropic 官方 API Key、OpenRouter 等中转 API，无严苛的 5 小时滚动窗口限制，但**每一笔调用都产生直接的账单费用**。
*   **监控重点（财务防超支控制）**：
    *   **精确计费计算**：深度结合前面提到的「缓存命中率」进行成本测算。
    *   **每日/预算警告**：允许用户设置“单日最大花费阈值”，超过指定金额时在终端显示强烈的红色告警，防止脚本跑飞导致天价账单。

### 架构实现思路
1. **自动嗅探**：通过抓包探针分析工具发出的网络请求头或目标域名（例如直接请求 `api.anthropic.com` 大概率是 API 模式，而走网页逆向或其他特定端口可能是订阅模型）。
2. **显式配置 (CLI Arguments)**：
   允许在启动 Dashboard 时声明监听目标所属的计费类型：
   ```bash
   # 监听 API 消费模式，并应用按量计价卡
   ./dashboard --billing-mode api --model claude-3-5-sonnet-20241022

   # 监听官方订阅模式，启动 5 小时防封推测探测
   ./dashboard --billing-mode subscription --plan pro
   ```

---

## 3. 下一步开发计划 (Next Steps)

1. ~~**[Schema Update]** 更新 SQLite 数据库表，增加 `cached_tokens` 和 `cost_usd` 字段。~~ ✅ 已有
2. ~~**[Pricing Engine]** 编写计费计算模块 (`internal/calculator`)。~~ ✅ 已有
3. ~~**[Cache Savings API]** 新增 `/api/cache-savings` 端点 + `CalculateCostNoCaching` 方法。~~ ✅ 已完成
4. ~~**[Session Tracker]** 针对订阅模式，开发环形队列缓存以追踪近 5 小时 Token 消耗轨迹。~~ ✅ 已完成
5. ~~**[Billing Mode CLI]** 新增 `--billing-mode` / `--plan` / `--budget-daily` CLI 参数。~~ ✅ 已完成
6. ~~**[Budget Alert Engine]** `internal/alert` 包：预算阈值告警引擎 + TUI 集成。~~ ✅ 已完成
7. **[Web UI Upgrade]** 前端面板增加缓存节省卡片与计费模式切换。
