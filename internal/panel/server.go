package panel

import (
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/api"
	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/hub"
)

// Start initializes and starts the Panel server
func Start(cfg *common.PanelConfig, webFS fs.FS) error {
	// Initialize database
	database, err := db.New(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("database init: %w", err)
	}
	defer database.Close()

	// Initialize WebSocket hub
	wsHub := hub.New(database)

	// Initialize handlers
	authHandler := &api.AuthHandler{DB: database, JWTSecret: cfg.JWTSecret}
	nodeHandler := &api.NodeHandler{DB: database, Hub: wsHub}
	nodeGroupHandler := &api.NodeGroupHandler{DB: database, Hub: wsHub}
	ruleHandler := &api.RuleHandler{DB: database, Hub: wsHub}
	statsHandler := &api.StatsHandler{DB: database}
	userHandler := &api.UserHandler{DB: database}
	installHandler := &api.InstallHandler{DB: database}
	panelWSHandler := &api.PanelWSHandler{Hub: wsHub, JWTSecret: cfg.JWTSecret}

	// Setup Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())
	r.Use(noCacheMiddleware())

	// === Public routes ===
	r.POST("/api/auth/login", authHandler.Login)
	r.POST("/api/auth/register", authHandler.Register)
	r.GET("/api/auth/setup", authHandler.CheckSetup)

	// === Node WebSocket (authenticated by API key) ===
	r.GET("/ws/node", nodeHandler.HandleNodeWS)

	// === Panel WebSocket (authenticated by JWT in query param) ===
	r.GET("/ws/panel", panelWSHandler.HandlePanelWS)

	// === Node install (authenticated by API key in URL) ===
	r.GET("/api/node/install/:key", installHandler.ServeInstallScript)
	r.GET("/api/node/binary", installHandler.ServeBinary)

	// === Protected API routes ===
	authorized := r.Group("/api")
	authorized.Use(api.AuthMiddleware(cfg.JWTSecret, database))
	{
		// Dashboard
		authorized.GET("/dashboard", statsHandler.GetDashboard)

		// Nodes (read: scoped by assignment; write: admin only)
		authorized.GET("/nodes", nodeHandler.ListNodes)
		authorized.GET("/nodes/:id", nodeHandler.GetNode)
		authorized.GET("/node-groups", nodeGroupHandler.ListNodeGroups)

		// Rules (read: own/admin scope; write: admin only)
		authorized.GET("/rules", ruleHandler.ListRules)
		authorized.GET("/rules/:id", ruleHandler.GetRule)

		// Traffic
		authorized.GET("/traffic/aggregate", statsHandler.GetAggregateTraffic)
		authorized.GET("/traffic/:rule_id", statsHandler.GetTrafficHistory)
		authorized.GET("/events", statsHandler.GetRecentEvents)

		// User self-service
		authorized.POST("/user/password", userHandler.ChangePassword)
		authorized.GET("/user/access", userHandler.GetSelfAccess)

		// Manager operations
		manager := authorized.Group("")
		manager.Use(api.ManagerMiddleware())
		{
			manager.POST("/nodes", nodeHandler.CreateNode)
			manager.DELETE("/nodes/:id", nodeHandler.DeleteNode)

			manager.POST("/rules", ruleHandler.CreateRule)
			manager.PUT("/rules/:id", ruleHandler.UpdateRule)
			manager.DELETE("/rules/:id", ruleHandler.DeleteRule)
			manager.POST("/rules/:id/toggle", ruleHandler.ToggleRule)
			manager.POST("/rules/:id/reset-traffic", ruleHandler.ResetTraffic)
			manager.POST("/rules/:id/test-latency", ruleHandler.TestLatency)
			manager.GET("/rules/:id/chain-latency", ruleHandler.GetChainLatency)

			manager.POST("/users", userHandler.CreateUser)
			manager.GET("/users", userHandler.ListUsers)
			manager.PUT("/users/:id", userHandler.UpdateUser)
			manager.GET("/users/:id/access", userHandler.GetUserAccess)
			manager.PUT("/users/:id/access", userHandler.ReplaceUserAccess)
			manager.DELETE("/users/:id", userHandler.DeleteUser)
		}

		// Admin-only operations
		admin := authorized.Group("")
		admin.Use(api.AdminMiddleware())
		{
			admin.POST("/node-groups", nodeGroupHandler.CreateNodeGroup)
			admin.DELETE("/node-groups/:id", nodeGroupHandler.DeleteNodeGroup)
		}
	}

	// === Serve embedded Web UI ===
	if webFS != nil {
		r.NoRoute(func(c *gin.Context) {
			requestPath := strings.TrimPrefix(c.Request.URL.Path, "/")
			if requestPath != "" {
				f, err := webFS.Open(requestPath)
				if err == nil {
					if info, statErr := f.Stat(); statErr == nil && !info.IsDir() {
						f.Close()
						if serveEmbeddedFile(c, webFS, requestPath) {
							return
						}
					}
					f.Close()
				}
			}

			// Fallback to index.html for SPA routing and the root path.
			c.Header("Cache-Control", "no-store")
			if serveEmbeddedFile(c, webFS, "index.html") {
				return
			}
			c.Status(http.StatusNotFound)
		})
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("🚀 FlowGate Panel starting on %s", addr)

	if cfg.TLS && cfg.CertFile != "" && cfg.KeyFile != "" {
		return r.RunTLS(addr, cfg.CertFile, cfg.KeyFile)
	}
	return r.Run(addr)
}

func serveEmbeddedFile(c *gin.Context, webFS fs.FS, name string) bool {
	data, err := fs.ReadFile(webFS, name)
	if err != nil {
		return false
	}

	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	c.Data(http.StatusOK, contentType, data)
	return true
}

func noCacheMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
