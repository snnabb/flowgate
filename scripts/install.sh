#!/bin/bash
# FlowGate Panel 一键安装脚本
# 用法: bash install.sh [--port 8080] [--uninstall]

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

INSTALL_DIR="/opt/flowgate"
SERVICE_NAME="flowgate-panel"
BINARY_NAME="flowgate"
PANEL_PORT=8080
DB_PATH="${INSTALL_DIR}/flowgate.db"
GITHUB_REPO="flowgate/flowgate"

print_banner() {
    echo -e "${CYAN}"
    echo "  ⚡ FlowGate - 轻量级端口转发面板"
    echo "  ────────────────────────────────"
    echo -e "${NC}"
}

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --port) PANEL_PORT="$2"; shift ;;
        --uninstall) UNINSTALL=true ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
    shift
done

uninstall() {
    echo -e "${YELLOW}正在卸载 FlowGate Panel...${NC}"
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    systemctl disable ${SERVICE_NAME} 2>/dev/null || true
    rm -f /etc/systemd/system/${SERVICE_NAME}.service
    systemctl daemon-reload
    rm -rf ${INSTALL_DIR}
    echo -e "${GREEN}✓ FlowGate Panel 已成功卸载${NC}"
    exit 0
}

if [ "$UNINSTALL" = true ]; then
    uninstall
fi

print_banner

# Check root
if [ "$(id -u)" -ne 0 ]; then
    echo -e "${RED}请使用 root 权限运行此脚本${NC}"
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case ${ARCH} in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l)  ARCH="arm" ;;
    *) echo -e "${RED}不支持的架构: ${ARCH}${NC}"; exit 1 ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')

echo -e "${GREEN}系统信息: ${OS}/${ARCH}${NC}"
echo -e "${GREEN}安装目录: ${INSTALL_DIR}${NC}"
echo -e "${GREEN}面板端口: ${PANEL_PORT}${NC}"
echo ""

# Create install directory
mkdir -p ${INSTALL_DIR}

# Check if binary exists (for local build)
if [ -f "./${BINARY_NAME}" ]; then
    if [ "$(realpath ./${BINARY_NAME})" != "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        echo -e "${CYAN}使用本地编译的二进制文件...${NC}"
        cp ./${BINARY_NAME} ${INSTALL_DIR}/${BINARY_NAME}
    else
        echo -e "${CYAN}在安装目录中检测到二进制文件...${NC}"
    fi
else
    echo -e "${CYAN}请将编译好的 flowgate 二进制文件放到 ${INSTALL_DIR}/ 目录${NC}"
    echo ""
    echo "编译命令 (在开发机上):"
    echo -e "  ${YELLOW}CGO_ENABLED=1 GOOS=linux GOARCH=${ARCH} go build -o flowgate -ldflags '-s -w' ./cmd/flowgate/${NC}"
    echo ""
    echo "然后上传到此服务器:"
    echo -e "  ${YELLOW}scp flowgate root@<server-ip>:${INSTALL_DIR}/${NC}"
    echo ""

    if [ ! -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        echo -e "${RED}未找到二进制文件，请先编译并上传${NC}"
        exit 1
    fi
fi

chmod +x ${INSTALL_DIR}/${BINARY_NAME}

# Generate JWT secret
JWT_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '=+/' | head -c 32)

# Create systemd service
cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=FlowGate Panel
After=network.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${BINARY_NAME} panel --port ${PANEL_PORT} --db ${DB_PATH} --secret ${JWT_SECRET}
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# Reload and start
systemctl daemon-reload
systemctl enable ${SERVICE_NAME}
systemctl restart ${SERVICE_NAME}

echo ""
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}  ✓ FlowGate Panel 安装成功!${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo ""
echo -e "  访问面板: ${CYAN}http://$(hostname -I | awk '{print $1}'):${PANEL_PORT}${NC}"
echo -e "  首次访问请注册管理员账号"
echo ""
echo -e "  管理命令:"
echo -e "    ${YELLOW}systemctl status ${SERVICE_NAME}${NC}   # 查看状态"
echo -e "    ${YELLOW}systemctl restart ${SERVICE_NAME}${NC}  # 重启面板"
echo -e "    ${YELLOW}systemctl stop ${SERVICE_NAME}${NC}     # 停止面板"
echo -e "    ${YELLOW}journalctl -u ${SERVICE_NAME} -f${NC}   # 查看日志"
echo ""
echo -e "  卸载: ${RED}bash install.sh --uninstall${NC}"
echo ""
