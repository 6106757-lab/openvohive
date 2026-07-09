//go:build linux
// +build linux

package netcfg

// GetPlatformConfigurator returns the platform configurator
// GetPlatformConfigurator 返回平台配置器
func GetPlatformConfigurator() NetworkConfigurator {
	if IsOpenWrt() {
		return NewOpenWrtConfigurator()
	}
	return NewLinuxConfigurator()
}
