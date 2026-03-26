package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/model"
)

type StatsHandler struct {
	DB *db.Database
}

// GetDashboard returns dashboard overview statistics for the current actor.
func (h *StatsHandler) GetDashboard(c *gin.Context) {
	stats, err := h.DB.GetDashboardStats(currentUser(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// GetTrafficHistory returns traffic log history for a visible rule.
func (h *StatsHandler) GetTrafficHistory(c *gin.Context) {
	ruleID, _ := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 || hours > 720 {
		hours = 24
	}

	rule, err := h.DB.GetRuleByID(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	allowed, err := canAccessOwner(h.DB, currentUser(c), rule.OwnerUserID)
	if err != nil || !allowed {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	logs, err := h.DB.GetTrafficLogs(ruleID, hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// GetAggregateTraffic returns hourly aggregate traffic scoped to the current actor.
func (h *StatsHandler) GetAggregateTraffic(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 || hours > 720 {
		hours = 24
	}

	logs, err := h.DB.GetAggregateTrafficLogsVisibleTo(currentUser(c), hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// GetRecentEvents returns the latest panel activity items.
func (h *StatsHandler) GetRecentEvents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	if limit <= 0 || limit > 100 {
		limit = 12
	}

	events, err := h.DB.ListRecentEvents(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if events == nil {
		events = []model.PanelEvent{}
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}
