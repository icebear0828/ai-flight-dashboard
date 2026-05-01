#!/bin/bash
# Auto-restart dashboard + scheduler after successful dashboard build
INPUT=$(cat)
CMD=$(echo "$INPUT" | jq -r '.tool_input.command // ""')
EXIT_CODE=$(echo "$INPUT" | jq -r '.tool_response.exitCode // 1')

# Only trigger on successful dashboard builds
[ "$EXIT_CODE" -ne 0 ] && exit 0
[[ "$CMD" != *"dashboard"* ]] && exit 0
[[ "$CMD" != *"npm run build"* ]] && exit 0

# Kill old process if port is occupied, then restart
lsof -ti:3200 | xargs kill -9 2>/dev/null
sleep 1
tmux respawn-pane -k -t dashboard 'cd ~/pi-mono/packages/dashboard && node dist/server/index.js' 2>/dev/null
tmux respawn-pane -k -t pi-scheduler 'cd ~/pi-mono && npx tsx .pi/extensions/shared/scheduling/scheduler-main.ts' 2>/dev/null

echo '{"hookSpecificOutput":{"additionalContext":"Dashboard + scheduler auto-restarted"}}'
exit 0
