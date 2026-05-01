# Claude Code Hooks 教程：自动日记 + 技能记忆上云

> 面向零基础用户的完整配置指南。读完后你将拥有：
> 1. 每次关闭 Claude Code 对话时，**自动用 LLM 写一篇中文工作日志**
> 2. 每次使用工具时，**自动记录行为模式到本地记忆库**
> 3. 技能包和记忆数据**自动同步到你的私有 GitHub 仓库**

---

## 前置知识：什么是 Hook？

Claude Code 提供了一套"钩子（Hook）"机制，让你可以在特定事件发生时，**自动执行自定义脚本**。

可用的钩子事件：

| 事件名 | 触发时机 |
|--------|---------|
| `PreToolUse` | Claude 调用工具**之前**（如执行命令、编辑文件） |
| `PostToolUse` | Claude 调用工具**之后** |
| `Notification` | Claude 需要你注意时（如等待审批） |
| `SessionEnd` | 你**关闭一次对话**时 |

钩子配置在 `~/.claude/settings.json` 文件中。

---

## 第一部分：关闭对话自动写日记

### 原理

```
你关闭 Claude Code 对话
       │
       ▼
  SessionEnd 钩子触发
       │
       ▼
  diary-log.sh 脚本运行
       │
       ├─ 1. 读取本次对话的完整 transcript (JSONL)
       ├─ 2. 提取项目名、Git 最近提交
       ├─ 3. 将 transcript 发送给一个 LLM API
       │     (使用"信息压缩引擎"系统提示词)
       ├─ 4. LLM 返回 3-5 条高密度中文摘要
       └─ 5. 追加写入 ~/.claude/diary/YYYY-MM-DD.md
```

### 配置步骤

#### 第 1 步：创建日记脚本

```bash
mkdir -p ~/.claude/hooks
```

创建文件 `~/.claude/hooks/diary-log.sh`，内容如下（精简版）：

```bash
#!/bin/bash
# SessionEnd hook: 用 LLM 自动写日记
set -euo pipefail

DIARY_DIR="$HOME/.claude/diary"
mkdir -p "$DIARY_DIR"

# 读取 Claude Code 传入的 JSON（包含 session_id, cwd, transcript_path）
INPUT=$(cat)
CWD=$(echo "$INPUT" | jq -r '.cwd // empty')
TRANSCRIPT_PATH=$(echo "$INPUT" | jq -r '.transcript_path // empty')

PROJECT=$(basename "${CWD:-unknown}")
TIMESTAMP=$(date '+%H:%M')
DATE=$(date '+%Y-%m-%d')
DIARY_FILE="$DIARY_DIR/$DATE.md"

# ========================================
# LLM 配置 — 改成你自己的 API 地址和 Key
# ========================================
LLM_URL="${DIARY_LLM_URL:-https://api.openai.com/v1/chat/completions}"
LLM_KEY="${DIARY_LLM_KEY:-sk-你的key}"
LLM_MODEL="${DIARY_LLM_MODEL:-gpt-4o-mini}"

# 系统提示词：信息压缩引擎
SYSTEM_PROMPT="你是一个高密度日志压缩器。将开发者-AI对话提炼为3-5条中文名词化要点。只记录已完成的工程成果。不使用第一人称。不输出任何开头结尾寒暄。"

# 从 transcript 提取对话文本
SUMMARY=""
if [ -n "$TRANSCRIPT_PATH" ] && [ -f "$TRANSCRIPT_PATH" ]; then
    # 提取对话（最后 500K 字符）
    CONTEXT=$(jq -r '
      select(.type == "user" or .type == "assistant")
      | (if .type == "user" then "User: " else "Assistant: " end)
        + (if (.message.content | type) == "string" then .message.content
           elif (.message.content | type) == "array" then
             [.message.content[] | select(.type == "text") | .text] | join(" ")
           else "" end)
    ' "$TRANSCRIPT_PATH" 2>/dev/null | tail -c 500000)

    if [ -n "$CONTEXT" ]; then
        ESCAPED_SYSTEM=$(echo "$SYSTEM_PROMPT" | jq -Rs .)
        USER_MSG="[项目]: $PROJECT
[对话内容]:
$CONTEXT"
        ESCAPED_USER=$(echo "$USER_MSG" | jq -Rs .)

        RESPONSE=$(curl -s --max-time 60 \
            -H "Authorization: Bearer $LLM_KEY" \
            -H "content-type: application/json" \
            "$LLM_URL" \
            -d "{
                \"model\": \"$LLM_MODEL\",
                \"max_tokens\": 500,
                \"temperature\": 0.3,
                \"messages\": [
                    {\"role\": \"system\", \"content\": $ESCAPED_SYSTEM},
                    {\"role\": \"user\", \"content\": $ESCAPED_USER}
                ]
            }" 2>/dev/null)

        SUMMARY=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // empty' 2>/dev/null)
    fi
fi

# 写入日记
{
    echo ""
    echo "### $TIMESTAMP — $PROJECT"
    echo ""
    echo "${SUMMARY:-(无法提取会话摘要)}"
    echo ""
} >> "$DIARY_FILE"

exit 0
```

给脚本添加可执行权限：

```bash
chmod +x ~/.claude/hooks/diary-log.sh
```

#### 第 2 步：注册到 settings.json

编辑 `~/.claude/settings.json`，在顶层 `hooks` 对象中加入：

```json
{
  "hooks": {
    "SessionEnd": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/diary-log.sh",
            "timeout": 30,
            "async": true
          }
        ]
      }
    ]
  }
}
```

> **关键参数说明**：
> - `"matcher": ""` → 匹配所有 session（空字符串 = 通配）
> - `"timeout": 30` → 最多等 30 秒（LLM API 可能较慢）
> - `"async": true` → 异步执行，不阻塞关闭流程

#### 第 3 步：配置你的 LLM API

日记脚本支持通过环境变量自定义 LLM 后端。在 `~/.zshrc` 中添加：

```bash
# 使用 OpenAI
export DIARY_LLM_URL="https://api.openai.com/v1/chat/completions"
export DIARY_LLM_KEY="sk-你的key"
export DIARY_LLM_MODEL="gpt-4o-mini"

# 或者使用本地 Ollama
# export DIARY_LLM_URL="http://localhost:11434/v1/chat/completions"
# export DIARY_LLM_KEY="ollama"
# export DIARY_LLM_MODEL="qwen2.5:7b"
```

#### 第 4 步：验证

关闭一次 Claude Code 对话后，检查日记文件：

```bash
cat ~/.claude/diary/$(date +%Y-%m-%d).md
```

你应该能看到类似这样的输出：

```markdown
### 17:30 — my-project

- AI Flight Dashboard 缓存节省分析 API 端点 (`/api/cache-savings`) 新增与测试覆盖
- 计费模式 CLI 参数扩展：`--billing-mode` / `--plan` / `--budget-daily` 三级配置体系落地
- 预算告警引擎 (`internal/alert`) 阈值分级逻辑实现与 TUI 集成渲染
```

---

## 第二部分：自动记录行为模式（Continuous Learning）

### 原理

```
你在 Claude Code 中使用任何工具
       │
       ▼
  PreToolUse / PostToolUse 钩子触发
       │
       ▼
  observe.sh 脚本运行
       │
       ├─ 1. 解析工具名、输入、输出
       ├─ 2. 检测当前项目（通过 git root）
       ├─ 3. 脱敏（自动删除 API key、密码等）
       └─ 4. 追加写入 ~/.claude/homunculus/projects/<id>/observations.jsonl
```

这些观察数据可以被后续的"观察者 Agent"分析，自动提炼出你的编码习惯（如"总是先写测试"），生成"直觉（instinct）"规则。

### 配置步骤

#### 第 1 步：获取技能包

将 `continuous-learning-v2` 放到你的技能目录中：

```bash
# 方法 A：如果你有现成的 skills 仓库
git clone https://github.com/你的用户名/claude-skills-private.git ~/claude-skills-private

# 方法 B：手动创建目录
mkdir -p ~/claude-skills-private/continuous-learning-v2/hooks
mkdir -p ~/claude-skills-private/continuous-learning-v2/scripts
mkdir -p ~/claude-skills-private/continuous-learning-v2/agents
```

#### 第 2 步：创建符号链接

```bash
mkdir -p ~/.claude/skills
ln -s ~/claude-skills-private/continuous-learning-v2/ ~/.claude/skills/continuous-learning-v2
```

#### 第 3 步：注册观察者钩子

在 `~/.claude/settings.json` 的 `hooks` 中添加：

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/skills/continuous-learning-v2/hooks/observe.sh pre",
            "timeout": 10,
            "async": true
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/skills/continuous-learning-v2/hooks/observe.sh post",
            "timeout": 10,
            "async": true
          }
        ]
      }
    ]
  }
}
```

> **关键设计**：
> - `"matcher": "*"` → 匹配所有工具调用
> - `"async": true` → 异步，不影响 Claude 正常响应速度
> - 脚本内置 5 层防自循环保护（防止 AI 子代理的工具调用被递归记录）

#### 第 4 步：验证

使用 Claude Code 做几次操作后：

```bash
# 查看观察数据
ls ~/.claude/homunculus/projects/

# 查看某个项目的观察记录
cat ~/.claude/homunculus/projects/*/observations.jsonl | head -3 | jq .
```

---

## 第三部分：技能包自动同步到私有仓库

### 原理

你的 `~/claude-skills-private/` 目录是一个普通 Git 仓库。你只需要设置一个 cron job 定期 commit + push。

### 配置步骤

#### 第 1 步：初始化私有仓库

```bash
cd ~/claude-skills-private

# 如果还没有 Git
git init
git remote add origin https://github.com/你的用户名/claude-skills-private.git
```

> 在 GitHub 上创建一个 **Private** 仓库。

#### 第 2 步：设置自动同步 cron

创建同步脚本 `~/claude-skills-private/sync.sh`：

```bash
#!/bin/bash
cd ~/claude-skills-private || exit 1

# 也同步 homunculus 记忆数据（可选）
rsync -a --delete ~/.claude/homunculus/ ./homunculus/ 2>/dev/null

git add -A
CHANGES=$(git status --porcelain)
if [ -n "$CHANGES" ]; then
    git commit -m "auto: sync skills $(date +%Y-%m-%d_%H:%M)"
    git push origin HEAD 2>/dev/null || true
fi
```

```bash
chmod +x ~/claude-skills-private/sync.sh
```

#### 第 3 步：添加 cron 任务

```bash
crontab -e
```

添加一行（每小时自动同步一次）：

```
0 * * * * ~/claude-skills-private/sync.sh >> /tmp/skills-sync.log 2>&1
```

#### 第 4 步：验证

```bash
# 手动运行一次
~/claude-skills-private/sync.sh

# 检查 GitHub 仓库是否有新 commit
cd ~/claude-skills-private && git log --oneline -3
```

---

## 完整的 settings.json 示例

以下是一个包含所有三个功能的最小配置：

```json
{
  "cleanupPeriodDays": 99999,
  "hooks": {
    "SessionEnd": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/diary-log.sh",
            "timeout": 30,
            "async": true
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/skills/continuous-learning-v2/hooks/observe.sh pre",
            "timeout": 10,
            "async": true
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/skills/continuous-learning-v2/hooks/observe.sh post",
            "timeout": 10,
            "async": true
          }
        ]
      }
    ]
  }
}
```

---

## 数据流全景图

```
你使用 Claude Code
    │
    ├─ [每次工具调用] ──► observe.sh ──► ~/.claude/homunculus/projects/xxx/observations.jsonl
    │                                          │
    │                                          ├─ 自动脱敏 (API key → [REDACTED])
    │                                          └─ 按项目隔离存储
    │
    ├─ [关闭对话时] ──► diary-log.sh ──► curl → LLM API ──► ~/.claude/diary/2026-04-29.md
    │                                                         │
    │                                                         └─ 3-5 条高密度中文摘要
    │
    └─ [每小时 cron] ──► sync.sh ──► git commit + push ──► GitHub 私有仓库
                                       │
                                       ├─ ~/claude-skills-private/  (技能包源码)
                                       └─ ~/claude-skills-private/homunculus/  (记忆数据)
```

## 常见问题

### Q: 日记没生成？
1. 检查 `jq` 是否安装：`brew install jq`
2. 检查 LLM API 是否可达：`curl -s $DIARY_LLM_URL`
3. 手动测试脚本：`echo '{"cwd":"/tmp","transcript_path":""}' | ~/.claude/hooks/diary-log.sh`

### Q: 观察数据为空？
1. 检查 Python 是否可用：`python3 --version`
2. 检查符号链接是否正确：`ls -la ~/.claude/skills/continuous-learning-v2`

### Q: 不想记录某个项目的行为？
在 `~/.zshrc` 中设置排除路径：
```bash
export ECC_OBSERVE_SKIP_PATHS="secret-project,company-internal"
```

### Q: 想完全禁用观察？
```bash
touch ~/.claude/homunculus/disabled
```
