package api

import (
	"net/http"
	"strings"
	"time"

	"nofx/ai500"
	"nofx/market"
	"nofx/mcp"
	_ "nofx/mcp/provider"
	"nofx/parity"
	"nofx/trader/bybit"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleRegisterParityRunner(c *gin.Context) {
	agentID := strings.TrimSpace(c.Query("agent_id"))
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id is required"})
		return
	}
	if s.ai500Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":      "AI500 observation store is unavailable",
			"error_code": "ai500_store_unavailable",
		})
		return
	}
	userID := c.GetString("user_id")
	fullConfig, err := s.store.Trader().GetFullConfig(userID, agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	if fullConfig.AIModel == nil || !fullConfig.AIModel.Enabled || strings.TrimSpace(fullConfig.AIModel.APIKey.String()) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent AI model is not enabled or has no API key"})
		return
	}
	if fullConfig.Exchange == nil || strings.ToLower(fullConfig.Exchange.ExchangeType) != "bybit" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parity context currently supports Bybit only"})
		return
	}
	if strings.TrimSpace(fullConfig.Exchange.APIKey.String()) == "" || strings.TrimSpace(fullConfig.Exchange.SecretKey.String()) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bybit read-only context credentials are missing"})
		return
	}

	aiClient := mcp.NewAIClientByProvider(fullConfig.AIModel.Provider)
	if aiClient == nil {
		if strings.TrimSpace(fullConfig.AIModel.CustomModelName) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "custom AI provider requires custom_model_name"})
			return
		}
		aiClient = mcp.NewClient()
	}
	aiClient.SetAPIKey(fullConfig.AIModel.APIKey.String(), fullConfig.AIModel.CustomAPIURL, fullConfig.AIModel.CustomModelName)
	aiClient.SetTimeout(120 * time.Second)
	model := strings.TrimSpace(fullConfig.AIModel.CustomModelName)
	workflow := parity.NewOpenAIWorkflow(aiClient, model, 3)
	exchangeReader := bybit.NewBybitTrader(fullConfig.Exchange.APIKey.String(), fullConfig.Exchange.SecretKey.String())
	contextProvider := parity.NewExchangeContextProvider(exchangeReader, market.NewAPIClient(), ai500.NewCandidateProviderWithConfig(s.ai500Store, s.ai500Symbols, ai500.CandidateProviderConfig{TTL: s.ai500ScoreTTL}))
	runner := parity.NewRunner(parity.RunnerConfig{
		AgentID: agentID, OwnerID: userID, Shadow: true, Deadline: 3 * time.Minute,
		RiskConfig: parity.RiskConfig{
			BTCETHMaxLeverage: 10, AltcoinMaxLeverage: 5,
			BTCETHMaxPositionRatio: 0.3, AltcoinMaxPositionRatio: 0.2,
			MinPositionGeneralUSD: 12, MinPositionBTCETHUSD: 60,
			MinRiskReward: 3, MaxPriceAge: 2 * time.Minute, MaxPriceDivergencePct: 1,
		},
	}, s.store.ParityCycle(), contextProvider, workflow, parity.NewShadowExecution(), parity.NoopSynchronizer{})
	s.RegisterParityRunner(agentID, runner)
	c.JSON(http.StatusCreated, gin.H{"agent_id": agentID, "mode": "shadow", "registered": true})
}
