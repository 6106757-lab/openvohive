#
# Copyright (C) 2024 OpenVoHive
#
# This is free software, licensed under the Apache License 2.0.
#

include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-openvohive
PKG_VERSION:=2.1.1
PKG_RELEASE:=1

PKG_MAINTAINER:=OpenVoHive <koudejun@live.com>
PKG_LICENSE:=Apache-2.0

include $(INCLUDE_DIR)/package.mk

define Package/luci-app-openvohive
  SECTION:=luci
  CATEGORY:=LuCI
  SUBMENU:=3. Applications
  TITLE:=LuCI support for Open-VoHive
  PKGARCH:=all
  DEPENDS:=+curl +coreutils +coreutils-stat +coreutils-mkdir
endef

define Package/luci-app-openvohive/description
  LuCI web interface for Open-VoHive 4G/5G modem manager.
  Provides service control, core update, status monitoring and log viewer.
endef

define Build/Prepare
endef

define Build/Compile
endef

define Package/luci-app-openvohive/conffiles
/etc/config/openvohive
endef

define Package/luci-app-openvohive/install
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./root/etc/config/openvohive $(1)/etc/config/openvohive

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./root/etc/init.d/openvohive $(1)/etc/init.d/openvohive

	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d
	$(INSTALL_DATA) ./root/usr/share/luci/menu.d/luci-app-openvohive.json $(1)/usr/share/luci/menu.d/luci-app-openvohive.json

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./root/usr/share/rpcd/acl.d/luci-app-openvohive.json $(1)/usr/share/rpcd/acl.d/luci-app-openvohive.json

	$(INSTALL_DIR) $(1)/usr/share/openvohive
	$(INSTALL_BIN) ./root/usr/share/openvohive/*.sh $(1)/usr/share/openvohive/

	$(INSTALL_DIR) $(1)/www/luci-static/resources/view/openvohive
	$(INSTALL_DATA) ./root/www/luci-static/resources/view/openvohive/index.js $(1)/www/luci-static/resources/view/openvohive/index.js

	$(INSTALL_DIR) $(1)/opt/openvohive/config
	$(INSTALL_DATA) ./root/opt/openvohive/config/config.yaml $(1)/opt/openvohive/config/config.yaml
endef

define Package/luci-app-openvohive/postinst
#!/bin/sh
if [ -z "$${IPKG_INSTROOT}" ]; then
	# 创建必要目录
	mkdir -p /opt/openvohive/data /opt/openvohive/logs /opt/openvohive/bin /tmp/openvohive/tasks
	# 启用服务
	/etc/init.d/openvohive enable 2>/dev/null || true
fi
exit 0
endef

define Package/luci-app-openvohive/prerm
#!/bin/sh
if [ -z "$${IPKG_INSTROOT}" ]; then
	/etc/init.d/openvohive stop 2>/dev/null || true
	/etc/init.d/openvohive disable 2>/dev/null || true
	killall -9 openvohive 2>/dev/null || true
fi
exit 0
endef

$(eval $(call BuildPackage,luci-app-openvohive))
