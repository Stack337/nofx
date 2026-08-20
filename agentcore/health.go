package agentcore

import (
	"context"
	"errors"

	"nofx/agent"
	"nofx/execution"
)

type ExchangeHealthGate struct {
	Exchange execution.Exchange
	Symbol   string
}

func (g ExchangeHealthGate) Ready(ctx context.Context, _ agent.Agent) error {
	if g.Exchange == nil {
		return errors.New("live health exchange is not configured")
	}
	if _, err := g.Exchange.Account(ctx); err != nil {
		return err
	}
	symbol := g.Symbol
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	mode, err := g.Exchange.PositionMode(ctx, symbol)
	if err != nil {
		return err
	}
	if mode != execution.PositionModeOneWay {
		return &execution.ExchangeError{Code: "position_mode_mismatch", Message: "live preflight requires one-way position mode"}
	}
	if _, err := g.Exchange.Instrument(ctx, symbol); err != nil {
		return err
	}
	return nil
}
