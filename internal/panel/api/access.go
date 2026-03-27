package api

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/model"
)

func currentUser(c *gin.Context) *model.User {
	if value, ok := c.Get("user"); ok {
		switch user := value.(type) {
		case *model.User:
			return user
		case model.User:
			return &user
		}
	}
	return nil
}

func isExpiredUser(user *model.User) bool {
	return user != nil && user.ExpiresAt != nil && !user.ExpiresAt.After(time.Now())
}

func isManagerRole(role string) bool {
	return role == "admin"
}

func canManageUser(actor *model.User, target *model.User) (bool, error) {
	if actor == nil || target == nil {
		return false, nil
	}
	return actor.Role == "admin" && target.Role != "admin", nil
}

func canAccessOwner(database *db.Database, actor *model.User, ownerUserID int64) (bool, error) {
	if actor == nil {
		return false, nil
	}
	if actor.Role == "admin" {
		return true, nil
	}
	if ownerUserID == actor.ID {
		return true, nil
	}
	return false, nil
}

func resolvedOwnerUser(database *db.Database, actor *model.User, requestedOwnerID *int64) (*model.User, error) {
	if actor == nil {
		return nil, errors.New("缺少当前用户")
	}

	ownerID := actor.ID
	if requestedOwnerID != nil {
		ownerID = *requestedOwnerID
	}
	if ownerID <= 0 {
		return nil, errors.New("规则所属用户无效")
	}

	owner, err := database.GetUserByID(ownerID)
	if err != nil {
		return nil, err
	}

	allowed, err := canAccessOwner(database, actor, owner.ID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errors.New("规则所属用户超出当前账号权限范围")
	}
	return owner, nil
}

func canUseNode(database *db.Database, actor *model.User, nodeID int64) (bool, *model.UserNodeAccess, error) {
	if actor == nil {
		return false, nil, nil
	}
	if actor.Role == "admin" {
		return true, nil, nil
	}
	access, err := database.GetUserNodeAccess(actor.ID, nodeID)
	if err != nil {
		return false, nil, nil
	}
	return true, access, nil
}

func ManagerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if !isManagerRole(role) {
			c.JSON(403, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}
