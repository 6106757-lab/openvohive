#!/bin/sh

TASK_DIR="/tmp/openvohive/tasks"
TASK_ID="${1}"
TASK_FILE="${TASK_DIR}/${TASK_ID}.json"

if [ -f "$TASK_FILE" ]; then
	# 标记为已取消
	sed -i 's/"state":"[^"]*"/"state":"canceled"/' "$TASK_FILE" 2>/dev/null
	# 杀掉对应 worker
	kill $(pgrep -f "task_worker.*$TASK_ID") 2>/dev/null || true
	printf '{"ok":true,"message":"任务已取消"}\n'
else
	printf '{"ok":false,"message":"任务不存在"}\n'
fi
