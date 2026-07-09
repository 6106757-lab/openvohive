# openvohive

> 4G/5G QMI 数据连接管理器 — 专为 OpenWrt 路由器 + Quectel RM520N-GL 模组设计

[![Build](https://github.com/6106757-lab/openvohive/actions/workflows/build.yml/badge.svg)](https://github.com/6106757-lab/openvohive/actions/workflows/build.yml)
[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/License-PolyForm--Noncommercial--1.0.0-blue.svg)](LICENSE)

自动拨号、IPv4/IPv6 双栈、流量统计、策略路由、掉线自动恢复。所有网络配置逻辑编译进二进制，无需外部脚本。

## 硬件支持

| 模组 | 接口 | 状态 |
|------|------|------|
| Quectel RM520N-GL | QMI (cdc-wdm0 / wwan0) | ✅ 完整支持 |
| 其他 QMI 模组 | QMI | ⚠️ 理论兼容，未测试 |

## 核心特性

| 功能 | 说明 |
|------|------|
| 自动拨号 | 开机自启，QMI 自动建立数据连接，无需手动操作 |
| IPv4/IPv6 双栈 | 同时获取 IPv4 和 IPv6 地址 |
| LAN IPv6 分配 | 自动将 wwan0 的 IPv6 /64 前缀分配给 br-lan，odhcpd RA 下发 |
| 策略路由 | 自动配置 fwmark + from-IP 策略路由，多出口不冲突 |
| NAT 穿透 | 自动配置 iptables MASQUERADE，LAN 设备透明上网 |
| 掉线恢复 | QMI 连接断开自动重连，IPv6 拨号失败自动重试 |
| 流量统计 | 每分钟采集，支持分钟/小时/天粒度聚合，Web 界面可视化 |
| Web 管理 | Vue 3 管理界面，实时状态、短信、流量分析、设备配置 |
| 短信管理 | QMI 短信收发，支持联系人管理和会话视图 |
| 多架构 | 支持 amd64 / arm64 / armv7，CI 自动构建 |

## 一键安装

在 OpenWrt 路由器上执行：

```bash
curl -sSL https://raw.githubusercontent.com/6106757-lab/openvohive/master/install.sh | sh
```

脚本会自动：
1. 检测 CPU 架构 (amd64 / arm64 / armv7)
2. 下载对应二进制到 `/opt/openvohive/`
3. 创建默认配置文件
4. 安装 `/etc/init.d/openvohive` 并启用开机自启
5. 启动服务

安装完成后访问 `http://<路由器IP>:7575` 进入 Web 管理界面。

> **默认账号密码**：`admin` / `V0h!ve@2025rt`，请安装后立即修改。

## 手动编译

### 前置条件

- Go 1.23+
- Node.js 18+ (用于构建 Web 前端)
- pnpm 或 npm

### 编译步骤

```bash
# 1. 克隆仓库
git clone https://github.com/6106757-lab/openvohive.git
cd openvohive

# 2. 构建 Web 前端
cd web
pnpm install        # 或 npm install
npx vite build
cd ..

# 3. 编译后端（静态链接，兼容 musl libc）
CGO_ENABLED=0 go build -o openvohive -ldflags="-s -w" .

# 4. 查看编译产物
file openvohive
# openvohive: ELF 64-bit LSB executable, statically linked, stripped
```

### 交叉编译

```bash
# amd64 (x86_64 软路由)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o openvohive-linux-amd64 -ldflags="-s -w" .

# arm64 (ARM 软路由 / 树莓派 4)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o openvohive-linux-arm64 -ldflags="-s -w" .

# armv7 (ARMv7 路由器)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o openvohive-linux-armv7 -ldflags="-s -w" .
```

> **重要**：必须设置 `CGO_ENABLED=0`，OpenWrt 使用 musl libc，glibc 动态链接的二进制无法运行。

## 部署到 OpenWrt

### 手动部署

```bash
# 1. 上传二进制到路由器
scp -P 22 openvohive-linux-amd64 root@192.168.1.1:/opt/openvohive/openvohive

# 2. 创建配置
ssh root@192.168.1.1 "mkdir -p /opt/openvohive/config"
scp config/config.example.yaml root@192.168.1.1:/opt/openvohive/config/config.yaml

# 3. 编辑配置，填入 modem_imei
ssh root@192.168.1.1 "vi /opt/openvohive/config/config.yaml"

# 4. 安装 init.d 服务（内容见 install.sh）
# 5. 启动
ssh root@192.168.1.1 "/etc/init.d/openvohive start"
```

### 目录结构

```
/opt/openvohive/
├── openvohive          # 主二进制
├── config/
│   └── config.yaml     # 配置文件
├── data/
│   └── vohive.db       # SQLite 数据库（自动创建）
└── logs/               # 日志目录
```

## 配置说明

```yaml
server:
    port: 7575           # Web 管理端口
    debug: false         # 调试模式（生产环境关闭）

web:
    username: admin      # 登录用户名
    password: "xxx"      # 登录密码

devices:
    - id: rm520n-1       # 设备标识
      name: RM520N-GL    # 显示名称
      device_backend: qmi
      modem_imei: "863004060062519"  # 模组 IMEI（重要！）
```

## 架构

```
┌─────────────────────────────────────────────┐
│                 openvohive                   │
├─────────────────────────────────────────────┤
│  Web UI (Vue 3)  │  API Server (Gin)        │
├──────────────────┼──────────────────────────┤
│  Device Pool     │  QMI Backend             │
│  - Bootstrap     │  - WDS (数据连接)         │
│  - Health Check  │  - NAS (网络注册/信号)     │
│  - Traffic       │  - WMS (短信)             │
│  - Policy        │  - DMS (设备管理)          │
├──────────────────┼──────────────────────────┤
│  Netcfg Daemon   │  内置网络配置守护进程      │
│  - 策略路由       │  - NAT 穿透              │
│  - LAN IPv6 分配  │  - odhcpd RA 管理        │
├──────────────────┴──────────────────────────┤
│              SQLite (vohive.db)              │
└─────────────────────────────────────────────┘
```

## CI/CD

推送代码到 master 分支自动触发构建，生成三个架构的二进制产物。

打 tag (`v1.0.0` 等) 自动创建 GitHub Release 并附带二进制文件。

## 免责声明

- 本项目 fork 自 [iniwex5/vohive](https://github.com/iniwex5/vohive)，原始版权归原作者所有
- 仅供个人学习、研究与测试，不建议直接用于生产环境
- 使用本项目请遵守当地法律法规及运营商服务条款

## License

[PolyForm Noncommercial License 1.0.0](LICENSE) — 仅限非商业用途。
