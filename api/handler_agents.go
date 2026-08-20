package api

import (
	"errors"
	"github.com/google/uuid"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"nofx/agent"
	"nofx/risk"
)

func (s *Server) handleCreateAgent(c *gin.Context) {
	var request struct {
		Name          string `json:"name"`
		ExchangeID    string `json:"exchange_id"`
		AIModelID     string `json:"ai_model_id"`
		StrategyID    string `json:"strategy_id"`
		RiskProfileID string `json:"risk_profile_id"`
		Schedule      string `json:"schedule"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Name) == "" || request.ExchangeID == "" || request.AIModelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, exchange_id, and ai_model_id are required"})
		return
	}
	item := agent.Agent{ID: uuid.NewString(), UserID: c.GetString("user_id"), Name: strings.TrimSpace(request.Name), ExchangeID: request.ExchangeID, AIModelID: request.AIModelID, StrategyID: request.StrategyID, RiskProfileID: request.RiskProfileID, Schedule: request.Schedule, Mode: agent.ModeShadow, Enabled: false}
	if err := s.store.Agent().Create(c.Request.Context(), item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create agent"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"agent": item})
}

func (s *Server) handleListAgents(c *gin.Context) {
	if s.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent storage unavailable"})
		return
	}
	items, err := s.store.Agent().List(c.Request.Context(), c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list agents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agents": items})
}

func (s *Server) handleGetAgent(c *gin.Context) {
	item, err := s.store.Agent().Get(c.Request.Context(), c.Param("id"))
	if err != nil || item.UserID != c.GetString("user_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	kill, _ := risk.NewKillSwitch(s.store.Agent()).Enabled(c.Request.Context(), item.ID)
	c.JSON(http.StatusOK, gin.H{"agent": item, "kill_switch_enabled": kill})
}

func (s *Server) handleAgentStart(c *gin.Context) {
	if s.agentService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent service is not configured"})
		return
	}
	agentID := c.Param("id")
	item, err := s.store.Agent().Get(c.Request.Context(), agentID)
	if err != nil || item.UserID != c.GetString("user_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	ctx := c.Request.Context()
	go func() { _, _ = s.agentService.RunCycle(ctx, agentID) }()
	c.JSON(http.StatusAccepted, gin.H{"agent_id": agentID, "status": "queued"})
}

func (s *Server) handleAgentStop(c *gin.Context) {
	item, err := s.store.Agent().Get(c.Request.Context(), c.Param("id"))
	if err != nil || item.UserID != c.GetString("user_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	if err := risk.NewKillSwitch(s.store.Agent()).Enable(c.Request.Context(), item.ID, "manual stop"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stop agent"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent_id": item.ID, "status": "stopped"})
}

func (s *Server) handleAgentKillSwitch(c *gin.Context) {
	item, err := s.store.Agent().Get(c.Request.Context(), c.Param("id"))
	if err != nil || item.UserID != c.GetString("user_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	var request struct {
		Enabled      bool   `json:"enabled"`
		Reason       string `json:"reason"`
		Confirmation string `json:"confirmation"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	switcher := risk.NewKillSwitch(s.store.Agent())
	if request.Enabled {
		err = switcher.Enable(c.Request.Context(), item.ID, request.Reason)
	} else {
		err = switcher.Disable(c.Request.Context(), item.ID, request.Confirmation)
	}
	if errors.Is(err, risk.ErrInvalidSwitchConfirmation) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation must be DISABLE <agent_id>"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update kill switch"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent_id": item.ID, "enabled": request.Enabled})
}

func (s *Server) handleAgentLiveConfirmation(c *gin.Context) {
	item, err := s.store.Agent().Get(c.Request.Context(), c.Param("id"))
	if err != nil || item.UserID != c.GetString("user_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	var request struct {
		Confirmation string `json:"confirmation"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Confirmation) != "ENABLE LIVE "+item.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation must be ENABLE LIVE <agent_id>"})
		return
	}
	if err := s.store.Agent().UpdateMode(c.Request.Context(), item.ID, agent.ModeLive, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enable live mode"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent_id": item.ID, "mode": agent.ModeLive, "live_confirmed": true})
}

func (s *Server) handleAgentCycles(c *gin.Context) {
	item, err := s.store.Agent().Get(c.Request.Context(), c.Param("id"))
	if err != nil || item.UserID != c.GetString("user_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	limit := 20
	cycles, err := s.store.Agent().ListCycles(c.Request.Context(), item.ID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list cycles"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cycles": cycles})
}
