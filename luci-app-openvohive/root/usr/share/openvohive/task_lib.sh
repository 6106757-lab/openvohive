#!/bin/sh

# 任务公共库
TASK_DIR="/tmp/openvohive/tasks"
mkdir -p "$TASK_DIR"

new_task_id() {
	echo "$$$(date +%s)$RANDOM" | md5sum | cut -c1-16
}

task_file() {
	echo "${TASK_DIR}/${1}.json"
}

write_task() {
	local id="$1" state="$2"
	shift 2
	cat > "$(task_file "$id")" <<EOF
{"id":"$id","state":"$state","percent":0,"total":0,"downloaded":0,"speed_bps":0,"log":[]$*}
EOF
}

append_log() {
	local id="$1" msg="$2"
	local f
	f="$(task_file "$id")"
	[ -f "$f" ] || return
	# 用 sed 在 log 数组末尾追加
	sed -i "s|\"log\":\[|\"log\":\[\"$(echo "$msg" | sed 's/"/\\"/g')\",|" "$f" 2>/dev/null
}
