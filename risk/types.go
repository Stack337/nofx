package risk

import (
	"fmt"
	"time"

	"nofx/provider"
)

const (
	CodeStaleDecision      = "RISK_STALE_DECISION"
	CodeKillSwitch         = "RISK_KILL_SWITCH"
	CodeMaxLeverage        = "RISK_MAX_LEVERAGE"
	CodeMaxNotional        = "RISK_MAX_NOTIONAL"
	CodeMaxDailyLoss       = "RISK_MAX_DAILY_LOSS"
	CodeMaxTradeLoss       = "RISK_MAX_TRADE_LOSS"
	CodeMaxPositions       = "RISK_MAX_POSITIONS"
	CodeProtectionRequired = "RISK_PROTECTION_REQUIRED"
	CodeUnsafeProtection   = "RISK_UNSAFE_PROTECTION"
	CodeInvalidMarketPrice = "RISK_INVALID_MARKET_PRICE"
)

type Policy struct {
	MaxLeverage       int
	MaxNotional       float64
	MaxDailyLoss      float64
	MaxTradeLoss      float64
	MaxPositions      int
	RequireProtection bool
	MaxDecisionAge    time.Duration
}

type AccountRisk struct {
	Equity            float64
	AvailableBalance  float64
	DailyPnL          float64
	OpenNotional      float64
	OpenPositions     int
	MarkPrices        map[string]float64
	KillSwitchEnabled bool
}

type AuthorizedDecision struct {
	Decision      provider.DecisionResponse
	Notional      float64
	LiveConfirmed bool
	AuthorizedAt  time.Time
}

type Rejection struct {
	Code    string
	Message string
}

func (e *Rejection) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
