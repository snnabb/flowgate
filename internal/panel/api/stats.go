package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/flowgate/flowgate/internal/panel/db"
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
