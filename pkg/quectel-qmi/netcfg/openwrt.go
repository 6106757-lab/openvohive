//go:build linux
// +build linux

package netcfg

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IsOpenWrt 在运行时探测当前系统是否为 OpenWrt。
// 探测顺序：环境变量 OPENWRT=1 > 存在 /etc/openwrt_release > ubus 可用。
func IsOpenWrt() bool {
	if os.Getenv("OPENWRT") == "1" {
		return true
	}
	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		return true
	}
	if _, err := exec.LookPath("ubus"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "ubus", "list", "network").Run(); err == nil {
			return true
		}
	}
	return false
}

// OpenWrtConfigurator 在 OpenWrt 上把 L3（IP/路由/DNS/MTU）交给 netifd 接管，
// 而 qmi_wwan 内核侧所需的 raw_ip / QMAP mux 仍由内嵌的 LinuxConfigurator 通过
// sysfs 完成（这些是 netifd 不做、且为 Raw IP 模式前置的必要内核操作）。
//
// 这样 VoHive 用 QMI 拨号拿到运营商下发的内网 IP/网关/DNS 后，通过
// `ubus call network add_dynamic` 把对应网口注册成 netifd 接口（默认名为
// vohive_<ifname>），由 OpenWrt 负责默认路由、DNS 下发给 dnsmasq、以及
// 防火墙 wan 区的 NAT——路由器（含 LAN 侧）即可正常使用该蜂窝连接。
type OpenWrtConfigurator struct {
	LinuxConfigurator                       // 复用 raw_ip / QMAP mux 等内核 sysfs 操作
	mu          sync.Mutex
	pending     map[string]*owPending
	committed   map[string]bool
	lastIfname  string
	applyMu     sync.Mutex // 串行化 applyNetifdStatic，杜绝并发 reload 互相打架导致 netifd 接口抖动/掉线
}

type owPending struct {
	v4       string
	v4Prefix int
	gw       string
	v6       string
	v6Prefix int
	gw6      string
	mtu      int
	dns      []string
	hasV4    bool
	hasV6    bool
	// pdPrefix/pdPrefixLen 是运营商委派的前缀(PD), 写入 netifd 接口的 ip6prefix,
	// 由 odhcpd 自动向 LAN 下发该前缀下的 IPv6 地址。
	pdPrefix    string
	pdPrefixLen int
	// wantV4OnLink/wantV6OnLink 表示是否采用 on-link 默认路由(无网关, default dev <iface>)。
	// 用户明确要求 IPv6 不填网关(ip6gw), 故 IPv6 走 on-link 默认路由。
	wantV4OnLink bool
	wantV6OnLink bool
}

// NewOpenWrtConfigurator 创建 OpenWrt 配置器。
func NewOpenWrtConfigurator() *OpenWrtConfigurator {
	return &OpenWrtConfigurator{
		pending:   make(map[string]*owPending),
		committed: make(map[string]bool),
	}
}

// owWANMetric 返回注册到 netifd 的接口默认路由优先级（metric）。
// 数值越小越优先。默认 100（作为备份 WAN，不抢主路由），可用环境变量
// VOHIVE_WAN_METRIC 覆盖。设更小的值（如 5）可让蜂窝成为主出口。
//
// 注意：必须使用与已有主 WAN（通常是 metric 10）不同的值，否则
// `ip route replace default` 会覆盖主 WAN 的默认路由导致回程中断。
func owWANMetric() int {
	if v := os.Getenv("VOHIVE_WAN_METRIC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 100
}

// netifdName 由数据网口名派生出稳定的 netifd 接口名（如 wwan0 -> vohive_wwan0）。
func (o *OpenWrtConfigurator) netifdName(ifname string) string {
	var b strings.Builder
	b.WriteString("vohive_")
	for _, r := range ifname {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (o *OpenWrtConfigurator) getPending(ifname string) *owPending {
	p := o.pending[ifname]
	if p == nil {
		p = &owPending{}
		o.pending[ifname] = p
	}
	o.lastIfname = ifname
	return p
}

func (o *OpenWrtConfigurator) SetIPAddress(ifname string, ip net.IP, prefixLen int) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	p := o.getPending(ifname)
	p.v4 = ip.String()
	p.v4Prefix = prefixLen
	p.hasV4 = true
	return nil
}

func (o *OpenWrtConfigurator) SetIPv6Address(ifname string, ip net.IP, prefixLen int) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	p := o.getPending(ifname)
	p.v6 = ip.String()
	p.v6Prefix = prefixLen
	p.hasV6 = true
	return nil
}

// SetIPv6DelegatedPrefix 将运营商委派前缀(PD)缓存到 pending, 稍后在
// applyNetifdStatic 中写入 netifd 接口的 ip6prefix, 由 odhcpd 向 LAN 自动下发。
func (o *OpenWrtConfigurator) SetIPv6DelegatedPrefix(ifname string, prefix net.IP, prefixLen int) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	p := o.getPending(ifname)
	if prefix != nil && prefixLen > 0 {
		p.pdPrefix = prefix.String()
		p.pdPrefixLen = prefixLen
	}
	return nil
}

func (o *OpenWrtConfigurator) AddDefaultRoute(ifname string, gateway net.IP) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	p := o.getPending(ifname)
	if gateway == nil {
		return nil
	}
	// 按地址族分流：IPv4 网关写入 gw，IPv6 网关写入 gw6，
	// 否则后设置的会覆盖先设置的，导致另一栈默认路由缺失。
	if gateway.To4() != nil {
		p.gw = gateway.String()
	} else {
		p.gw6 = gateway.String()
	}
	return nil
}

// AddDefaultRouteDirect 无网关（on-link）场景：记录意图, 稍后在 applyNetifdStatic
// 的 ifup 之后补一条 `default dev <iface>` 的 on-link 默认路由。
func (o *OpenWrtConfigurator) AddDefaultRouteDirect(ifname string, ipv6 bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	p := o.getPending(ifname)
	if ipv6 {
		p.wantV6OnLink = true
	} else {
		p.wantV4OnLink = true
	}
	return nil
}

// UpdateDNS 注意：NetworkConfigurator 接口未携带 ifname，但调用总发生在某网口
// 配置块内（紧随 SetIPAddress 之后），这里挂到最近一次被配置的网口上。
func (o *OpenWrtConfigurator) UpdateDNS(dns1, dns2 string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lastIfname == "" {
		return nil
	}
	p := o.getPending(o.lastIfname)
	if dns1 != "" {
		p.dns = append(p.dns, dns1)
	}
	if dns2 != "" && dns2 != dns1 {
		p.dns = append(p.dns, dns2)
	}
	return nil
}

func (o *OpenWrtConfigurator) SetMTU(ifname string, mtu int) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	p := o.getPending(ifname)
	p.mtu = mtu
	return nil
}

// BringUp 早期（尚未取得 IP）只确保内核设备 up；待配置齐备后（带 IP）提交给 netifd。
func (o *OpenWrtConfigurator) BringUp(ifname string) error {
	// 先确保内核侧网口 up（QMAP 虚拟网卡尤其需要）
	if err := o.LinuxConfigurator.BringUp(ifname); err != nil {
		log.Printf("[openwrt] bring up %s (kernel): %v", ifname, err)
	}
	return o.commit(ifname)
}

func (o *OpenWrtConfigurator) FlushAddresses(ifname string) error {
	// OpenWrt 下地址由 netifd 管理。这里只在设备层清一次内核地址，
	// 切勿在 flush 阶段移除 netifd 接口——拨号结束的 commit 会刷新并重建该接口，
	// 若此处先删，会与 commit 形成“删除-重建”竞态，导致接口时有时无、防火墙成员丢失。
	// 真正的清理由 BringDown->teardown 在断线/停止时完成。
	return o.LinuxConfigurator.FlushAddresses(ifname)
}

func (o *OpenWrtConfigurator) FlushRoutes(ifname string) error {
	o.teardown(ifname)
	return o.LinuxConfigurator.FlushRoutes(ifname)
}

func (o *OpenWrtConfigurator) BringDown(ifname string) error {
	o.teardown(ifname)
	return o.LinuxConfigurator.BringDown(ifname)
}

// RestoreDNS OpenWrt 下 DNS 由 netifd 管理，无需恢复 /etc/resolv.conf。
func (o *OpenWrtConfigurator) RestoreDNS() error { return nil }

// commit 把累积的 L3 配置交给 netifd 真正接管：
// 把 vohive_<ifname> 作为 `proto static` 的 uci 接口写入 /etc/config/network，
// 由 OpenWrt 负责 IP/默认路由(带 metric)/DNS 下发，LuCI 可见、防火墙 wan 区可挂 NAT，
// IPv4 与 IPv6 同时落位。这是“路由器像使用普通 WAN 一样使用该蜂窝口”的关键。
//
// 注：早期尝试过 `ubus call network add_dynamic`（proto=none）再内核配 IP 的方案，
// 但 proto=none 的 netifd 接口不会接管 L3（notify_proto 返回 Operation not supported），
// 导致 ifstatus 显示无 IP、防火墙/NAT 不生效。故改用 uci static 接口方案。
func (o *OpenWrtConfigurator) commit(ifname string) error {
	o.mu.Lock()
	p := o.pending[ifname]
	o.mu.Unlock()
	if p == nil || (!p.hasV4 && !p.hasV6) {
		// 还没拿到 IP（早期 BringUp），仅内核 up 即可，等待后续提交。
		return nil
	}

	name := o.netifdName(ifname)

	// 优先让 netifd 以 static 接口接管（含 IPv4+IPv6）。
	if err := o.applyNetifdStatic(name, ifname, p); err != nil {
		log.Printf("[openwrt] netifd 接管失败，回退内核直配: %v", err)
		if kerr := o.applyKernelFallback(ifname, p); kerr != nil {
			log.Printf("[openwrt] 内核直配也失败: %v", kerr)
			return kerr
		}
	}

	o.mu.Lock()
	o.committed[ifname] = true
	o.mu.Unlock()
	return nil
}

// applyNetifdStatic 通过 uci 把接口建成 netifd 拥有的 `proto static` 接口。
func (o *OpenWrtConfigurator) applyNetifdStatic(name, ifname string, p *owPending) error {
	// 串行化：configureNetworkInterface 会多次调用 BringUp（进而 commit→本函数），
	// 叠加 QMI 自动重连，可能在极短时间内重入本函数。多次 `uci delete+recreate+
	// reload+ifup` 并发执行会让 netifd 反复 bounce 接口，最终停在 down。
	// 用全局锁串行化，并用幂等短路避免无谓的重建抖动。
	o.applyMu.Lock()
	defer o.applyMu.Unlock()

	// 幂等：若 uci 配置已与期望一致且接口已 up，则只确保防火墙成员与 on-link 路由，
	// 不再 reload/ifup（这正是第二次 BringUp 与并发重连造成抖动的根因）。
	if o.owConfigured(name, p) && o.owIfaceUp(name) {
		if err := o.setFirewallWan(name, true); err != nil {
			log.Printf("[openwrt] 加入防火墙 wan 区失败（可忽略）: %v", err)
		}
		o.ensureOnLinkRoutes(name, ifname)
		log.Printf("[openwrt] netifd 接口 %s 配置未变且已 up，跳过重建", name)
		return nil
	}

	// 1) 清掉可能存在的旧块，重建（幂等）。
	runCmd("uci", "-q", "delete", "network."+name)

	setCmds := [][]string{
		{"uci", "set", "network." + name + "=interface"},
		{"uci", "set", "network." + name + ".proto=static"},
		{"uci", "set", "network." + name + ".device=" + ifname},
		{"uci", "set", "network." + name + ".metric=" + strconv.Itoa(owWANMetric())},
	}
	if p.hasV4 {
		setCmds = append(setCmds,
			[]string{"uci", "set", "network." + name + ".ipaddr=" + p.v4},
			[]string{"uci", "set", "network." + name + ".netmask=" + prefixToMask(p.v4Prefix)},
		)
		if p.gw != "" {
			setCmds = append(setCmds, []string{"uci", "set", "network." + name + ".gateway=" + p.gw})
		}
	}
	if p.hasV6 && p.v6 != "" {
		// WAN 自身 IPv6 地址收紧为 /128: 避免与下发给 LAN 的 /64(ip6prefix) 形成
		// “同一 /64 同时挂在 WAN 与 LAN 两个接口”的路由冲突(否则发往该 /64 的流量
		// 含 LAN 客户端会被错误路由到 WAN, 导致 LAN 客户端不可达)。
		// 仅当确实向 LAN 委派了前缀时才收紧为 /128; 否则保留原始前缀长度。
		addrLen := p.v6Prefix
		if p.pdPrefix != "" {
			addrLen = 128
		}
		setCmds = append(setCmds,
			[]string{"uci", "set", "network." + name + ".ip6addr=" + p.v6 + "/" + strconv.Itoa(addrLen)},
		)
		// 用户明确要求 IPv6 不填网关(ip6gw): 默认路由改为 on-link (default dev <iface>),
		// 在 ifup 之后由 ensureOnLinkRoutes 补。故此处不再写 ip6gw。
		// 不写 ip6prefix: 因为 proto=static 时 netifd 会生成 unreachable 路由，
		// 导致 LAN 客户端 IPv6 不可达。前缀同步由 netcfg_daemon 的 configureIPv6LAN 负责。
	}
	// DNS 列表
	// 注意：不要先 `uci delete network.<name>.dns`！函数开头已经整块删除并重建，
	// 新建时 .dns 选项本就不存在，而 `uci -q delete` 在选项缺失时仍返回退出码 1，
	// 会被 runCmd 当作错误，导致 applyNetifdStatic 在 commit/reload/ifup 之前就提前
	// return，整段 netifd 接管被跳过、退化内核直配、防火墙挂载也被跳过。
	// 因此直接 add_list 即可（整块已清空，不会残留旧 DNS）。
	if len(p.dns) > 0 {
		for _, d := range p.dns {
			setCmds = append(setCmds, []string{"uci", "add_list", "network." + name + ".dns=" + d})
		}
	}

	// MTU：蜂窝 QMAP/rmnet 隧道在 L3 之上还有封装开销，实际可承载 MTU 通常 < 1500。
	// 若沿用 1500（qmi_wwan 驱动默认 / 运营商名义值），大 TCP 段会在运营商链路被丢弃，
	// 表现为“ICMP、UDP、小包通，但 TCP（含 HTTPS）不通”的典型 MSS/MTU 黑洞，
	// 主机自身与局域网经该 WAN 的上行 TCP 都会失败。
	// 修复：把接口 MTU 钳到安全值——优先采用运营商下发值（若 ≤1440 视为合理），
	// 否则落到 1400。写进 uci 后由 netifd 在 ifup 时设到 wwan0，重启/重连均持久生效。
	mtu := p.mtu
	if mtu <= 0 || mtu > 1440 {
		mtu = 1400
	}
	setCmds = append(setCmds, []string{"uci", "set", "network." + name + ".mtu=" + strconv.Itoa(mtu)})
	for _, c := range setCmds {
		if out, err := runCmd(c[0], c[1:]...); err != nil {
			return fmt.Errorf("uci %v 失败: %w: %s", c, err, out)
		}
	}
	if out, err := runCmd("uci", "commit", "network"); err != nil {
		return fmt.Errorf("uci commit network 失败: %w: %s", err, out)
	}

	// 2) 让 netifd 重新加载配置并拉起接口（IP/路由/DNS 由 netifd 落位）。
	//    reload 为异步，且拨号早期 wwan0 可能尚未完全就绪，故带重试与“up”校验，
	//    确保接口真正被 netifd 接管（否则会退化成内核直配，LuCI 看不到 IP）。
	broughtUp := false
	for attempt := 1; attempt <= 3; attempt++ {
		if out, err := runCmd("ubus", "call", "network", "reload"); err != nil {
			log.Printf("[openwrt] ubus network reload 失败（尝试 %d），改 reload_config: %v (%s)", attempt, err, out)
			runCmd("reload_config")
		}
		o.waitInterfaceExists(name, 5*time.Second)
		runCmd("ifup", name)
		if o.waitInterfaceUp(name, 6*time.Second) {
			broughtUp = true
			break
		}
		log.Printf("[openwrt] 第 %d 次尝试拉起 %s 未就绪，重试", attempt, name)
	}
	if !broughtUp {
		return fmt.Errorf("netifd 多次尝试仍未拉起接口 %s", name)
	}

	// 4) 加入防火墙 wan 区（开启 masquerade/NAT）。
	if err := o.setFirewallWan(name, true); err != nil {
		log.Printf("[openwrt] 加入防火墙 wan 区失败（可忽略）: %v", err)
	}

	// 5) 补 on-link 默认路由（IPv6 不填网关，走 default dev <iface>）。
	o.ensureOnLinkRoutes(name, ifname)

	log.Printf("[openwrt] netifd 已接管 %s 为静态接口 %s (IPv4=%v, IPv6=%v, metric=%d)",
		ifname, name, p.hasV4, p.hasV6, owWANMetric())
	return nil
}

// ensureOnLinkRoutes 在 netifd 接口 up 后, 按 pending 意图补 on-link 默认路由
// (default dev <iface>)。IPv6 用户明确要求不填网关(ip6gw), 故走 on-link。
// 该路由不在 uci 中持久化, 因此每次 apply 都 ensure 一次(replace 幂等),
// 以覆盖 reload 重建接口后内核路由丢失的情况。
func (o *OpenWrtConfigurator) ensureOnLinkRoutes(name, ifname string) {
	if !o.owIfaceUp(name) {
		return
	}
	o.mu.Lock()
	p := o.pending[ifname]
	o.mu.Unlock()
	if p == nil {
		return
	}
	if p.wantV4OnLink && p.hasV4 && p.gw == "" {
		log.Printf("[openwrt] 添加 V4 on-link 默认路由 (dev %s, metric %d)", ifname, owWANMetric())
		if err := addDefaultRouteV4OnLink(ifname, owWANMetric()); err != nil {
			log.Printf("[openwrt] V4 on-link 默认路由失败: %v", err)
		}
	}
	if p.wantV6OnLink && p.hasV6 && p.gw6 == "" {
		log.Printf("[openwrt] 添加 V6 on-link 默认路由 (dev %s, metric %d)", ifname, owWANMetric())
		if err := addDefaultRouteV6OnLink(ifname, owWANMetric()); err != nil {
			log.Printf("[openwrt] V6 on-link 默认路由失败: %v", err)
		}
	}
}

// ensureLanPrefixRoute 从 wwan0 的 IPv6 地址提取 /64 前缀，
// 确保 br-lan 上有对应地址和路由，使 LAN 客户端能正确获得 IPv6。
// 仅在 OpenWrt 环境下生效（检测 /etc/config/network）。
func (o *OpenWrtConfigurator) ensureLanPrefixRoute(ip6 net.IP) error {
	// 提取 /64 前缀（前 4 组）
	mask := net.CIDRMask(64, 128)
	lanPrefix := ip6.Mask(mask)

	// 检查 br-lan 是否已有同前缀地址
	lanAddr := fmt.Sprintf("%s::1/64", lanPrefix.String()[:strings.LastIndex(lanPrefix.String(), ":")])
	check := exec.Command("ip", "-6", "addr", "show", "dev", "br-lan", "scope", "global")
	out, _ := check.CombinedOutput()
	if strings.Contains(string(out), lanPrefix.String()[:strings.LastIndex(lanPrefix.String(), ":")]) {
		return nil // 已存在，无需操作
	}

	// 更新 br-lan 地址
	delCmd := exec.Command("ip", "-6", "addr", "flush", "dev", "br-lan", "scope", "global")
	delCmd.Run() // 忽略错误

	addCmd := exec.Command("ip", "-6", "addr", "add", lanAddr, "dev", "br-lan")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("添加 br-lan IPv6 地址失败: %w: %s", err, string(out))
	}

	// 确保 /64 前缀路由指向 br-lan
	routeCmd := exec.Command("ip", "-6", "route", "replace",
		fmt.Sprintf("%s/64", lanPrefix.String()[:strings.LastIndex(lanPrefix.String(), ":")]),
		"dev", "br-lan")
	routeCmd.Run() // 忽略错误

	// 重启 odhcpd 使 LAN 客户端获得新前缀
	exec.Command("/etc/init.d/odhcpd", "restart").Run()

	log.Printf("[openwrt] br-lan IPv6 前缀已同步: %s", lanAddr)
	return nil
}

func addDefaultRouteV4OnLink(ifname string, metric int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ip", "route", "replace", "default", "dev", ifname, "metric", strconv.Itoa(metric))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip route replace v4 onlink: %w: %s", err, string(out))
	}
	return nil
}

func addDefaultRouteV6OnLink(ifname string, metric int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ip", "-6", "route", "replace", "default", "dev", ifname, "metric", strconv.Itoa(metric))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip -6 route replace onlink: %w: %s", err, string(out))
	}
	return nil
}

// owIfaceUp 瞬时判断 netifd 接口是否已 up（不等待），用于幂等短路。
func (o *OpenWrtConfigurator) owIfaceUp(name string) bool {
	out, err := ubusCall("network.interface."+name+" status", map[string]interface{}{})
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "\"up\": true")
}

// owConfigured 判断 uci 中该静态接口的配置是否已与期望一致。
// 仅比对与 L3 相关的关键项（proto/ifname/ip/gw/ip6/ip6gw/mtu），
// 用以决定是否可跳过破坏性的 delete+recreate+reload，避免 netifd 接口抖动。
func (o *OpenWrtConfigurator) owConfigured(name string, p *owPending) bool {
	get := func(k string) string {
		out, err := runCmd("uci", "-q", "get", "network."+name+"."+k)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}
	if get("proto") != "static" {
		return false
	}
	if p.hasV4 {
		if get("ipaddr") != p.v4 ||
			get("netmask") != prefixToMask(p.v4Prefix) ||
			get("gateway") != p.gw {
			return false
		}
	}
	if p.hasV6 && p.v6 != "" {
		if get("ip6addr") != p.v6+"/"+strconv.Itoa(p.v6Prefix) {
			return false
		}
		// IPv6 不再写 ip6gw(用户要求); 委派前缀(PD)写入 ip6prefix 供 LAN 分发。
		if p.pdPrefix != "" {
			if get("ip6prefix") != p.pdPrefix+"/"+strconv.Itoa(p.pdPrefixLen) {
				return false
			}
		}
	}
	mtu := p.mtu
	if mtu <= 0 || mtu > 1440 {
		mtu = 1400
	}
	if get("mtu") != strconv.Itoa(mtu) {
		return false
	}
	return true
}

// addDefaultRouteWithMetric 在指定网口添加带 metric 的默认路由（replace 语义，避免重复）。
func addDefaultRouteWithMetric(ifname string, gw net.IP, metric int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ip", "route", "replace", "default", "via", gw.String(), "dev", ifname, "metric", strconv.Itoa(metric))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip route replace: %w: %s", err, string(out))
	}
	return nil
}

// applyKernelFallback ubus/uci 不可用时，直接走内核 netlink 配 IP/路由，保证至少内核可用。
// 注意：回退路径的 V4/V6 默认路由都必须带 owWANMetric()，避免与主 WAN 形成 ECMP，
// 也避免回退产生意料之外的默认 metric（如 5000）造成路由混乱。
func (o *OpenWrtConfigurator) applyKernelFallback(ifname string, p *owPending) error {
	if p.hasV4 {
		ip := net.ParseIP(p.v4)
		if err := o.LinuxConfigurator.SetIPAddress(ifname, ip, p.v4Prefix); err != nil {
			return err
		}
		if p.gw != "" {
			if gw := net.ParseIP(p.gw); gw != nil {
				if err := addDefaultRouteWithMetric(ifname, gw, owWANMetric()); err != nil {
					return err
				}
			}
		}
	}
	if p.hasV6 {
		ip6 := net.ParseIP(p.v6)

		// 与 applyNetifdStatic 一致的策略：
		// 若运营商分配的是 /64 前缀，且该前缀将被下发给 LAN(br-lan)，
		// 则将 WAN 侧地址收紧为 /128，避免同一个 /64 同时出现在 wwan0
		// 和 br-lan 导致回程路由冲突(LAN 客户端不可达)。
		addrLen := p.v6Prefix
		if p.v6Prefix == 64 {
			addrLen = 128
			// 将 /64 前缀路由指向 br-lan，使回程包正确送达 LAN 客户端。
			if err := o.ensureLanPrefixRoute(ip6); err != nil {
				log.Printf("[openwrt] 同步 LAN 前缀路由失败: %v", err)
			}
		}

		if err := o.LinuxConfigurator.SetIPv6Address(ifname, ip6, addrLen); err != nil {
			return err
		}
		if p.gw6 != "" {
			if gw6 := net.ParseIP(p.gw6); gw6 != nil {
				if err := addDefaultRoute6WithMetric(ifname, gw6, owWANMetric()); err != nil {
					return err
				}
			}
		} else if p.pdPrefix != "" || p.wantV6OnLink {
			// IPv6 无网关时走 on-link 默认路由 (default dev <iface>)。
			if err := addDefaultRouteV6OnLink(ifname, owWANMetric()); err != nil {
				return err
			}
		}
	}
	log.Printf("[openwrt] 已回退为内核直配 %s（绕开 netifd，metric=%d）", ifname, owWANMetric())
	return nil
}

// addDefaultRoute6WithMetric 在指定网口添加带 metric 的 IPv6 默认路由（replace 语义）。
func addDefaultRoute6WithMetric(ifname string, gw net.IP, metric int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ip", "-6", "route", "replace", "default", "via", gw.String(), "dev", ifname, "metric", strconv.Itoa(metric))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip -6 route replace: %w: %s", err, string(out))
	}
	return nil
}

// teardown 移除已注册的 netifd 静态接口（uci 块）并清理防火墙成员，再清内部状态。
func (o *OpenWrtConfigurator) teardown(ifname string) {
	o.mu.Lock()
	committed := o.committed[ifname]
	delete(o.committed, ifname)
	delete(o.pending, ifname)
	o.mu.Unlock()
	if !committed {
		return
	}
	name := o.netifdName(ifname)
	// 从防火墙 wan 区移除（避免悬空引用），再删 uci 接口并 reload。
	if err := o.setFirewallWan(name, false); err != nil {
		log.Printf("[openwrt] 移除防火墙 wan 区成员失败（可忽略）: %v", err)
	}
	runCmd("uci", "-q", "delete", "network."+name)
	runCmd("uci", "commit", "network")
	if out, err := runCmd("ubus", "call", "network", "reload"); err != nil {
		log.Printf("[openwrt] ubus network reload 失败（可忽略）: %v (%s)", err, out)
	}
	log.Printf("[openwrt] 已移除 netifd 静态接口 %s", name)
}

// ubusCall 调用 ubus 命令。method 形如 "network add_dynamic" 或
// "network.interface.xxx add_dns"，会被按首个空格拆成 object 与 method 两个参数；
// payload 序列化为 JSON 作为 ubus 消息体。
//
// 注意：ubus CLI 要求 `ubus call <object> <method> [<message>]`，object 与 method
// 必须分开传参，不能拼成 "network add_dynamic" 一个整体（否则会被当成不存在的 object）。
func ubusCall(method string, payload interface{}) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{"call"}
	if parts := strings.SplitN(method, " ", 2); len(parts) == 2 {
		args = append(args, parts[0], parts[1])
	} else {
		args = append(args, method)
	}
	args = append(args, string(b))
	cmd := exec.CommandContext(ctx, "ubus", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("ubus call %s 失败: %w: %s", method, err, string(out))
	}
	return out, nil
}

func prefixToMask(prefix int) string {
	if prefix <= 0 || prefix > 32 {
		return "255.255.255.255"
	}
	return net.IP(net.CIDRMask(prefix, 32)).String()
}

// runCmd 执行外部命令（uci/ifup/fw4 等），返回合并输出。
func runCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// waitInterfaceUp 轮询 netifd 接口状态，直到 up（返回 true）或超时（返回 false）。
func (o *OpenWrtConfigurator) waitInterfaceUp(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := ubusCall("network.interface."+name+" status", map[string]interface{}{})
		if err == nil && strings.Contains(string(out), "\"up\": true") {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("[openwrt] 等待接口 %s up 超时", name)
	return false
}

// waitInterfaceExists 轮询 netifd 是否已在 ubus 中创建出该接口（reload 为异步）。
func (o *OpenWrtConfigurator) waitInterfaceExists(name string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if out, err := runCmd("ubus", "list", "network.interface."+name); err == nil && strings.TrimSpace(out) != "" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("[openwrt] 等待接口 %s 在 netifd 中创建超时（ifup 可能失败）", name)
}

// wanZoneSection 找到防火墙配置里 name=wan 的 zone 段名（如 firewall.@zone[1]）。
func (o *OpenWrtConfigurator) wanZoneSection() (string, error) {
	out, err := runCmd("uci", "show", "firewall")
	if err != nil {
		return "", err
	}
	var zones []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasSuffix(line, "=zone") && strings.Contains(line, "@zone[") {
			zones = append(zones, line[:strings.Index(line, "=")])
		}
	}
	for _, z := range zones {
		nm, _ := runCmd("uci", "-q", "get", z+".name")
		if strings.TrimSpace(nm) == "wan" {
			return z, nil
		}
	}
	return "", fmt.Errorf("未找到 name=wan 的防火墙区域")
}

// setFirewallWan 将接口加入/移出防火墙 wan 区（幂等），变更后重载防火墙使 NAT 生效。
func (o *OpenWrtConfigurator) setFirewallWan(name string, add bool) error {
	sec, err := o.wanZoneSection()
	if err != nil {
		return err
	}
	cur, _ := runCmd("uci", "-q", "get", sec+".network")
	present := false
	for _, f := range strings.Fields(cur) {
		if f == name {
			present = true
			break
		}
	}
	if add == present {
		// 已处于目标状态，无需改动。
		return nil
	}
	if add {
		runCmd("uci", "add_list", sec+".network="+name)
	} else {
		runCmd("uci", "del_list", sec+".network="+name)
	}
	if out, err := runCmd("uci", "commit", "firewall"); err != nil {
		return fmt.Errorf("uci commit firewall 失败: %w: %s", err, out)
	}
	// 重载防火墙（fw4/nftables），失败不致命。
	if out, err := runCmd("fw4", "reload"); err != nil {
		log.Printf("[openwrt] fw4 reload 失败，尝试 service firewall reload: %v (%s)", err, out)
		runCmd("service", "firewall", "reload")
	}
	return nil
}
