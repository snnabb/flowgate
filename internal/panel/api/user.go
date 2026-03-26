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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	actor := currentUser(c)
	if actor == nil || actor.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin required"})
		return
	}

	role := req.Role
	if role == "" {
		role = "user"
	}
	if role != "user" && role != "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	req.Role = role
	req.ParentID = nil

	password, err := preparePassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}

	user, err := h.DB.CreateUserWithOptions(&req, string(hash))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}

	_ = h.DB.CreateEvent("user", "User created", actor.Username+" created "+user.Username)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// ListUsers returns users visible to the current actor.
func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.DB.ListUsersVisibleTo(currentUser(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

// UpdateUser updates editable account fields.
func (h *UserHandler) UpdateUser(c *gin.Context) {
	actor := currentUser(c)
	if actor == nil || actor.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin required"})
		return
	}

	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	target, err := h.DB.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
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

	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.DB.UpdateUser(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.DB.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.DB.CreateEvent("user", "User updated", actor.Username+" updated "+updated.Username)
	c.JSON(http.StatusOK, gin.H{"user": updated})
}

// GetUserAccess returns the node assignments for one user.
func (h *UserHandler) GetUserAccess(c *gin.Context) {
	actor := currentUser(c)
	if actor == nil || actor.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin required"})
		return
	}

	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	target, err := h.DB.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
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

	access, err := h.DB.ListUserNodeAccess(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if access == nil {
		access = []model.UserNodeAccess{}
	}
	c.JSON(http.StatusOK, gin.H{"access": access})
}

// GetSelfAccess returns the current user's node assignments.
func (h *UserHandler) GetSelfAccess(c *gin.Context) {
	actor := currentUser(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current user"})
		return
	}

	access, err := h.DB.ListUserNodeAccess(actor.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if access == nil {
		access = []model.UserNodeAccess{}
	}
	c.JSON(http.StatusOK, gin.H{"access": access})
}

// ReplaceUserAccess replaces the node assignments for one user.
func (h *UserHandler) ReplaceUserAccess(c *gin.Context) {
	actor := currentUser(c)
	if actor == nil || actor.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin required"})
		return
	}

	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	target, err := h.DB.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
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

	var req model.ReplaceUserNodeAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.DB.ReplaceUserNodeAccess(id, req.Access); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	access, err := h.DB.ListUserNodeAccess(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.DB.CreateEvent("user", "User access updated", actor.Username+" updated node access for "+target.Username)
	c.JSON(http.StatusOK, gin.H{"access": access})
}

// DeleteUser deletes a user.
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	actor := currentUser(c)
	if actor == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing current user"})
		return
	}
	if actor.ID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete yourself"})
		return
	}

	target, err := h.DB.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
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
	_ = h.DB.CreateEvent("user", "User deleted", actor.Username+" deleted "+target.Username)
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

// ChangePassword allows a user to change their password.
func (h *UserHandler) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	username, _ := c.Get("username")
	user, err := h.DB.GetUserByUsername(username.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err := comparePassword(user.PasswordHash, req.OldPassword); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "old password incorrect"})
		return
	}

	newPassword, err := preparePassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}
	if err := h.DB.UpdateUserPassword(user.ID, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = h.DB.CreateEvent("user", "Password changed", user.Username+" changed their password")
	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}
