#!/bin/sh

BIN="/opt/openvohive/openvohive"
action="$1"

case "$action" in
	start)
		uci set openvohive.main.enabled='1'
		uci commit openvohive
		/etc/init.d/openvohive enable 2>/dev/null || true
		/etc/init.d/openvohive start 2>/dev/null || true
		sleep 2
		if pgrep -f "$BIN" >/dev/null 2>&1; then
			printf '{"ok":true,"message":"Open-VoHive 已启动"}\n'
		else
			printf '{"ok":false,"message":"启动失败，请查看日志"}\n'
		fi
		;;
	stop)
		/etc/init.d/openvohive stop 2>/dev/null || true
		killall -9 openvohive 2>/dev/null || true
		uci set openvohive.main.enabled='0'
		uci commit openvohive
		/etc/init.d/openvohive disable 2>/dev/null || true
		sleep 1
		printf '{"ok":true,"message":"Open-VoHive 已停止"}\n'
		;;
	restart)
		/etc/init.d/openvohive stop 2>/dev/null || true
		killall -9 openvohive 2>/dev/null || true
		sleep 2
		/etc/init.d/openvohive start 2>/dev/null || true
		sleep 2
		if pgrep -f "$BIN" >/dev/null 2>&1; then
			printf '{"ok":true,"message":"Open-VoHive 已重启"}\n'
		else
			printf '{"ok":false,"message":"重启失败，请查看日志"}\n'
		fi
		;;
	*)
		printf '{"ok":false,"message":"未知操作: %s"}\n' "$action"
		;;
esac
