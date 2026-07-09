#!/bin/sh
# openvohive wwan0 网口配置脚本
# 在 OpenWrt 上为 wwan0 创建防火墙/NAT 规则，使其能作为 WAN 口上网

set -e

echo "=== 1. 确保 wwan0 在防火墙 wan zone ==="
WAN_SEC=$(uci show firewall | grep "=zone" | grep "name='wan'" | head -1 | cut -d= -f1)
if [ -z "$WAN_SEC" ]; then
    echo "错误: 未找到 name=wan 的防火墙 zone"
    exit 1
fi
echo "WAN zone: $WAN_SEC"

CUR_NET=$(uci -q get ${WAN_SEC}.network)
echo "当前 wan zone 网络成员: $CUR_NET"

# 检查 wwan0 是否已在列表中
IN_LIST=0
for net in $CUR_NET; do
    [ "$net" = "wwan0" ] && IN_LIST=1
done

if [ "$IN_LIST" = "0" ]; then
    echo "添加 wwan0 到防火墙 wan zone..."
    uci add_list ${WAN_SEC}.network=wwan0
    uci commit firewall
    echo "已添加"
else
    echo "wwan0 已在防火墙 wan zone 中，跳过"
fi

echo ""
echo "=== 2. 确保 wan zone 有 masquerade ==="
MASQ=$(uci -q get ${WAN_SEC}.masq)
if [ "$MASQ" != "1" ]; then
    echo "启用 masquerade..."
    uci set ${WAN_SEC}.masq=1
    uci commit firewall
else
    echo "masquerade 已启用"
fi

echo ""
echo "=== 3. 重载防火墙 ==="
fw4 reload 2>/dev/null || service firewall reload 2>/dev/null || /etc/init.d/firewall reload

echo ""
echo "=== 4. 检查最终配置 ==="
echo "WAN zone 网络成员:"
uci -q get ${WAN_SEC}.network
echo ""
echo "=== 完成 ==="
echo "wwan0 已配置为 WAN 口，支持 NAT 上网"
echo "openvohive 拨号后会自动给 wwan0 分配 IP 和默认路由"
