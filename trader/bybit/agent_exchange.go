package bybit

import (
	"context"
	"fmt"
	"strings"

	"nofx/execution"
)

var _ execution.Exchange = (*AgentExchange)(nil)

type AgentExchange struct {
	Trader *BybitTrader
}

func NewAgentExchange(trader *BybitTrader) *AgentExchange {
	return &AgentExchange{Trader: trader}
}

func (e *AgentExchange) Account(ctx context.Context) (execution.AccountState, error) {
	if err := ctx.Err(); err != nil {
		return execution.AccountState{}, err
	}
	balance, err := e.Trader.GetBalance()
	if err != nil {
		return execution.AccountState{}, err
	}
	return execution.AccountState{
		Equity: number(balance["totalEquity"]), AvailableBalance: number(balance["availableBalance"]),
	}, nil
}

func (e *AgentExchange) Instrument(ctx context.Context, symbol string) (execution.InstrumentSpec, error) {
	if err := ctx.Err(); err != nil {
		return execution.InstrumentSpec{}, err
	}
	return execution.InstrumentSpec{QtyStep: e.Trader.getQtyStep(symbol)}, nil
}

func (e *AgentExchange) PositionMode(ctx context.Context, symbol string) (execution.PositionMode, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Existing Bybit order paths explicitly use positionIdx=0. Keep the
	// adapter truthful to that configured one-way mode until a dedicated mode
	// query is added to the Bybit client seam.
	return execution.PositionModeOneWay, nil
}

func (e *AgentExchange) Place(ctx context.Context, req execution.OrderRequest) (execution.OrderResult, error) {
	if err := ctx.Err(); err != nil {
		return execution.OrderResult{}, err
	}
	var raw map[string]interface{}
	var err error
	if req.ReduceOnly {
		if strings.EqualFold(req.Side, "buy") || strings.EqualFold(req.Side, "short") {
			raw, err = e.Trader.CloseShort(req.Symbol, req.Quantity)
		} else {
			raw, err = e.Trader.CloseLong(req.Symbol, req.Quantity)
		}
	} else if strings.EqualFold(req.Side, "buy") {
		raw, err = e.Trader.OpenLong(req.Symbol, req.Quantity, req.Leverage)
	} else {
		raw, err = e.Trader.OpenShort(req.Symbol, req.Quantity, req.Leverage)
	}
	if err != nil {
		return execution.OrderResult{}, err
	}
	return execution.OrderResult{OrderID: stringValue(raw["orderId"]), Status: execution.OrderStatusNew}, nil
}

func (e *AgentExchange) GetOrder(ctx context.Context, symbol, orderID string) (execution.OrderState, error) {
	if err := ctx.Err(); err != nil {
		return execution.OrderState{}, err
	}
	raw, err := e.Trader.GetOrderStatus(symbol, orderID)
	if err != nil {
		return execution.OrderState{}, err
	}
	return execution.OrderState{OrderID: orderID, Status: stringValue(raw["status"])}, nil
}

func (e *AgentExchange) SetProtection(ctx context.Context, req execution.ProtectionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	positionSide := "LONG"
	if strings.EqualFold(req.Side, "sell") || strings.EqualFold(req.Side, "short") {
		positionSide = "SHORT"
	}
	if req.StopLoss != nil {
		if err := e.Trader.SetStopLoss(req.Symbol, positionSide, req.Quantity, *req.StopLoss); err != nil {
			return err
		}
	}
	if req.TakeProfit != nil {
		if err := e.Trader.SetTakeProfit(req.Symbol, positionSide, req.Quantity, *req.TakeProfit); err != nil {
			return err
		}
	}
	return nil
}

func positionIndex(mode, side string) int {
	if strings.EqualFold(mode, "hedge") {
		if strings.EqualFold(side, "sell") || strings.EqualFold(side, "short") {
			return 2
		}
		return 1
	}
	return 0
}

func number(value interface{}) float64 {
	if number, ok := value.(float64); ok {
		return number
	}
	return 0
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
