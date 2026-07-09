#!/bin/sh

# 安装核心脚本
# 参数: $1 = 版本号 (如 v2.0.1 或 latest)
#       $2 = 架构 (自动检测，可选)

REPO="6106757-lab/openvohive"
VERSION="${1:-latest}"
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

ARCH="${2:-$(detect_arch)}"
ASSET="openvohive-linux-${ARCH}"

# 解析版本
if [ "$VERSION" = "latest" ]; then
	API_URL="https://api.github.com/repos/${REPO}/releases/latest"
	TAG=$(curl -s --max-time 10 -H "Accept: application/vnd.github.v3+json" "$API_URL" 2>/dev/null | jsonfilter -e '$.tag_name' 2>/dev/null)
	if [ -z "$TAG" ]; then
		printf '{"ok":false,"message":"无法获取最新版本"}\n'
		exit 0
	fi
else
	TAG="$VERSION"
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

printf '{"ok":true,"message":"开始下载 Open-VoHive %s (%s)","version":"%s","arch":"%s","url":"%s"}\n' \
	"$TAG" "$ARCH" "$TAG" "$ARCH" "$DOWNLOAD_URL"

# 创建目录
mkdir -p "$BIN_DIR/bin" "$BIN_DIR/data" "$BIN_DIR/logs" "$BIN_DIR/config"

# 下载
TMP_FILE="/tmp/openvohive_download_$$"
if curl -sL --max-time 300 -o "$TMP_FILE" "$DOWNLOAD_URL" 2>/dev/null; then
	# 停止服务
	/etc/init.d/openvohive stop 2>/dev/null || true
	killall -9 openvohive 2>/dev/null || true
	sleep 1

	# 替换
	mv "$TMP_FILE" "$BIN"
	chmod +x "$BIN"

	# 写版本信息
	echo "$TAG" > "$VERSION_FILE"
	echo "$ARCH" > "$ARCH_FILE"

	printf '{"ok":true,"message":"Open-VoHive %s 安装成功！请点击启用并启动。","version":"%s"}\n' "$TAG" "$TAG"
else
	rm -f "$TMP_FILE"
	printf '{"ok":false,"message":"下载失败: %s"}\n' "$DOWNLOAD_URL"
fi
