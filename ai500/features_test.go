package ai500

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"nofx/parity"
)

func featureMarket(now time.Time, symbol string) parity.MarketSnapshot {
	market := parity.MarketSnapshot{
		Symbol:         symbol,
		CurrentPrice:   120,
		MarkPrice:      120,
		PriceTimestamp: now.Add(-time.Minute),
		Candles:        map[string][]parity.CandleSnapshot{},
	}
	for _, timeframe := range []string{"5m", "15m", "1h", "4h"} {
		candles := make([]parity.CandleSnapshot, 0, 40)
		step := timeframeDuration(timeframe)
		start := now.Add(-step * 40)
		for i := 0; i < 40; i++ {
			closePrice := 80 + float64(i)
			openTime := start.Add(step * time.Duration(i))
			candles = append(candles, parity.CandleSnapshot{
				OpenTime: openTime, CloseTime: openTime.Add(step),
				Open: closePrice - 0.5, High: closePrice + 1,
				Low: closePrice - 1, Close: closePrice, Volume: 100 + float64(i),
			})
		}
		candles[len(candles)-1].Forming = true
		market.Candles[timeframe] = candles
	}
	return market
}

func timeframeDuration(timeframe string) time.Duration {
	switch timeframe {
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	default:
		return 4 * time.Hour
	}
}

func TestBuildFeaturesIsDeterministicAndUsesClosedCandles(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	market := featureMarket(now, "ETHUSDT")
	first, err := BuildFeatures(now, "ethusdt", market, nil)
	if err != nil {
		t.Fatalf("BuildFeatures() error = %v", err)
	}
	second, err := BuildFeatures(now, "ETHUSDT", market, nil)
	if err != nil {
		t.Fatalf("BuildFeatures() second error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("BuildFeatures is not deterministic")
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if len(encoded) == 0 || first.Symbol != "ETHUSDT" || !first.DataComplete {
		t.Fatalf("invalid feature snapshot: %+v", first)
	}
	for timeframe, features := range first.Timeframes {
		if features.Close != 118 || features.ReturnPct <= 0 {
			t.Errorf("%s features used forming candle or missed return: %+v", timeframe, features)
		}
		for name, value := range map[string]float64{
			"return": features.ReturnPct, "volatility": features.VolatilityPct,
			"ema_fast": features.EMAFast, "ema_slow": features.EMASlow,
			"rsi": features.RSI, "macd": features.MACD,
			"atr": features.ATRPercent, "volume_ratio": features.VolumeRatio,
		} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Errorf("%s %s is not finite: %v", timeframe, name, value)
			}
		}
	}
}

func TestBuildFeaturesIncludesBTCRegimeAndRequiredTimeframes(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	btc := featureMarket(now, "BTCUSDT")
	features, err := BuildFeatures(now, "ETHUSDT", featureMarket(now, "ETHUSDT"), &btc)
	if err != nil {
		t.Fatalf("BuildFeatures() error = %v", err)
	}
	if features.BTCRegime != "bullish" {
		t.Fatalf("BTC regime = %q, want bullish", features.BTCRegime)
	}
	if !reflect.DeepEqual(features.RequiredTimeframes, []string{"5m", "15m", "1h", "4h"}) {
		t.Fatalf("required timeframes = %#v", features.RequiredTimeframes)
	}
}

func TestBuildFeaturesRejectsMissingTimeframeAndInvalidPrice(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	market := featureMarket(now, "ETHUSDT")
	delete(market.Candles, "1h")
	if _, err := BuildFeatures(now, "ETHUSDT", market, nil); err == nil {
		t.Fatal("missing timeframe was accepted")
	}
	market = featureMarket(now, "ETHUSDT")
	market.CurrentPrice = 0
	if _, err := BuildFeatures(now, "ETHUSDT", market, nil); err == nil {
		t.Fatal("invalid price was accepted")
	}
}
