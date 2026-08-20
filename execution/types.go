package execution

import (
	"context"
	"errors"
)

type PositionMode string

const (
	PositionModeOneWay PositionMode = "one-way"
	PositionModeHedge  PositionMode = "hedge"
)

const (
	OrderStatusShadow   = "SHADOW"
	OrderStatusPaper    = "PAPER"
	OrderStatusNew      = "NEW"
	OrderStatusFilled   = "FILLED"
	OrderStatusCanceled = "CANCELED"
	OrderStatusUnknown  = "UNKNOWN"
)

var (
	ErrLiveGate            = errors.New("live execution gate is not confirmed")
	ErrNeedsReconciliation = errors.New("order status needs reconciliation")
)

type AccountState struct {
	Equity, AvailableBalance float64
}

type InstrumentSpec struct {
	QtyStep   float64
	PriceTick float64
}

type OrderRequest struct {
	Symbol, Side string
	Quantity     float64
	Leverage     int
	ReduceOnly   bool
	PositionIdx  int
	ClientID     string
}

type OrderResult struct {
	OrderID string
	Status  string
}

type OrderState = OrderResult

type ProtectionRequest struct {
	Symbol, Side string
	Quantity     float64
	StopLoss     *float64
	TakeProfit   *float64
	PositionIdx  int
}

type Exchange interface {
	Account(context.Context) (AccountState, error)
	Instrument(context.Context, string) (InstrumentSpec, error)
	PositionMode(context.Context, string) (PositionMode, error)
	Place(context.Context, OrderRequest) (OrderResult, error)
	GetOrder(context.Context, string, string) (OrderState, error)
	SetProtection(context.Context, ProtectionRequest) error
}
