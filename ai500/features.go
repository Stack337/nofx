package ai500

import (
	"errors"
	"math"
	"strings"
	"time"

	"nofx/market"
	"nofx/parity"
)

var requiredTimeframes = []string{"5m", "15m", "1h", "4h"}

type TimeframeFeatures struct {
	Close         float64 `json:"close"`
	ReturnPct     float64 `json:"return_pct"`
	VolatilityPct float64 `json:"volatility_pct"`
	EMAFast       float64 `json:"ema_fast"`
	EMASlow       float64 `json:"ema_slow"`
	RSI           float64 `json:"rsi"`
	MACD          float64 `json:"macd"`
	ATRPercent    float64 `json:"atr_percent"`
	VolumeRatio   float64 `json:"volume_ratio"`
}

type FeatureSnapshot struct {
	Symbol             string                       `json:"symbol"`
	BuiltAt            time.Time                    `json:"built_at"`
	SourceTimestamp    time.Time                    `json:"source_timestamp"`
	CurrentPrice       float64                      `json:"current_price"`
	Timeframes         map[string]TimeframeFeatures `json:"timeframes"`
	BTCRegime          string                       `json:"btc_regime"`
	DataComplete       bool                         `json:"data_complete"`
	RequiredTimeframes []string                     `json:"required_timeframes"`
	MissingTimeframes  []string                     `json:"missing_timeframes"`
}

type FeatureUnavailableError struct{ Reason string }

func (e *FeatureUnavailableError) Error() string { return "AI500 features unavailable: " + e.Reason }

func BuildFeatures(now time.Time, symbol string, snapshot parity.MarketSnapshot, btc *parity.MarketSnapshot) (FeatureSnapshot, error) {
	symbol = market.Normalize(strings.TrimSpace(symbol))
	if symbol == "" {
		return FeatureSnapshot{}, &FeatureUnavailableError{Reason: "symbol is required"}
	}
	if snapshot.CurrentPrice <= 0 || snapshot.PriceTimestamp.IsZero() {
		return FeatureSnapshot{}, &FeatureUnavailableError{Reason: "price is invalid"}
	}
	result := FeatureSnapshot{
		Symbol: symbol, BuiltAt: now.UTC(), SourceTimestamp: snapshot.PriceTimestamp.UTC(),
		CurrentPrice: snapshot.CurrentPrice, Timeframes: map[string]TimeframeFeatures{},
		BTCRegime: "neutral", RequiredTimeframes: append([]string(nil), requiredTimeframes...),
	}
	for _, timeframe := range requiredTimeframes {
		candles := closedCandles(snapshot.Candles[timeframe], now)
		if len(candles) < 30 {
			result.MissingTimeframes = append(result.MissingTimeframes, timeframe)
			continue
		}
		features, err := calculateTimeframe(candles)
		if err != nil {
			return FeatureSnapshot{}, &FeatureUnavailableError{Reason: timeframe + ": " + err.Error()}
		}
		result.Timeframes[timeframe] = features
	}
	if len(result.MissingTimeframes) > 0 {
		return FeatureSnapshot{}, &FeatureUnavailableError{Reason: "required timeframe data is missing"}
	}
	result.DataComplete = true
	if btc != nil {
		candles := closedCandles(btc.Candles["1h"], now)
		if len(candles) >= 30 {
			features, err := calculateTimeframe(candles)
			if err == nil {
				switch {
				case features.ReturnPct > 0 && features.EMAFast > features.EMASlow:
					result.BTCRegime = "bullish"
				case features.ReturnPct < 0 && features.EMAFast < features.EMASlow:
					result.BTCRegime = "bearish"
				}
			}
		}
	}
	return result, nil
}

func closedCandles(candles []parity.CandleSnapshot, now time.Time) []parity.CandleSnapshot {
	result := make([]parity.CandleSnapshot, 0, len(candles))
	for _, candle := range candles {
		if candle.Forming || candle.CloseTime.After(now) || candle.Close <= 0 {
			continue
		}
		result = append(result, candle)
	}
	return result
}

func calculateTimeframe(candles []parity.CandleSnapshot) (TimeframeFeatures, error) {
	if len(candles) < 30 {
		return TimeframeFeatures{}, errors.New("at least 30 closed candles are required")
	}
	closes := make([]float64, len(candles))
	volumes := make([]float64, len(candles))
	for i, candle := range candles {
		closes[i] = candle.Close
		volumes[i] = candle.Volume
	}
	last := closes[len(closes)-1]
	baseIndex := len(closes) - 6
	features := TimeframeFeatures{
		Close: last, ReturnPct: percentChange(closes[baseIndex], last),
		VolatilityPct: returnVolatility(closes, 20),
		EMAFast:       ema(closes, 12),
	}
	features.EMASlow = ema(closes, 26)
	features.RSI = rsi(closes, 14)
	features.MACD = features.EMAFast - features.EMASlow
	features.ATRPercent = atrPercent(candles, 14)
	features.VolumeRatio = volumeRatio(volumes, 20)
	for _, value := range []float64{features.ReturnPct, features.VolatilityPct, features.EMAFast, features.EMASlow, features.RSI, features.MACD, features.ATRPercent, features.VolumeRatio} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return TimeframeFeatures{}, errors.New("indicator is not finite")
		}
	}
	return features, nil
}

func percentChange(from, to float64) float64 {
	if from == 0 {
		return 0
	}
	return (to/from - 1) * 100
}

func ema(values []float64, period int) float64 {
	start := 0
	if len(values) > period {
		start = len(values) - period
	}
	result := values[start]
	alpha := 2.0 / float64(period+1)
	for _, value := range values[start+1:] {
		result = alpha*value + (1-alpha)*result
	}
	return result
}

func rsi(values []float64, period int) float64 {
	start := len(values) - period - 1
	if start < 0 {
		start = 0
	}
	var gains, losses float64
	for i := start + 1; i < len(values); i++ {
		delta := values[i] - values[i-1]
		if delta > 0 {
			gains += delta
		} else {
			losses -= delta
		}
	}
	if losses == 0 {
		if gains == 0 {
			return 50
		}
		return 100
	}
	return 100 - 100/(1+gains/losses)
}

func returnVolatility(values []float64, period int) float64 {
	start := len(values) - period
	if start < 1 {
		start = 1
	}
	returns := make([]float64, 0, len(values)-start)
	for i := start; i < len(values); i++ {
		returns = append(returns, percentChange(values[i-1], values[i]))
	}
	if len(returns) == 0 {
		return 0
	}
	var mean float64
	for _, value := range returns {
		mean += value
	}
	mean /= float64(len(returns))
	var variance float64
	for _, value := range returns {
		variance += (value - mean) * (value - mean)
	}
	return math.Sqrt(variance / float64(len(returns)))
}

func atrPercent(candles []parity.CandleSnapshot, period int) float64 {
	start := len(candles) - period
	if start < 1 {
		start = 1
	}
	var total float64
	for i := start; i < len(candles); i++ {
		previous := candles[i-1].Close
		trueRange := math.Max(candles[i].High-candles[i].Low, math.Max(math.Abs(candles[i].High-previous), math.Abs(candles[i].Low-previous)))
		total += trueRange
	}
	count := len(candles) - start
	if count == 0 || candles[len(candles)-1].Close == 0 {
		return 0
	}
	return total / float64(count) / candles[len(candles)-1].Close * 100
}

func volumeRatio(volumes []float64, period int) float64 {
	if len(volumes) < 2 {
		return 0
	}
	end := len(volumes) - 1
	start := end - period
	if start < 0 {
		start = 0
	}
	var total float64
	for _, volume := range volumes[start:end] {
		total += volume
	}
	if total == 0 || end == start {
		return 0
	}
	return volumes[end] / (total / float64(end-start))
}
