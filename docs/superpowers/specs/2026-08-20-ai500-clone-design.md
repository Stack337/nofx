# AI500 Clone Design

**Date:** 2026-08-20  
**Status:** Design approved for review; implementation not started

## Goal

Build an independent AI500-like scoring system for the NOFX/VergeX parity clone. The system must expose a VergeX-compatible aggregate score from 0 to 100 while preserving richer internal signals for directional trading, evaluation, and future model training.

The first release is shadow-only. It may read public market data, call the configured AI provider, persist scores, and feed candidate selection in the clone, but it must not submit real Bybit orders.

## Scope

The first release includes:

- Bybit public market-data ingestion;
- normalized feature construction for 5m, 15m, 1h, and 4h windows;
- LLM-generated structured scoring;
- deterministic validation and safety gates;
- append-only scoring observations and outcome labels;
- a future local-ML training boundary;
- integration with `parity.CandidateSnapshot` and the existing shadow runner;
- API/read models for score history and diagnostics after the core is stable.

It does not include live trading, autonomous model retraining in production, copying undocumented VergeX internals, or changing the existing VergeX/9Router/CLI Proxy services.

## Score contract

Each evaluated symbol produces a versioned observation:

```json
{
  "symbol": "BTCUSDT",
  "score": 0,
  "long_score": 0,
  "short_score": 0,
  "confidence": 0,
  "regime": "bullish|bearish|neutral",
  "reasons": ["..."],
  "feature_timestamp": "2026-08-20T00:00:00Z",
  "model_provider": "openrouter",
  "model_name": "...",
  "prompt_version": "ai500-v1",
  "schema_version": "ai500-score-v1"
}
```

All numeric scores are constrained to 0–100. `regime` is an enum. Reasons are short, bounded strings and never become a source of trading authority. The clone consumes the aggregate `score` for compatibility and retains the directional fields for filtering and analysis.

## Architecture

```text
Bybit public data
    -> market-data client
    -> feature builder
    -> LLM score workflow
    -> schema/range/freshness validation
    -> score store + outcome-label job
    -> candidate adapter
    -> parity shadow runner
```

### Market-data client

Use public Bybit endpoints already represented by the repository's market abstractions. The client must have bounded timeouts, retry only idempotent reads, normalize symbols, and attach a source timestamp to every snapshot. Missing or stale data causes an explicit unavailable result rather than a guessed score.

### Feature builder

Construct a stable, JSON-serializable feature vector from OHLCV and derivatives data:

- multi-timeframe returns and volatility;
- EMA relationships, RSI, MACD, ATR, and volume behavior;
- open-interest change, funding, basis, and liquidity where available;
- BTC market-regime context;
- data freshness and completeness flags.

The builder is deterministic and independently testable. It does not call an LLM and does not embed exchange credentials in the feature payload.

### LLM score workflow

The LLM receives only the normalized feature vector and a versioned system prompt. It must return the score contract using strict JSON output. The workflow records provider/model/prompt/schema versions, latency, and validation outcome, but never stores API keys.

The LLM is the primary scorer in v1. A deterministic validator remains authoritative for numeric ranges, enum values, data freshness, missing features, and minimum liquidity. Invalid output becomes an unavailable score and cannot enter the candidate pool.

### Future local-ML workflow

Every valid observation may be joined later with forward outcomes at 15m, 1h, 4h, and 24h. The dataset boundary stores the feature snapshot, LLM output, and labels separately from execution state. A local model can later be trained and evaluated against the LLM using precision@top-k, directional accuracy, return, drawdown, turnover, and score stability.

No automatic promotion from local model to production is allowed. Promotion requires an offline evaluation artifact and an explicit configuration change in shadow mode first.

### Clone integration

The AI500 adapter maps the latest valid observation into `parity.CandidateSnapshot`:

- `Symbol` receives the normalized symbol;
- `Score` receives the aggregate score;
- source metadata identifies `ai500`;
- directional score, confidence, regime, and observation ID remain available to the parity context and diagnostics.

Candidate selection may rank by aggregate score, but risk gates and the shadow execution contract remain separate. AI500 never directly places orders.

## Persistence and privacy

Persist only structured feature snapshots, score observations, model metadata, validation results, and outcome labels. Do not persist raw prompts, provider authorization headers, API keys, or full account payloads. Use append-only records with deterministic IDs or fingerprints to deduplicate repeated observations.

Retention and deletion must be explicit. Debug logs contain event names, counts, IDs, and timings—not market prompt text or credentials.

## Failure handling

- Public-data timeout: mark the snapshot unavailable and skip scoring.
- Partial feature set: score only if the configured required-feature set is complete; otherwise record a validation failure.
- LLM timeout/rate limit/invalid JSON: retry boundedly for idempotent scoring, then return unavailable without blocking the trading service.
- Stale score: exclude it from candidate selection after its configured TTL.
- Persistence failure: keep the current cycle safe and report an observable error; never fabricate a score.
- Shadow execution failure: persist the cycle failure and keep live execution disabled.

## Testing strategy

- Unit tests for symbol normalization, indicators, feature determinism, score validation, freshness, and deduplication.
- Contract tests for the LLM JSON schema, malformed responses, missing fields, and provider errors.
- Fixture tests for known market snapshots and expected score ranges.
- Replay tests for score observations joined to forward outcomes.
- Integration tests using a fake Bybit public API and fake AI provider.
- Shadow-mode tests proving that no private Bybit order endpoint is called.
- Property tests for score bounds and deterministic feature serialization.

## Rollout

1. Implement the score contract, validator, and deterministic feature builder.
2. Add fake-provider workflow tests and persist shadow observations.
3. Feed scores into the clone's candidate pool while keeping execution shadow-only.
4. Add outcome labeling and offline evaluation reports.
5. Add local-ML training as a separate, opt-in job.
6. Consider live-mode work only after parity, risk, and exchange-contract tests pass and the user explicitly confirms it.

## Decisions and non-goals

- The LLM is the primary scorer in v1; local ML is a later learner from labeled observations.
- The aggregate score remains VergeX-compatible; directional fields are additive.
- The design deliberately avoids modifying the working VergeX proxy and its SSE path.
- Real trading is disabled throughout this project phase.
