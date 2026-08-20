package ai500

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"nofx/market"
	"nofx/parity"
)

type Labeler interface {
	Label(context.Context, ScoreObservation, map[string]parity.MarketSnapshot) ([]OutcomeLabel, error)
}

type MarketLabeler struct{}

func NewMarketLabeler() *MarketLabeler { return &MarketLabeler{} }

func (l *MarketLabeler) Label(ctx context.Context, observation ScoreObservation, markets map[string]parity.MarketSnapshot) ([]OutcomeLabel, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(observation.ID) == "" || observation.FeatureTimestamp.IsZero() {
		return nil, errors.New("AI500 observation ID and feature timestamp are required for labeling")
	}
	symbol := market.Normalize(strings.TrimSpace(observation.Symbol))
	snapshot, ok := findLabelMarket(markets, symbol)
	if !ok {
		return nil, fmt.Errorf("market snapshot for %s is unavailable", symbol)
	}
	candles := completedLabelCandles(snapshot)
	baselineIndex := -1
	for index := range candles {
		if !candles[index].CloseTime.After(observation.FeatureTimestamp) {
			baselineIndex = index
		}
	}
	if baselineIndex < 0 || candles[baselineIndex].Close <= 0 {
		return nil, errors.New("baseline candle is unavailable")
	}

	baseline := candles[baselineIndex].Close
	short := observation.ShortScore > observation.LongScore
	result := make([]OutcomeLabel, 0, len(labelHorizons))
	for _, horizon := range labelHorizons {
		target := observation.FeatureTimestamp.Add(horizon.duration)
		futureIndex := candleAt(candles, baselineIndex+1, target)
		if futureIndex < 0 {
			continue
		}
		future := candles[futureIndex]
		result = append(result, OutcomeLabel{
			ObservationID:  observation.ID,
			Horizon:        horizon.name,
			ReturnPct:      (future.Close/baseline - 1) * 100,
			MaxDrawdownPct: labelDrawdown(candles[baselineIndex+1:futureIndex+1], baseline, short),
			LabeledAt:      future.CloseTime.UTC(),
		})
	}
	return result, nil
}

var labelHorizons = []struct {
	name     string
	duration time.Duration
}{
	{name: "15m", duration: 15 * time.Minute},
	{name: "1h", duration: time.Hour},
	{name: "4h", duration: 4 * time.Hour},
	{name: "24h", duration: 24 * time.Hour},
}

func findLabelMarket(markets map[string]parity.MarketSnapshot, symbol string) (parity.MarketSnapshot, bool) {
	for key, snapshot := range markets {
		if market.Normalize(strings.TrimSpace(key)) == symbol {
			return snapshot, true
		}
	}
	return parity.MarketSnapshot{}, false
}

func completedLabelCandles(snapshot parity.MarketSnapshot) []parity.CandleSnapshot {
	byClose := make(map[time.Time]parity.CandleSnapshot)
	for _, timeframe := range snapshot.Candles {
		for _, candle := range timeframe {
			if candle.Forming || candle.CloseTime.IsZero() || candle.Close <= 0 {
				continue
			}
			candle.CloseTime = candle.CloseTime.UTC()
			if _, exists := byClose[candle.CloseTime]; !exists {
				byClose[candle.CloseTime] = candle
			}
		}
	}
	result := make([]parity.CandleSnapshot, 0, len(byClose))
	for _, candle := range byClose {
		result = append(result, candle)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CloseTime.Before(result[j].CloseTime) })
	return result
}

func candleAt(candles []parity.CandleSnapshot, start int, target time.Time) int {
	for index := start; index < len(candles); index++ {
		if candles[index].CloseTime.Equal(target) {
			return index
		}
		if candles[index].CloseTime.After(target) {
			break
		}
	}
	return -1
}

func labelDrawdown(candles []parity.CandleSnapshot, baseline float64, short bool) float64 {
	worst := 0.0
	for _, candle := range candles {
		adverse := (candle.Low/baseline - 1) * 100
		if short {
			adverse = -(candle.High/baseline - 1) * 100
		}
		if adverse < worst {
			worst = adverse
		}
	}
	return worst
}
