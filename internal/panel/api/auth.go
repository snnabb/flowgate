package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/model"
)

// loginRateLimiter tracks failed login attempts per IP.
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

var rateLimiter = &loginRateLimiter{
	attempts: make(map[string][]time.Time),
}

const (
	maxLoginAttempts = 5
	loginWindow      = 5 * time.Minute
	lockoutDuration  = 10 * time.Minute
)

func (rl *loginRateLimiter) isBlocked(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	attempts := rl.attempts[ip]

	var recent []time.Time
	for _, ts := range attempts {
		if now.Sub(ts) < lockoutDuration {
			recent = append(recent, ts)
		}
	}
	rl.attempts[ip] = recent

	count := 0
	for _, ts := range recent {
		if now.Sub(ts) < loginWindow {
			count++
		}
	}
	return count >= maxLoginAttempts
}

func (rl *loginRateLimiter) recordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.attempts[ip] = append(rl.attempts[ip], time.Now())
}

func (rl *loginRateLimiter) clearIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

type AuthHandler struct {
	DB        *db.Database
	JWTSecret string
}

// Register handles the one-time bootstrap admin registration.
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效"})
		return
	}

	count, _ := h.DB.GetUserCount()
	if count > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "registration closed"})
		return
	}

	password, err := preparePassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	user, err := h.DB.CreateUser(req.Username, string(hash), "admin")
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
		return
	}

	_ = h.DB.CreateEvent("user", "管理员已初始化", user.Username+" 完成了面板初始化")
	token, _ := h.generateToken(user)
	c.JSON(http.StatusOK, model.LoginResponse{Token: token, User: *user})
}

// Login handles user login.
func (h *AuthHandler) Login(c *gin.Context) {
	clientIP := c.ClientIP()
	if rateLimiter.isBlocked(clientIP) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "登录尝试次数过多"})
		return
	}

	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效"})
		return
	}

	user, err := h.DB.GetUserByUsername(req.Username)
	if err != nil {
		rateLimiter.recordFailure(clientIP)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	if err := comparePassword(user.PasswordHash, req.Password); err != nil {
		rateLimiter.recordFailure(clientIP)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if !user.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "账号已禁用"})
		return
	}
	if isExpiredUser(user) {
		c.JSON(http.StatusForbidden, gin.H{"error": "账号已过期"})
		return
	}

	rateLimiter.clearIP(clientIP)
	token, _ := h.generateToken(user)
	c.JSON(http.StatusOK, model.LoginResponse{Token: token, User: *user})
}

// CheckSetup returns whether any user exists.
func (h *AuthHandler) CheckSetup(c *gin.Context) {
	count, _ := h.DB.GetUserCount()
	c.JSON(http.StatusOK, gin.H{"needs_setup": count == 0})
}

func (h *AuthHandler) generateToken(user *model.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.JWTSecret))
}

// AuthMiddleware validates JWT tokens.
func AuthMiddleware(secret string, database *db.Database) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}

		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}
		if database == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少认证数据库"})
			c.Abort()
			return
		}

		user, err := database.GetUserByID(int64(userIDFloat))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			c.Abort()
			return
		}
		if !user.Enabled {
			c.JSON(http.StatusForbidden, gin.H{"error": "账号已禁用"})
			c.Abort()
			return
		}
		if isExpiredUser(user) {
			c.JSON(http.StatusForbidden, gin.H{"error": "账号已过期"})
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Next()
	}
}

// AdminMiddleware ensures the user is an admin.
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}
