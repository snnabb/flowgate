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

// GetDashboard returns dashboard overview statistics
func (h *StatsHandler) GetDashboard(c *gin.Context) {
	stats, err := h.DB.GetDashboardStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// GetTrafficHistory returns traffic log history for a rule
func (h *StatsHandler) GetTrafficHistory(c *gin.Context) {
	ruleID, _ := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 || hours > 720 {
		hours = 24
	}

	logs, err := h.DB.GetTrafficLogs(ruleID, hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// GetAggregateTraffic returns hourly aggregate traffic across all rules.
func (h *StatsHandler) GetAggregateTraffic(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 || hours > 720 {
		hours = 24
	}

	logs, err := h.DB.GetAggregateTrafficLogs(hours)
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
