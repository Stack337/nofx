package parity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"nofx/market"
	paritydomain "nofx/parity/domain"
)

func TestAI500ParityShadowCycleNeverCallsPrivateBybitOrderPath(t *testing.T) {
	var orderCalls atomic.Int32
	bybit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v5/order/create" || request.URL.Path == "/v5/order/cancel-all" {
			orderCalls.Add(1)
			http.Error(w, "private order path must not be called in shadow mode", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer bybit.Close()

	now := time.Now().UTC()
	exchange := &shadowHTTPExchange{baseURL: bybit.URL}
	candles := &shadowHTTPCandles{baseURL: bybit.URL, now: now}
	contextProvider := NewExchangeContextProvider(exchange, candles, func(context.Context) ([]CandidateSnapshot, error) {
		return []CandidateSnapshot{{Symbol: "SOLUSDT", Score: 90, LongScore: 85, Confidence: 80, Sources: []string{"ai500"}}}, nil
	})
	persistence := newFakeCyclePersistence()
	execution := NewShadowExecution()
	runner := NewRunner(RunnerConfig{AgentID: "agent-ai500", Shadow: true, Deadline: time.Second, RiskConfig: defaultRiskConfig()},
		persistence, contextProvider, fakeAIWorkflow{decision: validLong(now)}, execution, noopSynchronizer{})
	runner.idGenerator = func() string { return "cycle-ai500-shadow" }

	result, err := runner.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("run shadow cycle: %v", err)
	}
	if result.State != paritydomain.CycleCompleted || !result.Execution.Simulated {
		t.Fatalf("result = %+v", result)
	}
	if orderCalls.Load() != 0 || execution.LiveRequests() != 0 {
		t.Fatalf("private order calls = %d, live requests = %d", orderCalls.Load(), execution.LiveRequests())
	}
}

type shadowHTTPExchange struct{ baseURL string }

func (s *shadowHTTPExchange) GetBalance() (map[string]any, error) {
	if err := shadowGET(s.baseURL + "/v5/account/wallet-balance"); err != nil {
		return nil, err
	}
	return map[string]any{"totalEquity": 100.0, "availableBalance": 100.0}, nil
}
func (s *shadowHTTPExchange) GetPositions() ([]map[string]any, error) {
	if err := shadowGET(s.baseURL + "/v5/position/list"); err != nil {
		return nil, err
	}
	return []map[string]any{}, nil
}
func (s *shadowHTTPExchange) GetMarketPrice(string) (float64, error) {
	if err := shadowGET(s.baseURL + "/v5/market/tickers"); err != nil {
		return 0, err
	}
	return 100, nil
}

type shadowHTTPCandles struct {
	baseURL string
	now     time.Time
}

func (s *shadowHTTPCandles) GetKlines(string, string, int) ([]market.Kline, error) {
	if err := shadowGET(s.baseURL + "/v5/market/kline"); err != nil {
		return nil, err
	}
	return []market.Kline{{OpenTime: s.now.Add(-time.Minute).UnixMilli(), CloseTime: s.now.UnixMilli(), Open: 100, High: 101, Low: 99, Close: 100, Volume: 10}}, nil
}

func shadowGET(url string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}
