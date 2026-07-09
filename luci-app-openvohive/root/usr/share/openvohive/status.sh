#!/bin/sh

BIN="/opt/openvohive/openvohive"
VERSION_FILE="/opt/openvohive/bin/version"
ARCH_FILE="/opt/openvohive/bin/arch"

json_escape() {
	printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

uci_get() {
	local value
	value="$(uci -q get "openvohive.main.$1" 2>/dev/null || true)"
	[ -n "$value" ] && printf '%s' "$value" || printf '%s' "$2"
}

detect_arch() {
	case "$(uname -m)" in
		aarch64|arm64) echo "arm64" ;;
		x86_64|amd64)   echo "amd64" ;;
		armv7l|armv7)   echo "armv7" ;;
		*)              echo "amd64" ;;
	esac
}

# 服务状态
is_running=0
if [ -f /var/run/openvohive.pid ]; then
	pid=$(cat /var/run/openvohive.pid 2>/dev/null)
	[ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && is_running=1
fi
[ "$is_running" -eq 0 ] && pgrep -f "$BIN" >/dev/null 2>&1 && is_running=1

enabled="$(uci_get enabled '0')"
core_installed=0
core_version=""
core_arch=""

if [ -x "$BIN" ]; then
	core_installed=1
	core_version="$(cat "$VERSION_FILE" 2>/dev/null || true)"
	[ -n "$core_version" ] || core_version="未知"
	core_arch="$(cat "$ARCH_FILE" 2>/dev/null || true)"
	[ -n "$core_arch" ] || core_arch="$(detect_arch)"
else
	core_arch="$(detect_arch)"
fi

# 端口状态
port_status="unknown"
if command -v ss >/dev/null 2>&1; then
	if ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq '[:.]7575$'; then
		port_status="listening"
	else
		port_status="free"
	fi
fi

# 内存
memory_used_kb=0
memory_total_kb=$(awk '/^MemTotal:/ {print $2; exit}' /proc/meminfo 2>/dev/null || echo 0)
if [ "$is_running" -eq 1 ]; then
	rpid=$(cat /var/run/openvohive.pid 2>/dev/null || pgrep -f "$BIN" | head -1)
	if [ -n "$rpid" ]; then
		memory_used_kb=$(awk '/^VmRSS:/ {print $2; exit}' "/proc/$rpid/status" 2>/dev/null || echo 0)
	fi
fi

# 磁盘
root_line=$(df -kP / 2>/dev/null | awk 'NR==2 {print $2" "$3" "$5}')
root_total_kb=$(echo "$root_line" | awk '{print $1}')
root_used_kb=$(echo "$root_line" | awk '{print $2}')
root_percent=$(echo "$root_line" | awk '{print $3}' | tr -d '%')

data_path="$(uci_get data_path '/opt/openvohive/data')"
data_line=$(df -kP "$data_path" 2>/dev/null | awk 'NR==2 {print $2" "$3" "$5}')
data_total_kb=$(echo "$data_line" | awk '{print $1}')
data_used_kb=$(echo "$data_line" | awk '{print $2}')
data_percent=$(echo "$data_line" | awk '{print $3}' | tr -d '%')

printf '{'
printf '"running":%s,' "$is_running"
printf '"enabled":%s,' "$enabled"
printf '"core_installed":%s,' "$core_installed"
printf '"core_version":"%s",' "$(json_escape "$core_version")"
printf '"core_arch":"%s",' "$(json_escape "$core_arch")"
printf '"port_status":"%s",' "$port_status"
printf '"memory_used_kb":%s,' "${memory_used_kb:-0}"
printf '"memory_total_kb":%s,' "${memory_total_kb:-0}"
printf '"root_total_kb":%s,' "${root_total_kb:-0}"
printf '"root_used_kb":%s,' "${root_used_kb:-0}"
printf '"root_percent":%s,' "${root_percent:-0}"
printf '"data_total_kb":%s,' "${data_total_kb:-0}"
printf '"data_used_kb":%s,' "${data_used_kb:-0}"
printf '"data_percent":%s' "${data_percent:-0}"
printf '}\n'
