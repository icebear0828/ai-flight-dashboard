# Claude Code Hooks 完全指南：让 AI 编程助手自动写日记、记住你的习惯

Claude Code 有一套"钩子（Hooks）"机制，可以在特定事件发生时自动执行脚本。配置完成后，每次你关闭对话，它会自动用 LLM 生成一篇工作日志；每次 Claude 调用工具，它会把你的编码行为模式记录下来；所有数据每小时自动同步到私有 GitHub 仓库。本文从零开始，带你完整配置这三个功能。

---

## 一、Hooks 是什么

Hooks 是 Claude Code 提供的事件触发机制。当指定事件发生时，Claude Code 会把事件数据（JSON 格式）通过标准输入传给你的脚本。

四个可用事件：

- **`PreToolUse`** — Claude 调用工具之前（执行命令、编辑文件等）
- **`PostToolUse`** — Claude 调用工具之后
- **`Notification`** — Claude 需要你审批操作时
- **`SessionEnd`** — 你关闭一次对话时

所有钩子配置写在 `~/.claude/settings.json` 文件中。

[Claude Code 终端窗口，屏幕上滚动显示工作日志自动生成的过程]

---

## 二、功能一：关闭对话自动写日记

### 工作原理

```
你关闭 Claude Code 对话
       ↓
SessionEnd 钩子触发
       ↓
diary-log.sh 脚本运行
       ↓
1. 读取本次对话的完整记录
2. 提取项目名、Git 最近提交
3. 发送给 LLM API
4. LLM 返回 3-5 条高密度摘要
5. 追加写入 ~/.claude/diary/YYYY-MM-DD.md
```

### 第 1 步：创建日记脚本

```bash
mkdir -p ~/.claude/hooks
```

创建文件 `~/.claude/hooks/diary-log.sh`：

```bash
#!/bin/bash
# SessionEnd hook: 用 LLM 自动写日记
set -euo pipefail

DIARY_DIR="$HOME/.claude/diary"
mkdir -p "$DIARY_DIR"

# 读取 Claude Code 传入的 JSON
INPUT=$(cat)
CWD=$(echo "$INPUT" | jq -r '.cwd // empty')
TRANSCRIPT_PATH=$(echo "$INPUT" | jq -r '.transcript_path // empty')

PROJECT=$(basename "${CWD:-unknown}")
TIMESTAMP=$(date '+%H:%M')
DATE=$(date '+%Y-%m-%d')
DIARY_FILE="$DIARY_DIR/$DATE.md"

# LLM 配置 — 改成你自己的 API 地址和 Key
LLM_URL="${DIARY_LLM_URL:-https://api.openai.com/v1/chat/completions}"
LLM_KEY="${DIARY_LLM_KEY:-sk-你的key}"
LLM_MODEL="${DIARY_LLM_MODEL:-gpt-4o-mini}"

# 系统提示词：信息压缩引擎
SYSTEM_PROMPT="你是一个高密度日志压缩器。将开发者-AI对话提炼为3-5条中文名词化要点。只记录已完成的工程成果。不使用第一人称。不输出任何开头结尾寒暄。"

# 从记录中提取对话文本
SUMMARY=""
if [ -n "$TRANSCRIPT_PATH" ] && [ -f "$TRANSCRIPT_PATH" ]; then
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

赋予执行权限：

```bash
chmod +x ~/.claude/hooks/diary-log.sh
```

### 第 2 步：注册到 settings.json

编辑 `~/.claude/settings.json`，加入以下内容：

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

三个关键参数：

- `"matcher": ""` — 空字符串匹配所有 session
- `"timeout": 30` — 最多等 30 秒（LLM API 可能较慢）
- `"async": true` — 异步执行，不阻塞关闭流程

### 第 3 步：配置你的 LLM API

在 `~/.zshrc` 中添加环境变量：

```bash
# 使用 OpenAI
export DIARY_LLM_URL="https://api.openai.com/v1/chat/completions"
export DIARY_LLM_KEY="sk-你的key"
export DIARY_LLM_MODEL="gpt-4o-mini"

# 或者使用本地 Ollama（离线可用）
# export DIARY_LLM_URL="http://localhost:11434/v1/chat/completions"
# export DIARY_LLM_KEY="ollama"
# export DIARY_LLM_MODEL="qwen2.5:7b"
```

### 第 4 步：验证

关闭一次 Claude Code 对话，然后检查日记文件：

```bash
cat ~/.claude/diary/$(date +%Y-%m-%d).md
```

正常情况下会看到类似输出：

```markdown
### 17:30 — my-project

- API 端点 `/api/cache-savings` 新增与测试覆盖
- CLI 参数三级配置体系落地：`--billing-mode` / `--plan` / `--budget-daily`
- 预算告警引擎阈值分级逻辑实现与 TUI 集成渲染
```

[开发者终端屏幕，按日期整理的工作日志文件列表，每个文件对应一天的 AI 编程工作记录]

---

## 三、功能二：自动记录行为模式

### 工作原理

每次 Claude 调用工具（运行命令、编辑文件、搜索代码），`observe.sh` 脚本会把工具名称、输入输出记录到本地文件。这些数据可以被后续的分析 Agent 处理，提炼出你的编码习惯（比如"总是先写测试"）。

```
Claude 调用任何工具
       ↓
PreToolUse / PostToolUse 钩子触发
       ↓
observe.sh 脚本运行
       ↓
1. 解析工具名、输入、输出
2. 检测当前项目（通过 git root）
3. 自动脱敏（API key、密码等替换为 [REDACTED]）
4. 写入 ~/.claude/homunculus/projects/<id>/observations.jsonl
```

脚本内置 5 层防自循环保护，不会把 AI 子代理的工具调用递归记录进去。

### 第 1 步：获取技能包

```bash
# 方法 A：如果你有现成的 skills 仓库
git clone https://github.com/你的用户名/claude-skills-private.git ~/claude-skills-private

# 方法 B：手动创建目录
mkdir -p ~/claude-skills-private/continuous-learning-v2/hooks
mkdir -p ~/claude-skills-private/continuous-learning-v2/scripts
mkdir -p ~/claude-skills-private/continuous-learning-v2/agents
```

### 第 2 步：创建符号链接

```bash
mkdir -p ~/.claude/skills
ln -s ~/claude-skills-private/continuous-learning-v2/ ~/.claude/skills/continuous-learning-v2
```

### 第 3 步：注册观察者钩子

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

### 第 4 步：验证

用 Claude Code 做几次操作后：

```bash
# 查看观察数据目录
ls ~/.claude/homunculus/projects/

# 查看某个项目的观察记录（前 3 条）
cat ~/.claude/homunculus/projects/*/observations.jsonl | head -3 | jq .
```

如果不想记录某个项目：

```bash
export ECC_OBSERVE_SKIP_PATHS="secret-project,company-internal"
```

完全禁用观察：

```bash
touch ~/.claude/homunculus/disabled
```

[服务器日志界面，实时滚动显示工具调用记录，每一行代表一次 AI 编程操作被捕获]

---

## 四、功能三：技能包自动同步到私有仓库

### 工作原理

`~/claude-skills-private/` 是一个普通 Git 仓库。通过 cron job 定期 commit + push，把你积累的技能包和行为数据备份到 GitHub 私有仓库。

### 第 1 步：初始化私有仓库

在 GitHub 创建一个 **Private** 仓库，然后：

```bash
cd ~/claude-skills-private
git init
git remote add origin https://github.com/你的用户名/claude-skills-private.git
```

### 第 2 步：创建同步脚本

创建 `~/claude-skills-private/sync.sh`：

```bash
#!/bin/bash
cd ~/claude-skills-private || exit 1

# 同步 homunculus 记忆数据（可选）
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

### 第 3 步：添加 cron 任务

```bash
crontab -e
```

添加一行，每小时自动同步：

```
0 * * * * ~/claude-skills-private/sync.sh >> /tmp/skills-sync.log 2>&1
```

### 第 4 步：验证

```bash
# 手动运行一次
~/claude-skills-private/sync.sh

# 检查最近 commit
cd ~/claude-skills-private && git log --oneline -3
```

---

## 五、完整配置文件

以下是包含所有三个功能的 `~/.claude/settings.json` 最小配置：

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

> `cleanupPeriodDays: 99999` 让 Claude Code 保留尽可能长时间的对话记录，方便日记脚本读取完整内容。

---

## 六、数据流全景

```
你使用 Claude Code
    │
    ├─ [每次工具调用]
    │       ↓
    │   observe.sh
    │       ↓
    │   ~/.claude/homunculus/projects/xxx/observations.jsonl
    │   （自动脱敏 + 按项目隔离存储）
    │
    ├─ [关闭对话时]
    │       ↓
    │   diary-log.sh → LLM API
    │       ↓
    │   ~/.claude/diary/2026-04-29.md
    │   （3-5 条高密度中文摘要）
    │
    └─ [每小时 cron]
            ↓
        sync.sh → git commit + push
            ↓
        GitHub 私有仓库
        ├─ ~/claude-skills-private/（技能包源码）
        └─ ~/claude-skills-private/homunculus/（记忆数据）
```

[数据流向图，从开发者电脑到云端 GitHub 仓库，三条路径分别标注日记、行为观察、自动同步]

---

## 七、常见问题

**Q: 日记没有生成？**

按顺序检查：

1. `jq` 是否安装：`brew install jq`
2. LLM API 是否可达：`curl -s $DIARY_LLM_URL`
3. 手动测试脚本：`echo '{"cwd":"/tmp","transcript_path":""}' | ~/.claude/hooks/diary-log.sh`

**Q: 观察数据为空？**

1. Python 是否可用：`python3 --version`
2. 符号链接是否正确：`ls -la ~/.claude/skills/continuous-learning-v2`

**Q: 不同项目使用不同 LLM？**

在 `~/.zshrc` 中单独设置 `DIARY_LLM_URL` 和 `DIARY_LLM_KEY` 环境变量，日记脚本会优先读取当前 shell 环境中的值。

**Q: 观察数据文件会无限增长吗？**

`observe.sh` 内置自动归档：单个文件超过 10MB 时自动归档；30 天前的旧文件自动删除。

---

## 八、进阶：PostToolUse 触发自动重启

PostToolUse 钩子还可以做更多事情。以下是一个实战案例：`npm run build` 成功后自动重启 dashboard 服务。

```bash
#!/bin/bash
# 自动重启 dashboard + scheduler
INPUT=$(cat)
CMD=$(echo "$INPUT" | jq -r '.tool_input.command // ""')
EXIT_CODE=$(echo "$INPUT" | jq -r '.tool_response.exitCode // 1')

# 只在 dashboard 构建成功时触发
[ "$EXIT_CODE" -ne 0 ] && exit 0
[[ "$CMD" != *"dashboard"* ]] && exit 0
[[ "$CMD" != *"npm run build"* ]] && exit 0

# 释放端口并重启服务
lsof -ti:3200 | xargs kill -9 2>/dev/null
sleep 1
tmux respawn-pane -k -t dashboard 'cd ~/pi-mono/packages/dashboard && node dist/server/index.js' 2>/dev/null
tmux respawn-pane -k -t pi-scheduler 'cd ~/pi-mono && npx tsx .pi/extensions/shared/scheduling/scheduler-main.ts' 2>/dev/null

echo '{"hookSpecificOutput":{"additionalContext":"Dashboard + scheduler auto-restarted"}}'
exit 0
```

这个脚本只会在构建命令成功后触发，对其他工具调用无影响。

[开发者工作区，多个终端窗口同时运行，左侧显示代码编辑，右侧显示服务自动重启的日志输出]

---

## 总结

三个功能对应三个独立脚本，互不干扰，可以按需选择：

**自动日记**
- 脚本：`diary-log.sh`
- 触发事件：`SessionEnd`
- 数据位置：`~/.claude/diary/`

**行为观察**
- 脚本：`observe.sh`
- 触发事件：`Pre/PostToolUse`
- 数据位置：`~/.claude/homunculus/`

**自动同步**
- 脚本：`sync.sh`
- 触发：cron（每小时）
- 数据位置：GitHub 私有仓库

配置完成后，Claude Code 从一个对话工具变成一个有记忆、会学习的工作伙伴。每天的工作不再只停留在聊天记录里，而是以结构化数据的形式沉淀下来。
