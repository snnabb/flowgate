package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/model"
)

type UserHandler struct {
	DB *db.Database
}

// CreateUser creates a new panel user.
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "鏃犳晥鐨勮姹?"})
		return
	}

	actor := currentUser(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current user"})
		return
	}

	role := req.Role
	if role == "" {
		role = "user"
	}

	switch actor.Role {
	case "admin":
		if role != "admin" && role != "reseller" && role != "user" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user role"})
			return
		}
		if req.ParentID != nil && *req.ParentID > 0 {
			if _, err := h.DB.GetUserByID(*req.ParentID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "parent user not found"})
				return
			}
		}
	case "reseller":
		if role != "user" {
			c.JSON(http.StatusForbidden, gin.H{"error": "reseller can only create user accounts"})
			return
		}
		req.ParentID = &actor.ID
	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "闇€瑕佺鐞嗘潈闄?"})
		return
	}

	req.Role = role

	password, err := preparePassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "瀵嗙爜鍔犲瘑澶辫触"})
		return
	}

	user, err := h.DB.CreateUserWithOptions(&req, string(hash))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "鐢ㄦ埛鍚嶅凡瀛樺湪"})
		return
	}
	_ = h.DB.CreateEvent("user", "鐢ㄦ埛宸插垱寤?", actor.Username+" 鍒涘缓浜嗙敤鎴?"+user.Username)

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// ListUsers returns users visible to the current manager.
func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.DB.ListUsersVisibleTo(currentUser(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

// DeleteUser deletes a user visible to the current manager.
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	actor := currentUser(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current user"})
		return
	}

	// Prevent deleting yourself.
	if actor.ID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "涓嶈兘鍒犻櫎鑷繁鐨勮处鍙?"})
		return
	}

	target, err := h.DB.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "鐢ㄦ埛涓嶅瓨鍦?"})
		return
	}

	allowed, err := canManageUser(actor, target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := h.DB.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.DB.CreateEvent("user", "鐢ㄦ埛宸插垹闄?", actor.Username+" 鍒犻櫎浜嗙敤鎴?"+target.Username)
	c.JSON(http.StatusOK, gin.H{"message": "鐢ㄦ埛宸插垹闄?"})
}

// ChangePassword allows a user to change their password
func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "鏃犳晥鐨勮姹?"})
		return
	}

	username, _ := c.Get("username")
	user, err := h.DB.GetUserByUsername(username.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "鐢ㄦ埛涓嶅瓨鍦?"})
		return
	}

	if err := comparePassword(user.PasswordHash, req.OldPassword); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "鏃у瘑鐮佷笉姝ｇ‘"})
		return
	}

	newPassword, err := preparePassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	_ = userID

	// Update password in DB
	h.DB.UpdateUserPassword(user.ID, string(hash))
	_ = h.DB.CreateEvent("user", "瀵嗙爜宸蹭慨鏀?", user.Username+" 淇敼浜嗗瘑鐮?")
	c.JSON(http.StatusOK, gin.H{"message": "瀵嗙爜宸蹭慨鏀?"})
}
