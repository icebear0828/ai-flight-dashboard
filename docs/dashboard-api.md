# AI Flight Dashboard

## Types

```typescript
interface PeriodCost {
  label: string
  cost: number
  tokens: number
}

interface ModelStats {
  model: string
  events: number
  input_tokens: number
  cached_tokens: number
  output_tokens: number
  total_cost: number
  input_price_per_m: number
  cached_price_per_m: number
  output_price_per_m: number
}

interface SourceStats {
  name: string
  total_input: number
  total_cached: number
  total_output: number
  total_cost: number
  total_events: number
  models: ModelStats[]
}

interface StatsResponse {
  periods: PeriodCost[]
  sources: SourceStats[]
  devices: string[]
}
```

## Endpoints

### 1. Get Live Stats
```
GET /api/stats
GET /api/stats?device={device_id}
```

Response:
```json
{
  "periods": [
    { "label": "1h", "cost": 0.5, "tokens": 120000 },
    { "label": "24h", "cost": 1.2, "tokens": 280000 },
    { "label": "ALL", "cost": 50.0 }
  ],
  "sources": [
    {
      "name": "Claude Code",
      "total_input": 15000,
      "total_cached": 5000,
      "total_output": 2000,
      "total_cost": 0.45,
      "total_events": 10,
      "models": [
        {
          "model": "claude-3-7-sonnet-20250219",
          "events": 10,
          "input_tokens": 15000,
          "cached_tokens": 5000,
          "output_tokens": 2000,
          "total_cost": 0.45
        }
      ]
    }
  ]
}
```

## UI Data Flow

```
┌───────────────────────────────────────────────┐
│              DashboardScreen                  │
│                                               │
│  [ Header with Live Indicator ]               │
│                                               │
│  [ Period Cost Cards Row (1h, 24h, ALL...) ]  │
│                                               │
│  [ Source Card ]        [ Source Card ]       │
│  - Total tokens         - Total tokens        │
│  - Donut Chart          - Donut Chart         │
│  - Models Table         - Models Table        │
└───────────────────────────────────────────────┘
```

### 页面布局

- DashboardScreen：全屏监控面板。顶部标题包含 ✈️ AI Flight Dashboard 标题和闪烁的 "LIVE" 徽章。下方先渲染一排 PeriodCost 小卡片。然后渲染 SourceStats 卡片网格。每个 Source 卡片包含该来源的总开销数字、Input/Cached/Output Token 数据的四大块统计（类似原来四个小格）、圆环图（Donut Chart）、以及该来源下的 Models 数据表格。整体遵循深色指挥中心风格，极简但信息密度高。
