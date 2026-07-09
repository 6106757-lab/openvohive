#!/bin/sh

. /usr/share/openvohive/task_lib.sh

TYPE="${1:-install_core}"
VERSION="${2:-latest}"
REPO="6106757-lab/openvohive"
BIN_DIR="/opt/openvohive"
BIN="$BIN_DIR/openvohive"
VERSION_FILE="$BIN_DIR/bin/version"
ARCH_FILE="$BIN_DIR/bin/arch"

detect_arch() {
	case "$(uname -m)" in
		aarch64|arm64) echo "arm64" ;;
		x86_64|amd64)   echo "amd64" ;;
		armv7l|armv7)   echo "armv7" ;;
		*)              echo "amd64" ;;
	esac
}

case "$TYPE" in
	install_core)
		ARCH="$(detect_arch)"
		ASSET="openvohive-linux-${ARCH}"

		if [ "$VERSION" = "latest" ]; then
			API_URL="https://api.github.com/repos/${REPO}/releases/latest"
			TAG=$(curl -s --max-time 10 -H "Accept: application/vnd.github.v3+json" "$API_URL" 2>/dev/null | jsonfilter -e '$.tag_name' 2>/dev/null)
			[ -z "$TAG" ] && { printf '{"ok":false,"message":"无法获取最新版本"}\n'; exit 0; }
		else
			TAG="$VERSION"
		fi

		DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
		TASK_ID="$(new_task_id)"
		TASK_FILE="$(task_file "$TASK_ID")"

		# 写初始状态
		cat > "$TASK_FILE" <<EOF
{"id":"$TASK_ID","type":"install_core","state":"starting","stage":"准备下载","percent":0,"total":0,"downloaded":0,"speed_bps":0,"file":"$ASSET","version":"$TAG","cancellable":true,"log":["开始下载 $ASSET ($TAG)","URL: $DOWNLOAD_URL"]}
EOF

		# 后台执行下载
		/usr/share/openvohive/task_worker.sh "$TASK_ID" "$DOWNLOAD_URL" "$BIN" "$VERSION_FILE" "$ARCH_FILE" "$TAG" "$ARCH" &
		printf '{"ok":true,"id":"%s","message":"任务已启动"}\n' "$TASK_ID"
		;;
	*)
		printf '{"ok":false,"message":"不支持的任务类型: %s"}\n' "$TYPE"
		;;
esac
