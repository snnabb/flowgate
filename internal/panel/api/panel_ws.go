package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/flowgate/flowgate/internal/panel/hub"
)

type PanelWSHandler struct {
	Hub       *hub.Hub
	JWTSecret string
}

// HandlePanelWS handles WebSocket connections from browser panels
func (h *PanelWSHandler) HandlePanelWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证令牌"})
		return
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		return []byte(h.JWTSecret), nil
	})
	if err != nil || !parsed.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证令牌"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[PanelWS] Upgrade failed: %v", err)
		return
	}

	client := h.Hub.PanelHub.Register(conn)
	go h.Hub.PanelHub.WritePump(client)
	h.Hub.PanelHub.ReadPump(client)
}
