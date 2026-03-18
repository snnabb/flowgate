package panel

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
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
	ruleHandler := &api.RuleHandler{DB: database, Hub: wsHub}
	statsHandler := &api.StatsHandler{DB: database}
	userHandler := &api.UserHandler{DB: database}

	// Setup Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	// === Public routes ===
	r.POST("/api/auth/login", authHandler.Login)
	r.POST("/api/auth/register", authHandler.Register)
	r.GET("/api/auth/setup", authHandler.CheckSetup)

	// === Node WebSocket (authenticated by API key) ===
	r.GET("/ws/node", nodeHandler.HandleNodeWS)

	// === Protected API routes ===
	authorized := r.Group("/api")
	authorized.Use(api.AuthMiddleware(cfg.JWTSecret))
	{
		// Dashboard
		authorized.GET("/dashboard", statsHandler.GetDashboard)

		// Nodes
		authorized.GET("/nodes", nodeHandler.ListNodes)
		authorized.POST("/nodes", nodeHandler.CreateNode)
		authorized.GET("/nodes/:id", nodeHandler.GetNode)
		authorized.DELETE("/nodes/:id", nodeHandler.DeleteNode)

		// Rules
		authorized.GET("/rules", ruleHandler.ListRules)
		authorized.POST("/rules", ruleHandler.CreateRule)
		authorized.GET("/rules/:id", ruleHandler.GetRule)
		authorized.PUT("/rules/:id", ruleHandler.UpdateRule)
		authorized.DELETE("/rules/:id", ruleHandler.DeleteRule)
		authorized.POST("/rules/:id/toggle", ruleHandler.ToggleRule)

		// Traffic
		authorized.GET("/traffic/:rule_id", statsHandler.GetTrafficHistory)

		// Users (admin only)
		admin := authorized.Group("")
		admin.Use(api.AdminMiddleware())
		{
			admin.GET("/users", userHandler.ListUsers)
			admin.DELETE("/users/:id", userHandler.DeleteUser)
		}

		// User self-service
		authorized.POST("/user/password", userHandler.ChangePassword)
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
						c.FileFromFS(requestPath, http.FS(webFS))
						return
					}
					f.Close()
				}
			}

			// Fallback to index.html for SPA routing and the root path.
			c.FileFromFS("index.html", http.FS(webFS))
		})
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("🚀 FlowGate Panel starting on %s", addr)

	if cfg.TLS && cfg.CertFile != "" && cfg.KeyFile != "" {
		return r.RunTLS(addr, cfg.CertFile, cfg.KeyFile)
	}
	return r.Run(addr)
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
