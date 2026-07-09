#!/bin/sh

TASK_DIR="/tmp/openvohive/tasks"
TASK_ID="${1}"

if [ -z "$TASK_ID" ]; then
	# 返回最近的任务
	latest=$(ls -t "$TASK_DIR"/*.json 2>/dev/null | head -1)
	if [ -n "$latest" ]; then
		cat "$latest"
	else
		printf '{"state":"idle","log":[]}\n'
	fi
else
	TASK_FILE="${TASK_DIR}/${TASK_ID}.json"
	if [ -f "$TASK_FILE" ]; then
		cat "$TASK_FILE"
	else
		printf '{"ok":false,"message":"任务 %s 不存在"}\n' "$TASK_ID"
	fi
fi
