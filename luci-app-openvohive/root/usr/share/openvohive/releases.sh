#!/bin/sh

MAX="${1:-5}"
REPO="6106757-lab/openvohive"
API="https://api.github.com/repos/${REPO}/releases?per_page=${MAX}"

# 自动检测设备架构
detect_arch() {
	case "$(uname -m)" in
		aarch64|arm64) echo "arm64" ;;
		x86_64|amd64)   echo "amd64" ;;
		armv7l|armv7)   echo "armv7" ;;
		*)              echo "amd64" ;;
	esac
}

ARCH="$(detect_arch)"

# 获取 release 列表
resp=$(curl -s --max-time 10 -H "Accept: application/vnd.github.v3+json" "$API" 2>/dev/null)

if [ -z "$resp" ]; then
	printf '{"ok":false,"message":"无法连接 GitHub API","latest":"","versions":[],"detected_arch":"%s"}\n' "$ARCH"
	exit 0
fi

# 提取版本号（tag_name，去掉 v 前缀）
# OpenWrt jsonfilter 语法: $[*].tag_name
versions=$(echo "$resp" | jsonfilter -e '$[*].tag_name' 2>/dev/null | sed 's/^v//' | head -n "$MAX")

# 最新版本
latest=$(echo "$versions" | head -1)

# 构造 JSON 数组
json_versions=""
first=1
for v in $versions; do
	if [ $first -eq 1 ]; then first=0; else json_versions="${json_versions},"; fi
	json_versions="${json_versions}\"v${v}\""
done

printf '{"ok":true,"latest":"v%s","versions":[%s],"detected_arch":"%s","repo":"%s"}\n' \
	"$latest" "$json_versions" "$ARCH" "$REPO"
