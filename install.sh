#!/bin/sh
# ============================================================
# openvohive 一键安装脚本
# 自动检测架构、下载对应二进制、部署到 OpenWrt/Linux 路由器
# ============================================================
set -e

REPO="6106757-lab/openvohive"
INSTALL_DIR="/opt/openvohive"
CONFIG_DIR="$INSTALL_DIR/config"
DATA_DIR="$INSTALL_DIR/data"
INIT_SCRIPT="/etc/init.d/openvohive"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
err()  { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# ---------- 检测架构 ----------
detect_arch() {
    MACHINE=$(uname -m)
    case "$MACHINE" in
        x86_64)      echo "amd64" ;;
        aarch64)     echo "arm64" ;;
        armv7l|armv6l) echo "armv7" ;;
        *)           err "不支持的架构: $MACHINE" ;;
    esac
}

# ---------- 获取最新版本 ----------
get_latest_version() {
    LATEST=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": "\(.*\)".*/\1/')
    if [ -z "$LATEST" ]; then
        # fallback: 从 master 分支构建的 artifact 下载
        LATEST="latest"
        warn "未找到 release 版本，将下载最新构建版本"
    fi
    echo "$LATEST"
}

# ---------- 下载二进制 ----------
download_binary() {
    ARCH="$1"
    VERSION="$2"
    BINARY_NAME="openvohive-linux-$ARCH"

    if [ "$VERSION" = "latest" ]; then
        # 从 GitHub Actions 最新成功的 workflow run 下载
        log "从 GitHub Actions 下载最新构建..."
        RUN_ID=$(curl -s "https://api.github.com/repos/$REPO/actions/runs?status=success&per_page=1" | grep '"id"' | head -1 | sed 's/[^0-9]//g')
        if [ -z "$RUN_ID" ]; then
            err "无法获取最新构建，请手动下载或使用 release 版本"
        fi
        curl -s -L "https://api.github.com/repos/$REPO/actions/runs/$RUN_ID/artifacts" > /tmp/artifacts.json
        ARTIFACT_URL=$(grep -o "\"archive_download_url\":\"[^\"]*$BINARY_NAME[^\"]*\"" /tmp/artifacts.json | head -1 | sed 's/"archive_download_url":"\([^"]*\)"/\1/')
        if [ -z "$ARTIFACT_URL" ]; then
            err "未找到 $BINARY_NAME 的构建产物"
        fi
        log "下载 $BINARY_NAME ..."
        curl -s -L -H "Authorization: Bearer ${GITHUB_TOKEN:-}" -o "/tmp/$BINARY_NAME.zip" "$ARTIFACT_URL" || \
            err "下载失败，请设置 GITHUB_TOKEN 或手动下载"
        unzip -o "/tmp/$BINARY_NAME.zip" -d /tmp/
        mv "/tmp/$BINARY_NAME" "$INSTALL_DIR/openvohive"
    else
        URL="https://github.com/$REPO/releases/download/$VERSION/$BINARY_NAME"
        log "下载 $URL ..."
        curl -s -L -o "$INSTALL_DIR/openvohive" "$URL" || err "下载失败: $URL"
    fi
    chmod +x "$INSTALL_DIR/openvohive"
    log "二进制已安装到 $INSTALL_DIR/openvohive"
}

# ---------- 创建默认配置 ----------
create_config() {
    if [ -f "$CONFIG_DIR/config.yaml" ]; then
        warn "配置文件已存在，跳过创建: $CONFIG_DIR/config.yaml"
        return
    fi
    mkdir -p "$CONFIG_DIR"
    cat > "$CONFIG_DIR/config.yaml" << 'EOF'
server:
    port: 7575
    debug: false
web:
    username: admin
    password: "V0h!ve@2025rt"
devices:
    - id: rm520n-1
      name: RM520N-GL QMI Cellular
      device_backend: qmi
      modem_imei: ""
EOF
    log "默认配置已创建: $CONFIG_DIR/config.yaml"
    warn "请编辑配置文件，填入正确的 modem_imei"
}

# ---------- 安装 init.d 服务 ----------
install_init() {
    if [ -f "$INIT_SCRIPT" ]; then
        warn "init.d 脚本已存在，覆盖更新"
    fi
    cat > "$INIT_SCRIPT" << 'SCRIPTEOF'
#!/bin/sh /etc/rc.common

START=95
STOP=10

PROG=/opt/openvohive/openvohive
CONFIG=/opt/openvohive/config/config.yaml
PIDFILE=/var/run/openvohive.pid

boot() { start; }

start() {
    if [ -f $PIDFILE ]; then
        OPID=$(cat $PIDFILE)
        kill $OPID 2>/dev/null
        sleep 1
        kill -9 $OPID 2>/dev/null
        rm -f $PIDFILE
    fi
    if netstat -tlnp 2>/dev/null | grep -q ':7575 '; then
        PORT_PID=$(netstat -tlnp 2>/dev/null | grep ':7575 ' | head -1 | awk '{print $NF}' | cut -d/ -f1)
        [ -n "$PORT_PID" ] && kill -9 $PORT_PID 2>/dev/null
        sleep 1
    fi

    modprobe qmi_wwan 2>/dev/null
    for i in $(seq 1 30); do
        [ -c /dev/cdc-wdm0 ] && break
        sleep 1
    done

    $PROG -c "$CONFIG" > /dev/null 2>&1 &
    echo $! > $PIDFILE
}

stop() {
    if [ -f $PIDFILE ]; then
        OPID=$(cat $PIDFILE)
        kill $OPID 2>/dev/null
        sleep 1
        kill -9 $OPID 2>/dev/null
        rm -f $PIDFILE
    fi
}

restart() { stop; sleep 2; start; }
SCRIPTEOF
    chmod +x "$INIT_SCRIPT"

    # 启用开机自启
    if [ -x /etc/rc.common ]; then
        "$INIT_SCRIPT" enable 2>/dev/null || \
            ln -sf "$INIT_SCRIPT" /etc/rc.d/S95openvohive 2>/dev/null
        log "已启用开机自启"
    fi
    log "init.d 脚本已安装: $INIT_SCRIPT"
}

# ---------- 主流程 ----------
main() {
    echo "============================================"
    echo "  openvohive 一键安装脚本"
    echo "============================================"
    echo ""

    ARCH=$(detect_arch)
    log "检测到架构: $ARCH"

    VERSION=$(get_latest_version)
    log "版本: $VERSION"

    # 创建目录
    mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR"

    # 停止旧服务
    if [ -f "$INIT_SCRIPT" ]; then
        "$INIT_SCRIPT" stop 2>/dev/null || true
        sleep 1
    fi

    # 下载/安装
    download_binary "$ARCH" "$VERSION"

    # 配置
    create_config

    # init.d
    install_init

    # 启动
    log "启动 openvohive..."
    "$INIT_SCRIPT" start
    sleep 5

    # 验证
    if ps | grep -q "[o]penvohive"; then
        log "openvohive 启动成功!"
        log "Web 管理界面: http://$(ip -4 addr show br-lan 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 | head -1):7575"
    else
        warn "启动可能失败，请检查日志: /root/logs/"
    fi

    echo ""
    log "安装完成!"
}

main "$@"
