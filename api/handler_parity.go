package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// handleParityCycles exposes persisted cycle state with explicit terminal
// errors, so clients can distinguish active work from a failed AI/exchange
// round. Ownership is checked against the authenticated user's traders.
func (s *Server) handleParityCycles(c *gin.Context) {
	agentID := strings.TrimSpace(c.Query("agent_id"))
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id is required"})
		return
	}

	limit := 20
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
			return
		}
		limit = parsed
	}

	userID := c.GetString("user_id")
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify agent ownership"})
		return
	}
	owned := false
	for _, trader := range traders {
		if trader.ID == agentID {
			owned = true
			break
		}
	}
	if !owned {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	records, err := s.store.ParityCycle().ListByAgent(agentID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load parity cycles"})
		return
	}
	response := make([]gin.H, 0, len(records))
	for _, record := range records {
		response = append(response, gin.H{
			"cycle_id":       record.ID,
			"agent_id":       record.AgentID,
			"state":          record.State,
			"shadow":         record.Shadow,
			"error_code":     record.ErrorCode,
			"error_message":  record.ErrorMessage,
			"started_at":     record.StartedAt,
			"updated_at":     record.UpdatedAt,
			"completed_at":   record.CompletedAt,
			"correlation_id": record.CorrelationID,
			"metadata":       record.Metadata,
		})
	}
	c.JSON(http.StatusOK, gin.H{"cycles": response})
}
