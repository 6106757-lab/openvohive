package device

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/openvohive/openvohive/pkg/logger"
)

// runNetworkConfigDaemon 网络配置守护循环
// 负责：策略路由、NAT、LAN IPv6 分配（编译进二进制，无需外部脚本）
func (p *Pool) runNetworkConfigDaemon() {
	logger.Info("网络配置守护进程已启动")

	// 等待初始连接建立
	time.Sleep(30 * time.Second)

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			logger.Info("网络配置守护进程已停止")
			return
		case <-ticker.C:
			p.applyNetworkConfig()
		}
	}
}

// applyNetworkConfig 应用网络配置（策略路由 + NAT + LAN IPv6）
func (p *Pool) applyNetworkConfig() {
	p.mu.RLock()
	workers := make([]*Worker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.mu.RUnlock()

	for _, w := range workers {
		if w == nil {
			continue
		}
		if !w.NetworkConnected() {
			continue
		}
		iface := w.Config.Interface
		if iface == "" {
			iface = "wwan0"
		}
		p.configureIPv4Routing(iface)
		p.configureIPv6LAN(iface)
	}
}

// configureIPv4Routing 配置 IPv4 策略路由和 NAT
func (p *Pool) configureIPv4Routing(iface string) {
	// 获取 wwan0 的 IP 和网关
	ipOutput, err := exec.Command("sh", "-c",
		fmt.Sprintf("ip -4 addr show %s | grep 'inet ' | awk '{print $2}'", iface)).Output()
	if err != nil {
		return
	}
	cidr := strings.TrimSpace(string(ipOutput))
	if cidr == "" {
		return
	}
	ip := strings.Split(cidr, "/")[0]

	// 获取默认网关
	gwOutput, err := exec.Command("sh", "-c",
		fmt.Sprintf("ip route show | grep 'default.*%s' | head -1 | awk '{print $3}'", iface)).Output()
	if err != nil {
		return
	}
	gw := strings.TrimSpace(string(gwOutput))
	if gw == "" {
		// 推测网关
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			last, _ := strconv.Atoi(parts[3])
			gw = fmt.Sprintf("%s.%s.%s.%d", parts[0], parts[1], parts[2], last+1)
		} else {
			return
		}
	}

	// 1. 路由表
	exec.Command("sh", "-c", "grep -q '200 wwan0_table' /etc/iproute2/rt_tables || echo '200 wwan0_table' >> /etc/iproute2/rt_tables").Run()

	// 2. 清理旧路由 + 添加新路由
	exec.Command("sh", "-c", fmt.Sprintf(
		"ip route del default dev %s metric 5000 2>/dev/null; "+
			"ip route replace default via %s dev %s metric 100 2>/dev/null",
		iface, gw, iface)).Run()

	// 3. 策略路由表
	exec.Command("sh", "-c", fmt.Sprintf(
		"ip route flush table 200 2>/dev/null; "+
			"ip route add default via %s dev %s table 200 2>/dev/null",
		gw, iface)).Run()

	// 4. 清理所有旧的 from-IP 策略规则（保留 fwmark 和当前 IP）
	// 清理 lookup wwan0_table 的 from-IP 规则
	exec.Command("sh", "-c",
		"ip rule show | grep 'wwan0_table' | grep 'from ' | grep -v fwmark | "+
			"while read line; do "+
			"  prio=$(echo \"$line\" | awk '{print $1}' | cut -d: -f1); "+
			"  oldip=$(echo \"$line\" | awk '{print $2}'); "+
			"  if [ \"$oldip\" != \""+ip+"\" ]; then "+
			"    ip rule del priority $prio 2>/dev/null; "+
			"  fi; "+
			"done").Run()

	// 5. 当前 IP 策略规则
	exec.Command("sh", "-c", fmt.Sprintf(
		"ip rule del from %s lookup 200 2>/dev/null; "+
			"ip rule add from %s lookup 200 2>/dev/null; "+
			"ip rule del fwmark 0x200 lookup 200 2>/dev/null; "+
			"ip rule add fwmark 0x200 lookup 200 2>/dev/null",
		ip, ip)).Run()

	// 6. NAT (先删除再添加，避免重复)
	exec.Command("sh", "-c", fmt.Sprintf(
		"while iptables -t nat -D POSTROUTING -s 172.20.1.0/24 -o %s -j MASQUERADE 2>/dev/null; do :; done; "+
			"iptables -t nat -A POSTROUTING -s 172.20.1.0/24 -o %s -j MASQUERADE",
		iface, iface)).Run()

	// 7. rp_filter
	exec.Command("sh", "-c", fmt.Sprintf(
		"echo 0 > /proc/sys/net/ipv4/conf/%s/rp_filter 2>/dev/null; "+
			"echo 0 > /proc/sys/net/ipv4/conf/br-lan/rp_filter 2>/dev/null",
		iface)).Run()

	logger.Debug(fmt.Sprintf("[netcfg] IPv4 策略路由: %s gw %s", ip, gw))
}

// configureIPv6LAN 给 br-lan 分配 IPv6 地址
func (p *Pool) configureIPv6LAN(iface string) {
	// 获取 wwan0 的全局 IPv6 前缀
	v6Output, err := exec.Command("sh", "-c",
		fmt.Sprintf("ip -6 addr show %s | grep 'inet6.*global' | awk '{print $2}' | head -1", iface)).Output()
	if err != nil {
		return
	}
	wwanV6 := strings.TrimSpace(string(v6Output))
	if wwanV6 == "" {
		return
	}

	// 提取 /64 前缀
	prefix := strings.Split(wwanV6, "/")[0]
	parts := strings.Split(prefix, ":")
	if len(parts) < 4 {
		return
	}
	prefix64 := strings.Join(parts[:4], ":") + "::/64"
	lanIP := strings.Join(parts[:4], ":") + "::1/64"

	// 检查 br-lan 是否已有全局 IPv6
	currentOutput, _ := exec.Command("sh", "-c",
		"ip -6 addr show br-lan | grep 'inet6.*global' | awk '{print $2}' | head -1").Output()
	currentV6 := strings.TrimSpace(string(currentOutput))

	if currentV6 == "" || !strings.HasPrefix(currentV6, strings.Join(parts[:4], ":")) {
		// 前缀变了或没有地址，重新分配

		// 关键: 先把 wwan0 的 IPv6 从 /64 收紧为 /128
		// 避免同一个 /64 同时出现在 wwan0 和 br-lan 导致回程路由冲突
		wwanAddr := strings.Split(wwanV6, "/")[0]
		exec.Command("sh", "-c", fmt.Sprintf(
			"ip -6 addr del %s/64 dev %s 2>/dev/null; "+
				"ip -6 addr add %s/128 dev %s 2>/dev/null",
			wwanAddr, iface, wwanAddr, iface)).Run()

		// 更新 br-lan
		exec.Command("sh", "-c", "ip -6 addr flush dev br-lan scope global 2>/dev/null").Run()
		exec.Command("sh", "-c", fmt.Sprintf("ip -6 addr add %s dev br-lan 2>/dev/null", lanIP)).Run()

		// 确保 /64 路由指向 br-lan，并删除可能存在的 unreachable 路由
		// (vohive_wwan0 的 ip6prefix 会生成 unreachable 路由，需要清除)
		exec.Command("sh", "-c", fmt.Sprintf(
			"ip -6 route del unreachable %s 2>/dev/null; "+
				"ip -6 route replace %s dev br-lan 2>/dev/null",
			prefix64, prefix64)).Run()

		// 更新 uci 配置（不再写 ip6assign/ip6prefix，避免 unreachable 路由）
		exec.Command("sh", "-c", fmt.Sprintf(
			"uci delete network.lan.ip6prefix 2>/dev/null; "+
				"uci delete network.lan.ip6assign 2>/dev/null; "+
				"uci commit network 2>/dev/null")).Run()

		// 重启 odhcpd 让 RA 生效
		exec.Command("sh", "-c", "/etc/init.d/odhcpd restart 2>/dev/null").Run()

		logger.Info(fmt.Sprintf("[netcfg] LAN IPv6 已分配: %s (wwan0 /64→/128)", lanIP))
	}

	// IPv6 NAT
	exec.Command("sh", "-c", fmt.Sprintf(
		"ip6tables -t nat -C POSTROUTING -s %s -o %s -j MASQUERADE 2>/dev/null || "+
			"ip6tables -t nat -A POSTROUTING -s %s -o %s -j MASQUERADE 2>/dev/null",
		prefix64, iface, prefix64, iface)).Run()
}

// 确保 bufio/os/exec 被使用
var _ = bufio.NewScanner
var _ = os.Getenv
