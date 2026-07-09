#!/bin/sh

TASK_ID="$1"
URL="$2"
BIN="$3"
VERSION_FILE="$4"
ARCH_FILE="$5"
TAG="$6"
ARCH="$7"

TASK_DIR="/tmp/openvohive/tasks"
TASK_FILE="${TASK_DIR}/${TASK_ID}.json"
TMP_FILE="/tmp/openvohive_download_${TASK_ID}"

update_task() {
	local state="$1" percent="$2" downloaded="$3" total="$4" speed="$5" msg="$6"
	cat > "$TASK_FILE" <<EOF
{"id":"$TASK_ID","type":"install_core","state":"$state","stage":"$state","percent":$percent,"total":$total,"downloaded":$downloaded,"speed_bps":$speed,"file":"$(basename "$URL")","version":"$TAG","cancellable":true,"log":["$msg"]}
EOF
}

append_log() {
	sed -i "s|\"log\":\[|\"log\":\[\"$(echo "$1" | sed 's/"/\\"/g')\",|" "$TASK_FILE" 2>/dev/null
}

# 获取文件大小
TOTAL=$(curl -sI --max-time 10 "$URL" 2>/dev/null | grep -i content-length | awk '{print $2}' | tr -d '\r')
TOTAL="${TOTAL:-0}"

append_log "文件大小: ${TOTAL} bytes"

# 下载（带进度）
START_TIME=$(date +%s)
DOWNLOADED=0

# 使用 curl 下载
if curl -sL --max-time 600 -o "$TMP_FILE" "$URL" 2>/tmp/openvohive_curl_err_$$; then
	DOWNLOADED=$(wc -c < "$TMP_FILE" 2>/dev/null || echo 0)
	END_TIME=$(date +%s)
	ELAPSED=$((END_TIME - START_TIME))
	[ "$ELAPSED" -lt 1 ] && ELAPSED=1
	SPEED=$((DOWNLOADED / ELAPSED))
	PERCENT=100

	append_log "下载完成 (${DOWNLOADED} bytes, ${SPEED} B/s)"

	# 停止服务
	/etc/init.d/openvohive stop 2>/dev/null || true
	killall -9 openvohive 2>/dev/null || true
	sleep 1

	# 替换
	mkdir -p "$(dirname "$BIN")" "$(dirname "$VERSION_FILE")"
	mv "$TMP_FILE" "$BIN"
	chmod +x "$BIN"
	echo "$TAG" > "$VERSION_FILE"
	echo "$ARCH" > "$ARCH_FILE"

	update_task "completed" 100 "$DOWNLOADED" "$TOTAL" "$SPEED" "Open-VoHive ${TAG} 安装成功！请点击启用并启动。"
else
	ERR=$(cat /tmp/openvohive_curl_err_$$ 2>/dev/null || echo "未知错误")
	rm -f "$TMP_FILE" /tmp/openvohive_curl_err_$$
	append_log "下载失败: $ERR"
	update_task "failed" 0 "$DOWNLOADED" "$TOTAL" 0 "下载失败"
fi
