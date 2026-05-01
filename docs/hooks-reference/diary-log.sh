#!/bin/bash
# SessionEnd hook: log a diary entry summarizing the session.
# Reads JSON from stdin with: session_id, cwd, transcript_path
# Appends a Markdown entry to ~/.claude/diary/YYYY-MM-DD.md
#
# Uses codex-proxy (OpenAI-compatible) with High-Entropy Log Synthesizer prompt.
# Override: DIARY_LLM_URL, DIARY_LLM_KEY, DIARY_LLM_MODEL, DIARY_MAX_CHARS

set -euo pipefail

DIARY_DIR="$HOME/.claude/diary"
mkdir -p "$DIARY_DIR"

# Read hook input
INPUT=$(cat)
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')
CWD=$(echo "$INPUT" | jq -r '.cwd // empty')
TRANSCRIPT_PATH=$(echo "$INPUT" | jq -r '.transcript_path // empty')

# Derive project name
PROJECT=$(basename "${CWD:-unknown}")
TIMESTAMP=$(date '+%H:%M')
DATE=$(date '+%Y-%m-%d')
DIARY_FILE="$DIARY_DIR/$DATE.md"

# LLM config
LLM_URL="${DIARY_LLM_URL:-http://192.168.10.6:8080/v1/chat/completions}"
LLM_KEY="${DIARY_LLM_KEY:-pwd}"
LLM_MODEL="${DIARY_LLM_MODEL:-codex}"
MAX_CHARS="${DIARY_MAX_CHARS:-800000}"  # ~800K chars, well within 1M token context

# --- jq helper: extract text from .message.content ---
JQ_EXTRACT_TEXT='
  if (.message.content | type) == "string" then .message.content
  elif (.message.content | type) == "array" then
    [.message.content[] | select(.type == "text") | .text] | join(" ")
  else "" end
'

# --- Git stats (if in a git repo) ---
GIT_STAT=""
if [ -n "$CWD" ] && [ -d "$CWD/.git" ]; then
    GIT_STAT=$(git -C "$CWD" log --oneline --since="1 hour ago" 2>/dev/null | head -10)
    if [ -z "$GIT_STAT" ]; then
        GIT_STAT=$(git -C "$CWD" diff --stat 2>/dev/null | tail -1)
    fi
fi

# --- System prompt: High-Entropy Log Synthesizer ---
read -r -d '' SYSTEM_PROMPT << 'SYSPROMPT' || true
# Role: High-Entropy Log Synthesizer

You are a highly specialized Information Compression Engine and Security Auditor designed for elite software engineers. Your core function is to ingest massive, noisy developer-LLM conversation transcripts, cross-reference them with Git commit statistics and project contexts (CWD), and distill the absolute essence of the engineering work into ultra-dense, high-information-entropy Chinese summaries. You act as a perfect bridge between sprawling machine dialogue and concise human technical journaling, guarded by a strict zero-leakage firewall.

## Goals
1. **Signal Extraction**: Sift through tens to hundreds of thousands of characters of conversation to isolate the actual, finalized engineering achievements, ignoring all debugging dead-ends, syntax errors, and AI pleasantries.
2. **Contextual Fusion**: Synthesize the `[Project Name]` (domain context) and `[Git Stats]` (scale of change) with the conversation's structural intent.
3. **Entropy Compression**: Translate verbose, narrative actions into exactly 3-5 high-entropy Chinese noun phrases.
4. **Absolute Sanitization**: Guarantee zero leakage of sensitive corporate, personal, or infrastructural data.

## Constraints
- **Formatting**: Output EXACTLY 3-5 bullet points using Markdown (`-`). Do NOT output any introductory greetings, explanations, formatting wrappers, or concluding remarks.
- **Language**: The output MUST be in highly professional, academic-level Chinese (简体中文).
- **Anti-Hallucination**: Only log the finalized, successful implementations explicitly confirmed in the transcript. Do NOT invent features, guess unwritten code, or include unadopted suggestions.
- **Security Redaction (Zero-Tolerance)**:
  - ABSOLUTELY NO API keys, internal IP addresses, real customer names/PII, database connection strings, passwords, or highly confidential internal repository/project names.
  - You MUST generalize sensitive entities (e.g., replace specific IPs with `[内网节点]`, replace keys with `[认证凭据]`, replace internal project names with `[内部核心业务系统]`).
- **Linguistic Entropy Rules**:
  - **MUST NOT** use first-person pronouns (我, 我们), conversational verbs (尝试了, 修复了, 讨论了), or chronological narratives (一开始, 然后, 最后).
  - **MUST** use **Nominalization (名词化/动名词)**. Construct phrases using `[Domain/Module] + [Action/State Change as a Noun]`.
SYSPROMPT

# --- Extract summary from transcript ---
SUMMARY=""
if [ -n "$TRANSCRIPT_PATH" ] && [ -f "$TRANSCRIPT_PATH" ]; then
    # Extract full conversation text (up to MAX_CHARS)
    CONTEXT=$(jq -r "
        select(.type == \"user\" or .type == \"assistant\")
        | (if .type == \"user\" then \"User: \" else \"Assistant: \" end)
          + ($JQ_EXTRACT_TEXT)
    " "$TRANSCRIPT_PATH" 2>/dev/null | tail -c "$MAX_CHARS")

    if [ -n "$CONTEXT" ]; then
        ESCAPED_SYSTEM=$(echo "$SYSTEM_PROMPT" | jq -Rs .)
        # Build user message with project context + git stats + transcript
        USER_MSG="[Project Name/CWD]: $PROJECT
[Git Stats]: ${GIT_STAT:-N/A}
[Transcript]:
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

    # Fallback: extract from transcript without LLM
    if [ -z "$SUMMARY" ]; then
        MSG_COUNT=$(jq -s '[.[] | select(.type == "user")] | length' "$TRANSCRIPT_PATH" 2>/dev/null)
        TOOLS_USED=$(jq -r 'select(.type == "assistant") | .message.content[]? | select(.type == "tool_use") | .name' "$TRANSCRIPT_PATH" 2>/dev/null | sort -u | head -10 | tr '\n' ', ' | sed 's/,$//')
        FIRST_MSG=$(jq -r "select(.type == \"user\") | $JQ_EXTRACT_TEXT" "$TRANSCRIPT_PATH" 2>/dev/null | head -1 | cut -c1-100)

        SUMMARY="- 会话轮数: ${MSG_COUNT:-0}"
        [ -n "$TOOLS_USED" ] && SUMMARY="$SUMMARY\n- 使用工具: $TOOLS_USED"
        [ -n "$FIRST_MSG" ] && SUMMARY="$SUMMARY\n- 起始话题: $FIRST_MSG"
    fi
fi

# --- Write diary entry ---
{
    echo ""
    echo "### $TIMESTAMP — $PROJECT"
    echo ""
    if [ -n "$SUMMARY" ]; then
        echo -e "$SUMMARY"
    else
        echo "- (无法提取会话摘要)"
    fi
    if [ -n "$GIT_STAT" ]; then
        echo ""
        echo "> $GIT_STAT"
    fi
    echo ""
} >> "$DIARY_FILE"

exit 0
