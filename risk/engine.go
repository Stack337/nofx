package risk

import (
	"context"
	"math"
	"strings"
	"time"

	"nofx/agent"
	"nofx/provider"
)

type Engine interface {
	Authorize(context.Context, provider.DecisionResponse, AccountRisk, Policy) (AuthorizedDecision, error)
}

type engine struct {
	now func() time.Time
}

func NewEngine(now func() time.Time) Engine {
	if now == nil {
		now = time.Now
	}
	return &engine{now: now}
}

func (e *engine) Authorize(ctx context.Context, decision provider.DecisionResponse, account AccountRisk, policy Policy) (AuthorizedDecision, error) {
	if err := ctx.Err(); err != nil {
		return AuthorizedDecision{}, err
	}
	now := e.now().UTC()
	if decision.Action == agent.DecisionHold {
		return AuthorizedDecision{Decision: decision, AuthorizedAt: now}, nil
	}
	if policy.MaxDecisionAge <= 0 {
		policy.MaxDecisionAge = 2 * time.Minute
	}
	if decision.GeneratedAt.IsZero() || decision.GeneratedAt.After(now.Add(time.Second)) || now.Sub(decision.GeneratedAt) > policy.MaxDecisionAge {
		return reject(CodeStaleDecision, "decision is missing, future-dated, or stale")
	}
	if decision.Action == agent.DecisionClose {
		return AuthorizedDecision{Decision: decision, AuthorizedAt: now}, nil
	}
	if account.KillSwitchEnabled {
		return reject(CodeKillSwitch, "agent kill switch is enabled")
	}
	if policy.MaxLeverage > 0 && decision.Leverage > policy.MaxLeverage {
		return reject(CodeMaxLeverage, "requested leverage exceeds policy")
	}
	if policy.MaxDailyLoss > 0 && account.DailyPnL <= -policy.MaxDailyLoss {
		return reject(CodeMaxDailyLoss, "daily loss limit reached")
	}
	if policy.MaxPositions > 0 && account.OpenPositions >= policy.MaxPositions {
		return reject(CodeMaxPositions, "maximum open position count reached")
	}
	price := account.MarkPrices[decision.Symbol]
	if !finitePositive(price) || !finitePositive(decision.Quantity) {
		return reject(CodeInvalidMarketPrice, "market price and quantity must be finite and positive")
	}
	notional := price * decision.Quantity
	if policy.MaxNotional > 0 && account.OpenNotional+notional > policy.MaxNotional {
		return reject(CodeMaxNotional, "aggregate notional exceeds policy")
	}
	if policy.RequireProtection && (decision.StopLoss == nil || decision.TakeProfit == nil) {
		return reject(CodeProtectionRequired, "stop loss and take profit are required")
	}
	if decision.StopLoss != nil || decision.TakeProfit != nil {
		if !safeProtection(decision.Side, price, decision.StopLoss, decision.TakeProfit) {
			return reject(CodeUnsafeProtection, "stop loss or take profit is on the unsafe side of market price")
		}
	}
	if policy.MaxTradeLoss > 0 && decision.StopLoss != nil {
		loss := math.Abs(price-*decision.StopLoss) * decision.Quantity
		if loss > policy.MaxTradeLoss {
			return reject(CodeMaxTradeLoss, "protected trade loss exceeds policy")
		}
	}
	return AuthorizedDecision{Decision: decision, Notional: notional, AuthorizedAt: now}, nil
}

func safeProtection(side string, price float64, stop, take *float64) bool {
	if stop != nil && !finitePositive(*stop) || take != nil && !finitePositive(*take) {
		return false
	}
	short := strings.EqualFold(side, "sell") || strings.EqualFold(side, "short")
	if short {
		return (stop == nil || *stop > price) && (take == nil || *take < price)
	}
	return (stop == nil || *stop < price) && (take == nil || *take > price)
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func reject(code, message string) (AuthorizedDecision, error) {
	return AuthorizedDecision{}, &Rejection{Code: code, Message: message}
}
