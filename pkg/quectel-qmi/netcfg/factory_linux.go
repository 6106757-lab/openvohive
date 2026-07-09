//go:build linux
// +build linux

package netcfg

// GetPlatformConfigurator returns the platform configurator
// GetPlatformConfigurator 返回平台配置器
func GetPlatformConfigurator() NetworkConfigurator {
	// 临时修复：禁用 OpenWrtConfigurator，直接使用 LinuxConfigurator（netlink 直配）。
	// OpenWrtConfigurator 通过 netifd 接管 wwan0 后会导致 QMI 数据平面不通（tx=0）。
	// 待排查 netifd proto=static 与 qmi_wwan raw-ip 模式的兼容性问题后再恢复。
	//
	// 原逻辑：
	//   if IsOpenWrt() { return NewOpenWrtConfigurator() }
	return NewLinuxConfigurator()
}
