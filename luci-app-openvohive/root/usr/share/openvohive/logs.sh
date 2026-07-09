#!/bin/sh

LOG_DIR="/opt/openvohive/logs"
MAX_LINES="${1:-100}"

if [ -d "$LOG_DIR" ]; then
	find "$LOG_DIR" -name "*.log" -type f -exec tail -n "$MAX_LINES" {} \; 2>/dev/null | tail -n "$MAX_LINES"
else
	echo "日志目录不存在: $LOG_DIR"
fi
