package parity

import (
	"math"
	"strings"
	"time"
)

type Action string

const (
	ActionOpenLong   Action = "open_long"
	ActionOpenShort  Action = "open_short"
	ActionCloseLong  Action = "close_long"
	ActionCloseShort Action = "close_short"
	ActionHold       Action = "hold"
	ActionWait       Action = "wait"
)

type RiskConfig struct {
	BTCETHMaxLeverage       int
	AltcoinMaxLeverage      int
	BTCETHMaxPositionRatio  float64
	AltcoinMaxPositionRatio float64
	MinPositionGeneralUSD   float64
	MinPositionBTCETHUSD    float64
	MinRiskReward           float64
	MaxPriceAge             time.Duration
	MaxPriceDivergencePct   float64
}

type DecisionInput struct {
	Symbol             string
	Action             Action
	Leverage           int
	PositionSizeUSD    float64
	StopLoss           float64
	TakeProfit         float64
	MarketPrice        float64
	PriceTimestamp     time.Time
	PriceDivergencePct float64
}

type RiskResult struct {
	Allowed     bool
	ReasonCode  string
	Decision    DecisionInput
	Adjustments []string
}

func rejected(decision DecisionInput, code string) RiskResult {
	return RiskResult{Allowed: false, ReasonCode: code, Decision: decision}
}

// ValidateDecision applies hard, deterministic rules before any exchange call.
func ValidateDecision(decision DecisionInput, accountEquity float64, now time.Time, config RiskConfig) RiskResult {
	decision.Symbol = strings.ToUpper(strings.TrimSpace(decision.Symbol))

	switch decision.Action {
	case ActionCloseLong, ActionCloseShort, ActionHold, ActionWait:
		return RiskResult{Allowed: true, Decision: decision}
	case ActionOpenLong, ActionOpenShort:
		// Continue with exposure checks below.
	default:
		return rejected(decision, "invalid_action")
	}

	if decision.Symbol == "" {
		return rejected(decision, "invalid_symbol")
	}
	if decision.PriceTimestamp.IsZero() || (config.MaxPriceAge > 0 && now.Sub(decision.PriceTimestamp) > config.MaxPriceAge) {
		return rejected(decision, "stale_price")
	}
	if math.Abs(decision.PriceDivergencePct) > config.MaxPriceDivergencePct {
		return rejected(decision, "divergent_price")
	}
	if decision.MarketPrice <= 0 {
		return rejected(decision, "invalid_market_price")
	}
	if decision.Leverage <= 0 {
		return rejected(decision, "invalid_leverage")
	}
	if decision.PositionSizeUSD <= 0 {
		return rejected(decision, "invalid_position_size")
	}
	if accountEquity <= 0 {
		return rejected(decision, "invalid_account_equity")
	}

	isBTCETH := decision.Symbol == "BTCUSDT" || decision.Symbol == "ETHUSDT"
	maxLeverage := config.AltcoinMaxLeverage
	positionRatio := config.AltcoinMaxPositionRatio
	minimumPosition := config.MinPositionGeneralUSD
	if isBTCETH {
		maxLeverage = config.BTCETHMaxLeverage
		positionRatio = config.BTCETHMaxPositionRatio
		minimumPosition = config.MinPositionBTCETHUSD
	}

	adjustments := []string{}
	if decision.Leverage > maxLeverage {
		decision.Leverage = maxLeverage
		adjustments = append(adjustments, "leverage_clamped")
	}
	if decision.PositionSizeUSD < minimumPosition {
		return rejected(decision, "minimum_order_value")
	}
	maxPosition := accountEquity * positionRatio
	if decision.PositionSizeUSD > maxPosition+(maxPosition*0.01) {
		return rejected(decision, "position_cap")
	}

	var risk, reward float64
	switch decision.Action {
	case ActionOpenLong:
		if decision.StopLoss <= 0 || decision.TakeProfit <= 0 ||
			decision.StopLoss >= decision.MarketPrice || decision.TakeProfit <= decision.MarketPrice {
			return rejected(decision, "invalid_sl_tp")
		}
		risk = decision.MarketPrice - decision.StopLoss
		reward = decision.TakeProfit - decision.MarketPrice
	case ActionOpenShort:
		if decision.StopLoss <= 0 || decision.TakeProfit <= 0 ||
			decision.StopLoss <= decision.MarketPrice || decision.TakeProfit >= decision.MarketPrice {
			return rejected(decision, "invalid_sl_tp")
		}
		risk = decision.StopLoss - decision.MarketPrice
		reward = decision.MarketPrice - decision.TakeProfit
	}

	if risk <= 0 || reward/risk < config.MinRiskReward {
		return rejected(decision, "risk_reward_too_low")
	}

	return RiskResult{
		Allowed:     true,
		Decision:    decision,
		Adjustments: adjustments,
	}
}
