#!/bin/bash
# FlowGate Node 一键安装脚本
# 用法: bash install_node.sh --panel ws://panel-ip:8080/ws/node --key <API_KEY> [--uninstall]

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

INSTALL_DIR="/opt/flowgate"
SERVICE_NAME="flowgate-node"
BINARY_NAME="flowgate"

PANEL_URL=""
API_KEY=""

print_banner() {
    echo -e "${CYAN}"
    echo "  ⚡ FlowGate Node - 转发节点安装"
    echo "  ────────────────────────────────"
    echo -e "${NC}"
}

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --panel) PANEL_URL="$2"; shift ;;
        --key) API_KEY="$2"; shift ;;
        --uninstall) UNINSTALL=true ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
    shift
done

uninstall() {
    echo -e "${YELLOW}正在卸载 FlowGate Node...${NC}"
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    systemctl disable ${SERVICE_NAME} 2>/dev/null || true
    rm -f /etc/systemd/system/${SERVICE_NAME}.service
    systemctl daemon-reload
    rm -rf ${INSTALL_DIR}
    echo -e "${GREEN}✓ FlowGate Node 已成功卸载${NC}"
    exit 0
}

if [ "$UNINSTALL" = true ]; then
    uninstall
fi

print_banner

# Validate params
if [ -z "$PANEL_URL" ] || [ -z "$API_KEY" ]; then
    echo -e "${RED}错误: 必须提供 --panel 和 --key 参数${NC}"
    echo ""
    echo "用法:"
    echo "  bash install_node.sh --panel ws://panel-ip:8080/ws/node --key YOUR_API_KEY"
    echo ""
    echo "  --panel  面板WebSocket地址"
    echo "  --key    节点API密钥 (在面板中创建节点后获取)"
    exit 1
fi

# Check root
if [ "$(id -u)" -ne 0 ]; then
    echo -e "${RED}请使用 root 权限运行此脚本${NC}"
    exit 1
fi

echo -e "${GREEN}面板地址: ${PANEL_URL}${NC}"
echo -e "${GREEN}API Key: ${API_KEY:0:8}...${NC}"
echo ""

# Create install directory
mkdir -p ${INSTALL_DIR}

# Check if binary exists
if [ -f "./${BINARY_NAME}" ]; then
    if [ "$(realpath ./${BINARY_NAME})" != "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        echo -e "${CYAN}使用本地二进制文件...${NC}"
        cp ./${BINARY_NAME} ${INSTALL_DIR}/${BINARY_NAME}
    else
        echo -e "${CYAN}在安装目录中检测到二进制文件...${NC}"
    fi
else
    if [ ! -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        echo -e "${RED}未找到二进制文件，请先上传到 ${INSTALL_DIR}/${NC}"
        exit 1
    fi
fi

chmod +x ${INSTALL_DIR}/${BINARY_NAME}

# Create systemd service
cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=FlowGate Node
After=network.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${BINARY_NAME} node --panel ${PANEL_URL} --key ${API_KEY}
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
echo -e "${GREEN}  ✓ FlowGate Node 安装成功!${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo ""
echo -e "  节点已连接到面板，请在面板中查看状态"
echo ""
echo -e "  管理命令:"
echo -e "    ${YELLOW}systemctl status ${SERVICE_NAME}${NC}   # 查看状态"
echo -e "    ${YELLOW}systemctl restart ${SERVICE_NAME}${NC}  # 重启节点"
echo -e "    ${YELLOW}systemctl stop ${SERVICE_NAME}${NC}     # 停止节点"
echo -e "    ${YELLOW}journalctl -u ${SERVICE_NAME} -f${NC}   # 查看日志"
echo ""
echo -e "  卸载: ${RED}bash install_node.sh --uninstall${NC}"
echo ""
