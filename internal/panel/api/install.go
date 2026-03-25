package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/flowgate/flowgate/internal/panel/db"
)

type InstallHandler struct {
	DB *db.Database
}

// ServeBinary serves the panel's own executable for node download.
func (h *InstallHandler) ServeBinary(c *gin.Context) {
	execPath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法获取可执行文件"})
		return
	}
	c.Header("Content-Disposition", "attachment; filename=flowgate")
	c.File(execPath)
}

// ServeInstallScript generates a bash install script for a specific node.
func (h *InstallHandler) ServeInstallScript(c *gin.Context) {
	apiKey := c.Param("key")
	_, err := h.DB.GetNodeByAPIKey(apiKey)
	if err != nil {
		c.String(http.StatusNotFound, "echo '错误: 无效的 API 密钥'; exit 1")
		return
	}

	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := c.Request.Host
	panelURL := scheme + "://" + host

	wsScheme := "ws"
	if scheme == "https" {
		wsScheme = "wss"
	}
	wsURL := wsScheme + "://" + host + "/ws/node"

	r := strings.NewReplacer(
		"{{PANEL_URL}}", panelURL,
		"{{WS_URL}}", wsURL,
		"{{API_KEY}}", apiKey,
	)

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, r.Replace(installScript))
}

const installScript = `#!/bin/bash
set -e

echo ""
echo "  FlowGate 节点一键部署"
echo "  ====================="
echo ""

if [ "$(id -u)" != "0" ]; then
    echo "错误: 请使用 root 用户运行此脚本"
    exit 1
fi

# Stop existing service if running
systemctl stop flowgate-node 2>/dev/null || true

echo "[1/4] 正在下载 FlowGate..."
curl -sSL "{{PANEL_URL}}/api/node/binary" -o /tmp/flowgate
chmod +x /tmp/flowgate

echo "[2/4] 正在安装到 /usr/local/bin/..."
mv -f /tmp/flowgate /usr/local/bin/flowgate

echo "[3/4] 正在配置系统服务..."
cat > /etc/systemd/system/flowgate-node.service << 'FGEOF'
[Unit]
Description=FlowGate Node
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/flowgate node --panel {{WS_URL}} --key {{API_KEY}}
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
FGEOF

systemctl daemon-reload
systemctl enable flowgate-node

echo "[4/4] 正在启动节点..."
systemctl restart flowgate-node
sleep 2

if systemctl is-active --quiet flowgate-node; then
    echo ""
    echo "  ✓ 部署完成! 节点已启动并设为开机自启"
else
    echo ""
    echo "  ✗ 节点启动失败，请检查日志:"
    echo "    journalctl -u flowgate-node -n 20"
fi

echo ""
echo "  常用命令:"
echo "    查看状态: systemctl status flowgate-node"
echo "    查看日志: journalctl -u flowgate-node -f"
echo "    重启节点: systemctl restart flowgate-node"
echo "    停止节点: systemctl stop flowgate-node"
echo "    卸载节点: systemctl disable --now flowgate-node && rm -f /usr/local/bin/flowgate /etc/systemd/system/flowgate-node.service && systemctl daemon-reload"
echo ""
`
