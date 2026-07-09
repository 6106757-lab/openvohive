#!/bin/bash
# ============================================================
# OpenVoHive IPK/APK 打包脚本
# 为 amd64 / arm64 / armv7 构建 LuCI 前端包和核心包
# ============================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT_DIR="$REPO_ROOT/dist"
PKG_VERSION="${PKG_VERSION:-2.0.1}"
PKG_RELEASE="${PKG_RELEASE:-1}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# 清理
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# ============================================================
# 1. 构建 luci-app-openvohive (noarch，通用)
# ============================================================
build_luci_ipk() {
    log "构建 luci-app-openvohive IPK (noarch)..."

    local LUCI_DIR="$REPO_ROOT/luci-app-openvohive"
    local PKG="luci-app-openvohive"
    local CONTROL_DIR="$OUTPUT_DIR/${PKG}_${PKG_VERSION}-${PKG_RELEASE}_all"

    mkdir -p "$CONTROL_DIR/control"

    # control 文件
    cat > "$CONTROL_DIR/control/control" <<EOF
Package: $PKG
Version: ${PKG_VERSION}-${PKG_RELEASE}
Architecture: all
Section: luci
Priority: optional
Maintainer: OpenVoHive <koudejun@live.com>
Description: LuCI support for Open-VoHive 4G/5G modem manager
Depends: curl, coreutils, coreutils-stat, coreutils-mkdir
Source: https://github.com/6106757-lab/openvohive
License: Apache-2.0
EOF

    # postinst
    cat > "$CONTROL_DIR/control/postinst" <<'SCRIPT'
#!/bin/sh
if [ -z "${IPKG_INSTROOT}" ]; then
	mkdir -p /opt/openvohive/data /opt/openvohive/logs /opt/openvohive/bin /tmp/openvohive/tasks
	/etc/init.d/openvohive enable 2>/dev/null || true
fi
exit 0
SCRIPT
    chmod 0755 "$CONTROL_DIR/control/postinst"

    # prerm
    cat > "$CONTROL_DIR/control/prerm" <<'SCRIPT'
#!/bin/sh
if [ -z "${IPKG_INSTROOT}" ]; then
	/etc/init.d/openvohive stop 2>/dev/null || true
	/etc/init.d/openvohive disable 2>/dev/null || true
	killall -9 openvohive 2>/dev/null || true
fi
exit 0
SCRIPT
    chmod 0755 "$CONTROL_DIR/control/prerm"

    # 复制文件
    cp -a "$LUCI_DIR/root/." "$CONTROL_DIR/"

    # 打包 IPK (tar.gz 格式)
    cd "$OUTPUT_DIR"
    local IPK_NAME="${PKG}_${PKG_VERSION}-${PKG_RELEASE}_all.ipk"

    # IPK 格式: debian-binary + control.tar.gz + data.tar.gz
    echo "2.0" > "$CONTROL_DIR/debian-binary"

    cd "$CONTROL_DIR/control"
    tar czf "$OUTPUT_DIR/${IPK_NAME}.control.tar.gz" ./*
    cd "$CONTROL_DIR"
    rm -rf control
    tar czf "$OUTPUT_DIR/${IPK_NAME}.data.tar.gz" --exclude=debian-binary ./*
    rm -f debian-binary

    # 最终组合
    cd "$OUTPUT_DIR"
    tar czf "$IPK_NAME" \
        --owner=0 --group=0 \
        ./${PKG}_*/debian-binary \
        ./${PKG}_*/control.tar.gz \
        ./${PKG}_*/data.tar.gz 2>/dev/null || {
        # 备选方案
        echo "2.0" > debian-binary
        cp "${IPK_NAME}.control.tar.gz" control.tar.gz
        cp "${IPK_NAME}.data.tar.gz" data.tar.gz
        tar czf "$IPK_NAME" debian-binary control.tar.gz data.tar.gz
        rm -f debian-binary control.tar.gz data.tar.gz
    }

    rm -rf "${CONTROL_DIR}" "${IPK_NAME}.control.tar.gz" "${IPK_NAME}.data.tar.gz"
    log "IPK 已生成: $OUTPUT_DIR/$IPK_NAME ($(du -h "$OUTPUT_DIR/$IPK_NAME" | cut -f1))"
}

# ============================================================
# 2. 构建 openvohive 核心 IPK (按架构)
# ============================================================
build_core_ipk() {
    local ARCH="$1"      # amd64 / arm64 / armv7
    local GOARCH="$2"    # amd64 / arm64 / arm
    local GOARM="$3"     # "" / "" / "7"
    local BINARY="$4"    # 预编译好的二进制路径

    log "构建 openvohive IPK ($ARCH)..."

    local PKG="openvohive"
    local CONTROL_DIR="$OUTPUT_DIR/${PKG}_${PKG_VERSION}-${PKG_RELEASE}_${ARCH}"

    mkdir -p "$CONTROL_DIR/control"
    mkdir -p "$CONTROL_DIR/opt/openvohive"
    mkdir -p "$CONTROL_DIR/opt/openvohive/config"
    mkdir -p "$CONTROL_DIR/opt/openvohive/data"
    mkdir -p "$CONTROL_DIR/opt/openvohive/logs"
    mkdir -p "$CONTROL_DIR/opt/openvohive/bin"

    # control 文件
    cat > "$CONTROL_DIR/control/control" <<EOF
Package: $PKG
Version: ${PKG_VERSION}-${PKG_RELEASE}
Architecture: $ARCH
Section: net
Priority: optional
Maintainer: OpenVoHive <koudejun@live.com>
Description: Open-VoHive 4G/5G Modem Manager Core
Depends: libstdcpp
Source: https://github.com/6106757-lab/openvohive
License: Apache-2.0
EOF

    # postinst
    cat > "$CONTROL_DIR/control/postinst" <<SCRIPT
#!/bin/sh
if [ -z "\${IPKG_INSTROOT}" ]; then
	echo "v${PKG_VERSION}" > /opt/openvohive/bin/version
	echo "${ARCH}" > /opt/openvohive/bin/arch
fi
exit 0
SCRIPT
    chmod 0755 "$CONTROL_DIR/control/postinst"

    # 复制二进制
    cp "$BINARY" "$CONTROL_DIR/opt/openvohive/openvohive"
    chmod 0755 "$CONTROL_DIR/opt/openvohive/openvohive"

    # 复制配置模板
    cp "$REPO_ROOT/config/config.yaml" "$CONTROL_DIR/opt/openvohive/config/config.yaml"

    # 打包
    cd "$OUTPUT_DIR"
    local IPK_NAME="${PKG}_${PKG_VERSION}-${PKG_RELEASE}_${ARCH}.ipk"

    echo "2.0" > "$CONTROL_DIR/debian-binary"

    cd "$CONTROL_DIR/control"
    tar czf "$OUTPUT_DIR/${IPK_NAME}.control.tar.gz" ./*
    cd "$CONTROL_DIR"
    rm -rf control
    tar czf "$OUTPUT_DIR/${IPK_NAME}.data.tar.gz" --exclude=debian-binary ./*
    rm -f debian-binary

    cd "$OUTPUT_DIR"
    tar czf "$IPK_NAME" \
        --owner=0 --group=0 \
        ./${PKG}_*/debian-binary \
        ./${PKG}_*/control.tar.gz \
        ./${PKG}_*/data.tar.gz 2>/dev/null || {
        echo "2.0" > debian-binary
        cp "${IPK_NAME}.control.tar.gz" control.tar.gz
        cp "${IPK_NAME}.data.tar.gz" data.tar.gz
        tar czf "$IPK_NAME" debian-binary control.tar.gz data.tar.gz
        rm -f debian-binary control.tar.gz data.tar.gz
    }

    rm -rf "${CONTROL_DIR}" "${IPK_NAME}.control.tar.gz" "${IPK_NAME}.data.tar.gz"
    log "IPK 已生成: $OUTPUT_DIR/$IPK_NAME ($(du -h "$OUTPUT_DIR/$IPK_NAME" | cut -f1))"
}

# ============================================================
# 3. 构建 APK 包 (新 OpenWrt 包管理格式)
# ============================================================
build_luci_apk() {
    log "构建 luci-app-openvohive APK (noarch)..."

    local LUCI_DIR="$REPO_ROOT/luci-app-openvohive"
    local PKG="luci-app-openvohive"
    local PKG_DIR="$OUTPUT_DIR/${PKG}_${PKG_VERSION}-${PKG_RELEASE}_all"

    mkdir -p "$PKG_DIR"

    # APK .pkginfo 文件
    cat > "$PKG_DIR/.pkginfo" <<EOF
name = $PKG
version = ${PKG_VERSION}-${PKG_RELEASE}
arch = all
description = LuCI support for Open-VoHive 4G/5G modem manager
maintainer = OpenVoHive <koudejun@live.com>
license = Apache-2.0
depends = curl coreutils coreutils-stat coreutils-mkdir
EOF

    # 复制文件树
    cp -a "$LUCI_DIR/root/." "$PKG_DIR/"
    rm -f "$PKG_DIR/.pkginfo"  # 确保 .pkginfo 不在 root 里

    # 重新写 .pkginfo
    cat > "$PKG_DIR/.pkginfo" <<EOF
name = $PKG
version = ${PKG_VERSION}-${PKG_RELEASE}
arch = all
description = LuCI support for Open-VoHive 4G/5G modem manager
maintainer = OpenVoHive <koudejun@live.com>
license = Apache-2.0
depends = curl coreutils coreutils-stat coreutils-mkdir
EOF

    # 打包为 .apk (tar.gz)
    local APK_NAME="${PKG}_${PKG_VERSION}-${PKG_RELEASE}_all.apk"
    cd "$PKG_DIR"
    tar czf "$OUTPUT_DIR/$APK_NAME" --owner=0 --group=0 ./
    cd "$OUTPUT_DIR"
    rm -rf "$PKG_DIR"

    log "APK 已生成: $OUTPUT_DIR/$APK_NAME ($(du -h "$OUTPUT_DIR/$APK_NAME" | cut -f1))"
}

build_core_apk() {
    local ARCH="$1"
    local BINARY="$2"

    log "构建 openvohive APK ($ARCH)..."

    local PKG="openvohive"
    local PKG_DIR="$OUTPUT_DIR/${PKG}_${PKG_VERSION}-${PKG_RELEASE}_${ARCH}"

    mkdir -p "$PKG_DIR/opt/openvohive"
    mkdir -p "$PKG_DIR/opt/openvohive/config"
    mkdir -p "$PKG_DIR/opt/openvohive/data"
    mkdir -p "$PKG_DIR/opt/openvohive/logs"
    mkdir -p "$PKG_DIR/opt/openvohive/bin"

    cp "$BINARY" "$PKG_DIR/opt/openvohive/openvohive"
    chmod 0755 "$PKG_DIR/opt/openvohive/openvohive"
    cp "$REPO_ROOT/config/config.yaml" "$PKG_DIR/opt/openvohive/config/config.yaml"

    cat > "$PKG_DIR/.pkginfo" <<EOF
name = $PKG
version = ${PKG_VERSION}-${PKG_RELEASE}
arch = $ARCH
description = Open-VoHive 4G/5G Modem Manager Core
maintainer = OpenVoHive <koudejun@live.com>
license = Apache-2.0
depends = libstdcpp
EOF

    local APK_NAME="${PKG}_${PKG_VERSION}-${PKG_RELEASE}_${ARCH}.apk"
    cd "$PKG_DIR"
    tar czf "$OUTPUT_DIR/$APK_NAME" --owner=0 --group=0 ./
    cd "$OUTPUT_DIR"
    rm -rf "$PKG_DIR"

    log "APK 已生成: $OUTPUT_DIR/$APK_NAME ($(du -h "$OUTPUT_DIR/$APK_NAME" | cut -f1))"
}

# ============================================================
# 主流程
# ============================================================
main() {
    local MODE="${1:-all}"

    echo ""
    echo "=============================================="
    echo "  OpenVoHive IPK/APK Builder v${PKG_VERSION}"
    echo "  Mode: $MODE"
    echo "=============================================="
    echo ""

    case "$MODE" in
        luci-only)
            build_luci_ipk
            build_luci_apk
            ;;
        core-only)
            # 只构建当前架构的核心包 (CI 中每个矩阵 job 调用一次)
            # 自动检测当前目录下的二进制
            local BINARY_DIR="$REPO_ROOT/dist/binaries"
            for f in "$BINARY_DIR"/openvohive-linux-* "$REPO_ROOT"/openvohive-linux-*; do
                if [ -f "$f" ]; then
                    local fname=$(basename "$f")
                    local arch="${fname#openvohive-linux-}"
                    local goarch="$arch"
                    [ "$arch" = "armv7" ] && goarch="arm"
                    log "检测到二进制: $fname -> arch=$arch"
                    build_core_ipk "$arch" "$goarch" "" "$f"
                    build_core_apk "$arch" "$f"
                fi
            done
            ;;
        all|*)
            # --- LuCI 前端包 (通用) ---
            build_luci_ipk
            build_luci_apk

            # --- 核心包 (按架构) ---
            local ARCHES=("amd64" "arm64" "armv7")
            local GOARCHES=("amd64" "arm64" "arm")
            local BINARY_DIR="$REPO_ROOT/dist/binaries"

            for i in "${!ARCHES[@]}"; do
                local arch="${ARCHES[$i]}"
                local goarch="${GOARCHES[$i]}"
                local binary_name="openvohive-linux-${arch}"
                local binary_path=""

                if [ -f "$BINARY_DIR/$binary_name" ]; then
                    binary_path="$BINARY_DIR/$binary_name"
                elif [ -f "$REPO_ROOT/$binary_name" ]; then
                    binary_path="$REPO_ROOT/$binary_name"
                else
                    warn "未找到 $binary_name，跳过核心包构建"
                    continue
                fi

                build_core_ipk "$arch" "$goarch" "" "$binary_path"
                build_core_apk "$arch" "$binary_path"
            done
            ;;
    esac

    echo ""
    log "=============================================="
    log "  所有包已生成到: $OUTPUT_DIR"
    ls -lh "$OUTPUT_DIR"/*.ipk "$OUTPUT_DIR"/*.apk 2>/dev/null || true
    log "=============================================="
}

main "$@"
