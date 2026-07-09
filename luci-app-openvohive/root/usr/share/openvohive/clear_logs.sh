#!/bin/sh

LOG_DIR="/opt/openvohive/logs"

if [ -d "$LOG_DIR" ]; then
	find "$LOG_DIR" -name "*.log" -type f -exec sh -c 'echo "" > "$1"' _ {} \; 2>/dev/null
	printf '{"ok":true,"message":"日志已清理"}\n'
else
	printf '{"ok":false,"message":"日志目录不存在"}\n'
fi
