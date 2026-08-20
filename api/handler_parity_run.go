package api

import (
	"net/http"
	"nofx/parity"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterParityRunner attaches a shadow-first runner to an owned agent. The
// registry is explicit so the API never silently creates a live exchange
// client from a trader's credentials.
func (s *Server) RegisterParityRunner(agentID string, runner *parity.Runner) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || runner == nil {
		return
	}
	s.parityMu.Lock()
	defer s.parityMu.Unlock()
	if s.parityRunners == nil {
		s.parityRunners = make(map[string]*parity.Runner)
	}
	s.parityRunners[agentID] = runner
}

func (s *Server) handleStartParityCycle(c *gin.Context) {
	agentID := strings.TrimSpace(c.Query("agent_id"))
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id is required"})
		return
	}

	s.parityMu.RLock()
	runner := s.parityRunners[agentID]
	s.parityMu.RUnlock()
	if runner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":      "parity runner is not registered for this agent",
			"error_code": "parity_runner_unavailable",
		})
		return
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

	cycleID, err := runner.StartCycle(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "error_code": "parity_cycle_start_failed"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"cycle_id": cycleID, "agent_id": agentID, "state": "scheduled"})
}
