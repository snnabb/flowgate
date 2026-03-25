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

// loginRateLimiter tracks failed login attempts per IP
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

	// Remove old attempts
	var recent []time.Time
	for _, t := range attempts {
		if now.Sub(t) < lockoutDuration {
			recent = append(recent, t)
		}
	}
	rl.attempts[ip] = recent

	// Count attempts within window
	count := 0
	for _, t := range recent {
		if now.Sub(t) < loginWindow {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求"})
		return
	}

	count, _ := h.DB.GetUserCount()
	if count > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "注册已关闭"})
		return
	}

	role := "admin"

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

	user, err := h.DB.CreateUser(req.Username, string(hash), role)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
		return
	}
	_ = h.DB.CreateEvent("user", "管理员初始化", user.Username+" 完成了管理员账号初始化")

	token, _ := h.generateToken(user)
	c.JSON(http.StatusOK, model.LoginResponse{Token: token, User: *user})
}

// Login handles user login
func (h *AuthHandler) Login(c *gin.Context) {
	clientIP := c.ClientIP()

	if rateLimiter.isBlocked(clientIP) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "登录尝试过多，请稍后再试"})
		return
	}

	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求"})
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

	rateLimiter.clearIP(clientIP)
	token, _ := h.generateToken(user)
	c.JSON(http.StatusOK, model.LoginResponse{Token: token, User: *user})
}

// CheckSetup returns whether any user exists (for first-time setup)
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

// AuthMiddleware validates JWT tokens
func AuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证令牌"})
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证令牌"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证信息"})
			c.Abort()
			return
		}

		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证信息"})
			c.Abort()
			return
		}
		username, ok := claims["username"].(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证信息"})
			c.Abort()
			return
		}
		role, ok := claims["role"].(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证信息"})
			c.Abort()
			return
		}

		c.Set("user_id", int64(userIDFloat))
		c.Set("username", username)
		c.Set("role", role)
		c.Next()
	}
}

// AdminMiddleware ensures the user is an admin
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}
